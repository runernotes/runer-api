// Package logging provides the single initialisation point for the application
// logger. Call Setup once from main() before any other log output is needed.
//
// Routing by environment:
//   - Development (or no POSTHOG_API_KEY): colour console writer to stdout.
//   - Production (POSTHOG_API_KEY set): JSON lines to stdout for log
//     aggregators + PostHog writer that forwards warn-and-above entries as
//     "server.log" capture events.
package logging

import (
	"fmt"
	"io"
	"os"
	"time"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"github.com/runernotes/runer-api/internal/config"
)

// Setup configures the global zerolog logger according to cfg and returns a
// shutdown function that flushes any buffered PostHog events. The caller must
// invoke the returned function before the process exits — typically via defer:
//
//	shutdown := logging.Setup(cfg)
//	defer shutdown()
//
// Setup must be called exactly once, early in main(), before any goroutines
// start writing log entries.
func Setup(cfg *config.Config) (shutdown func()) {
	shutdown = func() {} // safe no-op unless PostHog is wired

	var writers []io.Writer

	if cfg.IsProduction() {
		// JSON lines to stdout — consumed by Dokploy, Loki, Datadog, etc.
		// ConsoleWriter is not used in production because log aggregators expect
		// machine-readable JSON, not ANSI-coloured text.
		writers = append(writers, os.Stdout)
	} else {
		// Human-readable coloured output makes local development easier.
		writers = append(writers, zerolog.ConsoleWriter{
			Out:        os.Stdout,
			NoColor:    false,
			TimeFormat: time.RFC3339,
		})
	}

	if cfg.PostHogAPIKey != "" {
		pw, err := newPosthogWriter(cfg.PostHogAPIKey, cfg.PostHogEndpoint, zerolog.WarnLevel)
		if err != nil {
			// The logger isn't configured yet, so write directly to stderr.
			fmt.Fprintf(os.Stderr, "logging: failed to initialise PostHog writer: %v\n", err)
		} else {
			writers = append(writers, pw)
			shutdown = pw.close
		}
	}

	// MultiLevelWriter fans out to all writers. Writers that also implement
	// zerolog.LevelWriter (e.g. posthogWriter) receive WriteLevel calls so
	// they can apply their own level filter.
	multi := zerolog.MultiLevelWriter(writers...)
	log.Logger = zerolog.New(multi).With().Timestamp().Logger()

	return shutdown
}
