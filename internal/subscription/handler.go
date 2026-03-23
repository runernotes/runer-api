package subscription

import (
	"context"
	"net/http"

	"github.com/google/uuid"
	"github.com/labstack/echo/v5"
	"github.com/runernotes/runer-api/internal/api"
	internalmw "github.com/runernotes/runer-api/internal/middleware"
)

// usersRepository is the minimal interface needed to fetch a user's plan.
// Defined locally to avoid importing the users package directly.
type usersRepository interface {
	FindByID(ctx context.Context, id uuid.UUID) (UserRecord, error)
}

// UserRecord holds the user fields needed by the subscription handler.
// It is exported so that adapters in other packages can satisfy the usersRepository
// interface without importing the users package into subscription.
type UserRecord struct {
	Plan string
}

// notesRepository is the minimal interface needed to count a user's live notes.
// Defined locally to keep the subscription package self-contained.
type notesRepository interface {
	CountLiveNotes(ctx context.Context, userID uuid.UUID) (int64, error)
}

// Handler handles subscription-related HTTP requests.
type Handler struct {
	usersRepo     usersRepository
	notesRepo     notesRepository
	freeNoteLimit int
}

// NewHandler constructs a subscription Handler.
func NewHandler(usersRepo usersRepository, notesRepo notesRepository, freeNoteLimit int) *Handler {
	return &Handler{
		usersRepo:     usersRepo,
		notesRepo:     notesRepo,
		freeNoteLimit: freeNoteLimit,
	}
}

// GetSubscription handles GET /api/v1/subscription.
// It returns the authenticated user's plan, live note count, and note limit.
// For pro users note_limit is null (unlimited).
func (h *Handler) GetSubscription(c *echo.Context) error {
	val := c.Get(internalmw.UserContextKey)
	if val == nil {
		return c.JSON(http.StatusUnauthorized, api.ErrorResponse{Error: "unauthorized", Code: "UNAUTHORIZED"})
	}
	userID, ok := val.(uuid.UUID)
	if !ok {
		return c.JSON(http.StatusUnauthorized, api.ErrorResponse{Error: "unauthorized", Code: "UNAUTHORIZED"})
	}

	user, err := h.usersRepo.FindByID(c.Request().Context(), userID)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, api.ErrorResponse{Error: "failed to fetch subscription", Code: "INTERNAL_ERROR"})
	}

	count, err := h.notesRepo.CountLiveNotes(c.Request().Context(), userID)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, api.ErrorResponse{Error: "failed to fetch subscription", Code: "INTERNAL_ERROR"})
	}

	resp := SubscriptionResponse{
		Plan:      user.Plan,
		NoteCount: count,
		NoteLimit: nil, // pro plan: unlimited
	}

	if user.Plan == "free" {
		limit := h.freeNoteLimit
		resp.NoteLimit = &limit
	}

	return c.JSON(http.StatusOK, resp)
}

