package app

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
	internalmw "github.com/runernotes/runer-api/internal/middleware"
	"github.com/runernotes/runer-api/pkg/config"
	"gorm.io/gorm"
)

type App struct {
	Echo   *echo.Echo
	DB     *gorm.DB
	Logger zerolog.Logger
}

func New(db *gorm.DB, config *config.Config) *App {
	e := echo.New()

	a := &App{Echo: e, DB: db, Logger: log.Logger}

	printBanner()

	// Core middleware
	e.Use(middleware.Recover())
	e.Use(internalmw.RequestLogger())

	// Register all internal routes
	internalpkg.RegisterRoutes(e, db, config)

	return a
}

func (a *App) Start(addr string) error {
	printRegisteredRoutes(a.Echo)
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	return echo.StartConfig{Address: addr, HideBanner: true}.Start(ctx, a.Echo)
}

func printRegisteredRoutes(e *echo.Echo) {
	for _, r := range e.Router().Routes() {
		log.Info().Str("method", r.Method).Str("path", r.Path).Msg("route registered")
	}
}

func printBanner() {
	log.Info().Msg(`
  ____
 |  _ \ _   _ _ __   ___ _ __
 | |_) | | | | '_ \ / _ \ '__|
 |  _ <| |_| | | | |  __/ |
 |_| \_\\__,_|_| |_|\___|_|

  _   _       _
 | \ | | ___ | |_ ___  ___
 |  \| |/ _ \| __/ _ \/ __|
 | |\  | (_) | ||  __/\__ \
 |_| \_|\___/ \__\___||___/
`)
}
