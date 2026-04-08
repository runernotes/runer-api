package auth

import (
	"context"
	"errors"
	"net/http"
	"net/url"

	"github.com/labstack/echo/v5"
	"github.com/rs/zerolog"
	"github.com/runernotes/runer-api/internal/api"
)

type AuthHandler struct {
	service service
}

type service interface {
	LoginWithMagicLink(ctx context.Context, token string) (*LoginResponse, error)
	CreateMagicLink(ctx context.Context, email string) error
	Register(ctx context.Context, email string, name string) error
	RefreshAccessToken(ctx context.Context, rawRefreshToken string) (*LoginResponse, error)
	Logout(ctx context.Context, rawRefreshToken string) error
}

func NewAuthHandler(service service) *AuthHandler {
	return &AuthHandler{service: service}
}

func (h *AuthHandler) RequestMagicLink(c *echo.Context) error {
	var request MagicLinkRequest
	if err := c.Bind(&request); err != nil {
		return c.JSON(http.StatusBadRequest, api.ErrorResponse{Error: "invalid request body", Code: "INVALID_BODY"})
	}
	if err := c.Validate(&request); err != nil {
		return c.JSON(http.StatusBadRequest, api.ErrorResponse{Error: err.Error(), Code: "VALIDATION_ERROR"})
	}

	// Always return success to prevent email enumeration
	_ = h.service.CreateMagicLink(c.Request().Context(), request.Email)

	return c.JSON(http.StatusOK, api.MessageResponse{Message: "If the email is registered, a magic link has been sent."})
}

func (h *AuthHandler) Register(c *echo.Context) error {
	var request RegisterRequest
	if err := c.Bind(&request); err != nil {
		return c.JSON(http.StatusBadRequest, api.ErrorResponse{Error: "invalid request body", Code: "INVALID_BODY"})
	}
	if err := c.Validate(&request); err != nil {
		return c.JSON(http.StatusBadRequest, api.ErrorResponse{Error: err.Error(), Code: "VALIDATION_ERROR"})
	}

	err := h.service.Register(c.Request().Context(), request.Email, request.Name)
	if err != nil && !errors.Is(err, ErrUserAlreadyExists) {
		return c.JSON(http.StatusInternalServerError, api.ErrorResponse{Error: "registration failed", Code: "REGISTRATION_FAILED"})
	}

	return c.JSON(http.StatusOK, api.MessageResponse{Message: "If the email is not registered, a confirmation link has been sent."})
}

func (h *AuthHandler) Verify(c *echo.Context) error {
	var request VerifyRequest
	if err := c.Bind(&request); err != nil {
		return c.JSON(http.StatusBadRequest, api.ErrorResponse{Error: "invalid request body", Code: "INVALID_BODY"})
	}
	if err := c.Validate(&request); err != nil {
		return c.JSON(http.StatusBadRequest, api.ErrorResponse{Error: err.Error(), Code: "VALIDATION_ERROR"})
	}

	resp, err := h.service.LoginWithMagicLink(c.Request().Context(), request.Token)
	if err != nil {
		return c.JSON(http.StatusUnauthorized, api.ErrorResponse{Error: "invalid or expired token", Code: "INVALID_TOKEN"})
	}
	return c.JSON(http.StatusOK, resp)
}

func (h *AuthHandler) VerifyRedirect(c *echo.Context) error {
	token := c.QueryParam("token")
	if token == "" {
		return c.JSON(http.StatusBadRequest, api.ErrorResponse{Error: "token query parameter is required", Code: "MISSING_TOKEN"})
	}
	redirectURL := "runer://auth/verify?" + url.Values{"token": {token}}.Encode()
	return c.Redirect(http.StatusFound, redirectURL)
}

func (h *AuthHandler) Refresh(c *echo.Context) error {
	var request RefreshRequest
	if err := c.Bind(&request); err != nil {
		return c.JSON(http.StatusBadRequest, api.ErrorResponse{Error: "invalid request body", Code: "INVALID_BODY"})
	}
	if err := c.Validate(&request); err != nil {
		return c.JSON(http.StatusBadRequest, api.ErrorResponse{Error: err.Error(), Code: "VALIDATION_ERROR"})
	}

	resp, err := h.service.RefreshAccessToken(c.Request().Context(), request.RefreshToken)
	if err != nil {
		return c.JSON(http.StatusUnauthorized, api.ErrorResponse{Error: "invalid or expired refresh token", Code: "INVALID_REFRESH_TOKEN"})
	}
	return c.JSON(http.StatusOK, resp)
}

func (h *AuthHandler) Logout(c *echo.Context) error {
	var request LogoutRequest
	if err := c.Bind(&request); err != nil {
		return c.JSON(http.StatusBadRequest, api.ErrorResponse{Error: "invalid request body", Code: "INVALID_BODY"})
	}
	if err := c.Validate(&request); err != nil {
		return c.JSON(http.StatusBadRequest, api.ErrorResponse{Error: err.Error(), Code: "VALIDATION_ERROR"})
	}

	if err := h.service.Logout(c.Request().Context(), request.RefreshToken); err != nil {
		// Log but still return 204 — don't leak token validity information.
		zerolog.Ctx(c.Request().Context()).Warn().Err(err).Msg("logout error")
	}
	return c.NoContent(http.StatusNoContent)
}
