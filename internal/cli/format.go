package cli

import (
	"sort"
	"sync"

	ph "github.com/isometry/platform-health/pkg/platform_health"
)

// Formatter defines the interface for output formatters.
// Implementations convert HealthCheckResponse to formatted output bytes.
type Formatter interface {
	// Format converts the health check response to formatted output.
	// The cfg parameter provides output configuration (flat, quiet, compact).
	Format(status *ph.HealthCheckResponse, cfg OutputConfig) ([]byte, error)
}

// DefaultFormat is the default output format.
const DefaultFormat = "json"

var (
	formatters = make(map[string]Formatter)
	mu         sync.RWMutex
)

// RegisterFormatter registers a formatter by name.
// Called from init() in each formatter file.
func RegisterFormatter(name string, f Formatter) {
	mu.Lock()
	defer mu.Unlock()
	formatters[name] = f
}

// GetFormatter returns the formatter for the given name.
func GetFormatter(name string) (Formatter, bool) {
	mu.RLock()
	defer mu.RUnlock()
	f, ok := formatters[name]
	return f, ok
}

// FormatNames returns a sorted list of registered format names.
// Used for flag usage text and validation error messages.
func FormatNames() []string {
	mu.RLock()
	defer mu.RUnlock()
	names := make([]string, 0, len(formatters))
	for name := range formatters {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}
