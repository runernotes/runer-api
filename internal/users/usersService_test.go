package users

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mockUsersRepository implements the repository interface for unit testing.
type mockUsersRepository struct {
	findByIDFn func(ctx context.Context, id uuid.UUID) (User, error)
	createFn   func(ctx context.Context, user User) (User, error)
	updateFn   func(ctx context.Context, user User) (User, error)
	deleteFn   func(ctx context.Context, id uuid.UUID) error
	activateFn func(ctx context.Context, id uuid.UUID) (User, error)
}

func (m *mockUsersRepository) FindByID(ctx context.Context, id uuid.UUID) (User, error) {
	if m.findByIDFn != nil {
		return m.findByIDFn(ctx, id)
	}
	return User{}, nil
}

func (m *mockUsersRepository) Create(ctx context.Context, user User) (User, error) {
	if m.createFn != nil {
		return m.createFn(ctx, user)
	}
	return user, nil
}

func (m *mockUsersRepository) Update(ctx context.Context, user User) (User, error) {
	if m.updateFn != nil {
		return m.updateFn(ctx, user)
	}
	return user, nil
}

func (m *mockUsersRepository) Delete(ctx context.Context, id uuid.UUID) error {
	if m.deleteFn != nil {
		return m.deleteFn(ctx, id)
	}
	return nil
}

func (m *mockUsersRepository) Activate(ctx context.Context, id uuid.UUID) (User, error) {
	if m.activateFn != nil {
		return m.activateFn(ctx, id)
	}
	now := time.Now().UTC()
	return User{ID: id, ActivatedAt: &now}, nil
}

// TestActivate_SetsActivatedAt verifies that Activate delegates to the repository
// and returns the updated user with a non-nil ActivatedAt.
func TestActivate_SetsActivatedAt(t *testing.T) {
	ctx := context.Background()
	userID := uuid.New()
	now := time.Now().UTC()

	repo := &mockUsersRepository{
		activateFn: func(_ context.Context, id uuid.UUID) (User, error) {
			assert.Equal(t, userID, id)
			return User{ID: id, ActivatedAt: &now}, nil
		},
	}

	svc := NewUsersService(repo)
	user, err := svc.Activate(ctx, userID)
	require.NoError(t, err)
	require.NotNil(t, user.ActivatedAt)
	assert.Equal(t, userID, user.ID)
}

// TestActivate_Idempotent verifies that calling Activate on an already-activated user
// succeeds without error (the repository handles idempotency via WHERE activated_at IS NULL).
func TestActivate_Idempotent(t *testing.T) {
	ctx := context.Background()
	userID := uuid.New()
	original := time.Now().Add(-time.Hour).UTC()

	repo := &mockUsersRepository{
		activateFn: func(_ context.Context, id uuid.UUID) (User, error) {
			// Repository returns the user with the original (unchanged) activated_at.
			return User{ID: id, ActivatedAt: &original}, nil
		},
	}

	svc := NewUsersService(repo)
	user, err := svc.Activate(ctx, userID)
	require.NoError(t, err)
	require.NotNil(t, user.ActivatedAt)
	assert.Equal(t, original, *user.ActivatedAt)
}

// TestActivate_RepositoryError verifies that a repository error is propagated.
func TestActivate_RepositoryError(t *testing.T) {
	ctx := context.Background()
	userID := uuid.New()
	repoErr := errors.New("db error")

	repo := &mockUsersRepository{
		activateFn: func(_ context.Context, _ uuid.UUID) (User, error) {
			return User{}, repoErr
		},
	}

	svc := NewUsersService(repo)
	_, err := svc.Activate(ctx, userID)
	assert.ErrorIs(t, err, repoErr)
}

// TestGetByID_DelegatesToRepository verifies that GetByID forwards to the repository.
func TestGetByID_DelegatesToRepository(t *testing.T) {
	ctx := context.Background()
	userID := uuid.New()
	expected := User{ID: userID, Email: "test@example.com", Name: "Test"}

	repo := &mockUsersRepository{
		findByIDFn: func(_ context.Context, id uuid.UUID) (User, error) {
			assert.Equal(t, userID, id)
			return expected, nil
		},
	}

	svc := NewUsersService(repo)
	got, err := svc.GetByID(ctx, userID)
	require.NoError(t, err)
	assert.Equal(t, expected, got)
}
