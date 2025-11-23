package grpc

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"time"

	"github.com/mcuadros/go-defaults"
	"github.com/spf13/pflag"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/health/grpc_health_v1"

	"github.com/isometry/platform-health/pkg/commands/flags"
	ph "github.com/isometry/platform-health/pkg/platform_health"
	"github.com/isometry/platform-health/pkg/provider"
	"github.com/isometry/platform-health/pkg/utils"
)

var TypeGRPC = "grpc"

type GRPC struct {
	Name     string        `mapstructure:"-"`
	Host     string        `mapstructure:"host"`
	Port     int           `mapstructure:"port"`
	Service  string        `mapstructure:"service"`
	TLS      bool          `mapstructure:"tls" default:"false"`
	Insecure bool          `mapstructure:"insecure" default:"false"`
	Timeout  time.Duration `mapstructure:"timeout" default:"1s"`
}

// Compile-time interface check
var _ provider.FlagConfigurable = (*GRPC)(nil)

func init() {
	provider.Register(TypeGRPC, new(GRPC))
}

// GetProviderFlags returns flag definitions for CLI configuration.
func (i *GRPC) GetProviderFlags() flags.FlagValues {
	return flags.FlagValues{
		"host": {
			Kind:  "string",
			Usage: "target hostname",
		},
		"port": {
			Kind:  "int",
			Usage: "target port",
		},
		"service": {
			Kind:  "string",
			Usage: "gRPC service name to check",
		},
		"tls": {
			Kind:         "bool",
			DefaultValue: false,
			Usage:        "use TLS",
		},
		"insecure": {
			Kind:         "bool",
			DefaultValue: false,
			Usage:        "skip certificate verification",
		},
		"timeout": {
			Kind:         "duration",
			DefaultValue: "1s",
			Usage:        "request timeout",
		},
	}
}

// ConfigureFromFlags applies flag values to the provider.
func (i *GRPC) ConfigureFromFlags(fs *pflag.FlagSet) error {
	var errs []error
	var err error

	if i.Host, err = fs.GetString("host"); err != nil {
		errs = append(errs, err)
	}
	if i.Port, err = fs.GetInt("port"); err != nil {
		errs = append(errs, err)
	}
	if i.Service, err = fs.GetString("service"); err != nil {
		errs = append(errs, err)
	}
	if i.TLS, err = fs.GetBool("tls"); err != nil {
		errs = append(errs, err)
	}
	if i.Insecure, err = fs.GetBool("insecure"); err != nil {
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
	if i.Port == 0 {
		return fmt.Errorf("port is required")
	}
	return nil
}

func (i *GRPC) LogValue() slog.Value {
	logAttr := []slog.Attr{
		slog.String("name", i.Name),
		slog.String("host", i.Host),
		slog.Int("port", i.Port),
		slog.Any("timeout", i.Timeout),
	}
	return slog.GroupValue(logAttr...)
}

func (i *GRPC) Setup() error {
	defaults.SetDefaults(i)

	return nil
}

func (i *GRPC) GetType() string {
	return TypeGRPC
}

func (i *GRPC) GetName() string {
	return i.Name
}

func (i *GRPC) SetName(name string) {
	i.Name = name
}

func (i *GRPC) GetHealth(ctx context.Context) *ph.HealthCheckResponse {
	log := utils.ContextLogger(ctx, slog.String("provider", TypeGRPC), slog.Any("instance", i))
	log.Debug("checking")

	component := &ph.HealthCheckResponse{
		Type: TypeGRPC,
		Name: i.Name,
	}
	defer component.LogStatus(log)

	// query the standard grpc health service on host:port
	// to check if the service is healthy

	ctx, cancel := context.WithTimeout(ctx, i.Timeout)
	defer cancel()

	if i.Port == 443 {
		i.TLS = true
	}

	dialOptions := []grpc.DialOption{}
	if i.TLS {
		tlsConf := &tls.Config{
			ServerName: i.Host,
		}
		if i.Insecure {
			tlsConf.InsecureSkipVerify = true
		}
		dialOptions = append(dialOptions, grpc.WithTransportCredentials(credentials.NewTLS(tlsConf)))
	} else {
		dialOptions = append(dialOptions, grpc.WithTransportCredentials(insecure.NewCredentials()))
	}

	address := net.JoinHostPort(i.Host, fmt.Sprint(i.Port))
	conn, err := grpc.NewClient(address, dialOptions...)
	if err != nil {
		return component.Unhealthy(err.Error())
	}
	defer func() { _ = conn.Close() }()

	client := grpc_health_v1.NewHealthClient(conn)
	request := &grpc_health_v1.HealthCheckRequest{Service: i.Service}
	response, err := client.Check(ctx, request)
	if err != nil {
		return component.Unhealthy(err.Error())
	}

	if response.GetStatus() != grpc_health_v1.HealthCheckResponse_SERVING {
		return component.Unhealthy(response.Status.String())
	}

	return component.Healthy()
}
