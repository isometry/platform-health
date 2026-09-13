//go:build ui

package ui

import (
	"context"

	ph "github.com/isometry/platform-health/pkg/platform_health"
)

// NewTestScanner returns a scanner whose scan is the given check function, so
// tests can park or fail a scan on demand.
func NewTestScanner(rootCtx context.Context, cfg ScannerConfig, check func(context.Context) (*ph.HealthCheckResponse, error)) *Scanner {
	s := newScanner(rootCtx, cfg, "test")
	s.check = check
	return s
}
