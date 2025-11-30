package tcp

import (
	"context"
	"fmt"
	"log/slog"
	"net"

	"github.com/mcuadros/go-defaults"

	"github.com/isometry/platform-health/pkg/phctx"
	ph "github.com/isometry/platform-health/pkg/platform_health"
	"github.com/isometry/platform-health/pkg/provider"
)

const ProviderKind = "tcp"

type Component struct {
	provider.Base
	Host   string `mapstructure:"host"`
	Port   int    `mapstructure:"port" default:"80"`
	Closed bool   `mapstructure:"closed" default:"false"`
}

func init() {
	provider.Register(ProviderKind, new(Component))
}

func (c *Component) LogValue() slog.Value {
	logAttr := []slog.Attr{
		slog.String("name", c.GetName()),
		slog.String("host", c.Host),
		slog.Int("port", c.Port),
		slog.Bool("closed", c.Closed),
	}
	return slog.GroupValue(logAttr...)
}

func (c *Component) Setup() error {
	defaults.SetDefaults(c)

	return nil
}

func (c *Component) GetKind() string {
	return ProviderKind
}

func (c *Component) GetHealth(ctx context.Context) *ph.HealthCheckResponse {
	log := phctx.Logger(ctx, slog.String("provider", ProviderKind), slog.Any("instance", c))
	log.Debug("checking")

	component := &ph.HealthCheckResponse{
		Kind: ProviderKind,
		Name: c.GetName(),
	}
	defer component.LogStatus(log)

	address := net.JoinHostPort(c.Host, fmt.Sprint(c.Port))
	dialer := &net.Dialer{}
	conn, err := dialer.DialContext(ctx, "tcp", address)
	if err != nil {
		if c.Closed {
			return component.Healthy()
		} else {
			return component.Unhealthy(err.Error())
		}
	} else {
		_ = conn.Close()
		if c.Closed {
			return component.Unhealthy("port open")
		} else {
			return component.Healthy()
		}
	}
}
