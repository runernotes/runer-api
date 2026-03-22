package e2e_test

// Rate-limiter tests verify that the server enforces a per-IP burst limit.
// A fresh server (with a fresh limiter state) is started for each test so the
// burst counter begins at its maximum value and prior requests cannot interfere.

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/labstack/echo/v5"
	"github.com/labstack/echo/v5/middleware"
	internalpkg "github.com/runernotes/runer-api/internal"
	"github.com/runernotes/runer-api/internal/config"
	internalmw "github.com/runernotes/runer-api/internal/middleware"
	"github.com/runernotes/runer-api/internal/validator"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"
)

// newTestServerWithRateLimiter starts the same test infrastructure as newTestServer but
// also applies the rate-limiter middleware so that burst exhaustion can be tested.
// The rate-limiter state is fresh for each call; tests that exercise the limiter must use
// this helper rather than the standard newTestServer.
func newTestServerWithRateLimiter(t *testing.T) (*httptest.Server, *mockEmailSender) {
	t.Helper()
	ctx := context.Background()

	pgContainer, err := postgres.Run(ctx,
		"postgres:16-alpine",
		postgres.WithDatabase("runer_test"),
		postgres.WithUsername("postgres"),
		postgres.WithPassword("postgres"),
		testcontainers.WithWaitStrategy(
			wait.ForLog("database system is ready to accept connections").
				WithOccurrence(2).
				WithStartupTimeout(60*time.Second),
		),
	)
	if err != nil {
		t.Fatalf("start postgres container: %v", err)
	}
	t.Cleanup(func() { _ = pgContainer.Terminate(ctx) })

	connStr, err := pgContainer.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		t.Fatalf("get connection string: %v", err)
	}

	cfg := &config.Config{
		JWTSecret:               "e2e-test-secret",
		JWTTokenDuration:        15 * time.Minute,
		JWTRefreshTokenDuration: 7 * 24 * time.Hour,
		MagicLinkTokenDuration:  time.Hour,
		DatabaseURL:             connStr,
		DatabaseLogLevel:        "silent",
		DatabaseMaxIdleConns:    5,
		DatabaseMaxOpenConns:    10,
		DatabaseConnMaxLifetime: time.Hour,
		AppBaseURL:              "http://localhost",
	}

	db, err := config.Connect(cfg)
	if err != nil {
		t.Fatalf("connect to database: %v", err)
	}
	if err := config.Migrate(db); err != nil {
		t.Fatalf("migrate database: %v", err)
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
