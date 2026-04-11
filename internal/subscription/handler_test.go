package subscription

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/labstack/echo/v5"
	internalmw "github.com/runernotes/runer-api/internal/middleware"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- fakes ---

// fakeUsersRepository is an in-memory usersRepository implementation.
type fakeUsersRepository struct {
	user    UserRecord
	findErr error

	updateStripeCustomerIDErr error
	capturedStripeID          string
}

func (f *fakeUsersRepository) FindByID(_ context.Context, _ uuid.UUID) (UserRecord, error) {
	if f.findErr != nil {
		return UserRecord{}, f.findErr
	}
	return f.user, nil
}

func (f *fakeUsersRepository) UpdateStripeCustomerID(_ context.Context, _ uuid.UUID, stripeCustomerID string) error {
	f.capturedStripeID = stripeCustomerID
	return f.updateStripeCustomerIDErr
}

// fakeNotesRepository is an in-memory notesRepository implementation.
type fakeNotesRepository struct {
	count    int64
	countErr error
}

func (f *fakeNotesRepository) CountLiveNotes(_ context.Context, _ uuid.UUID) (int64, error) {
	if f.countErr != nil {
		return 0, f.countErr
	}
	return f.count, nil
}

// fakeStripeClient is a test-double for StripeClient.
type fakeStripeClient struct {
	customerID          string
	customerErr         error
	checkoutURL         string
	checkoutErr         error
	createCustomerCalls []string
}

func (f *fakeStripeClient) CreateCustomer(_ context.Context, email string) (string, error) {
	f.createCustomerCalls = append(f.createCustomerCalls, email)
	if f.customerErr != nil {
		return "", f.customerErr
	}
	id := f.customerID
	if id == "" {
		id = "cus_fake"
	}
	return id, nil
}

func (f *fakeStripeClient) CreateCheckoutSession(_ context.Context, _ CheckoutSessionParams) (string, error) {
	if f.checkoutErr != nil {
		return "", f.checkoutErr
	}
	u := f.checkoutURL
	if u == "" {
		u = "https://checkout.stripe.com/pay/test"
	}
	return u, nil
}

// fakeTracker is a no-op analytics tracker.
type fakeTracker struct{}

func (fakeTracker) Capture(_ string, _ string, _ map[string]any) {}
func (fakeTracker) Close()                                        {}

// --- helpers ---

// newEchoCtx creates an Echo context for a GET or POST request, optionally
// populating the user_id context value for authenticated scenarios.
func newEchoCtx(t *testing.T, method, path string, userID *uuid.UUID) (*echo.Context, *httptest.ResponseRecorder) {
	t.Helper()
	e := echo.New()
	req := httptest.NewRequest(method, path, strings.NewReader("{}"))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	if userID != nil {
		c.Set(internalmw.UserContextKey, *userID)
	}
	return c, rec
}

// newHandler builds a Handler with the supplied billing config and optional
// Stripe client.
func newHandler(
	usersRepo usersRepository,
	notesRepo notesRepository,
	freeNoteLimit int,
	billing BillingConfig,
	stripe StripeClient,
) *Handler {
	return NewHandler(usersRepo, notesRepo, freeNoteLimit, billing, stripe, fakeTracker{})
}

// ---- GetSubscription tests ----

// TestGetSubscription_FreeUser_ReturnsNoteLimit verifies that a free user sees
// a numeric NoteLimit set to the configured freeNoteLimit.
func TestGetSubscription_FreeUser_ReturnsNoteLimit(t *testing.T) {
	const limit = 50

	usersRepo := &fakeUsersRepository{user: UserRecord{Plan: "free"}}
	notesRepo := &fakeNotesRepository{count: 12}

	h := newHandler(usersRepo, notesRepo, limit, BillingConfig{}, nil)

	userID := uuid.New()
	c, rec := newEchoCtx(t, http.MethodGet, "/subscription", &userID)

	require.NoError(t, h.GetSubscription(c))
	assert.Equal(t, http.StatusOK, rec.Code)
	body := rec.Body.String()
	assert.Contains(t, body, `"plan":"free"`)
	assert.Contains(t, body, `"note_count":12`)
	assert.Contains(t, body, `"note_limit":50`)
}

// TestGetSubscription_ProUser_NullLimit verifies that a pro user sees a null
// note_limit — pro plan is unlimited.
func TestGetSubscription_ProUser_NullLimit(t *testing.T) {
	usersRepo := &fakeUsersRepository{user: UserRecord{Plan: "pro"}}
	notesRepo := &fakeNotesRepository{count: 200}

	h := newHandler(usersRepo, notesRepo, 50, BillingConfig{}, nil)

	userID := uuid.New()
	c, rec := newEchoCtx(t, http.MethodGet, "/subscription", &userID)

	require.NoError(t, h.GetSubscription(c))
	assert.Equal(t, http.StatusOK, rec.Code)
	body := rec.Body.String()
	assert.Contains(t, body, `"plan":"pro"`)
	assert.Contains(t, body, `"note_limit":null`)
}

// TestGetSubscription_BetaUser_NullLimit verifies that a beta user also sees a
// null note_limit — beta is treated as unlimited like pro.
func TestGetSubscription_BetaUser_NullLimit(t *testing.T) {
	usersRepo := &fakeUsersRepository{user: UserRecord{Plan: "beta"}}
	notesRepo := &fakeNotesRepository{count: 5}

	h := newHandler(usersRepo, notesRepo, 50, BillingConfig{}, nil)

	userID := uuid.New()
	c, rec := newEchoCtx(t, http.MethodGet, "/subscription", &userID)

	require.NoError(t, h.GetSubscription(c))
	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Contains(t, rec.Body.String(), `"note_limit":null`)
}

// TestGetSubscription_Unauthorized_Returns401 verifies that a request without
// a user_id in the Echo context is rejected with 401.
func TestGetSubscription_Unauthorized_Returns401(t *testing.T) {
	h := newHandler(&fakeUsersRepository{}, &fakeNotesRepository{}, 50, BillingConfig{}, nil)
	c, rec := newEchoCtx(t, http.MethodGet, "/subscription", nil) // no userID

	require.NoError(t, h.GetSubscription(c))
	assert.Equal(t, http.StatusUnauthorized, rec.Code)
}

// TestGetSubscription_UserLookupFails_Returns500 verifies that an internal error
// looking up the user returns 500.
func TestGetSubscription_UserLookupFails_Returns500(t *testing.T) {
	usersRepo := &fakeUsersRepository{findErr: errors.New("db error")}

	h := newHandler(usersRepo, &fakeNotesRepository{}, 50, BillingConfig{}, nil)

	userID := uuid.New()
	c, rec := newEchoCtx(t, http.MethodGet, "/subscription", &userID)

	require.NoError(t, h.GetSubscription(c))
	assert.Equal(t, http.StatusInternalServerError, rec.Code)
}

// TestGetSubscription_NoteCountFails_Returns500 verifies that a failure
// counting live notes results in a 500 response.
func TestGetSubscription_NoteCountFails_Returns500(t *testing.T) {
	usersRepo := &fakeUsersRepository{user: UserRecord{Plan: "free"}}
	notesRepo := &fakeNotesRepository{countErr: errors.New("count failed")}

	h := newHandler(usersRepo, notesRepo, 50, BillingConfig{}, nil)

	userID := uuid.New()
	c, rec := newEchoCtx(t, http.MethodGet, "/subscription", &userID)

	require.NoError(t, h.GetSubscription(c))
	assert.Equal(t, http.StatusInternalServerError, rec.Code)
}

// ---- CreateCheckout tests ----

// TestCreateCheckout_BillingDisabled_Returns501 verifies that the endpoint
// returns 501 Not Implemented when BILLING_ENABLED is false.
func TestCreateCheckout_BillingDisabled_Returns501(t *testing.T) {
	h := newHandler(&fakeUsersRepository{}, &fakeNotesRepository{}, 50, BillingConfig{Enabled: false}, nil)

	userID := uuid.New()
	c, rec := newEchoCtx(t, http.MethodPost, "/subscription/checkout", &userID)

	require.NoError(t, h.CreateCheckout(c))
	assert.Equal(t, http.StatusNotImplemented, rec.Code)
	assert.Contains(t, rec.Body.String(), "BILLING_DISABLED")
}

// TestCreateCheckout_Unauthorized_Returns401 verifies that a request without a
// user_id in context returns 401.
func TestCreateCheckout_Unauthorized_Returns401(t *testing.T) {
	h := newHandler(
		&fakeUsersRepository{},
		&fakeNotesRepository{},
		50,
		BillingConfig{Enabled: true},
		&fakeStripeClient{},
	)
	c, rec := newEchoCtx(t, http.MethodPost, "/subscription/checkout", nil)

	require.NoError(t, h.CreateCheckout(c))
	assert.Equal(t, http.StatusUnauthorized, rec.Code)
}

// TestCreateCheckout_NewCustomer_CreatesCustomerAndReturnsURL verifies the happy
// path: a user with no Stripe customer id gets one provisioned and the checkout
// URL is returned.
func TestCreateCheckout_NewCustomer_CreatesCustomerAndReturnsURL(t *testing.T) {
	stripeClient := &fakeStripeClient{
		customerID:  "cus_new_abc",
		checkoutURL: "https://checkout.stripe.com/pay/new",
	}
	usersRepo := &fakeUsersRepository{
		user: UserRecord{
			Plan:             "beta",
			Email:            "user@example.com",
			StripeCustomerID: nil, // no existing customer
		},
	}

	h := newHandler(
		usersRepo,
		&fakeNotesRepository{},
		50,
		BillingConfig{Enabled: true, PriceID: "price_pro", SuccessURL: "https://runer.app/success", CancelURL: "https://runer.app/cancel"},
		stripeClient,
	)

	userID := uuid.New()
	c, rec := newEchoCtx(t, http.MethodPost, "/subscription/checkout", &userID)

	require.NoError(t, h.CreateCheckout(c))
	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Contains(t, rec.Body.String(), "https://checkout.stripe.com/pay/new")

	// Stripe customer must have been created with the user's email.
	require.Len(t, stripeClient.createCustomerCalls, 1)
	assert.Equal(t, "user@example.com", stripeClient.createCustomerCalls[0])

	// The new customer ID must have been persisted.
	assert.Equal(t, "cus_new_abc", usersRepo.capturedStripeID)
}

// TestCreateCheckout_ExistingCustomer_ReusesCustomerID verifies that when the
// user already has a Stripe customer id it is reused without creating a new one.
func TestCreateCheckout_ExistingCustomer_ReusesCustomerID(t *testing.T) {
	existingID := "cus_existing_xyz"
	stripeClient := &fakeStripeClient{}
	usersRepo := &fakeUsersRepository{
		user: UserRecord{
			Plan:             "pro",
			Email:            "existing@example.com",
			StripeCustomerID: &existingID,
		},
	}

	h := newHandler(
		usersRepo,
		&fakeNotesRepository{},
		50,
		BillingConfig{Enabled: true, PriceID: "price_pro"},
		stripeClient,
	)

	userID := uuid.New()
	c, rec := newEchoCtx(t, http.MethodPost, "/subscription/checkout", &userID)

	require.NoError(t, h.CreateCheckout(c))
	assert.Equal(t, http.StatusOK, rec.Code)

	// No new customer must have been created.
	assert.Empty(t, stripeClient.createCustomerCalls, "existing customer must not trigger CreateCustomer")
}

// TestCreateCheckout_StripeCustomerError_Returns502 verifies that when Stripe
// returns an error during customer creation the handler returns 502.
func TestCreateCheckout_StripeCustomerError_Returns502(t *testing.T) {
	stripeClient := &fakeStripeClient{customerErr: errors.New("stripe unavailable")}
	usersRepo := &fakeUsersRepository{
		user: UserRecord{Plan: "beta", Email: "user@example.com", StripeCustomerID: nil},
	}

	h := newHandler(
		usersRepo,
		&fakeNotesRepository{},
		50,
		BillingConfig{Enabled: true},
		stripeClient,
	)

	userID := uuid.New()
	c, rec := newEchoCtx(t, http.MethodPost, "/subscription/checkout", &userID)

	require.NoError(t, h.CreateCheckout(c))
	assert.Equal(t, http.StatusBadGateway, rec.Code)
	assert.Contains(t, rec.Body.String(), "STRIPE_ERROR")
}

// TestCreateCheckout_StripeSessionError_Returns502 verifies that when Stripe
// returns an error creating the checkout session the handler returns 502.
func TestCreateCheckout_StripeSessionError_Returns502(t *testing.T) {
	existingID := "cus_ok"
	stripeClient := &fakeStripeClient{checkoutErr: errors.New("session creation failed")}
	usersRepo := &fakeUsersRepository{
		user: UserRecord{Plan: "pro", Email: "user@example.com", StripeCustomerID: &existingID},
	}

	h := newHandler(
		usersRepo,
		&fakeNotesRepository{},
		50,
		BillingConfig{Enabled: true, PriceID: "price_pro"},
		stripeClient,
	)

	userID := uuid.New()
	c, rec := newEchoCtx(t, http.MethodPost, "/subscription/checkout", &userID)

	require.NoError(t, h.CreateCheckout(c))
	assert.Equal(t, http.StatusBadGateway, rec.Code)
	assert.Contains(t, rec.Body.String(), "STRIPE_ERROR")
}

// TestCreateCheckout_PersistCustomerIDFails_Returns500 verifies that when
// UpdateStripeCustomerID fails after a successful Stripe API call the handler
// returns 500.
func TestCreateCheckout_PersistCustomerIDFails_Returns500(t *testing.T) {
	stripeClient := &fakeStripeClient{customerID: "cus_new"}
	usersRepo := &fakeUsersRepository{
		user:                      UserRecord{Plan: "beta", Email: "user@example.com", StripeCustomerID: nil},
		updateStripeCustomerIDErr: errors.New("persist failed"),
	}

	h := newHandler(
		usersRepo,
		&fakeNotesRepository{},
		50,
		BillingConfig{Enabled: true},
		stripeClient,
	)

	userID := uuid.New()
	c, rec := newEchoCtx(t, http.MethodPost, "/subscription/checkout", &userID)

	require.NoError(t, h.CreateCheckout(c))
	assert.Equal(t, http.StatusInternalServerError, rec.Code)
}
