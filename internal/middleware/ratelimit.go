package middleware

import (
	"net/http"
	"time"

	"github.com/labstack/echo/v5"
	"github.com/labstack/echo/v5/middleware"
)

func RateLimiter(perMinute int, burst int) echo.MiddlewareFunc {
	store := middleware.NewRateLimiterMemoryStoreWithConfig(
		middleware.RateLimiterMemoryStoreConfig{
			Rate:      float64(perMinute) / 60.0,
			Burst:     burst,
			ExpiresIn: 3 * time.Minute,
		},
	)
	return middleware.RateLimiterWithConfig(middleware.RateLimiterConfig{
		Store: store,
		IdentifierExtractor: func(c *echo.Context) (string, error) {
			return c.RealIP(), nil
		},
		ErrorHandler: func(c *echo.Context, err error) error {
			return c.JSON(http.StatusTooManyRequests,
				map[string]string{"error": "rate limit exceeded", "code": "RATE_LIMITED"})
		},
		DenyHandler: func(c *echo.Context, id string, err error) error {
			return c.JSON(http.StatusTooManyRequests,
				map[string]string{"error": "rate limit exceeded", "code": "RATE_LIMITED"})
		},
	})
}
