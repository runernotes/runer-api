package notes

import (
	"encoding/base64"
	"time"

	"github.com/google/uuid"
)

type UpsertNoteRequest struct {
	EncryptedPayload string     `json:"encrypted_payload" validate:"required"`
	BaseVersion      *time.Time `json:"base_version"` // the updated_at the client last saw; omit on first write
}

type NoteResponse struct {
	NoteID           uuid.UUID  `json:"note_id"`
	EncryptedPayload string     `json:"encrypted_payload"`
	CreatedAt        *time.Time `json:"created_at"`
	UpdatedAt        *time.Time `json:"updated_at"`
	TrashedAt        *time.Time `json:"trashed_at"`
}

type TombstoneResponse struct {
	NoteID    uuid.UUID  `json:"note_id"`
	DeletedAt *time.Time `json:"deleted_at"`
}

type NotesListResponse struct {
	Notes      []NoteResponse      `json:"notes"`
	Tombstones []TombstoneResponse `json:"tombstones"`
	NextCursor *string             `json:"next_cursor"` // non-null when more pages exist
	ServerTime *time.Time          `json:"server_time"`
}

func toNoteResponse(n *Note) NoteResponse {
	return NoteResponse{
		NoteID:           n.ID,
		EncryptedPayload: base64.StdEncoding.EncodeToString(n.EncryptedPayload),
		CreatedAt:        &n.CreatedAt,
		UpdatedAt:        &n.UpdatedAt,
		TrashedAt:        n.TrashedAt,
	}
}

func toTombstoneResponse(t NoteTombstone) TombstoneResponse {
	return TombstoneResponse{
		NoteID:    t.NoteID,
		DeletedAt: &t.DeletedAt,
	}
}
