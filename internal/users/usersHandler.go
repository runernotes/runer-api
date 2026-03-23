package users

import (
	"net/http"

	"github.com/labstack/echo/v5"
)

// UsersHandler handles user-related HTTP requests.
type UsersHandler struct{}

// NewUsersHandler constructs a UsersHandler.
func NewUsersHandler() *UsersHandler {
	return &UsersHandler{}
}

// GetUserByID handles GET /users/:id (stub — not yet implemented).
func (h *UsersHandler) GetUserByID(c *echo.Context) error {
	return c.JSON(http.StatusOK, nil)
}

// UpdateUser handles PUT /users/:id (stub — not yet implemented).
// When implemented, the request DTO must exclude the Plan field.
func (h *UsersHandler) UpdateUser(c *echo.Context) error {
	return c.JSON(http.StatusOK, nil)
}

func (h *UsersHandler) DeleteUser(c *echo.Context) error {
	return c.JSON(http.StatusOK, nil)
}
