package users

import (
	"context"

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
