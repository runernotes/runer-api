package webhook

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/google/uuid"
	"github.com/labstack/echo/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stripe/stripe-go/v81"
)

// fakeUsersRepo is an in-memory UsersRepository implementation used by
// webhook handler tests. It records every call so assertions can verify the
// exact operations the dispatcher performed.
type fakeUsersRepo struct {
	mu sync.Mutex

	// byStripeID maps a Stripe customer id to the user id the repo should
	// return from FindByStripeCustomerID. A missing entry returns
	// ErrNotFound so we can exercise the error path.
	byStripeID map[string]uuid.UUID

	planUpdates   []planUpdate
	subIDUpdates  []subIDUpdate
	findNotFound  bool
	findErrCalled int
}

type planUpdate struct {
	userID uuid.UUID
	plan   string
}

type subIDUpdate struct {
	userID uuid.UUID
	subID  *string
}

var errNotFound = errors.New("not found")

func newFakeRepo() *fakeUsersRepo {
	return &fakeUsersRepo{byStripeID: map[string]uuid.UUID{}}
}

func (r *fakeUsersRepo) FindByStripeCustomerID(_ context.Context, id string) (UserRecord, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.findNotFound {
		r.findErrCalled++
		return UserRecord{}, errNotFound
	}
	uid, ok := r.byStripeID[id]
	if !ok {
		return UserRecord{}, errNotFound
	}
	return UserRecord{ID: uid}, nil
}

func (r *fakeUsersRepo) UpdatePlan(_ context.Context, id uuid.UUID, plan string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.planUpdates = append(r.planUpdates, planUpdate{userID: id, plan: plan})
	return nil
}

func (r *fakeUsersRepo) UpdateStripeSubscriptionID(_ context.Context, id uuid.UUID, subID *string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.subIDUpdates = append(r.subIDUpdates, subIDUpdate{userID: id, subID: subID})
	return nil
}

// fakeVerifier is an EventVerifier that returns a pre-baked event or error.
type fakeVerifier struct {
	event stripe.Event
	err   error
}

func (v *fakeVerifier) Verify(_ []byte, _ string) (stripe.Event, error) {
	if v.err != nil {
		return stripe.Event{}, v.err
	}
	return v.event, nil
}

// newEchoContext constructs an Echo request context suitable for invoking a
// handler directly.
func newEchoContext(t *testing.T, body string) (*echo.Context, *httptest.ResponseRecorder) {
	t.Helper()
	e := echo.New()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/webhooks/stripe", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Stripe-Signature", "t=0,v1=fake")
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	return c, rec
}

// makeEvent builds a stripe.Event with the given type and a raw JSON data blob
// ready for dispatch tests (no signing involved — we swap the verifier instead).
func makeEvent(eventType stripe.EventType, rawData string) stripe.Event {
	return stripe.Event{
		ID:   "evt_test_" + string(eventType),
		Type: eventType,
		Data: &stripe.EventData{Raw: []byte(rawData)},
	}
}

func TestHandleStripe_BillingDisabled_ReturnsOK(t *testing.T) {
	t.Parallel()

	// Even with no verifier at all the disabled path must respond 200 so
	// Stripe does not mark the endpoint as down.
	h := NewHandler(false, nil, newFakeRepo())
	ctx, rec := newEchoContext(t, `{"type":"checkout.session.completed"}`)

	require.NoError(t, h.HandleStripe(ctx))
	assert.Equal(t, http.StatusOK, rec.Code)
}

func TestHandleStripe_InvalidSignature_Returns400(t *testing.T) {
	t.Parallel()

	h := NewHandler(true, &fakeVerifier{err: errors.New("bad signature")}, newFakeRepo())
	ctx, rec := newEchoContext(t, `{}`)

	require.NoError(t, h.HandleStripe(ctx))
	assert.Equal(t, http.StatusBadRequest, rec.Code)
	assert.Contains(t, rec.Body.String(), "INVALID_SIGNATURE")
}

func TestHandleStripe_CheckoutCompleted_UpgradesUser(t *testing.T) {
	t.Parallel()

	userID := uuid.New()
	repo := newFakeRepo()
	repo.byStripeID["cus_123"] = userID

	evt := makeEvent(
		"checkout.session.completed",
		`{"customer":"cus_123","subscription":"sub_456"}`,
	)
	h := NewHandler(true, &fakeVerifier{event: evt}, repo)

	ctx, rec := newEchoContext(t, `{}`)
	require.NoError(t, h.HandleStripe(ctx))

	assert.Equal(t, http.StatusOK, rec.Code)
	require.Len(t, repo.planUpdates, 1)
	assert.Equal(t, userID, repo.planUpdates[0].userID)
	assert.Equal(t, "pro", repo.planUpdates[0].plan)
	require.Len(t, repo.subIDUpdates, 1)
	require.NotNil(t, repo.subIDUpdates[0].subID)
	assert.Equal(t, "sub_456", *repo.subIDUpdates[0].subID)
}

func TestHandleStripe_SubscriptionDeleted_DowngradesUser(t *testing.T) {
	t.Parallel()

	userID := uuid.New()
	repo := newFakeRepo()
	repo.byStripeID["cus_999"] = userID

	evt := makeEvent("customer.subscription.deleted", `{"customer":"cus_999"}`)
	h := NewHandler(true, &fakeVerifier{event: evt}, repo)

	ctx, rec := newEchoContext(t, `{}`)
	require.NoError(t, h.HandleStripe(ctx))

	assert.Equal(t, http.StatusOK, rec.Code)
	require.Len(t, repo.planUpdates, 1)
	assert.Equal(t, "free", repo.planUpdates[0].plan)
	require.Len(t, repo.subIDUpdates, 1)
	assert.Nil(t, repo.subIDUpdates[0].subID, "subscription id must be cleared on cancellation")
}

func TestHandleStripe_PaymentFailed_NoStateChange(t *testing.T) {
	t.Parallel()

	repo := newFakeRepo()
	evt := makeEvent("invoice.payment_failed", `{"customer":"cus_any"}`)
	h := NewHandler(true, &fakeVerifier{event: evt}, repo)

	ctx, rec := newEchoContext(t, `{}`)
	require.NoError(t, h.HandleStripe(ctx))

	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Empty(t, repo.planUpdates, "payment_failed must not change plans in v1")
	assert.Empty(t, repo.subIDUpdates)
}

func TestHandleStripe_UnknownEvent_NoStateChange(t *testing.T) {
	t.Parallel()

	repo := newFakeRepo()
	evt := makeEvent("charge.refunded", `{"customer":"cus_any"}`)
	h := NewHandler(true, &fakeVerifier{event: evt}, repo)

	ctx, rec := newEchoContext(t, `{}`)
	require.NoError(t, h.HandleStripe(ctx))

	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Empty(t, repo.planUpdates)
	assert.Empty(t, repo.subIDUpdates)
}

func TestHandleStripe_DispatchFailure_StillReturns200(t *testing.T) {
	t.Parallel()

	// Repo returns not-found; handler logs but must not surface non-2xx
	// because Stripe retries on non-2xx.
	repo := newFakeRepo()
	repo.findNotFound = true

	evt := makeEvent("checkout.session.completed", `{"customer":"cus_unknown","subscription":"sub_x"}`)
	h := NewHandler(true, &fakeVerifier{event: evt}, repo)

	ctx, rec := newEchoContext(t, `{}`)
	require.NoError(t, h.HandleStripe(ctx))

	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Empty(t, repo.planUpdates)
}

// TestHandleStripe_RawBodyIsReadBeforeDispatch verifies the handler reads the
// entire request body (Stripe verifies against the raw bytes). A reader that
// reports a read error must cause a 400 response.
func TestHandleStripe_BodyReadError_Returns400(t *testing.T) {
	t.Parallel()

	h := NewHandler(true, &fakeVerifier{err: errors.New("never reached")}, newFakeRepo())

	e := echo.New()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/webhooks/stripe", erroringReader{})
	req.Header.Set("Stripe-Signature", "sig")
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	require.NoError(t, h.HandleStripe(c))
	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

// erroringReader fails on the first read — used to exercise the body-read
// error branch.
type erroringReader struct{}

func (erroringReader) Read(_ []byte) (int, error) { return 0, fmt.Errorf("forced read error") }
func (erroringReader) Close() error               { return nil }

var _ io.ReadCloser = erroringReader{}
