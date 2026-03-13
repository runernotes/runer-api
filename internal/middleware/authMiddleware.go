package middleware

import (
	"net/http"
	"strings"

	"github.com/labstack/echo/v5"
	"github.com/runernotes/runer-api-core/internal/api"
	"github.com/runernotes/runer-api-core/internal/utils"
)

const (
	UserContextKey = "user_id"
)

func AuthMiddleware(jwtManager *utils.JWTManager) echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c *echo.Context) error {
			authHeader := c.Request().Header.Get("Authorization")
			if authHeader == "" {
				return c.JSON(http.StatusUnauthorized, api.ErrorResponse{Error: "missing authorization header", Code: "UNAUTHORIZED"})
			}

			parts := strings.SplitN(authHeader, " ", 2)
			if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") {
				return c.JSON(http.StatusUnauthorized, api.ErrorResponse{Error: "invalid authorization header format", Code: "UNAUTHORIZED"})
			}

			claims, err := jwtManager.ValidateAccessToken(parts[1])
			if err != nil {
				return c.JSON(http.StatusUnauthorized, api.ErrorResponse{Error: "invalid or expired token", Code: "UNAUTHORIZED"})
			}

			c.Set(UserContextKey, claims.UserID)
			return next(c)
		}
	}
}
