package http_test

import (
	"context"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	ph "github.com/isometry/platform-health/pkg/platform_health"
	httpService "github.com/isometry/platform-health/pkg/provider/http"
)

func init() {
	slog.SetLogLoggerLevel(slog.LevelError)
}

func TestLocalHTTP(t *testing.T) {
	tests := []struct {
		name         string
		method       string
		status       int
		timeout      time.Duration
		serverDelay  time.Duration
		serverStatus int
		expected     ph.Status
	}{
		{
			name:         "HTTP server GET",
			method:       "GET",
			status:       http.StatusOK,
			timeout:      time.Second,
			serverDelay:  0,
			serverStatus: http.StatusOK,
			expected:     ph.Status_HEALTHY,
		},
		{
			name:         "HTTP server HEAD",
			method:       "HEAD",
			status:       http.StatusOK,
			timeout:      time.Second,
			serverDelay:  0,
			serverStatus: http.StatusOK,
			expected:     ph.Status_HEALTHY,
		},
		{
			name:         "HTTP server unexpected status",
			method:       "GET",
			status:       http.StatusOK,
			timeout:      time.Second,
			serverDelay:  0,
			serverStatus: http.StatusNotFound,
			expected:     ph.Status_UNHEALTHY,
		},
		{
			name:         "HTTP server timeout",
			method:       "GET",
			status:       http.StatusOK,
			timeout:      time.Microsecond * 10,
			serverDelay:  time.Microsecond * 20,
			serverStatus: http.StatusOK,
			expected:     ph.Status_UNHEALTHY,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), tt.timeout)
			defer cancel()

			server := httptest.NewServer(
				http.HandlerFunc(
					func(w http.ResponseWriter, r *http.Request) {
						select {
						case <-time.After(tt.serverDelay):
							w.WriteHeader(tt.serverStatus)
							w.Write([]byte("IGNORED"))
						case <-ctx.Done():
							return
						}
					}))
			defer server.CloseClientConnections()
			defer server.Close()

			instance := &httpService.HTTP{
				Name:    "TestService",
				URL:     server.URL,
				Method:  tt.method,
				Status:  []int{tt.status},
				Timeout: tt.timeout,
			}
			instance.SetDefaults()

			result := instance.GetHealth(ctx)

			assert.NotNil(t, result)
			assert.Equal(t, tt.expected, result.GetStatus())
		})
	}
}

func TestRemoteHTTP(t *testing.T) {
	tests := []struct {
		name     string
		url      string
		method   string
		status   []int
		timeout  time.Duration
		expected ph.Status
	}{
		{
			name:     "HTTP server GET",
			url:      "https://www.google.com",
			method:   "GET",
			status:   []int{http.StatusOK},
			timeout:  time.Second,
			expected: ph.Status_HEALTHY,
		},
		{
			name:     "HTTP server HEAD",
			url:      "https://www.google.com",
			method:   "HEAD",
			status:   []int{http.StatusOK},
			timeout:  time.Second,
			expected: ph.Status_HEALTHY,
		},
		{
			name:     "HTTP server unexpected status",
			url:      "https://www.google.com",
			method:   "GET",
			status:   []int{http.StatusNotFound},
			timeout:  time.Second,
			expected: ph.Status_UNHEALTHY,
		},
		{
			name:     "HTTP server timeout",
			url:      "https://www.google.com",
			method:   "GET",
			status:   []int{http.StatusOK},
			timeout:  time.Nanosecond,
			expected: ph.Status_UNHEALTHY,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), tt.timeout)
			defer cancel()

			instance := &httpService.HTTP{
				Name:    "TestHTTP",
				URL:     tt.url,
				Method:  tt.method,
				Status:  tt.status,
				Timeout: tt.timeout,
			}

			result := instance.GetHealth(ctx)

			assert.NotNil(t, result)
			assert.Equal(t, tt.expected, result.GetStatus())
		})
	}
}
