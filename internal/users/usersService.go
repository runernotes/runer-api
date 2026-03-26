package users

import (
	"context"

	"github.com/google/uuid"
)

type UsersService struct {
	repository repository
}

type repository interface {
	FindByID(ctx context.Context, id uuid.UUID) (User, error)
	Create(ctx context.Context, user User) (User, error)
	Update(ctx context.Context, user User) (User, error)
	Delete(ctx context.Context, id uuid.UUID) error
	Activate(ctx context.Context, id uuid.UUID) (User, error)
}

func NewUsersService(repository repository) *UsersService {
	return &UsersService{repository: repository}
}

// GetByID looks up a user by their ID.
func (s *UsersService) GetByID(ctx context.Context, id uuid.UUID) (User, error) {
	return s.repository.FindByID(ctx, id)
}

// Create creates a new user.
func (s *UsersService) Create(ctx context.Context, user User) (User, error) {
	return s.repository.Create(ctx, user)
}

// Update persists changes to an existing user.
func (s *UsersService) Update(ctx context.Context, user User) (User, error) {
	return s.repository.Update(ctx, user)
}

// Delete removes a user by ID.
func (s *UsersService) Delete(ctx context.Context, id uuid.UUID) error {
	return s.repository.Delete(ctx, id)
}

// Activate sets activated_at on the user if it is currently unset. The operation
// is idempotent: if the user is already activated the existing timestamp is preserved.
// Returns the (possibly unchanged) user.
func (s *UsersService) Activate(ctx context.Context, id uuid.UUID) (User, error) {
	return s.repository.Activate(ctx, id)
}
