// Package watch recursively watches the vault's recurring/ and events/
// trees via fsnotify, coalesces bursts through a per-path debounce, and
// suppresses events triggered by the server's own writes.
package watch

import (
	"errors"
	"fmt"
	"io/fs"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/arch-err/calemdar/internal/vault"
	"github.com/fsnotify/fsnotify"
)

// Source tells which tree a path comes from.
type Source int

const (
	SourceRecurring Source = iota
	SourceEvents
)

func (s Source) String() string {
	switch s {
	case SourceRecurring:
		return "recurring"
	case SourceEvents:
		return "events"
	}
	return "?"
}

// Kind tells whether a file exists (Changed) or has been removed (Deleted).
type Kind int

const (
	Changed Kind = iota
	Deleted
)

// Event is a coalesced, source-tagged filesystem change.
type Event struct {
	Kind   Kind
	Source Source
	Path   string
}

// Watcher fans fsnotify events into semantic Event values on a single channel.
type Watcher struct {
	v        *vault.Vault
	fs       *fsnotify.Watcher
	events   chan Event
	done     chan struct{}
	debounce time.Duration
	ttl      time.Duration // how long to suppress self-write events

	selfMu     sync.Mutex
	selfWrites map[string]selfWriteMark

	pendingMu sync.Mutex
	pending   map[string]*pendingEvent

	// inFlight tracks scheduled-but-unfinished debounce-fire goroutines.
	// Stop drains it before closing w.events so a parallel send doesn't
	// hit a closed channel.
	inFlight sync.WaitGroup
}

type pendingEvent struct {
	timer *time.Timer
	kind  Kind
	src   Source
}

// selfWriteMark remembers the post-write mtime so external edits (which
// change mtime) can be distinguished from our own writes.
type selfWriteMark struct {
	mtime    time.Time
	at       time.Time
	isRemove bool
}

// raceWindow is the grace period within which we suppress events even on
// mtime mismatch. Bulk-fanout reconciles (260+ files in a tight loop)
// occasionally see the kernel finalise mtime after our post-write stat,
// leading to false-positive external-edit detection on every single
// expanded occurrence. A few seconds of trust covers the race without
// silently absorbing genuine user edits — those happen seconds-to-hours
// after a write, not in the same fsnotify burst.
const raceWindow = 3 * time.Second

// Start begins watching. Returns immediately; events flow on Events().
func Start(v *vault.Vault) (*Watcher, error) {
	return StartWithDebounce(v, 500*time.Millisecond)
}

// StartWithDebounce is Start but with a configurable debounce duration.
func StartWithDebounce(v *vault.Vault, debounce time.Duration) (*Watcher, error) {
	fsw, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, fmt.Errorf("watch: new: %w", err)
	}
	w := &Watcher{
		v:          v,
		fs:         fsw,
		events:     make(chan Event, 64),
		done:       make(chan struct{}),
		debounce:   debounce,
		ttl:        30 * time.Second,
		selfWrites: make(map[string]selfWriteMark),
		pending:    make(map[string]*pendingEvent),
	}

	if err := w.addTree(v.RecurringDir()); err != nil {
		fsw.Close()
		return nil, err
	}
	if err := w.addTree(v.EventsDir()); err != nil {
		fsw.Close()
		return nil, err
	}

	go w.loop()
	go w.gcLoop()
	return w, nil
}

// Events returns the channel of coalesced events.
func (w *Watcher) Events() <-chan Event { return w.events }

// Stop closes the watcher cleanly. Order matters: signal goroutines via
// done, close fsnotify, drain in-flight fires, then close the events
// channel. Each step ensures the next can run without races.
func (w *Watcher) Stop() error {
	close(w.done)
	err := w.fs.Close()
	w.inFlight.Wait()
	close(w.events)
	return err
}

// NotifySelfWrite records a server-initiated write. Must be called AFTER
// the file operation completes so we can stat the new mtime. Subsequent
// raw events on this path whose mtime matches ours are suppressed; events
// whose mtime differs (an external edit) are NOT suppressed.
//
// For removes, the file is gone; we mark it as a removal and use TTL-only
// suppression for any immediate follow-up event.
func (w *Watcher) NotifySelfWrite(path string) {
	info, err := os.Stat(path)
	mark := selfWriteMark{at: time.Now()}
	if err != nil {
		mark.isRemove = true
	} else {
		mark.mtime = info.ModTime()
	}
	w.selfMu.Lock()
	w.selfWrites[path] = mark
	w.selfMu.Unlock()
}

// NotifySelfDelete records a server-initiated delete. MUST be called
// before the os.Remove syscall — once the inode is gone there's nothing
// to stat, so we mark TTL-only and let isRecentSelfWrite suppress the
// resulting DELETE event.
func (w *Watcher) NotifySelfDelete(path string) {
	mark := selfWriteMark{at: time.Now(), isRemove: true}
	w.selfMu.Lock()
	w.selfWrites[path] = mark
	w.selfMu.Unlock()
}

// ---------- internals ----------

func (w *Watcher) addTree(root string) error {
	if _, err := os.Stat(root); errors.Is(err, os.ErrNotExist) {
		// Create the dir so we can watch it. Helps first-launch UX.
		if err := os.MkdirAll(root, 0o755); err != nil {
			return err
		}
	}
	return filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() {
			return nil
		}
		return w.fs.Add(path)
	})
}

// gcLoop periodically sweeps the selfWrites map for expired marks. The
// hot-path isRecentSelfWrite only prunes the entry it looked up, so
// paths that never re-fire would otherwise leak forever (long-running
// daemons creating then never re-touching files). Cheap full-map walk
// at the same cadence as the suppression TTL.
func (w *Watcher) gcLoop() {
	ticker := time.NewTicker(w.ttl)
	defer ticker.Stop()
	for {
		select {
		case <-w.done:
			return
		case <-ticker.C:
			w.pruneSelfWrites()
		}
	}
}

func (w *Watcher) pruneSelfWrites() {
	w.selfMu.Lock()
	defer w.selfMu.Unlock()
	now := time.Now()
	for k, m := range w.selfWrites {
		if now.Sub(m.at) > w.ttl {
			delete(w.selfWrites, k)
		}
	}
}

func (w *Watcher) loop() {
	for {
		select {
		case <-w.done:
			w.flushPending()
			return
		case ev, ok := <-w.fs.Events:
			if !ok {
				return
			}
			w.onRaw(ev)
		case err, ok := <-w.fs.Errors:
			if !ok {
				return
			}
			log.Printf("watch: fsnotify error: %v", err)
		}
	}
}

func (w *Watcher) onRaw(ev fsnotify.Event) {
	// Track new subdirectories so their contents are watched too. Walk
	// recursively: `mkdir -p events/life/2026` can deliver the outer dir
	// event before we finish registering the inner one.
	if ev.Op&fsnotify.Create != 0 {
		if info, err := os.Stat(ev.Name); err == nil && info.IsDir() {
			if err := w.addTree(ev.Name); err != nil {
				log.Printf("watch: addTree %s: %v", ev.Name, err)
			}
		}
	}

	// We only care about .md files.
	if !strings.HasSuffix(ev.Name, ".md") {
		return
	}
	src, ok := w.classify(ev.Name)
	if !ok {
		return
	}

	// Suppress self-writes.
	if w.isRecentSelfWrite(ev.Name) {
		return
	}

	kind := Changed
	if ev.Op&fsnotify.Remove != 0 || ev.Op&fsnotify.Rename != 0 {
		kind = Deleted
	}
	w.enqueue(src, kind, ev.Name)
}

func (w *Watcher) enqueue(src Source, kind Kind, path string) {
	w.pendingMu.Lock()
	defer w.pendingMu.Unlock()

	if p, ok := w.pending[path]; ok {
		// Reset existing timer. Stop returns true iff the existing timer
		// was still pending; if it had already fired, the existing fire
		// goroutine will Done itself, so we only Add for the replacement
		// when Stop succeeded.
		if p.timer.Stop() {
			// keep inFlight counter even — replace the cancelled timer.
		} else {
			w.inFlight.Add(1)
		}
		p.kind = kind
		p.src = src
		p.timer = time.AfterFunc(w.debounce, w.firer(path))
		return
	}
	w.inFlight.Add(1)
	p := &pendingEvent{kind: kind, src: src}
	p.timer = time.AfterFunc(w.debounce, w.firer(path))
	w.pending[path] = p
}

// firer wraps fire(path) with the inFlight Done so every scheduled
// goroutine balances its Add. Used by enqueue when scheduling timers.
func (w *Watcher) firer(path string) func() {
	return func() {
		defer w.inFlight.Done()
		w.fire(path)
	}
}

func (w *Watcher) fire(path string) {
	w.pendingMu.Lock()
	p, ok := w.pending[path]
	if !ok {
		w.pendingMu.Unlock()
		return
	}
	delete(w.pending, path)
	w.pendingMu.Unlock()

	// Re-check self-write at fire time. There's a race where the raw event
	// can arrive before NotifySelfWrite has stored its mark; by the time the
	// debounce timer fires (500ms later), the mark is always set, so we can
	// catch and drop the false-external emission here.
	if w.isRecentSelfWrite(path) {
		return
	}

	// Refine Changed vs Deleted by checking existence at fire time — catches
	// e.g. REMOVE + CREATE in rapid succession (atomic rename on save).
	kind := p.kind
	if _, err := os.Stat(path); err == nil {
		kind = Changed
	} else {
		kind = Deleted
	}

	select {
	case w.events <- Event{Kind: kind, Source: p.src, Path: path}:
	case <-w.done:
	}
}

// flushPending drops every queued debounce timer and cleans the pending
// map. Each timer that hadn't yet fired (Stop returns true) had its
// inFlight counter incremented when it was scheduled, so we Done it
// here. Timers that already fired (Stop returns false) will Done
// themselves when the goroutine finishes.
//
// We intentionally do NOT close w.events here: Stop is responsible for
// the close, and only after inFlight has drained — otherwise an
// in-flight fire goroutine sending on the channel races a close and
// panics.
func (w *Watcher) flushPending() {
	w.pendingMu.Lock()
	defer w.pendingMu.Unlock()
	for _, p := range w.pending {
		if p.timer.Stop() {
			w.inFlight.Done()
		}
	}
	w.pending = map[string]*pendingEvent{}
}

func (w *Watcher) classify(path string) (Source, bool) {
	recDir := w.v.RecurringDir()
	evDir := w.v.EventsDir()
	switch {
	case strings.HasPrefix(path, recDir+string(os.PathSeparator)) || path == recDir:
		return SourceRecurring, true
	case strings.HasPrefix(path, evDir+string(os.PathSeparator)) || path == evDir:
		return SourceEvents, true
	}
	return 0, false
}

func (w *Watcher) isRecentSelfWrite(path string) bool {
	w.selfMu.Lock()
	defer w.selfMu.Unlock()
	m, ok := w.selfWrites[path]
	if !ok {
		return false
	}
	if time.Since(m.at) > w.ttl {
		delete(w.selfWrites, path)
		return false
	}
	if m.isRemove {
		return true
	}
	info, err := os.Stat(path)
	if err != nil {
		// File gone — either the user deleted it or our own follow-up event.
		// Treat as suppressed to avoid false "external edit" detection.
		return true
	}
	if info.ModTime().Equal(m.mtime) {
		return true
	}
	// mtime mismatch can still be a self-write under bulk-fanout races: the
	// kernel sometimes finalises mtime fractionally after our post-write
	// stat, so a follow-up stat sees a different value. Inside a short race
	// window after the mark, trust the path-level signal regardless of
	// mtime. Past the window, mtime mismatch genuinely means something
	// external rewrote the file.
	if time.Since(m.at) < raceWindow {
		return true
	}
	return false
}
