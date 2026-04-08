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

// TestConfig_Port_Default asserts that the PORT default is the integer 8080
// with no colon prefix — the bind address colon is added by the server at
// startup via fmt.Sprintf(":%d", cfg.Port).
func TestConfig_Port_Default(t *testing.T) {
	// Cannot run in parallel: mutates global viper state.
	viper.Reset()
	t.Cleanup(viper.Reset)

	setDefaults()

	got := viper.GetInt("PORT")
	assert.Equal(t, 8080, got, "PORT default must be integer 8080")
}

func TestConfig_BillingDefaults(t *testing.T) {
	viper.Reset()
	t.Cleanup(viper.Reset)

	setDefaults()

	assert.False(t, viper.GetBool("BILLING_ENABLED"))
	assert.Equal(t, "https://runer.app/billing/success", viper.GetString("STRIPE_SUCCESS_URL"))
	assert.Equal(t, "https://runer.app/billing/cancel", viper.GetString("STRIPE_CANCEL_URL"))
}

func TestConfig_CORSAllowedOrigins_Default(t *testing.T) {
	// Cannot run in parallel: mutates global viper state.
	viper.Reset()
	t.Cleanup(viper.Reset)

	setDefaults()

	raw := viper.GetString("CORS_ALLOWED_ORIGINS")
	assert.NotEmpty(t, raw, "CORS_ALLOWED_ORIGINS must have a non-empty default")

	// The default must cover the standard Vite dev ports and the API itself.
	cfg := Config{CORSAllowedOrigins: raw}
	origins := cfg.ParsedCORSOrigins()
	assert.Contains(t, origins, "http://localhost:5173")
	assert.Contains(t, origins, "http://localhost:1420")
	assert.Contains(t, origins, "http://localhost:8080")
}

// TestConfig_MaxRequestBodyBytes_Default asserts that the viper default for
// MAX_REQUEST_BODY is "1M" and resolves to exactly 1 MiB.
func TestConfig_MaxRequestBodyBytes_Default(t *testing.T) {
	// Cannot run in parallel: mutates global viper state.
	viper.Reset()
	t.Cleanup(viper.Reset)

	setDefaults()

	raw := viper.GetString("MAX_REQUEST_BODY")
	assert.Equal(t, "1M", raw, "MAX_REQUEST_BODY default must be 1M")

	cfg := Config{MaxRequestBody: raw}
	assert.Equal(t, int64(1<<20), cfg.MaxRequestBodyBytes())
}

// TestConfig_MaxRequestBodyBytes_Parsing covers the full range of supported
// suffix formats and edge cases for MaxRequestBodyBytes().
func TestConfig_MaxRequestBodyBytes_Parsing(t *testing.T) {
	t.Parallel()

	const mib = int64(1 << 20) // fallback default

	tests := []struct {
		name  string
		input string
		want  int64
	}{
		{"kilobytes", "512K", 512 * (1 << 10)},
		{"megabytes", "2M", 2 * (1 << 20)},
		{"gigabytes", "1G", 1 << 30},
		{"bare bytes", "65536", 65536},
		{"empty falls back to 1M", "", mib},
		{"invalid text falls back to 1M", "banana", mib},
		{"zero value falls back to 1M", "0M", mib},
		{"negative value falls back to 1M", "-1M", mib},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			cfg := Config{MaxRequestBody: tc.input}
			assert.Equal(t, tc.want, cfg.MaxRequestBodyBytes())
		})
	}
}

func TestConfig_ParsedCORSOrigins(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		raw  string
		want []string
	}{
		{
			name: "single origin",
			raw:  "https://runer.app",
			want: []string{"https://runer.app"},
		},
		{
			name: "multiple origins",
			raw:  "https://runer.app,app://localhost,capacitor://localhost",
			want: []string{"https://runer.app", "app://localhost", "capacitor://localhost"},
		},
		{
			name: "whitespace trimmed",
			raw:  "https://runer.app , app://localhost , http://localhost",
			want: []string{"https://runer.app", "app://localhost", "http://localhost"},
		},
		{
			name: "empty entries skipped",
			raw:  "https://runer.app,,http://localhost",
			want: []string{"https://runer.app", "http://localhost"},
		},
		{
			name: "empty string returns nil",
			raw:  "",
			want: nil,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			cfg := Config{CORSAllowedOrigins: tc.raw}
			got := cfg.ParsedCORSOrigins()
			assert.Equal(t, tc.want, got)
		})
	}
}
