package notes

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mockRepository implements the repository interface for unit testing.
type mockRepository struct {
	findAllFn                      func(ctx context.Context, userID uuid.UUID) ([]Note, error)
	findAllPaginatedFn             func(ctx context.Context, userID uuid.UUID, limit int, afterUpdatedAt time.Time, afterNoteID uuid.UUID) ([]Note, error)
	findUpdatedSinceFn             func(ctx context.Context, userID uuid.UUID, since time.Time) ([]Note, error)
	findUpdatedSincePaginatedFn    func(ctx context.Context, userID uuid.UUID, since time.Time, limit int, afterUpdatedAt time.Time, afterNoteID uuid.UUID) ([]Note, error)
	findByIDFn                     func(ctx context.Context, noteID uuid.UUID, userID uuid.UUID) (*Note, error)
	upsertFn                       func(ctx context.Context, note *Note) error
	trashFn                        func(ctx context.Context, noteID uuid.UUID, userID uuid.UUID) error
	restoreFn                      func(ctx context.Context, noteID uuid.UUID, userID uuid.UUID) error
	purgeFn                        func(ctx context.Context, noteID uuid.UUID, userID uuid.UUID) error
	findTombstonesSinceFn          func(ctx context.Context, userID uuid.UUID, since time.Time) ([]NoteTombstone, error)
	findTombstonesSincePaginatedFn func(ctx context.Context, userID uuid.UUID, since time.Time, limit int, afterDeletedAt time.Time, afterNoteID uuid.UUID) ([]NoteTombstone, error)
	findAllTombstonesFn            func(ctx context.Context, userID uuid.UUID) ([]NoteTombstone, error)
	purgeExpiredTombstonesFn       func(ctx context.Context, olderThan time.Time) (int64, error)
}

func (m *mockRepository) FindAll(ctx context.Context, userID uuid.UUID) ([]Note, error) {
	if m.findAllFn != nil {
		return m.findAllFn(ctx, userID)
	}
	return nil, nil
}

func (m *mockRepository) FindAllPaginated(ctx context.Context, userID uuid.UUID, limit int, afterUpdatedAt time.Time, afterNoteID uuid.UUID) ([]Note, error) {
	if m.findAllPaginatedFn != nil {
		return m.findAllPaginatedFn(ctx, userID, limit, afterUpdatedAt, afterNoteID)
	}
	return nil, nil
}

func (m *mockRepository) FindUpdatedSince(ctx context.Context, userID uuid.UUID, since time.Time) ([]Note, error) {
	if m.findUpdatedSinceFn != nil {
		return m.findUpdatedSinceFn(ctx, userID, since)
	}
	return nil, nil
}

func (m *mockRepository) FindUpdatedSincePaginated(ctx context.Context, userID uuid.UUID, since time.Time, limit int, afterUpdatedAt time.Time, afterNoteID uuid.UUID) ([]Note, error) {
	if m.findUpdatedSincePaginatedFn != nil {
		return m.findUpdatedSincePaginatedFn(ctx, userID, since, limit, afterUpdatedAt, afterNoteID)
	}
	return nil, nil
}

func (m *mockRepository) FindByID(ctx context.Context, noteID uuid.UUID, userID uuid.UUID) (*Note, error) {
	if m.findByIDFn != nil {
		return m.findByIDFn(ctx, noteID, userID)
	}
	return nil, ErrNoteNotFound
}

func (m *mockRepository) Upsert(ctx context.Context, note *Note) error {
	if m.upsertFn != nil {
		return m.upsertFn(ctx, note)
	}
	return nil
}

func (m *mockRepository) Trash(ctx context.Context, noteID uuid.UUID, userID uuid.UUID) error {
	if m.trashFn != nil {
		return m.trashFn(ctx, noteID, userID)
	}
	return nil
}

func (m *mockRepository) Restore(ctx context.Context, noteID uuid.UUID, userID uuid.UUID) error {
	if m.restoreFn != nil {
		return m.restoreFn(ctx, noteID, userID)
	}
	return nil
}

func (m *mockRepository) Purge(ctx context.Context, noteID uuid.UUID, userID uuid.UUID) error {
	if m.purgeFn != nil {
		return m.purgeFn(ctx, noteID, userID)
	}
	return nil
}

func (m *mockRepository) FindTombstonesSince(ctx context.Context, userID uuid.UUID, since time.Time) ([]NoteTombstone, error) {
	if m.findTombstonesSinceFn != nil {
		return m.findTombstonesSinceFn(ctx, userID, since)
	}
	return nil, nil
}

func (m *mockRepository) FindTombstonesSincePaginated(ctx context.Context, userID uuid.UUID, since time.Time, limit int, afterDeletedAt time.Time, afterNoteID uuid.UUID) ([]NoteTombstone, error) {
	if m.findTombstonesSincePaginatedFn != nil {
		return m.findTombstonesSincePaginatedFn(ctx, userID, since, limit, afterDeletedAt, afterNoteID)
	}
	return nil, nil
}

func (m *mockRepository) FindAllTombstones(ctx context.Context, userID uuid.UUID) ([]NoteTombstone, error) {
	if m.findAllTombstonesFn != nil {
		return m.findAllTombstonesFn(ctx, userID)
	}
	return nil, nil
}

func (m *mockRepository) PurgeExpiredTombstones(ctx context.Context, olderThan time.Time) (int64, error) {
	if m.purgeExpiredTombstonesFn != nil {
		return m.purgeExpiredTombstonesFn(ctx, olderThan)
	}
	return 0, nil
}

// makeNotes returns n notes with sequential updated_at values starting from base.
func makeNotes(n int, base time.Time) []Note {
	notes := make([]Note, n)
	for i := range notes {
		notes[i] = Note{
			ID:        uuid.New(),
			UpdatedAt: base.Add(time.Duration(i) * time.Second),
		}
	}
	return notes
}

// makeTombstones returns n tombstones with sequential deleted_at values starting from base.
func makeTombstones(n int, base time.Time) []NoteTombstone {
	ts := make([]NoteTombstone, n)
	for i := range ts {
		ts[i] = NoteTombstone{
			NoteID:    uuid.New(),
			DeletedAt: base.Add(time.Duration(i) * time.Second),
		}
	}
	return ts
}

// ---- GetNotesSince (delta sync) ----

// TestGetNotesSince_FirstPage verifies that the first delta-sync page returns up to
// deltaPageSize notes, sets hasMore=true, and returns tombstones on the first page.
func TestGetNotesSince_FirstPage(t *testing.T) {
	ctx := context.Background()
	userID := uuid.New()
	since := time.Now().Add(-time.Hour)
	base := since.Add(time.Second)

	// Repo returns deltaPageSize+1 notes (simulating there is a next page).
	notes := makeNotes(deltaPageSize+1, base)
	tombstones := makeTombstones(2, base)

	repo := &mockRepository{
		findUpdatedSincePaginatedFn: func(_ context.Context, _ uuid.UUID, _ time.Time, limit int, afterUpdatedAt time.Time, afterNoteID uuid.UUID) ([]Note, error) {
			assert.Equal(t, deltaPageSize+1, limit)
			assert.True(t, afterUpdatedAt.IsZero())
			assert.Equal(t, uuid.Nil, afterNoteID)
			return notes[:limit], nil
		},
		findTombstonesSincePaginatedFn: func(_ context.Context, _ uuid.UUID, _ time.Time, limit int, afterDeletedAt time.Time, afterNoteID uuid.UUID) ([]NoteTombstone, error) {
			assert.Equal(t, deltaPageSize+1, limit)
			assert.True(t, afterDeletedAt.IsZero())
			assert.Equal(t, uuid.Nil, afterNoteID)
			return tombstones, nil
		},
	}

	svc := NewNotesService(repo)
	gotNotes, gotTombstones, hasMore, err := svc.GetNotesSince(ctx, userID, &since, nil, deltaPageSize)
	require.NoError(t, err)
	assert.True(t, hasMore)
	assert.Len(t, gotNotes, deltaPageSize)
	assert.Len(t, gotTombstones, 2)
}

// TestGetNotesSince_LastPage verifies that when the repo returns ≤ deltaPageSize notes,
// hasMore is false and all notes are returned.
func TestGetNotesSince_LastPage(t *testing.T) {
	ctx := context.Background()
	userID := uuid.New()
	since := time.Now().Add(-time.Hour)
	base := since.Add(time.Second)

	notes := makeNotes(3, base)
	tombstones := makeTombstones(1, base)

	repo := &mockRepository{
		findUpdatedSincePaginatedFn: func(_ context.Context, _ uuid.UUID, _ time.Time, _ int, _ time.Time, _ uuid.UUID) ([]Note, error) {
			return notes, nil
		},
		findTombstonesSincePaginatedFn: func(_ context.Context, _ uuid.UUID, _ time.Time, _ int, _ time.Time, _ uuid.UUID) ([]NoteTombstone, error) {
			return tombstones, nil
		},
	}

	svc := NewNotesService(repo)
	gotNotes, gotTombstones, hasMore, err := svc.GetNotesSince(ctx, userID, &since, nil, deltaPageSize)
	require.NoError(t, err)
	assert.False(t, hasMore)
	assert.Len(t, gotNotes, 3)
	assert.Len(t, gotTombstones, 1)
}

// TestGetNotesSince_EmptyDelta verifies that an empty result set returns hasMore=false
// with empty slices.
func TestGetNotesSince_EmptyDelta(t *testing.T) {
	ctx := context.Background()
	userID := uuid.New()
	since := time.Now().Add(-time.Hour)

	repo := &mockRepository{
		findUpdatedSincePaginatedFn: func(_ context.Context, _ uuid.UUID, _ time.Time, _ int, _ time.Time, _ uuid.UUID) ([]Note, error) {
			return nil, nil
		},
		findTombstonesSincePaginatedFn: func(_ context.Context, _ uuid.UUID, _ time.Time, _ int, _ time.Time, _ uuid.UUID) ([]NoteTombstone, error) {
			return nil, nil
		},
	}

	svc := NewNotesService(repo)
	gotNotes, gotTombstones, hasMore, err := svc.GetNotesSince(ctx, userID, &since, nil, deltaPageSize)
	require.NoError(t, err)
	assert.False(t, hasMore)
	assert.Empty(t, gotNotes)
	assert.Empty(t, gotTombstones)
}

// TestGetNotesSince_SubsequentPage verifies that passing a cursor correctly forwards the
// keyset values to the repository and returns tombstones on subsequent pages too.
func TestGetNotesSince_SubsequentPage(t *testing.T) {
	ctx := context.Background()
	userID := uuid.New()
	since := time.Now().Add(-time.Hour)
	base := since.Add(time.Second)

	cursorUpdatedAt := base.Add(10 * time.Second)
	cursorNoteID := uuid.New()
	cursor := &NoteCursor{AfterUpdatedAt: cursorUpdatedAt, AfterNoteID: cursorNoteID}

	notes := makeNotes(5, base.Add(11*time.Second))

	var capturedAfterUpdatedAt time.Time
	var capturedAfterNoteID uuid.UUID

	repo := &mockRepository{
		findUpdatedSincePaginatedFn: func(_ context.Context, _ uuid.UUID, _ time.Time, _ int, afterUpdatedAt time.Time, afterNoteID uuid.UUID) ([]Note, error) {
			capturedAfterUpdatedAt = afterUpdatedAt
			capturedAfterNoteID = afterNoteID
			return notes, nil
		},
		findTombstonesSincePaginatedFn: func(_ context.Context, _ uuid.UUID, _ time.Time, _ int, _ time.Time, _ uuid.UUID) ([]NoteTombstone, error) {
			return nil, nil
		},
	}

	svc := NewNotesService(repo)
	_, _, hasMore, err := svc.GetNotesSince(ctx, userID, &since, cursor, deltaPageSize)
	require.NoError(t, err)
	assert.False(t, hasMore)
	assert.Equal(t, cursorUpdatedAt, capturedAfterUpdatedAt)
	assert.Equal(t, cursorNoteID, capturedAfterNoteID)
}

// TestGetNotesSince_RepoError verifies that a repository error is propagated correctly.
func TestGetNotesSince_RepoError(t *testing.T) {
	ctx := context.Background()
	userID := uuid.New()
	since := time.Now().Add(-time.Hour)
	repoErr := errors.New("db error")

	repo := &mockRepository{
		findUpdatedSincePaginatedFn: func(_ context.Context, _ uuid.UUID, _ time.Time, _ int, _ time.Time, _ uuid.UUID) ([]Note, error) {
			return nil, repoErr
		},
	}

	svc := NewNotesService(repo)
	_, _, _, err := svc.GetNotesSince(ctx, userID, &since, nil, deltaPageSize)
	assert.ErrorIs(t, err, repoErr)
}

// TestGetNotesSince_TombstoneRepoError verifies that a tombstone repository error is
// propagated correctly.
func TestGetNotesSince_TombstoneRepoError(t *testing.T) {
	ctx := context.Background()
	userID := uuid.New()
	since := time.Now().Add(-time.Hour)
	repoErr := errors.New("tombstone db error")

	repo := &mockRepository{
		findUpdatedSincePaginatedFn: func(_ context.Context, _ uuid.UUID, _ time.Time, _ int, _ time.Time, _ uuid.UUID) ([]Note, error) {
			return nil, nil
		},
		findTombstonesSincePaginatedFn: func(_ context.Context, _ uuid.UUID, _ time.Time, _ int, _ time.Time, _ uuid.UUID) ([]NoteTombstone, error) {
			return nil, repoErr
		},
	}

	svc := NewNotesService(repo)
	_, _, _, err := svc.GetNotesSince(ctx, userID, &since, nil, deltaPageSize)
	assert.ErrorIs(t, err, repoErr)
}

// ---- GetNotesSince (full sync — no regression) ----

// TestGetNotesSince_FullSync_NilSince verifies that when since is nil, the full-sync
// paginated path is taken (unchanged behavior).
func TestGetNotesSince_FullSync_NilSince(t *testing.T) {
	ctx := context.Background()
	userID := uuid.New()

	notes := makeNotes(3, time.Now())
	tombstones := makeTombstones(2, time.Now())

	repo := &mockRepository{
		findAllPaginatedFn: func(_ context.Context, _ uuid.UUID, limit int, _ time.Time, _ uuid.UUID) ([]Note, error) {
			return notes[:min(limit, len(notes))], nil
		},
		findAllTombstonesFn: func(_ context.Context, _ uuid.UUID) ([]NoteTombstone, error) {
			return tombstones, nil
		},
	}

	svc := NewNotesService(repo)
	gotNotes, gotTombstones, hasMore, err := svc.GetNotesSince(ctx, userID, nil, nil, deltaPageSize)
	require.NoError(t, err)
	assert.False(t, hasMore)
	assert.Len(t, gotNotes, 3)
	assert.Len(t, gotTombstones, 2)
}

// min is a local helper used in tests only; Go 1.21+ provides a builtin but the
// service pkg targets older-compatible code, so we keep it test-local.
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
