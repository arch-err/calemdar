package model

// Event is a single expanded occurrence stored in events/<calendar>/<year>/.
// Server-authored on expansion; human may edit via Obsidian, which flips
// UserOwned to true and pins the event against future regen.
type Event struct {
	Title     string `yaml:"title"`
	Date      string `yaml:"date"`
	StartTime string `yaml:"startTime,omitempty"`
	EndTime   string `yaml:"endTime,omitempty"`
	AllDay    bool   `yaml:"allDay"`
	Type      string `yaml:"type"` // always "single"

	SeriesID         string `yaml:"series-id,omitempty"`
	SeriesExpandedAt string `yaml:"series-expanded-at,omitempty"`
	UserOwned        bool   `yaml:"user-owned"`

	// Notify is the per-event notification rules. On expansion, copied
	// from the root. If the user overrides it on an expanded file, the
	// user-owned flip preserves the override across reconciles.
	Notify []NotifyEntry `yaml:"notify,omitempty"`

	// Body is the markdown body after the frontmatter. On expansion, this
	// is the root's body preceded by a wikilink paragraph to the root.
	Body string `yaml:"-"`

	// Path is the absolute path to the event file on disk.
	Path string `yaml:"-"`
}

// IsOneOff returns true for events not tied to a recurring series.
func (e *Event) IsOneOff() bool {
	return e.SeriesID == ""
}
