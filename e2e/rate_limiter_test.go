package e2e_test

// Rate-limiter tests verify that the server enforces a per-IP burst limit.
// A fresh server (with a fresh limiter state) is started for each test so the
// burst counter begins at its maximum value and prior requests cannot interfere.
//

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"
	"github.com/labstack/echo/v5"
	"github.com/labstack/echo/v5/middleware"
	internalpkg "github.com/runernotes/runer-api/internal"
	"github.com/runernotes/runer-api/internal/config"
	internalmw "github.com/runernotes/runer-api/internal/middleware"
	"github.com/runernotes/runer-api/internal/validator"
)

// newTestServerWithRateLimiter wires the same infrastructure as newTestServer but also
// applies the rate-limiter middleware so that burst exhaustion can be tested. The
// rate-limiter state is fresh for each call because a new Echo instance (and therefore
// a new limiter) is created. Tests that exercise the limiter must use this helper.
//
// The shared Postgres database (testDBConnStr) is reused; only the Echo instance and
// rate-limiter state are fresh.
func newTestServerWithRateLimiter(t *testing.T) (*httptest.Server, *mockEmailSender) {
	t.Helper()

	cfg := sharedCfg()

	db, err := config.Connect(cfg)
	if err != nil {
		t.Fatalf("connect to database: %v", err)
	}

	mock := &mockEmailSender{}

	e := echo.New()
	e.Validator = validator.New()
	e.Use(middleware.Recover())
	e.Use(middleware.RequestLogger())
	// Apply the rate limiter so burst exhaustion can be observed in tests.
	e.Use(internalmw.RateLimiter())

	internalpkg.RegisterRoutes(e, db, cfg, internalpkg.RouteOptions{
		EmailSender: mock,
	})

	srv := httptest.NewServer(e)
	t.Cleanup(srv.Close)

	return srv, mock
}

// TestRateLimiterAllowsRequestsWithinBurst verifies that a single request to a public
// endpoint is accepted (not rate-limited) when the burst bucket is full.
func TestRateLimiterAllowsRequestsWithinBurst(t *testing.T) {
	srv, _ := newTestServerWithRateLimiter(t)
	e := newExpect(t, srv)

	reqBody := map[string]string{"email": "ratelimit-" + uuid.NewString() + "@example.com"}

	// POST /auth/magic-link is publicly accessible and goes through the rate limiter.
	// A single request on a fresh server must succeed.
	e.POST("/api/v1/auth/magic-link").
		WithJSON(reqBody).
		Expect().
		Status(http.StatusOK)
}

// TestRateLimiterBurstExhaustedReturns429 verifies that sending more requests than the
// configured burst size from the same IP within a short window results in a 429 response
// with code RATE_LIMITED. The limiter is configured for a burst of 5; the sixth rapid
// request must be rejected.
func TestRateLimiterBurstExhaustedReturns429(t *testing.T) {
	srv, _ := newTestServerWithRateLimiter(t)
	e := newExpect(t, srv)

	// Use a public endpoint so auth tokens are not required.
	// POST /auth/magic-link is publicly accessible and goes through the rate limiter.
	reqBody := map[string]string{"email": "ratelimit-" + uuid.NewString() + "@example.com"}

	// Fire requests 1–5 rapidly to exhaust the burst (burst=5).
	for i := 1; i <= 5; i++ {
		e.POST("/api/v1/auth/magic-link").
			WithJSON(reqBody).
			Expect()
		// No status assertion here — some of these may or may not succeed depending on
		// how quickly the refill token replenishes; we only care that request #6 is 429.
	}

	// The 6th rapid request must be rate-limited because the burst of 5 is now exhausted.
	e.POST("/api/v1/auth/magic-link").
		WithJSON(reqBody).
		Expect().
		Status(http.StatusTooManyRequests).
		JSON().Object().
		HasValue("code", "RATE_LIMITED")
}
