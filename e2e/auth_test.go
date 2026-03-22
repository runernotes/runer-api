package e2e_test

import (
	"net/http"
	"testing"

	"github.com/google/uuid"
)

// TestRegisterValid verifies that a well-formed registration request returns 200
// and causes the mock email sender to capture a magic link token.
func TestRegisterValid(t *testing.T) {

	srv, mockEmail, _ := newTestServer(t)
	e := newExpect(t, srv)

	email := "alice-" + uuid.NewString() + "@example.com"

	e.POST("/api/v1/auth/register").
		WithJSON(map[string]string{
			"email": email,
			"name":  "Alice",
		}).
		Expect().
		Status(http.StatusOK).
		JSON().Object().
		ContainsKey("message")

	token := mockEmail.lastToken()
	if token == "" {
		t.Fatal("expected mock to capture a magic link token after register, got empty string")
	}
}

// TestRegisterInvalidEmail verifies that a registration request with a malformed email
// returns 400 with VALIDATION_ERROR and does not send a magic link.
func TestRegisterInvalidEmail(t *testing.T) {

	srv, mockEmail, _ := newTestServer(t)
	e := newExpect(t, srv)

	e.POST("/api/v1/auth/register").
		WithJSON(map[string]string{"email": "not-an-email", "name": "Alice"}).
		Expect().
		Status(http.StatusBadRequest).
		JSON().Object().
		HasValue("code", "VALIDATION_ERROR")

	if mockEmail.callCount() != 0 {
		t.Errorf("expected 0 magic links sent, got %d", mockEmail.callCount())
	}
}

// TestRegisterMissingName verifies that a registration request without a name field
// returns 400 with VALIDATION_ERROR and does not send a magic link.
func TestRegisterMissingName(t *testing.T) {

	srv, mockEmail, _ := newTestServer(t)
	e := newExpect(t, srv)

	email := "bob-" + uuid.NewString() + "@example.com"

	e.POST("/api/v1/auth/register").
		WithJSON(map[string]string{"email": email}).
		Expect().
		Status(http.StatusBadRequest).
		JSON().Object().
		HasValue("code", "VALIDATION_ERROR")

	if mockEmail.callCount() != 0 {
		t.Errorf("expected 0 magic links sent, got %d", mockEmail.callCount())
	}
}

// TestRegisterEmptyBody verifies that a registration request with an empty JSON object
// returns 400 with VALIDATION_ERROR and does not send a magic link.
func TestRegisterEmptyBody(t *testing.T) {

	srv, mockEmail, _ := newTestServer(t)
	e := newExpect(t, srv)

	e.POST("/api/v1/auth/register").
		WithJSON(map[string]string{}).
		Expect().
		Status(http.StatusBadRequest).
		JSON().Object().
		HasValue("code", "VALIDATION_ERROR")

	if mockEmail.callCount() != 0 {
		t.Errorf("expected 0 magic links sent, got %d", mockEmail.callCount())
	}
}

// TestRegisterDuplicateEmail verifies that registering the same email twice returns 200
// on both attempts (no user enumeration) but only issues a magic link for the first.
func TestRegisterDuplicateEmail(t *testing.T) {

	srv, mockEmail, _ := newTestServer(t)
	e := newExpect(t, srv)

	payload := map[string]string{
		"email": "duplicate-" + uuid.NewString() + "@example.com",
		"name":  "Duplicate",
	}

	e.POST("/api/v1/auth/register").
		WithJSON(payload).
		Expect().
		Status(http.StatusOK)

	firstToken := mockEmail.lastToken()
	if firstToken == "" {
		t.Fatal("expected magic link token after first registration")
	}

	// Second registration with the same email must still return 200 (no enumeration).
	e.POST("/api/v1/auth/register").
		WithJSON(payload).
		Expect().
		Status(http.StatusOK)

	// No new token should have been issued.
	if mockEmail.lastToken() != firstToken {
		t.Error("expected no new magic link token for duplicate registration")
	}
}

// TestLoginLogoutLifecycle covers the full auth flow end-to-end:
// register → verify magic link → access protected endpoint → logout → refresh fails.
func TestLoginLogoutLifecycle(t *testing.T) {

	srv, mockEmail, _ := newTestServer(t)
	e := newExpect(t, srv)

	email := "lifecycle-" + uuid.NewString() + "@example.com"

	// Register.
	e.POST("/api/v1/auth/register").
		WithJSON(map[string]string{
			"email": email,
			"name":  "Lifecycle",
		}).
		Expect().
		Status(http.StatusOK)

	token := mockEmail.lastToken()
	if token == "" {
		t.Fatal("expected mock to capture a magic link token after register, got empty string")
	}

	// Verify magic link → login.
	loginResp := e.POST("/api/v1/auth/verify").
		WithJSON(map[string]string{"token": token}).
		Expect().
		Status(http.StatusOK).
		JSON().Object()

	accessToken := loginResp.Value("access_token").String().NotEmpty().Raw()
	refreshToken := loginResp.Value("refresh_token").String().NotEmpty().Raw()

	// Access a protected endpoint.
	e.GET("/api/v1/notes").
		WithHeader("Authorization", "Bearer "+accessToken).
		Expect().
		Status(http.StatusOK)

	// Logout.
	e.POST("/api/v1/auth/logout").
		WithHeader("Authorization", "Bearer "+accessToken).
		WithJSON(map[string]string{"refresh_token": refreshToken}).
		Expect().
		Status(http.StatusNoContent)

	// Refresh with the now-revoked refresh token must fail.
	e.POST("/api/v1/auth/refresh").
		WithJSON(map[string]string{"refresh_token": refreshToken}).
		Expect().
		Status(http.StatusUnauthorized)
}

// TestLogoutValid verifies that a properly authenticated logout with a valid refresh
// token returns 204.
func TestLogoutValid(t *testing.T) {

	srv, mockEmail, _ := newTestServer(t)
	e := newExpect(t, srv)

	suffix := uuid.NewString()
	email := "user-" + suffix + "@example.com"

	e.POST("/api/v1/auth/register").
		WithJSON(map[string]string{"email": email, "name": "Logout Happy Path"}).
		Expect().
		Status(http.StatusOK)

	magicToken := mockEmail.lastToken()
	if magicToken == "" {
		t.Fatal("no magic link token captured")
	}

	resp := e.POST("/api/v1/auth/verify").
		WithJSON(map[string]string{"token": magicToken}).
		Expect().
		Status(http.StatusOK).
		JSON().Object()

	accessToken := resp.Value("access_token").String().NotEmpty().Raw()
	refreshToken := resp.Value("refresh_token").String().NotEmpty().Raw()

	e.POST("/api/v1/auth/logout").
		WithHeader("Authorization", "Bearer "+accessToken).
		WithJSON(map[string]string{"refresh_token": refreshToken}).
		Expect().
		Status(http.StatusNoContent)
}

// TestLogoutNoAuthHeader verifies that calling logout without an Authorization header
// returns 401 with UNAUTHORIZED.
func TestLogoutNoAuthHeader(t *testing.T) {

	srv, mockEmail, _ := newTestServer(t)
	e := newExpect(t, srv)

	suffix := uuid.NewString()
	email := "user-" + suffix + "@example.com"

	e.POST("/api/v1/auth/register").
		WithJSON(map[string]string{"email": email, "name": "Logout No Auth"}).
		Expect().
		Status(http.StatusOK)

	magicToken := mockEmail.lastToken()
	if magicToken == "" {
		t.Fatal("no magic link token captured")
	}

	resp := e.POST("/api/v1/auth/verify").
		WithJSON(map[string]string{"token": magicToken}).
		Expect().
		Status(http.StatusOK).
		JSON().Object()

	refreshToken := resp.Value("refresh_token").String().NotEmpty().Raw()

	e.POST("/api/v1/auth/logout").
		WithJSON(map[string]string{"refresh_token": refreshToken}).
		Expect().
		Status(http.StatusUnauthorized).
		JSON().Object().
		HasValue("code", "UNAUTHORIZED")
}

// TestLogoutMissingRefreshTokenField verifies that calling logout with a valid access token
// but without a refresh_token field in the body returns 400 with VALIDATION_ERROR.
func TestLogoutMissingRefreshTokenField(t *testing.T) {

	srv, mockEmail, _ := newTestServer(t)
	e := newExpect(t, srv)

	suffix := uuid.NewString()
	email := "user-" + suffix + "@example.com"

	e.POST("/api/v1/auth/register").
		WithJSON(map[string]string{"email": email, "name": "Logout Missing Field"}).
		Expect().
		Status(http.StatusOK)

	magicToken := mockEmail.lastToken()
	if magicToken == "" {
		t.Fatal("no magic link token captured")
	}

	resp := e.POST("/api/v1/auth/verify").
		WithJSON(map[string]string{"token": magicToken}).
		Expect().
		Status(http.StatusOK).
		JSON().Object()

	accessToken := resp.Value("access_token").String().NotEmpty().Raw()

	e.POST("/api/v1/auth/logout").
		WithHeader("Authorization", "Bearer "+accessToken).
		WithJSON(map[string]string{}).
		Expect().
		Status(http.StatusBadRequest).
		JSON().Object().
		HasValue("code", "VALIDATION_ERROR")
}

// TestLogoutInvalidRefreshTokenReturns204 verifies that the server does not leak token
// validity information — even an invalid refresh token causes logout to return 204.
func TestLogoutInvalidRefreshTokenReturns204(t *testing.T) {

	srv, mockEmail, _ := newTestServer(t)
	e := newExpect(t, srv)

	suffix := uuid.NewString()
	email := "user-" + suffix + "@example.com"

	e.POST("/api/v1/auth/register").
		WithJSON(map[string]string{"email": email, "name": "Logout Invalid Token"}).
		Expect().
		Status(http.StatusOK)

	magicToken := mockEmail.lastToken()
	if magicToken == "" {
		t.Fatal("no magic link token captured")
	}

	resp := e.POST("/api/v1/auth/verify").
		WithJSON(map[string]string{"token": magicToken}).
		Expect().
		Status(http.StatusOK).
		JSON().Object()

	accessToken := resp.Value("access_token").String().NotEmpty().Raw()

	// The server must not leak token validity information.
	e.POST("/api/v1/auth/logout").
		WithHeader("Authorization", "Bearer "+accessToken).
		WithJSON(map[string]string{"refresh_token": "not-a-real-refresh-token"}).
		Expect().
		Status(http.StatusNoContent)
}
