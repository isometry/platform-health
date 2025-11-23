package check

import (
	"fmt"
	"log/slog"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	slogctx "github.com/veqryn/slog-context"

	"github.com/isometry/platform-health/pkg/checks"
	"github.com/isometry/platform-health/pkg/commands/flags"
	"github.com/isometry/platform-health/pkg/config"
	ph "github.com/isometry/platform-health/pkg/platform_health"
	"github.com/isometry/platform-health/pkg/provider"
	"github.com/isometry/platform-health/pkg/server"
)

var (
	log  *slog.Logger
	conf provider.Config
)

func New() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "check",
		Short: "Perform one-shot health checks",
		Long: `Perform one-shot health checks without starting a server.

Run without arguments to check all configured components.
Use -c to check specific components.
Use a provider subcommand for ad-hoc checks without config.`,
		Args:    cobra.NoArgs,
		PreRunE: setup,
		RunE:    run,
	}

	checkFlags.Register(cmd.Flags(), false)

	// Add dynamic provider subcommands for ad-hoc checks
	addProviderSubcommands(cmd)

	return cmd
}

// addProviderSubcommands creates a subcommand for each flag-configurable provider.
func addProviderSubcommands(parent *cobra.Command) {
	for _, providerType := range provider.ProviderList() {
		instance := provider.NewInstance(providerType)
		if instance == nil {
			continue
		}

		// Only add subcommand if provider is flag-configurable
		flagConfigurable := provider.AsFlagConfigurable(instance)
		if flagConfigurable == nil {
			continue
		}

		// CEL capability is optional
		celCapable := provider.AsCELCapable(instance)

		// Create subcommand
		providerCmd := createProviderSubcommand(providerType, celCapable, flagConfigurable)
		parent.AddCommand(providerCmd)
	}
}

// createProviderSubcommand creates a subcommand for a specific provider type.
func createProviderSubcommand(providerType string, celCapable provider.CELCapable, flagConfigurable provider.FlagConfigurable) *cobra.Command {
	cmd := &cobra.Command{
		Use:   providerType,
		Short: fmt.Sprintf("Perform ad-hoc health check for %s provider", providerType),
		Long:  fmt.Sprintf("Create an ad-hoc %s provider instance and perform a health check.", providerType),
		PreRun: func(cmd *cobra.Command, args []string) {
			log = slog.Default()
			cmd.SetContext(slogctx.NewCtx(cmd.Context(), log))
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			return runProviderCheck(cmd, providerType)
		},
	}

	// Register provider-specific flags
	providerFlags := flagConfigurable.GetProviderFlags()
	providerFlags.Register(cmd.Flags(), true)

	// Add --check flag for inline CEL expressions only if provider supports CEL
	if celCapable != nil {
		cmd.Flags().StringSlice("check", nil, "CEL expression to evaluate (can be specified multiple times)")
	}

	// Add output flags for formatting
	flags.OutputFlags().Register(cmd.Flags(), true)

	return cmd
}

// runProviderCheck creates an ad-hoc provider instance and performs a health check.
func runProviderCheck(cmd *cobra.Command, providerType string) error {
	// Create new provider instance
	instance := provider.NewInstance(providerType)
	if instance == nil {
		return fmt.Errorf("provider type %q not registered", providerType)
	}

	// Configure from flags
	flagConfigurable := provider.AsFlagConfigurable(instance)
	if flagConfigurable == nil {
		return fmt.Errorf("provider type %q is not flag-configurable", providerType)
	}

	// Bind common flags without namespace, provider flags with namespace
	flags.BindFlags(cmd)
	flags.BindProviderFlags(cmd, providerType)

	if err := flagConfigurable.ConfigureFromFlags(viper.GetViper()); err != nil {
		return fmt.Errorf("failed to configure provider from flags: %w", err)
	}

	// Handle inline --check expressions for CEL-capable providers
	celCapable := provider.AsCELCapable(instance)
	if celCapable != nil {
		checkExprs, err := cmd.Flags().GetStringSlice("check")
		if err != nil {
			return fmt.Errorf("failed to get check expressions: %w", err)
		}
		if len(checkExprs) > 0 {
			exprs := make([]checks.Expression, len(checkExprs))
			for i, expr := range checkExprs {
				exprs[i] = checks.Expression{Expression: expr}
			}
			celCapable.SetChecks(exprs)
		}
	}

	// Setup the provider (compiles CEL expressions if applicable)
	if err := instance.Setup(); err != nil {
		return fmt.Errorf("failed to setup provider: %w", err)
	}

	// Perform health check
	response := instance.GetHealth(cmd.Context())

	// Format and print output
	return flags.FormatAndPrintStatus(response, flags.OutputConfigFromViper())
}

func setup(cmd *cobra.Command, _ []string) (err error) {
	flags.BindFlags(cmd)

	log = slog.Default()
	cmd.SetContext(slogctx.NewCtx(cmd.Context(), log))

	log.Info("providers registered", slog.Any("providers", provider.ProviderList()))

	paths, name := flags.ConfigPaths()
	conf, err = config.Load(cmd.Context(), paths, name)
	return err
}

func run(cmd *cobra.Command, _ []string) error {
	cmd.SilenceErrors = true

	serverId := "oneshot"
	srv, err := server.NewPlatformHealthServer(&serverId, conf)
	if err != nil {
		log.Error("failed to create server", "error", err)
		return err
	}

	status, err := srv.Check(cmd.Context(), &ph.HealthCheckRequest{
		Components: viper.GetStringSlice("component"),
	})
	if err != nil {
		slog.Info("failed to check", slog.Any("error", err))
		return err
	}

	return flags.FormatAndPrintStatus(status, flags.OutputConfigFromViper())
}
