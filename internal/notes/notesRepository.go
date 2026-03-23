package notes

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type NotesRepository struct {
	db *gorm.DB
}

func NewNotesRepository(db *gorm.DB) *NotesRepository {
	return &NotesRepository{db: db}
}

func (r *NotesRepository) FindAll(ctx context.Context, userID uuid.UUID) ([]Note, error) {
	var notes []Note
	if err := r.db.WithContext(ctx).Where("user_id = ?", userID).Find(&notes).Error; err != nil {
		return nil, err
	}
	return notes, nil
}

// FindAllPaginated returns up to limit notes for the user, ordered by (updated_at ASC, note_id ASC).
// Pass zero values for afterUpdatedAt and afterNoteID to start from the beginning.
func (r *NotesRepository) FindAllPaginated(ctx context.Context, userID uuid.UUID, limit int, afterUpdatedAt time.Time, afterNoteID uuid.UUID) ([]Note, error) {
	var notes []Note
	q := r.db.WithContext(ctx).Where("user_id = ?", userID)
	if !afterUpdatedAt.IsZero() {
		q = q.Where("(updated_at > ? OR (updated_at = ? AND note_id > ?))", afterUpdatedAt, afterUpdatedAt, afterNoteID)
	}
	if err := q.Order("updated_at ASC, note_id ASC").Limit(limit).Find(&notes).Error; err != nil {
		return nil, err
	}
	return notes, nil
}

func (r *NotesRepository) FindUpdatedSince(ctx context.Context, userID uuid.UUID, since time.Time) ([]Note, error) {
	var notes []Note
	if err := r.db.WithContext(ctx).Where("user_id = ? AND updated_at > ?", userID, since).Find(&notes).Error; err != nil {
		return nil, err
	}
	return notes, nil
}

// FindUpdatedSincePaginated returns up to limit notes for the user whose updated_at is
// strictly greater than since, ordered by (updated_at ASC, note_id ASC).
// Pass zero values for afterUpdatedAt and afterNoteID to start from the first page.
// Pass the last page's updated_at and note_id to continue from a previous page.
func (r *NotesRepository) FindUpdatedSincePaginated(ctx context.Context, userID uuid.UUID, since time.Time, limit int, afterUpdatedAt time.Time, afterNoteID uuid.UUID) ([]Note, error) {
	var notes []Note
	q := r.db.WithContext(ctx).Where("user_id = ? AND updated_at > ?", userID, since)
	if !afterUpdatedAt.IsZero() {
		q = q.Where("(updated_at > ? OR (updated_at = ? AND note_id > ?))", afterUpdatedAt, afterUpdatedAt, afterNoteID)
	}
	if err := q.Order("updated_at ASC, note_id ASC").Limit(limit).Find(&notes).Error; err != nil {
		return nil, err
	}
	return notes, nil
}

func (r *NotesRepository) FindByID(ctx context.Context, noteID uuid.UUID, userID uuid.UUID) (*Note, error) {
	var note Note
	if err := r.db.WithContext(ctx).Where("note_id = ? AND user_id = ?", noteID, userID).First(&note).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrNoteNotFound
		}
		return nil, err
	}
	return &note, nil
}

func (r *NotesRepository) Upsert(ctx context.Context, note *Note) error {
	result := r.db.WithContext(ctx).Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "note_id"}},
		DoUpdates: clause.Assignments(map[string]any{
			"encrypted_payload": note.EncryptedPayload,
			"updated_at":        time.Now().UTC(),
		}),
		Where: clause.Where{Exprs: []clause.Expression{
			clause.Eq{
				Column: clause.Column{Table: "notes", Name: "user_id"},
				Value:  note.UserID,
			},
		}},
	}).Create(note)

	if result.Error != nil {
		return result.Error
	}
	// RowsAffected == 0 means note_id exists but belongs to a different user
	if result.RowsAffected == 0 {
		return ErrNoteNotFound
	}
	return nil
}

// Trash soft-deletes a note by setting trashed_at to the current time.
// Returns ErrNoteNotFound if no matching row exists for the given noteID and userID.
func (r *NotesRepository) Trash(ctx context.Context, noteID uuid.UUID, userID uuid.UUID) error {
	now := time.Now().UTC()
	result := r.db.WithContext(ctx).Model(&Note{}).
		Where("note_id = ? AND user_id = ?", noteID, userID).
		Updates(map[string]any{
			"trashed_at": now,
			"updated_at": now,
		})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return ErrNoteNotFound
	}
	return nil
}

// Restore clears trashed_at on a note, making it live again.
// Returns ErrNoteNotFound if no matching row exists for the given noteID and userID.
func (r *NotesRepository) Restore(ctx context.Context, noteID uuid.UUID, userID uuid.UUID) error {
	now := time.Now().UTC()
	// gorm.Expr("NULL") is required to force GORM to write a NULL value;
	// GORM silently skips nil pointer values in map updates.
	result := r.db.WithContext(ctx).Model(&Note{}).
		Where("note_id = ? AND user_id = ?", noteID, userID).
		Updates(map[string]any{
			"trashed_at": gorm.Expr("NULL"),
			"updated_at": now,
		})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return ErrNoteNotFound
	}
	return nil
}

// Purge hard-deletes a note row and creates a NoteTombstone within a single transaction.
// Returns ErrNoteNotFound if no matching row exists for the given noteID and userID.
func (r *NotesRepository) Purge(ctx context.Context, noteID uuid.UUID, userID uuid.UUID) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		result := tx.Where("note_id = ? AND user_id = ?", noteID, userID).Delete(&Note{})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected == 0 {
			return ErrNoteNotFound
		}
		tombstone := NoteTombstone{
			NoteID:    noteID,
			UserID:    userID,
			DeletedAt: time.Now().UTC(),
		}
		return tx.Create(&tombstone).Error
	})
}

// CountLiveNotes returns the number of non-trashed notes for a user.
// Trashed notes (trashed_at IS NOT NULL) are excluded from the count so that
// a trashed note does not consume quota.
func (r *NotesRepository) CountLiveNotes(ctx context.Context, userID uuid.UUID) (int64, error) {
	var count int64
	err := r.db.WithContext(ctx).Model(&Note{}).Where("user_id = ? AND trashed_at IS NULL", userID).Count(&count).Error
	return count, err
}

func (r *NotesRepository) FindTombstonesSince(ctx context.Context, userID uuid.UUID, since time.Time) ([]NoteTombstone, error) {
	var tombstones []NoteTombstone
	if err := r.db.WithContext(ctx).Where("user_id = ? AND deleted_at > ?", userID, since).Find(&tombstones).Error; err != nil {
		return nil, err
	}
	return tombstones, nil
}

// FindTombstonesSincePaginated returns up to limit tombstones for the user whose deleted_at
// is strictly greater than since, ordered by (deleted_at ASC, note_id ASC).
// Pass zero values for afterDeletedAt and afterNoteID to start from the first page.
// Pass the last page's deleted_at and note_id to continue from a previous page.
func (r *NotesRepository) FindTombstonesSincePaginated(ctx context.Context, userID uuid.UUID, since time.Time, limit int, afterDeletedAt time.Time, afterNoteID uuid.UUID) ([]NoteTombstone, error) {
	var tombstones []NoteTombstone
	q := r.db.WithContext(ctx).Where("user_id = ? AND deleted_at > ?", userID, since)
	if !afterDeletedAt.IsZero() {
		q = q.Where("(deleted_at > ? OR (deleted_at = ? AND note_id > ?))", afterDeletedAt, afterDeletedAt, afterNoteID)
	}
	if err := q.Order("deleted_at ASC, note_id ASC").Limit(limit).Find(&tombstones).Error; err != nil {
		return nil, err
	}
	return tombstones, nil
}

func (r *NotesRepository) FindAllTombstones(ctx context.Context, userID uuid.UUID) ([]NoteTombstone, error) {
	var tombstones []NoteTombstone
	if err := r.db.WithContext(ctx).Where("user_id = ?", userID).Find(&tombstones).Error; err != nil {
		return nil, err
	}
	return tombstones, nil
}

func (r *NotesRepository) PurgeExpiredTombstones(ctx context.Context, olderThan time.Time) (int64, error) {
	result := r.db.WithContext(ctx).Where("deleted_at < ?", olderThan).Delete(&NoteTombstone{})
	return result.RowsAffected, result.Error
}
