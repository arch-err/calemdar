package writer

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/arch-err/calemdar/internal/model"
)

func TestWriteEventRoundTrip(t *testing.T) {
	dir := t.TempDir()
	e := &model.Event{
		Title:            "Workout",
		Date:             "2026-05-03",
		StartTime:        "10:00",
		EndTime:          "11:00",
		AllDay:           false,
		Type:             "single",
		SeriesID:         "sid",
		SeriesExpandedAt: "2026-04-24T11:00:00Z",
		UserOwned:        false,
		Body:             "[[workout]]\n\nbody text",
		Path:             filepath.Join(dir, "health", "2026", "2026-05-03-workout.md"),
	}
	if err := WriteEvent(e); err != nil {
		t.Fatal(err)
	}

	parsed, err := model.ParseEvent(e.Path)
	if err != nil {
		t.Fatal(err)
	}
	if parsed.Title != e.Title || parsed.Date != e.Date || parsed.SeriesID != e.SeriesID {
		t.Errorf("round-trip mismatch: %+v", parsed)
	}
	if !strings.Contains(parsed.Body, "[[workout]]") {
		t.Errorf("body missing wikilink: %q", parsed.Body)
	}
}
