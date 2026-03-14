package middleware

import (
	"time"

	"github.com/google/uuid"
	"github.com/labstack/echo/v5"
	"github.com/rs/zerolog/log"
)

func ConsoleAnalytics() echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c *echo.Context) error {
			start := time.Now()
			err := next(c)
			latency := time.Since(start)

			userID, _ := c.Get("user_id").(uuid.UUID)
			resp, _ := c.Response().(*echo.Response)
			status := 0
			var bytesOut int64
			if resp != nil {
				status = resp.Status
				bytesOut = resp.Size
			}

			log.Info().
				Str("event", "api_request").
				Str("method", c.Request().Method).
				Str("path", c.Request().URL.Path).
				Str("ip", c.RealIP()).
				Str("user_id", userID.String()).
				Int("status", status).
				Int64("latency_ms", latency.Milliseconds()).
				Int64("bytes_out", bytesOut).
				Msg("")

			return err
		}
	}
}
