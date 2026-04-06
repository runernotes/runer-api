package e2e_test

import (
	"encoding/base64"
	"fmt"
	"net/http"
	"testing"

	"github.com/google/uuid"
)

// TestBetaPlan_NewUserHasBetaPlanByDefault verifies that a freshly registered user
// is assigned the beta plan automatically, without any explicit upgrade.
// Beta users must see plan="beta" and note_limit=null (unlimited) from GET /subscription.
func TestBetaPlan_NewUserHasBetaPlanByDefault(t *testing.T) {
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

// TestBetaPlan_UnlimitedNotes verifies that a beta user can create more notes than
// the free-plan limit (3 in test config) without receiving QUOTA_EXCEEDED.
// This confirms that beta behaves identically to pro for quota purposes.
func TestBetaPlan_UnlimitedNotes(t *testing.T) {
	srv, mock, _ := newTestServer(t)
	e := newExpect(t, srv)

	// New users default to beta — no plan change needed.
	token := registerAndLogin(t, e, mock, uuid.NewString())

	// Create more notes than the free-plan limit — all must succeed.
	for i := 0; i < 5; i++ {
		createNote(t, e, token)
	}

	// Subscription must reflect the correct note count with no limit.
	e.GET("/api/v1/subscription").
		WithHeader("Authorization", "Bearer "+token).
		Expect().
		Status(http.StatusOK).
		JSON().Object().
		HasValue("plan", "beta").
		HasValue("note_count", 5).
		Value("note_limit").IsNull()
}

// TestBetaPlan_FreeUserUnaffected verifies that explicitly downgrading a user to the
// free plan still enforces the note quota. Beta being the default must not change the
// semantics of the free plan for users who are on it.
func TestBetaPlan_FreeUserUnaffected(t *testing.T) {
	srv, mock, db := newTestServer(t)
	e := newExpect(t, srv)

	suffix := uuid.NewString()
	email := fmt.Sprintf("user-%s@example.com", suffix)
	token := registerAndLogin(t, e, mock, suffix)

	// Explicitly set this user to free to verify free-plan behaviour is unchanged.
	userID := getUserIDByEmail(t, db, email)
	setUserPlan(t, db, userID, "free")

	// Subscription must report plan=free and a numeric limit.
	e.GET("/api/v1/subscription").
		WithHeader("Authorization", "Bearer "+token).
		Expect().
		Status(http.StatusOK).
		JSON().Object().
		HasValue("plan", "free").
		HasValue("note_limit", 3)

	// Fill up to the limit.
	createNote(t, e, token)
	createNote(t, e, token)
	createNote(t, e, token)

	// The next note must be rejected.
	noteID := uuid.New().String()
	payload := base64.StdEncoding.EncodeToString([]byte("over-limit"))

	e.PUT("/api/v1/notes/"+noteID).
		WithHeader("Authorization", "Bearer "+token).
		WithJSON(map[string]any{"encrypted_payload": payload}).
		Expect().
		Status(http.StatusForbidden).
		JSON().Object().
		HasValue("code", "QUOTA_EXCEEDED")
}

// TestBetaPlan_SubscriptionReflectsBetaPlan verifies that GET /subscription returns
// "plan": "beta" for a beta user, distinguishing them from both free and pro users.
func TestBetaPlan_SubscriptionReflectsBetaPlan(t *testing.T) {
	srv, mock, db := newTestServer(t)
	e := newExpect(t, srv)

	suffix := uuid.NewString()
	email := fmt.Sprintf("user-%s@example.com", suffix)
	token := registerAndLogin(t, e, mock, suffix)

	// Verify the plan is reported as beta out of the box.
	userID := getUserIDByEmail(t, db, email)
	_ = userID // confirm the user exists in the database

	e.GET("/api/v1/subscription").
		WithHeader("Authorization", "Bearer "+token).
		Expect().
		Status(http.StatusOK).
		JSON().Object().
		HasValue("plan", "beta").
		Value("note_limit").IsNull()
}

// TestBetaPlan_UpgradeToProFromBeta verifies that a beta user can be upgraded to pro
// and that the subscription endpoint reflects the change correctly.
// This mirrors the future migration path described in the spec.
func TestBetaPlan_UpgradeToProFromBeta(t *testing.T) {
	srv, mock, db := newTestServer(t)
	e := newExpect(t, srv)

	suffix := uuid.NewString()
	email := fmt.Sprintf("user-%s@example.com", suffix)
	token := registerAndLogin(t, e, mock, suffix)

	// Confirm beta plan on registration.
	e.GET("/api/v1/subscription").
		WithHeader("Authorization", "Bearer "+token).
		Expect().
		Status(http.StatusOK).
		JSON().Object().
		HasValue("plan", "beta")

	// Simulate the migration: upgrade beta → pro.
	userID := getUserIDByEmail(t, db, email)
	setUserPlan(t, db, userID, "pro")

	// Subscription must now show pro with no limit.
	e.GET("/api/v1/subscription").
		WithHeader("Authorization", "Bearer "+token).
		Expect().
		Status(http.StatusOK).
		JSON().Object().
		HasValue("plan", "pro").
		Value("note_limit").IsNull()
}

// TestBetaPlan_DowngradeToFreeFromBeta verifies that a beta user can be downgraded
// to free and that quota enforcement kicks in immediately after the change.
// This mirrors the future migration path where the beta period ends.
func TestBetaPlan_DowngradeToFreeFromBeta(t *testing.T) {
	srv, mock, db := newTestServer(t)
	e := newExpect(t, srv)

	suffix := uuid.NewString()
	email := fmt.Sprintf("user-%s@example.com", suffix)
	token := registerAndLogin(t, e, mock, suffix)

	// Create notes freely while on beta (no quota).
	createNote(t, e, token)
	createNote(t, e, token)
	createNote(t, e, token)

	// Simulate end of beta: downgrade beta → free.
	userID := getUserIDByEmail(t, db, email)
	setUserPlan(t, db, userID, "free")

	// Subscription must now show free with a numeric limit.
	e.GET("/api/v1/subscription").
		WithHeader("Authorization", "Bearer "+token).
		Expect().
		Status(http.StatusOK).
		JSON().Object().
		HasValue("plan", "free").
		HasValue("note_limit", 3)

	// The user is already at (or over) the limit — a new note must be rejected.
	noteID := uuid.New().String()
	payload := base64.StdEncoding.EncodeToString([]byte("post-beta"))

	e.PUT("/api/v1/notes/"+noteID).
		WithHeader("Authorization", "Bearer "+token).
		WithJSON(map[string]any{"encrypted_payload": payload}).
		Expect().
		Status(http.StatusForbidden).
		JSON().Object().
		HasValue("code", "QUOTA_EXCEEDED")
}
