package grpc_test

import (
	"log/slog"
	"net"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/health"
	"google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/resolver"

	"github.com/isometry/platform-health/pkg/client"
	ph "github.com/isometry/platform-health/pkg/platform_health"
	"github.com/isometry/platform-health/pkg/provider"
	grpcProvider "github.com/isometry/platform-health/pkg/provider/grpc"
)

func init() {
	slog.SetLogLoggerLevel(slog.LevelError)
}

func TestGetHealth(t *testing.T) {
	// workaround for grpc resolver with Zscaler
	resolver.SetDefaultScheme("passthrough")

	listener, err := net.Listen("tcp", "localhost:0")
	if err != nil {
		t.Fatalf("Failed to set up test server: %v", err)
	}
	listenPort := listener.Addr().(*net.TCPAddr).Port

	// Start a gRPC server that implements the Health service
	server := grpc.NewServer()
	healthServer := health.NewServer()
	grpc_health_v1.RegisterHealthServer(server, healthServer)

	go func() { _ = server.Serve(listener) }()

	t.Cleanup(func() {
		server.Stop()
		_ = listener.Close()
	})

	tests := []struct {
		name     string
		grpc     *grpcProvider.Component
		status   grpc_health_v1.HealthCheckResponse_ServingStatus
		expected ph.Status
	}{
		{
			name: "HealthyService",
			grpc: &grpcProvider.Component{
				Host:    "localhost",
				Port:    listenPort,
				Service: "",
			},
			status:   grpc_health_v1.HealthCheckResponse_SERVING,
			expected: ph.Status_HEALTHY,
		},
		{
			name: "UnhealthyService",
			grpc: &grpcProvider.Component{
				Host:    "localhost",
				Port:    listenPort,
				Service: "",
			},
			status:   grpc_health_v1.HealthCheckResponse_NOT_SERVING,
			expected: ph.Status_UNHEALTHY,
		},
		{
			name: "UnknownService",
			grpc: &grpcProvider.Component{
				Host:    "localhost",
				Port:    listenPort,
				Service: "unknown",
			},
			status:   grpc_health_v1.HealthCheckResponse_UNKNOWN,
			expected: ph.Status_UNHEALTHY,
		},
		{
			name: "InvalidTarget",
			grpc: &grpcProvider.Component{
				Host:    "localhost",
				Port:    1,
				Service: "",
			},
			status:   grpc_health_v1.HealthCheckResponse_UNKNOWN,
			expected: ph.Status_UNHEALTHY,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.grpc.SetName("test")
			healthServer.SetServingStatus(tt.grpc.Service, tt.status)
			require.NoError(t, tt.grpc.Setup())
			service := tt.grpc.GetHealth(t.Context())
			assert.Equal(t, tt.expected, service.Status)
		})
	}
}

// TestTLSSpecDecodesTriState checks that an omitted tls key stays unset while
// an explicit false is preserved, since only an explicit false can force
// plaintext on a port that otherwise implies TLS.
func TestTLSSpecDecodesTriState(t *testing.T) {
	unset, err := provider.NewInstance("grpc", provider.WithSpec(map[string]any{"host": "h"}))
	require.NoError(t, err)
	assert.Nil(t, unset.(*grpcProvider.Component).TLS)

	off, err := provider.NewInstance("grpc", provider.WithSpec(map[string]any{"host": "h", "tls": false}))
	require.NoError(t, err)
	require.NotNil(t, off.(*grpcProvider.Component).TLS)
	assert.False(t, *off.(*grpcProvider.Component).TLS)
}

// TestTLSOverrideOnImpliedPort runs a plaintext health server on a port the
// dial helper treats as TLS-implied, so only an explicit tls: false can reach it.
func TestTLSOverrideOnImpliedPort(t *testing.T) {
	resolver.SetDefaultScheme("passthrough")

	listener, err := net.Listen("tcp", "localhost:0")
	require.NoError(t, err)
	listenPort := listener.Addr().(*net.TCPAddr).Port

	server := grpc.NewServer()
	grpc_health_v1.RegisterHealthServer(server, health.NewServer())
	go func() { _ = server.Serve(listener) }()
	t.Cleanup(func() {
		server.Stop()
		_ = listener.Close()
	})

	saved := client.TLSPorts
	client.TLSPorts = []int{listenPort}
	t.Cleanup(func() { client.TLSPorts = saved })

	off := false
	tests := []struct {
		name     string
		tls      *bool
		expected ph.Status
	}{
		{"unset implies tls and fails against plaintext", nil, ph.Status_UNHEALTHY},
		{"explicit false forces plaintext", &off, ph.Status_HEALTHY},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := &grpcProvider.Component{Host: "localhost", Port: listenPort, TLS: tt.tls}
			c.SetName("test")
			require.NoError(t, c.Setup())
			assert.Equal(t, tt.expected, c.GetHealth(t.Context()).Status)
		})
	}
}
