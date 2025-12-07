package root

import (
	"log/slog"
	"os"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	slogctx "github.com/veqryn/slog-context"
	"golang.org/x/term"

	"github.com/isometry/platform-health/pkg/commands/check"
	"github.com/isometry/platform-health/pkg/commands/client"
	"github.com/isometry/platform-health/pkg/commands/context"
	"github.com/isometry/platform-health/pkg/commands/flags"
	"github.com/isometry/platform-health/pkg/commands/migrate"
	"github.com/isometry/platform-health/pkg/commands/server"
	"github.com/isometry/platform-health/pkg/commands/validate"
	"github.com/isometry/platform-health/pkg/phctx"
)

func New() *cobra.Command {
	// Create owned viper instance with :: delimiter to allow dots in component names
	v := phctx.NewViper()

	cmd := &cobra.Command{
		Use:           "ph",
		Short:         "Platform Health - unified health check tool",
		Long:          `Platform Health provides health checking capabilities for distributed systems.`,
		SilenceErrors: false,
		SilenceUsage:  true,
		PersistentPreRun: func(cmd *cobra.Command, args []string) {
			// Store viper in context and bind flags
			ctx := phctx.ContextWithViper(cmd.Context(), v)
			flags.BindFlags(cmd, v)
			setupLogging(v)

			// Inject configured logger into context for downstream use
			ctx = slogctx.NewCtx(ctx, slog.Default())
			cmd.SetContext(ctx)

			// Silence errors when quiet level >= 3 (exit code only mode)
			if v.GetInt("quiet") >= 3 {
				cmd.Root().SilenceErrors = true
			}
		},
	}

	// Configure viper for environment variable support
	v.SetEnvPrefix("PH")
	v.AutomaticEnv()
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_", "-", "_"))

	// Persistent flags available to all subcommands
	pflags := cmd.PersistentFlags()
	pflags.String("log-format", "auto", "log format (auto|json|text)")
	pflags.Bool("debug", false, "debug mode")
	pflags.CountP("log-level", "v", "log level (-v=warn, -vv=info, -vvv=debug)")
	pflags.Bool("strict", false, "fail on any configuration error")

	// Add subcommands
	cmd.AddCommand(client.New())
	cmd.AddCommand(server.New())
	cmd.AddCommand(check.New())
	cmd.AddCommand(context.New())
	cmd.AddCommand(migrate.New())
	cmd.AddCommand(validate.New())

	return cmd
}

func setupLogging(v *viper.Viper) {
	verbosity := v.GetInt("log-level")
	debugMode := v.GetBool("debug")
	logFormat := v.GetString("log-format")

	level := new(slog.LevelVar)
	level.Set(slog.LevelError - slog.Level(verbosity*4))

	handlerOpts := &slog.HandlerOptions{
		AddSource: debugMode,
		Level:     level,
	}

	// Resolve "auto" format based on TTY detection
	useJSON := logFormat == "json" || (logFormat == "auto" && !term.IsTerminal(int(os.Stdout.Fd())))

	var handler slog.Handler
	if useJSON {
		handler = slog.NewJSONHandler(os.Stderr, handlerOpts)
	} else {
		handler = slog.NewTextHandler(os.Stderr, handlerOpts)
	}

	slog.SetDefault(slog.New(handler))
}
