package e2e_test

import (
	"context"
	"log"
	"net/http/httptest"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/labstack/echo/v5"
	"github.com/labstack/echo/v5/middleware"
	internalpkg "github.com/runernotes/runer-api/internal"
	"github.com/runernotes/runer-api/internal/config"
	"github.com/runernotes/runer-api/internal/logging"
	internalmw "github.com/runernotes/runer-api/internal/middleware"
	"github.com/runernotes/runer-api/internal/subscription"
	"github.com/runernotes/runer-api/internal/validator"
	"github.com/runernotes/runer-api/internal/webhook"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"
	"gorm.io/gorm"
)

// testDBConnStr holds the shared Postgres connection string for all e2e tests.
// It is populated once by TestMain before any test runs.
var testDBConnStr string

// TestMain starts a single shared Postgres container, runs migrations once, and then
// executes all tests in the package. Using a single container instead of one per test
// eliminates container startup overhead as the dominant cost in the test suite.
func TestMain(m *testing.M) {
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
		log.Fatalf("start postgres container: %v", err)
	}
	defer func() { _ = pgContainer.Terminate(ctx) }()

	testDBConnStr, err = pgContainer.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		log.Fatalf("get connection string: %v", err)
	}

	// Configure the global zerolog logger once for the entire test run.
	// sharedCfg() has no POSTHOG_API_KEY, so Setup produces a dev ConsoleWriter
	// with no PostHog side-effects. This ensures ZerologRequestLogger and
	// ZerologAccessLogger have a properly initialised global logger to derive
	// per-request loggers from.
	cfg := sharedCfg()
	shutdown := logging.Setup(cfg)
	defer shutdown()
	db, err := config.Connect(cfg)
	if err != nil {
		log.Fatalf("connect to database: %v", err)
	}
	if err := config.Migrate(db); err != nil {
		log.Fatalf("migrate database: %v", err)
	}

	os.Exit(m.Run())
}

// sharedCfg returns a *config.Config pointing at the shared test database.
// It must only be called after TestMain has populated testDBConnStr.
func sharedCfg() *config.Config {
	return &config.Config{
		JWTSecret:               "e2e-test-secret",
		JWTTokenDuration:        15 * time.Minute,
		JWTRefreshTokenDuration: 7 * 24 * time.Hour,
		MagicLinkTokenDuration:  time.Hour,
		DatabaseURL:             testDBConnStr,
		DatabaseLogLevel:        "silent",
		DatabaseMaxIdleConns:    5,
		DatabaseMaxOpenConns:    10,
		DatabaseConnMaxLifetime: time.Hour,
		AppBaseURL:              "http://localhost",
		// Use a small limit so quota tests don't need to create 50 notes.
		FreeNoteLimit: 3,
		// Cover the real client origins so CORS e2e tests work against realistic values.
		// http://localhost covers Capacitor Android; other entries cover Electron and Vite dev.
		CORSAllowedOrigins: "app://localhost,capacitor://localhost,http://localhost,http://localhost:5173,http://localhost:1420",
		// 1M matches the production default and is large enough for all normal test payloads.
		// Tests that need a tighter limit pass maxRequestBody via testServerOpts.
		MaxRequestBody: "1M",
	}
}

// mockEmailSender captures the magic link token and isNewUser flag instead of sending a real email.
type mockEmailSender struct {
	mu          sync.Mutex
	token       string
	count       int
	isNewUser   bool
}

func (m *mockEmailSender) SendMagicLinkEmail(_ context.Context, _ string, token string, isNewUser bool) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.token = token
	m.count++
	m.isNewUser = isNewUser
	return nil
}

func (m *mockEmailSender) lastToken() string {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.token
}

func (m *mockEmailSender) callCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.count
}

// lastIsNewUser returns the isNewUser flag captured from the most recent SendMagicLinkEmail call.
func (m *mockEmailSender) lastIsNewUser() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.isNewUser
}

// testServerOpts lets individual tests override parts of the server wiring
// without changing the signature of newTestServer for the majority of tests.
// A nil value or zero-valued struct produces the default configuration: no
// billing, no Stripe clients.
type testServerOpts struct {
	// billingEnabled toggles cfg.BillingEnabled and the related Stripe config
	// fields. When true, tests must also provide stripeClient and
	// stripeVerifier, otherwise checkout/webhook calls will fail.
	billingEnabled bool
	stripePriceID  string
	stripeClient   subscription.StripeClient
	stripeVerifier webhook.EventVerifier

	// maxRequestBody overrides the body size limit for this server instance.
	// Expressed in the same human-readable format as MAX_REQUEST_BODY env var
	// (e.g. "1K", "512K", "1M"). Defaults to sharedCfg().MaxRequestBody when
	// empty.
	maxRequestBody string
}

// newTestServer connects to the shared Postgres database, wires the full Echo app with a
// fresh mockEmailSender, and returns an httptest.Server ready for requests. Only the
// database is shared across tests; the Echo instance, HTTP server, and mock are
// created fresh for each test. Pass an optional testServerOpts to customise
// billing behaviour.
func newTestServer(t *testing.T, opts ...testServerOpts) (*httptest.Server, *mockEmailSender, *gorm.DB) {
	t.Helper()

	var opt testServerOpts
	if len(opts) > 0 {
		opt = opts[0]
	}

	cfg := sharedCfg()
	if opt.billingEnabled {
		cfg.BillingEnabled = true
		cfg.StripeSecretKey = "sk_test_e2e"
		cfg.StripeWebhookSecret = "whsec_e2e"
		cfg.StripePriceID = opt.stripePriceID
		if cfg.StripePriceID == "" {
			cfg.StripePriceID = "price_e2e"
		}
		cfg.StripeSuccessURL = "https://runer.app/billing/success"
		cfg.StripeCancelURL = "https://runer.app/billing/cancel"
	}
	if opt.maxRequestBody != "" {
		cfg.MaxRequestBody = opt.maxRequestBody
	}

	db, err := config.Connect(cfg)
	if err != nil {
		t.Fatalf("connect to database: %v", err)
	}
	// Close the connection pool when the test ends — without this, Postgres
	// quickly exhausts its max_connections as the full e2e suite runs.
	t.Cleanup(func() {
		if sqlDB, err := db.DB(); err == nil {
			_ = sqlDB.Close()
		}
	})

	mock := &mockEmailSender{}

	e := echo.New()
	e.Validator = validator.New()
	e.Use(middleware.Recover())
	e.Use(middleware.CORSWithConfig(middleware.CORSConfig{
		AllowOrigins: cfg.ParsedCORSOrigins(),
		AllowMethods: []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"},
		AllowHeaders: []string{"Authorization", "Content-Type"},
	}))
	e.Use(middleware.BodyLimit(cfg.MaxRequestBodyBytes()))
	e.Use(middleware.RequestID())
	e.Use(internalmw.ZerologRequestLogger())
	e.Use(internalmw.ZerologAccessLogger())

	internalpkg.RegisterRoutes(e, db, cfg, internalpkg.RouteOptions{
		EmailSender:         mock,
		StripeClient:        opt.stripeClient,
		StripeEventVerifier: opt.stripeVerifier,
	})

	srv := httptest.NewServer(e)
	t.Cleanup(srv.Close)

	return srv, mock, db
}
