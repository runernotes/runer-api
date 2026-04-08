package config

import (
	"errors"
	"fmt"
	"strings"
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
	RateLimitPerMinute      int           `mapstructure:"RATE_LIMIT_PER_MINUTE"`
	RateLimitBurst          int           `mapstructure:"RATE_LIMIT_BURST"`
	ResendAPIKey            string        `mapstructure:"RESEND_API_KEY"`
	EmailFrom               string        `mapstructure:"EMAIL_FROM"`

	// CORSAllowedOrigins is a comma-separated list of origins the server will
	// reflect in Access-Control-Allow-Origin responses. Set via CORS_ALLOWED_ORIGINS.
	// Use ParsedCORSOrigins() to obtain the split slice for use in middleware.
	CORSAllowedOrigins string `mapstructure:"CORS_ALLOWED_ORIGINS"`

	// Billing / Stripe. All fields are optional unless BillingEnabled is true,
	// in which case Validate() requires the Stripe secrets and the Resend key.
	BillingEnabled      bool   `mapstructure:"BILLING_ENABLED"`
	StripeSecretKey     string `mapstructure:"STRIPE_SECRET_KEY"`
	StripeWebhookSecret string `mapstructure:"STRIPE_WEBHOOK_SECRET"`
	StripePriceID       string `mapstructure:"STRIPE_PRICE_ID"`
	StripeSuccessURL    string `mapstructure:"STRIPE_SUCCESS_URL"`
	StripeCancelURL     string `mapstructure:"STRIPE_CANCEL_URL"`
}

func (c *Config) IsDevelopment() bool {
	return c.Env == "development"
}

// ParsedCORSOrigins splits the comma-separated CORS_ALLOWED_ORIGINS value into
// a slice of trimmed, non-empty origin strings ready for use in Echo's CORS
// middleware. Returns nil when the raw value is empty.
func (c *Config) ParsedCORSOrigins() []string {
	if c.CORSAllowedOrigins == "" {
		return nil
	}
	parts := strings.Split(c.CORSAllowedOrigins, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if trimmed := strings.TrimSpace(p); trimmed != "" {
			out = append(out, trimmed)
		}
	}
	return out
}

func (c *Config) Validate() error {
	if c.JWTSecret == "" {
		return errors.New("JWT_SECRET must be set")
	}
	if c.RateLimitPerMinute <= 0 {
		return errors.New("RATE_LIMIT_PER_MINUTE must be greater than 0")
	}
	if c.RateLimitBurst <= 0 {
		return errors.New("RATE_LIMIT_BURST must be greater than 0")
	}

	if c.BillingEnabled {
		if c.StripeSecretKey == "" {
			return errors.New("STRIPE_SECRET_KEY must be set when BILLING_ENABLED is true")
		}
		if c.StripeWebhookSecret == "" {
			return errors.New("STRIPE_WEBHOOK_SECRET must be set when BILLING_ENABLED is true")
		}
		if c.StripePriceID == "" {
			return errors.New("STRIPE_PRICE_ID must be set when BILLING_ENABLED is true")
		}
		if c.ResendAPIKey == "" {
			return errors.New("RESEND_API_KEY must be set when BILLING_ENABLED is true")
		}
	}

	return nil
}

func setDefaults() {
	viper.SetDefault("PORT", ":8080")
	viper.SetDefault("ENV", "development")
	// 15m matches SPEC-API §4.1 / NFR-1: short access token TTL minimises the window
	// during which a revoked or plan-changed token remains valid.
	viper.SetDefault("JWT_TOKEN_DURATION", "15m")
	viper.SetDefault("JWT_REFRESH_TOKEN_DURATION", "168h")
	viper.SetDefault("MAGIC_LINK_TOKEN_DURATION", "1h")
	viper.SetDefault("DATABASE_URL", "postgresql://postgres:postgres@localhost:5432/runer_notes")
	viper.SetDefault("DATABASE_LOG_LEVEL", "warn")
	viper.SetDefault("DATABASE_MAX_IDLE_CONNS", 10)
	viper.SetDefault("DATABASE_MAX_OPEN_CONNS", 100)
	viper.SetDefault("DATABASE_CONN_MAX_LIFETIME", "1h")
	viper.SetDefault("APP_BASE_URL", "http://localhost:8080")
	viper.SetDefault("FREE_NOTE_LIMIT", 50)
	viper.SetDefault("RATE_LIMIT_PER_MINUTE", 40)
	viper.SetDefault("RATE_LIMIT_BURST", 15)
	viper.SetDefault("EMAIL_FROM", "noreply@example.com")

	// Default to localhost-only origins so the server is usable out of the box
	// for local development without exposing the API to arbitrary web origins.
	// Override with CORS_ALLOWED_ORIGINS in production (Dokploy / env var).
	viper.SetDefault("CORS_ALLOWED_ORIGINS", "http://localhost:5173,http://localhost:1420,http://localhost:8080")

	viper.SetDefault("BILLING_ENABLED", false)
	viper.SetDefault("STRIPE_SUCCESS_URL", "https://runer.app/billing/success")
	viper.SetDefault("STRIPE_CANCEL_URL", "https://runer.app/billing/cancel")
}

func Load(target any) error {
	viper.SetConfigFile(".env")
	viper.AutomaticEnv()
	setDefaults()
	_ = viper.BindEnv("JWT_SECRET")
	_ = viper.BindEnv("RESEND_API_KEY")
	_ = viper.BindEnv("BILLING_ENABLED")
	_ = viper.BindEnv("STRIPE_SECRET_KEY")
	_ = viper.BindEnv("STRIPE_WEBHOOK_SECRET")
	_ = viper.BindEnv("STRIPE_PRICE_ID")
	_ = viper.BindEnv("STRIPE_SUCCESS_URL")
	_ = viper.BindEnv("STRIPE_CANCEL_URL")
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
