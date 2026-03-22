package e2e_test

// Magic link and verify-redirect tests.

import (
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/gavv/httpexpect/v2"
	"github.com/google/uuid"
	"github.com/runernotes/runer-api/internal/auth"
	"github.com/runernotes/runer-api/internal/utils"
	"gorm.io/gorm"
)

// TestMagicLinkKnownEmail verifies that requesting a magic link for a known email
// returns 200 and sends a new, distinct magic link token that can be used to log in.
func TestMagicLinkKnownEmail(t *testing.T) {
	srv, mockEmail, _ := newTestServer(t)
	e := newExpect(t, srv)

	// Register a user so that the "known email" request has a real account to target.
	suffix := uuid.NewString()
	knownEmail := "user-" + suffix + "@example.com"

	e.POST("/api/v1/auth/register").
		WithJSON(map[string]string{"email": knownEmail, "name": "Magic Link User"}).
		Expect().
		Status(http.StatusOK)

	registrationToken := mockEmail.lastToken()
	if registrationToken == "" {
		t.Fatal("no magic link token captured after registration")
	}

	countBefore := mockEmail.callCount()

	e.POST("/api/v1/auth/magic-link").
		WithJSON(map[string]string{"email": knownEmail}).
		Expect().
		Status(http.StatusOK)

	if mockEmail.callCount() != countBefore+1 {
		t.Errorf("expected callCount to increase by 1, got %d (want %d)",
			mockEmail.callCount(), countBefore+1)
	}

	newToken := mockEmail.lastToken()
	if newToken == "" {
		t.Fatal("no magic link token captured after magic-link request")
	}
	if newToken == registrationToken {
		t.Error("expected a new distinct magic link token, but got the same one as registration")
	}

	// The new token must be usable to log in.
	e.POST("/api/v1/auth/verify").
		WithJSON(map[string]string{"token": newToken}).
		Expect().
		Status(http.StatusOK).
		JSON().Object().
		ContainsKey("access_token")
}

// TestMagicLinkUnknownEmail verifies that requesting a magic link for an unknown email
// returns 200 without sending a magic link (no user enumeration).
func TestMagicLinkUnknownEmail(t *testing.T) {
	srv, mockEmail, _ := newTestServer(t)
	e := newExpect(t, srv)

	countBefore := mockEmail.callCount()
	unknownEmail := "nobody-" + uuid.NewString() + "@example.com"

	e.POST("/api/v1/auth/magic-link").
		WithJSON(map[string]string{"email": unknownEmail}).
		Expect().
		Status(http.StatusOK)

	if mockEmail.callCount() != countBefore {
		t.Errorf("expected callCount to stay at %d for unknown email, got %d",
			countBefore, mockEmail.callCount())
	}
}

// TestVerifyValidToken verifies that a fresh magic link token returns 200 with
// both access_token and refresh_token fields.
func TestVerifyValidToken(t *testing.T) {
	srv, mock, _ := newTestServer(t)
	e := newExpect(t, srv)

	suffix := uuid.NewString()
	email := "user-" + suffix + "@example.com"
	e.POST("/api/v1/auth/register").
		WithJSON(map[string]string{"email": email, "name": "Verify Test"}).
		Expect().
		Status(http.StatusOK)

	token := mock.lastToken()
	if token == "" {
		t.Fatal("no magic link token captured after registration")
	}

	e.POST("/api/v1/auth/verify").
		WithJSON(map[string]string{"token": token}).
		Expect().
		Status(http.StatusOK).
		JSON().Object().
		ContainsKey("access_token")
}

// TestVerifyReplayProtection verifies that the second use of the same magic link token
// is rejected with 401 and INVALID_TOKEN.
func TestVerifyReplayProtection(t *testing.T) {
	srv, mock, _ := newTestServer(t)
	e := newExpect(t, srv)

	suffix := uuid.NewString()
	email := "user-" + suffix + "@example.com"
	e.POST("/api/v1/auth/register").
		WithJSON(map[string]string{"email": email, "name": "Verify Test"}).
		Expect().
		Status(http.StatusOK)

	token := mock.lastToken()
	if token == "" {
		t.Fatal("no magic link token captured after registration")
	}

	// First use must succeed.
	e.POST("/api/v1/auth/verify").
		WithJSON(map[string]string{"token": token}).
		Expect().
		Status(http.StatusOK).
		JSON().Object().
		ContainsKey("access_token")

	// Second use of the same token must fail.
	e.POST("/api/v1/auth/verify").
		WithJSON(map[string]string{"token": token}).
		Expect().
		Status(http.StatusUnauthorized).
		JSON().Object().
		HasValue("code", "INVALID_TOKEN")
}

// TestVerifyExpiredToken verifies that an already-expired magic link token is rejected
// with 401 and INVALID_TOKEN.
func TestVerifyExpiredToken(t *testing.T) {
	srv, mock, db := newTestServer(t)
	e := newExpect(t, srv)

	// Register a real user so the inserted expired token satisfies the FK constraint.
	suffix := uuid.NewString()
	accessToken := registerAndLogin(t, e, mock, suffix)
	if accessToken == "" {
		t.Fatal("setup login failed — no access token")
	}

	// Look up the user that was just created.
	var userRecord struct {
		ID uuid.UUID `gorm:"column:id"`
	}
	email := "user-" + suffix + "@example.com"
	if err := db.Table("users").Select("id").Where("email = ?", email).Scan(&userRecord).Error; err != nil {
		t.Fatalf("finding test user: %v", err)
	}
	if userRecord.ID == uuid.Nil {
		t.Fatal("test user not found in database")
	}

	rawExpiredToken := "expired-token-" + uuid.NewString()
	tokenHash := utils.ComputeSHA256(rawExpiredToken)

	expiredRecord := auth.MagicLinkToken{
		TokenID:   uuid.New(),
		UserID:    userRecord.ID,
		Email:     email,
		Token:     tokenHash,
		ExpiresAt: time.Now().Add(-time.Hour),
	}
	if err := db.Create(&expiredRecord).Error; err != nil {
		t.Fatalf("inserting expired magic link token: %v", err)
	}

	e.POST("/api/v1/auth/verify").
		WithJSON(map[string]string{"token": rawExpiredToken}).
		Expect().
		Status(http.StatusUnauthorized).
		JSON().Object().
		HasValue("code", "INVALID_TOKEN")
}

// TestVerifyGarbageToken verifies that a token value that does not exist in the database
// is rejected with 401 and INVALID_TOKEN.
func TestVerifyGarbageToken(t *testing.T) {
	srv, _, _ := newTestServer(t)
	e := newExpect(t, srv)

	e.POST("/api/v1/auth/verify").
		WithJSON(map[string]string{"token": "this-is-not-a-real-token"}).
		Expect().
		Status(http.StatusUnauthorized).
		JSON().Object().
		HasValue("code", "INVALID_TOKEN")
}

// TestVerifyMissingTokenField verifies that submitting an empty JSON body to /auth/verify
// returns 400 with VALIDATION_ERROR.
func TestVerifyMissingTokenField(t *testing.T) {
	srv, _, _ := newTestServer(t)
	e := newExpect(t, srv)

	e.POST("/api/v1/auth/verify").
		WithJSON(map[string]string{}).
		Expect().
		Status(http.StatusBadRequest).
		JSON().Object().
		HasValue("code", "VALIDATION_ERROR")
}

// TestVerifyRedirectValidToken verifies that a valid token query parameter causes
// GET /auth/verify-redirect to return 302 with a deep-link Location header.
func TestVerifyRedirectValidToken(t *testing.T) {
	srv, _, _ := newTestServer(t)
	e := newExpect(t, srv)

	token := "some-opaque-token-value"

	resp := e.GET("/api/v1/auth/verify-redirect").
		WithQuery("token", token).
		WithRedirectPolicy(httpexpect.DontFollowRedirects).
		Expect().
		Status(http.StatusFound)

	location := resp.Header("Location").NotEmpty().Raw()
	if !strings.HasPrefix(location, "runer://auth/verify?token=") {
		t.Errorf("expected Location to start with runer://auth/verify?token=, got: %s", location)
	}
}

// TestVerifyRedirectMissingToken verifies that GET /auth/verify-redirect without a token
// query parameter returns 400 with MISSING_TOKEN.
func TestVerifyRedirectMissingToken(t *testing.T) {
	srv, _, _ := newTestServer(t)
	e := newExpect(t, srv)

	e.GET("/api/v1/auth/verify-redirect").
		Expect().
		Status(http.StatusBadRequest).
		JSON().Object().
		HasValue("code", "MISSING_TOKEN")
}

// insertExpiredRefreshToken creates a refresh token row with expires_at in the past
// for the given user and returns the raw (unhashed) token string.
func insertExpiredRefreshToken(t *testing.T, db *gorm.DB, userID uuid.UUID) string {
	t.Helper()

	rawToken := "expired-refresh-" + uuid.NewString()
	tokenHash := utils.ComputeSHA256(rawToken)

	record := auth.RefreshToken{
		TokenID:   uuid.New(),
		UserID:    userID,
		TokenHash: tokenHash,
		ExpiresAt: time.Now().Add(-time.Hour), // already expired
	}
	if err := db.Create(&record).Error; err != nil {
		t.Fatalf("inserting expired refresh token: %v", err)
	}

	return rawToken
}
