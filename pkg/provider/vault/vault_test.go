package vault_test

import (
	"context"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	ph "github.com/isometry/platform-health/pkg/platform_health"
	vaultProvider "github.com/isometry/platform-health/pkg/provider/vault"
)

func init() {
	slog.SetLogLoggerLevel(slog.LevelError)
}

func TestVaultGetHealth(t *testing.T) {
	tests := []struct {
		name     string
		delay    time.Duration
		response string
		timeout  time.Duration
		expected ph.Status
	}{
		{
			name:     "Vault healthy",
			response: `{"initialized":true,"sealed":false,"standby":false}`,
			timeout:  time.Second,
			expected: ph.Status_HEALTHY,
		},
		{
			name:     "Vault uninitialized",
			response: `{"initialized":false,"sealed":true,"standby":false}`,
			timeout:  time.Second,
			expected: ph.Status_UNHEALTHY,
		},
		{
			name:     "Vault sealed",
			response: `{"initialized":true,"sealed":true,"standby":false}`,
			timeout:  time.Second,
			expected: ph.Status_UNHEALTHY,
		},
		{
			name:     "Vault timeout",
			delay:    10 * time.Millisecond,
			response: `{"initialized":true,"sealed":false,"standby":false}`,
			timeout:  time.Millisecond,
			expected: ph.Status_UNHEALTHY,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), tt.timeout+time.Second)
			defer cancel()

			server := httptest.NewServer(
				http.HandlerFunc(
					func(w http.ResponseWriter, r *http.Request) {
						select {
						case <-time.After(tt.delay):
							w.WriteHeader(200)
							w.Header().Set("Content-Type", "application/json")
							_, _ = w.Write([]byte(tt.response))
						case <-ctx.Done():
							return
						}
					}))

			t.Cleanup(func() {
				server.CloseClientConnections()
				server.Close()
			})

			instance := &vaultProvider.Component{
				Name:    "TestService",
				Address: server.URL,
				Timeout: tt.timeout,
			}
			require.NoError(t, instance.Setup())

			result := instance.GetHealth(ctx)

			assert.NotNil(t, result)
			assert.Equal(t, tt.expected, result.GetStatus())
		})
	}
}
