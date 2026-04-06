package e2e_test

import (
	"fmt"
	"net/http"
	"testing"

	"github.com/google/uuid"
	"github.com/runernotes/runer-api/internal/users"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// setUserPlan directly updates the plan column for the given user in the database.
// This simulates an admin or billing webhook change without an API endpoint.
func setUserPlan(t *testing.T, db *gorm.DB, userID uuid.UUID, plan string) {
	t.Helper()
	err := db.Model(&users.User{}).Where("id = ?", userID).Update("plan", plan).Error
	require.NoError(t, err)
}

// getUserIDByEmail queries the database for the user's UUID matching the given email.
func getUserIDByEmail(t *testing.T, db *gorm.DB, email string) uuid.UUID {
	t.Helper()
	var user users.User
	err := db.Where("email = ?", email).First(&user).Error
	require.NoError(t, err)
	return user.ID
}

// TestGetSubscription_NewUser verifies that a freshly registered user has a beta plan
// with note_count 0 and note_limit null (unlimited) — new registrations default to beta.
func TestGetSubscription_NewUser(t *testing.T) {
	srv, mock, _ := newTestServer(t)
	e := newExpect(t, srv)

	token := registerAndLogin(t, e, mock, uuid.NewString())

	e.GET("/api/v1/subscription").
		WithHeader("Authorization", "Bearer "+token).
		Expect().
		Status(http.StatusOK).
		JSON().Object().
		HasValue("plan", "beta").
		HasValue("note_count", 0).
		Value("note_limit").IsNull()
}

// TestGetSubscription_RequiresAuth verifies that GET /subscription without a token
// returns 401, and that the same endpoint with a valid token returns 200.
func TestGetSubscription_RequiresAuth(t *testing.T) {
	srv, mock, _ := newTestServer(t)
	e := newExpect(t, srv)

	// Without token — must be 401.
	e.GET("/api/v1/subscription").
		Expect().
		Status(http.StatusUnauthorized)

	// With valid token — must be 200.
	token := registerAndLogin(t, e, mock, uuid.NewString())
	e.GET("/api/v1/subscription").
		WithHeader("Authorization", "Bearer "+token).
		Expect().
		Status(http.StatusOK)
}

// TestGetSubscription_ProUser_NullLimit verifies that a pro user gets note_limit: null.
func TestGetSubscription_ProUser_NullLimit(t *testing.T) {
	srv, mock, db := newTestServer(t)
	e := newExpect(t, srv)

	suffix := uuid.NewString()
	email := fmt.Sprintf("user-%s@example.com", suffix)
	token := registerAndLogin(t, e, mock, suffix)

	userID := getUserIDByEmail(t, db, email)
	setUserPlan(t, db, userID, "pro")

	e.GET("/api/v1/subscription").
		WithHeader("Authorization", "Bearer "+token).
		Expect().
		Status(http.StatusOK).
		JSON().Object().
		HasValue("plan", "pro").
		HasValue("note_count", 0).
		Value("note_limit").IsNull()
}

// TestGetSubscription_NoteCountReflectsLiveNotes verifies that:
//   - note_count increases as notes are created
//   - trashing a note decreases note_count (trashed notes do not count toward quota)
func TestGetSubscription_NoteCountReflectsLiveNotes(t *testing.T) {
	srv, mock, db := newTestServer(t)
	e := newExpect(t, srv)

	suffix := uuid.NewString()
	email := fmt.Sprintf("user-%s@example.com", suffix)
	token := registerAndLogin(t, e, mock, suffix)

	// Upgrade to pro so we can create notes freely without hitting the quota.
	userID := getUserIDByEmail(t, db, email)
	setUserPlan(t, db, userID, "pro")

	// Create 3 notes.
	note1 := createNote(t, e, token)
	createNote(t, e, token)
	createNote(t, e, token)

	// Subscription must report note_count: 3.
	e.GET("/api/v1/subscription").
		WithHeader("Authorization", "Bearer "+token).
		Expect().
		Status(http.StatusOK).
		JSON().Object().
		HasValue("note_count", 3)

	// Trash one note — note_count must drop to 2.
	e.DELETE("/api/v1/notes/"+note1).
		WithHeader("Authorization", "Bearer "+token).
		Expect().
		Status(http.StatusNoContent)

	e.GET("/api/v1/subscription").
		WithHeader("Authorization", "Bearer "+token).
		Expect().
		Status(http.StatusOK).
		JSON().Object().
		HasValue("note_count", 2)
}
