package client

import (
	"context"
	"crypto/tls"
	"fmt"
	"log/slog"
	"net"

	"github.com/spf13/cobra"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"

	"github.com/isometry/platform-health/internal/cli"
	"github.com/isometry/platform-health/pkg/phctx"
	ph "github.com/isometry/platform-health/pkg/platform_health"
	_ "github.com/isometry/platform-health/pkg/platform_health/details"
)

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
	v := phctx.Viper(cmd.Context())
	cli.BindFlags(cmd, v)

	// Override with positional argument if provided
	if len(args) == 1 {
		host, port, err := cli.ParseHostPort(args[0])
		if err != nil {
			return err
		}
		v.Set("server", host)
		v.Set("port", port)
	}

	return nil
}

func query(cmd *cobra.Command, _ []string) (err error) {
	v := phctx.Viper(cmd.Context())
	targetHost := v.GetString("server")
	targetPort := v.GetInt("port")
	address := net.JoinHostPort(targetHost, fmt.Sprint(targetPort))

	ctx, cancel := context.WithTimeout(cmd.Context(), v.GetDuration("timeout"))
	defer cancel()

	log := phctx.Logger(ctx)

	tlsEnabled := v.GetBool("tls") || targetPort == 443 || targetPort == 8443

	dialOptions := []grpc.DialOption{}
	if tlsEnabled {
		tlsConf := &tls.Config{
			ServerName: targetHost,
		}
		if v.GetBool("insecure") {
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
		Components: v.GetStringSlice("component"),
		FailFast:   v.GetBool("fail-fast"),
	})
	if err != nil {
		log.Info("failed to check", slog.Any("error", err))
		return err
	}

	return cli.FormatAndPrintStatus(status, cli.OutputConfigFromViper(v))
}
