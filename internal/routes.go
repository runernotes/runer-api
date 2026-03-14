package internal

import (
	"github.com/labstack/echo/v5"
	"github.com/runernotes/runer-api/internal/auth"
	"github.com/runernotes/runer-api/internal/config"
	"github.com/runernotes/runer-api/internal/email"
	internalmw "github.com/runernotes/runer-api/internal/middleware"
	"github.com/runernotes/runer-api/internal/notes"
	"github.com/runernotes/runer-api/internal/users"
	"github.com/runernotes/runer-api/internal/utils"
	"gorm.io/gorm"
)

// RouteOptions allows optional overrides when registering routes (e.g. for testing).
type RouteOptions struct {
	EmailSender email.Sender
}

func RegisterRoutes(e *echo.Echo, db *gorm.DB, cfg *config.Config, opts ...RouteOptions) {
	jwtManager := utils.NewJWTManager(cfg.JWTSecret, cfg.JWTTokenDuration, cfg.JWTRefreshTokenDuration)
	authMW := internalmw.AuthMiddleware(jwtManager)

	usersRepository := users.NewUsersRepository(db)

	var opt RouteOptions
	if len(opts) > 0 {
		opt = opts[0]
	}
	sender := opt.EmailSender
	if sender == nil {
		sender = email.NewConsoleSender()
	}
	emailService := email.NewEmailService(sender)

	authRepository := auth.NewAuthRepository(db)
	authService := auth.NewAuthService(authRepository, usersRepository, emailService, jwtManager, cfg.MagicLinkTokenDuration, cfg.JWTRefreshTokenDuration)
	authHandler := auth.NewAuthHandler(authService)

	notesRepository := notes.NewNotesRepository(db)
	notesService := notes.NewNotesService(notesRepository)
	notesHandler := notes.NewNotesHandler(notesService)

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
	notesGroup.DELETE("/:note_id", notesHandler.Delete)
}
