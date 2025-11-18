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
	defer func() { _ = listener.Close() }()

	port := listener.Addr().(*net.TCPAddr).Port

	tests := []struct {
		name     string
		port     int
		closed   bool
		timeout  time.Duration
		expected ph.Status
	}{
		{
			name:     "Port open",
			port:     port,
			expected: ph.Status_HEALTHY,
		},
		{
			name:     "Port closed, wanted open",
			port:     1,
			expected: ph.Status_UNHEALTHY,
		},
		{
			name:     "Port closed, wanted closed",
			port:     1,
			closed:   true,
			expected: ph.Status_HEALTHY,
		},
		{
			name:     "Unexpected timeout",
			port:     port,
			timeout:  time.Nanosecond,
			expected: ph.Status_UNHEALTHY,
		},
		{
			name:     "Expected timeout",
			port:     port,
			closed:   true,
			timeout:  time.Nanosecond,
			expected: ph.Status_HEALTHY,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			instance := &tcp.TCP{
				Name:    tt.name,
				Host:    "localhost",
				Port:    tt.port,
				Closed:  tt.closed,
				Timeout: tt.timeout,
			}
			instance.SetDefaults()

			result := instance.GetHealth(context.Background())

			assert.NotNil(t, result)
			assert.Equal(t, tcp.TypeTCP, result.GetType())
			assert.Equal(t, tt.name, result.GetName())
			assert.Equal(t, tt.expected, result.GetStatus())
		})
	}
}
