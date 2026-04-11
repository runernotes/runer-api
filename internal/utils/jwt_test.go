package utils

import (
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const testJWTSecret = "test-secret-value-at-least-32-bytes!!"

// newTestJWTManager returns a JWTManager configured for testing with a short
// 15-minute access token duration.
func newTestJWTManager() *JWTManager {
	return NewJWTManager(testJWTSecret, 15*time.Minute, 7*24*time.Hour)
}

// TestGenerateAccessToken_ProducesNonEmptyToken verifies that GenerateAccessToken
// returns a non-empty string and no error for valid inputs.
func TestGenerateAccessToken_ProducesNonEmptyToken(t *testing.T) {
	m := newTestJWTManager()
	userID := uuid.New()

	token, err := m.GenerateAccessToken(userID, "user@example.com")
	require.NoError(t, err)
	assert.NotEmpty(t, token, "generated access token must not be empty")
}

// TestValidateAccessToken_Valid_ReturnsClaims verifies that a freshly generated
// token round-trips correctly through ValidateAccessToken and the claims match
// the values used during generation.
func TestValidateAccessToken_Valid_ReturnsClaims(t *testing.T) {
	m := newTestJWTManager()
	userID := uuid.New()
	email := "alice@example.com"

	tokenStr, err := m.GenerateAccessToken(userID, email)
	require.NoError(t, err)

	claims, err := m.ValidateAccessToken(tokenStr)
	require.NoError(t, err)
	require.NotNil(t, claims)
	assert.Equal(t, userID, claims.UserID)
	assert.Equal(t, email, claims.Email)
	assert.Equal(t, AccessToken, claims.TokenType)
}

// TestValidateAccessToken_ExpiredToken_ReturnsErrExpiredToken verifies that a
// token whose exp claim is already in the past is rejected with ErrExpiredToken.
func TestValidateAccessToken_ExpiredToken_ReturnsErrExpiredToken(t *testing.T) {
	m := newTestJWTManager()
	userID := uuid.New()

	// Craft a JWT that expired 1 hour ago using the same signing key.
	claims := &Claims{
		UserID:    userID,
		Email:     "user@example.com",
		TokenType: AccessToken,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(-time.Hour)),
			IssuedAt:  jwt.NewNumericDate(time.Now().Add(-2 * time.Hour)),
			Subject:   userID.String(),
			Issuer:    "runer-api",
		},
	}
	tok := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	tokenStr, err := tok.SignedString([]byte(testJWTSecret))
	require.NoError(t, err)

	_, err = m.ValidateAccessToken(tokenStr)
	assert.ErrorIs(t, err, ErrExpiredToken)
}

// TestValidateAccessToken_WrongSecret_ReturnsErrInvalidToken verifies that a
// token signed with a different secret key is rejected with ErrInvalidToken.
func TestValidateAccessToken_WrongSecret_ReturnsErrInvalidToken(t *testing.T) {
	// Generate with a different secret.
	other := NewJWTManager("completely-different-secret-value-!!", 15*time.Minute, 0)
	userID := uuid.New()

	tokenStr, err := other.GenerateAccessToken(userID, "user@example.com")
	require.NoError(t, err)

	// Validate with the test manager (different secret) — must be rejected.
	m := newTestJWTManager()
	_, err = m.ValidateAccessToken(tokenStr)
	assert.ErrorIs(t, err, ErrInvalidToken)
}

// TestValidateAccessToken_WrongTokenType_ReturnsErrInvalidToken verifies that a
// JWT with the correct signature but a non-access token_type is rejected.
// This guards against using a refresh-scheme token in place of an access token.
func TestValidateAccessToken_WrongTokenType_ReturnsErrInvalidToken(t *testing.T) {
	m := newTestJWTManager()
	userID := uuid.New()

	// Build a token with a non-access token_type claim.
	claims := &Claims{
		UserID:    userID,
		Email:     "user@example.com",
		TokenType: "refresh", // wrong type
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(15 * time.Minute)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
			Subject:   userID.String(),
			Issuer:    "runer-api",
		},
	}
	tok := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	tokenStr, err := tok.SignedString([]byte(testJWTSecret))
	require.NoError(t, err)

	_, err = m.ValidateAccessToken(tokenStr)
	assert.ErrorIs(t, err, ErrInvalidToken)
}

// TestValidateAccessToken_MalformedString_ReturnsErrInvalidToken verifies that
// garbage input (not a JWT at all) is rejected with ErrInvalidToken.
func TestValidateAccessToken_MalformedString_ReturnsErrInvalidToken(t *testing.T) {
	m := newTestJWTManager()
	_, err := m.ValidateAccessToken("this-is-not-a-jwt")
	assert.ErrorIs(t, err, ErrInvalidToken)
}
