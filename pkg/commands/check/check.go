package check

import (
	"fmt"
	"log/slog"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	slogctx "github.com/veqryn/slog-context"

	"github.com/isometry/platform-health/pkg/checks"
	"github.com/isometry/platform-health/pkg/commands/flags"
	"github.com/isometry/platform-health/pkg/commands/shared"
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
	shared.AddProviderSubcommands(cmd, shared.ProviderSubcommandOptions{
		RequireChecks: false,
		SetupFlags: func(cmd *cobra.Command, instance provider.Instance) {
			// Set up logging in PreRun
			cmd.PreRun = func(cmd *cobra.Command, args []string) {
				log = slog.Default()
				cmd.SetContext(slogctx.NewCtx(cmd.Context(), log))
			}
			cmd.Short = fmt.Sprintf("Perform ad-hoc health check for %s provider", cmd.Use)
			cmd.Long = fmt.Sprintf("Create an ad-hoc %s provider instance and perform a health check.", cmd.Use)

			// Add --check flag for inline CEL expressions only if provider supports checks
			if provider.SupportsChecks(instance) {
				cmd.Flags().StringArray("check", nil, "CEL expression to evaluate (can be specified multiple times)")
			}

			// Add output flags for formatting
			flags.OutputFlags().Register(cmd.Flags(), true)
		},
		RunFunc: runProviderCheck,
	})

	return cmd
}

// runProviderCheck creates an ad-hoc provider instance and performs a health check.
func runProviderCheck(cmd *cobra.Command, providerType string) error {
	// Bind common flags without namespace
	flags.BindFlags(cmd)

	// Create and configure provider from flags
	instance, err := shared.CreateAndConfigureProvider(cmd, providerType)
	if err != nil {
		return err
	}

	// Handle inline --check expressions for providers that support checks
	if checkProvider := provider.AsInstanceWithChecks(instance); checkProvider != nil {
		checkExprs, err := cmd.Flags().GetStringArray("check")
		if err != nil {
			return fmt.Errorf("failed to get check expressions: %w", err)
		}
		if len(checkExprs) > 0 {
			exprs := make([]checks.Expression, len(checkExprs))
			for i, expr := range checkExprs {
				exprs[i] = checks.Expression{Expression: expr}
			}
			checkProvider.SetChecks(exprs)
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
	srv, err := server.NewPlatformHealthServer(&serverId, conf,
		server.WithParallelism(viper.GetInt("parallelism")),
	)
	if err != nil {
		log.Error("failed to create server", "error", err)
		return err
	}

	status, err := srv.Check(cmd.Context(), &ph.HealthCheckRequest{
		Components: viper.GetStringSlice("component"),
		FailFast:   viper.GetBool("fail-fast"),
	})
	if err != nil {
		slog.Info("failed to check", slog.Any("error", err))
		return err
	}

	return flags.FormatAndPrintStatus(status, flags.OutputConfigFromViper())
}
