package satellite

import (
	"context"
	"crypto/tls"
	"fmt"
	"log/slog"
	"net"
	"time"

	"github.com/mcuadros/go-defaults"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"

	ph "github.com/isometry/platform-health/pkg/platform_health"
	"github.com/isometry/platform-health/pkg/provider"
	"github.com/isometry/platform-health/pkg/utils"
)

const TypeSatellite = "satellite"

type Satellite struct {
	Name     string        `mapstructure:"name"`
	Host     string        `mapstructure:"host"`
	Port     int           `mapstructure:"port"`
	TLS      bool          `mapstructure:"tls"`
	Insecure bool          `mapstructure:"insecure"`
	Timeout  time.Duration `mapstructure:"timeout" default:"30s"`
}

func init() {
	provider.Register(TypeSatellite, new(Satellite))
}

func (i *Satellite) LogValue() slog.Value {
	logAttr := []slog.Attr{
		slog.String("name", i.Name),
		slog.String("host", i.Host),
		slog.Int("port", i.Port),
		slog.Any("timeout", i.Timeout),
	}
	return slog.GroupValue(logAttr...)
}

func (i *Satellite) SetDefaults() {
	defaults.SetDefaults(i)
}

func (i *Satellite) GetType() string {
	return TypeSatellite
}

func (i *Satellite) GetName() string {
	return i.Name
}

func (i *Satellite) GetHealth(ctx context.Context) *ph.HealthCheckResponse {
	log := utils.ContextLogger(ctx, slog.String("provider", TypeSatellite), slog.Any("instance", i))
	log.Debug("checking")

	component := &ph.HealthCheckResponse{
		Type: TypeSatellite,
		Name: i.Name,
	}
	defer component.LogStatus(log)

	ctx, cancel := context.WithTimeout(ctx, i.Timeout)
	defer cancel()

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
	conn, err := grpc.DialContext(ctx, address, dialOptions...)
	if err != nil {
		return component.Unhealthy(err.Error())
	}

	status, err := ph.NewHealthClient(conn).Check(ctx, nil)

	if err != nil {
		return component.Unhealthy(err.Error())
	}

	component.Status = status.Status
	component.Components = status.Components

	return component
}
