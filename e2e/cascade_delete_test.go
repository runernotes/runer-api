package e2e_test

import (
	"testing"

	"github.com/google/uuid"
	"github.com/runernotes/runer-api/internal/auth"
	"github.com/runernotes/runer-api/internal/notes"
	"github.com/runernotes/runer-api/internal/users"
)

// TestCascadeDeleteUser verifies that deleting a user from the database
// automatically removes all related records (notes, magic link tokens,
// refresh tokens) via ON DELETE CASCADE foreign keys.
func TestCascadeDeleteUser(t *testing.T) {
	srv, mock, db := newTestServer(t)
	e := newExpect(t, srv)

	// 1. Register, login, and create a note — this produces rows in:
	//    - users
	//    - magic_link_tokens
	//    - refresh_tokens
	//    - notes
	accessToken := registerAndLogin(t, e, mock, uuid.NewString())
	createNote(t, e, accessToken)

	// 2. Find the user that was just created.
	var user users.User
	if err := db.First(&user).Error; err != nil {
		t.Fatalf("finding user: %v", err)
	}
	userID := user.ID

	// 3. Verify child rows exist before deletion.
	assertCount := func(t *testing.T, label string, model any, expected int) {
		t.Helper()
		var count int64
		if err := db.Model(model).Where("user_id = ?", userID).Count(&count).Error; err != nil {
			t.Fatalf("counting %s: %v", label, err)
		}
		if int(count) != expected {
			t.Fatalf("%s: expected %d rows, got %d", label, expected, count)
		}
	}

	assertCount(t, "notes (before)", &notes.Note{}, 1)
	assertCount(t, "magic_link_tokens (before)", &auth.MagicLinkToken{}, 1)
	assertCount(t, "refresh_tokens (before)", &auth.RefreshToken{}, 1)

	// 4. Delete the user directly in the database.
	if err := db.Unscoped().Delete(&users.User{}, "id = ?", userID).Error; err != nil {
		t.Fatalf("deleting user: %v", err)
	}

	// 5. Verify all child rows were cascade-deleted.
	assertCount(t, "notes (after)", &notes.Note{}, 0)
	assertCount(t, "magic_link_tokens (after)", &auth.MagicLinkToken{}, 0)
	assertCount(t, "refresh_tokens (after)", &auth.RefreshToken{}, 0)
}
