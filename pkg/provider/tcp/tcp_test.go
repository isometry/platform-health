package tcp_test

import (
	"context"
	"log/slog"
	"net"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	ph "github.com/isometry/platform-health/pkg/platform_health"
	"github.com/isometry/platform-health/pkg/provider/tcp"
)

func init() {
	slog.SetLogLoggerLevel(slog.LevelError)
}

func TestTCP(t *testing.T) {
	// Set up a test server
	listener, err := net.Listen("tcp", "localhost:0")
	if err != nil {
		t.Fatalf("Failed to set up test server: %v", err)
	}
	defer listener.Close()

	tests := []struct {
		name   string
		port   int
		status ph.Status
	}{
		{
			name:   "Port open",
			port:   listener.Addr().(*net.TCPAddr).Port,
			status: ph.Status_HEALTHY,
		},
		{
			name:   "Port closed",
			port:   1,
			status: ph.Status_UNHEALTHY,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			instance := &tcp.TCP{
				Name:    "TestTCP",
				Host:    "localhost",
				Port:    tt.port,
				Timeout: time.Second,
			}
			instance.SetDefaults()

			result := instance.GetHealth(context.Background())

			assert.NotNil(t, result)
			assert.Equal(t, tcp.TypeTCP, result.GetType())
			assert.Equal(t, instance.Name, result.GetName())
			assert.Equal(t, tt.status, result.GetStatus())
		})
	}
}
