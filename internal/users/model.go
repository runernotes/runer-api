package users

import (
	"time"

	"github.com/google/uuid"
)

type Plan string

const (
	PlanFree Plan = "free"
	PlanPro  Plan = "pro"
)


type User struct {
	ID          uuid.UUID  `gorm:"primaryKey;type:uuid;default:gen_random_uuid()"`
	Email       string     `gorm:"not null;unique"`
	Name        string     `gorm:"not null"`
	Plan        Plan       `gorm:"type:varchar(20);not null;default:'free'"`
	ActivatedAt *time.Time `gorm:"column:activated_at"`
	CreatedAt   time.Time  `gorm:"autoCreateTime"`
	UpdatedAt   time.Time  `gorm:"autoUpdateTime"`
}
