package e2e_test

import (
	"net/http"
	"testing"

	"github.com/google/uuid"
)

// TestGetMe_ReturnsUserProfile verifies that a freshly registered user can call GET /users/me
// and receive their id, name, and email. activated_at must be null because the user has not
// activated their account yet.
func TestGetMe_ReturnsUserProfile(t *testing.T) {
	srv, mock, _ := newTestServer(t)
	e := newExpect(t, srv)

	token := registerAndLogin(t, e, mock, uuid.NewString())

	obj := e.GET("/api/v1/users/me").
		WithHeader("Authorization", "Bearer "+token).
		Expect().
		Status(http.StatusOK).
		JSON().Object()

	obj.ContainsKey("id")
	obj.ContainsKey("name")
	obj.ContainsKey("email")
	obj.Value("activated_at").IsNull()
}

// TestGetMe_RequiresAuth verifies that GET /users/me returns 401 when no Authorization header
// is provided. A baseline request with a valid token must return 200 first to prove the
// handler is functional, not simply broken.
func TestGetMe_RequiresAuth(t *testing.T) {
	srv, mock, _ := newTestServer(t)
	e := newExpect(t, srv)

	// Baseline: with a valid token the endpoint must be reachable.
	token := registerAndLogin(t, e, mock, uuid.NewString())
	e.GET("/api/v1/users/me").
		WithHeader("Authorization", "Bearer "+token).
		Expect().
		Status(http.StatusOK)

	// Without a token the endpoint must reject the request.
	e.GET("/api/v1/users/me").
		Expect().
		Status(http.StatusUnauthorized)
}

// TestActivate_SetsActivatedAt verifies that calling POST /users/me/activate sets activated_at
// on the user. After activation, GET /users/me must return a non-null activated_at.
func TestActivate_SetsActivatedAt(t *testing.T) {
	srv, mock, _ := newTestServer(t)
	e := newExpect(t, srv)

	token := registerAndLogin(t, e, mock, uuid.NewString())

	// activated_at must be null before activation.
	e.GET("/api/v1/users/me").
		WithHeader("Authorization", "Bearer "+token).
		Expect().
		Status(http.StatusOK).
		JSON().Object().
		Value("activated_at").IsNull()

	// Activate the account.
	e.POST("/api/v1/users/me/activate").
		WithHeader("Authorization", "Bearer "+token).
		Expect().
		Status(http.StatusNoContent)

	// activated_at must now be set.
	e.GET("/api/v1/users/me").
		WithHeader("Authorization", "Bearer "+token).
		Expect().
		Status(http.StatusOK).
		JSON().Object().
		Value("activated_at").NotNull()
}

// TestActivate_IsIdempotent verifies that calling POST /users/me/activate twice both return
// 204 and that activated_at remains non-null after the second call.
func TestActivate_IsIdempotent(t *testing.T) {
	srv, mock, _ := newTestServer(t)
	e := newExpect(t, srv)

	token := registerAndLogin(t, e, mock, uuid.NewString())

	// First activation.
	e.POST("/api/v1/users/me/activate").
		WithHeader("Authorization", "Bearer "+token).
		Expect().
		Status(http.StatusNoContent)

	// Second activation — must also return 204.
	e.POST("/api/v1/users/me/activate").
		WithHeader("Authorization", "Bearer "+token).
		Expect().
		Status(http.StatusNoContent)

	// activated_at must still be set after the idempotent second call.
	e.GET("/api/v1/users/me").
		WithHeader("Authorization", "Bearer "+token).
		Expect().
		Status(http.StatusOK).
		JSON().Object().
		Value("activated_at").NotNull()
}

// TestActivate_RequiresAuth verifies that POST /users/me/activate returns 401 when no
// Authorization header is provided. A baseline request with a valid token must return 204
// first to prove the handler is functional.
func TestActivate_RequiresAuth(t *testing.T) {
	srv, mock, _ := newTestServer(t)
	e := newExpect(t, srv)

	// Baseline: with a valid token activation must succeed.
	token := registerAndLogin(t, e, mock, uuid.NewString())
	e.POST("/api/v1/users/me/activate").
		WithHeader("Authorization", "Bearer "+token).
		Expect().
		Status(http.StatusNoContent)

	// Without a token the endpoint must reject the request.
	e.POST("/api/v1/users/me/activate").
		Expect().
		Status(http.StatusUnauthorized)
}

// TestMagicLink_SendsNewUserEmail_WhenNotActivated verifies that CreateMagicLink passes
// isNewUser=true when the user has not yet activated, and isNewUser=false after activation.
// This ensures the correct email variant (welcome vs. returning user) is sent at each stage.
func TestMagicLink_SendsNewUserEmail_WhenNotActivated(t *testing.T) {
	srv, mock, _ := newTestServer(t)
	e := newExpect(t, srv)

	suffix := uuid.NewString()
	email := "user-" + suffix + "@example.com"

	// Register the user — this issues a magic link with isNewUser=true (captured by mock
	// but not asserted here; that invariant is implied by the registration flow).
	token := registerAndLogin(t, e, mock, suffix)

	// Request another magic link while the user is not yet activated.
	// The service must pass isNewUser=true because ActivatedAt is still nil.
	e.POST("/api/v1/auth/magic-link").
		WithJSON(map[string]string{"email": email}).
		Expect().
		Status(http.StatusOK)

	if !mock.lastIsNewUser() {
		t.Fatal("expected isNewUser=true when requesting magic link for non-activated user")
	}

	// Activate the user.
	e.POST("/api/v1/users/me/activate").
		WithHeader("Authorization", "Bearer "+token).
		Expect().
		Status(http.StatusNoContent)

	// Request a magic link again — now ActivatedAt is set so isNewUser must be false.
	e.POST("/api/v1/auth/magic-link").
		WithJSON(map[string]string{"email": email}).
		Expect().
		Status(http.StatusOK)

	if mock.lastIsNewUser() {
		t.Fatal("expected isNewUser=false when requesting magic link for an activated user")
	}
}
