package satellite_test

import (
	"context"
	"log/slog"
	"net"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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

// setupTestServer creates a test gRPC server and returns the port.
// The server is automatically stopped when the test completes.
func setupTestServer(t *testing.T, serverId string, config *testConfig) int {
	t.Helper()

	listener, err := net.Listen("tcp", "localhost:0")
	require.NoError(t, err)

	port := listener.Addr().(*net.TCPAddr).Port

	testServer, err := server.NewPlatformHealthServer(&serverId, config)
	require.NoError(t, err)

	go func() { _ = testServer.Serve(listener) }()

	t.Cleanup(func() {
		testServer.Stop()
	})

	return port
}

func TestSatelliteGetHealth(t *testing.T) {
	// workaround for grpc resolver with Zscaler
	resolver.SetDefaultScheme("passthrough")

	serverId := "root"
	config := &testConfig{}
	port := setupTestServer(t, serverId, config)

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
				mock.Healthy("Test"),
			},
			expected: ph.Status_HEALTHY,
		},
		{
			name: "UnhealthyComponent",
			port: port,
			config: []provider.Instance{
				mock.Unhealthy("Test"),
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
			component.SetName("TestSatellite")
			require.NoError(t, component.Setup())

			*config = tt.config

			ctx := server.ContextWithHops(context.Background(), tt.hops)

			result := component.GetHealth(ctx)

			assert.NotNil(t, result)
			assert.Equal(t, satellite.TypeSatellite, result.GetType())
			assert.Equal(t, "TestSatellite", result.GetName())
			assert.Equal(t, tt.expected, result.GetStatus())
		})
	}
}

func TestSatelliteComponents(t *testing.T) {
	resolver.SetDefaultScheme("passthrough")

	serverId := "test"
	config := &testConfig{
		mock.Healthy("allowed"),
		mock.Healthy("other"),
	}
	port := setupTestServer(t, serverId, config)

	tests := []struct {
		name           string
		components     []string              // configured allowlist
		contextPaths   server.ComponentPaths // incoming request
		expectedStatus ph.Status
		expectMessage  string
	}{
		{
			name:           "NoConfigNoContext",
			components:     nil,
			contextPaths:   nil,
			expectedStatus: ph.Status_HEALTHY,
		},
		{
			name:           "ConfiguredAsDefault",
			components:     []string{"allowed"},
			contextPaths:   nil,
			expectedStatus: ph.Status_HEALTHY,
		},
		{
			name:           "ValidContextComponent",
			components:     []string{"allowed", "other"},
			contextPaths:   server.ComponentPaths{{"allowed"}},
			expectedStatus: ph.Status_HEALTHY,
		},
		{
			name:           "InvalidContextComponent",
			components:     []string{"allowed"},
			contextPaths:   server.ComponentPaths{{"forbidden"}},
			expectedStatus: ph.Status_UNHEALTHY,
			expectMessage:  "not allowed",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sat := &satellite.Satellite{
				Name:       "test",
				Host:       "localhost",
				Port:       port,
				Timeout:    time.Second,
				Components: tt.components,
			}
			require.NoError(t, sat.Setup())

			ctx := context.Background()
			if tt.contextPaths != nil {
				ctx = server.ContextWithComponentPaths(ctx, tt.contextPaths)
			}

			result := sat.GetHealth(ctx)
			assert.Equal(t, tt.expectedStatus, result.Status)
			if tt.expectMessage != "" {
				assert.Contains(t, result.Message, tt.expectMessage)
			}
		})
	}
}

func TestSatelliteComponentFiltering(t *testing.T) {
	resolver.SetDefaultScheme("passthrough")

	serverId := "test"
	config := &testConfig{
		mock.Healthy("healthy"),
		mock.Unhealthy("unhealthy"),
	}
	port := setupTestServer(t, serverId, config)

	tests := []struct {
		name           string
		components     []string
		expectedStatus ph.Status
	}{
		{
			name:           "AllComponents",
			components:     nil,
			expectedStatus: ph.Status_UNHEALTHY,
		},
		{
			name:           "OnlyHealthy",
			components:     []string{"healthy"},
			expectedStatus: ph.Status_HEALTHY,
		},
		{
			name:           "OnlyUnhealthy",
			components:     []string{"unhealthy"},
			expectedStatus: ph.Status_UNHEALTHY,
		},
		{
			name:           "BothComponents",
			components:     []string{"healthy", "unhealthy"},
			expectedStatus: ph.Status_UNHEALTHY,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sat := &satellite.Satellite{
				Name:       "test",
				Host:       "localhost",
				Port:       port,
				Timeout:    time.Second,
				Components: tt.components,
			}
			require.NoError(t, sat.Setup())

			result := sat.GetHealth(context.Background())
			assert.Equal(t, tt.expectedStatus, result.Status)
		})
	}
}
