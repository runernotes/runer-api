package middleware

import (
	"net/url"
	"strings"

	"github.com/labstack/echo/v5"
	echomw "github.com/labstack/echo/v5/middleware"
	"github.com/rs/zerolog/log"
)

// sensitiveQueryParams is the set of query parameter names whose values are
// replaced with "[REDACTED]" in access logs. Add any future credential-carrying
// parameter names here — the middleware stays route-agnostic.
var sensitiveQueryParams = map[string]struct{}{
	"token": {},
}

// redactQueryString parses rawURI, replaces the values of any parameter whose
// name appears in sensitiveQueryParams with "[REDACTED]", and returns the full
// URI with the sanitised query string appended. URIs without a query string are
// returned unchanged.
func redactQueryString(rawURI string) string {
	path, raw, hasQuery := strings.Cut(rawURI, "?")
	if !hasQuery {
		return rawURI // no query string — nothing to redact
	}

	params, err := url.ParseQuery(raw)
	if err != nil {
		// Malformed query string — log the path only rather than risking
		// leaking a partially-parsed value.
		return path
	}

	redacted := false
	for name := range sensitiveQueryParams {
		if _, exists := params[name]; exists {
			params[name] = []string{"[REDACTED]"}
			redacted = true
		}
	}

	if !redacted {
		return rawURI // nothing sensitive — return original to avoid re-encoding
	}
	return path + "?" + params.Encode()
}

// ZerologRequestLogger returns an Echo middleware that derives a per-request
// zerolog.Logger carrying the request_id injected by middleware.RequestID(),
// and stores it on the request context. Downstream handlers retrieve it via:
//
//	zerolog.Ctx(c.Request().Context()).Info()...
//
// Every log line emitted that way will automatically include the request_id
// field, making it trivial to correlate all log output for a single request.
//
// This middleware must be registered AFTER middleware.RequestID() in the stack.
func ZerologRequestLogger() echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c *echo.Context) error {
			reqID := c.Response().Header().Get(echo.HeaderXRequestID)
			// Derive a logger copy that carries request_id. We derive from
			// log.Logger (the global, already configured by logging.Setup) so
			// the writer chain (stdout/PostHog) is inherited automatically.
			reqLogger := log.Logger.With().Str("request_id", reqID).Logger()
			ctx := reqLogger.WithContext(c.Request().Context())
			c.SetRequest(c.Request().WithContext(ctx))
			return next(c)
		}
	}
}

// ZerologAccessLogger returns an Echo RequestLoggerWithConfig that emits one
// structured JSON access-log line per completed request using zerolog. It
// replaces middleware.RequestLogger() which emits plain text.
//
// Fields logged: request_id, method, uri, status, remote_ip, latency_ms.
// Query strings are preserved for diagnostic value (e.g. ?since=...&limit=100)
// but values for parameters listed in sensitiveQueryParams (e.g. "token") are
// replaced with "[REDACTED]" before logging. Access logs are emitted at Info
// level, so they do not reach PostHog (which filters to warn-and-above).
func ZerologAccessLogger() echo.MiddlewareFunc {
	return echomw.RequestLoggerWithConfig(echomw.RequestLoggerConfig{
		LogMethod:    true,
		LogURI:       true,
		LogStatus:    true,
		LogLatency:   true,
		LogRequestID: true,
		LogRemoteIP:  true,
		LogValuesFunc: func(c *echo.Context, v echomw.RequestLoggerValues) error {
			log.Logger.Info().
				Str("request_id", v.RequestID).
				Str("method", v.Method).
				Str("uri", redactQueryString(v.URI)).
				Int("status", v.Status).
				Str("remote_ip", v.RemoteIP).
				Int64("latency_ms", v.Latency.Milliseconds()).
				Msg("request")
			return nil
		},
	})
}
