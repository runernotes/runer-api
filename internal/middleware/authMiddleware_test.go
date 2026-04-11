package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/labstack/echo/v5"
	"github.com/runernotes/runer-api/internal/utils"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const authTestSecret = "auth-middleware-test-secret-32bytes!"

// newAuthTestJWTManager returns a JWTManager for use in auth middleware tests.
func newAuthTestJWTManager() *utils.JWTManager {
	return utils.NewJWTManager(authTestSecret, 15*time.Minute, 7*24*time.Hour)
}

// runAuthMiddleware builds an Echo context from req, wraps a no-op next handler
// with AuthMiddleware, and returns the recorder for status/body assertions. The
// capturedUserID pointer is set to the user_id stored in the context if the
// middleware lets the request through.
func runAuthMiddleware(t *testing.T, req *http.Request, jm *utils.JWTManager) (*httptest.ResponseRecorder, *uuid.UUID) {
	t.Helper()
	e := echo.New()
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	var capturedID uuid.UUID
	next := func(c *echo.Context) error {
		if val := c.Get(UserContextKey); val != nil {
			if id, ok := val.(uuid.UUID); ok {
				capturedID = id
			}
		}
		return c.JSON(http.StatusOK, map[string]string{"ok": "true"})
	}

	mw := AuthMiddleware(jm)
	err := mw(next)(c)
	require.NoError(t, err)
	return rec, &capturedID
}

// TestAuthMiddleware_MissingAuthHeader_Returns401 verifies that a request with
// no Authorization header is rejected with 401 and UNAUTHORIZED code.
func TestAuthMiddleware_MissingAuthHeader_Returns401(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/api/v1/notes", nil)
	rec, _ := runAuthMiddleware(t, req, newAuthTestJWTManager())

	assert.Equal(t, http.StatusUnauthorized, rec.Code)
	assert.Contains(t, rec.Body.String(), "UNAUTHORIZED")
}

// TestAuthMiddleware_InvalidFormat_NotBearer_Returns401 verifies that an
// Authorization header using a non-Bearer scheme (e.g. "Token") is rejected.
func TestAuthMiddleware_InvalidFormat_NotBearer_Returns401(t *testing.T) {
	jm := newAuthTestJWTManager()
	userID := uuid.New()
	tok, err := jm.GenerateAccessToken(userID, "user@example.com")
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/notes", nil)
	req.Header.Set("Authorization", "Token "+tok) // wrong scheme
	rec, _ := runAuthMiddleware(t, req, jm)

	assert.Equal(t, http.StatusUnauthorized, rec.Code)
	assert.Contains(t, rec.Body.String(), "UNAUTHORIZED")
}

// TestAuthMiddleware_MalformedToken_Returns401 verifies that an Authorization
// header with "Bearer" but a non-JWT value is rejected.
func TestAuthMiddleware_MalformedToken_Returns401(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/api/v1/notes", nil)
	req.Header.Set("Authorization", "Bearer not-a-jwt-string")
	rec, _ := runAuthMiddleware(t, req, newAuthTestJWTManager())

	assert.Equal(t, http.StatusUnauthorized, rec.Code)
	assert.Contains(t, rec.Body.String(), "UNAUTHORIZED")
}

// TestAuthMiddleware_ExpiredToken_Returns401 verifies that a JWT whose exp is in
// the past is rejected with 401, even when the signature is otherwise valid.
func TestAuthMiddleware_ExpiredToken_Returns401(t *testing.T) {
	userID := uuid.New()
	claims := &utils.Claims{
		UserID:    userID,
		Email:     "expired@example.com",
		TokenType: utils.AccessToken,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(-time.Hour)),
			IssuedAt:  jwt.NewNumericDate(time.Now().Add(-2 * time.Hour)),
			Subject:   userID.String(),
			Issuer:    "runer-api",
		},
	}
	tok := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	tokenStr, err := tok.SignedString([]byte(authTestSecret))
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/notes", nil)
	req.Header.Set("Authorization", "Bearer "+tokenStr)
	rec, _ := runAuthMiddleware(t, req, newAuthTestJWTManager())

	assert.Equal(t, http.StatusUnauthorized, rec.Code)
	assert.Contains(t, rec.Body.String(), "UNAUTHORIZED")
}

// TestAuthMiddleware_WrongSecret_Returns401 verifies that a JWT signed with a
// key that differs from the server's secret is rejected.
func TestAuthMiddleware_WrongSecret_Returns401(t *testing.T) {
	otherManager := utils.NewJWTManager("wrong-secret-value-at-least-32-bytes!!", 15*time.Minute, 0)
	userID := uuid.New()
	tok, err := otherManager.GenerateAccessToken(userID, "attacker@example.com")
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/notes", nil)
	req.Header.Set("Authorization", "Bearer "+tok)
	// Validate against the correct manager (different secret) — must reject.
	rec, _ := runAuthMiddleware(t, req, newAuthTestJWTManager())

	assert.Equal(t, http.StatusUnauthorized, rec.Code)
}

// TestAuthMiddleware_ValidToken_SetsUserIDInContext verifies that a valid JWT
// passes the middleware and the user_id is stored in the Echo context for the
// downstream handler to use.
func TestAuthMiddleware_ValidToken_SetsUserIDInContext(t *testing.T) {
	jm := newAuthTestJWTManager()
	userID := uuid.New()
	tok, err := jm.GenerateAccessToken(userID, "user@example.com")
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/notes", nil)
	req.Header.Set("Authorization", "Bearer "+tok)
	rec, capturedID := runAuthMiddleware(t, req, jm)

	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, userID, *capturedID, "user_id in context must match the JWT sub")
}
