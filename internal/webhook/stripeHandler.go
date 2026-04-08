// Package webhook implements unauthenticated inbound webhook handlers.
//
// The Stripe webhook verifies the Stripe-Signature header on the raw request
// body before dispatching events. It is mounted outside the authenticated
// route group because Stripe will retry any non-2xx response — after a
// signature check passes, the handler must always return 200 even when it
// decides to ignore the event.
package webhook

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"

	"github.com/google/uuid"
	"github.com/labstack/echo/v5"
	"github.com/rs/zerolog/log"
	"github.com/runernotes/runer-api/internal/api"
	"github.com/stripe/stripe-go/v81"
	"github.com/stripe/stripe-go/v81/webhook"
)

// EventVerifier validates the Stripe-Signature header against the raw payload
// and returns the decoded event. It exists as an interface so tests can
// bypass signature verification while still exercising the dispatch logic.
type EventVerifier interface {
	Verify(payload []byte, signatureHeader string) (stripe.Event, error)
}

// sdkVerifier wraps webhook.ConstructEvent from stripe-go.
type sdkVerifier struct{ secret string }

// NewSDKVerifier returns the production EventVerifier backed by stripe-go.
func NewSDKVerifier(secret string) EventVerifier {
	return &sdkVerifier{secret: secret}
}

func (v *sdkVerifier) Verify(payload []byte, signatureHeader string) (stripe.Event, error) {
	return webhook.ConstructEvent(payload, signatureHeader, v.secret)
}

// UsersRepository captures the user operations needed by the webhook handler.
// It is deliberately narrow so handler tests can mock it in-memory.
type UsersRepository interface {
	FindByStripeCustomerID(ctx context.Context, stripeCustomerID string) (UserRecord, error)
	UpdatePlan(ctx context.Context, id uuid.UUID, plan string) error
	UpdateStripeSubscriptionID(ctx context.Context, id uuid.UUID, subID *string) error
}

// UserRecord is the minimal projection of the user row the webhook needs.
type UserRecord struct {
	ID uuid.UUID
}

// Handler processes Stripe webhook events.
type Handler struct {
	billingEnabled bool
	verifier       EventVerifier
	usersRepo      UsersRepository
}

// NewHandler constructs a webhook handler. verifier may be nil when billing
// is disabled — in that case HandleStripe short-circuits to 200 without
// attempting signature verification.
func NewHandler(billingEnabled bool, verifier EventVerifier, usersRepo UsersRepository) *Handler {
	return &Handler{
		billingEnabled: billingEnabled,
		verifier:       verifier,
		usersRepo:      usersRepo,
	}
}

// HandleStripe handles POST /api/v1/webhooks/stripe. It is unauthenticated and
// must always return 200 once the request has been accepted (valid signature)
// because Stripe retries on any non-2xx response.
func (h *Handler) HandleStripe(c *echo.Context) error {
	// When billing is disabled we discard the event without looking at it.
	// We still return 200 so the Stripe endpoint is not marked as down in
	// self-hosted deployments that happen to have a webhook configured.
	if !h.billingEnabled || h.verifier == nil {
		// Drain the body to allow connection reuse.
		_, _ = io.Copy(io.Discard, c.Request().Body)
		return c.JSON(http.StatusOK, api.MessageResponse{Message: "ok"})
	}

	// Stripe validates signatures against the exact bytes we received, so we
	// must read the body before any JSON parsing.
	body, err := io.ReadAll(c.Request().Body)
	if err != nil {
		log.Error().Err(err).Msg("stripe webhook: failed to read request body")
		return c.JSON(http.StatusBadRequest, api.ErrorResponse{
			Error: "invalid request body",
			Code:  "INVALID_BODY",
		})
	}

	signature := c.Request().Header.Get("Stripe-Signature")
	event, err := h.verifier.Verify(body, signature)
	if err != nil {
		log.Warn().Err(err).Msg("stripe webhook: signature verification failed")
		return c.JSON(http.StatusBadRequest, api.ErrorResponse{
			Error: "invalid stripe signature",
			Code:  "INVALID_SIGNATURE",
		})
	}

	ctx := c.Request().Context()
	if err := h.dispatch(ctx, event); err != nil {
		// Dispatch errors are logged but do not translate to non-2xx responses:
		// Stripe retries on failure and we want at-most-once handling for
		// idempotent operations. Persistent errors can be investigated via logs.
		log.Error().
			Err(err).
			Str("event_id", event.ID).
			Str("event_type", string(event.Type)).
			Msg("stripe webhook: dispatch failed")
	}

	return c.JSON(http.StatusOK, api.MessageResponse{Message: "ok"})
}

// dispatch routes an event to the correct handler. Unknown events are a
// successful no-op — Stripe sends many event types we do not care about.
func (h *Handler) dispatch(ctx context.Context, event stripe.Event) error {
	switch event.Type {
	case "checkout.session.completed":
		return h.handleCheckoutCompleted(ctx, event)
	case "customer.subscription.deleted":
		return h.handleSubscriptionDeleted(ctx, event)
	case "invoice.payment_failed":
		return h.handlePaymentFailed(ctx, event)
	default:
		log.Debug().
			Str("event_type", string(event.Type)).
			Str("event_id", event.ID).
			Msg("stripe webhook: ignoring unhandled event type")
		return nil
	}
}

// minimalCheckoutSession mirrors the fields we need from a Stripe Checkout
// Session payload. We parse the raw JSON directly rather than relying on the
// SDK's session type because we only need two string fields and this keeps
// our decoder resilient to SDK changes.
type minimalCheckoutSession struct {
	Customer     string `json:"customer"`
	Subscription string `json:"subscription"`
}

func (h *Handler) handleCheckoutCompleted(ctx context.Context, event stripe.Event) error {
	var sess minimalCheckoutSession
	if err := json.Unmarshal(event.Data.Raw, &sess); err != nil {
		return err
	}
	if sess.Customer == "" {
		return errors.New("stripe webhook: checkout.session.completed missing customer")
	}

	user, err := h.usersRepo.FindByStripeCustomerID(ctx, sess.Customer)
	if err != nil {
		return err
	}

	if err := h.usersRepo.UpdatePlan(ctx, user.ID, "pro"); err != nil {
		return err
	}

	if sess.Subscription != "" {
		subID := sess.Subscription
		if err := h.usersRepo.UpdateStripeSubscriptionID(ctx, user.ID, &subID); err != nil {
			return err
		}
	}

	log.Info().
		Str("event", "plan_upgraded").
		Str("user_id", user.ID.String()).
		Str("plan", "pro").
		Msg("stripe webhook: plan upgraded")
	return nil
}

type minimalSubscription struct {
	Customer string `json:"customer"`
}

func (h *Handler) handleSubscriptionDeleted(ctx context.Context, event stripe.Event) error {
	var sub minimalSubscription
	if err := json.Unmarshal(event.Data.Raw, &sub); err != nil {
		return err
	}
	if sub.Customer == "" {
		return errors.New("stripe webhook: customer.subscription.deleted missing customer")
	}

	user, err := h.usersRepo.FindByStripeCustomerID(ctx, sub.Customer)
	if err != nil {
		return err
	}

	if err := h.usersRepo.UpdatePlan(ctx, user.ID, "free"); err != nil {
		return err
	}
	if err := h.usersRepo.UpdateStripeSubscriptionID(ctx, user.ID, nil); err != nil {
		return err
	}

	log.Info().
		Str("event", "plan_downgraded").
		Str("user_id", user.ID.String()).
		Str("plan", "free").
		Msg("stripe webhook: plan downgraded")
	return nil
}

type minimalInvoice struct {
	Customer string `json:"customer"`
}

func (h *Handler) handlePaymentFailed(_ context.Context, event stripe.Event) error {
	var inv minimalInvoice
	if err := json.Unmarshal(event.Data.Raw, &inv); err != nil {
		return err
	}
	log.Warn().
		Str("event", "payment_failed").
		Str("stripe_customer_id", inv.Customer).
		Msg("stripe webhook: invoice payment failed")
	return nil
}
