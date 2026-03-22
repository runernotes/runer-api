package e2e_test

// Auth middleware security tests.

import (
	"net/http"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
)

// TestAuthMiddlewareValidBearerToken verifies that a well-formed, unexpired JWT with
// the correct signing secret is accepted by the auth middleware and returns 200.
func TestAuthMiddlewareValidBearerToken(t *testing.T) {
	srv, mock, _ := newTestServer(t)
	e := newExpect(t, srv)

	validToken := registerAndLogin(t, e, mock, uuid.NewString())

	e.GET("/api/v1/notes").
		WithHeader("Authorization", "Bearer "+validToken).
		Expect().
		Status(http.StatusOK)
}

// TestAuthMiddlewareNoAuthHeader verifies that a request without an Authorization header
// is rejected with 401 and UNAUTHORIZED.
func TestAuthMiddlewareNoAuthHeader(t *testing.T) {
	srv, _, _ := newTestServer(t)
	e := newExpect(t, srv)

	e.GET("/api/v1/notes").
		Expect().
		Status(http.StatusUnauthorized).
		JSON().Object().
		HasValue("code", "UNAUTHORIZED")
}

// TestAuthMiddlewareExpiredJWT verifies that a JWT whose exp claim is in the past is
// rejected with 401 and UNAUTHORIZED, even when the signature is otherwise valid.
func TestAuthMiddlewareExpiredJWT(t *testing.T) {
	srv, _, _ := newTestServer(t)
	e := newExpect(t, srv)

	// Craft a JWT that expired one hour ago using the same secret the test server uses.
	expiredToken := buildExpiredJWT(t, "e2e-test-secret")

	e.GET("/api/v1/notes").
		WithHeader("Authorization", "Bearer "+expiredToken).
		Expect().
		Status(http.StatusUnauthorized).
		JSON().Object().
		HasValue("code", "UNAUTHORIZED")
}

// TestAuthMiddlewareWrongSecret verifies that a JWT signed with an unknown secret is
// rejected with 401 and UNAUTHORIZED.
func TestAuthMiddlewareWrongSecret(t *testing.T) {
	srv, _, _ := newTestServer(t)
	e := newExpect(t, srv)

	wrongSecretToken := buildValidJWTWithSecret(t, "completely-different-secret-value")

	e.GET("/api/v1/notes").
		WithHeader("Authorization", "Bearer "+wrongSecretToken).
		Expect().
		Status(http.StatusUnauthorized).
		JSON().Object().
		HasValue("code", "UNAUTHORIZED")
}

// TestAuthMiddlewareNonBearerScheme verifies that an Authorization header using the
// "Token" scheme instead of "Bearer" is rejected with 401 and UNAUTHORIZED.
func TestAuthMiddlewareNonBearerScheme(t *testing.T) {
	srv, mock, _ := newTestServer(t)
	e := newExpect(t, srv)

	validToken := registerAndLogin(t, e, mock, uuid.NewString())

	// "Token <value>" is not the Bearer scheme; must be rejected.
	e.GET("/api/v1/notes").
		WithHeader("Authorization", "Token "+validToken).
		Expect().
		Status(http.StatusUnauthorized).
		JSON().Object().
		HasValue("code", "UNAUTHORIZED")
}

// buildExpiredJWT creates a JWT signed with the given secret whose exp is one hour in
// the past. This avoids any sleep in the test.
func buildExpiredJWT(t *testing.T, secret string) string {
	t.Helper()

	userID := uuid.New()
	now := time.Now()

	claims := jwt.MapClaims{
		"user_id":    userID.String(),
		"email":      "expired@example.com",
		"token_type": "access",
		"sub":        userID.String(),
		"iss":        "runer-api",
		"iat":        now.Add(-2 * time.Hour).Unix(),
		"nbf":        now.Add(-2 * time.Hour).Unix(),
		"exp":        now.Add(-time.Hour).Unix(), // already expired
	}

	tok := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	signed, err := tok.SignedString([]byte(secret))
	if err != nil {
		t.Fatalf("buildExpiredJWT: sign token: %v", err)
	}

	return signed
}

// buildValidJWTWithSecret creates a non-expired JWT signed with the provided secret.
// Use this to simulate a token signed by an unknown or incorrect signing key.
func buildValidJWTWithSecret(t *testing.T, secret string) string {
	t.Helper()

	userID := uuid.New()
	now := time.Now()

	claims := jwt.MapClaims{
		"user_id":    userID.String(),
		"email":      "attacker@example.com",
		"token_type": "access",
		"sub":        userID.String(),
		"iss":        "runer-api",
		"iat":        now.Unix(),
		"nbf":        now.Unix(),
		"exp":        now.Add(15 * time.Minute).Unix(),
	}

	tok := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	signed, err := tok.SignedString([]byte(secret))
	if err != nil {
		t.Fatalf("buildValidJWTWithSecret: sign token: %v", err)
	}

	return signed
}
