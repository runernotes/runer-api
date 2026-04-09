// Package analytics provides a thin, non-blocking wrapper around PostHog for
// capturing product analytics events. It is intentionally separate from the
// logging package so that analytics events and log forwarding can evolve
// independently and a failure in one never affects the other.
//
// Usage:
//
//	tracker := analytics.New(cfg.PostHogAPIKey, cfg.PostHogEndpoint)
//	defer tracker.Close()
//	tracker.Capture("note.created", userID, map[string]any{"note_id": noteID})
package analytics

import (
	"fmt"
	"os"
	"time"

	"github.com/posthog/posthog-go"
)

// Tracker captures product analytics events. Implementations must be safe for
// concurrent use. A Tracker must never return an error from Capture — a PostHog
// outage or misconfiguration must not affect application availability.
type Tracker interface {
	// Capture enqueues an analytics event. It is non-blocking and always
	// succeeds from the caller's perspective.
	Capture(event string, distinctID string, properties map[string]any)

	// Close flushes any buffered events and releases resources. The caller
	// must invoke Close before the process exits — typically via defer.
	Close()
}

// NoopTracker silently discards all events. Returned by New when no API key is
// configured so callers never need to guard against a nil Tracker.
type NoopTracker struct{}

func (NoopTracker) Capture(_ string, _ string, _ map[string]any) {}
func (NoopTracker) Close()                                        {}

// PostHogTracker forwards events to the PostHog API via the official SDK.
type PostHogTracker struct {
	client posthog.Client
}

// newWithClient constructs a PostHogTracker from an already-initialised PostHog
// client. Used in tests to inject a stub without making real network calls.
func newWithClient(client posthog.Client) *PostHogTracker {
	return &PostHogTracker{client: client}
}

// New creates a Tracker backed by PostHog when apiKey is non-empty. If apiKey
// is empty or client initialisation fails, a NoopTracker is returned so the
// application keeps running. Failures are written to stderr rather than the
// application logger to avoid a dependency cycle.
func New(apiKey, endpoint string) Tracker {
	if apiKey == "" {
		return NoopTracker{}
	}

	cfg := posthog.Config{
		// Flush event batches every 5 s — the same interval used by the logging
		// PostHog writer so both clients behave consistently.
		Interval: 5 * time.Second,
		// Cap Close() during graceful shutdown to avoid hanging the process.
		ShutdownTimeout: 10 * time.Second,
	}
	if endpoint != "" {
		cfg.Endpoint = endpoint
	}

	client, err := posthog.NewWithConfig(apiKey, cfg)
	if err != nil {
		fmt.Fprintf(os.Stderr, "analytics: failed to initialise PostHog client: %v\n", err)
		return NoopTracker{}
	}

	return &PostHogTracker{client: client}
}

// Capture enqueues a PostHog Capture event. The call is non-blocking — the SDK
// batches events internally and flushes them on its own interval. Any enqueue
// error is intentionally swallowed so a PostHog outage never surfaces to callers.
func (t *PostHogTracker) Capture(event string, distinctID string, properties map[string]any) {
	props := posthog.NewProperties()
	for k, v := range properties {
		props.Set(k, v)
	}

	_ = t.client.Enqueue(posthog.Capture{
		DistinctId: distinctID,
		Event:      event,
		Properties: props,
	})
}

// Close flushes any buffered events and shuts down the underlying PostHog client.
func (t *PostHogTracker) Close() {
	_ = t.client.Close()
}
