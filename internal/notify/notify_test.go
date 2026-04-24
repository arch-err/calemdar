package notify

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/arch-err/calemdar/internal/config"
	"github.com/arch-err/calemdar/internal/model"
)

// capturedRequest records what the fake ntfy saw.
type capturedRequest struct {
	path   string
	method string
	body   string
	title  string
	tags   string
}

// fakeNtfy returns an httptest.Server and a mutex-protected slice of
// requests. Use this to assert POST contents.
func fakeNtfy(t *testing.T) (*httptest.Server, *[]capturedRequest, *sync.Mutex) {
	t.Helper()
	var mu sync.Mutex
	var reqs []capturedRequest
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		mu.Lock()
		reqs = append(reqs, capturedRequest{
			path:   r.URL.Path,
			method: r.Method,
			body:   string(b),
			title:  r.Header.Get("Title"),
			tags:   r.Header.Get("Tags"),
		})
		mu.Unlock()
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(srv.Close)
	return srv, &reqs, &mu
}

func TestSendTestPostsToTopicPath(t *testing.T) {
	srv, reqs, mu := fakeNtfy(t)
	cfg := config.Notifications{
		Enabled:   true,
		NtfyURL:   srv.URL,
		NtfyTopic: "my-topic",
	}
	n := New(nil, cfg)
	if err := n.SendTest(context.Background()); err != nil {
		t.Fatal(err)
	}
	mu.Lock()
	defer mu.Unlock()
	if len(*reqs) != 1 {
		t.Fatalf("got %d requests, want 1", len(*reqs))
	}
	got := (*reqs)[0]
	if got.method != http.MethodPost {
		t.Errorf("method = %q", got.method)
	}
	if got.path != "/my-topic" {
		t.Errorf("path = %q", got.path)
	}
	if !strings.Contains(got.body, "calemdar notify test") {
		t.Errorf("body = %q", got.body)
	}
	if got.title != "calemdar: test" {
		t.Errorf("title = %q", got.title)
	}
}

func TestSendUpcomingFormatsBodyAndHeaders(t *testing.T) {
	srv, reqs, mu := fakeNtfy(t)
	cfg := config.Notifications{
		Enabled:   true,
		NtfyURL:   srv.URL,
		NtfyTopic: "my-topic",
	}
	n := New(nil, cfg)
	e := &model.Event{
		Path:      "/vault/events/health/2026-05-04-workout.md",
		Title:     "Workout",
		Date:      "2026-05-04",
		StartTime: "10:00",
		EndTime:   "11:00",
	}
	if err := n.send(context.Background(), e, 5); err != nil {
		t.Fatal(err)
	}
	mu.Lock()
	defer mu.Unlock()
	if len(*reqs) != 1 {
		t.Fatalf("got %d requests, want 1", len(*reqs))
	}
	got := (*reqs)[0]
	if got.path != "/my-topic" {
		t.Errorf("path = %q", got.path)
	}
	if got.title != "calemdar: upcoming" {
		t.Errorf("title = %q", got.title)
	}
	if !strings.Contains(got.body, "Workout") || !strings.Contains(got.body, "5m") {
		t.Errorf("body = %q", got.body)
	}
	if !strings.Contains(got.tags, "calendar") || !strings.Contains(got.tags, "health") {
		t.Errorf("tags = %q, want includes calendar,health", got.tags)
	}
}

func TestDedupeKey(t *testing.T) {
	a := dedupeKey("/p.md", 5)
	b := dedupeKey("/p.md", 5)
	c := dedupeKey("/p.md", 60)
	if a != b {
		t.Errorf("same inputs should match: %q %q", a, b)
	}
	if a == c {
		t.Errorf("different leads should differ: %q %q", a, c)
	}
}

func TestGCRemovesOldEntries(t *testing.T) {
	n := New(nil, config.Notifications{})
	now := time.Date(2026, 5, 10, 12, 0, 0, 0, time.UTC)
	n.seen["a|5"] = "2026-05-08" // older than 1 day before now → drop
	n.seen["b|5"] = "2026-05-09" // exactly 1 day before now cutoff date → keep (==)
	n.seen["c|5"] = "2026-05-10" // today → keep
	n.gc(now)
	if _, ok := n.seen["a|5"]; ok {
		t.Errorf("expected a|5 dropped")
	}
	if _, ok := n.seen["b|5"]; !ok {
		t.Errorf("expected b|5 kept")
	}
	if _, ok := n.seen["c|5"]; !ok {
		t.Errorf("expected c|5 kept")
	}
}

func TestExtractCalendarFromPath(t *testing.T) {
	got := extractCalendar("/vault/events/health/2026-05-04-workout.md")
	if got != "health" {
		t.Errorf("got %q, want health", got)
	}
	if extractCalendar("/no/events/here/file.md") == "no" {
		// make sure we don't go past end
	}
	if extractCalendar("/bare/path.md") != "" {
		t.Errorf("no match should return empty")
	}
}

func TestFormatBody(t *testing.T) {
	e := &model.Event{Title: "Meeting", StartTime: "14:30", EndTime: "15:00"}
	got := formatBody(e, 60)
	if !strings.Contains(got, "Meeting") || !strings.Contains(got, "60m") || !strings.Contains(got, "14:30") {
		t.Errorf("body = %q", got)
	}
}
