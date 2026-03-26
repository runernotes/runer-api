package users

import (
	"context"
	"errors"
	"net/http"

	"github.com/google/uuid"
	"github.com/labstack/echo/v5"
	"github.com/runernotes/runer-api/internal/api"
	internalmw "github.com/runernotes/runer-api/internal/middleware"
)

// userService defines the subset of UsersService methods required by UsersHandler.
type userService interface {
	GetByID(ctx context.Context, id uuid.UUID) (User, error)
	Activate(ctx context.Context, id uuid.UUID) (User, error)
}

// UsersHandler handles user-related HTTP requests.
type UsersHandler struct {
	service userService
}

// NewUsersHandler constructs a UsersHandler with the given service.
func NewUsersHandler(service userService) *UsersHandler {
	return &UsersHandler{service: service}
}

// getUserID extracts the authenticated user's UUID from the Echo context.
// It follows the same pattern as notesHandler.getUserID.
func getUserID(c *echo.Context) (uuid.UUID, error) {
	val := c.Get(internalmw.UserContextKey)
	if val == nil {
		return uuid.Nil, errors.New("user_id not found in context")
	}
	userID, ok := val.(uuid.UUID)
	if !ok {
		return uuid.Nil, errors.New("user_id has invalid type")
	}
	return userID, nil
}

// GetMe handles GET /api/v1/users/me.
// Returns the authenticated user's profile including their activation status.
func (h *UsersHandler) GetMe(c *echo.Context) error {
	userID, err := getUserID(c)
	if err != nil {
		return c.JSON(http.StatusUnauthorized, api.ErrorResponse{Error: "unauthorized", Code: "UNAUTHORIZED"})
	}

	user, err := h.service.GetByID(c.Request().Context(), userID)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, api.ErrorResponse{Error: "failed to fetch user", Code: "INTERNAL_ERROR"})
	}

	return c.JSON(http.StatusOK, toUserResponse(user))
}

// Activate handles POST /api/v1/users/me/activate.
// Sets activated_at on the user if not already set. Always returns 204 No Content.
func (h *UsersHandler) Activate(c *echo.Context) error {
	userID, err := getUserID(c)
	if err != nil {
		return c.JSON(http.StatusUnauthorized, api.ErrorResponse{Error: "unauthorized", Code: "UNAUTHORIZED"})
	}

	if _, err := h.service.Activate(c.Request().Context(), userID); err != nil {
		return c.JSON(http.StatusInternalServerError, api.ErrorResponse{Error: "failed to activate user", Code: "INTERNAL_ERROR"})
	}

	return c.NoContent(http.StatusNoContent)
}
