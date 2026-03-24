package config

import (
	"errors"
	"fmt"
	"time"

	"github.com/spf13/viper"
)

type Config struct {
	Port                    string        `mapstructure:"PORT"`
	Env                     string        `mapstructure:"ENV"`
	JWTSecret               string        `mapstructure:"JWT_SECRET"`
	JWTTokenDuration        time.Duration `mapstructure:"JWT_TOKEN_DURATION"`
	JWTRefreshTokenDuration time.Duration `mapstructure:"JWT_REFRESH_TOKEN_DURATION"`
	MagicLinkTokenDuration  time.Duration `mapstructure:"MAGIC_LINK_TOKEN_DURATION"`
	DatabaseURL             string        `mapstructure:"DATABASE_URL"`
	DatabaseLogLevel        string        `mapstructure:"DATABASE_LOG_LEVEL"`
	DatabaseMaxIdleConns    int           `mapstructure:"DATABASE_MAX_IDLE_CONNS"`
	DatabaseMaxOpenConns    int           `mapstructure:"DATABASE_MAX_OPEN_CONNS"`
	DatabaseConnMaxLifetime time.Duration `mapstructure:"DATABASE_CONN_MAX_LIFETIME"`
	AppBaseURL              string        `mapstructure:"APP_BASE_URL"`
	FreeNoteLimit           int           `mapstructure:"FREE_NOTE_LIMIT"`
	ResendAPIKey            string        `mapstructure:"RESEND_API_KEY"`
	EmailFrom               string        `mapstructure:"EMAIL_FROM"`
}

func (c *Config) IsDevelopment() bool {
	return c.Env == "development"
}

func (c *Config) Validate() error {
	if c.JWTSecret == "" {
		return errors.New("JWT_SECRET must be set")
	}
	return nil
}

func setDefaults() {
	viper.SetDefault("PORT", ":8080")
	viper.SetDefault("ENV", "development")
	viper.SetDefault("JWT_TOKEN_DURATION", "1h")
	viper.SetDefault("JWT_REFRESH_TOKEN_DURATION", "168h")
	viper.SetDefault("MAGIC_LINK_TOKEN_DURATION", "1h")
	viper.SetDefault("DATABASE_URL", "postgresql://postgres:postgres@localhost:5432/runer_notes")
	viper.SetDefault("DATABASE_LOG_LEVEL", "warn")
	viper.SetDefault("DATABASE_MAX_IDLE_CONNS", 10)
	viper.SetDefault("DATABASE_MAX_OPEN_CONNS", 100)
	viper.SetDefault("DATABASE_CONN_MAX_LIFETIME", "1h")
	viper.SetDefault("APP_BASE_URL", "http://localhost:8080")
	viper.SetDefault("FREE_NOTE_LIMIT", 50)
	viper.SetDefault("EMAIL_FROM", "noreply@example.com")
}

func Load(target any) error {
	viper.SetConfigFile(".env")
	viper.AutomaticEnv()
	setDefaults()
	_ = viper.BindEnv("JWT_SECRET")
	_ = viper.ReadInConfig()

	if err := viper.Unmarshal(target); err != nil {
		return fmt.Errorf("config: failed to unmarshal configuration: %w", err)
	}

	if cfg, ok := target.(*Config); ok {
		if err := cfg.Validate(); err != nil {
			return fmt.Errorf("config: invalid configuration: %w", err)
		}
	}

	return nil
}
