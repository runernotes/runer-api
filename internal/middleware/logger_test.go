package middleware

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestRedactQueryString_NoQueryString(t *testing.T) {
	assert.Equal(t, "/api/v1/notes", redactQueryString("/api/v1/notes"),
		"URI with no query string must be returned unchanged")
}

func TestRedactQueryString_NonSensitiveParamsPreserved(t *testing.T) {
	input := "/api/v1/notes?since=2026-01-01T00:00:00Z&limit=100"
	assert.Equal(t, input, redactQueryString(input),
		"non-sensitive query params must be preserved verbatim")
}

func TestRedactQueryString_TokenRedacted(t *testing.T) {
	result := redactQueryString("/api/v1/auth/verify-redirect?token=k7Hx9Qm2vL4pN1wR8sT3yU")
	assert.Contains(t, result, "token=%5BREDACTED%5D",
		"token param value must be replaced with [REDACTED]")
	assert.NotContains(t, result, "k7Hx9Qm2vL4pN1wR8sT3yU",
		"raw token value must not appear in the output")
}

func TestRedactQueryString_TokenRedactedAlongsideOtherParams(t *testing.T) {
	result := redactQueryString("/path?foo=bar&token=secret&baz=qux")
	assert.Contains(t, result, "foo=bar", "non-sensitive param foo must be preserved")
	assert.Contains(t, result, "baz=qux", "non-sensitive param baz must be preserved")
	assert.NotContains(t, result, "secret", "raw token value must not appear")
}

func TestRedactQueryString_MalformedQueryStringFallsBackToPath(t *testing.T) {
	result := redactQueryString("/path?%ZZ=invalid")
	assert.Equal(t, "/path", result,
		"malformed query string must fall back to path-only to avoid leaking partial values")
}

func TestRedactQueryString_EmptyQueryString(t *testing.T) {
	// A trailing '?' with no params is valid — should not panic.
	result := redactQueryString("/path?")
	assert.Equal(t, "/path?", result,
		"empty query string must be returned unchanged")
}
