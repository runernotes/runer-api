package auth

import (
	"time"

	"github.com/google/uuid"
)

type MagicLinkToken struct {
	TokenID   uuid.UUID  `gorm:"primaryKey;type:uuid;default:gen_random_uuid()"`
	UserID    uuid.UUID  `gorm:"not null;index"`
	Email     string     `gorm:"not null"`
	Token     string     `gorm:"not null;uniqueIndex"`
	ExpiresAt time.Time  `gorm:"not null"`
	UsedAt    *time.Time `gorm:"index"`
}

type RefreshToken struct {
	TokenID   uuid.UUID `gorm:"primaryKey;type:uuid;default:gen_random_uuid()"`
	UserID    uuid.UUID `gorm:"not null;index"`
	TokenHash string    `gorm:"column:token_hash;not null;uniqueIndex"`
	ExpiresAt time.Time `gorm:"not null"`
	CreatedAt time.Time `gorm:"autoCreateTime"`
	RevokedAt *time.Time
}
