package grpc

import (
	"context"
	"crypto/tls"
	"fmt"
	"log/slog"
	"net"

	"github.com/mcuadros/go-defaults"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/health/grpc_health_v1"

	"github.com/isometry/platform-health/pkg/phctx"
	ph "github.com/isometry/platform-health/pkg/platform_health"
	"github.com/isometry/platform-health/pkg/provider"
)

const ProviderType = "grpc"

type Component struct {
	provider.Base
	Host     string `mapstructure:"host"`
	Port     int    `mapstructure:"port" default:"8080"`
	Service  string `mapstructure:"service"`
	TLS      bool   `mapstructure:"tls" default:"false"`
	Insecure bool   `mapstructure:"insecure" default:"false"`
}

func init() {
	provider.Register(ProviderType, new(Component))
}

func (c *Component) LogValue() slog.Value {
	logAttr := []slog.Attr{
		slog.String("name", c.GetName()),
		slog.String("host", c.Host),
		slog.Int("port", c.Port),
	}
	return slog.GroupValue(logAttr...)
}

func (c *Component) Setup() error {
	defaults.SetDefaults(c)

	return nil
}

func (c *Component) GetType() string {
	return ProviderType
}

func (c *Component) GetHealth(ctx context.Context) *ph.HealthCheckResponse {
	log := phctx.Logger(ctx, slog.String("provider", ProviderType), slog.Any("instance", c))
	log.Debug("checking")

	component := &ph.HealthCheckResponse{
		Type: ProviderType,
		Name: c.GetName(),
	}
	defer component.LogStatus(log)

	// query the standard grpc health service on host:port
	// to check if the service is healthy

	if c.Port == 443 {
		c.TLS = true
	}

	dialOptions := []grpc.DialOption{}
	if c.TLS {
		tlsConf := &tls.Config{
			ServerName: c.Host,
		}
		if c.Insecure {
			tlsConf.InsecureSkipVerify = true
		}
		dialOptions = append(dialOptions, grpc.WithTransportCredentials(credentials.NewTLS(tlsConf)))
	} else {
		dialOptions = append(dialOptions, grpc.WithTransportCredentials(insecure.NewCredentials()))
	}

	address := net.JoinHostPort(c.Host, fmt.Sprint(c.Port))
	conn, err := grpc.NewClient(address, dialOptions...)
	if err != nil {
		return component.Unhealthy(err.Error())
	}
	defer func() { _ = conn.Close() }()

	client := grpc_health_v1.NewHealthClient(conn)
	request := &grpc_health_v1.HealthCheckRequest{Service: c.Service}
	response, err := client.Check(ctx, request)
	if err != nil {
		return component.Unhealthy(err.Error())
	}

	if response.GetStatus() != grpc_health_v1.HealthCheckResponse_SERVING {
		return component.Unhealthy(response.Status.String())
	}

	return component.Healthy()
}
