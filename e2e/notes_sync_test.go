package e2e_test

import (
	"encoding/base64"
	"net/http"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

// TestUpsertNoteCreate verifies that a PUT /notes/:id without a base_version creates a
// new note and returns the note_id, updated_at, and encrypted_payload.
func TestUpsertNoteCreate(t *testing.T) {
	srv, mock, _ := newTestServer(t)
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

// TestUpsertNoteUpdateWithCorrectBaseVersion verifies that a PUT /notes/:id with the
// correct base_version succeeds and returns the updated note.
func TestUpsertNoteUpdateWithCorrectBaseVersion(t *testing.T) {
	srv, mock, _ := newTestServer(t)
	e := newExpect(t, srv)
	token := registerAndLogin(t, e, mock, uuid.NewString())

	noteID := uuid.New().String()
	payload1 := base64.StdEncoding.EncodeToString([]byte("version 1"))
	payload2 := base64.StdEncoding.EncodeToString([]byte("version 2"))

	v1 := e.PUT("/api/v1/notes/"+noteID).
		WithHeader("Authorization", "Bearer "+token).
		WithJSON(map[string]any{"encrypted_payload": payload1}).
		Expect().
		Status(http.StatusOK).
		JSON().Object()

	updatedAt := v1.Value("updated_at").String().NotEmpty().Raw()

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

// TestUpsertNoteStaleBaseVersionReturns409 verifies that sending a stale base_version
// returns 409 with the current server note in the response body.
func TestUpsertNoteStaleBaseVersionReturns409(t *testing.T) {
	srv, mock, _ := newTestServer(t)
	e := newExpect(t, srv)
	token := registerAndLogin(t, e, mock, uuid.NewString())

	noteID := uuid.New().String()
	payload1 := base64.StdEncoding.EncodeToString([]byte("version 1"))
	payload2 := base64.StdEncoding.EncodeToString([]byte("version 2"))
	payloadConflict := base64.StdEncoding.EncodeToString([]byte("conflicting edit"))

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

// TestUpsertNoteConflictResolutionRepush verifies that after receiving a 409, the client
// can re-push with the server's updated_at as the new base_version and succeed.
func TestUpsertNoteConflictResolutionRepush(t *testing.T) {
	srv, mock, _ := newTestServer(t)
	e := newExpect(t, srv)
	token := registerAndLogin(t, e, mock, uuid.NewString())

	noteID := uuid.New().String()
	payload1 := base64.StdEncoding.EncodeToString([]byte("version 1"))
	payload2 := base64.StdEncoding.EncodeToString([]byte("version 2"))
	payloadResolved := base64.StdEncoding.EncodeToString([]byte("resolved content"))

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

	// Re-push with server's updated_at as the new base_version.
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

// TestUpsertNoteMissingPayloadReturns400 verifies that a PUT /notes/:id with no
// encrypted_payload field returns 400 with VALIDATION_ERROR.
func TestUpsertNoteMissingPayloadReturns400(t *testing.T) {
	srv, mock, _ := newTestServer(t)
	e := newExpect(t, srv)
	token := registerAndLogin(t, e, mock, uuid.NewString())

	e.PUT("/api/v1/notes/"+uuid.New().String()).
		WithHeader("Authorization", "Bearer "+token).
		WithJSON(map[string]any{}).
		Expect().
		Status(http.StatusBadRequest).
		JSON().Object().
		HasValue("code", "VALIDATION_ERROR")
}

// TestUpsertNoteNonBase64PayloadReturns400 verifies that a PUT /notes/:id with a
// non-base64 encrypted_payload returns 400 with INVALID_PAYLOAD.
func TestUpsertNoteNonBase64PayloadReturns400(t *testing.T) {
	srv, mock, _ := newTestServer(t)
	e := newExpect(t, srv)
	token := registerAndLogin(t, e, mock, uuid.NewString())

	e.PUT("/api/v1/notes/"+uuid.New().String()).
		WithHeader("Authorization", "Bearer "+token).
		WithJSON(map[string]any{"encrypted_payload": "this is not valid base64!!!"}).
		Expect().
		Status(http.StatusBadRequest).
		JSON().Object().
		HasValue("code", "INVALID_PAYLOAD")
}

// TestUpsertNoteNonUUIDPathReturns400 verifies that a PUT /notes/:id with a non-UUID
// path segment returns 400 with INVALID_PARAM.
func TestUpsertNoteNonUUIDPathReturns400(t *testing.T) {
	srv, mock, _ := newTestServer(t)
	e := newExpect(t, srv)
	token := registerAndLogin(t, e, mock, uuid.NewString())

	validPayload := base64.StdEncoding.EncodeToString([]byte("valid content"))

	e.PUT("/api/v1/notes/not-a-uuid").
		WithHeader("Authorization", "Bearer "+token).
		WithJSON(map[string]any{"encrypted_payload": validPayload}).
		Expect().
		Status(http.StatusBadRequest).
		JSON().Object().
		HasValue("code", "INVALID_PARAM")
}

// TestGetNotesEmptyListForNewUser verifies that a newly registered user receives an empty
// notes list with null next_cursor and a server_time.
func TestGetNotesEmptyListForNewUser(t *testing.T) {
	srv, mock, _ := newTestServer(t)
	e := newExpect(t, srv)
	token := registerAndLogin(t, e, mock, uuid.NewString())

	resp := e.GET("/api/v1/notes").
		WithHeader("Authorization", "Bearer "+token).
		Expect().
		Status(http.StatusOK).
		JSON().Object()

	resp.Value("notes").Array().Length().IsEqual(0)
	resp.Value("tombstones").Array().Length().IsEqual(0)
	resp.Value("next_cursor").IsNull()
	resp.Value("server_time").NotNull()
}

// TestGetNotesFullSyncReturnsAllCreatedNotes verifies that a full sync returns all notes
// that have been created by the authenticated user.
func TestGetNotesFullSyncReturnsAllCreatedNotes(t *testing.T) {
	srv, mock, _ := newTestServer(t)
	e := newExpect(t, srv)
	token := registerAndLogin(t, e, mock, uuid.NewString())

	id1 := createNote(t, e, token)
	id2 := createNote(t, e, token)

	resp := e.GET("/api/v1/notes").
		WithHeader("Authorization", "Bearer "+token).
		Expect().
		Status(http.StatusOK).
		JSON().Object()

	notes := resp.Value("notes").Array()
	notes.Length().IsEqual(2)

	returnedIDs := make([]string, 0)
	for _, v := range notes.Iter() {
		returnedIDs = append(returnedIDs, v.Object().Value("note_id").String().Raw())
	}
	require.Contains(t, returnedIDs, id1)
	require.Contains(t, returnedIDs, id2)
}

// TestGetNotesDeltaSyncReturnsOnlyNotesAfterSince verifies that a delta sync with a
// since timestamp returns only notes updated after that timestamp.
func TestGetNotesDeltaSyncReturnsOnlyNotesAfterSince(t *testing.T) {
	srv, mock, _ := newTestServer(t)
	e := newExpect(t, srv)
	token := registerAndLogin(t, e, mock, uuid.NewString())

	// Create a note before the since timestamp.
	_ = createNote(t, e, token) // created before since — must not appear

	since := time.Now().UTC()

	// Create a note after the since timestamp.
	id2 := createNote(t, e, token)

	resp := e.GET("/api/v1/notes").
		WithHeader("Authorization", "Bearer "+token).
		WithQuery("since", since.Format(time.RFC3339Nano)).
		Expect().
		Status(http.StatusOK).
		JSON().Object()

	notes := resp.Value("notes").Array()
	notes.Length().IsEqual(1)

	returnedID := notes.Value(0).Object().Value("note_id").String().Raw()
	require.Equal(t, id2, returnedID, "only the note created after since must be returned")
}

// TestGetNotesInvalidSinceParamReturns400 verifies that a malformed since query parameter
// returns 400 with INVALID_PARAM.
func TestGetNotesInvalidSinceParamReturns400(t *testing.T) {
	srv, mock, _ := newTestServer(t)
	e := newExpect(t, srv)
	token := registerAndLogin(t, e, mock, uuid.NewString())

	e.GET("/api/v1/notes").
		WithHeader("Authorization", "Bearer "+token).
		WithQuery("since", "not-a-valid-date").
		Expect().
		Status(http.StatusBadRequest).
		JSON().Object().
		HasValue("code", "INVALID_PARAM")
}

// TestGetNotesFullLifecycleReflectsInFullSync verifies that after creating, updating,
// trashing, and restoring a note it appears in a full sync as an active note.
func TestGetNotesFullLifecycleReflectsInFullSync(t *testing.T) {
	srv, mock, _ := newTestServer(t)
	e := newExpect(t, srv)
	token := registerAndLogin(t, e, mock, uuid.NewString())

	noteID := uuid.New().String()
	payload1 := base64.StdEncoding.EncodeToString([]byte("initial"))
	payload2 := base64.StdEncoding.EncodeToString([]byte("updated"))

	v1 := e.PUT("/api/v1/notes/"+noteID).
		WithHeader("Authorization", "Bearer "+token).
		WithJSON(map[string]any{"encrypted_payload": payload1}).
		Expect().
		Status(http.StatusOK).
		JSON().Object()

	updatedAt := v1.Value("updated_at").String().NotEmpty().Raw()

	e.PUT("/api/v1/notes/"+noteID).
		WithHeader("Authorization", "Bearer "+token).
		WithJSON(map[string]any{
			"encrypted_payload": payload2,
			"base_version":      updatedAt,
		}).
		Expect().
		Status(http.StatusOK)

	e.DELETE("/api/v1/notes/"+noteID).
		WithHeader("Authorization", "Bearer "+token).
		Expect().
		Status(http.StatusNoContent)

	e.POST("/api/v1/notes/"+noteID+"/restore").
		WithHeader("Authorization", "Bearer "+token).
		Expect().
		Status(http.StatusOK)

	syncResp := e.GET("/api/v1/notes").
		WithHeader("Authorization", "Bearer "+token).
		Expect().
		Status(http.StatusOK).
		JSON().Object()

	// The note must appear in the active list.
	notes := syncResp.Value("notes").Array()
	found := false
	for _, v := range notes.Iter() {
		obj := v.Object()
		if obj.Value("note_id").String().Raw() == noteID {
			obj.Value("trashed_at").IsNull()
			found = true
			break
		}
	}
	if !found {
		t.Errorf("note %s not found in full sync response", noteID)
	}
}

// TestGetNotesPurgedNoteAppearsAsTombstone verifies that after purging a note a full
// sync returns no notes and a tombstone for the purged note.
func TestGetNotesPurgedNoteAppearsAsTombstone(t *testing.T) {
	srv, mock, _ := newTestServer(t)
	e := newExpect(t, srv)
	// Use a fresh user so tombstone count is predictable.
	token := registerAndLogin(t, e, mock, uuid.NewString())
	noteID := createNote(t, e, token)

	e.DELETE("/api/v1/notes/"+noteID+"/purge").
		WithHeader("Authorization", "Bearer "+token).
		Expect().
		Status(http.StatusNoContent)

	syncResp := e.GET("/api/v1/notes").
		WithHeader("Authorization", "Bearer "+token).
		Expect().
		Status(http.StatusOK).
		JSON().Object()

	syncResp.Value("notes").Array().Length().IsEqual(0)

	tombstones := syncResp.Value("tombstones").Array()
	tombstones.Length().IsEqual(1)
	tombstones.Value(0).Object().HasValue("note_id", noteID)
}

// TestGetNoteByIDReturnsCorrectFields verifies that GET /notes/:id returns the full note
// object with the expected fields for an existing note.
func TestGetNoteByIDReturnsCorrectFields(t *testing.T) {
	srv, mock, _ := newTestServer(t)
	e := newExpect(t, srv)
	token := registerAndLogin(t, e, mock, uuid.NewString())

	noteID := uuid.New().String()
	payload := base64.StdEncoding.EncodeToString([]byte("test content"))

	e.PUT("/api/v1/notes/"+noteID).
		WithHeader("Authorization", "Bearer "+token).
		WithJSON(map[string]any{"encrypted_payload": payload}).
		Expect().
		Status(http.StatusOK)

	obj := e.GET("/api/v1/notes/"+noteID).
		WithHeader("Authorization", "Bearer "+token).
		Expect().
		Status(http.StatusOK).
		JSON().Object()

	obj.HasValue("note_id", noteID)
	obj.HasValue("encrypted_payload", payload)
	obj.Value("created_at").NotNull()
	obj.Value("updated_at").NotNull()
	obj.Value("trashed_at").IsNull()
}

// TestGetNoteByIDUnknownUUIDReturns404 verifies that requesting a note with a UUID that
// does not belong to the user returns 404 with NOT_FOUND.
func TestGetNoteByIDUnknownUUIDReturns404(t *testing.T) {
	srv, mock, _ := newTestServer(t)
	e := newExpect(t, srv)
	token := registerAndLogin(t, e, mock, uuid.NewString())

	e.GET("/api/v1/notes/"+uuid.New().String()).
		WithHeader("Authorization", "Bearer "+token).
		Expect().
		Status(http.StatusNotFound).
		JSON().Object().HasValue("code", "NOT_FOUND")
}

// TestGetNoteByIDNonUUIDPathReturns400 verifies that a non-UUID path segment returns
// 400 with INVALID_PARAM.
func TestGetNoteByIDNonUUIDPathReturns400(t *testing.T) {
	srv, mock, _ := newTestServer(t)
	e := newExpect(t, srv)
	token := registerAndLogin(t, e, mock, uuid.NewString())

	e.GET("/api/v1/notes/not-a-uuid").
		WithHeader("Authorization", "Bearer "+token).
		Expect().
		Status(http.StatusBadRequest).
		JSON().Object().
		HasValue("code", "INVALID_PARAM")
}
