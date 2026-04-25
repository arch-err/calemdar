package model

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// MaxNotifyEntries caps the number of notify entries permitted on any
// single root or event. The cap exists so a malformed (or hostile) vault
// file can't fan out into thousands of timer slots per occurrence.
const MaxNotifyEntries = 16

// MinLead is the smallest non-zero lead the scheduler honours. Below this
// the tick-window arithmetic stops being meaningful.
var MinLead = time.Minute

// MaxLead is the longest lead permitted. Caps how far past today the
// scheduler must scan to be sure no event-bound notif is missed.
var MaxLead = 23 * time.Hour

// actionNameRE constrains action names declared in vault frontmatter to a
// small, audit-friendly charset. Lowercase, starts alpha, hyphens allowed.
var actionNameRE = regexp.MustCompile(`^[a-z][a-z0-9-]{0,47}$`)

// NotifyEntry is a single notification rule attached to an event or root.
//
//	notify:
//	  - lead: 5m            # required; "0", "5m", "1h", "30m", "23h"
//	    via: [system, ntfy] # optional; backends that deliver the message
//	    action: pre-meeting # optional; named entry in actions.yaml
type NotifyEntry struct {
	Lead   string   `yaml:"lead"`
	Via    []string `yaml:"via,omitempty"`
	Action string   `yaml:"action,omitempty"`
}

// LeadDuration parses Lead into a time.Duration. Returns an error if the
// value isn't a recognised duration or falls outside [0, MaxLead].
func (n NotifyEntry) LeadDuration() (time.Duration, error) {
	return ParseLead(n.Lead)
}

// Validate checks one entry's fields. Validate is called per entry by
// ValidateNotifyList so callers see the full list of issues at once.
func (n NotifyEntry) Validate() error {
	d, err := ParseLead(n.Lead)
	if err != nil {
		return err
	}
	if d != 0 && d < MinLead {
		return fmt.Errorf("notify: lead %q below minimum %s", n.Lead, MinLead)
	}
	if d > MaxLead {
		return fmt.Errorf("notify: lead %q above maximum %s", n.Lead, MaxLead)
	}
	for _, v := range n.Via {
		if v == "" {
			return fmt.Errorf("notify: empty via entry")
		}
	}
	if n.Action != "" && !actionNameRE.MatchString(n.Action) {
		return fmt.Errorf("notify: action name %q must match %s", n.Action, actionNameRE.String())
	}
	if len(n.Via) == 0 && n.Action == "" {
		return fmt.Errorf("notify: entry must specify at least one via backend or an action")
	}
	return nil
}

// ValidateNotifyList enforces the per-event cap and runs Validate on each
// entry. Returns the first error encountered; future versions could
// accumulate.
func ValidateNotifyList(list []NotifyEntry) error {
	if len(list) > MaxNotifyEntries {
		return fmt.Errorf("notify: %d entries exceeds cap of %d", len(list), MaxNotifyEntries)
	}
	for i, n := range list {
		if err := n.Validate(); err != nil {
			return fmt.Errorf("notify[%d]: %w", i, err)
		}
	}
	return nil
}

// ParseLead converts a duration string into a time.Duration. Accepted forms:
// "0", "<n>m", "<n>h" — composites like "1h30m" are accepted as well via
// time.ParseDuration. Bare integers ("5") are treated as minutes for
// ergonomics. Negative values are rejected.
func ParseLead(s string) (time.Duration, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, fmt.Errorf("notify: lead is required")
	}
	if s == "0" {
		return 0, nil
	}
	// Bare integer: interpret as minutes.
	if n, err := strconv.Atoi(s); err == nil {
		if n < 0 {
			return 0, fmt.Errorf("notify: lead %q must be >= 0", s)
		}
		return time.Duration(n) * time.Minute, nil
	}
	d, err := time.ParseDuration(s)
	if err != nil {
		return 0, fmt.Errorf("notify: lead %q: %w", s, err)
	}
	if d < 0 {
		return 0, fmt.Errorf("notify: lead %q must be >= 0", s)
	}
	return d, nil
}
