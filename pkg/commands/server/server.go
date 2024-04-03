package server

import (
	"fmt"
	"log/slog"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/google/uuid"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/isometry/platform-health/pkg/config"
	"github.com/isometry/platform-health/pkg/provider"
	"github.com/isometry/platform-health/pkg/server"
)

var (
	listenHost     string
	listenPort     int
	configPaths    []string
	configName     string
	noGrpcHealthV1 bool
	grpcReflection bool
	jsonOutput     bool
	debugMode      bool
	verbosity      int
)

var ServerCmd = &cobra.Command{
	Args:         cobra.MaximumNArgs(1),
	Use:          fmt.Sprintf("%s [flags] [host:port]", filepath.Base(os.Args[0])),
	PreRun:       setup,
	RunE:         serve,
	SilenceUsage: true,
}

func init() {
	viper.AutomaticEnv()
	viper.SetEnvKeyReplacer(strings.NewReplacer(".", "_", "-", "_"))

	serverFlags.register(ServerCmd.Flags(), false)
}

func setup(cmd *cobra.Command, _ []string) {
	var logLevel = new(slog.LevelVar)
	logLevel.Set(slog.LevelWarn - slog.Level(verbosity*4))

	opts := &slog.HandlerOptions{
		AddSource: debugMode,
		Level:     logLevel,
	}

	var handler slog.Handler
	if jsonOutput {
		handler = slog.NewJSONHandler(os.Stdout, opts)
	} else {
		handler = slog.NewTextHandler(os.Stdout, opts)
	}

	slog.SetDefault(slog.New(handler))
}

func serve(c *cobra.Command, args []string) error {
	slog.Info("providers registered", slog.Any("providers", provider.ProviderList()))

	conf, err := config.Load(c.Context(), configPaths, configName)
	if err != nil {
		return err
	}

	if len(args) == 1 {
		var listenPortStr string
		listenHost, listenPortStr, err = net.SplitHostPort(args[0])
		if err != nil {
			return err
		}
		listenPort, err = strconv.Atoi(listenPortStr)
		if err != nil {
			return err
		}
	}

	address := net.JoinHostPort(listenHost, fmt.Sprint(listenPort))
	listener, err := net.Listen("tcp", address)
	if err != nil {
		slog.Error("failed to open listener", slog.Any("error", err))
		return err
	}

	slog.Info("listening", "address", address)

	serverId := uuid.New().String()

	opts := []server.Option{}
	if !noGrpcHealthV1 {
		opts = append(opts, server.WithHealthService())
	}
	if grpcReflection {
		opts = append(opts, server.WithReflection())
	}

	srv, err := server.NewPlatformHealthServer(&serverId, conf, opts...)
	if err != nil {
		slog.Error("failed to create server", "error", err)
		return err
	}

	return srv.Serve(listener)
}
