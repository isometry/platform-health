package tcp

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"time"

	"github.com/mcuadros/go-defaults"

	"github.com/isometry/platform-health/pkg/phctx"
	ph "github.com/isometry/platform-health/pkg/platform_health"
	"github.com/isometry/platform-health/pkg/provider"
)

const ProviderType = "tcp"

type Component struct {
	Name    string        `mapstructure:"-"`
	Host    string        `mapstructure:"host"`
	Port    int           `mapstructure:"port" default:"80"`
	Closed  bool          `mapstructure:"closed" default:"false"`
	Timeout time.Duration `mapstructure:"timeout" default:"1s"`
}

func init() {
	provider.Register(ProviderType, new(Component))
}

func (c *Component) LogValue() slog.Value {
	logAttr := []slog.Attr{
		slog.String("name", c.Name),
		slog.String("host", c.Host),
		slog.Int("port", c.Port),
		slog.Bool("closed", c.Closed),
		slog.Any("timeout", c.Timeout),
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

func (c *Component) GetName() string {
	return c.Name
}

func (c *Component) SetName(name string) {
	c.Name = name
}

func (c *Component) GetHealth(ctx context.Context) *ph.HealthCheckResponse {
	log := phctx.Logger(ctx, slog.String("provider", ProviderType), slog.Any("instance", c))
	log.Debug("checking")

	ctx, cancel := context.WithTimeout(ctx, c.Timeout)
	defer cancel()

	component := &ph.HealthCheckResponse{
		Type: ProviderType,
		Name: c.Name,
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
