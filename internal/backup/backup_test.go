package backup

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/arch-err/calemdar/internal/vault"
)

func newVault(t *testing.T) *vault.Vault {
	t.Helper()
	return &vault.Vault{Root: t.TempDir()}
}

func TestWriteFromBytesAndList(t *testing.T) {
	v := newVault(t)
	t1 := time.Date(2026, 4, 25, 10, 0, 0, 0, time.UTC)
	t2 := t1.Add(time.Hour)

	if _, err := WriteFromBytes(v, "workout", []byte("---\nslug: workout\n---\nbody1\n"), t1); err != nil {
		t.Fatal(err)
	}
	if _, err := WriteFromBytes(v, "workout", []byte("---\nslug: workout\n---\nbody2\n"), t2); err != nil {
		t.Fatal(err)
	}
	if _, err := WriteFromBytes(v, "standup", []byte("---\nslug: standup\n---\n"), t1); err != nil {
		t.Fatal(err)
	}

	all, err := List(v)
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != 3 {
		t.Fatalf("got %d backups, want 3", len(all))
	}
	// standup sorts before workout; within workout newest-first.
	if all[0].Slug != "standup" {
		t.Errorf("expected standup first, got %s", all[0].Slug)
	}
	if all[1].Slug != "workout" || !all[1].When.Equal(t2) {
		t.Errorf("expected workout @ t2 second, got %+v", all[1])
	}
	if all[2].Slug != "workout" || !all[2].When.Equal(t1) {
		t.Errorf("expected workout @ t1 third, got %+v", all[2])
	}
}

func TestLatestForSlug(t *testing.T) {
	v := newVault(t)
	t1 := time.Date(2026, 4, 25, 10, 0, 0, 0, time.UTC)
	t2 := t1.Add(time.Hour)

	_, _ = WriteFromBytes(v, "workout", []byte("a"), t1)
	_, _ = WriteFromBytes(v, "workout", []byte("b"), t2)

	got, err := LatestForSlug(v, "workout")
	if err != nil {
		t.Fatal(err)
	}
	if got == nil {
		t.Fatal("nil entry")
	}
	if !got.When.Equal(t2) {
		t.Errorf("When = %v, want %v", got.When, t2)
	}

	miss, err := LatestForSlug(v, "nosuch")
	if err != nil {
		t.Fatal(err)
	}
	if miss != nil {
		t.Errorf("expected nil for unknown slug, got %+v", miss)
	}
}

func TestWriteFromFileRoundTrip(t *testing.T) {
	v := newVault(t)
	src := filepath.Join(t.TempDir(), "workout.md")
	content := []byte("---\nid: abc\n---\nhello\n")
	if err := os.WriteFile(src, content, 0o644); err != nil {
		t.Fatal(err)
	}

	dst, err := WriteFromFile(v, "workout", src, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(dst)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(content) {
		t.Errorf("contents differ:\nsrc: %q\ndst: %q", content, got)
	}
}

func TestParseEntryRejectsMalformed(t *testing.T) {
	v := newVault(t)
	dir := Dir(v)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	// File without the expected timestamp shape — should be skipped.
	if err := os.WriteFile(filepath.Join(dir, "junk.md"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	// Valid file alongside.
	if _, err := WriteFromBytes(v, "ok", []byte("y"), time.Now()); err != nil {
		t.Fatal(err)
	}

	all, err := List(v)
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != 1 || all[0].Slug != "ok" {
		t.Errorf("expected [ok], got %+v", all)
	}
}

func TestFilenameStampNoColons(t *testing.T) {
	t1 := time.Date(2026, 4, 25, 10, 30, 45, 0, time.UTC)
	got := Filename("workout", t1)
	if strings.Contains(got, ":") {
		t.Errorf("filename contains colon: %q", got)
	}
	if !strings.HasPrefix(got, "workout-") || !strings.HasSuffix(got, ".md") {
		t.Errorf("malformed filename: %q", got)
	}
}
