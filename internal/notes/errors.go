package notes

import "errors"

var (
	ErrNoteNotFound = errors.New("note not found")
	ErrConflict     = errors.New("conflict: server version is newer")
)
