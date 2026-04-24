package vault

import (
	"fmt"
	"os"
	"path/filepath"
)

// ScaffoldReport lists which directories Scaffold had to create.
type ScaffoldReport struct {
	Created []string
	Existed []string
}

// Scaffold creates the standard calemdar tree inside the vault:
//
//	<base>/recurring/
//	<base>/events/<each calendar>/
//	<base>/archive/
//
// where <base> = Root/BasePath. Idempotent — existing directories are
// recorded but not touched.
func Scaffold(v *Vault, calendars []string) (*ScaffoldReport, error) {
	rep := &ScaffoldReport{}

	paths := []string{
		v.BaseDir(),
		v.RecurringDir(),
		v.ArchiveDir(),
		v.EventsDir(),
	}
	for _, cal := range calendars {
		paths = append(paths, filepath.Join(v.EventsDir(), cal))
	}

	for _, p := range paths {
		if _, err := os.Stat(p); err == nil {
			rep.Existed = append(rep.Existed, p)
			continue
		}
		if err := os.MkdirAll(p, 0o755); err != nil {
			return rep, fmt.Errorf("scaffold: mkdir %s: %w", p, err)
		}
		rep.Created = append(rep.Created, p)
	}
	return rep, nil
}
