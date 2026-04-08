package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/labstack/echo/v5"
	"github.com/labstack/echo/v5/middleware"
	"github.com/rs/zerolog/log"
	internalpkg "github.com/runernotes/runer-api/internal"
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

	internalpkg.RegisterRoutes(e, db, &cfg)

	for _, r := range e.Router().Routes() {
		log.Info().Str("method", r.Method).Str("path", r.Path).Msg("route registered")
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if err := (echo.StartConfig{Address: fmt.Sprintf(":%d", cfg.Port), HideBanner: true}).Start(ctx, e); err != nil {
		log.Fatal().Err(err).Msg("server failed")
	}
}
