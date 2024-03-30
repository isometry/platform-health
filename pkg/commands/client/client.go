package client

import (
	"context"
	"crypto/tls"
	"fmt"
	"log/slog"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	slogctx "github.com/veqryn/slog-context"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/protobuf/encoding/protojson"

	ph "github.com/isometry/platform-health/pkg/platform_health"
	_ "github.com/isometry/platform-health/pkg/platform_health/details"
)

var (
	targetHost         string
	targetPort         int
	tlsClient          bool
	insecureSkipVerify bool
	clientTimeout      time.Duration
	flatOutput         bool
	quietLevel         int
)

var ClientCmd = &cobra.Command{
	Args:         cobra.MaximumNArgs(1),
	Use:          fmt.Sprintf("%s [flags] [host:port]", filepath.Base(os.Args[0])),
	PreRunE:      setup,
	RunE:         query,
	SilenceUsage: true,
}

func setup(c *cobra.Command, args []string) (err error) {
	handler := slog.NewJSONHandler(os.Stdout, nil)
	slog.SetDefault(slog.New(handler))

	if len(args) == 1 {
		var targetPortStr string
		targetHost, targetPortStr, err = net.SplitHostPort(args[0])
		if err != nil {
			return err
		}
		targetPort, err = strconv.Atoi(targetPortStr)
		if err != nil {
			return err
		}
	}

	return nil
}

func query(c *cobra.Command, _ []string) (err error) {
	address := net.JoinHostPort(targetHost, fmt.Sprint(targetPort))

	ctx, cancel := context.WithTimeout(context.Background(), clientTimeout)
	defer cancel()

	ctx = slogctx.NewCtx(ctx, slog.Default())
	c.SetContext(ctx)

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

	conn, err := grpc.DialContext(ctx, address, dialOptions...)
	if err != nil {
		slog.Error("failed to connect to server", slog.String("server", targetHost), slog.Any("error", err))
		return err
	}

	health := ph.NewHealthClient(conn)

	status, err := health.Check(ctx, &ph.HealthCheckRequest{})
	if err != nil {
		slog.Info("failed to check", slog.Any("error", err))
		return err
	}

	switch {
	case quietLevel > 1:
		c.SilenceUsage = true
		if status.Status == ph.Status_HEALTHY {
			return nil
		} else {
			return fmt.Errorf("unhealthy")
		}
	case quietLevel > 0:
		status.Components = nil
	}

	if flatOutput {
		status.Components = status.Flatten(status.Name)
	}

	pjson, err := protojson.Marshal(status)
	if err != nil {
		return err
	}

	fmt.Println(string(pjson))

	return nil
}

func init() {
	ClientCmd.Flags().StringVarP(&targetHost, "server", "s", "localhost", "server host")
	viper.BindPFlag("server", ClientCmd.Flags().Lookup("server"))
	ClientCmd.Flags().IntVarP(&targetPort, "port", "p", 8080, "server port")
	viper.BindPFlag("server", ClientCmd.Flags().Lookup("server"))

	ClientCmd.Flags().BoolVar(&tlsClient, "tls", false, "enable tls")
	viper.BindPFlag("tls", ClientCmd.Flags().Lookup("tls"))

	ClientCmd.Flags().BoolVarP(&insecureSkipVerify, "insecure", "k", false, "disable certificate verification")
	viper.BindPFlag("insecure", ClientCmd.Flags().Lookup("insecure"))

	ClientCmd.Flags().DurationVarP(&clientTimeout, "timeout", "t", 10*time.Second, "timeout")
	viper.BindPFlag("timeout", ClientCmd.Flags().Lookup("timeout"))

	ClientCmd.Flags().BoolVarP(&flatOutput, "flat", "f", false, "flat output")
	viper.BindPFlag("flat", ClientCmd.Flags().Lookup("flat"))

	ClientCmd.Flags().CountVarP(&quietLevel, "quiet", "q", "quiet output")
	viper.BindPFlag("quiet", ClientCmd.Flags().Lookup("quiet"))

	ClientCmd.Flags().SortFlags = false
}
