package auth

import "errors"

var (
	ErrUserNotFound        = errors.New("user not found")
	ErrUserAlreadyExists   = errors.New("user already exists")
	ErrInvalidToken        = errors.New("invalid token")
	ErrExpiredToken        = errors.New("token has expired")
	ErrMissingToken        = errors.New("token is required")
	ErrTokenAlreadyUsed    = errors.New("token already used")
	ErrInvalidRefreshToken = errors.New("invalid refresh token")
	ErrExpiredRefreshToken = errors.New("refresh token has expired")
	ErrRevokedRefreshToken = errors.New("refresh token has been revoked")
)
