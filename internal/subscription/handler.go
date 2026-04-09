package subscription

import (
	"context"
	"net/http"

	"github.com/google/uuid"
	"github.com/labstack/echo/v5"
	"github.com/rs/zerolog"
	"github.com/runernotes/runer-api/internal/analytics"
	"github.com/runernotes/runer-api/internal/api"
	internalmw "github.com/runernotes/runer-api/internal/middleware"
)

// usersRepository is the minimal interface needed by the subscription handler.
// Defined locally to avoid importing the users package directly.
type usersRepository interface {
	FindByID(ctx context.Context, id uuid.UUID) (UserRecord, error)
	UpdateStripeCustomerID(ctx context.Context, id uuid.UUID, stripeCustomerID string) error
}

// UserRecord holds the user fields needed by the subscription handler.
// It is exported so that adapters in other packages can satisfy the usersRepository
// interface without importing the users package into subscription.
type UserRecord struct {
	Plan             string
	Email            string
	StripeCustomerID *string
}

// notesRepository is the minimal interface needed to count a user's live notes.
type notesRepository interface {
	CountLiveNotes(ctx context.Context, userID uuid.UUID) (int64, error)
}

// BillingConfig exposes only the fields the subscription handler needs from
// the global config. Keeping this a local struct avoids coupling the
// subscription package to the full config.Config type.
type BillingConfig struct {
	Enabled    bool
	PriceID    string
	SuccessURL string
	CancelURL  string
}

// Handler handles subscription-related HTTP requests.
type Handler struct {
	usersRepo     usersRepository
	notesRepo     notesRepository
	freeNoteLimit int
	billing       BillingConfig
	stripe        StripeClient // nil when billing is disabled
	tracker       analytics.Tracker
}

// NewHandler constructs a subscription Handler. stripeClient may be nil when
// billing is disabled — CreateCheckout will return 501 Not Implemented in that
// case.
func NewHandler(
	usersRepo usersRepository,
	notesRepo notesRepository,
	freeNoteLimit int,
	billing BillingConfig,
	stripeClient StripeClient,
	tracker analytics.Tracker,
) *Handler {
	return &Handler{
		usersRepo:     usersRepo,
		notesRepo:     notesRepo,
		freeNoteLimit: freeNoteLimit,
		billing:       billing,
		stripe:        stripeClient,
		tracker:       tracker,
	}
}

// userIDFromContext extracts the authenticated user id set by the auth middleware,
// returning false when the value is missing or the wrong type.
func userIDFromContext(c *echo.Context) (uuid.UUID, bool) {
	val := c.Get(internalmw.UserContextKey)
	if val == nil {
		return uuid.Nil, false
	}
	id, ok := val.(uuid.UUID)
	return id, ok
}

// GetSubscription handles GET /api/v1/subscription.
// It returns the authenticated user's plan, live note count, and note limit.
// For pro users note_limit is null (unlimited).
func (h *Handler) GetSubscription(c *echo.Context) error {
	userID, ok := userIDFromContext(c)
	if !ok {
		return c.JSON(http.StatusUnauthorized, api.ErrorResponse{Error: "unauthorized", Code: "UNAUTHORIZED"})
	}

	user, err := h.usersRepo.FindByID(c.Request().Context(), userID)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, api.ErrorResponse{Error: "failed to fetch subscription", Code: "INTERNAL_ERROR"})
	}

	count, err := h.notesRepo.CountLiveNotes(c.Request().Context(), userID)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, api.ErrorResponse{Error: "failed to fetch subscription", Code: "INTERNAL_ERROR"})
	}

	resp := SubscriptionResponse{
		Plan:      user.Plan,
		NoteCount: count,
		NoteLimit: nil, // pro plan: unlimited
	}

	if user.Plan == "free" {
		limit := h.freeNoteLimit
		resp.NoteLimit = &limit
	}

	return c.JSON(http.StatusOK, resp)
}

// CreateCheckout handles POST /api/v1/subscription/checkout. It creates a
// Stripe Checkout session for the authenticated user, provisioning a Stripe
// customer on first use. It returns 501 Not Implemented when BILLING_ENABLED
// is false (self-hosted mode).
func (h *Handler) CreateCheckout(c *echo.Context) error {
	if !h.billing.Enabled || h.stripe == nil {
		return c.JSON(http.StatusNotImplemented, api.ErrorResponse{
			Error: "billing is not enabled on this server",
			Code:  "BILLING_DISABLED",
		})
	}

	userID, ok := userIDFromContext(c)
	if !ok {
		return c.JSON(http.StatusUnauthorized, api.ErrorResponse{Error: "unauthorized", Code: "UNAUTHORIZED"})
	}

	ctx := c.Request().Context()

	user, err := h.usersRepo.FindByID(ctx, userID)
	if err != nil {
		zerolog.Ctx(ctx).Error().Err(err).Str("user_id", userID.String()).Msg("checkout: failed to load user")
		return c.JSON(http.StatusInternalServerError, api.ErrorResponse{
			Error: "failed to load user",
			Code:  "INTERNAL_ERROR",
		})
	}

	// Create or reuse Stripe customer.
	var customerID string
	if user.StripeCustomerID != nil && *user.StripeCustomerID != "" {
		customerID = *user.StripeCustomerID
	} else {
		if user.Email == "" {
			return c.JSON(http.StatusInternalServerError, api.ErrorResponse{
				Error: "user has no email on record",
				Code:  "INTERNAL_ERROR",
			})
		}
		newID, cerr := h.stripe.CreateCustomer(ctx, user.Email)
		if cerr != nil {
			zerolog.Ctx(ctx).Error().Err(cerr).Str("user_id", userID.String()).Msg("checkout: stripe customer creation failed")
			return c.JSON(http.StatusBadGateway, api.ErrorResponse{
				Error: "failed to create stripe customer",
				Code:  "STRIPE_ERROR",
			})
		}
		if err := h.usersRepo.UpdateStripeCustomerID(ctx, userID, newID); err != nil {
			zerolog.Ctx(ctx).Error().Err(err).Str("user_id", userID.String()).Msg("checkout: failed to persist stripe customer id")
			return c.JSON(http.StatusInternalServerError, api.ErrorResponse{
				Error: "failed to persist stripe customer id",
				Code:  "INTERNAL_ERROR",
			})
		}
		customerID = newID
	}

	url, err := h.stripe.CreateCheckoutSession(ctx, CheckoutSessionParams{
		CustomerID: customerID,
		PriceID:    h.billing.PriceID,
		SuccessURL: h.billing.SuccessURL,
		CancelURL:  h.billing.CancelURL,
	})
	if err != nil {
		zerolog.Ctx(ctx).Error().Err(err).Str("user_id", userID.String()).Msg("checkout: stripe session creation failed")
		return c.JSON(http.StatusBadGateway, api.ErrorResponse{
			Error: "failed to create checkout session",
			Code:  "STRIPE_ERROR",
		})
	}

	zerolog.Ctx(ctx).Info().
		Str("event", "checkout_session_created").
		Str("user_id", userID.String()).
		Str("stripe_customer_id", customerID).
		Msg("stripe checkout session created")

	h.tracker.Capture("subscription.checkout_started", userID.String(), nil)
	return c.JSON(http.StatusOK, CheckoutResponse{CheckoutURL: url})
}
