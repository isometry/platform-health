//go:build ui

package root_test

import (
	"testing"

	"github.com/isometry/platform-health/pkg/commands/root"
)

// The published image is built with -tags ui and the chart's sidecar runs
// `ph ui` from it, so losing this registration breaks the sidecar at runtime.
func TestUICommandRegistered(t *testing.T) {
	cmd, _, err := root.New().Find([]string{"ui"})
	if err != nil {
		t.Fatalf("ui subcommand not found in a -tags ui build: %v", err)
	}
	if cmd.Name() != "ui" {
		t.Errorf("Find([ui]) resolved to %q, want \"ui\"", cmd.Name())
	}
}
