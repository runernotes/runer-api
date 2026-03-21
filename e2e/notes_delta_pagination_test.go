package e2e_test

import (
	"net/http"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

// TestDeltaSyncPagination_FirstPage verifies that the first page of a delta sync with
// fewer notes than the page limit returns all notes, null next_cursor, and server_time.
func TestDeltaSyncPagination_FirstPage(t *testing.T) {
	srv, mock, _ := newTestServer(t)
	e := newExpect(t, srv)

	token := registerAndLogin(t, e, mock, uuid.NewString())

	// Record the sync baseline before creating notes.
	since := time.Now().UTC().Add(-time.Second)

	// Create 3 notes (well under deltaPageSize=100).
	n1 := createNote(t, e, token)
	n2 := createNote(t, e, token)
	n3 := createNote(t, e, token)

	// Delta sync — no cursor, should return all 3 notes on one page.
	syncResp := e.GET("/api/v1/notes").
		WithHeader("Authorization", "Bearer "+token).
		WithQuery("since", since.Format(time.RFC3339Nano)).
		Expect().
		Status(http.StatusOK).
		JSON().Object()

	// server_time must always be present.
	syncResp.Value("server_time").NotNull()

	notes := syncResp.Value("notes").Array()
	notes.Length().IsEqual(3)

	// next_cursor must be absent on the single (last) page.
	syncResp.Value("next_cursor").IsNull()

	// Collect returned IDs and verify all three notes are present.
	returnedIDs := make([]string, 0, 3)
	for _, v := range notes.Iter() {
		returnedIDs = append(returnedIDs, v.Object().Value("note_id").String().Raw())
	}
	require.ElementsMatch(t, []string{n1, n2, n3}, returnedIDs)
}

// TestDeltaSyncPagination_CursorContinuation verifies multi-page delta sync:
//  1. Creates 3 notes then requests with limit=2 to force a second page.
//  2. Page 1 must have next_cursor set and return 2 notes.
//  3. Page 2 (using the cursor) returns the remaining note with no next_cursor.
//  4. All note IDs appear exactly once across both pages.
func TestDeltaSyncPagination_CursorContinuation(t *testing.T) {
	srv, mock, _ := newTestServer(t)
	e := newExpect(t, srv)

	token := registerAndLogin(t, e, mock, uuid.NewString())

	since := time.Now().UTC().Add(-time.Second)

	noteID1 := createNote(t, e, token)
	noteID2 := createNote(t, e, token)
	noteID3 := createNote(t, e, token)

	// Request page 1 with limit=2 to force a second page.
	page1 := e.GET("/api/v1/notes").
		WithHeader("Authorization", "Bearer "+token).
		WithQuery("since", since.Format(time.RFC3339Nano)).
		WithQuery("limit", "2").
		Expect().
		Status(http.StatusOK).
		JSON().Object()

	// server_time must be present on intermediate pages.
	page1.Value("server_time").NotNull()

	// next_cursor must be present — there are more notes.
	cursorRaw := page1.Value("next_cursor").String().NotEmpty().Raw()

	page1Notes := page1.Value("notes").Array()
	page1Notes.Length().IsEqual(2)

	// Request page 2 using the cursor.
	page2 := e.GET("/api/v1/notes").
		WithHeader("Authorization", "Bearer "+token).
		WithQuery("since", since.Format(time.RFC3339Nano)).
		WithQuery("limit", "2").
		WithQuery("cursor", cursorRaw).
		Expect().
		Status(http.StatusOK).
		JSON().Object()

	// server_time must be present on the last page too.
	page2.Value("server_time").NotNull()

	// next_cursor absent on last page.
	page2.Value("next_cursor").IsNull()

	page2Notes := page2.Value("notes").Array()
	page2Notes.Length().IsEqual(1)

	// Collect all note IDs across both pages.
	allIDs := make([]string, 0, 3)
	for _, v := range page1Notes.Iter() {
		allIDs = append(allIDs, v.Object().Value("note_id").String().Raw())
	}
	for _, v := range page2Notes.Iter() {
		allIDs = append(allIDs, v.Object().Value("note_id").String().Raw())
	}

	require.ElementsMatch(t, []string{noteID1, noteID2, noteID3}, allIDs,
		"all notes must appear exactly once across pages")
}

// TestDeltaSyncPagination_TombstoneInResponse verifies that tombstones appear in delta
// sync responses and that a purged note produces a tombstone with no note entry.
func TestDeltaSyncPagination_TombstoneInResponse(t *testing.T) {
	srv, mock, _ := newTestServer(t)
	e := newExpect(t, srv)

	token := registerAndLogin(t, e, mock, uuid.NewString())

	since := time.Now().UTC().Add(-time.Second)

	noteID := createNote(t, e, token)

	// Purge the note — this creates a tombstone.
	e.DELETE("/api/v1/notes/"+noteID+"/purge").
		WithHeader("Authorization", "Bearer "+token).
		Expect().
		Status(http.StatusNoContent)

	// Delta sync should return the tombstone and no notes.
	syncResp := e.GET("/api/v1/notes").
		WithHeader("Authorization", "Bearer "+token).
		WithQuery("since", since.Format(time.RFC3339Nano)).
		Expect().
		Status(http.StatusOK).
		JSON().Object()

	syncResp.Value("server_time").NotNull()
	syncResp.Value("next_cursor").IsNull()

	syncResp.Value("notes").Array().Length().IsEqual(0)

	tombstones := syncResp.Value("tombstones").Array()
	tombstones.Length().IsEqual(1)
	tombstones.Value(0).Object().HasValue("note_id", noteID)
}

// TestDeltaSyncPagination_ServerTimeAlwaysPresent verifies that server_time appears on
// every page of a paginated delta sync, not only on the last page.
func TestDeltaSyncPagination_ServerTimeAlwaysPresent(t *testing.T) {
	srv, mock, _ := newTestServer(t)
	e := newExpect(t, srv)

	token := registerAndLogin(t, e, mock, uuid.NewString())

	since := time.Now().UTC().Add(-time.Second)

	// Create 3 notes then page through with limit=1 to produce 3 pages.
	createNote(t, e, token)
	createNote(t, e, token)
	createNote(t, e, token)

	cursor := ""
	pages := 0
	for {
		req := e.GET("/api/v1/notes").
			WithHeader("Authorization", "Bearer "+token).
			WithQuery("since", since.Format(time.RFC3339Nano)).
			WithQuery("limit", "1")
		if cursor != "" {
			req = req.WithQuery("cursor", cursor)
		}

		resp := req.Expect().
			Status(http.StatusOK).
			JSON().Object()

		// server_time must be present on every page.
		resp.Value("server_time").NotNull()

		pages++

		nextVal := resp.Value("next_cursor").Raw()
		if nextVal == nil {
			break
		}
		cursor = resp.Value("next_cursor").String().Raw()

		// Safety guard against infinite loop in case of a pagination bug.
		if pages > 10 {
			t.Fatal("too many pages — pagination is not terminating correctly")
		}
	}

	// 3 notes with limit=1 must produce exactly 3 pages.
	require.Equal(t, 3, pages, "expected 3 pages for 3 notes with limit=1")
}

// TestDeltaSyncPagination_EmptyDelta verifies that a delta sync with no changes returns
// empty notes, empty tombstones, null next_cursor, and a server_time.
func TestDeltaSyncPagination_EmptyDelta(t *testing.T) {
	srv, mock, _ := newTestServer(t)
	e := newExpect(t, srv)

	token := registerAndLogin(t, e, mock, uuid.NewString())

	// Use a "now" baseline so nothing will appear in the delta window.
	since := time.Now().UTC()

	syncResp := e.GET("/api/v1/notes").
		WithHeader("Authorization", "Bearer "+token).
		WithQuery("since", since.Format(time.RFC3339Nano)).
		Expect().
		Status(http.StatusOK).
		JSON().Object()

	syncResp.Value("server_time").NotNull()
	syncResp.Value("next_cursor").IsNull()
	syncResp.Value("notes").Array().Length().IsEqual(0)
	syncResp.Value("tombstones").Array().Length().IsEqual(0)
}
