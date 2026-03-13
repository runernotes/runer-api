package main

import (
	"github.com/rs/zerolog/log"
	"github.com/runernotes/runer-api-core/pkg/app"
	"github.com/runernotes/runer-api-core/pkg/config"
	"github.com/runernotes/runer-api-core/pkg/logger"
)

func main() {
	logger.Init()

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

	a := app.New(db, &cfg)
	if err := a.Start(cfg.Port); err != nil {
		log.Fatal().Err(err).Msg("server failed")
	}
}
