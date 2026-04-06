package e2e_test

import (
	"encoding/base64"
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/google/uuid"
)

// TestQuota_FreeUser_ExceedsLimit verifies that a free user who has reached the
// note limit (3 in test config) receives 403 QUOTA_EXCEEDED on the next create,
// while the note at exactly the limit succeeds.
// New registrations default to beta (unlimited), so the user is explicitly
// downgraded to free before the quota behaviour is exercised.
func TestQuota_FreeUser_ExceedsLimit(t *testing.T) {
	srv, mock, db := newTestServer(t)
	e := newExpect(t, srv)

	suffix := uuid.NewString()
	email := fmt.Sprintf("user-%s@example.com", suffix)
	token := registerAndLogin(t, e, mock, suffix)

	// Downgrade from the default beta plan to free so quota limits apply.
	userID := getUserIDByEmail(t, db, email)
	setUserPlan(t, db, userID, "free")

	// Create notes 1, 2, 3 — all should succeed (limit is 3).
	createNote(t, e, token)
	createNote(t, e, token)
	createNote(t, e, token) // exactly at limit — must succeed

	// The 4th note must be rejected.
	noteID := uuid.New().String()
	payload := base64.StdEncoding.EncodeToString([]byte("hello"))

	e.PUT("/api/v1/notes/"+noteID).
		WithHeader("Authorization", "Bearer "+token).
		WithJSON(map[string]any{"encrypted_payload": payload}).
		Expect().
		Status(http.StatusForbidden).
		JSON().Object().
		HasValue("code", "QUOTA_EXCEEDED")
}

// TestQuota_TrashedNotesDoNotCount verifies that trashing a note frees up quota,
// allowing a new note to be created while staying within the limit.
// New registrations default to beta (unlimited), so the user is explicitly
// downgraded to free before the quota behaviour is exercised.
func TestQuota_TrashedNotesDoNotCount(t *testing.T) {
	srv, mock, db := newTestServer(t)
	e := newExpect(t, srv)

	suffix := uuid.NewString()
	email := fmt.Sprintf("user-%s@example.com", suffix)
	token := registerAndLogin(t, e, mock, suffix)

	// Downgrade from the default beta plan to free so quota limits apply.
	userID := getUserIDByEmail(t, db, email)
	setUserPlan(t, db, userID, "free")

	// Create 3 notes — at the limit.
	note1 := createNote(t, e, token)
	createNote(t, e, token)
	createNote(t, e, token)

	// Trash one note to bring the live count back below the limit.
	e.DELETE("/api/v1/notes/"+note1).
		WithHeader("Authorization", "Bearer "+token).
		Expect().
		Status(http.StatusNoContent)

	// Creating another note must now succeed (2 live notes < limit of 3).
	createNote(t, e, token)
}

// TestQuota_ProUser_NoLimit verifies that a pro user can create notes beyond the
// free plan limit without encountering QUOTA_EXCEEDED.
func TestQuota_ProUser_NoLimit(t *testing.T) {
	srv, mock, db := newTestServer(t)
	e := newExpect(t, srv)

	suffix := uuid.NewString()
	email := fmt.Sprintf("user-%s@example.com", suffix)
	token := registerAndLogin(t, e, mock, suffix)

	userID := getUserIDByEmail(t, db, email)
	setUserPlan(t, db, userID, "pro")

	// Create more notes than the free limit — all must succeed.
	for i := 0; i < 5; i++ {
		createNote(t, e, token)
	}
}

// TestQuota_UpdateWithBaseVersion_SkipsCheck verifies that updating an existing note
// using base_version does not trigger a quota check, even when the user is at the limit.
// New registrations default to beta (unlimited), so the user is explicitly
// downgraded to free before the quota behaviour is exercised.
func TestQuota_UpdateWithBaseVersion_SkipsCheck(t *testing.T) {
	srv, mock, db := newTestServer(t)
	e := newExpect(t, srv)

	suffix := uuid.NewString()
	email := fmt.Sprintf("user-%s@example.com", suffix)
	token := registerAndLogin(t, e, mock, suffix)

	// Downgrade from the default beta plan to free so quota limits apply.
	userID := getUserIDByEmail(t, db, email)
	setUserPlan(t, db, userID, "free")

	// Create 3 notes — at the limit.
	noteID := createNote(t, e, token)
	createNote(t, e, token)
	createNote(t, e, token)

	// Fetch the current updated_at for noteID to use as base_version.
	noteResp := e.GET("/api/v1/notes/"+noteID).
		WithHeader("Authorization", "Bearer "+token).
		Expect().
		Status(http.StatusOK).
		JSON().Object()

	baseVersion := noteResp.Value("updated_at").String().NotEmpty().Raw()

	// Update the existing note with base_version — must succeed despite being at quota.
	payload := base64.StdEncoding.EncodeToString([]byte("updated"))
	e.PUT("/api/v1/notes/"+noteID).
		WithHeader("Authorization", "Bearer "+token).
		WithJSON(map[string]any{
			"encrypted_payload": payload,
			"base_version":      baseVersion,
		}).
		Expect().
		Status(http.StatusOK)
}

// TestQuota_RePush_SkipsCheck verifies that re-pushing a note (PUT without base_version
// to an existing note_id) does not trigger a quota check when the user is at the limit.
// New registrations default to beta (unlimited), so the user is explicitly
// downgraded to free before the quota behaviour is exercised.
func TestQuota_RePush_SkipsCheck(t *testing.T) {
	srv, mock, db := newTestServer(t)
	e := newExpect(t, srv)

	suffix := uuid.NewString()
	email := fmt.Sprintf("user-%s@example.com", suffix)
	token := registerAndLogin(t, e, mock, suffix)

	// Downgrade from the default beta plan to free so quota limits apply.
	userID := getUserIDByEmail(t, db, email)
	setUserPlan(t, db, userID, "free")

	// Create 3 notes — at the limit.
	noteID := createNote(t, e, token)
	createNote(t, e, token)
	createNote(t, e, token)

	// Brief sleep to ensure updated_at changes on the re-push.
	time.Sleep(5 * time.Millisecond)

	// Re-push the first note without base_version — must succeed (not a new note).
	payload := base64.StdEncoding.EncodeToString([]byte("re-pushed"))
	e.PUT("/api/v1/notes/"+noteID).
		WithHeader("Authorization", "Bearer "+token).
		WithJSON(map[string]any{"encrypted_payload": payload}).
		Expect().
		Status(http.StatusOK)
}
