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

const TypeVault = "vault"

type Vault struct {
	Name     string        `mapstructure:"name"`
	Address  string        `mapstructure:"address"`
	Timeout  time.Duration `mapstructure:"timeout" default:"1s"`
	Insecure bool          `mapstructure:"insecure"`
}

func init() {
	provider.Register(TypeVault, new(Vault))
}

func (i *Vault) LogValue() slog.Value {
	logAttr := []slog.Attr{
		slog.String("name", i.Name),
		slog.String("address", i.Address),
		slog.Any("timeout", i.Timeout),
		slog.Bool("insecure", i.Insecure),
	}
	return slog.GroupValue(logAttr...)
}

func (i *Vault) SetDefaults() {
	defaults.SetDefaults(i)
}

func (i *Vault) GetType() string {
	return TypeVault
}

func (i *Vault) GetName() string {
	return i.Name
}

func (i *Vault) GetHealth(ctx context.Context) *ph.HealthCheckResponse {
	log := utils.ContextLogger(ctx, slog.String("provider", TypeVault), slog.Any("instance", i))
	log.Debug("checking")

	ctx, cancel := context.WithTimeout(ctx, i.Timeout)
	defer cancel()

	component := &ph.HealthCheckResponse{
		Type: TypeVault,
		Name: i.Name,
	}
	defer component.LogStatus(log)

	config := vault.DefaultConfig()
	config.Address = i.Address
	config.Timeout = i.Timeout
	config.ConfigureTLS(&vault.TLSConfig{Insecure: i.Insecure})

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
