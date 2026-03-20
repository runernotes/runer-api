package e2e_test

import (
	"context"
	"net/http/httptest"
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
)

// mockEmailSender captures the magic link token instead of sending a real email.
type mockEmailSender struct {
	mu    sync.Mutex
	token string
	count int
}

func (m *mockEmailSender) SendMagicLinkEmail(_ context.Context, _ string, token string) error {
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

// newTestServer starts a real Postgres container, migrates the schema, wires the full Echo
// app with a mockEmailSender, and returns an httptest.Server ready for requests.
func newTestServer(t *testing.T) (*httptest.Server, *mockEmailSender) {
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

	internalpkg.RegisterRoutes(e, db, cfg, internalpkg.RouteOptions{
		EmailSender: mock,
	})

	srv := httptest.NewServer(e)
	t.Cleanup(srv.Close)

	return srv, mock
}
