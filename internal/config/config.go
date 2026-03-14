package config

import (
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
	AppBaseURL string `mapstructure:"APP_BASE_URL"`
}

func (c *Config) IsDevelopment() bool {
	return c.Env == "development"
}

func setDefaults() {
	viper.SetDefault("PORT", ":8080")
	viper.SetDefault("ENV", "development")
	viper.SetDefault("JWT_SECRET", "dev-secret-change-me-in-production")
	viper.SetDefault("JWT_TOKEN_DURATION", "1h")
	viper.SetDefault("JWT_REFRESH_TOKEN_DURATION", "168h")
	viper.SetDefault("MAGIC_LINK_TOKEN_DURATION", "1h")
	viper.SetDefault("DATABASE_URL", "postgresql://postgres:postgres@localhost:5432/runer_notes")
	viper.SetDefault("DATABASE_LOG_LEVEL", "warn")
	viper.SetDefault("DATABASE_MAX_IDLE_CONNS", 10)
	viper.SetDefault("DATABASE_MAX_OPEN_CONNS", 100)
	viper.SetDefault("DATABASE_CONN_MAX_LIFETIME", "1h")
	viper.SetDefault("APP_BASE_URL", "http://localhost:8080")
}

func Load(target any) error {
	viper.SetConfigFile(".env")
	viper.AutomaticEnv()
	setDefaults()
	_ = viper.ReadInConfig()
	return viper.Unmarshal(target)
}
