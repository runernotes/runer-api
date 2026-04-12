package main

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"testing"
	"time"

	"github.com/labstack/echo/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// startTestServer starts the server on a random port and returns the address.
// It runs Start in a background goroutine and returns the errChan so the caller
// can wait for clean termination.
func startTestServer(t *testing.T, ctx context.Context, sc echo.StartConfig, e *echo.Echo) (addr string, errChan <-chan error) {
	t.Helper()

	addrChan := make(chan string, 1)
	ch := make(chan error, 1)

	sc.ListenerAddrFunc = func(a net.Addr) {
		addrChan <- a.String()
	}

	go func() {
		ch <- sc.Start(ctx, e)
	}()

	select {
	case a := <-addrChan:
		return a, ch
	case err := <-ch:
		t.Fatalf("server failed to start: %v", err)
		return "", nil
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for server to start")
		return "", nil
	}
}

// TestBuildStartConfig_GracefulShutdown_InFlightRequestCompletes verifies that
// when SIGTERM is received while a request is in-flight, the server waits for
// that request to finish before closing — no connections are dropped.
func TestBuildStartConfig_GracefulShutdown_InFlightRequestCompletes(t *testing.T) {
	e := echo.New()

	// A handler that signals when it has started processing, then sleeps briefly
	// to simulate real work. This will be in-flight when we cancel the context.
	handlerStarted := make(chan struct{})
	e.GET("/slow", func(c *echo.Context) error {
		close(handlerStarted)
		time.Sleep(80 * time.Millisecond)
		return (*c).String(http.StatusOK, "ok")
	})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sc := buildStartConfig(0, nil) // port 0 → OS picks a free port
	sc.GracefulTimeout = 5 * time.Second

	addr, errChan := startTestServer(t, ctx, sc, e)

	// Fire the slow request in the background.
	type result struct {
		status int
		body   string
	}
	respChan := make(chan result, 1)
	go func() {
		resp, err := http.Get(fmt.Sprintf("http://%s/slow", addr))
		if err != nil {
			respChan <- result{}
			return
		}
		defer func() { _ = resp.Body.Close() }()
		b, _ := io.ReadAll(resp.Body)
		respChan <- result{status: resp.StatusCode, body: string(b)}
	}()

	// Wait until the handler has actually started, then trigger shutdown.
	<-handlerStarted
	cancel()

	// The in-flight request must complete successfully within the grace window.
	select {
	case r := <-respChan:
		assert.Equal(t, http.StatusOK, r.status)
		assert.Equal(t, "ok", r.body)
	case <-time.After(3 * time.Second):
		t.Fatal("in-flight request did not complete within the graceful shutdown window")
	}

	// Wait for the server to fully stop.
	require.NoError(t, <-errChan)

	// After shutdown, new connections must be refused.
	_, err := http.Get(fmt.Sprintf("http://%s/slow", addr))
	assert.Error(t, err, "expected connection refused after shutdown")
}

// TestBuildStartConfig_GracefulShutdown_TimeoutFiresOnShutdownError verifies
// that OnShutdownError is called with context.DeadlineExceeded when a handler
// outlives the GracefulTimeout, so the operator is alerted that connections
// were forcibly closed.
func TestBuildStartConfig_GracefulShutdown_TimeoutFiresOnShutdownError(t *testing.T) {
	e := echo.New()

	handlerStarted := make(chan struct{})
	e.GET("/very-slow", func(c *echo.Context) error {
		close(handlerStarted)
		time.Sleep(500 * time.Millisecond) // outlives the 50 ms grace timeout
		return (*c).String(http.StatusOK, "ok")
	})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	shutdownErrChan := make(chan error, 1)
	sc := buildStartConfig(0, func(err error) {
		shutdownErrChan <- err
	})
	sc.GracefulTimeout = 50 * time.Millisecond // intentionally short

	addr, errChan := startTestServer(t, ctx, sc, e)

	// Fire the very slow request and discard the response (it will be cut off).
	go func() {
		//nolint:errcheck
		http.Get(fmt.Sprintf("http://%s/very-slow", addr)) //nolint:bodyclose
	}()

	<-handlerStarted
	cancel()

	// OnShutdownError must be called because the handler outlives the timeout.
	select {
	case err := <-shutdownErrChan:
		assert.ErrorIs(t, err, context.DeadlineExceeded,
			"expected DeadlineExceeded when handler outlives GracefulTimeout")
	case <-time.After(2 * time.Second):
		t.Fatal("OnShutdownError was not called within the expected timeframe")
	}

	<-errChan
}

// TestBuildStartConfig_IdleTimeoutIsSet verifies that BeforeServeFunc correctly
// applies the 120-second idle timeout to the underlying http.Server.
func TestBuildStartConfig_IdleTimeoutIsSet(t *testing.T) {
	capturedServer := make(chan *http.Server, 1)

	sc := buildStartConfig(0, nil)

	// Wrap the BeforeServeFunc so we can capture the configured server.
	original := sc.BeforeServeFunc
	sc.BeforeServeFunc = func(s *http.Server) error {
		err := original(s)
		capturedServer <- s
		return err
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	e := echo.New()
	e.GET("/ok", func(c *echo.Context) error {
		return (*c).String(http.StatusOK, "ok")
	})

	_, errChan := startTestServer(t, ctx, sc, e)

	srv := <-capturedServer
	assert.Equal(t, 120*time.Second, srv.IdleTimeout,
		"IdleTimeout must be 120 s to reclaim idle keep-alive connections")

	cancel()
	<-errChan
}
