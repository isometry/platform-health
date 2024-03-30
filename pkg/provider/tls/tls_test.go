package tls_test

import (
	"context"
	"log/slog"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	ph "github.com/isometry/platform-health/pkg/platform_health"
	"github.com/isometry/platform-health/pkg/provider/tls"
)

func init() {
	slog.SetLogLoggerLevel(slog.LevelError)
}

func TestTLS(t *testing.T) {
	tests := []struct {
		name     string
		host     string
		port     int
		validity time.Duration
		sans     []string
		timeout  time.Duration
		expected ph.Status
	}{
		{
			name:     "Valid target",
			host:     "google.com",
			port:     443,
			validity: time.Hour,
			timeout:  time.Second,
			expected: ph.Status_HEALTHY,
		},
		{
			name:     "Valid target with expiring expiring certificate",
			host:     "google.com",
			port:     443,
			validity: 9552 * time.Hour, // 398 days
			timeout:  time.Second,
			expected: ph.Status_UNHEALTHY,
		},
		{
			name:     "Valid target with good SAN",
			host:     "google.com",
			port:     443,
			validity: time.Hour,
			sans:     []string{"*.google.com"},
			timeout:  time.Second,
			expected: ph.Status_HEALTHY,
		},

		{
			name:     "Valid target with missing SAN",
			host:     "google.com",
			port:     443,
			validity: time.Hour,
			sans:     []string{"example.com"},
			timeout:  time.Second,
			expected: ph.Status_UNHEALTHY,
		},
		{
			name:     "Valid target with timeout",
			host:     "google.com",
			port:     443,
			validity: time.Hour,
			timeout:  time.Nanosecond,
			expected: ph.Status_UNHEALTHY,
		},
		{
			name:     "Invalid target",
			host:     "google.com", // replace with an invalid server for testing
			port:     80,
			validity: time.Hour,
			timeout:  time.Second,
			expected: ph.Status_UNHEALTHY,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), tt.timeout)
			defer cancel()

			instance := &tls.TLS{
				Name:        "TestTLS",
				Host:        tt.host,
				Port:        tt.port,
				MinValidity: tt.validity,
				SANs:        tt.sans,
				Timeout:     tt.timeout,
			}
			instance.SetDefaults()

			result := instance.GetHealth(ctx)

			assert.NotNil(t, result)
			assert.Equal(t, tls.TypeTLS, result.GetType())
			assert.Equal(t, instance.Name, result.GetName())
			assert.Equal(t, tt.expected, result.GetStatus())
		})
	}
}
