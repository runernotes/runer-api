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
	"github.com/runernotes/runer-api/internal/validator"
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

	// Connect and migrate once for the entire test run.
	cfg := sharedCfg()
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
	}
}

// mockEmailSender captures the magic link token instead of sending a real email.
type mockEmailSender struct {
	mu    sync.Mutex
	token string
	count int
}

func (m *mockEmailSender) SendMagicLinkEmail(_ context.Context, _ string, token string, _ bool) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.token = token
	m.count++
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

// newTestServer connects to the shared Postgres database, wires the full Echo app with a
// fresh mockEmailSender, and returns an httptest.Server ready for requests. Only the
// database is shared across tests; the Echo instance, HTTP server, and mock are
// created fresh for each test.
func newTestServer(t *testing.T) (*httptest.Server, *mockEmailSender, *gorm.DB) {
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

	internalpkg.RegisterRoutes(e, db, cfg, internalpkg.RouteOptions{
		EmailSender: mock,
	})

	srv := httptest.NewServer(e)
	t.Cleanup(srv.Close)

	return srv, mock, db
}
