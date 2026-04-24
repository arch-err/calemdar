// Package writer serializes Events to disk in the Full Calendar frontmatter
// format.
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

// WriteEvent writes e to e.Path with YAML frontmatter + body. Creates parent
// directories as needed. Overwrites any existing file.
func WriteEvent(e *model.Event) error {
	if e.Path == "" {
		return fmt.Errorf("write: event.Path empty")
	}
	return writeMarkdown(e.Path, e, e.Body)
}

// WriteRoot writes r to r.Path with YAML frontmatter + body.
func WriteRoot(r *model.Root) error {
	if r.Path == "" {
		return fmt.Errorf("write: root.Path empty")
	}
	return writeMarkdown(r.Path, r, r.Body)
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
