// Package notify schedules and delivers per-event notifications.
//
// The package has two halves:
//
//   - Backends: pluggable senders (ntfy, system) that take a Notification
//     and put it in front of a human. New backends register through
//     Register() so the scheduler doesn't grow knowledge of each one.
//
//   - Scheduler: a tick loop owned by the serve daemon. Every minute it
//     queries the SQLite cache for events with notify rules whose
//     fire-at falls in the most recent tick window. Dedupe lives in a
//     persistent table so daemon restarts don't replay history.
package notify

import (
	"context"
)

// Notification is the cross-backend payload. Built once per fire by the
// scheduler from the event + per-rule context, then handed to each
// backend selected via the rule's `via:` list.
type Notification struct {
	// Title shown in the push (e.g. "calemdar: upcoming").
	Title string
	// Body is the human-readable line. Short — phones truncate.
	Body string
	// Tags hint UI/icon classification (calendar, severity).
	Tags []string

	// Per-event metadata. Backends generally don't need these, but the
	// action runner does. Carried here so scheduler can hand a single
	// struct to both sides.
	EventTitle string
	EventDate  string
	EventStart string
	EventEnd   string
	EventPath  string
	Calendar   string
	Lead       string // original lead string from the rule, e.g. "5m"
}

// Backend is the contract every notif sender implements.
type Backend interface {
	// Name is the short identifier used in `via:` lists.
	Name() string
	// Send delivers n. Returns an error if delivery failed; the
	// scheduler logs and moves on — no retries in v1.
	Send(ctx context.Context, n Notification) error
}
