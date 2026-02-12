package server

import (
	"fmt"
	"log/slog"
	"net"

	"github.com/google/uuid"
	"github.com/spf13/cobra"

	"github.com/isometry/platform-health/internal/cliflags"
	"github.com/isometry/platform-health/pkg/config"
	"github.com/isometry/platform-health/pkg/netutil"
	"github.com/isometry/platform-health/pkg/phctx"
	"github.com/isometry/platform-health/pkg/provider"
	"github.com/isometry/platform-health/pkg/server"
)

var conf provider.Config

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
	ctx := cmd.Context()
	v := phctx.Viper(ctx)
	cliflags.BindFlags(cmd, v)

	log := phctx.Logger(ctx)
	log.Info("providers registered", slog.Any("providers", provider.ProviderList()))

	// Override with positional argument if provided
	if len(args) == 1 {
		host, port, err := netutil.ParseHostPort(args[0])
		if err != nil {
			return err
		}
		v.Set("listen", host)
		v.Set("port", port)
	}

	paths, name := cliflags.ConfigPaths(v)
	strict := v.GetBool("strict")

	result, err := config.Load(ctx, paths, name, strict)
	if err != nil {
		return err
	}

	// In strict mode, fail if any configuration errors were found
	if strict {
		if err := result.EnforceStrict(log); err != nil {
			return err
		}
	}

	conf = result
	return nil
}

func serve(cmd *cobra.Command, _ []string) (err error) {
	ctx := cmd.Context()
	v := phctx.Viper(ctx)
	log := phctx.Logger(ctx)

	address := net.JoinHostPort(
		v.GetString("listen"),
		fmt.Sprint(v.GetInt("port")))
	listener, err := net.Listen("tcp", address)
	if err != nil {
		log.Error("failed to open listener", slog.Any("error", err))
		return err
	}

	log.Info("listening", "address", address)

	serverId := uuid.New().String()

	opts := []server.Option{
		server.WithParallelism(v.GetInt("parallelism")),
	}
	if !v.GetBool("no-grpc-health-v1") {
		opts = append(opts, server.WithHealthService())
	}
	if v.GetBool("grpc-reflection") {
		opts = append(opts, server.WithReflection())
	}

	srv, err := server.NewPlatformHealthServer(&serverId, conf, opts...)
	if err != nil {
		log.Error("failed to create server", "error", err)
		return err
	}

	return srv.Serve(listener)
}
