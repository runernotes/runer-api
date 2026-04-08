package e2e_test

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestCORS_AllowedOrigin verifies that the API echoes back the correct
// Access-Control-Allow-Origin header when the request comes from a known client origin.
func TestCORS_AllowedOrigin(t *testing.T) {
	t.Parallel()

	srv, _, _ := newTestServer(t)

	// Preflight from an allowed origin.
	req, err := http.NewRequest(http.MethodOptions, srv.URL+"/api/v1/notes", nil)
	require.NoError(t, err)
	req.Header.Set("Origin", "app://localhost")
	req.Header.Set("Access-Control-Request-Method", "GET")
	req.Header.Set("Access-Control-Request-Headers", "Authorization")

	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	assert.Equal(t, "app://localhost", resp.Header.Get("Access-Control-Allow-Origin"),
		"preflight from allowed origin must return matching ACAO header")
}

// TestCORS_DisallowedOrigin verifies that the API does not echo back an
// Access-Control-Allow-Origin header for origins not in the allowlist.
func TestCORS_DisallowedOrigin(t *testing.T) {
	t.Parallel()

	srv, _, _ := newTestServer(t)

	req, err := http.NewRequest(http.MethodOptions, srv.URL+"/api/v1/notes", nil)
	require.NoError(t, err)
	req.Header.Set("Origin", "https://evil.example.com")
	req.Header.Set("Access-Control-Request-Method", "GET")
	req.Header.Set("Access-Control-Request-Headers", "Authorization")

	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	assert.Empty(t, resp.Header.Get("Access-Control-Allow-Origin"),
		"preflight from disallowed origin must not return ACAO header")
}

// TestCORS_WildcardRejected verifies that the wildcard origin "*" is never
// present in production — the server must return a specific origin or nothing.
func TestCORS_WildcardRejected(t *testing.T) {
	t.Parallel()

	srv, _, _ := newTestServer(t)

	req, err := http.NewRequest(http.MethodGet, srv.URL+"/api/v1/notes", nil)
	require.NoError(t, err)
	req.Header.Set("Origin", "app://localhost")

	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	acao := resp.Header.Get("Access-Control-Allow-Origin")
	assert.NotEqual(t, "*", acao,
		"server must never respond with Access-Control-Allow-Origin: * (wildcard)")
}

// TestCORS_CapacitorOriginAllowed verifies that Capacitor mobile client origins
// are accepted. Capacitor iOS uses capacitor://localhost; Android uses http://localhost.
func TestCORS_CapacitorOriginAllowed(t *testing.T) {
	t.Parallel()

	srv, _, _ := newTestServer(t)

	for _, origin := range []string{"capacitor://localhost", "http://localhost"} {
		t.Run(origin, func(t *testing.T) {
			t.Parallel()

			req, err := http.NewRequest(http.MethodOptions, srv.URL+"/api/v1/notes", nil)
			require.NoError(t, err)
			req.Header.Set("Origin", origin)
			req.Header.Set("Access-Control-Request-Method", "GET")
			req.Header.Set("Access-Control-Request-Headers", "Authorization")

			resp, err := http.DefaultClient.Do(req)
			require.NoError(t, err)
			defer func() { _ = resp.Body.Close() }()

			assert.Equal(t, origin, resp.Header.Get("Access-Control-Allow-Origin"),
				"Capacitor origin %q must be allowed", origin)
		})
	}
}
