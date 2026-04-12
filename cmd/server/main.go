package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/labstack/echo/v5"
	"github.com/labstack/echo/v5/middleware"
	"github.com/rs/zerolog/log"
	internalpkg "github.com/runernotes/runer-api/internal"
	"github.com/runernotes/runer-api/internal/analytics"
	"github.com/runernotes/runer-api/internal/config"
	"github.com/runernotes/runer-api/internal/logging"
	internalmw "github.com/runernotes/runer-api/internal/middleware"
	"github.com/runernotes/runer-api/internal/validator"
)

func main() {
	var cfg config.Config

	if err := config.Load(&cfg); err != nil {
		// Logger isn't set up yet — use a temporary console logger for this
		// one fatal message so the error is readable before the process exits.
		fmt.Fprintf(os.Stderr, "failed to load config: %v\n", err)
		os.Exit(1)
	}

	// Configure the global zerolog logger exactly once. All subsequent log
	// calls anywhere in the codebase use log.Logger (the global) or a derived
	// per-request copy stored on the request context.
	shutdown := logging.Setup(&cfg)
	defer shutdown()

	// Create the analytics tracker. When POSTHOG_API_KEY is not set this is a
	// no-op tracker. This is a separate PostHog client from the one used for log
	// forwarding so that analytics events and server logs can be managed and
	// filtered independently in PostHog.
	tracker := analytics.New(cfg.PostHogAPIKey, cfg.PostHogEndpoint)
	defer tracker.Close()

	db, err := config.Connect(&cfg)
	if err != nil {
		log.Fatal().Err(err).Msg("failed to connect to database")
	}

	if err := config.Migrate(db); err != nil {
		log.Fatal().Err(err).Msg("failed to migrate database")
	}

	e := echo.New()
	e.Validator = validator.New()

	e.Use(middleware.Recover())
	e.Use(middleware.CORSWithConfig(middleware.CORSConfig{
		AllowOrigins: cfg.ParsedCORSOrigins(),
		AllowMethods: []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"},
		AllowHeaders: []string{"Authorization", "Content-Type"},
	}))
	// Bound request bodies early so oversized payloads are rejected before
	// any handler or auth logic runs, preventing OOM on large uploads.
	e.Use(middleware.BodyLimit(cfg.MaxRequestBodyBytes()))
	// RequestID must come before ZerologRequestLogger so the header is set
	// before we derive the per-request logger from it.
	e.Use(middleware.RequestID())
	e.Use(internalmw.ZerologRequestLogger())
	e.Use(internalmw.ZerologAccessLogger())
	e.Use(internalmw.RateLimiter(cfg.RateLimitPerMinute, cfg.RateLimitBurst))

	internalpkg.RegisterRoutes(e, db, &cfg, internalpkg.RouteOptions{
		Tracker: tracker,
	})

	for _, r := range e.Router().Routes() {
		log.Info().Str("method", r.Method).Str("path", r.Path).Msg("route registered")
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	sc := buildStartConfig(cfg.Port, func(err error) {
		log.Error().Err(err).Msg("server shutdown timed out — some in-flight connections may have been dropped")
	})

	if err := sc.Start(ctx, e); err != nil {
		log.Fatal().Err(err).Msg("server failed")
	}
}

// buildStartConfig constructs the Echo StartConfig with production-safe defaults:
//   - 30-second graceful drain window so in-flight requests survive a SIGTERM.
//   - 120-second idle timeout to reclaim long-lived idle connections.
//   - ReadTimeout / WriteTimeout are set to 30 s by Echo v5 internally.
//
// The onShutdownError callback is invoked when active connections are not
// drained within the grace period. Pass nil to use Echo's default slog logger.
func buildStartConfig(port int, onShutdownError func(error)) echo.StartConfig {
	return echo.StartConfig{
		Address:         fmt.Sprintf(":%d", port),
		HideBanner:      true,
		GracefulTimeout: 30 * time.Second,
		OnShutdownError: onShutdownError,
		// Echo v5 sets ReadTimeout/WriteTimeout to 30 s by default.
		// We only need to add IdleTimeout here.
		BeforeServeFunc: func(s *http.Server) error {
			s.IdleTimeout = 120 * time.Second
			return nil
		},
	}
}
