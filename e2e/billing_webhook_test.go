package e2e_test

import (
	"errors"
	"fmt"
	"net/http"
	"sync"
	"testing"

	"github.com/gavv/httpexpect/v2"
	"github.com/google/uuid"
	"github.com/runernotes/runer-api/internal/users"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stripe/stripe-go/v81"
	"gorm.io/gorm"
)

// fakeEventVerifier is a webhook.EventVerifier that returns a pre-programmed
// event or error. Individual tests mutate the `next` field before calling the
// endpoint to control which event the handler dispatches.
type fakeEventVerifier struct {
	mu sync.Mutex

	next    stripe.Event
	nextErr error

	lastPayload []byte
	lastHeader  string
}

func (f *fakeEventVerifier) Verify(payload []byte, header string) (stripe.Event, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.lastPayload = append([]byte(nil), payload...)
	f.lastHeader = header
	if f.nextErr != nil {
		return stripe.Event{}, f.nextErr
	}
	return f.next, nil
}

// seededUserInline registers a new user via the API and seeds a Stripe customer
// id directly into the database. It returns the user's id and the Stripe id so
// webhook tests can reference them.
func seededUserInline(
	t *testing.T,
	e *httpexpect.Expect,
	mock *mockEmailSender,
	db *gorm.DB,
	stripeCustomerID string,
) (uuid.UUID, string) {
	t.Helper()

	suffix := uuid.NewString()
	email := fmt.Sprintf("user-%s@example.com", suffix)

	// Register — we do not need the access token, just the user row.
	registerAndLogin(t, e, mock, suffix)

	var u users.User
	require.NoError(t, db.Where("email = ?", email).First(&u).Error)

	require.NoError(t, db.Model(&users.User{}).
		Where("id = ?", u.ID).
		Update("stripe_customer_id", stripeCustomerID).Error)

	return u.ID, email
}

// TestStripeWebhook_InvalidSignature_Returns400 verifies the handler refuses
// events that fail signature verification, returning 400.
func TestStripeWebhook_InvalidSignature_Returns400(t *testing.T) {
	verifier := &fakeEventVerifier{nextErr: errors.New("invalid signature")}

	srv, _, _ := newTestServer(t, testServerOpts{
		billingEnabled: true,
		stripeVerifier: verifier,
	})
	e := newExpect(t, srv)

	e.POST("/api/v1/webhooks/stripe").
		WithHeader("Stripe-Signature", "t=0,v1=deadbeef").
		WithBytes([]byte(`{}`)).
		Expect().
		Status(http.StatusBadRequest).
		JSON().Object().
		HasValue("code", "INVALID_SIGNATURE")
}

// TestStripeWebhook_BillingDisabled_Returns200 verifies that when billing is
// disabled the endpoint still accepts events and returns 200 so Stripe does
// not mark the endpoint as down.
func TestStripeWebhook_BillingDisabled_Returns200(t *testing.T) {
	srv, _, _ := newTestServer(t) // billing disabled by default
	e := newExpect(t, srv)

	e.POST("/api/v1/webhooks/stripe").
		WithHeader("Stripe-Signature", "t=0,v1=anything").
		WithBytes([]byte(`{"type":"checkout.session.completed"}`)).
		Expect().
		Status(http.StatusOK)
}

// TestStripeWebhook_CheckoutCompleted_UpgradesUser exercises the full happy
// path: a verified event upgrades the matching user to the pro plan and
// stores the subscription id.
func TestStripeWebhook_CheckoutCompleted_UpgradesUser(t *testing.T) {
	verifier := &fakeEventVerifier{}

	srv, mock, db := newTestServer(t, testServerOpts{
		billingEnabled: true,
		stripeVerifier: verifier,
	})
	e := newExpect(t, srv)

	stripeCustomerID := "cus_upgrade_" + uuid.NewString()
	userID, _ := seededUserInline(t, e, mock, db, stripeCustomerID)

	// Program the next verified event as a checkout.session.completed for
	// this user's Stripe customer id.
	verifier.next = stripe.Event{
		ID:   "evt_checkout_1",
		Type: "checkout.session.completed",
		Data: &stripe.EventData{
			Raw: []byte(fmt.Sprintf(`{"customer":%q,"subscription":"sub_pro_123"}`, stripeCustomerID)),
		},
	}

	e.POST("/api/v1/webhooks/stripe").
		WithHeader("Stripe-Signature", "t=0,v1=fake").
		WithBytes([]byte(`{}`)).
		Expect().
		Status(http.StatusOK)

	// DB must now show plan=pro and the subscription id stored.
	var u users.User
	require.NoError(t, db.Where("id = ?", userID).First(&u).Error)
	assert.Equal(t, users.PlanPro, u.Plan)
	require.NotNil(t, u.StripeSubscriptionID)
	assert.Equal(t, "sub_pro_123", *u.StripeSubscriptionID)
}

// TestStripeWebhook_SubscriptionDeleted_DowngradesUser verifies that a
// customer.subscription.deleted event flips the user back to the free plan
// and clears their stored subscription id.
func TestStripeWebhook_SubscriptionDeleted_DowngradesUser(t *testing.T) {
	verifier := &fakeEventVerifier{}

	srv, mock, db := newTestServer(t, testServerOpts{
		billingEnabled: true,
		stripeVerifier: verifier,
	})
	e := newExpect(t, srv)

	stripeCustomerID := "cus_downgrade_" + uuid.NewString()
	userID, _ := seededUserInline(t, e, mock, db, stripeCustomerID)

	// Seed the user as pro with a subscription id so we can observe the
	// clear-down.
	subID := "sub_to_be_cleared"
	require.NoError(t, db.Model(&users.User{}).
		Where("id = ?", userID).
		Updates(map[string]any{"plan": "pro", "stripe_subscription_id": &subID}).Error)

	verifier.next = stripe.Event{
		ID:   "evt_deleted_1",
		Type: "customer.subscription.deleted",
		Data: &stripe.EventData{
			Raw: []byte(fmt.Sprintf(`{"customer":%q}`, stripeCustomerID)),
		},
	}

	e.POST("/api/v1/webhooks/stripe").
		WithHeader("Stripe-Signature", "t=0,v1=fake").
		WithBytes([]byte(`{}`)).
		Expect().
		Status(http.StatusOK)

	var u users.User
	require.NoError(t, db.Where("id = ?", userID).First(&u).Error)
	assert.Equal(t, users.PlanFree, u.Plan)
	assert.Nil(t, u.StripeSubscriptionID, "subscription id must be cleared on cancellation")
}

// TestStripeWebhook_PaymentFailed_NoStateChange verifies that
// invoice.payment_failed is logged only: no DB mutations occur in v1.
func TestStripeWebhook_PaymentFailed_NoStateChange(t *testing.T) {
	verifier := &fakeEventVerifier{}

	srv, mock, db := newTestServer(t, testServerOpts{
		billingEnabled: true,
		stripeVerifier: verifier,
	})
	e := newExpect(t, srv)

	stripeCustomerID := "cus_payfail_" + uuid.NewString()
	userID, _ := seededUserInline(t, e, mock, db, stripeCustomerID)

	// Snapshot the user state before the webhook fires.
	var before users.User
	require.NoError(t, db.Where("id = ?", userID).First(&before).Error)

	verifier.next = stripe.Event{
		ID:   "evt_payfail_1",
		Type: "invoice.payment_failed",
		Data: &stripe.EventData{
			Raw: []byte(fmt.Sprintf(`{"customer":%q}`, stripeCustomerID)),
		},
	}

	e.POST("/api/v1/webhooks/stripe").
		WithHeader("Stripe-Signature", "t=0,v1=fake").
		WithBytes([]byte(`{}`)).
		Expect().
		Status(http.StatusOK)

	var after users.User
	require.NoError(t, db.Where("id = ?", userID).First(&after).Error)
	assert.Equal(t, before.Plan, after.Plan)
	assert.Equal(t, before.StripeSubscriptionID, after.StripeSubscriptionID)
}

// TestStripeWebhook_UnknownEvent_NoStateChange verifies that event types we
// do not handle (e.g. charge.refunded) return 200 and do not touch the DB.
func TestStripeWebhook_UnknownEvent_NoStateChange(t *testing.T) {
	verifier := &fakeEventVerifier{}

	srv, mock, db := newTestServer(t, testServerOpts{
		billingEnabled: true,
		stripeVerifier: verifier,
	})
	e := newExpect(t, srv)

	stripeCustomerID := "cus_unknown_" + uuid.NewString()
	userID, _ := seededUserInline(t, e, mock, db, stripeCustomerID)

	var before users.User
	require.NoError(t, db.Where("id = ?", userID).First(&before).Error)

	verifier.next = stripe.Event{
		ID:   "evt_unknown_1",
		Type: "charge.refunded",
		Data: &stripe.EventData{
			Raw: []byte(fmt.Sprintf(`{"customer":%q}`, stripeCustomerID)),
		},
	}

	e.POST("/api/v1/webhooks/stripe").
		WithHeader("Stripe-Signature", "t=0,v1=fake").
		WithBytes([]byte(`{}`)).
		Expect().
		Status(http.StatusOK)

	var after users.User
	require.NoError(t, db.Where("id = ?", userID).First(&after).Error)
	assert.Equal(t, before.Plan, after.Plan)
	assert.Equal(t, before.StripeSubscriptionID, after.StripeSubscriptionID)
}
