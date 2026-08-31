package satellite

import (
	"context"
	"fmt"
	"log/slog"
	"slices"
	"strings"
	"time"

	"github.com/mcuadros/go-defaults"

	"github.com/isometry/platform-health/pkg/client"
	"github.com/isometry/platform-health/pkg/phctx"
	ph "github.com/isometry/platform-health/pkg/platform_health"
	"github.com/isometry/platform-health/pkg/provider"
)

const (
	ProviderType   = "satellite"
	DefaultTimeout = 30 * time.Second
)

type Component struct {
	provider.Base

	Host       string   `mapstructure:"host"`
	Port       int      `mapstructure:"port"`
	TLS        bool     `mapstructure:"tls"`
	Insecure   bool     `mapstructure:"insecure"`
	Components []string `mapstructure:"components"`
	FailFast   bool     `mapstructure:"fail_fast"`
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
	if len(c.Components) > 0 {
		logAttr = append(logAttr, slog.Int("components", len(c.Components)))
	}
	if c.FailFast {
		logAttr = append(logAttr, slog.Bool("fail_fast", c.FailFast))
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

func (i *Component) GetHealth(ctx context.Context) *ph.HealthCheckResponse {
	log := phctx.Logger(ctx, slog.String("provider", ProviderType), slog.Any("instance", i))
	log.Debug("checking")

	component := &ph.HealthCheckResponse{
		Type: ProviderType,
		Name: i.GetName(),
	}
	defer component.LogStatus(log)

	conn, err := client.Dial(client.DialConfig{
		Host:     i.Host,
		Port:     i.Port,
		TLS:      i.TLS,
		Insecure: i.Insecure,
	})
	if err != nil {
		return component.Unhealthy(err.Error())
	}
	defer func() { _ = conn.Close() }()

	// Build request with hops for loop detection and fail-fast propagation
	request := &ph.HealthCheckRequest{
		Hops:     phctx.HopsFromContext(ctx),
		FailFast: i.FailFast || phctx.FailFastFromContext(ctx),
	}

	// Handle component filtering
	contextPaths := phctx.ComponentPathsFromContext(ctx)

	if len(i.Components) > 0 {
		if len(contextPaths) > 0 {
			// Validate context components against configured components
			for _, path := range contextPaths {
				c := strings.Join(path, "/")
				if !slices.Contains(i.Components, c) {
					return component.Unhealthy(fmt.Sprintf("component %q not allowed", c))
				}
				request.Components = append(request.Components, c)
			}
		} else {
			// No context - use configured as default
			request.Components = i.Components
		}
	} else if len(contextPaths) > 0 {
		// No static component filtering - forward context as-is
		for _, path := range contextPaths {
			request.Components = append(request.Components, strings.Join(path, "/"))
		}
	}

	status, err := ph.NewHealthClient(conn).Check(ctx, request)

	if err != nil {
		return component.Unhealthy(err.Error())
	}

	// If a loop was detected, expose serverId to assist debugging
	if status.Status == ph.Status_LOOP_DETECTED {
		component.ServerId = status.ServerId
	}

	component.Status = status.Status
	component.Details = status.Details
	component.Components = status.Components

	return component
}
