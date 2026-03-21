package notes

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
)

type repository interface {
	FindAll(ctx context.Context, userID uuid.UUID) ([]Note, error)
	FindAllPaginated(ctx context.Context, userID uuid.UUID, limit int, afterUpdatedAt time.Time, afterNoteID uuid.UUID) ([]Note, error)
	FindUpdatedSince(ctx context.Context, userID uuid.UUID, since time.Time) ([]Note, error)
	FindUpdatedSincePaginated(ctx context.Context, userID uuid.UUID, since time.Time, limit int, afterUpdatedAt time.Time, afterNoteID uuid.UUID) ([]Note, error)
	FindByID(ctx context.Context, noteID uuid.UUID, userID uuid.UUID) (*Note, error)
	Upsert(ctx context.Context, note *Note) error
	Trash(ctx context.Context, noteID uuid.UUID, userID uuid.UUID) error
	Restore(ctx context.Context, noteID uuid.UUID, userID uuid.UUID) error
	Purge(ctx context.Context, noteID uuid.UUID, userID uuid.UUID) error
	FindTombstonesSince(ctx context.Context, userID uuid.UUID, since time.Time) ([]NoteTombstone, error)
	FindTombstonesSincePaginated(ctx context.Context, userID uuid.UUID, since time.Time, limit int, afterDeletedAt time.Time, afterNoteID uuid.UUID) ([]NoteTombstone, error)
	FindAllTombstones(ctx context.Context, userID uuid.UUID) ([]NoteTombstone, error)
	PurgeExpiredTombstones(ctx context.Context, olderThan time.Time) (int64, error)
}

type NotesService struct {
	repository repository
}

func NewNotesService(repository repository) *NotesService {
	return &NotesService{repository: repository}
}

// GetNotesSince returns notes and tombstones for the user.
//
// When since is non-nil (delta sync), notes updated after that time are returned paginated
// using cursor-based keyset pagination ordered by (updated_at ASC, note_id ASC). Tombstones
// are likewise paginated by (deleted_at ASC, note_id ASC). Pass a non-nil cursor to continue
// from a previous page. hasMore is true when there are additional pages to fetch.
//
// When since is nil (full sync), notes are paginated using the same cursor-based mechanism
// but across all notes, and all tombstones are returned on the first page only.
func (s *NotesService) GetNotesSince(ctx context.Context, userID uuid.UUID, since *time.Time, cursor *NoteCursor, limit int) ([]Note, []NoteTombstone, bool, error) {
	afterUpdatedAt := time.Time{}
	afterNoteID := uuid.Nil
	if cursor != nil {
		afterUpdatedAt = cursor.AfterUpdatedAt
		afterNoteID = cursor.AfterNoteID
	}

	if since != nil {
		// Delta sync: paginate notes updated after since.
		notes, err := s.repository.FindUpdatedSincePaginated(ctx, userID, *since, limit+1, afterUpdatedAt, afterNoteID)
		if err != nil {
			return nil, nil, false, err
		}

		hasMore := len(notes) > limit
		if hasMore {
			notes = notes[:limit]
		}

		// Tombstones are paginated using the same cursor's note_id as keyset anchor, but
		// ordered by deleted_at. The cursor carries a single AfterUpdatedAt/AfterNoteID pair;
		// for delta sync we apply the same note_id boundary to tombstones as well so that both
		// collections advance together across pages.
		tombstones, err := s.repository.FindTombstonesSincePaginated(ctx, userID, *since, limit+1, afterUpdatedAt, afterNoteID)
		if err != nil {
			return nil, nil, false, err
		}
		hasTombstoneMore := len(tombstones) > limit
		if hasTombstoneMore {
			tombstones = tombstones[:limit]
			hasMore = true
		}

		return notes, tombstones, hasMore, nil
	}

	// Full sync: paginate notes, return all tombstones on first page only.
	notes, err := s.repository.FindAllPaginated(ctx, userID, limit+1, afterUpdatedAt, afterNoteID)
	if err != nil {
		return nil, nil, false, err
	}

	hasMore := len(notes) > limit
	if hasMore {
		notes = notes[:limit]
	}

	// Only fetch tombstones on the first page to avoid returning them repeatedly.
	var tombstones []NoteTombstone
	if cursor == nil {
		tombstones, err = s.repository.FindAllTombstones(ctx, userID)
		if err != nil {
			return nil, nil, false, err
		}
	}

	return notes, tombstones, hasMore, nil
}

func (s *NotesService) GetNoteByID(ctx context.Context, noteID uuid.UUID, userID uuid.UUID) (*Note, error) {
	return s.repository.FindByID(ctx, noteID, userID) // ErrNoteNotFound already surfaced
}

func (s *NotesService) UpsertNote(ctx context.Context, userID uuid.UUID, noteID uuid.UUID, encryptedPayload []byte, baseVersion *time.Time) (*Note, error) {
	if baseVersion != nil {
		existing, err := s.repository.FindByID(ctx, noteID, userID)
		if err != nil && !errors.Is(err, ErrNoteNotFound) {
			return nil, err
		}
		if existing != nil && existing.UpdatedAt.After(*baseVersion) {
			return nil, &ConflictError{ServerNote: existing}
		}
	}

	note := &Note{
		ID:               noteID,
		UserID:           userID,
		EncryptedPayload: encryptedPayload,
	}

	if err := s.repository.Upsert(ctx, note); err != nil {
		return nil, err
	}

	return s.repository.FindByID(ctx, noteID, userID)
}

// TrashNote soft-deletes a note by setting its trashed_at timestamp.
func (s *NotesService) TrashNote(ctx context.Context, noteID uuid.UUID, userID uuid.UUID) error {
	return s.repository.Trash(ctx, noteID, userID)
}

// RestoreNote clears the trashed_at timestamp and returns the updated note.
func (s *NotesService) RestoreNote(ctx context.Context, noteID uuid.UUID, userID uuid.UUID) (*Note, error) {
	if err := s.repository.Restore(ctx, noteID, userID); err != nil {
		return nil, err
	}
	return s.repository.FindByID(ctx, noteID, userID)
}

// PurgeNote hard-deletes a note and creates a tombstone for sync clients.
func (s *NotesService) PurgeNote(ctx context.Context, noteID uuid.UUID, userID uuid.UUID) error {
	return s.repository.Purge(ctx, noteID, userID)
}
