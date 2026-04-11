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
	countLiveNotesFn               func(ctx context.Context, userID uuid.UUID) (int64, error)
}

// mockUsersRepo implements the usersRepository interface for unit testing.
type mockUsersRepo struct {
	findByIDFn func(ctx context.Context, id uuid.UUID) (UserPlan, error)
}

func (m *mockUsersRepo) FindByID(ctx context.Context, id uuid.UUID) (UserPlan, error) {
	if m.findByIDFn != nil {
		return m.findByIDFn(ctx, id)
	}
	return UserPlan{Plan: "free"}, nil
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

func (m *mockRepository) CountLiveNotes(ctx context.Context, userID uuid.UUID) (int64, error) {
	if m.countLiveNotesFn != nil {
		return m.countLiveNotesFn(ctx, userID)
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

	svc := newServiceNoQuota(repo)
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

	svc := newServiceNoQuota(repo)
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

	svc := newServiceNoQuota(repo)
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

	svc := newServiceNoQuota(repo)
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

	svc := newServiceNoQuota(repo)
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

	svc := newServiceNoQuota(repo)
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

	svc := newServiceNoQuota(repo)
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

// ---- UpsertNote quota checks ----

// newServiceNoQuota constructs a NotesService with no quota enforcement for tests
// that don't exercise quota logic.
func newServiceNoQuota(repo repository) *NotesService {
	return NewNotesService(repo, &mockUsersRepo{}, 0)
}

// newServiceWithQuota constructs a NotesService with the given repo and users repo,
// wiring in a free note limit for quota enforcement.
func newServiceWithQuota(repo repository, usersRepo *mockUsersRepo, limit int) *NotesService {
	return NewNotesService(repo, usersRepo, limit)
}

// TestUpsertNote_QuotaExceeded_NewNote verifies that a free user creating a new note
// beyond the limit receives ErrQuotaExceeded.
func TestUpsertNote_QuotaExceeded_NewNote(t *testing.T) {
	ctx := context.Background()
	userID := uuid.New()
	noteID := uuid.New()
	const limit = 3

	usersRepo := &mockUsersRepo{
		findByIDFn: func(_ context.Context, _ uuid.UUID) (UserPlan, error) {
			return UserPlan{Plan: "free"}, nil
		},
	}

	repo := &mockRepository{
		// Note does not exist yet — this is a create.
		findByIDFn: func(_ context.Context, _ uuid.UUID, _ uuid.UUID) (*Note, error) {
			return nil, ErrNoteNotFound
		},
		countLiveNotesFn: func(_ context.Context, _ uuid.UUID) (int64, error) {
			return int64(limit), nil // already at limit
		},
	}

	svc := newServiceWithQuota(repo, usersRepo, limit)
	_, created, err := svc.UpsertNote(ctx, userID, noteID, []byte("data"), nil)
	require.ErrorIs(t, err, ErrQuotaExceeded)
	assert.False(t, created)
}

// TestUpsertNote_QuotaNotExceeded_NewNote verifies that a free user creating a note
// below the limit succeeds.
func TestUpsertNote_QuotaNotExceeded_NewNote(t *testing.T) {
	ctx := context.Background()
	userID := uuid.New()
	noteID := uuid.New()
	const limit = 3

	usersRepo := &mockUsersRepo{
		findByIDFn: func(_ context.Context, _ uuid.UUID) (UserPlan, error) {
			return UserPlan{Plan: "free"}, nil
		},
	}

	savedNote := &Note{ID: noteID, UserID: userID}
	upsertCalled := false

	repo := &mockRepository{
		findByIDFn: func(_ context.Context, id uuid.UUID, _ uuid.UUID) (*Note, error) {
			if upsertCalled {
				return savedNote, nil
			}
			return nil, ErrNoteNotFound // first call: note does not exist
		},
		countLiveNotesFn: func(_ context.Context, _ uuid.UUID) (int64, error) {
			return int64(limit - 1), nil // one below limit
		},
		upsertFn: func(_ context.Context, _ *Note) error {
			upsertCalled = true
			return nil
		},
	}

	svc := newServiceWithQuota(repo, usersRepo, limit)
	note, created, err := svc.UpsertNote(ctx, userID, noteID, []byte("data"), nil)
	require.NoError(t, err)
	require.NotNil(t, note)
	assert.True(t, created, "first upsert of a note must be reported as created")
}

// TestUpsertNote_ProUser_SkipsQuota verifies that a pro user can create notes beyond
// the free limit without receiving ErrQuotaExceeded.
func TestUpsertNote_ProUser_SkipsQuota(t *testing.T) {
	ctx := context.Background()
	userID := uuid.New()
	noteID := uuid.New()
	const limit = 3

	usersRepo := &mockUsersRepo{
		findByIDFn: func(_ context.Context, _ uuid.UUID) (UserPlan, error) {
			return UserPlan{Plan: "pro"}, nil
		},
	}

	savedNote := &Note{ID: noteID, UserID: userID}
	upsertCalled := false

	repo := &mockRepository{
		findByIDFn: func(_ context.Context, _ uuid.UUID, _ uuid.UUID) (*Note, error) {
			if upsertCalled {
				return savedNote, nil
			}
			return nil, ErrNoteNotFound
		},
		countLiveNotesFn: func(_ context.Context, _ uuid.UUID) (int64, error) {
			// Should not be called for pro users.
			t.Error("CountLiveNotes must not be called for pro users")
			return int64(limit + 10), nil
		},
		upsertFn: func(_ context.Context, _ *Note) error {
			upsertCalled = true
			return nil
		},
	}

	svc := newServiceWithQuota(repo, usersRepo, limit)
	note, created, err := svc.UpsertNote(ctx, userID, noteID, []byte("data"), nil)
	require.NoError(t, err)
	require.NotNil(t, note)
	assert.True(t, created, "a new note from a pro user must be reported as created")
}

// TestUpsertNote_WithBaseVersion_SkipsQuota verifies that an update with a base_version
// (client knows about the existing note) never triggers a quota check.
func TestUpsertNote_WithBaseVersion_SkipsQuota(t *testing.T) {
	ctx := context.Background()
	userID := uuid.New()
	noteID := uuid.New()
	const limit = 3
	baseVersion := time.Now().Add(-time.Minute)

	usersRepo := &mockUsersRepo{
		findByIDFn: func(_ context.Context, _ uuid.UUID) (UserPlan, error) {
			// Should not be called when base_version is set.
			t.Error("FindByID (users) must not be called when base_version is provided")
			return UserPlan{Plan: "free"}, nil
		},
	}

	existing := &Note{ID: noteID, UserID: userID, UpdatedAt: baseVersion.Add(-time.Second)}
	upsertCalled := false

	repo := &mockRepository{
		findByIDFn: func(_ context.Context, _ uuid.UUID, _ uuid.UUID) (*Note, error) {
			if upsertCalled {
				return existing, nil
			}
			return existing, nil
		},
		upsertFn: func(_ context.Context, _ *Note) error {
			upsertCalled = true
			return nil
		},
	}

	svc := newServiceWithQuota(repo, usersRepo, limit)
	note, created, err := svc.UpsertNote(ctx, userID, noteID, []byte("data"), &baseVersion)
	require.NoError(t, err)
	require.NotNil(t, note)
	assert.False(t, created, "a versioned update must not be reported as created")
}

// TestUpsertNote_ConflictError_ReturnsConflictError verifies that a versioned
// update where the stored note's updated_at is newer than the provided baseVersion
// returns a *ConflictError containing the current server note.
func TestUpsertNote_ConflictError_ReturnsConflictError(t *testing.T) {
	ctx := context.Background()
	userID := uuid.New()
	noteID := uuid.New()
	baseVersion := time.Now().Add(-time.Minute) // client's stale version
	serverVersion := baseVersion.Add(time.Second)    // server is newer

	existing := &Note{ID: noteID, UserID: userID, UpdatedAt: serverVersion}

	repo := &mockRepository{
		findByIDFn: func(_ context.Context, _ uuid.UUID, _ uuid.UUID) (*Note, error) {
			return existing, nil
		},
	}

	svc := newServiceNoQuota(repo)
	_, _, err := svc.UpsertNote(ctx, userID, noteID, []byte("data"), &baseVersion)
	require.Error(t, err)

	var conflictErr *ConflictError
	require.ErrorAs(t, err, &conflictErr, "error must be a *ConflictError")
	assert.Equal(t, existing, conflictErr.ServerNote, "ConflictError must carry the current server note")
}

// TestUpsertNote_BetaUser_SkipsQuota verifies that a beta user can create notes
// beyond the free plan limit without triggering a quota check — beta is unlimited.
func TestUpsertNote_BetaUser_SkipsQuota(t *testing.T) {
	ctx := context.Background()
	userID := uuid.New()
	noteID := uuid.New()
	const limit = 3

	usersRepo := &mockUsersRepo{
		findByIDFn: func(_ context.Context, _ uuid.UUID) (UserPlan, error) {
			return UserPlan{Plan: "beta"}, nil
		},
	}

	savedNote := &Note{ID: noteID, UserID: userID}
	upsertCalled := false

	repo := &mockRepository{
		findByIDFn: func(_ context.Context, _ uuid.UUID, _ uuid.UUID) (*Note, error) {
			if upsertCalled {
				return savedNote, nil
			}
			return nil, ErrNoteNotFound
		},
		countLiveNotesFn: func(_ context.Context, _ uuid.UUID) (int64, error) {
			t.Error("CountLiveNotes must not be called for beta users")
			return int64(limit + 10), nil
		},
		upsertFn: func(_ context.Context, _ *Note) error {
			upsertCalled = true
			return nil
		},
	}

	svc := newServiceWithQuota(repo, usersRepo, limit)
	note, created, err := svc.UpsertNote(ctx, userID, noteID, []byte("data"), nil)
	require.NoError(t, err)
	require.NotNil(t, note)
	assert.True(t, created, "new note from a beta user must be reported as created")
}

// TestEnforceQuota_UserLookupFailure_PropagatesError verifies that when the
// users repository returns an error during a quota check the error is wrapped
// and propagated to the caller.
func TestEnforceQuota_UserLookupFailure_PropagatesError(t *testing.T) {
	ctx := context.Background()
	userID := uuid.New()
	noteID := uuid.New()
	lookupErr := errors.New("user db error")

	usersRepo := &mockUsersRepo{
		findByIDFn: func(_ context.Context, _ uuid.UUID) (UserPlan, error) {
			return UserPlan{}, lookupErr
		},
	}
	repo := &mockRepository{
		findByIDFn: func(_ context.Context, _ uuid.UUID, _ uuid.UUID) (*Note, error) {
			return nil, ErrNoteNotFound // new note
		},
	}

	svc := newServiceWithQuota(repo, usersRepo, 3)
	_, _, err := svc.UpsertNote(ctx, userID, noteID, []byte("data"), nil)
	require.Error(t, err)
	assert.ErrorIs(t, err, lookupErr)
}

// TestEnforceQuota_CountLiveNotes_Failure_PropagatesError verifies that when
// CountLiveNotes fails the error is wrapped and returned to the caller.
func TestEnforceQuota_CountLiveNotes_Failure_PropagatesError(t *testing.T) {
	ctx := context.Background()
	userID := uuid.New()
	noteID := uuid.New()
	countErr := errors.New("count query failed")

	usersRepo := &mockUsersRepo{
		findByIDFn: func(_ context.Context, _ uuid.UUID) (UserPlan, error) {
			return UserPlan{Plan: "free"}, nil
		},
	}
	repo := &mockRepository{
		findByIDFn: func(_ context.Context, _ uuid.UUID, _ uuid.UUID) (*Note, error) {
			return nil, ErrNoteNotFound
		},
		countLiveNotesFn: func(_ context.Context, _ uuid.UUID) (int64, error) {
			return 0, countErr
		},
	}

	svc := newServiceWithQuota(repo, usersRepo, 3)
	_, _, err := svc.UpsertNote(ctx, userID, noteID, []byte("data"), nil)
	require.Error(t, err)
	assert.ErrorIs(t, err, countErr)
}

// ---- GetNoteByID tests ----

// TestGetNoteByID_Found verifies that when the repository returns a note it is
// returned to the caller without modification.
func TestGetNoteByID_Found(t *testing.T) {
	ctx := context.Background()
	noteID := uuid.New()
	userID := uuid.New()
	expected := &Note{ID: noteID, UserID: userID}

	repo := &mockRepository{
		findByIDFn: func(_ context.Context, nid uuid.UUID, uid uuid.UUID) (*Note, error) {
			assert.Equal(t, noteID, nid)
			assert.Equal(t, userID, uid)
			return expected, nil
		},
	}

	svc := newServiceNoQuota(repo)
	got, err := svc.GetNoteByID(ctx, noteID, userID)
	require.NoError(t, err)
	assert.Equal(t, expected, got)
}

// TestGetNoteByID_NotFound_PropagatesErrNoteNotFound verifies that when the note
// does not exist ErrNoteNotFound is propagated to the caller.
func TestGetNoteByID_NotFound_PropagatesErrNoteNotFound(t *testing.T) {
	repo := &mockRepository{
		findByIDFn: func(_ context.Context, _ uuid.UUID, _ uuid.UUID) (*Note, error) {
			return nil, ErrNoteNotFound
		},
	}

	svc := newServiceNoQuota(repo)
	_, err := svc.GetNoteByID(context.Background(), uuid.New(), uuid.New())
	assert.ErrorIs(t, err, ErrNoteNotFound)
}

// TestGetNoteByID_RepoError_PropagatesError verifies that a generic repository
// error is propagated to the caller unchanged.
func TestGetNoteByID_RepoError_PropagatesError(t *testing.T) {
	dbErr := errors.New("db error")
	repo := &mockRepository{
		findByIDFn: func(_ context.Context, _ uuid.UUID, _ uuid.UUID) (*Note, error) {
			return nil, dbErr
		},
	}

	svc := newServiceNoQuota(repo)
	_, err := svc.GetNoteByID(context.Background(), uuid.New(), uuid.New())
	assert.ErrorIs(t, err, dbErr)
}

// ---- TrashNote tests ----

// TestTrashNote_Success verifies that TrashNote delegates to the repository
// and returns no error on success.
func TestTrashNote_Success(t *testing.T) {
	noteID := uuid.New()
	userID := uuid.New()
	called := false

	repo := &mockRepository{
		trashFn: func(_ context.Context, nid uuid.UUID, uid uuid.UUID) error {
			assert.Equal(t, noteID, nid)
			assert.Equal(t, userID, uid)
			called = true
			return nil
		},
	}

	svc := newServiceNoQuota(repo)
	err := svc.TrashNote(context.Background(), noteID, userID)
	require.NoError(t, err)
	assert.True(t, called)
}

// TestTrashNote_NotFound_PropagatesError verifies that ErrNoteNotFound from the
// repository is propagated to the caller.
func TestTrashNote_NotFound_PropagatesError(t *testing.T) {
	repo := &mockRepository{
		trashFn: func(_ context.Context, _ uuid.UUID, _ uuid.UUID) error {
			return ErrNoteNotFound
		},
	}

	svc := newServiceNoQuota(repo)
	err := svc.TrashNote(context.Background(), uuid.New(), uuid.New())
	assert.ErrorIs(t, err, ErrNoteNotFound)
}

// ---- RestoreNote tests ----

// TestRestoreNote_Success_ReturnsUpdatedNote verifies that RestoreNote calls
// Restore and then FindByID, returning the refreshed note.
func TestRestoreNote_Success_ReturnsUpdatedNote(t *testing.T) {
	noteID := uuid.New()
	userID := uuid.New()
	restored := &Note{ID: noteID, UserID: userID, EncryptedPayload: []byte("data")}

	repo := &mockRepository{
		restoreFn: func(_ context.Context, _ uuid.UUID, _ uuid.UUID) error { return nil },
		findByIDFn: func(_ context.Context, _ uuid.UUID, _ uuid.UUID) (*Note, error) {
			return restored, nil
		},
	}

	svc := newServiceNoQuota(repo)
	got, err := svc.RestoreNote(context.Background(), noteID, userID)
	require.NoError(t, err)
	assert.Equal(t, restored, got)
}

// TestRestoreNote_RestoreFailure_PropagatesError verifies that when repo.Restore
// returns an error it is propagated and FindByID is never called.
func TestRestoreNote_RestoreFailure_PropagatesError(t *testing.T) {
	restoreErr := errors.New("restore db error")

	repo := &mockRepository{
		restoreFn: func(_ context.Context, _ uuid.UUID, _ uuid.UUID) error {
			return restoreErr
		},
		findByIDFn: func(_ context.Context, _ uuid.UUID, _ uuid.UUID) (*Note, error) {
			require.Fail(t, "FindByID must not be called when Restore fails")
			return nil, nil
		},
	}

	svc := newServiceNoQuota(repo)
	_, err := svc.RestoreNote(context.Background(), uuid.New(), uuid.New())
	assert.ErrorIs(t, err, restoreErr)
}

// TestRestoreNote_FindAfterRestore_PropagatesError verifies that when Restore
// succeeds but the subsequent FindByID fails the error is propagated.
func TestRestoreNote_FindAfterRestore_PropagatesError(t *testing.T) {
	findErr := errors.New("find db error")

	repo := &mockRepository{
		restoreFn:  func(_ context.Context, _ uuid.UUID, _ uuid.UUID) error { return nil },
		findByIDFn: func(_ context.Context, _ uuid.UUID, _ uuid.UUID) (*Note, error) { return nil, findErr },
	}

	svc := newServiceNoQuota(repo)
	_, err := svc.RestoreNote(context.Background(), uuid.New(), uuid.New())
	assert.ErrorIs(t, err, findErr)
}

// ---- PurgeNote tests ----

// TestPurgeNote_Success verifies that PurgeNote delegates to the repository.
func TestPurgeNote_Success(t *testing.T) {
	noteID := uuid.New()
	userID := uuid.New()
	called := false

	repo := &mockRepository{
		purgeFn: func(_ context.Context, nid uuid.UUID, uid uuid.UUID) error {
			assert.Equal(t, noteID, nid)
			assert.Equal(t, userID, uid)
			called = true
			return nil
		},
	}

	svc := newServiceNoQuota(repo)
	err := svc.PurgeNote(context.Background(), noteID, userID)
	require.NoError(t, err)
	assert.True(t, called)
}

// TestPurgeNote_NotFound_PropagatesError verifies that ErrNoteNotFound from the
// repository is propagated to the caller.
func TestPurgeNote_NotFound_PropagatesError(t *testing.T) {
	repo := &mockRepository{
		purgeFn: func(_ context.Context, _ uuid.UUID, _ uuid.UUID) error {
			return ErrNoteNotFound
		},
	}

	svc := newServiceNoQuota(repo)
	err := svc.PurgeNote(context.Background(), uuid.New(), uuid.New())
	assert.ErrorIs(t, err, ErrNoteNotFound)
}

// ---- Full sync cursor edge case ----

// TestGetNotesSince_FullSync_WithCursor_NoTombstones verifies that when a
// cursor is present during a full sync (subsequent pages), tombstones are
// NOT fetched — the spec states tombstones only appear on the first full-sync page.
func TestGetNotesSince_FullSync_WithCursor_NoTombstones(t *testing.T) {
	ctx := context.Background()
	userID := uuid.New()
	cursor := &NoteCursor{AfterUpdatedAt: time.Now(), AfterNoteID: uuid.New()}

	tombstonesCalled := false

	repo := &mockRepository{
		findAllPaginatedFn: func(_ context.Context, _ uuid.UUID, _ int, _ time.Time, _ uuid.UUID) ([]Note, error) {
			return nil, nil
		},
		findAllTombstonesFn: func(_ context.Context, _ uuid.UUID) ([]NoteTombstone, error) {
			tombstonesCalled = true
			return []NoteTombstone{{NoteID: uuid.New()}}, nil
		},
	}

	svc := newServiceNoQuota(repo)
	_, tombstones, _, err := svc.GetNotesSince(ctx, userID, nil, cursor, deltaPageSize)
	require.NoError(t, err)
	assert.False(t, tombstonesCalled, "tombstones must not be fetched on subsequent full-sync pages")
	assert.Empty(t, tombstones)
}

// TestUpsertNote_ExistingNote_NoBaseVersion_SkipsQuota verifies that a re-push
// (PUT without base_version to a note_id that already exists) does not trigger
// a quota check — the user is not adding a new note.
func TestUpsertNote_ExistingNote_NoBaseVersion_SkipsQuota(t *testing.T) {
	ctx := context.Background()
	userID := uuid.New()
	noteID := uuid.New()
	const limit = 3

	usersRepo := &mockUsersRepo{
		findByIDFn: func(_ context.Context, _ uuid.UUID) (UserPlan, error) {
			return UserPlan{Plan: "free"}, nil
		},
	}

	existing := &Note{ID: noteID, UserID: userID, UpdatedAt: time.Now().Add(-time.Minute)}
	upsertCalled := false

	repo := &mockRepository{
		findByIDFn: func(_ context.Context, _ uuid.UUID, _ uuid.UUID) (*Note, error) {
			return existing, nil // note already exists
		},
		countLiveNotesFn: func(_ context.Context, _ uuid.UUID) (int64, error) {
			// Must not be called — this is an update, not a create.
			t.Error("CountLiveNotes must not be called when note already exists")
			return int64(limit), nil
		},
		upsertFn: func(_ context.Context, _ *Note) error {
			upsertCalled = true
			return nil
		},
	}

	svc := newServiceWithQuota(repo, usersRepo, limit)
	note, created, err := svc.UpsertNote(ctx, userID, noteID, []byte("data"), nil)
	require.NoError(t, err)
	require.NotNil(t, note)
	require.True(t, upsertCalled)
	assert.False(t, created, "a re-push of an existing note must not be reported as created")
}
