package e2e_test

import (
	"context"
	"fmt"
	"net/http"
	"sync"
	"testing"

	"github.com/google/uuid"
	"github.com/runernotes/runer-api/internal/subscription"
	"github.com/runernotes/runer-api/internal/users"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fakeStripeClient is an in-memory subscription.StripeClient used by the
// billing checkout e2e tests. It records each call so tests can assert the
// exact arguments the handler passed.
type fakeStripeClient struct {
	mu sync.Mutex

	// customerID is the value returned from CreateCustomer. Defaults to
	// "cus_fake" when empty.
	customerID string
	// checkoutURL is the value returned from CreateCheckoutSession.
	checkoutURL string

	// Recorded state for assertions.
	createCustomerCalls []string // emails
	createSessionCalls  []subscription.CheckoutSessionParams
}

func (f *fakeStripeClient) CreateCustomer(_ context.Context, email string) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.createCustomerCalls = append(f.createCustomerCalls, email)
	id := f.customerID
	if id == "" {
		id = "cus_fake"
	}
	return id, nil
}

func (f *fakeStripeClient) CreateCheckoutSession(_ context.Context, params subscription.CheckoutSessionParams) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.createSessionCalls = append(f.createSessionCalls, params)
	url := f.checkoutURL
	if url == "" {
		url = "https://checkout.stripe.com/c/pay/test"
	}
	return url, nil
}

func (f *fakeStripeClient) customerEmails() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]string, len(f.createCustomerCalls))
	copy(out, f.createCustomerCalls)
	return out
}

func (f *fakeStripeClient) sessionParams() []subscription.CheckoutSessionParams {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]subscription.CheckoutSessionParams, len(f.createSessionCalls))
	copy(out, f.createSessionCalls)
	return out
}

// TestCheckout_RequiresAuth verifies that POST /subscription/checkout is
// protected by the auth middleware.
func TestCheckout_RequiresAuth(t *testing.T) {
	srv, _, _ := newTestServer(t, testServerOpts{
		billingEnabled: true,
		stripeClient:   &fakeStripeClient{},
	})
	e := newExpect(t, srv)

	e.POST("/api/v1/subscription/checkout").
		Expect().
		Status(http.StatusUnauthorized)
}

// TestCheckout_BillingDisabled_Returns501 verifies the self-hosted default
// path: with BILLING_ENABLED=false the endpoint must return 501 Not
// Implemented regardless of the caller's auth status.
func TestCheckout_BillingDisabled_Returns501(t *testing.T) {
	srv, mock, _ := newTestServer(t) // defaults: billing disabled
	e := newExpect(t, srv)

	token := registerAndLogin(t, e, mock, uuid.NewString())

	e.POST("/api/v1/subscription/checkout").
		WithHeader("Authorization", "Bearer "+token).
		Expect().
		Status(http.StatusNotImplemented).
		JSON().Object().
		HasValue("code", "BILLING_DISABLED")
}

// TestCheckout_FirstTime_CreatesCustomerAndSession covers the happy path for
// a user with no prior Stripe customer id: the handler must provision a new
// Stripe customer, persist the id on the users row, and return the hosted
// checkout URL.
func TestCheckout_FirstTime_CreatesCustomerAndSession(t *testing.T) {
	stripe := &fakeStripeClient{
		customerID:  "cus_first_time",
		checkoutURL: "https://checkout.stripe.com/c/pay/abc123",
	}

	srv, mock, db := newTestServer(t, testServerOpts{
		billingEnabled: true,
		stripePriceID:  "price_pro_monthly",
		stripeClient:   stripe,
	})
	e := newExpect(t, srv)

	suffix := uuid.NewString()
	email := fmt.Sprintf("user-%s@example.com", suffix)
	token := registerAndLogin(t, e, mock, suffix)

	e.POST("/api/v1/subscription/checkout").
		WithHeader("Authorization", "Bearer "+token).
		Expect().
		Status(http.StatusOK).
		JSON().Object().
		HasValue("checkout_url", "https://checkout.stripe.com/c/pay/abc123")

	// Assert the fake was called with the right inputs.
	emails := stripe.customerEmails()
	require.Len(t, emails, 1, "expected exactly one Stripe customer to be created")
	assert.Equal(t, email, emails[0])

	sessions := stripe.sessionParams()
	require.Len(t, sessions, 1)
	assert.Equal(t, "cus_first_time", sessions[0].CustomerID)
	assert.Equal(t, "price_pro_monthly", sessions[0].PriceID)
	assert.Equal(t, "https://runer.app/billing/success", sessions[0].SuccessURL)
	assert.Equal(t, "https://runer.app/billing/cancel", sessions[0].CancelURL)

	// Assert the DB was updated with the new Stripe customer id.
	var u users.User
	require.NoError(t, db.Where("email = ?", email).First(&u).Error)
	require.NotNil(t, u.StripeCustomerID)
	assert.Equal(t, "cus_first_time", *u.StripeCustomerID)
}

// TestCheckout_ReusesExistingCustomer verifies that subsequent checkout
// requests for a user with a stored Stripe customer id do not create a new
// customer — they reuse the persisted id.
func TestCheckout_ReusesExistingCustomer(t *testing.T) {
	stripe := &fakeStripeClient{}

	srv, mock, db := newTestServer(t, testServerOpts{
		billingEnabled: true,
		stripeClient:   stripe,
	})
	e := newExpect(t, srv)

	suffix := uuid.NewString()
	email := fmt.Sprintf("user-%s@example.com", suffix)
	token := registerAndLogin(t, e, mock, suffix)

	// Pre-seed a Stripe customer id on the user row.
	existingID := "cus_existing_abc"
	require.NoError(t, db.Model(&users.User{}).
		Where("email = ?", email).
		Update("stripe_customer_id", existingID).Error)

	e.POST("/api/v1/subscription/checkout").
		WithHeader("Authorization", "Bearer "+token).
		Expect().
		Status(http.StatusOK).
		JSON().Object().
		ContainsKey("checkout_url")

	// No new customer must have been created.
	assert.Empty(t, stripe.customerEmails(), "existing customers must not be re-created")

	sessions := stripe.sessionParams()
	require.Len(t, sessions, 1)
	assert.Equal(t, existingID, sessions[0].CustomerID)
}
