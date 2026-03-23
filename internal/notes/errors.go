package notes

import "errors"

var ErrNoteNotFound = errors.New("note not found")

// ErrQuotaExceeded is returned by UpsertNote when a free-plan user attempts to
// create a new note beyond their plan limit.
var ErrQuotaExceeded = errors.New("quota exceeded")

// ConflictError is returned when the server's stored version is newer than the
// client's base version. It carries the current server note so the handler can
// return it in the 409 body, allowing the client to resolve the conflict and
// re-push without an extra round trip.
type ConflictError struct {
	ServerNote *Note
}

func (e *ConflictError) Error() string {
	return "conflict: server version is newer"
}
