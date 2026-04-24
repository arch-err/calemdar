package model

import (
	"bufio"
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// splitFrontmatter separates the YAML frontmatter from the body of a
// markdown file. Frontmatter is delimited by lines containing only "---".
// The leading "---" must be the first line of the file.
//
// Returns (frontmatterBytes, bodyString, error). If no frontmatter is present,
// returns ("", fullContent, nil).
func splitFrontmatter(raw []byte) ([]byte, string, error) {
	scanner := bufio.NewScanner(bytes.NewReader(raw))
	scanner.Buffer(make([]byte, 1024*1024), 16*1024*1024)

	if !scanner.Scan() {
		return nil, "", nil
	}
	if strings.TrimRight(scanner.Text(), "\r\n") != "---" {
		// No frontmatter — return full content as body.
		return nil, string(raw), nil
	}

	var fm bytes.Buffer
	closed := false
	for scanner.Scan() {
		line := scanner.Text()
		if strings.TrimRight(line, "\r\n") == "---" {
			closed = true
			break
		}
		fm.WriteString(line)
		fm.WriteByte('\n')
	}
	if !closed {
		return nil, "", fmt.Errorf("frontmatter not closed")
	}

	var body bytes.Buffer
	for scanner.Scan() {
		body.WriteString(scanner.Text())
		body.WriteByte('\n')
	}
	if err := scanner.Err(); err != nil {
		return nil, "", err
	}
	return fm.Bytes(), body.String(), nil
}

// ParseRoot reads a recurring root file and populates a Root struct.
func ParseRoot(path string) (*Root, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", path, err)
	}
	fm, body, err := splitFrontmatter(raw)
	if err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	if fm == nil {
		return nil, fmt.Errorf("parse %s: no frontmatter", path)
	}

	var r Root
	if err := yaml.Unmarshal(fm, &r); err != nil {
		return nil, fmt.Errorf("parse %s: yaml: %w", path, err)
	}
	r.Body = body
	r.Path = path
	r.Slug = strings.TrimSuffix(filepath.Base(path), ".md")
	return &r, nil
}

// ParseEvent reads an expanded event (or one-off) file.
func ParseEvent(path string) (*Event, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", path, err)
	}
	fm, body, err := splitFrontmatter(raw)
	if err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	if fm == nil {
		return nil, fmt.Errorf("parse %s: no frontmatter", path)
	}

	var e Event
	if err := yaml.Unmarshal(fm, &e); err != nil {
		return nil, fmt.Errorf("parse %s: yaml: %w", path, err)
	}
	e.Body = body
	e.Path = path
	return &e, nil
}
