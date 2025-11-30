package satellite_test

import (
	"log/slog"
	"net"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/resolver"

	"github.com/isometry/platform-health/pkg/phctx"
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
			instance := &satellite.Component{
				Host: "localhost",
				Port: tt.port,
			}
			instance.SetName("TestSatellite")
			instance.SetTimeout(time.Second)
			require.NoError(t, instance.Setup())

			*config = tt.config

			ctx := phctx.ContextWithHops(t.Context(), tt.hops)

			result := instance.GetHealth(ctx)

			assert.NotNil(t, result)
			assert.Equal(t, satellite.ProviderKind, result.GetKind())
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
		components     []string             // configured allowlist
		contextPaths   phctx.ComponentPaths // incoming request
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
			contextPaths:   phctx.ComponentPaths{{"allowed"}},
			expectedStatus: ph.Status_HEALTHY,
		},
		{
			name:           "InvalidContextComponent",
			components:     []string{"allowed"},
			contextPaths:   phctx.ComponentPaths{{"forbidden"}},
			expectedStatus: ph.Status_UNHEALTHY,
			expectMessage:  "not allowed",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			instance := &satellite.Component{
				Host:       "localhost",
				Port:       port,
				Components: tt.components,
			}
			instance.SetName("test")
			instance.SetTimeout(time.Second)
			require.NoError(t, instance.Setup())

			ctx := t.Context()
			if tt.contextPaths != nil {
				ctx = phctx.ContextWithComponentPaths(ctx, tt.contextPaths)
			}

			result := instance.GetHealth(ctx)
			assert.Equal(t, tt.expectedStatus, result.Status)
			if tt.expectMessage != "" {
				require.NotEmpty(t, result.Messages)
				assert.Contains(t, result.Messages[0], tt.expectMessage)
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
			instance := &satellite.Component{
				Host:       "localhost",
				Port:       port,
				Components: tt.components,
			}
			instance.SetName("test")
			instance.SetTimeout(time.Second)
			require.NoError(t, instance.Setup())

			result := instance.GetHealth(t.Context())
			assert.Equal(t, tt.expectedStatus, result.Status)
		})
	}
}
