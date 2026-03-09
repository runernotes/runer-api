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
}

func NewUsersService(repository repository) *UsersService {
	return &UsersService{repository: repository}
}

func (s *UsersService) Create(ctx context.Context, user User) (User, error) {
	return s.repository.Create(ctx, user)
}

func (s *UsersService) Update(ctx context.Context, user User) (User, error) {
	return s.repository.Update(ctx, user)
}

func (s *UsersService) Delete(ctx context.Context, id uuid.UUID) error {
	return s.repository.Delete(ctx, id)
}
