package auth

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"time"

	"github.com/google/uuid"
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
	GetMagicLinkToken(ctx context.Context, token string) (MagicLinkToken, error)
	MarkMagicLinkTokenAsUsed(ctx context.Context, tokenId uuid.UUID) error
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
	SendMagicLinkEmail(ctx context.Context, email string, token string) error
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

	return s.createAndSendMagicLink(ctx, email, user.ID)
}

func (s *AuthService) CreateMagicLink(ctx context.Context, email string) error {
	user, err := s.usersRepository.FindByEmail(ctx, email)
	if err != nil {
		return err
	}
	if user.ID == uuid.Nil {
		return ErrUserNotFound
	}

	return s.createAndSendMagicLink(ctx, email, user.ID)
}

func (s *AuthService) createAndSendMagicLink(ctx context.Context, email string, userId uuid.UUID) error {
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
	return s.emailSender.SendMagicLinkEmail(ctx, email, rawToken)
}

func (s *AuthService) LoginWithMagicLink(ctx context.Context, token string) (*LoginResponse, error) {
	hashedToken := utils.ComputeSHA256(token)
	ml, err := s.repository.GetMagicLinkToken(ctx, hashedToken)
	if err != nil {
		return nil, ErrInvalidToken
	}
	if ml.ExpiresAt.Before(time.Now()) {
		return nil, ErrExpiredToken
	}
	if ml.UsedAt != nil {
		return nil, ErrTokenAlreadyUsed
	}
	if err = s.repository.MarkMagicLinkTokenAsUsed(ctx, ml.TokenID); err != nil {
		return nil, err
	}

	accessToken, err := s.jwtManager.GenerateAccessToken(ml.UserID, ml.Email)
	if err != nil {
		return nil, err
	}

	rawRefreshToken, refreshTokenHash, err := generateOpaqueToken()
	if err != nil {
		return nil, err
	}

	rt := RefreshToken{
		UserID:    ml.UserID,
		TokenHash: refreshTokenHash,
		ExpiresAt: time.Now().Add(s.refreshTokenDuration),
	}
	if err := s.repository.CreateRefreshToken(ctx, rt); err != nil {
		return nil, err
	}

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
