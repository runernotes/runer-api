package e2e_test

import (
	"encoding/base64"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gavv/httpexpect/v2"
	"github.com/google/uuid"
)

// registerAndLogin creates a new user account with a unique email address, verifies the magic
// link and returns the access token. Each call produces an independent user.
func registerAndLogin(t *testing.T, e *httpexpect.Expect, mock *mockEmailSender, suffix string) string {
	t.Helper()

	email := fmt.Sprintf("user-%s@example.com", suffix)

	e.POST("/api/v1/auth/register").
		WithJSON(map[string]string{"email": email, "name": "Test User"}).
		Expect().
		Status(http.StatusOK)

	token := mock.lastToken()
	if token == "" {
		t.Fatalf("registerAndLogin: no magic link token captured for %s", email)
	}

	loginResp := e.POST("/api/v1/auth/verify").
		WithJSON(map[string]string{"token": token}).
		Expect().
		Status(http.StatusOK).
		JSON().Object()

	return loginResp.Value("access_token").String().NotEmpty().Raw()
}

// createNote creates a note with a random UUID and a dummy encrypted payload and returns the
// note_id string from the server response.
func createNote(t *testing.T, e *httpexpect.Expect, accessToken string) string {
	t.Helper()

	noteID := uuid.New().String()
	payload := base64.StdEncoding.EncodeToString([]byte("hello"))

	e.PUT("/api/v1/notes/"+noteID).
		WithHeader("Authorization", "Bearer "+accessToken).
		WithJSON(map[string]any{"encrypted_payload": payload}).
		Expect().
		Status(http.StatusOK).
		JSON().Object().HasValue("note_id", noteID)

	return noteID
}

// newExpect creates an httpexpect instance wired to the given httptest.Server.
func newExpect(t *testing.T, srv *httptest.Server) *httpexpect.Expect {
	t.Helper()
	return httpexpect.WithConfig(httpexpect.Config{
		BaseURL:  srv.URL,
		Reporter: httpexpect.NewRequireReporter(t),
		Client:   srv.Client(),
	})
}

// TestTrashNote verifies that DELETE /:note_id sets trashed_at on the note.
// After trashing, a GET returns 200 with a non-null trashed_at.
func TestTrashNote(t *testing.T) {
	srv, mock, _ := newTestServer(t)
	e := newExpect(t, srv)

	token := registerAndLogin(t, e, mock, uuid.NewString())
	noteID := createNote(t, e, token)

	// Trash the note.
	e.DELETE("/api/v1/notes/"+noteID).
		WithHeader("Authorization", "Bearer "+token).
		Expect().
		Status(http.StatusNoContent)

	// Fetch the note — trashed_at must now be set.
	e.GET("/api/v1/notes/"+noteID).
		WithHeader("Authorization", "Bearer "+token).
		Expect().
		Status(http.StatusOK).
		JSON().Object().
		Value("trashed_at").NotNull()
}

// TestTrashNoteNotFound ensures DELETE on an unknown UUID returns 404.
func TestTrashNoteNotFound(t *testing.T) {
	srv, mock, _ := newTestServer(t)
	e := newExpect(t, srv)

	token := registerAndLogin(t, e, mock, uuid.NewString())

	e.DELETE("/api/v1/notes/"+uuid.New().String()).
		WithHeader("Authorization", "Bearer "+token).
		Expect().
		Status(http.StatusNotFound).
		JSON().Object().HasValue("code", "NOT_FOUND")
}

// TestRestoreNote verifies that POST /:note_id/restore clears trashed_at.
func TestRestoreNote(t *testing.T) {
	srv, mock, _ := newTestServer(t)
	e := newExpect(t, srv)

	token := registerAndLogin(t, e, mock, uuid.NewString())
	noteID := createNote(t, e, token)

	// Trash, then restore.
	e.DELETE("/api/v1/notes/"+noteID).
		WithHeader("Authorization", "Bearer "+token).
		Expect().
		Status(http.StatusNoContent)

	e.POST("/api/v1/notes/"+noteID+"/restore").
		WithHeader("Authorization", "Bearer "+token).
		Expect().
		Status(http.StatusOK).
		JSON().Object().
		Value("trashed_at").IsNull()
}

// TestRestoreNoteNotFound ensures POST restore on an unknown UUID returns 404.
func TestRestoreNoteNotFound(t *testing.T) {
	srv, mock, _ := newTestServer(t)
	e := newExpect(t, srv)

	token := registerAndLogin(t, e, mock, uuid.NewString())

	e.POST("/api/v1/notes/"+uuid.New().String()+"/restore").
		WithHeader("Authorization", "Bearer "+token).
		Expect().
		Status(http.StatusNotFound).
		JSON().Object().HasValue("code", "NOT_FOUND")
}

// TestPurgeNote verifies that trashing then purging a note hard-deletes it and creates a
// tombstone that appears in the next full sync.
func TestPurgeNote(t *testing.T) {
	srv, mock, _ := newTestServer(t)
	e := newExpect(t, srv)

	token := registerAndLogin(t, e, mock, uuid.NewString())
	noteID := createNote(t, e, token)

	// Trash, then purge.
	e.DELETE("/api/v1/notes/"+noteID).
		WithHeader("Authorization", "Bearer "+token).
		Expect().
		Status(http.StatusNoContent)

	e.DELETE("/api/v1/notes/"+noteID+"/purge").
		WithHeader("Authorization", "Bearer "+token).
		Expect().
		Status(http.StatusNoContent)

	// The note must be gone.
	e.GET("/api/v1/notes/"+noteID).
		WithHeader("Authorization", "Bearer "+token).
		Expect().
		Status(http.StatusNotFound)

	// A tombstone for the note must appear in a full sync.
	syncResp := e.GET("/api/v1/notes").
		WithHeader("Authorization", "Bearer "+token).
		Expect().
		Status(http.StatusOK).
		JSON().Object()

	tombstones := syncResp.Value("tombstones").Array()
	tombstones.Length().IsEqual(1)
	tombstones.Value(0).Object().HasValue("note_id", noteID)
}

// TestPurgeNoteThatIsNotTrashed verifies that a note can be purged directly without being
// trashed first. The note should disappear and a tombstone should be created.
func TestPurgeNoteThatIsNotTrashed(t *testing.T) {
	srv, mock, _ := newTestServer(t)
	e := newExpect(t, srv)

	token := registerAndLogin(t, e, mock, uuid.NewString())
	noteID := createNote(t, e, token)

	// Purge without trashing first.
	e.DELETE("/api/v1/notes/"+noteID+"/purge").
		WithHeader("Authorization", "Bearer "+token).
		Expect().
		Status(http.StatusNoContent)

	// The note must be gone.
	e.GET("/api/v1/notes/"+noteID).
		WithHeader("Authorization", "Bearer "+token).
		Expect().
		Status(http.StatusNotFound)

	// A tombstone must exist.
	syncResp := e.GET("/api/v1/notes").
		WithHeader("Authorization", "Bearer "+token).
		Expect().
		Status(http.StatusOK).
		JSON().Object()

	tombstones := syncResp.Value("tombstones").Array()
	tombstones.Length().IsEqual(1)
	tombstones.Value(0).Object().HasValue("note_id", noteID)
}

// TestPurgeNoteNotFound ensures DELETE purge on an unknown UUID returns 404.
func TestPurgeNoteNotFound(t *testing.T) {
	srv, mock, _ := newTestServer(t)
	e := newExpect(t, srv)

	token := registerAndLogin(t, e, mock, uuid.NewString())

	e.DELETE("/api/v1/notes/"+uuid.New().String()+"/purge").
		WithHeader("Authorization", "Bearer "+token).
		Expect().
		Status(http.StatusNotFound).
		JSON().Object().HasValue("code", "NOT_FOUND")
}

// TestDeltaSyncPicksUpTrash verifies that a delta sync (GET /notes?since=T0) includes the
// trashed note (with non-null trashed_at) and no tombstones.
func TestDeltaSyncPicksUpTrash(t *testing.T) {
	srv, mock, _ := newTestServer(t)
	e := newExpect(t, srv)

	token := registerAndLogin(t, e, mock, uuid.NewString())
	noteID := createNote(t, e, token)

	// Record a sync baseline timestamp before the trash operation.
	t0 := time.Now().UTC().Add(-time.Second)

	e.DELETE("/api/v1/notes/"+noteID).
		WithHeader("Authorization", "Bearer "+token).
		Expect().
		Status(http.StatusNoContent)

	// Delta sync since t0 must include the note with trashed_at set.
	syncResp := e.GET("/api/v1/notes").
		WithHeader("Authorization", "Bearer "+token).
		WithQuery("since", t0.Format(time.RFC3339Nano)).
		Expect().
		Status(http.StatusOK).
		JSON().Object()

	notes := syncResp.Value("notes").Array()
	notes.Length().IsEqual(1)
	notes.Value(0).Object().HasValue("note_id", noteID)
	notes.Value(0).Object().Value("trashed_at").NotNull()

	// No tombstones — the note was only soft-deleted.
	syncResp.Value("tombstones").Array().Length().IsEqual(0)
}

// TestDeltaSyncPicksUpRestore verifies that after restoring a note a delta sync returns the
// note with trashed_at: null.
func TestDeltaSyncPicksUpRestore(t *testing.T) {
	srv, mock, _ := newTestServer(t)
	e := newExpect(t, srv)

	token := registerAndLogin(t, e, mock, uuid.NewString())
	noteID := createNote(t, e, token)

	// Trash the note, then record T1 and restore.
	e.DELETE("/api/v1/notes/"+noteID).
		WithHeader("Authorization", "Bearer "+token).
		Expect().
		Status(http.StatusNoContent)

	t1 := time.Now().UTC().Add(-time.Millisecond)

	e.POST("/api/v1/notes/"+noteID+"/restore").
		WithHeader("Authorization", "Bearer "+token).
		Expect().
		Status(http.StatusOK)

	// Delta sync since T1 must include the note with trashed_at: null.
	syncResp := e.GET("/api/v1/notes").
		WithHeader("Authorization", "Bearer "+token).
		WithQuery("since", t1.Format(time.RFC3339Nano)).
		Expect().
		Status(http.StatusOK).
		JSON().Object()

	notes := syncResp.Value("notes").Array()
	notes.Length().IsEqual(1)
	notes.Value(0).Object().HasValue("note_id", noteID)
	notes.Value(0).Object().Value("trashed_at").IsNull()
}

// TestDeltaSyncPicksUpPurge verifies that after purging a note a delta sync returns no notes
// for that ID but includes a tombstone.
func TestDeltaSyncPicksUpPurge(t *testing.T) {
	srv, mock, _ := newTestServer(t)
	e := newExpect(t, srv)

	token := registerAndLogin(t, e, mock, uuid.NewString())
	noteID := createNote(t, e, token)

	// Record baseline before purge.
	t0 := time.Now().UTC().Add(-time.Second)

	// Trash then purge.
	e.DELETE("/api/v1/notes/"+noteID).
		WithHeader("Authorization", "Bearer "+token).
		Expect().
		Status(http.StatusNoContent)

	e.DELETE("/api/v1/notes/"+noteID+"/purge").
		WithHeader("Authorization", "Bearer "+token).
		Expect().
		Status(http.StatusNoContent)

	// Delta sync since T0: the note must not appear in notes, but a tombstone must exist.
	syncResp := e.GET("/api/v1/notes").
		WithHeader("Authorization", "Bearer "+token).
		WithQuery("since", t0.Format(time.RFC3339Nano)).
		Expect().
		Status(http.StatusOK).
		JSON().Object()

	// The purged note must not appear in the notes list.
	notes := syncResp.Value("notes").Array()
	notes.Length().IsEqual(0)

	// A tombstone for the purged note must be present.
	tombstones := syncResp.Value("tombstones").Array()
	tombstones.Length().IsEqual(1)
	tombstones.Value(0).Object().HasValue("note_id", noteID)
}

// TestUserCannotTrashOtherUsersNote ensures User B receives 404 when attempting to trash a
// note that belongs to User A.
func TestUserCannotTrashOtherUsersNote(t *testing.T) {
	srv, mock, _ := newTestServer(t)
	e := newExpect(t, srv)

	tokenA := registerAndLogin(t, e, mock, uuid.NewString())
	noteID := createNote(t, e, tokenA)

	tokenB := registerAndLogin(t, e, mock, uuid.NewString())

	e.DELETE("/api/v1/notes/"+noteID).
		WithHeader("Authorization", "Bearer "+tokenB).
		Expect().
		Status(http.StatusNotFound).
		JSON().Object().HasValue("code", "NOT_FOUND")
}

// TestUserCannotPurgeOtherUsersNote ensures User B receives 404 when attempting to purge a
// note that belongs to User A.
func TestUserCannotPurgeOtherUsersNote(t *testing.T) {
	srv, mock, _ := newTestServer(t)
	e := newExpect(t, srv)

	tokenA := registerAndLogin(t, e, mock, uuid.NewString())
	noteID := createNote(t, e, tokenA)

	tokenB := registerAndLogin(t, e, mock, uuid.NewString())

	e.DELETE("/api/v1/notes/"+noteID+"/purge").
		WithHeader("Authorization", "Bearer "+tokenB).
		Expect().
		Status(http.StatusNotFound).
		JSON().Object().HasValue("code", "NOT_FOUND")
}

// TestUserCannotRestoreOtherUsersNote ensures User B receives 404 when attempting to restore
// a trashed note that belongs to User A.
func TestUserCannotRestoreOtherUsersNote(t *testing.T) {
	srv, mock, _ := newTestServer(t)
	e := newExpect(t, srv)

	tokenA := registerAndLogin(t, e, mock, uuid.NewString())
	noteID := createNote(t, e, tokenA)

	// Trash with User A's token first.
	e.DELETE("/api/v1/notes/"+noteID).
		WithHeader("Authorization", "Bearer "+tokenA).
		Expect().
		Status(http.StatusNoContent)

	tokenB := registerAndLogin(t, e, mock, uuid.NewString())

	// User B must not be able to restore User A's note.
	e.POST("/api/v1/notes/"+noteID+"/restore").
		WithHeader("Authorization", "Bearer "+tokenB).
		Expect().
		Status(http.StatusNotFound).
		JSON().Object().HasValue("code", "NOT_FOUND")
}
