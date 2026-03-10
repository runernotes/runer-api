package config

import (
	"github.com/rs/zerolog/log"
	"github.com/runernotes/runer-api/internal/auth"
	"github.com/runernotes/runer-api/internal/notes"
	"github.com/runernotes/runer-api/internal/users"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func Connect(cfg *Config) (*gorm.DB, error) {
	var logLevel logger.LogLevel
	switch cfg.DatabaseLogLevel {
	case "silent":
		logLevel = logger.Silent
	case "error":
		logLevel = logger.Error
	case "info":
		logLevel = logger.Info
	default:
		logLevel = logger.Warn
	}

	db, err := gorm.Open(postgres.Open(cfg.DatabaseURL), &gorm.Config{
		Logger: logger.Default.LogMode(logLevel),
	})
	if err != nil {
		return nil, err
	}

	sqlDB, err := db.DB()
	if err != nil {
		return nil, err
	}

	// Connection pool settings
	sqlDB.SetMaxIdleConns(cfg.DatabaseMaxIdleConns)
	sqlDB.SetMaxOpenConns(cfg.DatabaseMaxOpenConns)
	sqlDB.SetConnMaxLifetime(cfg.DatabaseConnMaxLifetime)

	log.Info().Msg("Database connected successfully")

	return db, nil
}

func Migrate(db *gorm.DB) error {
	err := db.AutoMigrate(
		&users.User{},
		&notes.Note{},
		&notes.NoteTombstone{},
		&auth.MagicLinkToken{},
		&auth.RefreshToken{},
	)
	if err != nil {
		return err
	}
	log.Info().Msg("Database migrated successfully")
	return nil
}
