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

// TestGetNotesInvalidCursor verifies that passing a cursor value that is not valid base64
// returns 400 with INVALID_PARAM.
func TestGetNotesInvalidCursor(t *testing.T) {
	srv, mock, _ := newTestServer(t)
	e := newExpect(t, srv)

	token := registerAndLogin(t, e, mock, uuid.NewString())

	// A cursor that is not valid base64 must be rejected with INVALID_PARAM.
	e.GET("/api/v1/notes").
		WithHeader("Authorization", "Bearer "+token).
		WithQuery("cursor", "!!!not-valid-base64!!!").
		Expect().
		Status(http.StatusBadRequest).
		JSON().Object().
		HasValue("code", "INVALID_PARAM")
}

// TestFullSyncReturnsAllNotesAndTombstonesOnFirstPage verifies that a full pull (no since)
// returns all notes and all tombstones on the first page when the total count is within the
// page limit. The spec states that tombstones are only included on the first page of a full
// sync, so this test confirms tombstones appear alongside notes without requiring pagination.
func TestFullSyncReturnsAllNotesAndTombstonesOnFirstPage(t *testing.T) {
	srv, mock, _ := newTestServer(t)
	e := newExpect(t, srv)

	token := registerAndLogin(t, e, mock, uuid.NewString())

	// Create two active notes and one note that will be purged (producing a tombstone).
	id1 := createNote(t, e, token)
	id2 := createNote(t, e, token)
	idPurged := createNote(t, e, token)

	// Purge the third note to create a tombstone.
	e.DELETE("/api/v1/notes/"+idPurged+"/purge").
		WithHeader("Authorization", "Bearer "+token).
		Expect().
		Status(http.StatusNoContent)

	// Full sync — no since, no cursor.
	syncResp := e.GET("/api/v1/notes").
		WithHeader("Authorization", "Bearer "+token).
		Expect().
		Status(http.StatusOK).
		JSON().Object()

	// server_time must be present.
	syncResp.Value("server_time").NotNull()

	// All two active notes must appear.
	notes := syncResp.Value("notes").Array()
	notes.Length().IsEqual(2)

	returnedIDs := make([]string, 0, 2)
	for _, v := range notes.Iter() {
		returnedIDs = append(returnedIDs, v.Object().Value("note_id").String().Raw())
	}
	require.ElementsMatch(t, []string{id1, id2}, returnedIDs)

	// The tombstone for the purged note must appear on the first page.
	tombstones := syncResp.Value("tombstones").Array()
	tombstones.Length().IsEqual(1)
	tombstones.Value(0).Object().HasValue("note_id", idPurged)
}

// TestGetNotesLimitZeroReturnsBadRequest verifies that sending limit=0 is rejected with
// 400 and INVALID_PARAM because zero is below the minimum allowed page size of 1.
func TestGetNotesLimitZeroReturnsBadRequest(t *testing.T) {
	srv, mock, _ := newTestServer(t)
	e := newExpect(t, srv)

	token := registerAndLogin(t, e, mock, uuid.NewString())

	// limit=0 is below the minimum and must be rejected.
	e.GET("/api/v1/notes").
		WithHeader("Authorization", "Bearer "+token).
		WithQuery("limit", "0").
		Expect().
		Status(http.StatusBadRequest).
		JSON().Object().
		HasValue("code", "INVALID_PARAM")
}

// TestGetNotesNonNumericLimitReturnsBadRequest verifies that a non-numeric limit query
// parameter is rejected with 400 and INVALID_PARAM.
func TestGetNotesNonNumericLimitReturnsBadRequest(t *testing.T) {
	srv, mock, _ := newTestServer(t)
	e := newExpect(t, srv)

	token := registerAndLogin(t, e, mock, uuid.NewString())

	// A non-numeric limit must be rejected with INVALID_PARAM.
	e.GET("/api/v1/notes").
		WithHeader("Authorization", "Bearer "+token).
		WithQuery("limit", "abc").
		Expect().
		Status(http.StatusBadRequest).
		JSON().Object().
		HasValue("code", "INVALID_PARAM")
}

// TestGetNotesLimitAboveMaxIsClamped verifies that sending limit=999, which exceeds the
// server's maximum page size of 500, succeeds with 200 and returns at most 500 notes rather
// than being rejected. This confirms the server clamps the value rather than refusing it.
func TestGetNotesLimitAboveMaxIsClamped(t *testing.T) {
	srv, mock, _ := newTestServer(t)
	e := newExpect(t, srv)

	token := registerAndLogin(t, e, mock, uuid.NewString())

	// Create a small number of notes to confirm the response is shaped correctly.
	createNote(t, e, token)
	createNote(t, e, token)

	// limit=999 exceeds the maximum of 500 but must still return 200.
	resp := e.GET("/api/v1/notes").
		WithHeader("Authorization", "Bearer "+token).
		WithQuery("limit", "999").
		Expect().
		Status(http.StatusOK).
		JSON().Object()

	// The response must be a valid sync payload.
	resp.Value("server_time").NotNull()

	// The notes array must not exceed 500 items (our 2 notes are well within that).
	notes := resp.Value("notes").Array()
	require.LessOrEqual(t, len(notes.Iter()), 500,
		"notes returned must not exceed the maximum page size of 500")
}

// TestFullSyncReturnsTombstonesOnlyOnFirstPageWhenPaginating verifies that when a full
// sync spans multiple pages, tombstones are included only on the first page and are absent
// from subsequent pages. This prevents clients from double-processing tombstones during
// a paginated full sync.
func TestFullSyncReturnsTombstonesOnlyOnFirstPageWhenPaginating(t *testing.T) {
	srv, mock, _ := newTestServer(t)
	e := newExpect(t, srv)

	token := registerAndLogin(t, e, mock, uuid.NewString())

	// Create enough notes to require at least two pages with limit=2.
	id1 := createNote(t, e, token)
	id2 := createNote(t, e, token)
	id3 := createNote(t, e, token)
	idPurged := createNote(t, e, token)

	// Purge one note to produce a tombstone.
	e.DELETE("/api/v1/notes/"+idPurged+"/purge").
		WithHeader("Authorization", "Bearer "+token).
		Expect().
		Status(http.StatusNoContent)

	// Page 1 of full sync (limit=2): must contain tombstones.
	page1 := e.GET("/api/v1/notes").
		WithHeader("Authorization", "Bearer "+token).
		WithQuery("limit", "2").
		Expect().
		Status(http.StatusOK).
		JSON().Object()

	page1.Value("server_time").NotNull()

	// Page 1 must include the tombstone.
	page1Tombstones := page1.Value("tombstones").Array()
	page1Tombstones.Length().IsEqual(1)
	page1Tombstones.Value(0).Object().HasValue("note_id", idPurged)

	// There must be more pages.
	cursor := page1.Value("next_cursor").String().NotEmpty().Raw()

	// Page 2 of full sync (using cursor): tombstones must be empty.
	page2 := e.GET("/api/v1/notes").
		WithHeader("Authorization", "Bearer "+token).
		WithQuery("limit", "2").
		WithQuery("cursor", cursor).
		Expect().
		Status(http.StatusOK).
		JSON().Object()

	page2.Value("server_time").NotNull()
	page2.Value("tombstones").Array().Length().IsEqual(0)

	// Collect all note IDs across both pages and verify all three active notes appear.
	allIDs := make([]string, 0, 3)
	for _, v := range page1.Value("notes").Array().Iter() {
		allIDs = append(allIDs, v.Object().Value("note_id").String().Raw())
	}
	for _, v := range page2.Value("notes").Array().Iter() {
		allIDs = append(allIDs, v.Object().Value("note_id").String().Raw())
	}
	require.ElementsMatch(t, []string{id1, id2, id3}, allIDs,
		"all three active notes must appear exactly once across pages")
}
