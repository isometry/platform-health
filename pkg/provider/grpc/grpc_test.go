package grpc_test

import (
	"context"
	"log/slog"
	"net"
	"testing"

	"github.com/stretchr/testify/assert"
	"google.golang.org/grpc"
	"google.golang.org/grpc/health"
	"google.golang.org/grpc/health/grpc_health_v1"

	ph "github.com/isometry/platform-health/pkg/platform_health"
	provider_grpc "github.com/isometry/platform-health/pkg/provider/grpc"
)

func init() {
	slog.SetLogLoggerLevel(slog.LevelError)
}

func TestGetHealth(t *testing.T) {
	listener, err := net.Listen("tcp", "localhost:0")
	if err != nil {
		t.Fatalf("Failed to set up test server: %v", err)
	}
	listenPort := listener.Addr().(*net.TCPAddr).Port
	defer listener.Close()

	// Start a gRPC server that implements the Health service
	server := grpc.NewServer()
	healthServer := health.NewServer()
	grpc_health_v1.RegisterHealthServer(server, healthServer)

	go server.Serve(listener)
	defer server.Stop()

	tests := []struct {
		name     string
		grpc     *provider_grpc.GRPC
		status   grpc_health_v1.HealthCheckResponse_ServingStatus
		expected ph.Status
	}{
		{
			name: "HealthyService",
			grpc: &provider_grpc.GRPC{
				Name:    "test",
				Host:    "localhost",
				Port:    listenPort,
				Service: "",
			},
			status:   grpc_health_v1.HealthCheckResponse_SERVING,
			expected: ph.Status_HEALTHY,
		},
		{
			name: "UnhealthyService",
			grpc: &provider_grpc.GRPC{
				Name:    "test",
				Host:    "localhost",
				Port:    listenPort,
				Service: "",
			},
			status:   grpc_health_v1.HealthCheckResponse_NOT_SERVING,
			expected: ph.Status_UNHEALTHY,
		},
		{
			name: "UnknownService",
			grpc: &provider_grpc.GRPC{
				Name:    "test",
				Host:    "localhost",
				Port:    listenPort,
				Service: "unknown",
			},
			status:   grpc_health_v1.HealthCheckResponse_UNKNOWN,
			expected: ph.Status_UNHEALTHY,
		},
		{
			name: "InvalidTarget",
			grpc: &provider_grpc.GRPC{
				Name:    "test",
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
			healthServer.SetServingStatus(tt.grpc.Service, tt.status)
			tt.grpc.SetDefaults()
			service := tt.grpc.GetHealth(context.Background())
			assert.Equal(t, tt.expected, service.Status)
		})
	}
}
