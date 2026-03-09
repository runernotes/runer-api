package middleware

import (
	"github.com/labstack/echo/v5"
	"github.com/rs/zerolog/log"
)

const LoggerKey = "logger"

// RequestLogger returns a middleware that logs each request with zerolog.
// It also stores a request-scoped logger in the context under LoggerKey,
// so handlers can retrieve it via c.Get(middleware.LoggerKey).(zerolog.Logger).
func RequestLogger() echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c *echo.Context) error {
			reqLogger := log.With().
				Str("method", c.Request().Method).
				Str("uri", c.Request().RequestURI).
				Logger()
			c.Set(LoggerKey, reqLogger)

			err := next(c)

			status := c.Response().(*echo.Response).Status
			event := reqLogger.Info()
			if err != nil {
				event = reqLogger.Error().Err(err)
			}
			event.Int("status", status).Msg("request")

			return err
		}
	}
}
