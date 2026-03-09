package notes

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/google/uuid"
	"github.com/labstack/echo/v5"
	"github.com/rs/zerolog/log"
	"github.com/runernotes/runer-api/internal/api"
	internalmw "github.com/runernotes/runer-api/internal/middleware"
)

type NotesHandler struct {
	service service
}

const defaultPageSize = 100
const maxPageSize = 500

type service interface {
	GetNotesSince(ctx context.Context, userID uuid.UUID, since *time.Time, cursor *NoteCursor, limit int) ([]Note, []NoteTombstone, bool, error)
	GetNoteByID(ctx context.Context, noteID uuid.UUID, userID uuid.UUID) (*Note, error)
	UpsertNote(ctx context.Context, userID uuid.UUID, noteID uuid.UUID, encryptedPayload []byte, baseVersion *time.Time) (*Note, error)
	DeleteNote(ctx context.Context, noteID uuid.UUID, userID uuid.UUID) error
}

func NewNotesHandler(service service) *NotesHandler {
	return &NotesHandler{service: service}
}

func getUserID(c *echo.Context) (uuid.UUID, error) {
	val := c.Get(internalmw.UserContextKey)
	if val == nil {
		return uuid.Nil, errors.New("user_id not found in context")
	}
	userID, ok := val.(uuid.UUID)
	if !ok {
		return uuid.Nil, errors.New("user_id has invalid type")
	}
	return userID, nil
}

func (h *NotesHandler) GetAll(c *echo.Context) error {
	userID, err := getUserID(c)
	if err != nil {
		return c.JSON(http.StatusUnauthorized, api.ErrorResponse{Error: "unauthorized", Code: "UNAUTHORIZED"})
	}

	var since *time.Time
	if sinceParam := c.QueryParam("since"); sinceParam != "" {
		t, err := time.Parse(time.RFC3339, sinceParam)
		if err != nil {
			return c.JSON(http.StatusBadRequest, api.ErrorResponse{Error: "invalid since parameter, expected ISO-8601 format", Code: "INVALID_PARAM"})
		}
		since = &t
	}

	var cursor *NoteCursor
	if cursorParam := c.QueryParam("cursor"); cursorParam != "" {
		raw, err := base64.StdEncoding.DecodeString(cursorParam)
		if err != nil {
			return c.JSON(http.StatusBadRequest, api.ErrorResponse{Error: "invalid cursor", Code: "INVALID_PARAM"})
		}
		var cur NoteCursor
		if err := json.Unmarshal(raw, &cur); err != nil {
			return c.JSON(http.StatusBadRequest, api.ErrorResponse{Error: "invalid cursor", Code: "INVALID_PARAM"})
		}
		cursor = &cur
	}

	limit := defaultPageSize
	if limitParam := c.QueryParam("limit"); limitParam != "" {
		n, err := strconv.Atoi(limitParam)
		if err != nil || n < 1 {
			return c.JSON(http.StatusBadRequest, api.ErrorResponse{Error: "invalid limit parameter", Code: "INVALID_PARAM"})
		}
		if n > maxPageSize {
			n = maxPageSize
		}
		limit = n
	}

	notes, tombstones, hasMore, err := h.service.GetNotesSince(c.Request().Context(), userID, since, cursor, limit)
	if err != nil {
		log.Warn().Err(err).Msg(err.Error())
		return c.JSON(http.StatusInternalServerError, api.ErrorResponse{Error: "failed to fetch notes", Code: "INTERNAL_ERROR"})
	}

	noteResponses := make([]NoteResponse, 0, len(notes))
	for _, n := range notes {
		noteResponses = append(noteResponses, toNoteResponse(&n))
	}

	tombstoneResponses := make([]TombstoneResponse, 0, len(tombstones))
	for _, t := range tombstones {
		tombstoneResponses = append(tombstoneResponses, toTombstoneResponse(t))
	}

	var nextCursor *string
	if hasMore && len(notes) > 0 {
		last := notes[len(notes)-1]
		raw, err := json.Marshal(NoteCursor{AfterUpdatedAt: last.UpdatedAt, AfterNoteID: last.ID})
		if err == nil {
			encoded := base64.StdEncoding.EncodeToString(raw)
			nextCursor = &encoded
		}
	}

	serverTime := time.Now().UTC()
	return c.JSON(http.StatusOK, NotesListResponse{
		Notes:      noteResponses,
		Tombstones: tombstoneResponses,
		NextCursor: nextCursor,
		ServerTime: &serverTime,
	})
}

func (h *NotesHandler) GetByID(c *echo.Context) error {
	userID, err := getUserID(c)
	if err != nil {
		return c.JSON(http.StatusUnauthorized, api.ErrorResponse{Error: "unauthorized", Code: "UNAUTHORIZED"})
	}

	noteID, err := uuid.Parse(c.Param("note_id"))
	if err != nil {
		return c.JSON(http.StatusBadRequest, api.ErrorResponse{Error: "invalid note_id", Code: "INVALID_PARAM"})
	}

	note, err := h.service.GetNoteByID(c.Request().Context(), noteID, userID)
	if err != nil {
		if errors.Is(err, ErrNoteNotFound) {
			return c.JSON(http.StatusNotFound, api.ErrorResponse{Error: "note not found", Code: "NOT_FOUND"})
		}
		return c.JSON(http.StatusInternalServerError, api.ErrorResponse{Error: "failed to fetch note", Code: "INTERNAL_ERROR"})
	}

	return c.JSON(http.StatusOK, toNoteResponse(note))
}

func (h *NotesHandler) Upsert(c *echo.Context) error {
	userID, err := getUserID(c)
	if err != nil {
		return c.JSON(http.StatusUnauthorized, api.ErrorResponse{Error: "unauthorized", Code: "UNAUTHORIZED"})
	}

	noteID, err := uuid.Parse(c.Param("note_id"))
	if err != nil {
		return c.JSON(http.StatusBadRequest, api.ErrorResponse{Error: "invalid note_id", Code: "INVALID_PARAM"})
	}

	var req UpsertNoteRequest
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, api.ErrorResponse{Error: "invalid request body", Code: "INVALID_BODY"})
	}
	if err := c.Validate(&req); err != nil {
		return c.JSON(http.StatusBadRequest, api.ErrorResponse{Error: err.Error(), Code: "VALIDATION_ERROR"})
	}

	payloadBytes, err := base64.StdEncoding.DecodeString(req.EncryptedPayload)
	if err != nil {
		return c.JSON(http.StatusBadRequest, api.ErrorResponse{Error: "encrypted_payload must be valid base64", Code: "INVALID_PAYLOAD"})
	}

	note, err := h.service.UpsertNote(c.Request().Context(), userID, noteID, payloadBytes, req.BaseVersion)
	if err != nil {
		if errors.Is(err, ErrConflict) {
			return c.JSON(http.StatusConflict, api.ErrorResponse{Error: "conflict: server version is newer", Code: "CONFLICT"})
		}
		return c.JSON(http.StatusInternalServerError, api.ErrorResponse{Error: "failed to save note", Code: "INTERNAL_ERROR"})
	}

	return c.JSON(http.StatusOK, toNoteResponse(note))
}

func (h *NotesHandler) Delete(c *echo.Context) error {
	userID, err := getUserID(c)
	if err != nil {
		return c.JSON(http.StatusUnauthorized, api.ErrorResponse{Error: "unauthorized", Code: "UNAUTHORIZED"})
	}

	noteID, err := uuid.Parse(c.Param("note_id"))
	if err != nil {
		return c.JSON(http.StatusBadRequest, api.ErrorResponse{Error: "invalid note_id", Code: "INVALID_PARAM"})
	}

	err = h.service.DeleteNote(c.Request().Context(), noteID, userID)
	if err != nil {
		if errors.Is(err, ErrNoteNotFound) {
			return c.JSON(http.StatusNotFound, api.ErrorResponse{Error: "note not found", Code: "NOT_FOUND"})
		}
		return c.JSON(http.StatusInternalServerError, api.ErrorResponse{Error: "failed to delete note", Code: "INTERNAL_ERROR"})
	}

	return c.NoContent(http.StatusNoContent)
}
