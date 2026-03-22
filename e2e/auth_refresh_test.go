package e2e_test

// Refresh token tests.

import (
	"net/http"
	"testing"

	"github.com/google/uuid"
)

// TestRefreshValidToken verifies that a valid refresh token returns a new access token
// that can be used to access protected routes.
func TestRefreshValidToken(t *testing.T) {
	srv, mock, _ := newTestServer(t)
	e := newExpect(t, srv)

	suffix := uuid.NewString()
	email := "user-" + suffix + "@example.com"

	e.POST("/api/v1/auth/register").
		WithJSON(map[string]string{"email": email, "name": "Refresh Test"}).
		Expect().
		Status(http.StatusOK)

	magicToken := mock.lastToken()
	if magicToken == "" {
		t.Fatal("no magic link token captured after registration")
	}

	resp := e.POST("/api/v1/auth/verify").
		WithJSON(map[string]string{"token": magicToken}).
		Expect().
		Status(http.StatusOK).
		JSON().Object()

	refreshToken := resp.Value("refresh_token").String().NotEmpty().Raw()

	refreshResp := e.POST("/api/v1/auth/refresh").
		WithJSON(map[string]string{"refresh_token": refreshToken}).
		Expect().
		Status(http.StatusOK).
		JSON().Object()

	newAccessToken := refreshResp.Value("access_token").String().NotEmpty().Raw()

	// The new access token must work on a protected route.
	e.GET("/api/v1/notes").
		WithHeader("Authorization", "Bearer "+newAccessToken).
		Expect().
		Status(http.StatusOK)
}

// TestRefreshRevokedTokenReturns401 verifies that a refresh token that has been revoked
// via logout is rejected with 401 and INVALID_REFRESH_TOKEN.
func TestRefreshRevokedTokenReturns401(t *testing.T) {
	srv, mock, _ := newTestServer(t)
	e := newExpect(t, srv)

	suffix := uuid.NewString()
	email := "user-" + suffix + "@example.com"

	e.POST("/api/v1/auth/register").
		WithJSON(map[string]string{"email": email, "name": "Refresh Test"}).
		Expect().
		Status(http.StatusOK)

	magicToken := mock.lastToken()
	if magicToken == "" {
		t.Fatal("no magic link token captured after registration")
	}

	resp := e.POST("/api/v1/auth/verify").
		WithJSON(map[string]string{"token": magicToken}).
		Expect().
		Status(http.StatusOK).
		JSON().Object()

	accessToken := resp.Value("access_token").String().NotEmpty().Raw()
	refreshToken := resp.Value("refresh_token").String().NotEmpty().Raw()

	// Revoke via logout.
	e.POST("/api/v1/auth/logout").
		WithHeader("Authorization", "Bearer "+accessToken).
		WithJSON(map[string]string{"refresh_token": refreshToken}).
		Expect().
		Status(http.StatusNoContent)

	// The revoked token must now be rejected.
	e.POST("/api/v1/auth/refresh").
		WithJSON(map[string]string{"refresh_token": refreshToken}).
		Expect().
		Status(http.StatusUnauthorized).
		JSON().Object().
		HasValue("code", "INVALID_REFRESH_TOKEN")
}

// TestRefreshExpiredTokenReturns401 verifies that an expired refresh token (inserted
// directly into the database) is rejected with 401 and INVALID_REFRESH_TOKEN.
func TestRefreshExpiredTokenReturns401(t *testing.T) {
	srv, mock, db := newTestServer(t)
	e := newExpect(t, srv)

	// Register a user to get a valid user_id for the FK constraint.
	suffix := uuid.NewString()
	email := "user-" + suffix + "@example.com"

	e.POST("/api/v1/auth/register").
		WithJSON(map[string]string{"email": email, "name": "Refresh Expire"}).
		Expect().
		Status(http.StatusOK)

	magicToken := mock.lastToken()
	if magicToken == "" {
		t.Fatal("no magic link token captured")
	}

	e.POST("/api/v1/auth/verify").
		WithJSON(map[string]string{"token": magicToken}).
		Expect().
		Status(http.StatusOK)

	var userRecord struct {
		ID uuid.UUID `gorm:"column:id"`
	}
	if err := db.Table("users").Select("id").Where("email = ?", email).Scan(&userRecord).Error; err != nil {
		t.Fatalf("finding test user: %v", err)
	}
	if userRecord.ID == uuid.Nil {
		t.Fatal("test user not found in database")
	}

	expiredRawToken := insertExpiredRefreshToken(t, db, userRecord.ID)

	e.POST("/api/v1/auth/refresh").
		WithJSON(map[string]string{"refresh_token": expiredRawToken}).
		Expect().
		Status(http.StatusUnauthorized).
		JSON().Object().
		HasValue("code", "INVALID_REFRESH_TOKEN")
}

// TestRefreshMissingFieldReturns400 verifies that submitting an empty body to
// POST /auth/refresh returns 400 with VALIDATION_ERROR.
func TestRefreshMissingFieldReturns400(t *testing.T) {
	srv, _, _ := newTestServer(t)
	e := newExpect(t, srv)

	e.POST("/api/v1/auth/refresh").
		WithJSON(map[string]string{}).
		Expect().
		Status(http.StatusBadRequest).
		JSON().Object().
		HasValue("code", "VALIDATION_ERROR")
}

// TestRefreshGarbageTokenReturns401 verifies that a token value that does not exist in
// the database is rejected with 401 and INVALID_REFRESH_TOKEN.
func TestRefreshGarbageTokenReturns401(t *testing.T) {
	srv, _, _ := newTestServer(t)
	e := newExpect(t, srv)

	e.POST("/api/v1/auth/refresh").
		WithJSON(map[string]string{"refresh_token": "not-a-real-refresh-token"}).
		Expect().
		Status(http.StatusUnauthorized).
		JSON().Object().
		HasValue("code", "INVALID_REFRESH_TOKEN")
}
