package auth

import (
	"context"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// AuthRepository provides database access for authentication-related entities.
type AuthRepository struct {
	db *gorm.DB
}

// NewAuthRepository constructs an AuthRepository backed by the given GORM database.
func NewAuthRepository(db *gorm.DB) *AuthRepository {
	return &AuthRepository{db: db}
}

// CreateMagicLinkToken persists a new magic-link token record.
func (r *AuthRepository) CreateMagicLinkToken(ctx context.Context, token MagicLinkToken) error {
	return r.db.WithContext(ctx).Create(&token).Error
}

// VerifyAndConsumeMagicLinkToken atomically validates and marks a magic-link token
// as used in a single UPDATE ... RETURNING statement, eliminating the race window
// that existed when verification and consumption were separate round-trips.
//
// It returns the associated user_id on success. If no row is updated — because the
// token does not exist, has already been used, or has expired — ErrInvalidToken is
// returned. Callers must not perform additional expiry or used_at checks; the
// database enforces those invariants atomically.
func (r *AuthRepository) VerifyAndConsumeMagicLinkToken(ctx context.Context, tokenHash string) (uuid.UUID, error) {
	type result struct {
		TokenID uuid.UUID
		UserID  uuid.UUID
	}
	var row result

	// The magic_link_tokens table stores the hash in the "token" column (GORM default
	// mapping for the Token field on MagicLinkToken). The UPDATE atomically checks
	// that the token is unused and not yet expired before setting used_at.
	tx := r.db.WithContext(ctx).Raw(`
		UPDATE magic_link_tokens
		SET used_at = NOW()
		WHERE token = ?
		  AND used_at IS NULL
		  AND expires_at > NOW()
		RETURNING token_id, user_id
	`, tokenHash).Scan(&row)

	if tx.Error != nil {
		return uuid.Nil, tx.Error
	}
	if tx.RowsAffected == 0 {
		return uuid.Nil, ErrInvalidToken
	}
	return row.UserID, nil
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
