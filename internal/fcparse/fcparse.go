// Package fcparse reads Full Calendar's native event frontmatter and
// translates recurring events (type: recurring | rrule) into calemdar's
// Root schema.
package fcparse

// Event type tags used by Full Calendar in the frontmatter `type:` field.
const (
	TypeSingle    = "single"
	TypeRecurring = "recurring" // daysOfWeek-based
	TypeRRule     = "rrule"     // RFC 5545 rrule string
)

// Recurring is the FC frontmatter shape for type: recurring.
type Recurring struct {
	Title      string   `yaml:"title"`
	Type       string   `yaml:"type"`
	DaysOfWeek []string `yaml:"daysOfWeek"`
	StartRecur string   `yaml:"startRecur"`
	EndRecur   string   `yaml:"endRecur,omitempty"`
	StartTime  string   `yaml:"startTime,omitempty"`
	EndTime    string   `yaml:"endTime,omitempty"`
	AllDay     bool     `yaml:"allDay"`
}

// RRule is the FC frontmatter shape for type: rrule.
type RRule struct {
	Title     string   `yaml:"title"`
	Type      string   `yaml:"type"`
	RRule     string   `yaml:"rrule"`
	StartDate string   `yaml:"startDate"`
	SkipDates []string `yaml:"skipDates,omitempty"`
	StartTime string   `yaml:"startTime,omitempty"`
	EndTime   string   `yaml:"endTime,omitempty"`
	AllDay    bool     `yaml:"allDay"`
}

// typeProbe picks off just the type field to dispatch without unmarshalling
// the full shape.
type typeProbe struct {
	Type string `yaml:"type"`
}
