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
	// RunFunc is the command handler
	RunFunc func(cmd *cobra.Command, providerType string) error
}

// AddProviderSubcommands creates a subcommand for each qualifying provider.
func AddProviderSubcommands(parent *cobra.Command, opts ProviderSubcommandOptions) {
	for _, providerType := range provider.ProviderList() {
		instance, err := provider.New(providerType)
		if err != nil {
			panic(err)
		}

		if opts.RequireChecks && !provider.SupportsChecks(instance) {
			continue
		}

		// Create subcommand
		providerCmd := createProviderSubcommand(providerType, instance, opts)
		parent.AddCommand(providerCmd)
	}
}

// createProviderSubcommand creates a subcommand for a specific provider type.
func createProviderSubcommand(
	providerType string,
	instance provider.Instance,
	opts ProviderSubcommandOptions,
) *cobra.Command {
	cmd := &cobra.Command{
		Use:   providerType,
		Short: fmt.Sprintf("Ad-hoc %s provider", providerType),
		RunE: func(cmd *cobra.Command, _ []string) error {
			return opts.RunFunc(cmd, providerType)
		},
	}

	// Register provider-specific flags (derived via reflection)
	providerFlags := provider.ProviderFlags(instance)
	providerFlags.Register(cmd.Flags(), true)

	if opts.SetupFlags != nil {
		opts.SetupFlags(cmd, instance)
	}

	return cmd
}

// CreateAndConfigureProvider creates a provider instance and configures it from flags.
// The instance is fully configured and Setup() has been called.
func CreateAndConfigureProvider(cmd *cobra.Command, providerType string) (provider.Instance, error) {
	return provider.NewInstance(providerType, provider.WithFlags(cmd.Flags()))
}
