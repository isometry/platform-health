package vault

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	vault "github.com/hashicorp/vault/api"
	"github.com/mcuadros/go-defaults"
	"github.com/spf13/pflag"

	"github.com/isometry/platform-health/pkg/commands/flags"
	ph "github.com/isometry/platform-health/pkg/platform_health"
	"github.com/isometry/platform-health/pkg/provider"
	"github.com/isometry/platform-health/pkg/utils"
)

const TypeVault = "vault"

type Vault struct {
	Name     string        `mapstructure:"-"`
	Address  string        `mapstructure:"address"`
	Timeout  time.Duration `mapstructure:"timeout" default:"1s"`
	Insecure bool          `mapstructure:"insecure"`
}

// Compile-time interface check
var _ provider.FlagConfigurable = (*Vault)(nil)

func init() {
	provider.Register(TypeVault, new(Vault))
}

// GetProviderFlags returns flag definitions for CLI configuration.
func (i *Vault) GetProviderFlags() flags.FlagValues {
	return flags.FlagValues{
		"address": {
			Kind:  "string",
			Usage: "Vault server URL",
		},
		"timeout": {
			Kind:         "duration",
			DefaultValue: "1s",
			Usage:        "request timeout",
		},
		"insecure": {
			Kind:         "bool",
			DefaultValue: false,
			Usage:        "skip TLS verification",
		},
	}
}

// ConfigureFromFlags applies flag values to the provider.
func (i *Vault) ConfigureFromFlags(fs *pflag.FlagSet) error {
	var errs []error
	var err error

	if i.Address, err = fs.GetString("address"); err != nil {
		errs = append(errs, err)
	}
	if i.Timeout, err = fs.GetDuration("timeout"); err != nil {
		errs = append(errs, err)
	}
	if i.Insecure, err = fs.GetBool("insecure"); err != nil {
		errs = append(errs, err)
	}

	if len(errs) > 0 {
		return fmt.Errorf("flag errors: %w", errors.Join(errs...))
	}

	if i.Address == "" {
		return fmt.Errorf("address is required")
	}
	return nil
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

func (i *Vault) Setup() error {
	defaults.SetDefaults(i)

	return nil
}

func (i *Vault) GetType() string {
	return TypeVault
}

func (i *Vault) GetName() string {
	return i.Name
}

func (i *Vault) SetName(name string) {
	i.Name = name
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
	if err := config.ConfigureTLS(&vault.TLSConfig{Insecure: i.Insecure}); err != nil {
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
