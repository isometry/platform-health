package satellite_test

import (
	"context"
	"log/slog"
	"net"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"google.golang.org/grpc/resolver"

	ph "github.com/isometry/platform-health/pkg/platform_health"
	"github.com/isometry/platform-health/pkg/provider"
	"github.com/isometry/platform-health/pkg/provider/mock"
	"github.com/isometry/platform-health/pkg/provider/satellite"
	"github.com/isometry/platform-health/pkg/server"
)

func init() {
	slog.SetLogLoggerLevel(slog.LevelError)
}

type testConfig []provider.Instance

func (c *testConfig) GetInstances() []provider.Instance {
	return *c
}

func TestSatelliteGetHealth(t *testing.T) {
	// workaround for grpc resolver with Zscaler
	resolver.SetDefaultScheme("passthrough")

	// Start listener for the main server
	listener, err := net.Listen("tcp", "localhost:0")
	if err != nil {
		t.Fatalf("Failed to set up test listener: %v", err)
	}

	port := listener.Addr().(*net.TCPAddr).Port

	serverId := "root"
	config := &testConfig{}
	testServer, err := server.NewPlatformHealthServer(&serverId, config)
	if err != nil {
		t.Fatalf("Failed to set up test server: %v", err)
	}

	go func() { _ = testServer.Serve(listener) }()
	defer testServer.Stop()

	tests := []struct {
		name     string
		port     int
		hops     []string
		config   testConfig
		expected ph.Status
	}{
		{
			name:     "EmptyConfig",
			port:     port,
			config:   []provider.Instance{},
			expected: ph.Status_HEALTHY,
		},
		{
			name: "HealthyComponent",
			port: port,
			config: []provider.Instance{
				&mock.Mock{
					Name:   "Test",
					Health: ph.Status_HEALTHY,
				},
			},
			expected: ph.Status_HEALTHY,
		},
		{
			name: "UnhealthyComponent",
			port: port,
			config: []provider.Instance{
				&mock.Mock{
					Name:   "Test",
					Health: ph.Status_UNHEALTHY,
				},
			},
			expected: ph.Status_UNHEALTHY,
		},
		{
			name:     "SatelliteDown",
			port:     1,
			expected: ph.Status_UNHEALTHY,
		},
		{
			name:     "SatelliteLoop",
			port:     port,
			hops:     []string{serverId},
			config:   []provider.Instance{},
			expected: ph.Status_LOOP_DETECTED,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			component := &satellite.Satellite{
				Name:    "TestSatellite",
				Host:    "localhost",
				Port:    tt.port,
				Timeout: time.Second,
			}
			component.SetDefaults()

			*config = tt.config

			ctx := server.ContextWithHops(context.Background(), tt.hops)

			result := component.GetHealth(ctx)

			assert.NotNil(t, result)
			assert.Equal(t, satellite.TypeSatellite, result.GetType())
			assert.Equal(t, component.Name, result.GetName())
			assert.Equal(t, tt.expected, result.GetStatus())
		})
	}
}
