package tcp

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"time"

	"github.com/mcuadros/go-defaults"
	"github.com/spf13/pflag"

	"github.com/isometry/platform-health/pkg/commands/flags"
	ph "github.com/isometry/platform-health/pkg/platform_health"
	"github.com/isometry/platform-health/pkg/provider"
	"github.com/isometry/platform-health/pkg/utils"
)

const TypeTCP = "tcp"

type TCP struct {
	Name    string        `mapstructure:"-"`
	Host    string        `mapstructure:"host"`
	Port    int           `mapstructure:"port" default:"80"`
	Closed  bool          `mapstructure:"closed" default:"false"`
	Timeout time.Duration `mapstructure:"timeout" default:"1s"`
}

// Compile-time interface check
var _ provider.FlagConfigurable = (*TCP)(nil)

func init() {
	provider.Register(TypeTCP, new(TCP))
}

// GetProviderFlags returns flag definitions for CLI configuration.
func (i *TCP) GetProviderFlags() flags.FlagValues {
	return flags.FlagValues{
		"host": {
			Kind:  "string",
			Usage: "target hostname",
		},
		"port": {
			Kind:         "int",
			DefaultValue: 80,
			Usage:        "target port",
		},
		"closed": {
			Kind:  "bool",
			Usage: "expect port to be closed",
		},
		"timeout": {
			Kind:         "duration",
			DefaultValue: "1s",
			Usage:        "connection timeout",
		},
	}
}

// ConfigureFromFlags applies flag values to the provider.
func (i *TCP) ConfigureFromFlags(fs *pflag.FlagSet) error {
	var errs []error
	var err error

	if i.Host, err = fs.GetString("host"); err != nil {
		errs = append(errs, err)
	}
	if i.Port, err = fs.GetInt("port"); err != nil {
		errs = append(errs, err)
	}
	if i.Closed, err = fs.GetBool("closed"); err != nil {
		errs = append(errs, err)
	}
	if i.Timeout, err = fs.GetDuration("timeout"); err != nil {
		errs = append(errs, err)
	}

	if len(errs) > 0 {
		return fmt.Errorf("flag errors: %w", errors.Join(errs...))
	}

	if i.Host == "" {
		return fmt.Errorf("host is required")
	}
	return nil
}

func (i *TCP) LogValue() slog.Value {
	logAttr := []slog.Attr{
		slog.String("name", i.Name),
		slog.String("host", i.Host),
		slog.Int("port", i.Port),
		slog.Bool("closed", i.Closed),
		slog.Any("timeout", i.Timeout),
	}
	return slog.GroupValue(logAttr...)
}

func (i *TCP) Setup() error {
	defaults.SetDefaults(i)

	return nil
}

func (i *TCP) GetType() string {
	return TypeTCP
}

func (i *TCP) GetName() string {
	return i.Name
}

func (i *TCP) SetName(name string) {
	i.Name = name
}

func (i *TCP) GetHealth(ctx context.Context) *ph.HealthCheckResponse {
	log := utils.ContextLogger(ctx, slog.String("provider", TypeTCP), slog.Any("instance", i))
	log.Debug("checking")

	ctx, cancel := context.WithTimeout(ctx, i.Timeout)
	defer cancel()

	component := &ph.HealthCheckResponse{
		Type: TypeTCP,
		Name: i.Name,
	}
	defer component.LogStatus(log)

	address := net.JoinHostPort(i.Host, fmt.Sprint(i.Port))
	dialer := &net.Dialer{}
	conn, err := dialer.DialContext(ctx, "tcp", address)
	if err != nil {
		if i.Closed {
			return component.Healthy()
		} else {
			return component.Unhealthy(err.Error())
		}
	} else {
		_ = conn.Close()
		if i.Closed {
			return component.Unhealthy("port open")
		} else {
			return component.Healthy()
		}
	}
}
