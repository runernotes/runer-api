package e2e_test

// Notes isolation security tests.
//
// These tests verify that one user cannot read or mutate another user's notes.

import (
	"encoding/base64"
	"net/http"
	"sync"
	"testing"

	"github.com/google/uuid"
)

// TestNotesIsolationRead verifies that user B receives 404 when attempting to read a
// note that belongs to user A.
func TestNotesIsolationRead(t *testing.T) {
	srv, mock, _ := newTestServer(t)
	e := newExpect(t, srv)

	tokenA := registerAndLogin(t, e, mock, uuid.NewString())
	noteID := createNote(t, e, tokenA)

	tokenB := registerAndLogin(t, e, mock, uuid.NewString())

	// User B cannot read user A's note — must get 404, not 200 or 403.
	e.GET("/api/v1/notes/"+noteID).
		WithHeader("Authorization", "Bearer "+tokenB).
		Expect().
		Status(http.StatusNotFound).
		JSON().Object().
		HasValue("code", "NOT_FOUND")
}

// TestNotesIsolationUpdate verifies that user B receives 404 when attempting to PUT to
// a note_id that belongs to user A with the note's exact UUID. The upsert must not
// silently create a new note under user B's account. After the failed attempt, the note
// under user A must remain unchanged.
func TestNotesIsolationUpdate(t *testing.T) {
	srv, mock, _ := newTestServer(t)
	e := newExpect(t, srv)

	tokenA := registerAndLogin(t, e, mock, uuid.NewString())
	noteID := createNote(t, e, tokenA)

	// Record the original payload so we can verify it was not overwritten.
	originalPayload := base64.StdEncoding.EncodeToString([]byte("hello"))

	tokenB := registerAndLogin(t, e, mock, uuid.NewString())

	// User B cannot update user A's note — must get 404.
	attackerPayload := base64.StdEncoding.EncodeToString([]byte("tampered by attacker"))
	e.PUT("/api/v1/notes/"+noteID).
		WithHeader("Authorization", "Bearer "+tokenB).
		WithJSON(map[string]any{"encrypted_payload": attackerPayload}).
		Expect().
		Status(http.StatusNotFound).
		JSON().Object().
		HasValue("code", "NOT_FOUND")

	// Confirm the note under user A was not overwritten.
	e.GET("/api/v1/notes/"+noteID).
		WithHeader("Authorization", "Bearer "+tokenA).
		Expect().
		Status(http.StatusOK).
		JSON().Object().
		HasValue("encrypted_payload", originalPayload)
}

// TestConcurrentUserIsolation verifies that two users operating concurrently cannot
// access each other's data. Both users create a note in parallel, then each confirms
// they can read their own note but not the other's.
func TestConcurrentUserIsolation(t *testing.T) {
	srv, mock, _ := newTestServer(t)
	e := newExpect(t, srv)

	// Register and log in both users sequentially so the mock's lastToken() is
	// unambiguous for each registration.
	tokenA := registerAndLogin(t, e, mock, uuid.NewString())
	tokenB := registerAndLogin(t, e, mock, uuid.NewString())

	var (
		noteIDA string
		noteIDB string
		wg      sync.WaitGroup
		mu      sync.Mutex
	)

	// Both users create a note concurrently.
	wg.Add(2)

	go func() {
		defer wg.Done()
		id := uuid.New().String()
		payload := base64.StdEncoding.EncodeToString([]byte("user A note"))
		e.PUT("/api/v1/notes/"+id).
			WithHeader("Authorization", "Bearer "+tokenA).
			WithJSON(map[string]any{"encrypted_payload": payload}).
			Expect().
			Status(http.StatusOK)
		mu.Lock()
		noteIDA = id
		mu.Unlock()
	}()

	go func() {
		defer wg.Done()
		id := uuid.New().String()
		payload := base64.StdEncoding.EncodeToString([]byte("user B note"))
		e.PUT("/api/v1/notes/"+id).
			WithHeader("Authorization", "Bearer "+tokenB).
			WithJSON(map[string]any{"encrypted_payload": payload}).
			Expect().
			Status(http.StatusOK)
		mu.Lock()
		noteIDB = id
		mu.Unlock()
	}()

	wg.Wait()

	// Verify each note was successfully created before asserting cross-user isolation.
	e.GET("/api/v1/notes/"+noteIDA).
		WithHeader("Authorization", "Bearer "+tokenA).
		Expect().
		Status(http.StatusOK)

	e.GET("/api/v1/notes/"+noteIDB).
		WithHeader("Authorization", "Bearer "+tokenB).
		Expect().
		Status(http.StatusOK)

	// Isolation: neither user can access the other's note.
	e.GET("/api/v1/notes/"+noteIDA).
		WithHeader("Authorization", "Bearer "+tokenB).
		Expect().
		Status(http.StatusNotFound)

	e.GET("/api/v1/notes/"+noteIDB).
		WithHeader("Authorization", "Bearer "+tokenA).
		Expect().
		Status(http.StatusNotFound)
}
