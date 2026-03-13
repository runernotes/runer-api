package internal

import (
	"github.com/labstack/echo/v5"
	"github.com/runernotes/runer-api-core/internal/auth"
	"github.com/runernotes/runer-api-core/internal/email"
	internalmw "github.com/runernotes/runer-api-core/internal/middleware"
	"github.com/runernotes/runer-api-core/internal/notes"
	"github.com/runernotes/runer-api-core/internal/users"
	"github.com/runernotes/runer-api-core/internal/utils"
	"github.com/runernotes/runer-api-core/pkg/config"
	"gorm.io/gorm"
)

func RegisterRoutes(e *echo.Echo, db *gorm.DB, config *config.Config) {
	jwtManager := utils.NewJWTManager(config.JWTSecret, config.JWTTokenDuration, config.JWTRefreshTokenDuration)
	authMW := internalmw.AuthMiddleware(jwtManager)

	usersRepository := users.NewUsersRepository(db)
	emailSender := email.NewConsoleSender()
	emailService := email.NewEmailService(emailSender)

	authRepository := auth.NewAuthRepository(db)
	authService := auth.NewAuthService(authRepository, usersRepository, emailService, jwtManager, config.MagicLinkTokenDuration, config.JWTRefreshTokenDuration)
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
