package server

import (
	"fmt"
	"log/slog"
	"net"

	"github.com/google/uuid"
	"github.com/spf13/cobra"
	slogctx "github.com/veqryn/slog-context"

	"github.com/isometry/platform-health/pkg/commands/flags"
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

	log  *slog.Logger
	conf provider.Config
)

func New() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "server [host:port]",
		Short:   "Run gRPC health check server",
		Long:    "Start the Platform Health gRPC server to respond to health check requests.",
		Args:    cobra.MaximumNArgs(1),
		PreRunE: setup,
		RunE:    serve,
	}

	serverFlags.Register(cmd.Flags(), false)

	return cmd
}

func setup(cmd *cobra.Command, args []string) (err error) {
	log = slog.Default()

	cmd.SetContext(slogctx.NewCtx(cmd.Context(), log))

	log.Info("providers registered", slog.Any("providers", provider.ProviderList()))

	if len(args) == 1 {
		listenHost, listenPort, err = flags.ParseHostPort(args[0])
		if err != nil {
			return err
		}
	}

	conf, err = config.Load(cmd.Context(), configPaths, configName)
	return err
}

func serve(_ *cobra.Command, _ []string) (err error) {
	address := net.JoinHostPort(listenHost, fmt.Sprint(listenPort))
	listener, err := net.Listen("tcp", address)
	if err != nil {
		log.Error("failed to open listener", slog.Any("error", err))
		return err
	}

	log.Info("listening", "address", address)

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
		log.Error("failed to create server", "error", err)
		return err
	}

	return srv.Serve(listener)
}
