// Package autoown flips an event's user-owned flag to true when the file
// has been modified outside the server. Idempotent.
package autoown

import (
	"github.com/arch-err/calemdar/internal/model"
	"github.com/arch-err/calemdar/internal/writer"
)

// FlipIfNeeded reads the event at path; if user-owned is false, sets it to
// true and writes back. Returns true iff the flag was flipped.
func FlipIfNeeded(path string) (bool, error) {
	e, err := model.ParseEvent(path)
	if err != nil {
		return false, err
	}
	if e.UserOwned {
		return false, nil
	}
	e.UserOwned = true
	if err := writer.WriteEvent(e); err != nil {
		return false, err
	}
	return true, nil
}
