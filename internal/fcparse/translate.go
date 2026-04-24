package fcparse

import (
	"bufio"
	"bytes"
	"fmt"
	"os"
	"strings"

	"github.com/arch-err/calemdar/internal/model"
	"github.com/google/uuid"
	"gopkg.in/yaml.v3"
)

// Detect returns the FC type of the frontmatter at path, or "" if none.
// Does not fail on non-FC files — they're just not recurring.
func Detect(path string) (string, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	fm, _, err := splitFrontmatter(raw)
	if err != nil || fm == nil {
		return "", err
	}
	var t typeProbe
	if err := yaml.Unmarshal(fm, &t); err != nil {
		return "", err
	}
	return t.Type, nil
}

// ReadRecurring parses an FC type: recurring event.
func ReadRecurring(path string) (*Recurring, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	fm, _, err := splitFrontmatter(raw)
	if err != nil || fm == nil {
		return nil, fmt.Errorf("read recurring %s: no frontmatter", path)
	}
	var r Recurring
	if err := yaml.Unmarshal(fm, &r); err != nil {
		return nil, fmt.Errorf("read recurring %s: %w", path, err)
	}
	return &r, nil
}

// ReadRRule parses an FC type: rrule event.
func ReadRRule(path string) (*RRule, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	fm, _, err := splitFrontmatter(raw)
	if err != nil || fm == nil {
		return nil, fmt.Errorf("read rrule %s: no frontmatter", path)
	}
	var r RRule
	if err := yaml.Unmarshal(fm, &r); err != nil {
		return nil, fmt.Errorf("read rrule %s: %w", path, err)
	}
	return &r, nil
}

// ReadBody returns the body (content after the closing frontmatter delimiter).
func ReadBody(path string) (string, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	_, body, err := splitFrontmatter(raw)
	return body, err
}

// TranslateRecurring converts an FC type: recurring event into a calemdar Root.
// calendar is the top-level folder name derived from the event's path.
func TranslateRecurring(fc *Recurring, calendar string) (*model.Root, error) {
	if fc.Title == "" {
		return nil, fmt.Errorf("translate: missing title")
	}
	if fc.StartRecur == "" {
		return nil, fmt.Errorf("translate: missing startRecur")
	}
	byday := make([]string, 0, len(fc.DaysOfWeek))
	for _, d := range fc.DaysOfWeek {
		tok, ok := fcDayToToken(strings.ToUpper(strings.TrimSpace(d)))
		if !ok {
			return nil, fmt.Errorf("translate: unknown daysOfWeek value %q", d)
		}
		byday = append(byday, tok)
	}
	if len(byday) == 0 {
		return nil, fmt.Errorf("translate: daysOfWeek empty (type: recurring requires at least one)")
	}

	id, err := newUUID()
	if err != nil {
		return nil, err
	}

	return &model.Root{
		ID:        id,
		Calendar:  calendar,
		Title:     fc.Title,
		StartDate: fc.StartRecur,
		Until:     fc.EndRecur,
		StartTime: fc.StartTime,
		EndTime:   fc.EndTime,
		AllDay:    fc.AllDay,
		Freq:      model.FreqWeekly,
		Interval:  1,
		ByDay:     byday,
	}, nil
}

// TranslateRRule converts an FC type: rrule event into a calemdar Root.
func TranslateRRule(fc *RRule, calendar string) (*model.Root, error) {
	if fc.Title == "" {
		return nil, fmt.Errorf("translate: missing title")
	}
	if fc.StartDate == "" {
		return nil, fmt.Errorf("translate: missing startDate")
	}
	rr, err := ParseRRule(fc.RRule)
	if err != nil {
		return nil, err
	}

	freq := strings.ToLower(rr.Freq) // DAILY → daily etc.
	byday := make([]string, 0, len(rr.ByDay))
	for _, d := range rr.ByDay {
		tok, ok := rruleDayToToken(d)
		if !ok {
			return nil, fmt.Errorf("translate: unknown BYDAY %q", d)
		}
		byday = append(byday, tok)
	}

	if freq == model.FreqWeekly && len(byday) == 0 {
		return nil, fmt.Errorf("translate: WEEKLY rrule requires BYDAY for v1")
	}
	if freq == model.FreqMonthly && len(rr.ByMonthDay) == 0 {
		return nil, fmt.Errorf("translate: MONTHLY rrule requires BYMONTHDAY for v1")
	}

	id, err := newUUID()
	if err != nil {
		return nil, err
	}

	return &model.Root{
		ID:         id,
		Calendar:   calendar,
		Title:      fc.Title,
		StartDate:  fc.StartDate,
		Until:      rr.Until,
		StartTime:  fc.StartTime,
		EndTime:    fc.EndTime,
		AllDay:     fc.AllDay,
		Freq:       freq,
		Interval:   rr.Interval,
		ByDay:      byday,
		ByMonthDay: rr.ByMonthDay,
		Exceptions: fc.SkipDates,
	}, nil
}

// fcDayToToken maps FC's single-letter daysOfWeek values to our 3-letter tokens.
// FC uses U=Sun, M=Mon, T=Tue, W=Wed, R=Thu, F=Fri, S=Sat.
func fcDayToToken(s string) (string, bool) {
	switch s {
	case "U":
		return "sun", true
	case "M":
		return "mon", true
	case "T":
		return "tue", true
	case "W":
		return "wed", true
	case "R":
		return "thu", true
	case "F":
		return "fri", true
	case "S":
		return "sat", true
	}
	return "", false
}

// rruleDayToToken maps RFC 5545 two-letter BYDAY codes to our 3-letter tokens.
func rruleDayToToken(s string) (string, bool) {
	switch s {
	case "MO":
		return "mon", true
	case "TU":
		return "tue", true
	case "WE":
		return "wed", true
	case "TH":
		return "thu", true
	case "FR":
		return "fri", true
	case "SA":
		return "sat", true
	case "SU":
		return "sun", true
	}
	return "", false
}

func newUUID() (string, error) {
	u, err := uuid.NewV7()
	if err != nil {
		return "", err
	}
	return u.String(), nil
}

// splitFrontmatter is a local copy so fcparse doesn't depend on model's
// unexported helper. Same semantics as model.splitFrontmatter.
func splitFrontmatter(raw []byte) ([]byte, string, error) {
	scanner := bufio.NewScanner(bytes.NewReader(raw))
	scanner.Buffer(make([]byte, 1024*1024), 16*1024*1024)

	if !scanner.Scan() {
		return nil, "", nil
	}
	if strings.TrimRight(scanner.Text(), "\r\n") != "---" {
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
