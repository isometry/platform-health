// Package shared provides common utilities for provider-based commands.
package shared

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/isometry/platform-health/pkg/provider"
)

// ProviderSubcommandOptions configures the creation of provider subcommands.
type ProviderSubcommandOptions struct {
	// RequireChecks filters to only providers that support checks
	RequireChecks bool
	// SetupFlags adds extra flags to the subcommand (e.g., --check, --output)
	SetupFlags func(*cobra.Command, provider.Instance)
	// RunFunc is the command handler (args contains positional arguments)
	RunFunc func(cmd *cobra.Command, providerKind string, args []string) error
}

// AddProviderSubcommands creates a subcommand for each qualifying provider.
func AddProviderSubcommands(parent *cobra.Command, opts ProviderSubcommandOptions) {
	for _, providerKind := range provider.ProviderList() {
		// Create a raw instance to check capabilities (no Setup/validation)
		instance, err := provider.New(providerKind)
		if err != nil {
			continue // Unknown provider - skip silently
		}

		// Check capability check
		if opts.RequireChecks && !provider.SupportsChecks(instance) {
			continue
		}

		// Create subcommand
		providerCmd := createProviderSubcommand(providerKind, instance, opts)
		parent.AddCommand(providerCmd)
	}
}

// createProviderSubcommand creates a subcommand for a specific provider kind.
func createProviderSubcommand(
	providerKind string,
	instance provider.Instance,
	opts ProviderSubcommandOptions,
) *cobra.Command {
	cmd := &cobra.Command{
		Use:   providerKind,
		Short: fmt.Sprintf("Ad-hoc %s provider", providerKind),
		RunE: func(cmd *cobra.Command, args []string) error {
			return opts.RunFunc(cmd, providerKind, args)
		},
	}

	// Register provider-specific flags (derived via reflection)
	providerFlags := provider.ProviderFlags(instance)
	providerFlags.Register(cmd.Flags(), true)

	// Add any extra flags
	if opts.SetupFlags != nil {
		opts.SetupFlags(cmd, instance)
	}

	return cmd
}

// CreateAndConfigureProvider creates a provider instance and configures it from flags.
// The instance is fully configured and Setup() has been called.
func CreateAndConfigureProvider(cmd *cobra.Command, providerKind string) (provider.Instance, error) {
	return provider.NewInstance(providerKind, provider.WithFlags(cmd.Flags()))
}
