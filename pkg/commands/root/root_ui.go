//go:build ui

package root

import (
	"github.com/spf13/cobra"

	"github.com/isometry/platform-health/pkg/commands/ui"
)

// registerUI adds the dashboard subcommand.
func registerUI(cmd *cobra.Command) {
	cmd.AddCommand(ui.New())
}
