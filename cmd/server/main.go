package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"

	"github.com/labstack/echo/v5"
	"github.com/labstack/echo/v5/middleware"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	internalpkg "github.com/runernotes/runer-api/internal"
	"github.com/runernotes/runer-api/internal/config"
	internalmw "github.com/runernotes/runer-api/internal/middleware"
	"github.com/runernotes/runer-api/internal/validator"
)

func main() {
	output := zerolog.ConsoleWriter{Out: os.Stdout, NoColor: false}
	log.Logger = zerolog.New(output).With().Timestamp().Logger()

	var cfg config.Config

	if err := config.Load(&cfg); err != nil {
		log.Fatal().Err(err).Msg("failed to load config")
	}

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
		AllowOrigins: []string{"*"},
		AllowMethods: []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"},
		AllowHeaders: []string{"Authorization", "Content-Type"},
	}))
	e.Use(middleware.RequestLogger())
	e.Use(internalmw.RateLimiter(cfg.RateLimitPerMinute, cfg.RateLimitBurst))

	internalpkg.RegisterRoutes(e, db, &cfg)

	for _, r := range e.Router().Routes() {
		log.Info().Str("method", r.Method).Str("path", r.Path).Msg("route registered")
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if err := (echo.StartConfig{Address: cfg.Port, HideBanner: true}).Start(ctx, e); err != nil {
		log.Fatal().Err(err).Msg("server failed")
	}
}
