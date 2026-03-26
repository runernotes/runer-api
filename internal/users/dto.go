package users

import (
	"time"

	"github.com/google/uuid"
)

// UserResponse is the JSON representation of a user returned from the API.
type UserResponse struct {
	ID          uuid.UUID  `json:"id"`
	Name        string     `json:"name"`
	Email       string     `json:"email"`
	ActivatedAt *time.Time `json:"activated_at"`
}

// toUserResponse converts a User model to its API response representation.
func toUserResponse(u User) UserResponse {
	return UserResponse{
		ID:          u.ID,
		Name:        u.Name,
		Email:       u.Email,
		ActivatedAt: u.ActivatedAt,
	}
}
