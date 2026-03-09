package users

import (
	"net/http"

	"github.com/google/uuid"
	"github.com/labstack/echo/v5"
)

type UsersHandler struct {
	service service
}

type service interface {
	GetUserByID(id uuid.UUID) (User, error)
	UpdateUser(user User) (User, error)
	DeleteUser(id uuid.UUID) error
}

func NewUsersHandler(service service) *UsersHandler {
	return &UsersHandler{service: service}
}

func (h *UsersHandler) GetUserByID(c *echo.Context) error {
	return c.JSON(http.StatusOK, nil)
}

func (h *UsersHandler) UpdateUser(c *echo.Context) error {

	return c.JSON(http.StatusOK, nil)
}

func (h *UsersHandler) DeleteUser(c *echo.Context) error {
	return c.JSON(http.StatusOK, nil)
}
