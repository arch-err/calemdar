// Package writer serializes Events (and Roots) to disk with YAML frontmatter.
// It also exposes a SelfWriteNotifier hook so other parts of the codebase can
// tell the fsnotify watcher "this change is mine, don't react to it".
package writer

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/arch-err/calemdar/internal/model"
	"gopkg.in/yaml.v3"
)

// SelfWriteNotifier, when non-nil, is called with the path before any file
// operation so the caller (typically the fsnotify watcher) can suppress the
// raw event it's about to see.
var SelfWriteNotifier func(path string)

// SelfDeleteNotifier, when non-nil, is called BEFORE a self-initiated
// os.Remove on path so the watcher can suppress the resulting DELETE
// event. Callers using a writer-level delete (e.g. the recurring delete
// CLI) should invoke NotifySelfDelete just before the os.Remove syscall.
//
// Why a separate hook from SelfWriteNotifier: writes can be detected
// post-syscall by comparing mtime; deletes leave nothing to stat, so the
// watcher needs the heads-up before the inode disappears.
var SelfDeleteNotifier func(path string)

// NotifySelf calls the notifier if set. Callers doing a non-writer file op
// (os.Remove, os.Rename) should invoke this before the syscall.
func NotifySelf(path string) {
	if SelfWriteNotifier != nil {
		SelfWriteNotifier(path)
	}
}

// NotifySelfDelete marks path as about-to-be-deleted by ourselves. Must be
// called before the os.Remove syscall. If no SelfDeleteNotifier is wired
// (e.g. CLI process with no daemon), this falls back to NotifySelf so the
// in-process watcher (if any) still gets the heads-up.
func NotifySelfDelete(path string) {
	if SelfDeleteNotifier != nil {
		SelfDeleteNotifier(path)
		return
	}
	NotifySelf(path)
}

// WriteEvent writes e to e.Path with YAML frontmatter + body. Creates parent
// directories as needed. Overwrites any existing file. Notifies the watcher
// of the self-write after the syscall completes (so the post-write mtime
// can be captured for accurate suppression).
func WriteEvent(e *model.Event) error {
	if e.Path == "" {
		return fmt.Errorf("write: event.Path empty")
	}
	if err := writeMarkdown(e.Path, e, e.Body); err != nil {
		return err
	}
	NotifySelf(e.Path)
	return nil
}

// WriteRoot writes r to r.Path with YAML frontmatter + body. See WriteEvent.
func WriteRoot(r *model.Root) error {
	if r.Path == "" {
		return fmt.Errorf("write: root.Path empty")
	}
	if err := writeMarkdown(r.Path, r, r.Body); err != nil {
		return err
	}
	NotifySelf(r.Path)
	return nil
}

func writeMarkdown(path string, fmStruct any, body string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("write: mkdir: %w", err)
	}

	var fm bytes.Buffer
	enc := yaml.NewEncoder(&fm)
	enc.SetIndent(2)
	if err := enc.Encode(fmStruct); err != nil {
		return fmt.Errorf("write: yaml: %w", err)
	}
	if err := enc.Close(); err != nil {
		return fmt.Errorf("write: yaml close: %w", err)
	}

	var out strings.Builder
	out.WriteString("---\n")
	out.Write(fm.Bytes())
	out.WriteString("---\n\n")
	out.WriteString(body)
	if !strings.HasSuffix(body, "\n") {
		out.WriteString("\n")
	}
	return os.WriteFile(path, []byte(out.String()), 0o644)
}
