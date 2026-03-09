package auth

import (
	"context"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

type AuthRepository struct {
	db *gorm.DB
}

func NewAuthRepository(db *gorm.DB) *AuthRepository {
	return &AuthRepository{db: db}
}

func (r *AuthRepository) CreateMagicLinkToken(ctx context.Context, token MagicLinkToken) error {
	return r.db.WithContext(ctx).Create(&token).Error
}

func (r *AuthRepository) GetMagicLinkToken(ctx context.Context, token string) (MagicLinkToken, error) {
	var ml MagicLinkToken
	if err := r.db.WithContext(ctx).Where("token = ?", token).First(&ml).Error; err != nil {
		return MagicLinkToken{}, err
	}
	return ml, nil
}

func (r *AuthRepository) MarkMagicLinkTokenAsUsed(ctx context.Context, tokenId uuid.UUID) error {
	result := r.db.WithContext(ctx).Model(&MagicLinkToken{}).Where("token_id = ? AND used_at IS NULL", tokenId).Update("used_at", time.Now())
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return ErrTokenAlreadyUsed
	}
	return nil
}

func (r *AuthRepository) CreateRefreshToken(ctx context.Context, token RefreshToken) error {
	return r.db.WithContext(ctx).Create(&token).Error
}

func (r *AuthRepository) GetRefreshTokenByHash(ctx context.Context, hash string) (*RefreshToken, error) {
	var rt RefreshToken
	if err := r.db.WithContext(ctx).Where("token_hash = ?", hash).First(&rt).Error; err != nil {
		return nil, err
	}
	return &rt, nil
}

func (r *AuthRepository) RevokeRefreshToken(ctx context.Context, tokenID uuid.UUID) error {
	now := time.Now()
	return r.db.WithContext(ctx).Model(&RefreshToken{}).Where("token_id = ?", tokenID).Update("revoked_at", &now).Error
}

func (r *AuthRepository) RevokeAllUserRefreshTokens(ctx context.Context, userID uuid.UUID) error {
	now := time.Now()
	return r.db.WithContext(ctx).Model(&RefreshToken{}).Where("user_id = ? AND revoked_at IS NULL", userID).Update("revoked_at", &now).Error
}
