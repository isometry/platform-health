package grpc

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
	"google.golang.org/grpc/health/grpc_health_v1"

	ph "github.com/isometry/platform-health/pkg/platform_health"
	"github.com/isometry/platform-health/pkg/provider"
	"github.com/isometry/platform-health/pkg/utils"
)

var TypeGRPC = "grpc"

type GRPC struct {
	Name     string        `mapstructure:"name"`
	Host     string        `mapstructure:"host"`
	Port     int           `mapstructure:"port"`
	Service  string        `mapstructure:"service"`
	TLS      bool          `mapstructure:"tls" default:"false"`
	Insecure bool          `mapstructure:"insecure" default:"false"`
	Timeout  time.Duration `mapstructure:"timeout" default:"1s"`
}

func init() {
	provider.Register(TypeGRPC, new(GRPC))
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

func (i *GRPC) SetDefaults() {
	defaults.SetDefaults(i)
}

func (i *GRPC) GetType() string {
	return TypeGRPC
}

func (i *GRPC) GetName() string {
	return i.Name
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
	conn, err := grpc.DialContext(ctx, address, dialOptions...)
	if err != nil {
		return component.Unhealthy(err.Error())
	}
	defer conn.Close()

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
