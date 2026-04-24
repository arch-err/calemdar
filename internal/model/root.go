package model

// Root is a recurring event template stored in recurring/<slug>.md.
// Human-authored; server reads but never writes.
type Root struct {
	ID         string   `yaml:"id"`
	Calendar   string   `yaml:"calendar"`
	Title      string   `yaml:"title"`
	StartDate  string   `yaml:"start-date"`
	Until      string   `yaml:"until,omitempty"`
	StartTime  string   `yaml:"start-time,omitempty"`
	EndTime    string   `yaml:"end-time,omitempty"`
	AllDay     bool     `yaml:"all-day"`
	Freq       string   `yaml:"freq"`
	Interval   int      `yaml:"interval"`
	ByDay      []string `yaml:"byday,omitempty"`
	ByMonthDay []int    `yaml:"bymonthday,omitempty"`
	Exceptions []string `yaml:"exceptions,omitempty"`

	// Body is the markdown body after the frontmatter. Copied into each
	// expanded event verbatim.
	Body string `yaml:"-"`

	// Path is the absolute path to the root file on disk.
	Path string `yaml:"-"`

	// Slug is the filename without .md extension.
	Slug string `yaml:"-"`
}
