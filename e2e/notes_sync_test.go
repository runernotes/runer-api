package e2e_test

import (
	"encoding/base64"
	"net/http"
	"testing"

	"github.com/google/uuid"
)

// TestUpsertNoteFirstWrite verifies that a PUT without base_version creates the note and
// returns 200 with note_id, encrypted_payload, and timestamps.
func TestUpsertNoteFirstWrite(t *testing.T) {
	srv, mock := newTestServer(t)
	e := newExpect(t, srv)

	token := registerAndLogin(t, e, mock, uuid.NewString())
	noteID := uuid.New().String()
	payload := base64.StdEncoding.EncodeToString([]byte("initial content"))

	obj := e.PUT("/api/v1/notes/"+noteID).
		WithHeader("Authorization", "Bearer "+token).
		WithJSON(map[string]any{"encrypted_payload": payload}).
		Expect().
		Status(http.StatusOK).
		JSON().Object()

	obj.HasValue("note_id", noteID)
	obj.Value("updated_at").NotNull()
	obj.Value("encrypted_payload").String().IsEqual(payload)
}

// TestUpsertNoteUpdate verifies that a PUT with the correct base_version updates the note
// and returns a new updated_at.
func TestUpsertNoteUpdate(t *testing.T) {
	srv, mock := newTestServer(t)
	e := newExpect(t, srv)

	token := registerAndLogin(t, e, mock, uuid.NewString())
	noteID := uuid.New().String()
	payload1 := base64.StdEncoding.EncodeToString([]byte("version 1"))
	payload2 := base64.StdEncoding.EncodeToString([]byte("version 2"))

	// First write — no base_version.
	v1 := e.PUT("/api/v1/notes/"+noteID).
		WithHeader("Authorization", "Bearer "+token).
		WithJSON(map[string]any{"encrypted_payload": payload1}).
		Expect().
		Status(http.StatusOK).
		JSON().Object()

	updatedAt := v1.Value("updated_at").String().NotEmpty().Raw()

	// Second write — supply base_version from first response.
	v2 := e.PUT("/api/v1/notes/"+noteID).
		WithHeader("Authorization", "Bearer "+token).
		WithJSON(map[string]any{
			"encrypted_payload": payload2,
			"base_version":      updatedAt,
		}).
		Expect().
		Status(http.StatusOK).
		JSON().Object()

	v2.HasValue("note_id", noteID)
	v2.Value("encrypted_payload").String().IsEqual(payload2)
}

// TestUpsertNoteConflict verifies that a PUT with a stale base_version returns 409 and the
// response body is the full current server note (not an error object), so the client can
// resolve the conflict without an extra round trip.
func TestUpsertNoteConflict(t *testing.T) {
	srv, mock := newTestServer(t)
	e := newExpect(t, srv)

	token := registerAndLogin(t, e, mock, uuid.NewString())
	noteID := uuid.New().String()
	payload1 := base64.StdEncoding.EncodeToString([]byte("version 1"))
	payload2 := base64.StdEncoding.EncodeToString([]byte("version 2"))
	payloadConflict := base64.StdEncoding.EncodeToString([]byte("conflicting edit"))

	// Baseline: first write succeeds.
	v1 := e.PUT("/api/v1/notes/"+noteID).
		WithHeader("Authorization", "Bearer "+token).
		WithJSON(map[string]any{"encrypted_payload": payload1}).
		Expect().
		Status(http.StatusOK).
		JSON().Object()

	originalUpdatedAt := v1.Value("updated_at").String().NotEmpty().Raw()

	// Advance the server version.
	e.PUT("/api/v1/notes/"+noteID).
		WithHeader("Authorization", "Bearer "+token).
		WithJSON(map[string]any{
			"encrypted_payload": payload2,
			"base_version":      originalUpdatedAt,
		}).
		Expect().
		Status(http.StatusOK)

	// Push with the stale base_version — must get 409 with the server note in the body.
	conflict := e.PUT("/api/v1/notes/"+noteID).
		WithHeader("Authorization", "Bearer "+token).
		WithJSON(map[string]any{
			"encrypted_payload": payloadConflict,
			"base_version":      originalUpdatedAt,
		}).
		Expect().
		Status(http.StatusConflict).
		JSON().Object()

	// Response must be the full note object, not an error body.
	conflict.HasValue("note_id", noteID)
	conflict.Value("encrypted_payload").String().IsEqual(payload2) // server's current payload
	conflict.Value("updated_at").NotNull()
}

// TestUpsertNoteConflictResolve verifies the full conflict resolution flow:
// detect conflict → re-push with server's updated_at → accepted.
func TestUpsertNoteConflictResolve(t *testing.T) {
	srv, mock := newTestServer(t)
	e := newExpect(t, srv)

	token := registerAndLogin(t, e, mock, uuid.NewString())
	noteID := uuid.New().String()
	payload1 := base64.StdEncoding.EncodeToString([]byte("version 1"))
	payload2 := base64.StdEncoding.EncodeToString([]byte("version 2"))
	payloadResolved := base64.StdEncoding.EncodeToString([]byte("resolved content"))

	// First write.
	v1 := e.PUT("/api/v1/notes/"+noteID).
		WithHeader("Authorization", "Bearer "+token).
		WithJSON(map[string]any{"encrypted_payload": payload1}).
		Expect().
		Status(http.StatusOK).
		JSON().Object()

	originalUpdatedAt := v1.Value("updated_at").String().NotEmpty().Raw()

	// Advance server version.
	e.PUT("/api/v1/notes/"+noteID).
		WithHeader("Authorization", "Bearer "+token).
		WithJSON(map[string]any{
			"encrypted_payload": payload2,
			"base_version":      originalUpdatedAt,
		}).
		Expect().
		Status(http.StatusOK)

	// Trigger conflict — get server's updated_at from 409 body.
	conflictResp := e.PUT("/api/v1/notes/"+noteID).
		WithHeader("Authorization", "Bearer "+token).
		WithJSON(map[string]any{
			"encrypted_payload": payloadResolved,
			"base_version":      originalUpdatedAt,
		}).
		Expect().
		Status(http.StatusConflict).
		JSON().Object()

	serverUpdatedAt := conflictResp.Value("updated_at").String().NotEmpty().Raw()

	// Re-push the resolved content using the server's updated_at as the new base_version.
	e.PUT("/api/v1/notes/"+noteID).
		WithHeader("Authorization", "Bearer "+token).
		WithJSON(map[string]any{
			"encrypted_payload": payloadResolved,
			"base_version":      serverUpdatedAt,
		}).
		Expect().
		Status(http.StatusOK).
		JSON().Object().
		HasValue("note_id", noteID)
}
