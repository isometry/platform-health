package check

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/spf13/cobra"

	"github.com/isometry/platform-health/pkg/checks"
	"github.com/isometry/platform-health/pkg/commands/flags"
	"github.com/isometry/platform-health/pkg/commands/shared"
	"github.com/isometry/platform-health/pkg/config"
	"github.com/isometry/platform-health/pkg/phctx"
	ph "github.com/isometry/platform-health/pkg/platform_health"
	"github.com/isometry/platform-health/pkg/provider"
	"github.com/isometry/platform-health/pkg/server"
)

var conf provider.Config

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
			cmd.Short = fmt.Sprintf("Perform ad-hoc health check for %s provider", cmd.Use)
			cmd.Long = fmt.Sprintf("Create an ad-hoc %s provider instance and perform a health check.", cmd.Use)

			// Add --check and --check-each flags for inline CEL expressions only if provider supports checks
			if provider.SupportsChecks(instance) {
				cmd.Flags().StringArray("check", nil, "CEL expression to evaluate (can be specified multiple times)")
				cmd.Flags().StringArray("check-each", nil, "CEL expression evaluated per-item (can be specified multiple times)")
			}

			// Add output flags for formatting
			flags.OutputFlags().Register(cmd.Flags(), true)

			// Add timeout flag for ad-hoc checks
			flags.TimeoutFlags().Register(cmd.Flags(), true)
		},
		RunFunc: runProviderCheck,
	})

	return cmd
}

// runProviderCheck creates an ad-hoc provider instance and performs a health check.
func runProviderCheck(cmd *cobra.Command, providerType string) error {
	// Bind common flags without namespace
	v := phctx.Viper(cmd.Context())
	flags.BindFlags(cmd, v)

	// Apply global timeout
	ctx, cancel := context.WithTimeout(cmd.Context(), v.GetDuration("timeout"))
	defer cancel()

	// Create and configure provider from flags
	instance, err := shared.CreateAndConfigureProvider(cmd, providerType)
	if err != nil {
		return err
	}

	// Handle inline --check and --check-each expressions for providers that support checks
	if checkProvider := provider.AsInstanceWithChecks(instance); checkProvider != nil {
		checkExprs, err := cmd.Flags().GetStringArray("check")
		if err != nil {
			return fmt.Errorf("failed to get check expressions: %w", err)
		}
		checkEachExprs, err := cmd.Flags().GetStringArray("check-each")
		if err != nil {
			return fmt.Errorf("failed to get check-each expressions: %w", err)
		}

		if len(checkExprs) > 0 || len(checkEachExprs) > 0 {
			exprs := make([]checks.Expression, 0, len(checkExprs)+len(checkEachExprs))
			for _, expr := range checkExprs {
				exprs = append(exprs, checks.Expression{Expression: expr})
			}
			for _, expr := range checkEachExprs {
				exprs = append(exprs, checks.Expression{Expression: expr, Mode: "each"})
			}
			if err := checkProvider.SetChecks(exprs); err != nil {
				return fmt.Errorf("invalid check expression: %w", err)
			}
		}
	}

	// Perform health check with duration tracking
	response := provider.GetHealthWithDuration(ctx, instance)

	// Format and print output
	return flags.FormatAndPrintStatus(response, flags.OutputConfigFromViper(v))
}

func setup(cmd *cobra.Command, _ []string) (err error) {
	ctx := cmd.Context()
	v := phctx.Viper(ctx)
	flags.BindFlags(cmd, v)

	log := phctx.Logger(ctx)
	log.Info("providers registered", slog.Any("providers", provider.ProviderList()))

	paths, name := flags.ConfigPaths(v)
	strict := v.GetBool("strict")

	result, err := config.Load(ctx, paths, name, strict)
	if err != nil {
		return err
	}

	// In strict mode, fail if any configuration errors were found
	if strict && result.HasErrors() {
		for _, e := range result.ValidationErrors {
			log.Error("configuration error", slog.Any("error", e))
		}
		return fmt.Errorf("configuration validation failed with %d error(s)", len(result.ValidationErrors))
	}

	conf = result
	return nil
}

func run(cmd *cobra.Command, _ []string) error {
	v := phctx.Viper(cmd.Context())

	// Apply global timeout
	ctx, cancel := context.WithTimeout(cmd.Context(), v.GetDuration("timeout"))
	defer cancel()

	log := phctx.Logger(ctx)

	serverId := "oneshot"
	srv, err := server.NewPlatformHealthServer(&serverId, conf,
		server.WithParallelism(v.GetInt("parallelism")),
	)
	if err != nil {
		log.Error("failed to create server", "error", err)
		return err
	}

	status, err := srv.Check(ctx, &ph.HealthCheckRequest{
		Components: v.GetStringSlice("component"),
		FailFast:   v.GetBool("fail-fast"),
	})
	if err != nil {
		log.Info("failed to check", slog.Any("error", err))
		return err
	}

	return flags.FormatAndPrintStatus(status, flags.OutputConfigFromViper(v))
}
