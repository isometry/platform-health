// Package shared provides common utilities for provider-based commands.
package shared

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/isometry/platform-health/pkg/provider"
)

// ProviderSubcommandOptions configures the creation of provider subcommands.
type ProviderSubcommandOptions struct {
	// RequireCEL filters to only CEL-capable providers
	RequireCEL bool
	// SetupFlags adds extra flags to the subcommand (e.g., --check, --output)
	SetupFlags func(*cobra.Command, provider.CELCapable)
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

		// Must be flag-configurable
		flagConfigurable := provider.AsFlagConfigurable(instance)
		if flagConfigurable == nil {
			continue
		}

		// CEL capability check
		celCapable := provider.AsCELCapable(instance)
		if opts.RequireCEL && celCapable == nil {
			continue
		}

		// Create subcommand
		providerCmd := createProviderSubcommand(providerType, flagConfigurable, celCapable, opts)
		parent.AddCommand(providerCmd)
	}
}

// createProviderSubcommand creates a subcommand for a specific provider type.
func createProviderSubcommand(
	providerType string,
	flagConfigurable provider.FlagConfigurable,
	celCapable provider.CELCapable,
	opts ProviderSubcommandOptions,
) *cobra.Command {
	cmd := &cobra.Command{
		Use:   providerType,
		Short: fmt.Sprintf("Ad-hoc %s provider", providerType),
		RunE: func(cmd *cobra.Command, args []string) error {
			return opts.RunFunc(cmd, providerType)
		},
	}

	// Register provider-specific flags
	providerFlags := flagConfigurable.GetProviderFlags()
	providerFlags.Register(cmd.Flags(), true)

	// Add any extra flags
	if opts.SetupFlags != nil {
		opts.SetupFlags(cmd, celCapable)
	}

	return cmd
}

// CreateAndConfigureProvider creates a provider instance and configures it from flags.
// Returns the configured instance and its CEL capability (may be nil).
// Note: caller must call instance.Setup() after any additional configuration.
func CreateAndConfigureProvider(cmd *cobra.Command, providerType string) (provider.Instance, provider.CELCapable, error) {
	// Create new provider instance
	instance := provider.NewInstance(providerType)
	if instance == nil {
		return nil, nil, fmt.Errorf("provider type %q not registered", providerType)
	}

	// Configure from flags
	flagConfigurable := provider.AsFlagConfigurable(instance)
	if flagConfigurable == nil {
		return nil, nil, fmt.Errorf("provider type %q is not flag-configurable", providerType)
	}

	// Pass flags directly to provider (no viper namespace needed)
	if err := flagConfigurable.ConfigureFromFlags(cmd.Flags()); err != nil {
		return nil, nil, fmt.Errorf("failed to configure provider from flags: %w", err)
	}

	celCapable := provider.AsCELCapable(instance)
	return instance, celCapable, nil
}
