package internal

import (
	"context"
	"net/http"

	"github.com/google/uuid"
	"github.com/labstack/echo/v5"
	"github.com/runernotes/runer-api/internal/analytics"
	"github.com/runernotes/runer-api/internal/auth"
	"github.com/runernotes/runer-api/internal/config"
	"github.com/runernotes/runer-api/internal/email"
	internalmw "github.com/runernotes/runer-api/internal/middleware"
	"github.com/runernotes/runer-api/internal/notes"
	"github.com/runernotes/runer-api/internal/subscription"
	"github.com/runernotes/runer-api/internal/users"
	"github.com/runernotes/runer-api/internal/utils"
	"github.com/runernotes/runer-api/internal/webhook"
	"gorm.io/gorm"
)

// RouteOptions allows optional overrides when registering routes (e.g. for testing).
type RouteOptions struct {
	EmailSender email.Sender
	// StripeClient, when non-nil, overrides the real Stripe SDK client used by
	// the subscription handler. Tests inject an in-memory fake; production
	// callers leave this nil so NewStripeClient is used.
	StripeClient subscription.StripeClient
	// StripeEventVerifier, when non-nil, overrides the real Stripe webhook
	// signature verifier. Tests inject a fake verifier that accepts any
	// signature; production leaves this nil.
	StripeEventVerifier webhook.EventVerifier
	// Tracker is the analytics event tracker injected into handlers. When nil,
	// a NoopTracker is used so tests that don't care about analytics still work
	// without any extra setup.
	Tracker analytics.Tracker
}

// notesUsersRepoAdapter bridges users.UsersRepository to the notes.usersRepository
// interface without creating a circular import. The notes package expects
// FindByID to return notes.UserPlan.
type notesUsersRepoAdapter struct {
	inner *users.UsersRepository
}

func (a *notesUsersRepoAdapter) FindByID(ctx context.Context, id uuid.UUID) (notes.UserPlan, error) {
	u, err := a.inner.FindByID(ctx, id)
	if err != nil {
		return notes.UserPlan{}, err
	}
	return notes.UserPlan{Plan: string(u.Plan)}, nil
}

// subscriptionUsersRepoAdapter bridges users.UsersRepository to the
// subscription.usersRepository interface which returns subscription.UserRecord.
type subscriptionUsersRepoAdapter struct {
	inner *users.UsersRepository
}

func (a *subscriptionUsersRepoAdapter) FindByID(ctx context.Context, id uuid.UUID) (subscription.UserRecord, error) {
	u, err := a.inner.FindByID(ctx, id)
	if err != nil {
		return subscription.UserRecord{}, err
	}
	return subscription.UserRecord{
		Plan:             string(u.Plan),
		Email:            u.Email,
		StripeCustomerID: u.StripeCustomerID,
	}, nil
}

func (a *subscriptionUsersRepoAdapter) UpdateStripeCustomerID(ctx context.Context, id uuid.UUID, stripeCustomerID string) error {
	return a.inner.UpdateStripeCustomerID(ctx, id, stripeCustomerID)
}

// webhookUsersRepoAdapter bridges users.UsersRepository to the webhook
// package's narrow UsersRepository interface.
type webhookUsersRepoAdapter struct {
	inner *users.UsersRepository
}

func (a *webhookUsersRepoAdapter) FindByStripeCustomerID(ctx context.Context, stripeCustomerID string) (webhook.UserRecord, error) {
	u, err := a.inner.FindByStripeCustomerID(ctx, stripeCustomerID)
	if err != nil {
		return webhook.UserRecord{}, err
	}
	return webhook.UserRecord{ID: u.ID}, nil
}

func (a *webhookUsersRepoAdapter) UpdatePlan(ctx context.Context, id uuid.UUID, plan string) error {
	return a.inner.UpdatePlan(ctx, id, users.Plan(plan))
}

func (a *webhookUsersRepoAdapter) UpdateStripeSubscriptionID(ctx context.Context, id uuid.UUID, subID *string) error {
	return a.inner.UpdateStripeSubscriptionID(ctx, id, subID)
}

// RegisterRoutes wires all application dependencies and registers HTTP routes.
func RegisterRoutes(e *echo.Echo, db *gorm.DB, cfg *config.Config, opts ...RouteOptions) {
	jwtManager := utils.NewJWTManager(cfg.JWTSecret, cfg.JWTTokenDuration, cfg.JWTRefreshTokenDuration)
	authMW := internalmw.AuthMiddleware(jwtManager)

	var opt RouteOptions
	if len(opts) > 0 {
		opt = opts[0]
	}

	// Resolve the analytics tracker. When nil is provided (e.g. in tests that
	// don't care about analytics) fall back to a no-op so no special setup is
	// needed in every test that calls RegisterRoutes.
	tracker := opt.Tracker
	if tracker == nil {
		tracker = analytics.NoopTracker{}
	}

	usersRepository := users.NewUsersRepository(db)
	usersService := users.NewUsersService(usersRepository)
	usersHandler := users.NewUsersHandler(usersService, tracker)

	sender := opt.EmailSender
	if sender == nil {
		sender = email.NewResendSender(cfg.ResendAPIKey, cfg.EmailFrom)
	}
	emailService := email.NewEmailService(sender)

	authRepository := auth.NewAuthRepository(db)
	authService := auth.NewAuthService(authRepository, usersRepository, emailService, jwtManager, cfg.MagicLinkTokenDuration, cfg.JWTRefreshTokenDuration)
	authHandler := auth.NewAuthHandler(authService, tracker)

	notesRepository := notes.NewNotesRepository(db)
	notesUsersRepo := &notesUsersRepoAdapter{inner: usersRepository}
	notesService := notes.NewNotesService(notesRepository, notesUsersRepo, cfg.FreeNoteLimit)
	notesHandler := notes.NewNotesHandler(notesService, tracker)

	subscriptionUsersRepo := &subscriptionUsersRepoAdapter{inner: usersRepository}

	// Stripe client: use the injected fake if tests provided one, otherwise
	// build the real SDK client only when billing is enabled. Leaving the
	// client nil in self-hosted mode ensures CreateCheckout returns 501 even
	// if somebody accidentally forgets to also disable the route.
	stripeClient := opt.StripeClient
	if stripeClient == nil && cfg.BillingEnabled {
		stripeClient = subscription.NewStripeClient(cfg.StripeSecretKey)
	}

	subscriptionHandler := subscription.NewHandler(
		subscriptionUsersRepo,
		notesRepository,
		cfg.FreeNoteLimit,
		subscription.BillingConfig{
			Enabled:    cfg.BillingEnabled,
			PriceID:    cfg.StripePriceID,
			SuccessURL: cfg.StripeSuccessURL,
			CancelURL:  cfg.StripeCancelURL,
		},
		stripeClient,
		tracker,
	)

	// Webhook verifier: inject-for-test, else real stripe-go verifier when billing is on.
	stripeVerifier := opt.StripeEventVerifier
	if stripeVerifier == nil && cfg.BillingEnabled && cfg.StripeWebhookSecret != "" {
		stripeVerifier = webhook.NewSDKVerifier(cfg.StripeWebhookSecret)
	}
	webhookHandler := webhook.NewHandler(
		cfg.BillingEnabled,
		stripeVerifier,
		&webhookUsersRepoAdapter{inner: usersRepository},
	)

	e.GET("/health", func(c *echo.Context) error {
		return (*c).JSON(http.StatusOK, map[string]string{"status": "ok"})
	})

	v1 := e.Group("/api/v1")

	// Public auth routes
	v1.POST("/auth/register", authHandler.Register)
	v1.POST("/auth/magic-link", authHandler.RequestMagicLink)
	v1.POST("/auth/verify", authHandler.Verify)
	v1.GET("/auth/verify-redirect", authHandler.VerifyRedirect)
	v1.POST("/auth/refresh", authHandler.Refresh)

	// Protected auth routes
	v1.POST("/auth/logout", authHandler.Logout, authMW)

	// Protected notes routes
	notesGroup := v1.Group("/notes", authMW)
	notesGroup.GET("", notesHandler.GetAll)
	notesGroup.GET("/:note_id", notesHandler.GetByID)
	notesGroup.PUT("/:note_id", notesHandler.Upsert)
	notesGroup.DELETE("/:note_id", notesHandler.Trash)
	notesGroup.POST("/:note_id/restore", notesHandler.Restore)
	notesGroup.DELETE("/:note_id/purge", notesHandler.Purge)

	// Protected subscription routes
	v1.GET("/subscription", subscriptionHandler.GetSubscription, authMW)
	v1.POST("/subscription/checkout", subscriptionHandler.CreateCheckout, authMW)

	// Unauthenticated webhook — must not live under authMW.
	v1.POST("/webhooks/stripe", webhookHandler.HandleStripe)

	// Protected users routes
	v1.GET("/users/me", usersHandler.GetMe, authMW)
	v1.POST("/users/me/activate", usersHandler.Activate, authMW)
}
