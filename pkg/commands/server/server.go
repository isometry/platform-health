package server

import (
	"fmt"
	"log/slog"
	"net"
	"os"
	"path/filepath"
	"strconv"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/isometry/platform-health/pkg/config"
	"github.com/isometry/platform-health/pkg/provider"
	"github.com/isometry/platform-health/pkg/server"
	"github.com/isometry/platform-health/pkg/utils"
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

func verbosityToLevel(v int) slog.Level {
	switch {
	case v > 1:
		return slog.LevelDebug
	case v == 1:
		return slog.LevelInfo
	case v < 0:
		return slog.LevelError
	default:
		return slog.LevelWarn
	}
}

var ServerCmd = &cobra.Command{
	Args:         cobra.MaximumNArgs(1),
	Use:          fmt.Sprintf("%s [flags] [host:port]", filepath.Base(os.Args[0])),
	PreRun:       setup,
	RunE:         serve,
	SilenceUsage: true,
}

func setup(_ *cobra.Command, _ []string) {
	var logLevel = new(slog.LevelVar)
	logLevel.Set(verbosityToLevel(verbosity))
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

	conf, err := config.New(c.Context(), configPaths, configName)
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

	opts := []server.Option{}
	if !noGrpcHealthV1 {
		opts = append(opts, server.WithHealthService())
	}
	if grpcReflection {
		opts = append(opts, server.WithReflection())
	}

	srv, err := server.NewPlatformHealthServer(conf, opts...)
	if err != nil {
		slog.Error("failed to create server", "error", err)
		return err
	}

	return srv.Serve(listener)
}

func init() {
	ServerCmd.Flags().StringVarP(&listenHost, "bind", "l", "", "listen on host (default all interfaces)")
	ServerCmd.Flags().Lookup("bind").NoOptDefVal = "localhost"
	viper.BindPFlag(config.ServerFlagPrefix+".listener.host", ServerCmd.Flags().Lookup("bind"))

	ServerCmd.Flags().IntVarP(&listenPort, "port", "p", 8080, "listen on port")
	viper.BindPFlag(config.ServerFlagPrefix+".listener.port", ServerCmd.Flags().Lookup("port"))

	ServerCmd.Flags().StringSliceVarP(&configPaths, "config-path", "C", []string{"/config", "."}, "configuration paths")
	viper.BindPFlag(config.ServerFlagPrefix+".config.path", ServerCmd.Flags().Lookup("config-path"))

	ServerCmd.Flags().StringVarP(&configName, "config-name", "c", "platform-health", "configuration name")
	viper.BindPFlag(config.ServerFlagPrefix+".config.name", ServerCmd.Flags().Lookup("config-name"))

	ServerCmd.Flags().BoolVarP(&noGrpcHealthV1, "no-grpc-health-v1", "H", false, "disable gRPC Health v1")
	viper.BindPFlag(config.ServerFlagPrefix+".grpc-health-v1", ServerCmd.Flags().Lookup("grpc-health-v1"))

	ServerCmd.Flags().BoolVarP(&grpcReflection, "grpc-reflection", "R", false, "enable gRPC reflection")
	viper.BindPFlag(config.ServerFlagPrefix+".grpc-reflection", ServerCmd.Flags().Lookup("grpc-reflection"))

	ServerCmd.Flags().BoolVarP(&jsonOutput, "json", "j", !utils.IsTTY(), "json logs")
	viper.BindPFlag(config.ServerFlagPrefix+".json", ServerCmd.Flags().Lookup("json"))

	ServerCmd.Flags().BoolVarP(&debugMode, "debug", "d", false, "debug mode")
	viper.BindPFlag(config.ServerFlagPrefix+".debug", ServerCmd.Flags().Lookup("debug"))

	ServerCmd.Flags().CountVarP(&verbosity, "verbosity", "v", "verbose output")
	viper.BindPFlag(config.ServerFlagPrefix+".verbosity", ServerCmd.Flags().Lookup("verbosity"))

	ServerCmd.Flags().SortFlags = false
}
