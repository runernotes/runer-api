package notes

import (
	"time"

	"github.com/google/uuid"
)

type Note struct {
	ID               uuid.UUID  `gorm:"column:note_id;primaryKey;type:uuid"`
	UserID           uuid.UUID  `gorm:"column:user_id;type:uuid;not null;index:idx_user_updated,priority:1"`
	EncryptedPayload []byte     `gorm:"column:encrypted_payload;type:bytea;not null"`
	CreatedAt        time.Time  `gorm:"autoCreateTime"`
	UpdatedAt        time.Time  `gorm:"autoUpdateTime;index:idx_user_updated,priority:2"`
	TrashedAt        *time.Time `gorm:"column:trashed_at;index"`
}

type NoteTombstone struct {
	NoteID    uuid.UUID `gorm:"column:note_id;primaryKey;type:uuid"`
	UserID    uuid.UUID `gorm:"column:user_id;type:uuid;not null;index"`
	DeletedAt time.Time `gorm:"not null"`
}

// NoteCursor is the position marker for cursor-based pagination of notes,
// ordered by (updated_at ASC, note_id ASC).
type NoteCursor struct {
	AfterUpdatedAt time.Time `json:"updated_at"`
	AfterNoteID    uuid.UUID `json:"note_id"`
}
