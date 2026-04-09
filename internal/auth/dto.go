package auth

type RegisterRequest struct {
	Email string `json:"email" validate:"required,email"`
	Name  string `json:"name" validate:"required"`
}

type MagicLinkRequest struct {
	Email string `json:"email" validate:"required,email"`
}

type VerifyRequest struct {
	Token string `json:"token" validate:"required"`
}

type RefreshRequest struct {
	RefreshToken string `json:"refresh_token" validate:"required"`
}

type LogoutRequest struct {
	RefreshToken string `json:"refresh_token" validate:"required"`
}

type LoginResponse struct {
	// UserID is the authenticated user's UUID. Clients can use this to
	// initialise their local state without needing to decode the JWT.
	UserID       string `json:"user_id"`
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
}
