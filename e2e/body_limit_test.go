package e2e_test

import (
	"bytes"
	"net/http"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestBodyLimit_OversizedPayloadRejected verifies that a request body exceeding
// MAX_REQUEST_BODY is rejected with HTTP 413 before any handler logic runs.
// A 1K limit is used so the test does not need to allocate many megabytes; the
// middleware behaviour is identical regardless of the configured threshold.
func TestBodyLimit_OversizedPayloadRejected(t *testing.T) {
	t.Parallel()

	srv, mock, _ := newTestServer(t, testServerOpts{maxRequestBody: "1K"})
	e := newExpect(t, srv)
	token := registerAndLogin(t, e, mock, uuid.NewString())

	// Build a payload that is clearly above the 1K limit (2 KiB of 'x').
	oversizedBody := bytes.Repeat([]byte("x"), 2*1024)

	req, err := http.NewRequest(
		http.MethodPut,
		srv.URL+"/api/v1/notes/"+uuid.New().String(),
		bytes.NewReader(oversizedBody),
	)
	require.NoError(t, err)
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	assert.Equal(t, http.StatusRequestEntityTooLarge, resp.StatusCode,
		"request body exceeding the configured limit must be rejected with 413")
}

// TestBodyLimit_WithinLimitAccepted verifies that a normally-sized request body
// (well below the configured limit) is accepted as before.
func TestBodyLimit_WithinLimitAccepted(t *testing.T) {
	t.Parallel()

	// Use the default 1M limit — small JSON note bodies are never a problem.
	srv, mock, _ := newTestServer(t)
	e := newExpect(t, srv)
	token := registerAndLogin(t, e, mock, uuid.NewString())

	noteID := uuid.New().String()
	smallPayload := `{"encrypted_payload":"aGVsbG8="}`

	req, err := http.NewRequest(
		http.MethodPut,
		srv.URL+"/api/v1/notes/"+noteID,
		bytes.NewBufferString(smallPayload),
	)
	require.NoError(t, err)
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	assert.Equal(t, http.StatusOK, resp.StatusCode,
		"request body within the limit must be accepted normally")
}
