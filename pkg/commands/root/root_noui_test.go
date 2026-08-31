//go:build !ui

package root_test

import (
	"testing"

	"github.com/isometry/platform-health/pkg/commands/root"
)

// Find falls back to the root command for an unknown name rather than
// erroring, so assert on the resolved name, not on err.
func TestUICommandAbsent(t *testing.T) {
	cmd, _, _ := root.New().Find([]string{"ui"})
	if cmd.Name() == "ui" {
		t.Error("ui subcommand is registered in a build without -tags ui")
	}
}
