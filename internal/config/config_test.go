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
//
// JWTSecret is 32 bytes (Task 4 minimum).
// ResendAPIKey and EmailFrom are populated because email is always required (Task 1).
func baseValidConfig() Config {
	return Config{
		JWTSecret:          "12345678901234567890123456789012", // exactly 32 bytes
		RateLimitPerMinute: 40,
		RateLimitBurst:     15,
		ResendAPIKey:       "re_test_123",
		EmailFrom:          "noreply@example.com",
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
		// Task 4: JWT_SECRET length enforcement
		{"missing jwt secret", func(c *Config) { c.JWTSecret = "" }, "JWT_SECRET must be set"},
		{"jwt secret 31 bytes rejected", func(c *Config) { c.JWTSecret = "1234567890123456789012345678901" }, "JWT_SECRET must be at least 32 bytes"},
		{"jwt secret 32 bytes accepted", func(c *Config) { c.JWTSecret = "12345678901234567890123456789012" }, ""},
		// Task 1: Resend / email always required (regardless of BillingEnabled)
		{"missing resend api key", func(c *Config) { c.ResendAPIKey = "" }, "RESEND_API_KEY must be set"},
		{"missing email from", func(c *Config) { c.EmailFrom = "" }, "EMAIL_FROM must be set"},
		// Rate limits
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
	// ResendAPIKey/EmailFrom come from baseValidConfig(); they are globally required
	// (Task 1) so they are not billing-specific here.
	fullBilling := func() Config {
		c := baseValidConfig()
		c.BillingEnabled = true
		c.StripeSecretKey = "sk_test_123"
		c.StripeWebhookSecret = "whsec_123"
		c.StripePriceID = "price_123"
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

// TestConfig_Validate_ResendAlwaysRequired locks in the Task-1 fix: magic link auth
// always sends email, so RESEND_API_KEY and EMAIL_FROM must be present regardless of
// whether billing is enabled.
func TestConfig_Validate_ResendAlwaysRequired(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		mutate  func(*Config)
		wantErr string
	}{
		{
			"billing disabled, empty resend key → error",
			func(c *Config) { c.BillingEnabled = false; c.ResendAPIKey = "" },
			"RESEND_API_KEY must be set",
		},
		{
			"billing disabled, empty email from → error",
			func(c *Config) { c.BillingEnabled = false; c.EmailFrom = "" },
			"EMAIL_FROM must be set",
		},
		{
			"billing enabled, empty resend key → error",
			func(c *Config) {
				c.BillingEnabled = true
				c.StripeSecretKey = "sk_test_123"
				c.StripeWebhookSecret = "whsec_123"
				c.StripePriceID = "price_123"
				c.ResendAPIKey = ""
			},
			"RESEND_API_KEY must be set",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			cfg := baseValidConfig()
			tc.mutate(&cfg)
			err := cfg.Validate()
			require.Error(t, err)
			assert.Contains(t, err.Error(), tc.wantErr)
		})
	}
}

// TestConfig_Validate_JWTSecretMinLength locks in the Task-4 fix: a secret shorter
// than 32 bytes makes HS256 trivially brute-forceable.
func TestConfig_Validate_JWTSecretMinLength(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		secret  string
		wantErr bool
	}{
		{"empty secret", "", true},
		{"1 byte", "x", true},
		{"31 bytes", "1234567890123456789012345678901", true},
		{"exactly 32 bytes", "12345678901234567890123456789012", false},
		{"33 bytes", "123456789012345678901234567890123", false},
		{"64 hex bytes (openssl rand output)", "a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2", false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			cfg := baseValidConfig()
			cfg.JWTSecret = tc.secret
			err := cfg.Validate()
			if tc.wantErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), "JWT_SECRET")
			} else {
				require.NoError(t, err)
			}
		})
	}
}

// TestConfig_Validate_ProductionCORS locks in the Task-5 fix: a production deploy
// must not boot with localhost origins, which would silently block real clients.
func TestConfig_Validate_ProductionCORS(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		origins string
		wantErr string
	}{
		{
			"empty origins in production",
			"",
			"CORS_ALLOWED_ORIGINS must be set in production",
		},
		{
			"localhost origin in production",
			"http://localhost:5173",
			"CORS_ALLOWED_ORIGINS must not contain localhost in production",
		},
		{
			"localhost mixed with real origin in production",
			"https://runer.app,http://localhost:5173",
			"CORS_ALLOWED_ORIGINS must not contain localhost in production",
		},
		{
			"real origins only in production",
			"https://runer.app,app://localhost.runer.com",
			// "localhost" substring in a domain like "localhost.runer.com" is intentionally
			// caught — operators should use the actual production domain only.
			"CORS_ALLOWED_ORIGINS must not contain localhost in production",
		},
		{
			"proper production origins",
			"https://runer.app,capacitor://runer.app",
			"",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			cfg := baseValidConfig()
			cfg.Env = "production"
			cfg.CORSAllowedOrigins = tc.origins
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

// TestConfig_Validate_DevelopmentCORSIgnored confirms that the CORS production
// guards do not fire in the default development environment.
func TestConfig_Validate_DevelopmentCORSIgnored(t *testing.T) {
	t.Parallel()

	cfg := baseValidConfig()
	cfg.Env = "development"
	cfg.CORSAllowedOrigins = "http://localhost:5173"
	require.NoError(t, cfg.Validate(), "localhost CORS must be allowed in development")
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

// TestConfig_RefreshTokenDuration_Default locks in the Task-3 fix: spec §3.3/§4
// requires a 30-day (720h) refresh-token TTL. A previous regression defaulted to
// 168h (7 days), logging users out 4× too often.
func TestConfig_RefreshTokenDuration_Default(t *testing.T) {
	// Cannot run in parallel: mutates global viper state.
	viper.Reset()
	t.Cleanup(viper.Reset)

	setDefaults()

	got := viper.GetDuration("JWT_REFRESH_TOKEN_DURATION")
	assert.Equal(t, 720*time.Hour, got, "JWT_REFRESH_TOKEN_DURATION default must be 720h (30 days)")
}

// TestConfig_MagicLinkTokenDuration_Default locks in the Task-2 fix: spec §3.2/§4.5.6/§8
// requires a 15-minute magic link TTL. A previous regression defaulted to 1h,
// 4× the specified value, widening the replay window for leaked tokens.
func TestConfig_MagicLinkTokenDuration_Default(t *testing.T) {
	// Cannot run in parallel: mutates global viper state.
	viper.Reset()
	t.Cleanup(viper.Reset)

	setDefaults()

	got := viper.GetDuration("MAGIC_LINK_TOKEN_DURATION")
	assert.Equal(t, 15*time.Minute, got, "MAGIC_LINK_TOKEN_DURATION default must be 15m")
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
