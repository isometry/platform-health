package satellite

import (
	"context"
	"crypto/tls"
	"fmt"
	"log/slog"
	"net"
	"slices"
	"strings"
	"time"

	"github.com/mcuadros/go-defaults"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"

	ph "github.com/isometry/platform-health/pkg/platform_health"
	"github.com/isometry/platform-health/pkg/provider"
	"github.com/isometry/platform-health/pkg/server"
	"github.com/isometry/platform-health/pkg/utils"
)

const TypeSatellite = "satellite"

type Satellite struct {
	Name       string        `mapstructure:"-"`
	Host       string        `mapstructure:"host"`
	Port       int           `mapstructure:"port"`
	TLS        bool          `mapstructure:"tls"`
	Insecure   bool          `mapstructure:"insecure"`
	Timeout    time.Duration `mapstructure:"timeout" default:"30s"`
	Components []string      `mapstructure:"components"`
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
	if len(i.Components) > 0 {
		logAttr = append(logAttr, slog.Int("components", len(i.Components)))
	}
	return slog.GroupValue(logAttr...)
}

func (i *Satellite) Setup() error {
	defaults.SetDefaults(i)

	return nil
}

func (i *Satellite) GetType() string {
	return TypeSatellite
}

func (i *Satellite) GetName() string {
	return i.Name
}

func (i *Satellite) SetName(name string) {
	i.Name = name
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

	if i.Port == 443 || i.Port == 8443 {
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

	// Build request with hops for loop detection
	request := &ph.HealthCheckRequest{
		Hops: server.HopsFromContext(ctx),
	}

	// Handle component filtering
	contextPaths := server.ComponentPathsFromContext(ctx)

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
