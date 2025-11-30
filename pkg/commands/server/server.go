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
	"github.com/isometry/platform-health/pkg/phctx"
	"github.com/isometry/platform-health/pkg/provider"
	"github.com/isometry/platform-health/pkg/server"
)

var (
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
	v := phctx.Viper(cmd.Context())
	flags.BindFlags(cmd, v)

	log = slog.Default()
	cmd.SetContext(slogctx.NewCtx(cmd.Context(), log))

	log.Info("providers registered", slog.Any("providers", provider.ProviderList()))

	// Override with positional argument if provided
	if len(args) == 1 {
		host, port, err := flags.ParseHostPort(args[0])
		if err != nil {
			return err
		}
		v.Set("listen", host)
		v.Set("port", port)
	}

	paths, name := flags.ConfigPaths(v)
	strict := v.GetBool("strict")

	result, err := config.Load(cmd.Context(), paths, name, strict)
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

func serve(cmd *cobra.Command, _ []string) (err error) {
	v := phctx.Viper(cmd.Context())

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
