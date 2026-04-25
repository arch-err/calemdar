package model

import (
	"testing"
	"time"
)

func TestParseLeadAcceptsCommonForms(t *testing.T) {
	cases := map[string]time.Duration{
		"0":      0,
		"5":      5 * time.Minute, // bare integer interpreted as minutes
		"5m":     5 * time.Minute,
		"30m":    30 * time.Minute,
		"1h":     time.Hour,
		"1h30m":  time.Hour + 30*time.Minute,
		"  10m ": 10 * time.Minute,
	}
	for in, want := range cases {
		got, err := ParseLead(in)
		if err != nil {
			t.Errorf("ParseLead(%q): %v", in, err)
			continue
		}
		if got != want {
			t.Errorf("ParseLead(%q) = %v, want %v", in, got, want)
		}
	}
}

func TestParseLeadRejectsBadInput(t *testing.T) {
	bad := []string{"", "five", "-5m", "5x"}
	for _, in := range bad {
		if _, err := ParseLead(in); err == nil {
			t.Errorf("ParseLead(%q) = nil, want error", in)
		}
	}
}

func TestNotifyEntryValidate(t *testing.T) {
	// Happy path with via.
	n := NotifyEntry{Lead: "5m", Via: []string{"system"}}
	if err := n.Validate(); err != nil {
		t.Errorf("happy: %v", err)
	}
	// Happy path with action.
	n2 := NotifyEntry{Lead: "0", Action: "pre-meeting"}
	if err := n2.Validate(); err != nil {
		t.Errorf("happy w/ action: %v", err)
	}
	// Below MinLead.
	n3 := NotifyEntry{Lead: "30s", Via: []string{"system"}}
	if err := n3.Validate(); err == nil {
		t.Errorf("expected MinLead error")
	}
	// Above MaxLead.
	n4 := NotifyEntry{Lead: "48h", Via: []string{"system"}}
	if err := n4.Validate(); err == nil {
		t.Errorf("expected MaxLead error")
	}
	// Bad action name.
	n5 := NotifyEntry{Lead: "5m", Action: "Bad Name!"}
	if err := n5.Validate(); err == nil {
		t.Errorf("expected action regex error")
	}
	// Empty: neither via nor action.
	n6 := NotifyEntry{Lead: "5m"}
	if err := n6.Validate(); err == nil {
		t.Errorf("expected error: must have via or action")
	}
}

func TestValidateNotifyListCapsCount(t *testing.T) {
	list := make([]NotifyEntry, MaxNotifyEntries+1)
	for i := range list {
		list[i] = NotifyEntry{Lead: "5m", Via: []string{"system"}}
	}
	if err := ValidateNotifyList(list); err == nil {
		t.Errorf("expected cap error for %d entries", len(list))
	}

	list = list[:MaxNotifyEntries]
	if err := ValidateNotifyList(list); err != nil {
		t.Errorf("at-cap should pass: %v", err)
	}
}
