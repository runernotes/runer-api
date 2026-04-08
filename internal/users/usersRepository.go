package users

import (
	"context"
	"errors"
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

// FindByStripeCustomerID looks up a user by their Stripe customer id.
// Used by the Stripe webhook handler to locate the user for an incoming event.
func (r *UsersRepository) FindByStripeCustomerID(ctx context.Context, stripeCustomerID string) (User, error) {
	if stripeCustomerID == "" {
		return User{}, gorm.ErrRecordNotFound
	}
	var user User
	if err := r.db.WithContext(ctx).
		Where("stripe_customer_id = ?", stripeCustomerID).
		First(&user).Error; err != nil {
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

// UpdateStripeCustomerID persists the Stripe customer id for a user. It is called
// the first time a user initiates Stripe checkout.
func (r *UsersRepository) UpdateStripeCustomerID(ctx context.Context, id uuid.UUID, stripeCustomerID string) error {
	if stripeCustomerID == "" {
		return errors.New("stripe customer id must be non-empty")
	}
	return r.db.WithContext(ctx).Model(&User{}).
		Where("id = ?", id).
		Update("stripe_customer_id", stripeCustomerID).Error
}

// UpdatePlan sets the user's plan. Called by the Stripe webhook handler on
// checkout.session.completed (→ pro) and customer.subscription.deleted (→ free).
func (r *UsersRepository) UpdatePlan(ctx context.Context, id uuid.UUID, plan Plan) error {
	return r.db.WithContext(ctx).Model(&User{}).
		Where("id = ?", id).
		Update("plan", plan).Error
}

// UpdateStripeSubscriptionID stores or clears the Stripe subscription id for a user.
// Pass nil to clear (e.g. on subscription cancellation).
func (r *UsersRepository) UpdateStripeSubscriptionID(ctx context.Context, id uuid.UUID, subID *string) error {
	return r.db.WithContext(ctx).Model(&User{}).
		Where("id = ?", id).
		Update("stripe_subscription_id", subID).Error
}
