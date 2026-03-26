package users

import (
	"context"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

type UsersRepository struct {
	db *gorm.DB
}

func NewUsersRepository(db *gorm.DB) *UsersRepository {
	return &UsersRepository{db: db}
}

func (r *UsersRepository) FindByID(ctx context.Context, id uuid.UUID) (User, error) {
	var user User
	if err := r.db.WithContext(ctx).Where("id = ?", id).First(&user).Error; err != nil {
		return User{}, err
	}
	return user, nil
}

func (r *UsersRepository) FindByEmail(ctx context.Context, email string) (User, error) {
	var user User
	if err := r.db.WithContext(ctx).Where("email = ?", email).First(&user).Error; err != nil {
		return User{}, err
	}
	return user, nil
}

func (r *UsersRepository) Create(ctx context.Context, user User) (User, error) {
	if err := r.db.WithContext(ctx).Create(&user).Error; err != nil {
		return User{}, err
	}
	return user, nil
}

func (r *UsersRepository) Update(ctx context.Context, user User) (User, error) {
	if err := r.db.WithContext(ctx).Save(&user).Error; err != nil {
		return User{}, err
	}
	return user, nil
}

func (r *UsersRepository) Delete(ctx context.Context, id uuid.UUID) error {
	if err := r.db.WithContext(ctx).Where("id = ?", id).Delete(&User{}).Error; err != nil {
		return err
	}
	return nil
}

// Activate sets activated_at to the current time only when it is currently NULL,
// making the operation idempotent. It then re-fetches and returns the updated user.
func (r *UsersRepository) Activate(ctx context.Context, id uuid.UUID) (User, error) {
	now := time.Now().UTC()
	if err := r.db.WithContext(ctx).Model(&User{}).
		Where("id = ? AND activated_at IS NULL", id).
		Updates(map[string]any{"activated_at": now}).Error; err != nil {
		return User{}, err
	}
	return r.FindByID(ctx, id)
}
