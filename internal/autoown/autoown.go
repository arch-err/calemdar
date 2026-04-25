// Package autoown flips an event's user-owned flag to true when the file
// has been modified outside the server. Idempotent.
package autoown

import (
	"time"

	"github.com/arch-err/calemdar/internal/model"
	"github.com/arch-err/calemdar/internal/writer"
)

// FreshExpansionWindow is how recent an event's `series-expanded-at`
// stamp can be before we treat the on-disk file as a server-write
// rather than a user edit.
//
// Why a window: the daemon's reconcile writes events in a tight loop.
// The fsnotify CREATE for each one races the watcher's per-process
// self-write suppression — and across processes (CLI `extend` vs
// daemon serve) suppression doesn't apply at all. We need a signal
// that's robust to who wrote the file.
//
// `series-expanded-at` is set by the expander on every fresh write
// and is preserved verbatim across user edits — users edit body /
// frontmatter values, not this stamp. So a recent stamp is a strong
// "this came from us" signal regardless of process.
const FreshExpansionWindow = 60 * time.Second

// FlipIfNeeded reads the event at path; if user-owned is false AND the
// file isn't a freshly-expanded daemon write, sets the flag to true
// and writes back. Returns true iff the flag was flipped.
func FlipIfNeeded(path string) (bool, error) {
	e, err := model.ParseEvent(path)
	if err != nil {
		return false, err
	}
	if e.UserOwned {
		return false, nil
	}
	if isFreshExpansion(e) {
		return false, nil
	}
	e.UserOwned = true
	if err := writer.WriteEvent(e); err != nil {
		return false, err
	}
	return true, nil
}

// isFreshExpansion reports whether e was written by an expander
// recently enough that we should not interpret it as a user edit.
// Empty / unparseable timestamps fall through (treated as not fresh
// — true one-offs and pre-stamp events still get autoown'd correctly).
func isFreshExpansion(e *model.Event) bool {
	if e.SeriesExpandedAt == "" {
		return false
	}
	t, err := time.Parse(time.RFC3339, e.SeriesExpandedAt)
	if err != nil {
		return false
	}
	return time.Since(t) < FreshExpansionWindow
}
