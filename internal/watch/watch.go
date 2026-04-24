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
	selfWrites map[string]time.Time

	pendingMu sync.Mutex
	pending   map[string]*pendingEvent
}

type pendingEvent struct {
	timer *time.Timer
	kind  Kind
	src   Source
}

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
		ttl:        2 * time.Second,
		selfWrites: make(map[string]time.Time),
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
	return w, nil
}

// Events returns the channel of coalesced events.
func (w *Watcher) Events() <-chan Event { return w.events }

// Stop closes the watcher and its events channel.
func (w *Watcher) Stop() error {
	close(w.done)
	return w.fs.Close()
}

// NotifySelfWrite records a server-initiated write. Raw fsnotify events on
// this path within ttl are suppressed.
func (w *Watcher) NotifySelfWrite(path string) {
	w.selfMu.Lock()
	w.selfWrites[path] = time.Now()
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
			_ = w.addTree(ev.Name)
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
		p.timer.Stop()
		p.kind = kind
		p.src = src
		p.timer = time.AfterFunc(w.debounce, func() { w.fire(path) })
		return
	}
	p := &pendingEvent{kind: kind, src: src}
	p.timer = time.AfterFunc(w.debounce, func() { w.fire(path) })
	w.pending[path] = p
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

func (w *Watcher) flushPending() {
	w.pendingMu.Lock()
	defer w.pendingMu.Unlock()
	for _, p := range w.pending {
		p.timer.Stop()
	}
	w.pending = map[string]*pendingEvent{}
	close(w.events)
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
	t, ok := w.selfWrites[path]
	if !ok {
		return false
	}
	if time.Since(t) > w.ttl {
		delete(w.selfWrites, path)
		return false
	}
	return true
}
