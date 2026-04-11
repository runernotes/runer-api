package e2e_test

// notes_auth_test.go verifies that every notes endpoint enforces authentication.
// Each test has a positive baseline (the endpoint is reachable with a valid token)
// followed by the negative assertion (no token → 401 UNAUTHORIZED).
// This pattern prevents false positives from routes that are simply not registered.

import (
	"encoding/base64"
	"net/http"
	"testing"

	"github.com/google/uuid"
)

// TestGetNotes_RequiresAuth verifies GET /notes returns 401 without a token.
func TestGetNotes_RequiresAuth(t *testing.T) {
	srv, mock, _ := newTestServer(t)
	e := newExpect(t, srv)

	// Positive baseline: with a valid token the endpoint returns 200.
	token := registerAndLogin(t, e, mock, uuid.NewString())
	e.GET("/api/v1/notes").
		WithHeader("Authorization", "Bearer "+token).
		Expect().
		Status(http.StatusOK)

	// Negative: no token must produce 401.
	e.GET("/api/v1/notes").
		Expect().
		Status(http.StatusUnauthorized).
		JSON().Object().
		HasValue("code", "UNAUTHORIZED")
}

// TestGetNoteByID_RequiresAuth verifies GET /notes/:id returns 401 without a token.
func TestGetNoteByID_RequiresAuth(t *testing.T) {
	srv, mock, _ := newTestServer(t)
	e := newExpect(t, srv)

	token := registerAndLogin(t, e, mock, uuid.NewString())
	noteID := createNote(t, e, token)

	// Positive baseline: owner can read it.
	e.GET("/api/v1/notes/"+noteID).
		WithHeader("Authorization", "Bearer "+token).
		Expect().
		Status(http.StatusOK)

	// Negative: no token → 401.
	e.GET("/api/v1/notes/"+noteID).
		Expect().
		Status(http.StatusUnauthorized).
		JSON().Object().
		HasValue("code", "UNAUTHORIZED")
}

// TestUpsertNote_RequiresAuth verifies PUT /notes/:id returns 401 without a token.
func TestUpsertNote_RequiresAuth(t *testing.T) {
	srv, mock, _ := newTestServer(t)
	e := newExpect(t, srv)

	token := registerAndLogin(t, e, mock, uuid.NewString())
	noteID := uuid.New().String()
	payload := base64.StdEncoding.EncodeToString([]byte("content"))

	// Positive baseline: with a valid token the upsert succeeds.
	e.PUT("/api/v1/notes/"+noteID).
		WithHeader("Authorization", "Bearer "+token).
		WithJSON(map[string]any{"encrypted_payload": payload}).
		Expect().
		Status(http.StatusOK)

	// Negative: no token → 401.
	e.PUT("/api/v1/notes/"+uuid.New().String()).
		WithJSON(map[string]any{"encrypted_payload": payload}).
		Expect().
		Status(http.StatusUnauthorized).
		JSON().Object().
		HasValue("code", "UNAUTHORIZED")
}

// TestTrashNote_RequiresAuth verifies DELETE /notes/:id returns 401 without a token.
func TestTrashNote_RequiresAuth(t *testing.T) {
	srv, mock, _ := newTestServer(t)
	e := newExpect(t, srv)

	token := registerAndLogin(t, e, mock, uuid.NewString())
	noteID := createNote(t, e, token)

	// Positive baseline: owner can trash it.
	e.DELETE("/api/v1/notes/"+noteID).
		WithHeader("Authorization", "Bearer "+token).
		Expect().
		Status(http.StatusNoContent)

	// Negative: no token → 401.
	e.DELETE("/api/v1/notes/"+uuid.New().String()).
		Expect().
		Status(http.StatusUnauthorized).
		JSON().Object().
		HasValue("code", "UNAUTHORIZED")
}

// TestRestoreNote_RequiresAuth verifies POST /notes/:id/restore returns 401 without a token.
func TestRestoreNote_RequiresAuth(t *testing.T) {
	srv, mock, _ := newTestServer(t)
	e := newExpect(t, srv)

	token := registerAndLogin(t, e, mock, uuid.NewString())
	noteID := createNote(t, e, token)

	// Trash the note first so restore has something to act on.
	e.DELETE("/api/v1/notes/"+noteID).
		WithHeader("Authorization", "Bearer "+token).
		Expect().
		Status(http.StatusNoContent)

	// Positive baseline: owner can restore it.
	e.POST("/api/v1/notes/"+noteID+"/restore").
		WithHeader("Authorization", "Bearer "+token).
		Expect().
		Status(http.StatusOK)

	// Negative: no token → 401.
	e.POST("/api/v1/notes/"+uuid.New().String()+"/restore").
		Expect().
		Status(http.StatusUnauthorized).
		JSON().Object().
		HasValue("code", "UNAUTHORIZED")
}

// TestPurgeNote_RequiresAuth verifies DELETE /notes/:id/purge returns 401 without a token.
func TestPurgeNote_RequiresAuth(t *testing.T) {
	srv, mock, _ := newTestServer(t)
	e := newExpect(t, srv)

	token := registerAndLogin(t, e, mock, uuid.NewString())
	noteID := createNote(t, e, token)

	// Positive baseline: owner can purge it.
	e.DELETE("/api/v1/notes/"+noteID+"/purge").
		WithHeader("Authorization", "Bearer "+token).
		Expect().
		Status(http.StatusNoContent)

	// Negative: no token → 401.
	e.DELETE("/api/v1/notes/"+uuid.New().String()+"/purge").
		Expect().
		Status(http.StatusUnauthorized).
		JSON().Object().
		HasValue("code", "UNAUTHORIZED")
}
