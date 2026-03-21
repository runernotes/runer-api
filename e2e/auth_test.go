package e2e_test

import (
	"net/http"
	"testing"

	"github.com/gavv/httpexpect/v2"
)

// TestRegisterLoginLogout covers the full auth lifecycle:
//  1. Register a new user → magic link is sent (captured by mock)
//  2. Verify the magic link token → receive access + refresh tokens
//  3. Access a protected endpoint with the access token → 200
//  4. Logout with the refresh token → 204
//  5. Attempt to refresh using the revoked refresh token → 401
func TestRegisterLoginLogout(t *testing.T) {
	srv, mockEmail, _ := newTestServer(t)

	e := httpexpect.WithConfig(httpexpect.Config{
		BaseURL:  srv.URL,
		Reporter: httpexpect.NewRequireReporter(t),
		Client:   srv.Client(),
	})

	// 1. Register
	e.POST("/api/v1/auth/register").
		WithJSON(map[string]string{
			"email": "alice@example.com",
			"name":  "Alice",
		}).
		Expect().
		Status(http.StatusOK).
		JSON().Object().
		ContainsKey("message")

	// 2. Verify magic link → login
	token := mockEmail.lastToken()
	if token == "" {
		t.Fatal("expected mock to capture a magic link token after register, got empty string")
	}

	loginResp := e.POST("/api/v1/auth/verify").
		WithJSON(map[string]string{"token": token}).
		Expect().
		Status(http.StatusOK).
		JSON().Object()

	accessToken := loginResp.Value("access_token").String().NotEmpty().Raw()
	refreshToken := loginResp.Value("refresh_token").String().NotEmpty().Raw()

	// 3. Access a protected endpoint
	e.GET("/api/v1/notes").
		WithHeader("Authorization", "Bearer "+accessToken).
		Expect().
		Status(http.StatusOK)

	// 4. Logout
	e.POST("/api/v1/auth/logout").
		WithHeader("Authorization", "Bearer "+accessToken).
		WithJSON(map[string]string{"refresh_token": refreshToken}).
		Expect().
		Status(http.StatusNoContent)

	// 5. Refresh with the now-revoked refresh token must fail
	e.POST("/api/v1/auth/refresh").
		WithJSON(map[string]string{"refresh_token": refreshToken}).
		Expect().
		Status(http.StatusUnauthorized)
}

// TestRegisterInvalidEmail ensures the validator rejects a malformed email address.
// A valid request is sent first to confirm the server is healthy and the 400 is
// caused specifically by the bad input, not a broken handler.
func TestRegisterInvalidEmail(t *testing.T) {
	srv, mockEmail, _ := newTestServer(t)

	e := httpexpect.WithConfig(httpexpect.Config{
		BaseURL:  srv.URL,
		Reporter: httpexpect.NewRequireReporter(t),
		Client:   srv.Client(),
	})

	// Baseline: a valid request must succeed so we know the handler is working.
	e.POST("/api/v1/auth/register").
		WithJSON(map[string]string{"email": "alice@example.com", "name": "Alice"}).
		Expect().
		Status(http.StatusOK)

	if mockEmail.lastToken() == "" {
		t.Fatal("baseline valid registration did not trigger a magic link")
	}

	// Negative: malformed email must be rejected before the service is reached.
	e.POST("/api/v1/auth/register").
		WithJSON(map[string]string{"email": "not-an-email", "name": "Alice"}).
		Expect().
		Status(http.StatusBadRequest).
		JSON().Object().
		HasValue("code", "VALIDATION_ERROR")

	// The mock must not have been called a second time — validation failed before the service.
	if mockEmail.callCount() != 1 {
		t.Errorf("expected 1 magic link sent (baseline only), got %d", mockEmail.callCount())
	}
}

// TestRegisterMissingName ensures the validator rejects a request with no name field.
// A valid request is sent first to confirm the server is healthy and the 400 is
// caused specifically by the missing field, not a broken handler.
func TestRegisterMissingName(t *testing.T) {
	srv, mockEmail, _ := newTestServer(t)

	e := httpexpect.WithConfig(httpexpect.Config{
		BaseURL:  srv.URL,
		Reporter: httpexpect.NewRequireReporter(t),
		Client:   srv.Client(),
	})

	// Baseline: a valid request must succeed so we know the handler is working.
	e.POST("/api/v1/auth/register").
		WithJSON(map[string]string{"email": "bob@example.com", "name": "Bob"}).
		Expect().
		Status(http.StatusOK)

	if mockEmail.lastToken() == "" {
		t.Fatal("baseline valid registration did not trigger a magic link")
	}

	// Negative: missing name must be rejected before the service is reached.
	e.POST("/api/v1/auth/register").
		WithJSON(map[string]string{"email": "bob2@example.com"}).
		Expect().
		Status(http.StatusBadRequest).
		JSON().Object().
		HasValue("code", "VALIDATION_ERROR")

	// The mock must not have been called a second time — validation failed before the service.
	if mockEmail.callCount() != 1 {
		t.Errorf("expected 1 magic link sent (baseline only), got %d", mockEmail.callCount())
	}
}

// TestRegisterDuplicate ensures that registering with the same email a second time
// returns 200 (to prevent email enumeration) but does NOT send another magic link.
func TestRegisterDuplicate(t *testing.T) {
	srv, mockEmail, _ := newTestServer(t)

	e := httpexpect.WithConfig(httpexpect.Config{
		BaseURL:  srv.URL,
		Reporter: httpexpect.NewRequireReporter(t),
		Client:   srv.Client(),
	})

	payload := map[string]string{
		"email": "bob@example.com",
		"name":  "Bob",
	}

	// First registration — succeeds and sends a magic link.
	e.POST("/api/v1/auth/register").
		WithJSON(payload).
		Expect().
		Status(http.StatusOK)

	firstToken := mockEmail.lastToken()
	if firstToken == "" {
		t.Fatal("expected magic link token after first registration")
	}

	// Second registration with same email — must still return 200 (no enumeration).
	e.POST("/api/v1/auth/register").
		WithJSON(payload).
		Expect().
		Status(http.StatusOK)

	// No new token should have been issued.
	if mockEmail.lastToken() != firstToken {
		t.Error("expected no new magic link token for duplicate registration")
	}
}
