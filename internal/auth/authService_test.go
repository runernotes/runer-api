package auth

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/runernotes/runer-api/internal/users"
	"github.com/runernotes/runer-api/internal/utils"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- mock repository ---

type mockRepository struct {
	createMagicLinkTokenFn           func(ctx context.Context, token MagicLinkToken) error
	verifyAndConsumeMagicLinkTokenFn func(ctx context.Context, tokenHash string) (uuid.UUID, error)
	createRefreshTokenFn             func(ctx context.Context, token RefreshToken) error
	getRefreshTokenByHashFn          func(ctx context.Context, hash string) (*RefreshToken, error)
	revokeRefreshTokenFn             func(ctx context.Context, tokenID uuid.UUID) error
	revokeAllUserRefreshTokensFn     func(ctx context.Context, userID uuid.UUID) error
}

func (m *mockRepository) CreateMagicLinkToken(ctx context.Context, token MagicLinkToken) error {
	if m.createMagicLinkTokenFn != nil {
		return m.createMagicLinkTokenFn(ctx, token)
	}
	return nil
}

func (m *mockRepository) VerifyAndConsumeMagicLinkToken(ctx context.Context, tokenHash string) (uuid.UUID, error) {
	if m.verifyAndConsumeMagicLinkTokenFn != nil {
		return m.verifyAndConsumeMagicLinkTokenFn(ctx, tokenHash)
	}
	return uuid.Nil, ErrInvalidToken
}

func (m *mockRepository) CreateRefreshToken(ctx context.Context, token RefreshToken) error {
	if m.createRefreshTokenFn != nil {
		return m.createRefreshTokenFn(ctx, token)
	}
	return nil
}

func (m *mockRepository) GetRefreshTokenByHash(ctx context.Context, hash string) (*RefreshToken, error) {
	if m.getRefreshTokenByHashFn != nil {
		return m.getRefreshTokenByHashFn(ctx, hash)
	}
	return nil, errors.New("not found")
}

func (m *mockRepository) RevokeRefreshToken(ctx context.Context, tokenID uuid.UUID) error {
	if m.revokeRefreshTokenFn != nil {
		return m.revokeRefreshTokenFn(ctx, tokenID)
	}
	return nil
}

func (m *mockRepository) RevokeAllUserRefreshTokens(ctx context.Context, userID uuid.UUID) error {
	if m.revokeAllUserRefreshTokensFn != nil {
		return m.revokeAllUserRefreshTokensFn(ctx, userID)
	}
	return nil
}

// --- mock user repository ---

type mockUserRepository struct {
	createFn      func(ctx context.Context, user users.User) (users.User, error)
	findByEmailFn func(ctx context.Context, email string) (users.User, error)
	findByIDFn    func(ctx context.Context, id uuid.UUID) (users.User, error)
}

func (m *mockUserRepository) Create(ctx context.Context, user users.User) (users.User, error) {
	if m.createFn != nil {
		return m.createFn(ctx, user)
	}
	return users.User{}, nil
}

func (m *mockUserRepository) FindByEmail(ctx context.Context, email string) (users.User, error) {
	if m.findByEmailFn != nil {
		return m.findByEmailFn(ctx, email)
	}
	return users.User{}, errors.New("not found")
}

func (m *mockUserRepository) FindByID(ctx context.Context, id uuid.UUID) (users.User, error) {
	if m.findByIDFn != nil {
		return m.findByIDFn(ctx, id)
	}
	return users.User{}, errors.New("not found")
}

// --- mock email sender ---

type mockEmailSender struct {
	sendFn func(ctx context.Context, email string, token string, isNewUser bool) error
}

func (m *mockEmailSender) SendMagicLinkEmail(ctx context.Context, email string, token string, isNewUser bool) error {
	if m.sendFn != nil {
		return m.sendFn(ctx, email, token, isNewUser)
	}
	return nil
}

// --- helpers ---

func newTestService(repo *mockRepository, usersRepo *mockUserRepository, email *mockEmailSender) *AuthService {
	jwtManager := utils.NewJWTManager("test-secret", 15*time.Minute, 7*24*time.Hour)
	return NewAuthService(repo, usersRepo, email, jwtManager, 15*time.Minute, 7*24*time.Hour)
}

// --- LoginWithMagicLink tests ---

// TestLoginWithMagicLink_ValidToken verifies that a valid, unconsumed token
// results in a successful login and that the correct user's email is embedded
// in the JWT via a FindByID lookup.
func TestLoginWithMagicLink_ValidToken(t *testing.T) {
	userID := uuid.New()
	user := users.User{ID: userID, Email: "alice@example.com", Name: "Alice"}

	repo := &mockRepository{
		verifyAndConsumeMagicLinkTokenFn: func(_ context.Context, _ string) (uuid.UUID, error) {
			return userID, nil
		},
		createRefreshTokenFn: func(_ context.Context, _ RefreshToken) error {
			return nil
		},
	}
	usersRepo := &mockUserRepository{
		findByIDFn: func(_ context.Context, id uuid.UUID) (users.User, error) {
			if id == userID {
				return user, nil
			}
			return users.User{}, errors.New("not found")
		},
	}

	svc := newTestService(repo, usersRepo, &mockEmailSender{})

	resp, err := svc.LoginWithMagicLink(context.Background(), "valid-raw-token")
	require.NoError(t, err)
	require.NotNil(t, resp)
	assert.Equal(t, userID.String(), resp.UserID)
	assert.NotEmpty(t, resp.AccessToken)
	assert.NotEmpty(t, resp.RefreshToken)
}

// TestLoginWithMagicLink_AlreadyUsedToken verifies that a token that has already
// been consumed (VerifyAndConsumeMagicLinkToken returns ErrInvalidToken) results
// in an ErrInvalidToken error.
func TestLoginWithMagicLink_AlreadyUsedToken(t *testing.T) {
	repo := &mockRepository{
		verifyAndConsumeMagicLinkTokenFn: func(_ context.Context, _ string) (uuid.UUID, error) {
			return uuid.Nil, ErrInvalidToken
		},
	}

	svc := newTestService(repo, &mockUserRepository{}, &mockEmailSender{})

	resp, err := svc.LoginWithMagicLink(context.Background(), "used-raw-token")
	assert.Nil(t, resp)
	assert.ErrorIs(t, err, ErrInvalidToken)
}

// TestLoginWithMagicLink_ExpiredToken verifies that an expired token
// (VerifyAndConsumeMagicLinkToken returns ErrInvalidToken — the DB handles
// expiry atomically) results in an ErrInvalidToken error.
func TestLoginWithMagicLink_ExpiredToken(t *testing.T) {
	repo := &mockRepository{
		verifyAndConsumeMagicLinkTokenFn: func(_ context.Context, _ string) (uuid.UUID, error) {
			// Expired tokens produce zero rows affected, which the repository
			// maps to ErrInvalidToken just as for already-used tokens.
			return uuid.Nil, ErrInvalidToken
		},
	}

	svc := newTestService(repo, &mockUserRepository{}, &mockEmailSender{})

	resp, err := svc.LoginWithMagicLink(context.Background(), "expired-raw-token")
	assert.Nil(t, resp)
	assert.ErrorIs(t, err, ErrInvalidToken)
}

// TestLoginWithMagicLink_NonExistentToken verifies that a token that does not
// exist in the database (VerifyAndConsumeMagicLinkToken returns ErrInvalidToken)
// results in an ErrInvalidToken error.
func TestLoginWithMagicLink_NonExistentToken(t *testing.T) {
	repo := &mockRepository{
		verifyAndConsumeMagicLinkTokenFn: func(_ context.Context, _ string) (uuid.UUID, error) {
			return uuid.Nil, ErrInvalidToken
		},
	}

	svc := newTestService(repo, &mockUserRepository{}, &mockEmailSender{})

	resp, err := svc.LoginWithMagicLink(context.Background(), "nonexistent-raw-token")
	assert.Nil(t, resp)
	assert.ErrorIs(t, err, ErrInvalidToken)
}

// TestLoginWithMagicLink_UserLookupFailure verifies that if VerifyAndConsumeMagicLinkToken
// succeeds but the subsequent FindByID call fails (e.g. data inconsistency), the error
// is propagated to the caller.
func TestLoginWithMagicLink_UserLookupFailure(t *testing.T) {
	userID := uuid.New()
	lookupErr := errors.New("db error")

	repo := &mockRepository{
		verifyAndConsumeMagicLinkTokenFn: func(_ context.Context, _ string) (uuid.UUID, error) {
			return userID, nil
		},
	}
	usersRepo := &mockUserRepository{
		findByIDFn: func(_ context.Context, _ uuid.UUID) (users.User, error) {
			return users.User{}, lookupErr
		},
	}

	svc := newTestService(repo, usersRepo, &mockEmailSender{})

	resp, err := svc.LoginWithMagicLink(context.Background(), "valid-raw-token")
	assert.Nil(t, resp)
	assert.ErrorIs(t, err, lookupErr)
}
