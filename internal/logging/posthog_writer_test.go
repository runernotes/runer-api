package logging

import (
	"context"
	"testing"

	"github.com/posthog/posthog-go"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
)

// --- resolveDistinctID ---

func TestResolveDistinctID_UserID(t *testing.T) {
	fields := map[string]any{
		"user_id":    "abc-123",
		"request_id": "req-456",
	}
	assert.Equal(t, "abc-123", resolveDistinctID(fields),
		"user_id should take highest priority")
}

func TestResolveDistinctID_RequestIDFallback(t *testing.T) {
	fields := map[string]any{
		"request_id": "req-456",
	}
	assert.Equal(t, "req:req-456", resolveDistinctID(fields),
		"request_id fallback should be prefixed with 'req:'")
}

func TestResolveDistinctID_ServerFallback(t *testing.T) {
	assert.Equal(t, "server", resolveDistinctID(map[string]any{}),
		"empty fields should fall back to 'server'")
}

func TestResolveDistinctID_EmptyUserIDFallsThrough(t *testing.T) {
	fields := map[string]any{
		"user_id":    "",
		"request_id": "req-789",
	}
	assert.Equal(t, "req:req-789", resolveDistinctID(fields),
		"empty user_id string should fall through to request_id")
}

func TestResolveDistinctID_WrongTypeIgnored(t *testing.T) {
	fields := map[string]any{
		"user_id": 12345, // not a string — should be ignored
	}
	assert.Equal(t, "server", resolveDistinctID(fields),
		"non-string user_id should be ignored and fall back to 'server'")
}

// --- posthogWriter level filtering ---

// stubPosthogClient is a minimal posthog.Client implementation that counts
// how many times Enqueue is called so tests can assert forwarding behaviour
// without hitting the network.
type stubPosthogClient struct {
	enqueueCount int
}

func (s *stubPosthogClient) Enqueue(_ posthog.Message) error {
	s.enqueueCount++
	return nil
}

func (s *stubPosthogClient) Close() error                           { return nil }
func (s *stubPosthogClient) CloseWithContext(_ context.Context) error { return nil }
func (s *stubPosthogClient) IsFeatureEnabled(_ posthog.FeatureFlagPayload) (any, error) {
	return nil, nil
}
func (s *stubPosthogClient) GetFeatureFlag(_ posthog.FeatureFlagPayload) (any, error) {
	return nil, nil
}
func (s *stubPosthogClient) GetFeatureFlagResult(_ posthog.FeatureFlagPayload) (*posthog.FeatureFlagResult, error) {
	return nil, nil
}
func (s *stubPosthogClient) GetFeatureFlagPayload(_ posthog.FeatureFlagPayload) (string, error) {
	return "", nil
}
func (s *stubPosthogClient) GetRemoteConfigPayload(_ string) (string, error) { return "", nil }
func (s *stubPosthogClient) GetAllFlags(_ posthog.FeatureFlagPayloadNoKey) (map[string]any, error) {
	return nil, nil
}
func (s *stubPosthogClient) ReloadFeatureFlags() error                        { return nil }
func (s *stubPosthogClient) GetFeatureFlags() ([]posthog.FeatureFlag, error)  { return nil, nil }

func newTestPosthogWriter(minLevel zerolog.Level) (*posthogWriter, *stubPosthogClient) {
	stub := &stubPosthogClient{}
	pw := &posthogWriter{client: stub, minLevel: minLevel}
	return pw, stub
}

func TestPosthogWriterLevel_BelowMinLevelDiscarded(t *testing.T) {
	pw, stub := newTestPosthogWriter(zerolog.WarnLevel)

	entry := []byte(`{"level":"info","message":"should be discarded"}`)
	n, err := pw.WriteLevel(zerolog.InfoLevel, entry)

	assert.NoError(t, err)
	assert.Equal(t, len(entry), n, "should return full length even when discarding")
	assert.Equal(t, 0, stub.enqueueCount, "info entry must not be forwarded when minLevel is warn")
}

func TestPosthogWriterLevel_AtMinLevelForwarded(t *testing.T) {
	pw, stub := newTestPosthogWriter(zerolog.WarnLevel)

	entry := []byte(`{"level":"warn","message":"should be forwarded"}`)
	_, err := pw.WriteLevel(zerolog.WarnLevel, entry)

	assert.NoError(t, err)
	assert.Equal(t, 1, stub.enqueueCount, "warn entry must be forwarded when minLevel is warn")
}

func TestPosthogWriterLevel_AboveMinLevelForwarded(t *testing.T) {
	pw, stub := newTestPosthogWriter(zerolog.WarnLevel)

	entry := []byte(`{"level":"error","message":"should be forwarded"}`)
	_, err := pw.WriteLevel(zerolog.ErrorLevel, entry)

	assert.NoError(t, err)
	assert.Equal(t, 1, stub.enqueueCount, "error entry must be forwarded when minLevel is warn")
}

func TestPosthogWriterLevel_MalformedJSONDiscardedSilently(t *testing.T) {
	pw, stub := newTestPosthogWriter(zerolog.WarnLevel)

	_, err := pw.WriteLevel(zerolog.ErrorLevel, []byte(`not json`))

	assert.NoError(t, err, "malformed JSON must not return an error")
	assert.Equal(t, 0, stub.enqueueCount, "malformed JSON must not be forwarded")
}
