package auth

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/rs/zerolog"
	"github.com/runernotes/runer-api/internal/users"
	"github.com/runernotes/runer-api/internal/utils"
)

type AuthService struct {
	repository             repository
	usersRepository        userRepository
	emailSender            emailSender
	jwtManager             *utils.JWTManager
	magicLinkTokenDuration time.Duration
	refreshTokenDuration   time.Duration
}

type repository interface {
	CreateMagicLinkToken(ctx context.Context, token MagicLinkToken) error
	// VerifyAndConsumeMagicLinkToken atomically marks the token as used and returns
	// the associated user_id. Returns ErrInvalidToken when zero rows are matched
	// (token not found, already used, or expired).
	VerifyAndConsumeMagicLinkToken(ctx context.Context, tokenHash string) (uuid.UUID, error)
	CreateRefreshToken(ctx context.Context, token RefreshToken) error
	GetRefreshTokenByHash(ctx context.Context, hash string) (*RefreshToken, error)
	RevokeRefreshToken(ctx context.Context, tokenID uuid.UUID) error
	RevokeAllUserRefreshTokens(ctx context.Context, userID uuid.UUID) error
}

type userRepository interface {
	Create(ctx context.Context, user users.User) (users.User, error)
	FindByEmail(ctx context.Context, email string) (users.User, error)
	FindByID(ctx context.Context, id uuid.UUID) (users.User, error)
}

type emailSender interface {
	SendMagicLinkEmail(ctx context.Context, email string, token string, isNewUser bool) error
}

func NewAuthService(repository repository, users userRepository, emailSender emailSender,
	jwtManager *utils.JWTManager, magicLinkTokenDuration time.Duration, refreshTokenDuration time.Duration) *AuthService {
	return &AuthService{repository: repository, usersRepository: users, emailSender: emailSender, jwtManager: jwtManager, magicLinkTokenDuration: magicLinkTokenDuration, refreshTokenDuration: refreshTokenDuration}
}

func (s *AuthService) Register(ctx context.Context, email string, name string) error {
	_, err := s.usersRepository.FindByEmail(ctx, email)
	if err == nil {
		// Found a user — already exists
		return ErrUserAlreadyExists
	}

	user := users.User{
		Email: email,
		Name:  name,
	}
	user, err = s.usersRepository.Create(ctx, user)
	if err != nil {
		return err
	}

	zerolog.Ctx(ctx).Info().
		Str("event", "user_registered").
		Str("user_id", user.ID.String()).
		Msg("new user registered")

	return s.createAndSendMagicLink(ctx, email, user.ID, true)
}

func (s *AuthService) CreateMagicLink(ctx context.Context, email string) error {
	user, err := s.usersRepository.FindByEmail(ctx, email)
	if err != nil {
		return err
	}
	if user.ID == uuid.Nil {
		return ErrUserNotFound
	}

	// A user is considered "new" until they have activated their account.
	// Sending the new-user email variant until first activation ensures the
	// welcome flow is shown even if the initial registration email was missed.
	isNewUser := user.ActivatedAt == nil
	return s.createAndSendMagicLink(ctx, email, user.ID, isNewUser)
}

func (s *AuthService) createAndSendMagicLink(ctx context.Context, email string, userId uuid.UUID, isNewUser bool) error {
	rawToken, tokenHash, err := generateOpaqueToken()
	if err != nil {
		return err
	}
	token := MagicLinkToken{
		Token:     tokenHash,
		UserID:    userId,
		Email:     email,
		ExpiresAt: time.Now().Add(s.magicLinkTokenDuration),
	}
	if err = s.repository.CreateMagicLinkToken(ctx, token); err != nil {
		return err
	}
	return s.emailSender.SendMagicLinkEmail(ctx, email, rawToken, isNewUser)
}

// LoginWithMagicLink verifies a raw magic-link token and, on success, issues a new
// access token and refresh token pair. The verification is performed atomically
// in the database to prevent replay attacks from concurrent requests.
func (s *AuthService) LoginWithMagicLink(ctx context.Context, token string) (*LoginResponse, error) {
	hashedToken := utils.ComputeSHA256(token)

	// VerifyAndConsumeMagicLinkToken atomically validates the token (checking
	// used_at IS NULL and expires_at > NOW()) and marks it as used in one
	// UPDATE...RETURNING statement. This eliminates the read-check-write race
	// window that existed when three separate DB round-trips were used.
	userID, err := s.repository.VerifyAndConsumeMagicLinkToken(ctx, hashedToken)
	if err != nil {
		// Only log the security event for an intentionally invalid token.
		// Infrastructure errors (DB timeout, connection failure) are a
		// different failure category and must not be mischaracterised as
		// a token attack in the audit log.
		if errors.Is(err, ErrInvalidToken) {
			zerolog.Ctx(ctx).Warn().
				Str("event", "magic_link_verify_failed").
				Msg("invalid, expired, or replayed magic link token")
		}
		return nil, err
	}

	// Fetch the user to obtain the email address needed for JWT generation.
	// The magic_link_tokens table no longer needs to be the source of email truth.
	user, err := s.usersRepository.FindByID(ctx, userID)
	if err != nil {
		return nil, err
	}

	accessToken, err := s.jwtManager.GenerateAccessToken(userID, user.Email)
	if err != nil {
		return nil, err
	}

	rawRefreshToken, refreshTokenHash, err := generateOpaqueToken()
	if err != nil {
		return nil, err
	}

	rt := RefreshToken{
		UserID:    userID,
		TokenHash: refreshTokenHash,
		ExpiresAt: time.Now().Add(s.refreshTokenDuration),
	}
	if err := s.repository.CreateRefreshToken(ctx, rt); err != nil {
		return nil, err
	}

	zerolog.Ctx(ctx).Info().
		Str("event", "user_login").
		Str("user_id", userID.String()).
		Msg("magic link verified, session created")

	return &LoginResponse{
		AccessToken:  accessToken,
		RefreshToken: rawRefreshToken,
	}, nil
}

func (s *AuthService) RefreshAccessToken(ctx context.Context, rawRefreshToken string) (*LoginResponse, error) {
	tokenHash := utils.ComputeSHA256(rawRefreshToken)
	rt, err := s.repository.GetRefreshTokenByHash(ctx, tokenHash)
	if err != nil {
		return nil, ErrInvalidRefreshToken
	}
	if rt.RevokedAt != nil {
		return nil, ErrRevokedRefreshToken
	}
	if rt.ExpiresAt.Before(time.Now()) {
		return nil, ErrExpiredRefreshToken
	}

	user, err := s.usersRepository.FindByID(ctx, rt.UserID)
	if err != nil {
		return nil, err
	}

	accessToken, err := s.jwtManager.GenerateAccessToken(rt.UserID, user.Email)
	if err != nil {
		return nil, err
	}

	return &LoginResponse{
		AccessToken:  accessToken,
		RefreshToken: rawRefreshToken,
	}, nil
}

func (s *AuthService) Logout(ctx context.Context, rawRefreshToken string) error {
	tokenHash := utils.ComputeSHA256(rawRefreshToken)
	rt, err := s.repository.GetRefreshTokenByHash(ctx, tokenHash)
	if err != nil {
		return ErrInvalidRefreshToken
	}
	return s.repository.RevokeRefreshToken(ctx, rt.TokenID)
}

func generateOpaqueToken() (raw string, hash string, err error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", "", err
	}
	raw = base64.RawURLEncoding.EncodeToString(b)
	return raw, utils.ComputeSHA256(raw), nil
}
