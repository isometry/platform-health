//go:build !ui

package root

import "github.com/spf13/cobra"

// registerUI is a no-op without -tags ui.
func registerUI(*cobra.Command) {}
