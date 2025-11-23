package check

import (
	"log/slog"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	slogctx "github.com/veqryn/slog-context"

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
		Use:     "check",
		Short:   "Perform one-shot health checks",
		Long:    "Load configuration and perform health checks without starting a server.",
		Args:    cobra.NoArgs,
		PreRunE: setup,
		RunE:    run,
	}

	checkFlags.Register(cmd.Flags(), false)

	return cmd
}

func setup(cmd *cobra.Command, _ []string) (err error) {
	flags.BindFlags(cmd, "check")

	log = slog.Default()
	cmd.SetContext(slogctx.NewCtx(cmd.Context(), log))

	log.Info("providers registered", slog.Any("providers", provider.ProviderList()))

	conf, err = config.Load(cmd.Context(),
		viper.GetStringSlice("check.config-path"),
		viper.GetString("check.config-name"))
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
		Components: viper.GetStringSlice("check.component"),
	})
	if err != nil {
		slog.Info("failed to check", slog.Any("error", err))
		return err
	}

	return flags.FormatAndPrintStatus(status, flags.OutputConfig{
		Flat:       viper.GetBool("check.flat"),
		Quiet:      viper.GetInt("check.quiet"),
		Components: viper.GetStringSlice("check.component"),
	})
}
