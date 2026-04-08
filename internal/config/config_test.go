package config

import (
	"testing"
	"time"

	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// baseValidConfig returns a Config that passes Validate() for non-billing scenarios.
// Individual tests mutate it before asserting.
func baseValidConfig() Config {
	return Config{
		JWTSecret:          "test-secret",
		RateLimitPerMinute: 40,
		RateLimitBurst:     15,
	}
}

func TestConfig_Validate_Core(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		mutate  func(*Config)
		wantErr string
	}{
		{"valid", func(c *Config) {}, ""},
		{"missing jwt secret", func(c *Config) { c.JWTSecret = "" }, "JWT_SECRET"},
		{"rate limit per minute zero", func(c *Config) { c.RateLimitPerMinute = 0 }, "RATE_LIMIT_PER_MINUTE"},
		{"rate limit burst zero", func(c *Config) { c.RateLimitBurst = 0 }, "RATE_LIMIT_BURST"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			cfg := baseValidConfig()
			tc.mutate(&cfg)
			err := cfg.Validate()
			if tc.wantErr == "" {
				require.NoError(t, err)
			} else {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tc.wantErr)
			}
		})
	}
}

func TestConfig_Validate_Billing(t *testing.T) {
	t.Parallel()

	// A fully-populated billing-enabled config that should pass validation.
	fullBilling := func() Config {
		c := baseValidConfig()
		c.BillingEnabled = true
		c.StripeSecretKey = "sk_test_123"
		c.StripeWebhookSecret = "whsec_123"
		c.StripePriceID = "price_123"
		c.ResendAPIKey = "re_123"
		return c
	}

	tests := []struct {
		name    string
		mutate  func(*Config)
		wantErr string
	}{
		{"valid full billing", func(c *Config) {}, ""},
		{"missing stripe secret", func(c *Config) { c.StripeSecretKey = "" }, "STRIPE_SECRET_KEY"},
		{"missing webhook secret", func(c *Config) { c.StripeWebhookSecret = "" }, "STRIPE_WEBHOOK_SECRET"},
		{"missing price id", func(c *Config) { c.StripePriceID = "" }, "STRIPE_PRICE_ID"},
		{"missing resend key", func(c *Config) { c.ResendAPIKey = "" }, "RESEND_API_KEY"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			cfg := fullBilling()
			tc.mutate(&cfg)
			err := cfg.Validate()
			if tc.wantErr == "" {
				require.NoError(t, err)
			} else {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tc.wantErr)
			}
		})
	}
}

func TestConfig_Validate_BillingDisabled_IgnoresStripe(t *testing.T) {
	t.Parallel()

	cfg := baseValidConfig() // BillingEnabled zero-value = false
	require.NoError(t, cfg.Validate(), "billing disabled must not require stripe vars")
}

// TestConfig_JWTTokenDuration_Default locks in the NFR-1 requirement that the
// default access-token TTL is 15 minutes. A previous regression defaulted to 1h.
func TestConfig_JWTTokenDuration_Default(t *testing.T) {
	// Cannot run in parallel: mutates global viper state.
	viper.Reset()
	t.Cleanup(viper.Reset)

	setDefaults()

	got := viper.GetDuration("JWT_TOKEN_DURATION")
	assert.Equal(t, 15*time.Minute, got, "JWT_TOKEN_DURATION default must be 15m")
}

func TestConfig_BillingDefaults(t *testing.T) {
	viper.Reset()
	t.Cleanup(viper.Reset)

	setDefaults()

	assert.False(t, viper.GetBool("BILLING_ENABLED"))
	assert.Equal(t, "https://runer.app/billing/success", viper.GetString("STRIPE_SUCCESS_URL"))
	assert.Equal(t, "https://runer.app/billing/cancel", viper.GetString("STRIPE_CANCEL_URL"))
}
