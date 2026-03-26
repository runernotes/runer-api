package internal

import (
	"context"
	"net/http"

	"github.com/google/uuid"
	"github.com/labstack/echo/v5"
	"github.com/runernotes/runer-api/internal/auth"
	"github.com/runernotes/runer-api/internal/config"
	"github.com/runernotes/runer-api/internal/email"
	internalmw "github.com/runernotes/runer-api/internal/middleware"
	"github.com/runernotes/runer-api/internal/notes"
	"github.com/runernotes/runer-api/internal/subscription"
	"github.com/runernotes/runer-api/internal/users"
	"github.com/runernotes/runer-api/internal/utils"
	"gorm.io/gorm"
)

// RouteOptions allows optional overrides when registering routes (e.g. for testing).
type RouteOptions struct {
	EmailSender email.Sender
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
	return subscription.UserRecord{Plan: string(u.Plan)}, nil
}

// RegisterRoutes wires all application dependencies and registers HTTP routes.
func RegisterRoutes(e *echo.Echo, db *gorm.DB, cfg *config.Config, opts ...RouteOptions) {
	jwtManager := utils.NewJWTManager(cfg.JWTSecret, cfg.JWTTokenDuration, cfg.JWTRefreshTokenDuration)
	authMW := internalmw.AuthMiddleware(jwtManager)

	usersRepository := users.NewUsersRepository(db)
	usersService := users.NewUsersService(usersRepository)
	usersHandler := users.NewUsersHandler(usersService)

	var opt RouteOptions
	if len(opts) > 0 {
		opt = opts[0]
	}
	sender := opt.EmailSender
	if sender == nil {
		if cfg.ResendAPIKey != "" {
			sender = email.NewResendSender(cfg.ResendAPIKey, cfg.EmailFrom)
		} else {
			sender = email.NewConsoleSender()
		}
	}
	emailService := email.NewEmailService(sender)

	authRepository := auth.NewAuthRepository(db)
	authService := auth.NewAuthService(authRepository, usersRepository, emailService, jwtManager, cfg.MagicLinkTokenDuration, cfg.JWTRefreshTokenDuration)
	authHandler := auth.NewAuthHandler(authService)

	notesRepository := notes.NewNotesRepository(db)
	notesUsersRepo := &notesUsersRepoAdapter{inner: usersRepository}
	notesService := notes.NewNotesService(notesRepository, notesUsersRepo, cfg.FreeNoteLimit)
	notesHandler := notes.NewNotesHandler(notesService)

	subscriptionUsersRepo := &subscriptionUsersRepoAdapter{inner: usersRepository}
	subscriptionHandler := subscription.NewHandler(subscriptionUsersRepo, notesRepository, cfg.FreeNoteLimit)

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

	// Protected subscription route
	v1.GET("/subscription", subscriptionHandler.GetSubscription, authMW)

	// Protected users routes
	v1.GET("/users/me", usersHandler.GetMe, authMW)
	v1.POST("/users/me/activate", usersHandler.Activate, authMW)
}
