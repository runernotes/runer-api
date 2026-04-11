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

// ---- Register tests ----

// TestRegister_NewUser_CreatesUserAndSendsMagicLink verifies the happy path:
// when the email is not in the system the user is created and a magic link is sent.
func TestRegister_NewUser_CreatesUserAndSendsMagicLink(t *testing.T) {
	userID := uuid.New()

	usersRepo := &mockUserRepository{
		findByEmailFn: func(_ context.Context, _ string) (users.User, error) {
			// User not found — triggers new registration.
			return users.User{}, errors.New("not found")
		},
		createFn: func(_ context.Context, u users.User) (users.User, error) {
			u.ID = userID
			return u, nil
		},
	}

	var emailSent bool
	emailMock := &mockEmailSender{
		sendFn: func(_ context.Context, _ string, _ string, isNewUser bool) error {
			emailSent = true
			assert.True(t, isNewUser, "registration must flag the user as new")
			return nil
		},
	}

	repo := &mockRepository{
		createMagicLinkTokenFn: func(_ context.Context, _ MagicLinkToken) error { return nil },
	}

	svc := newTestService(repo, usersRepo, emailMock)
	err := svc.Register(context.Background(), "new@example.com", "New User")
	require.NoError(t, err)
	assert.True(t, emailSent, "magic link email must be sent on successful registration")
}

// TestRegister_ExistingUser_ReturnsErrUserAlreadyExists verifies that attempting
// to register an email that is already in the database returns ErrUserAlreadyExists.
func TestRegister_ExistingUser_ReturnsErrUserAlreadyExists(t *testing.T) {
	existingUser := users.User{ID: uuid.New(), Email: "existing@example.com"}

	usersRepo := &mockUserRepository{
		findByEmailFn: func(_ context.Context, _ string) (users.User, error) {
			return existingUser, nil // already registered
		},
	}

	svc := newTestService(&mockRepository{}, usersRepo, &mockEmailSender{})
	err := svc.Register(context.Background(), "existing@example.com", "Existing")
	assert.ErrorIs(t, err, ErrUserAlreadyExists)
}

// TestRegister_CreateUserFailure_PropagatesError verifies that a database error
// during user creation is returned to the caller.
func TestRegister_CreateUserFailure_PropagatesError(t *testing.T) {
	dbErr := errors.New("db create failed")

	usersRepo := &mockUserRepository{
		findByEmailFn: func(_ context.Context, _ string) (users.User, error) {
			return users.User{}, errors.New("not found")
		},
		createFn: func(_ context.Context, _ users.User) (users.User, error) {
			return users.User{}, dbErr
		},
	}

	svc := newTestService(&mockRepository{}, usersRepo, &mockEmailSender{})
	err := svc.Register(context.Background(), "new@example.com", "New User")
	assert.ErrorIs(t, err, dbErr)
}

// TestRegister_EmailSendFailure_PropagatesError verifies that a failure in the
// email sender is propagated to the caller after the user is created.
func TestRegister_EmailSendFailure_PropagatesError(t *testing.T) {
	sendErr := errors.New("smtp unavailable")
	userID := uuid.New()

	usersRepo := &mockUserRepository{
		findByEmailFn: func(_ context.Context, _ string) (users.User, error) {
			return users.User{}, errors.New("not found")
		},
		createFn: func(_ context.Context, u users.User) (users.User, error) {
			u.ID = userID
			return u, nil
		},
	}
	emailMock := &mockEmailSender{
		sendFn: func(_ context.Context, _ string, _ string, _ bool) error {
			return sendErr
		},
	}
	repo := &mockRepository{
		createMagicLinkTokenFn: func(_ context.Context, _ MagicLinkToken) error { return nil },
	}

	svc := newTestService(repo, usersRepo, emailMock)
	err := svc.Register(context.Background(), "new@example.com", "New User")
	assert.ErrorIs(t, err, sendErr)
}

// TestRegister_CreateMagicLinkToken_Failure_PropagatesError verifies that a
// failure persisting the magic link token is propagated to the caller.
func TestRegister_CreateMagicLinkToken_Failure_PropagatesError(t *testing.T) {
	tokenErr := errors.New("token insert failed")
	userID := uuid.New()

	usersRepo := &mockUserRepository{
		findByEmailFn: func(_ context.Context, _ string) (users.User, error) {
			return users.User{}, errors.New("not found")
		},
		createFn: func(_ context.Context, u users.User) (users.User, error) {
			u.ID = userID
			return u, nil
		},
	}
	repo := &mockRepository{
		createMagicLinkTokenFn: func(_ context.Context, _ MagicLinkToken) error {
			return tokenErr
		},
	}

	svc := newTestService(repo, usersRepo, &mockEmailSender{})
	err := svc.Register(context.Background(), "new@example.com", "New User")
	assert.ErrorIs(t, err, tokenErr)
}

// ---- CreateMagicLink tests ----

// TestCreateMagicLink_KnownUser_SendsMagicLink verifies that a known user
// triggers magic link creation and email delivery.
func TestCreateMagicLink_KnownUser_SendsMagicLink(t *testing.T) {
	userID := uuid.New()
	now := time.Now()
	user := users.User{ID: userID, Email: "known@example.com", ActivatedAt: &now}

	usersRepo := &mockUserRepository{
		findByEmailFn: func(_ context.Context, _ string) (users.User, error) {
			return user, nil
		},
	}

	var emailSent bool
	emailMock := &mockEmailSender{
		sendFn: func(_ context.Context, _ string, _ string, _ bool) error {
			emailSent = true
			return nil
		},
	}
	repo := &mockRepository{
		createMagicLinkTokenFn: func(_ context.Context, _ MagicLinkToken) error { return nil },
	}

	svc := newTestService(repo, usersRepo, emailMock)
	err := svc.CreateMagicLink(context.Background(), "known@example.com")
	require.NoError(t, err)
	assert.True(t, emailSent)
}

// TestCreateMagicLink_UnknownUser_PropagatesError verifies that when FindByEmail
// returns an error the service propagates it without sending an email.
func TestCreateMagicLink_UnknownUser_PropagatesError(t *testing.T) {
	lookupErr := errors.New("not found")

	usersRepo := &mockUserRepository{
		findByEmailFn: func(_ context.Context, _ string) (users.User, error) {
			return users.User{}, lookupErr
		},
	}

	emailMock := &mockEmailSender{
		sendFn: func(_ context.Context, _ string, _ string, _ bool) error {
			require.Fail(t, "email must not be sent for unknown user")
			return nil
		},
	}

	svc := newTestService(&mockRepository{}, usersRepo, emailMock)
	err := svc.CreateMagicLink(context.Background(), "nobody@example.com")
	assert.ErrorIs(t, err, lookupErr)
}

// TestCreateMagicLink_NotActivated_IsNewUserTrue verifies that a user whose
// ActivatedAt is nil causes the email sender to receive isNewUser=true.
func TestCreateMagicLink_NotActivated_IsNewUserTrue(t *testing.T) {
	userID := uuid.New()
	user := users.User{ID: userID, Email: "fresh@example.com", ActivatedAt: nil}

	usersRepo := &mockUserRepository{
		findByEmailFn: func(_ context.Context, _ string) (users.User, error) {
			return user, nil
		},
	}

	var capturedIsNewUser bool
	emailMock := &mockEmailSender{
		sendFn: func(_ context.Context, _ string, _ string, isNewUser bool) error {
			capturedIsNewUser = isNewUser
			return nil
		},
	}
	repo := &mockRepository{
		createMagicLinkTokenFn: func(_ context.Context, _ MagicLinkToken) error { return nil },
	}

	svc := newTestService(repo, usersRepo, emailMock)
	require.NoError(t, svc.CreateMagicLink(context.Background(), "fresh@example.com"))
	assert.True(t, capturedIsNewUser, "non-activated user must receive the new-user email")
}

// TestCreateMagicLink_Activated_IsNewUserFalse verifies that a user with a
// non-nil ActivatedAt causes isNewUser=false to be sent to the email sender.
func TestCreateMagicLink_Activated_IsNewUserFalse(t *testing.T) {
	userID := uuid.New()
	activatedAt := time.Now().Add(-24 * time.Hour)
	user := users.User{ID: userID, Email: "active@example.com", ActivatedAt: &activatedAt}

	usersRepo := &mockUserRepository{
		findByEmailFn: func(_ context.Context, _ string) (users.User, error) {
			return user, nil
		},
	}

	var capturedIsNewUser bool
	emailMock := &mockEmailSender{
		sendFn: func(_ context.Context, _ string, _ string, isNewUser bool) error {
			capturedIsNewUser = isNewUser
			return nil
		},
	}
	repo := &mockRepository{
		createMagicLinkTokenFn: func(_ context.Context, _ MagicLinkToken) error { return nil },
	}

	svc := newTestService(repo, usersRepo, emailMock)
	require.NoError(t, svc.CreateMagicLink(context.Background(), "active@example.com"))
	assert.False(t, capturedIsNewUser, "activated user must receive the returning-user email")
}

// ---- RefreshAccessToken tests ----

// TestRefreshAccessToken_Valid_ReturnsNewAccessToken verifies the happy path:
// a valid, non-revoked, non-expired refresh token produces a new access token.
func TestRefreshAccessToken_Valid_ReturnsNewAccessToken(t *testing.T) {
	userID := uuid.New()
	tokenID := uuid.New()
	rawToken := "valid-raw-refresh-token"

	repo := &mockRepository{
		getRefreshTokenByHashFn: func(_ context.Context, _ string) (*RefreshToken, error) {
			return &RefreshToken{
				TokenID:   tokenID,
				UserID:    userID,
				ExpiresAt: time.Now().Add(24 * time.Hour),
				RevokedAt: nil,
			}, nil
		},
	}
	usersRepo := &mockUserRepository{
		findByIDFn: func(_ context.Context, id uuid.UUID) (users.User, error) {
			return users.User{ID: id, Email: "user@example.com"}, nil
		},
	}

	svc := newTestService(repo, usersRepo, &mockEmailSender{})
	resp, err := svc.RefreshAccessToken(context.Background(), rawToken)
	require.NoError(t, err)
	require.NotNil(t, resp)
	assert.NotEmpty(t, resp.AccessToken)
}

// TestRefreshAccessToken_TokenNotFound_ReturnsErrInvalidRefresh verifies that
// when the refresh token does not exist in the database the caller gets
// ErrInvalidRefreshToken.
func TestRefreshAccessToken_TokenNotFound_ReturnsErrInvalidRefresh(t *testing.T) {
	repo := &mockRepository{
		getRefreshTokenByHashFn: func(_ context.Context, _ string) (*RefreshToken, error) {
			return nil, errors.New("not found")
		},
	}

	svc := newTestService(repo, &mockUserRepository{}, &mockEmailSender{})
	_, err := svc.RefreshAccessToken(context.Background(), "unknown-token")
	assert.ErrorIs(t, err, ErrInvalidRefreshToken)
}

// TestRefreshAccessToken_Revoked_ReturnsErrRevokedRefresh verifies that a
// refresh token that has been revoked (RevokedAt != nil) is rejected.
func TestRefreshAccessToken_Revoked_ReturnsErrRevokedRefresh(t *testing.T) {
	revokedAt := time.Now().Add(-time.Hour)
	repo := &mockRepository{
		getRefreshTokenByHashFn: func(_ context.Context, _ string) (*RefreshToken, error) {
			return &RefreshToken{
				TokenID:   uuid.New(),
				UserID:    uuid.New(),
				ExpiresAt: time.Now().Add(24 * time.Hour),
				RevokedAt: &revokedAt,
			}, nil
		},
	}

	svc := newTestService(repo, &mockUserRepository{}, &mockEmailSender{})
	_, err := svc.RefreshAccessToken(context.Background(), "revoked-token")
	assert.ErrorIs(t, err, ErrRevokedRefreshToken)
}

// TestRefreshAccessToken_Expired_ReturnsErrExpiredRefresh verifies that a
// refresh token past its expiry date is rejected.
func TestRefreshAccessToken_Expired_ReturnsErrExpiredRefresh(t *testing.T) {
	repo := &mockRepository{
		getRefreshTokenByHashFn: func(_ context.Context, _ string) (*RefreshToken, error) {
			return &RefreshToken{
				TokenID:   uuid.New(),
				UserID:    uuid.New(),
				ExpiresAt: time.Now().Add(-time.Hour), // already expired
				RevokedAt: nil,
			}, nil
		},
	}

	svc := newTestService(repo, &mockUserRepository{}, &mockEmailSender{})
	_, err := svc.RefreshAccessToken(context.Background(), "expired-token")
	assert.ErrorIs(t, err, ErrExpiredRefreshToken)
}

// TestRefreshAccessToken_UserLookupFailure_PropagatesError verifies that when
// the user cannot be fetched after a valid token is found the error is propagated.
func TestRefreshAccessToken_UserLookupFailure_PropagatesError(t *testing.T) {
	lookupErr := errors.New("db error")

	repo := &mockRepository{
		getRefreshTokenByHashFn: func(_ context.Context, _ string) (*RefreshToken, error) {
			return &RefreshToken{
				TokenID:   uuid.New(),
				UserID:    uuid.New(),
				ExpiresAt: time.Now().Add(24 * time.Hour),
				RevokedAt: nil,
			}, nil
		},
	}
	usersRepo := &mockUserRepository{
		findByIDFn: func(_ context.Context, _ uuid.UUID) (users.User, error) {
			return users.User{}, lookupErr
		},
	}

	svc := newTestService(repo, usersRepo, &mockEmailSender{})
	_, err := svc.RefreshAccessToken(context.Background(), "valid-token")
	assert.ErrorIs(t, err, lookupErr)
}

// ---- Logout tests ----

// TestLogout_Valid_RevokesRefreshToken verifies that a valid refresh token is
// looked up and then revoked by calling RevokeRefreshToken.
func TestLogout_Valid_RevokesRefreshToken(t *testing.T) {
	tokenID := uuid.New()
	var revokedID uuid.UUID

	repo := &mockRepository{
		getRefreshTokenByHashFn: func(_ context.Context, _ string) (*RefreshToken, error) {
			return &RefreshToken{TokenID: tokenID, UserID: uuid.New()}, nil
		},
		revokeRefreshTokenFn: func(_ context.Context, id uuid.UUID) error {
			revokedID = id
			return nil
		},
	}

	svc := newTestService(repo, &mockUserRepository{}, &mockEmailSender{})
	err := svc.Logout(context.Background(), "some-raw-token")
	require.NoError(t, err)
	assert.Equal(t, tokenID, revokedID, "the token's own ID must be passed to RevokeRefreshToken")
}

// TestLogout_TokenNotFound_ReturnsErrInvalidRefresh verifies that when the
// refresh token cannot be found the caller gets ErrInvalidRefreshToken.
func TestLogout_TokenNotFound_ReturnsErrInvalidRefresh(t *testing.T) {
	repo := &mockRepository{
		getRefreshTokenByHashFn: func(_ context.Context, _ string) (*RefreshToken, error) {
			return nil, errors.New("not found")
		},
	}

	svc := newTestService(repo, &mockUserRepository{}, &mockEmailSender{})
	err := svc.Logout(context.Background(), "unknown-raw-token")
	assert.ErrorIs(t, err, ErrInvalidRefreshToken)
}
