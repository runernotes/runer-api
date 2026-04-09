package analytics

import (
	"context"
	"testing"

	"github.com/posthog/posthog-go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- stub PostHog client ---

// stubClient records the most recent Capture call so tests can assert on the
// event name, distinct ID, and properties without making any network calls.
type stubClient struct {
	lastCapture *posthog.Capture
	closeCalled bool
}

func (s *stubClient) Enqueue(msg posthog.Message) error {
	if c, ok := msg.(posthog.Capture); ok {
		s.lastCapture = &c
	}
	return nil
}

func (s *stubClient) Close() error {
	s.closeCalled = true
	return nil
}

func (s *stubClient) CloseWithContext(_ context.Context) error       { return nil }
func (s *stubClient) IsFeatureEnabled(_ posthog.FeatureFlagPayload) (any, error) {
	return nil, nil
}
func (s *stubClient) GetFeatureFlag(_ posthog.FeatureFlagPayload) (any, error) { return nil, nil }
func (s *stubClient) GetFeatureFlagResult(_ posthog.FeatureFlagPayload) (*posthog.FeatureFlagResult, error) {
	return nil, nil
}
func (s *stubClient) GetFeatureFlagPayload(_ posthog.FeatureFlagPayload) (string, error) {
	return "", nil
}
func (s *stubClient) GetRemoteConfigPayload(_ string) (string, error) { return "", nil }
func (s *stubClient) GetAllFlags(_ posthog.FeatureFlagPayloadNoKey) (map[string]any, error) {
	return nil, nil
}
func (s *stubClient) ReloadFeatureFlags() error                       { return nil }
func (s *stubClient) GetFeatureFlags() ([]posthog.FeatureFlag, error) { return nil, nil }

// --- NoopTracker ---

// TestNoopTracker_CaptureIsSafe verifies that calling Capture on a NoopTracker
// never panics, regardless of inputs.
func TestNoopTracker_CaptureIsSafe(t *testing.T) {
	var tr Tracker = NoopTracker{}

	assert.NotPanics(t, func() {
		tr.Capture("some.event", "user-123", map[string]any{"key": "value"})
	})
}

// TestNoopTracker_CloseIsSafe verifies that Close never panics.
func TestNoopTracker_CloseIsSafe(t *testing.T) {
	var tr Tracker = NoopTracker{}
	assert.NotPanics(t, func() { tr.Close() })
}

// TestNoopTracker_NilPropertiesIsSafe verifies that a nil properties map is
// handled gracefully.
func TestNoopTracker_NilPropertiesIsSafe(t *testing.T) {
	var tr Tracker = NoopTracker{}
	assert.NotPanics(t, func() { tr.Capture("event", "id", nil) })
}

// --- New factory ---

// TestNew_EmptyAPIKey_ReturnsNoop verifies that an empty API key produces a
// NoopTracker so callers are never handed a nil Tracker.
func TestNew_EmptyAPIKey_ReturnsNoop(t *testing.T) {
	tr := New("", "")
	_, isNoop := tr.(NoopTracker)
	assert.True(t, isNoop, "expected NoopTracker when apiKey is empty")
}

// --- PostHogTracker ---

// TestPostHogTracker_Capture_ForwardsEvent verifies that Capture enqueues a
// PostHog Capture message with the correct event name and distinct ID.
func TestPostHogTracker_Capture_ForwardsEvent(t *testing.T) {
	stub := &stubClient{}
	tr := newWithClient(stub)

	tr.Capture("note.created", "user-abc", map[string]any{"note_id": "n1"})

	require.NotNil(t, stub.lastCapture, "Enqueue must have been called")
	assert.Equal(t, "note.created", stub.lastCapture.Event)
	assert.Equal(t, "user-abc", stub.lastCapture.DistinctId)
}

// TestPostHogTracker_Capture_SetsProperties verifies that all provided
// properties are forwarded to the PostHog Capture message.
func TestPostHogTracker_Capture_SetsProperties(t *testing.T) {
	stub := &stubClient{}
	tr := newWithClient(stub)

	tr.Capture("note.trashed", "user-xyz", map[string]any{
		"note_id": "note-1",
		"plan":    "free",
	})

	require.NotNil(t, stub.lastCapture)
	props := stub.lastCapture.Properties
	assert.Equal(t, "note-1", props["note_id"])
	assert.Equal(t, "free", props["plan"])
}

// TestPostHogTracker_Capture_NilPropertiesSafe verifies that a nil properties
// map does not cause a panic — only the event metadata is enqueued.
func TestPostHogTracker_Capture_NilPropertiesSafe(t *testing.T) {
	stub := &stubClient{}
	tr := newWithClient(stub)

	assert.NotPanics(t, func() {
		tr.Capture("user.activated", "user-abc", nil)
	})
	require.NotNil(t, stub.lastCapture)
	assert.Equal(t, "user.activated", stub.lastCapture.Event)
}

// TestPostHogTracker_Close_CallsClientClose verifies that Close flushes the
// underlying PostHog client.
func TestPostHogTracker_Close_CallsClientClose(t *testing.T) {
	stub := &stubClient{}
	tr := newWithClient(stub)

	tr.Close()

	assert.True(t, stub.closeCalled, "Close must call the underlying client's Close")
}
