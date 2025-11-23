package root

import (
	"log/slog"
	"os"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/isometry/platform-health/pkg/commands/check"
	"github.com/isometry/platform-health/pkg/commands/client"
	"github.com/isometry/platform-health/pkg/commands/context"
	"github.com/isometry/platform-health/pkg/commands/flags"
	"github.com/isometry/platform-health/pkg/commands/migrate"
	"github.com/isometry/platform-health/pkg/commands/server"
	"github.com/isometry/platform-health/pkg/utils"
)

func New() *cobra.Command {
	cmd := &cobra.Command{
		Use:           "ph",
		Short:         "Platform Health - unified health check tool",
		Long:          `Platform Health provides health checking capabilities for distributed systems.`,
		SilenceErrors: true,
		SilenceUsage:  true,
		PersistentPreRun: func(cmd *cobra.Command, args []string) {
			flags.BindFlags(cmd)
			setupLogging()
		},
	}

	// Configure Viper for environment variable support
	viper.SetEnvPrefix("PH")
	viper.AutomaticEnv()
	viper.SetEnvKeyReplacer(strings.NewReplacer(".", "_", "-", "_"))

	// Persistent flags available to all subcommands
	pflags := cmd.PersistentFlags()
	pflags.String("log-format", "auto", "log format (auto|json|text)")
	pflags.Bool("debug", false, "debug mode")
	pflags.CountP("log-level", "v", "log level (-v=warn, -vv=info, -vvv=debug)")

	// Add subcommands
	cmd.AddCommand(client.New())
	cmd.AddCommand(server.New())
	cmd.AddCommand(check.New())
	cmd.AddCommand(context.New())
	cmd.AddCommand(migrate.New())

	return cmd
}

func setupLogging() {
	verbosity := viper.GetInt("log-level")
	debugMode := viper.GetBool("debug")
	logFormat := viper.GetString("log-format")

	level := new(slog.LevelVar)
	level.Set(slog.LevelError - slog.Level(verbosity*4))

	handlerOpts := &slog.HandlerOptions{
		AddSource: debugMode,
		Level:     level,
	}

	// Resolve "auto" format based on TTY detection
	useJSON := logFormat == "json" || (logFormat == "auto" && !utils.IsTTY())

	var handler slog.Handler
	if useJSON {
		handler = slog.NewJSONHandler(os.Stderr, handlerOpts)
	} else {
		handler = slog.NewTextHandler(os.Stderr, handlerOpts)
	}

	slog.SetDefault(slog.New(handler))
}
