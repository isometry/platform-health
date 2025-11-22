package client

import (
	"context"
	"crypto/tls"
	"fmt"
	"log/slog"
	"net"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	slogctx "github.com/veqryn/slog-context"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"

	"github.com/isometry/platform-health/pkg/commands/flags"
	ph "github.com/isometry/platform-health/pkg/platform_health"
	_ "github.com/isometry/platform-health/pkg/platform_health/details"
)

var log *slog.Logger

func New() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "client [host:port]",
		Short:   "Query health check server",
		Long:    "Connect to a Platform Health gRPC server and query its health status.",
		Args:    cobra.MaximumNArgs(1),
		PreRunE: setup,
		RunE:    query,
	}

	clientFlags.Register(cmd.Flags(), false)

	return cmd
}

func setup(cmd *cobra.Command, args []string) (err error) {
	flags.BindFlags(cmd, "client")

	log = slog.Default()

	// Override with positional argument if provided
	if len(args) == 1 {
		host, port, err := flags.ParseHostPort(args[0])
		if err != nil {
			return err
		}
		viper.Set("client.server", host)
		viper.Set("client.port", port)
	}

	return nil
}

func query(cmd *cobra.Command, _ []string) (err error) {
	targetHost := viper.GetString("client.server")
	targetPort := viper.GetInt("client.port")
	address := net.JoinHostPort(targetHost, fmt.Sprint(targetPort))

	ctx, cancel := context.WithTimeout(context.Background(), viper.GetDuration("client.timeout"))
	defer cancel()

	ctx = slogctx.NewCtx(ctx, log)
	cmd.SetContext(ctx)

	tlsEnabled := viper.GetBool("client.tls") || targetPort == 443 || targetPort == 8443

	dialOptions := []grpc.DialOption{}
	if tlsEnabled {
		tlsConf := &tls.Config{
			ServerName: targetHost,
		}
		if viper.GetBool("client.insecure") {
			tlsConf.InsecureSkipVerify = true
		}
		dialOptions = append(dialOptions, grpc.WithTransportCredentials(credentials.NewTLS(tlsConf)))
	} else {
		dialOptions = append(dialOptions, grpc.WithTransportCredentials(insecure.NewCredentials()))
	}

	conn, err := grpc.NewClient(address, dialOptions...)
	if err != nil {
		log.Error("failed to connect to server", slog.String("server", targetHost), slog.Any("error", err))
		return err
	}

	health := ph.NewHealthClient(conn)

	status, err := health.Check(ctx, &ph.HealthCheckRequest{
		Components: viper.GetStringSlice("client.component"),
	})
	if err != nil {
		log.Info("failed to check", slog.Any("error", err))
		return err
	}

	return flags.FormatAndPrintStatus(status, flags.OutputConfig{
		Flat:       viper.GetBool("client.flat"),
		Quiet:      viper.GetInt("client.quiet"),
		Components: viper.GetStringSlice("client.component"),
	})
}
