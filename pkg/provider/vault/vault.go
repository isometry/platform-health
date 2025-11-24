package vault

import (
	"context"
	"log/slog"
	"time"

	vault "github.com/hashicorp/vault/api"
	"github.com/mcuadros/go-defaults"

	ph "github.com/isometry/platform-health/pkg/platform_health"
	"github.com/isometry/platform-health/pkg/provider"
	"github.com/isometry/platform-health/pkg/utils"
)

const ProviderType = "vault"

type Component struct {
	Name     string        `mapstructure:"-"`
	Address  string        `mapstructure:"address"`
	Timeout  time.Duration `mapstructure:"timeout" default:"1s"`
	Insecure bool          `mapstructure:"insecure"`
}

func init() {
	provider.Register(ProviderType, new(Component))
}

func (c *Component) LogValue() slog.Value {
	logAttr := []slog.Attr{
		slog.String("name", c.Name),
		slog.String("address", c.Address),
		slog.Any("timeout", c.Timeout),
		slog.Bool("insecure", c.Insecure),
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
	log := utils.ContextLogger(ctx, slog.String("provider", ProviderType), slog.Any("instance", c))
	log.Debug("checking")

	ctx, cancel := context.WithTimeout(ctx, c.Timeout)
	defer cancel()

	component := &ph.HealthCheckResponse{
		Type: ProviderType,
		Name: c.Name,
	}
	defer component.LogStatus(log)

	config := vault.DefaultConfig()
	config.Address = c.Address
	config.Timeout = c.Timeout
	if err := config.ConfigureTLS(&vault.TLSConfig{Insecure: c.Insecure}); err != nil {
		return component.Unhealthy(err.Error())
	}

	client, err := vault.NewClient(config)
	if err != nil {
		return component.Unhealthy(err.Error())
	}

	health, err := client.Sys().HealthWithContext(ctx)
	if err != nil {
		return component.Unhealthy(err.Error())
	}

	if !health.Initialized {
		return component.Unhealthy("vault is not initialized")
	}

	if health.Sealed {
		return component.Unhealthy("vault is sealed")
	}

	return component.Healthy()
}
