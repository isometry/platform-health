package client

import (
	"context"
	"crypto/tls"
	"fmt"
	"log/slog"
	"net"
	"time"

	"github.com/spf13/cobra"
	slogctx "github.com/veqryn/slog-context"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"

	"github.com/isometry/platform-health/pkg/commands/flags"
	ph "github.com/isometry/platform-health/pkg/platform_health"
	_ "github.com/isometry/platform-health/pkg/platform_health/details"
)

var (
	targetHost         string
	targetPort         int
	tlsClient          bool
	insecureSkipVerify bool
	clientTimeout      time.Duration
	components         []string
	flatOutput         bool
	quietLevel         int

	log *slog.Logger
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

func setup(_ *cobra.Command, args []string) (err error) {
	log = slog.Default()

	if len(args) == 1 {
		targetHost, targetPort, err = flags.ParseHostPort(args[0])
		if err != nil {
			return err
		}
	}

	return nil
}

func query(cmd *cobra.Command, _ []string) (err error) {
	address := net.JoinHostPort(targetHost, fmt.Sprint(targetPort))

	ctx, cancel := context.WithTimeout(context.Background(), clientTimeout)
	defer cancel()

	ctx = slogctx.NewCtx(ctx, log)
	cmd.SetContext(ctx)

	if targetPort == 443 || targetPort == 8443 {
		tlsClient = true
	}

	dialOptions := []grpc.DialOption{}
	if tlsClient {
		tlsConf := &tls.Config{
			ServerName: targetHost,
		}
		if insecureSkipVerify {
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
		Components: components,
	})
	if err != nil {
		log.Info("failed to check", slog.Any("error", err))
		return err
	}

	return flags.FormatAndPrintStatus(status, flatOutput, quietLevel)
}
