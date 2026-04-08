package logging

import (
	"encoding/json"
	"os"
	"time"

	"github.com/posthog/posthog-go"
	"github.com/rs/zerolog"
)

// posthogWriter is a zerolog LevelWriter that forwards log entries at or above
// minLevel to PostHog as "server.log" capture events. The PostHog client
// batches events internally and is safe for concurrent use.
type posthogWriter struct {
	client   posthog.Client
	minLevel zerolog.Level
}

// newPosthogWriter creates a PostHog-backed zerolog LevelWriter.
// endpoint may be empty to use the SDK default (https://app.posthog.com);
// set it to https://eu.i.posthog.com for EU data residency.
// minLevel controls which log levels are forwarded — zerolog.WarnLevel means
// only warn, error, fatal, and panic entries reach PostHog.
func newPosthogWriter(apiKey, endpoint string, minLevel zerolog.Level) (*posthogWriter, error) {
	// adapterLogger writes only to stderr and has no PostHog writer in its
	// chain. This breaks the potential cycle where PostHog SDK log output →
	// zerologPosthogAdapter → global log.Logger → posthogWriter → PostHog SDK.
	adapterLogger := zerolog.New(os.Stderr).With().Timestamp().Logger()

	cfg := posthog.Config{
		// Flush batches every 5 s; the SDK default of 250 events per batch is fine.
		Interval: 5 * time.Second,
		// Bound Close() during server shutdown to avoid hanging the process.
		ShutdownTimeout: 10 * time.Second,
		// Route PostHog SDK's own internal log output through a dedicated
		// stderr logger that is outside the main writer chain, preventing a
		// recursive write cycle back into posthogWriter.
		Logger: &zerologPosthogAdapter{logger: adapterLogger},
	}
	if endpoint != "" {
		cfg.Endpoint = endpoint
	}

	client, err := posthog.NewWithConfig(apiKey, cfg)
	if err != nil {
		return nil, err
	}

	return &posthogWriter{client: client, minLevel: minLevel}, nil
}

// WriteLevel implements zerolog.LevelWriter. Entries below minLevel are
// silently discarded so info/debug logs never reach PostHog.
func (w *posthogWriter) WriteLevel(l zerolog.Level, p []byte) (int, error) {
	if l < w.minLevel {
		return len(p), nil
	}
	return w.Write(p)
}

// Write implements io.Writer. It parses the zerolog JSON entry and enqueues a
// PostHog Capture event. Errors in parsing or enqueueing are swallowed so that
// a PostHog outage or misconfiguration never breaks the application.
func (w *posthogWriter) Write(p []byte) (int, error) {
	var fields map[string]any
	if err := json.Unmarshal(p, &fields); err != nil {
		return len(p), nil // malformed entry — skip silently
	}

	props := posthog.NewProperties()
	for k, v := range fields {
		props.Set(k, v)
	}

	_ = w.client.Enqueue(posthog.Capture{
		DistinctId: resolveDistinctID(fields),
		Event:      "server.log",
		Properties: props,
	})

	return len(p), nil
}

// resolveDistinctID picks the best actor identifier for a log entry.
// PostHog groups events by distinct_id; we prefer the authenticated user_id
// (so errors appear on the user's profile), fall back to request_id for
// correlation, and use the static "server" marker for background events.
func resolveDistinctID(fields map[string]any) string {
	if uid, ok := fields["user_id"].(string); ok && uid != "" {
		return uid
	}
	if rid, ok := fields["request_id"].(string); ok && rid != "" {
		// Prefix to avoid collision with real user UUIDs.
		return "req:" + rid
	}
	return "server"
}

// close flushes any buffered PostHog events and shuts down the client.
// It is called by the shutdown function returned from Setup.
func (w *posthogWriter) close() {
	_ = w.client.Close()
}

// zerologPosthogAdapter implements posthog.Logger by routing the PostHog SDK's
// internal log messages through a dedicated zerolog logger that writes only to
// stderr. Using a separate logger (rather than the global log.Logger) prevents
// a recursive cycle: PostHog SDK log → adapter → global logger → posthogWriter
// → PostHog SDK enqueue → (potentially) PostHog SDK log again.
type zerologPosthogAdapter struct {
	logger zerolog.Logger
}

func (z *zerologPosthogAdapter) Debugf(format string, args ...any) {
	z.logger.Debug().Msgf("[posthog] "+format, args...)
}

func (z *zerologPosthogAdapter) Logf(format string, args ...any) {
	z.logger.Info().Msgf("[posthog] "+format, args...)
}

func (z *zerologPosthogAdapter) Warnf(format string, args ...any) {
	z.logger.Warn().Msgf("[posthog] "+format, args...)
}

func (z *zerologPosthogAdapter) Errorf(format string, args ...any) {
	z.logger.Error().Msgf("[posthog] "+format, args...)
}
