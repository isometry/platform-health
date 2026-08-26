package grpc

import (
	"context"
	"log/slog"
	"time"

	"github.com/mcuadros/go-defaults"
	"google.golang.org/grpc/health/grpc_health_v1"

	"github.com/isometry/platform-health/pkg/client"
	"github.com/isometry/platform-health/pkg/phctx"
	ph "github.com/isometry/platform-health/pkg/platform_health"
	"github.com/isometry/platform-health/pkg/provider"
)

const (
	ProviderType   = "grpc"
	DefaultTimeout = 1 * time.Second
)

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
	if c.GetTimeout() == 0 {
		c.SetTimeout(DefaultTimeout)
	}
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

	conn, err := client.Dial(client.DialConfig{
		Host:     c.Host,
		Port:     c.Port,
		TLS:      c.TLS,
		Insecure: c.Insecure,
	})
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
