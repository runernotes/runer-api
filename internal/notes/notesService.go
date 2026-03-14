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
	FindByID(ctx context.Context, noteID uuid.UUID, userID uuid.UUID) (*Note, error)
	Upsert(ctx context.Context, note *Note) error
	Delete(ctx context.Context, noteID uuid.UUID, userID uuid.UUID) error
	FindTombstonesSince(ctx context.Context, userID uuid.UUID, since time.Time) ([]NoteTombstone, error)
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
// When since is non-nil (delta sync), all notes updated after that time are returned with no
// pagination — the result set is expected to be small.
//
// When since is nil (full sync), notes are paginated using cursor-based pagination ordered by
// (updated_at ASC, note_id ASC). Pass a non-nil cursor to continue from a previous page.
// Tombstones are always returned in full (they carry no payload and are small).
// hasMore is true when there are additional pages to fetch.
func (s *NotesService) GetNotesSince(ctx context.Context, userID uuid.UUID, since *time.Time, cursor *NoteCursor, limit int) ([]Note, []NoteTombstone, bool, error) {
	if since != nil {
		notes, err := s.repository.FindUpdatedSince(ctx, userID, *since)
		if err != nil {
			return nil, nil, false, err
		}
		tombstones, err := s.repository.FindTombstonesSince(ctx, userID, *since)
		if err != nil {
			return nil, nil, false, err
		}
		return notes, tombstones, false, nil
	}

	// Full sync: paginate notes, return all tombstones.
	afterUpdatedAt := time.Time{}
	afterNoteID := uuid.Nil
	if cursor != nil {
		afterUpdatedAt = cursor.AfterUpdatedAt
		afterNoteID = cursor.AfterNoteID
	}

	// Fetch one extra to determine if there is a next page.
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
			return nil, ErrConflict
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

func (s *NotesService) DeleteNote(ctx context.Context, noteID uuid.UUID, userID uuid.UUID) error {
	return s.repository.Delete(ctx, noteID, userID)
}
