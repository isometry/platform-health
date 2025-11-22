package root

import (
	"log/slog"
	"os"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/isometry/platform-health/pkg/commands/check"
	"github.com/isometry/platform-health/pkg/commands/client"
	"github.com/isometry/platform-health/pkg/commands/server"
	"github.com/isometry/platform-health/pkg/utils"
)

var (
	jsonOutput bool
	debugMode  bool
	verbosity  int
)

func New() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "ph",
		Short: "Platform Health - unified health check tool",
		Long: `Platform Health provides health checking capabilities for distributed systems.

Use 'ph server' to run the gRPC health check server.
Use 'ph client' to query a health check server.
Use 'ph check' to perform one-shot health checks locally.`,
		SilenceErrors: true,
		SilenceUsage:  true,
		PersistentPreRun: func(cmd *cobra.Command, args []string) {
			setupLogging()
		},
	}

	viper.AutomaticEnv()
	viper.SetEnvKeyReplacer(strings.NewReplacer(".", "_", "-", "_"))

	// Persistent flags available to all subcommands
	pflags := cmd.PersistentFlags()
	pflags.BoolVarP(&jsonOutput, "json", "j", !utils.IsTTY(), "json logs")
	pflags.BoolVarP(&debugMode, "debug", "d", false, "debug mode")
	pflags.CountVarP(&verbosity, "verbosity", "v", "verbose output")

	// Add subcommands
	cmd.AddCommand(client.New())
	cmd.AddCommand(server.New())
	cmd.AddCommand(check.New())

	return cmd
}

func setupLogging() {
	level := new(slog.LevelVar)
	level.Set(slog.LevelWarn - slog.Level(verbosity*4))

	handlerOpts := &slog.HandlerOptions{
		AddSource: debugMode,
		Level:     level,
	}

	var handler slog.Handler
	if jsonOutput {
		handler = slog.NewJSONHandler(os.Stderr, handlerOpts)
	} else {
		handler = slog.NewTextHandler(os.Stderr, handlerOpts)
	}

	slog.SetDefault(slog.New(handler))
}
