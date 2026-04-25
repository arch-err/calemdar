package notify

import (
	"fmt"
	"sort"
	"sync"
)

// registry is the package-level backend table. Constructors register
// themselves at startup (or are added explicitly during tests). Lookups
// from the scheduler happen on every fire, so guard with a RWMutex.
var (
	regMu    sync.RWMutex
	registry = map[string]Backend{}
)

// Register adds b to the registry under b.Name(). Subsequent registrations
// with the same name overwrite (last-write-wins) — useful for tests that
// inject a fake.
func Register(b Backend) {
	if b == nil {
		return
	}
	regMu.Lock()
	registry[b.Name()] = b
	regMu.Unlock()
}

// Reset clears the registry. Test-only helper. Production code never
// needs to drop registered backends.
func Reset() {
	regMu.Lock()
	registry = map[string]Backend{}
	regMu.Unlock()
}

// Get returns the named backend or an error if it isn't registered.
func Get(name string) (Backend, error) {
	regMu.RLock()
	b, ok := registry[name]
	regMu.RUnlock()
	if !ok {
		return nil, fmt.Errorf("notify: backend %q not registered", name)
	}
	return b, nil
}

// Names returns registered backend names sorted alphabetically.
func Names() []string {
	regMu.RLock()
	defer regMu.RUnlock()
	out := make([]string, 0, len(registry))
	for k := range registry {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
