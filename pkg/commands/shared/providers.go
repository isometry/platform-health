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
		instance := provider.NewInstance(providerType)
		if instance == nil {
			continue
		}

		// Check capability check
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
		RunE: func(cmd *cobra.Command, args []string) error {
			return opts.RunFunc(cmd, providerType)
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
// Note: caller must call instance.Setup() after any additional configuration.
func CreateAndConfigureProvider(cmd *cobra.Command, providerType string) (provider.Instance, error) {
	// Create new provider instance
	instance := provider.NewInstance(providerType)
	if instance == nil {
		return nil, fmt.Errorf("provider type %q not registered", providerType)
	}

	// Configure from flags (via reflection)
	if err := provider.ConfigureFromFlags(instance, cmd.Flags()); err != nil {
		return nil, fmt.Errorf("failed to configure provider from flags: %w", err)
	}

	return instance, nil
}
