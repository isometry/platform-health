package check

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/spf13/cobra"

	"github.com/isometry/platform-health/internal/cliflags"
	"github.com/isometry/platform-health/internal/output"
	"github.com/isometry/platform-health/pkg/checks"
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

			if provider.SupportsChecks(instance) {
				cmd.Flags().StringArray("check", nil, "CEL expression to evaluate (can be specified multiple times)")
				cmd.Flags().StringArray("check-each", nil, "CEL expression evaluated per-item (can be specified multiple times)")
			}

			cliflags.OutputFlags().Register(cmd.Flags(), true)
			cliflags.TimeoutFlags().Register(cmd.Flags(), true)
		},
		RunFunc: runProviderCheck,
	})

	return cmd
}

// runProviderCheck creates an ad-hoc provider instance and performs a health check.
func runProviderCheck(cmd *cobra.Command, providerType string) error {
	v := phctx.Viper(cmd.Context())
	cliflags.BindFlags(cmd, v)

	ctx, cancel := context.WithTimeout(cmd.Context(), v.GetDuration("timeout"))
	defer cancel()

	instance, err := shared.CreateAndConfigureProvider(cmd, providerType)
	if err != nil {
		return err
	}

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

	response := provider.GetHealthWithDuration(ctx, instance)

	return output.FormatAndPrint(response, output.ConfigFromViper(v))
}

func setup(cmd *cobra.Command, _ []string) (err error) {
	ctx := cmd.Context()
	v := phctx.Viper(ctx)
	cliflags.BindFlags(cmd, v)

	log := phctx.Logger(ctx)
	log.Info("providers registered", slog.Any("providers", provider.ProviderList()))

	paths, name := cliflags.ConfigPaths(v)
	strict := v.GetBool("strict")

	result, err := config.Load(ctx, paths, name, strict)
	if err != nil {
		return err
	}

	if err := result.EnforceStrict(log); err != nil {
		return err
	}

	conf = result
	return nil
}

func run(cmd *cobra.Command, _ []string) error {
	v := phctx.Viper(cmd.Context())

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

	return output.FormatAndPrint(status, output.ConfigFromViper(v))
}
