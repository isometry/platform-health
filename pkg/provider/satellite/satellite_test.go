package satellite_test

import (
	"context"
	"log/slog"
	"net"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	ph "github.com/isometry/platform-health/pkg/platform_health"
	"github.com/isometry/platform-health/pkg/provider"
	"github.com/isometry/platform-health/pkg/provider/mock"
	"github.com/isometry/platform-health/pkg/provider/satellite"
	"github.com/isometry/platform-health/pkg/server"
)

func init() {
	slog.SetLogLoggerLevel(slog.LevelError)
}

type testConfig struct {
	Services []provider.Instance
}

func (c *testConfig) GetInstances() []provider.Instance {
	return c.Services
}

func TestSatellite(t *testing.T) {
	// Start listener for the main server
	listener, err := net.Listen("tcp", "localhost:0")
	if err != nil {
		t.Fatalf("Failed to set up test listener: %v", err)
	}

	port := listener.Addr().(*net.TCPAddr).Port

	config := &testConfig{}
	testServer, err := server.NewPlatformHealthServer(config)
	if err != nil {
		t.Fatalf("Failed to set up test server: %v", err)
	}

	go testServer.Serve(listener)
	defer testServer.Stop()

	tests := []struct {
		name     string
		port     int
		config   testConfig
		expected ph.Status
	}{
		{
			name:     "healthy satellite",
			port:     port,
			expected: ph.Status_HEALTHY,
		},
		{
			name: "unhealthy satellite",
			port: port,
			config: testConfig{
				Services: []provider.Instance{
					&mock.Mock{
						Name:   "Test",
						Health: ph.Status_UNHEALTHY,
					},
				},
			},
			expected: ph.Status_UNHEALTHY,
		},
		{
			name:     "invalid satellite",
			port:     1,
			expected: ph.Status_UNHEALTHY,
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

			result := component.GetHealth(context.Background())

			assert.NotNil(t, result)
			assert.Equal(t, satellite.TypeSatellite, result.GetType())
			assert.Equal(t, component.Name, result.GetName())
			assert.Equal(t, tt.expected, result.GetStatus())
		})
	}
}
