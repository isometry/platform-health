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
	"github.com/isometry/platform-health/pkg/config"
	ph "github.com/isometry/platform-health/pkg/platform_health"
	"github.com/isometry/platform-health/pkg/provider"
	"github.com/isometry/platform-health/pkg/server"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	slogctx "github.com/veqryn/slog-context"
	"google.golang.org/protobuf/encoding/protojson"
)

var (
	listenHost     string
	listenPort     int
	configPaths    []string
	configName     string
	oneShot        bool
	noGrpcHealthV1 bool
	grpcReflection bool
	jsonOutput     bool
	debugMode      bool
	verbosity      int

	log   *slog.Logger
	level *slog.LevelVar
	conf  provider.Config
)

var ServerCmd = &cobra.Command{
	Args:    cobra.MaximumNArgs(1),
	Use:     fmt.Sprintf("%s [flags] [host:port]", filepath.Base(os.Args[0])),
	PreRunE: setup,
	RunE: func(cmd *cobra.Command, args []string) error {
		if oneShot {
			return oneshot(cmd, args)
		}
		return serve(cmd, args)
	},
	SilenceUsage: true,
}

func init() {
	viper.AutomaticEnv()
	viper.SetEnvKeyReplacer(strings.NewReplacer(".", "_", "-", "_"))

	serverFlags.register(ServerCmd.Flags(), false)
}

func setup(cmd *cobra.Command, _ []string) (err error) {
	level = new(slog.LevelVar)
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
	log = slog.Default()

	cmd.SetContext(slogctx.NewCtx(cmd.Context(), log))

	log.Info("providers registered", slog.Any("providers", provider.ProviderList()))

	conf, err = config.Load(cmd.Context(), configPaths, configName)
	return err
}

func serve(_ *cobra.Command, args []string) (err error) {
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
		log.Error("failed to open listener", slog.Any("error", err))
		return err
	}

	log.Info("listening", "address", address)

	serverId := uuid.New().String()

	var opts []server.Option
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

func oneshot(cmd *cobra.Command, _ []string) error {
	cmd.SilenceErrors = true
	level.Set(slog.LevelError)

	serverId := "oneshot"
	srv, err := server.NewPlatformHealthServer(&serverId, conf)
	if err != nil {
		log.Error("failed to create server", "error", err)
		return err
	}

	status, err := srv.Check(cmd.Context(), &ph.HealthCheckRequest{})
	if err != nil {
		slog.Info("failed to check", slog.Any("error", err))
		return err
	}

	pjson, err := protojson.Marshal(status)
	if err != nil {
		return err
	}

	fmt.Println(string(pjson))

	return status.IsHealthy()
}
