package mock_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	ph "github.com/isometry/platform-health/pkg/platform_health"
	"github.com/isometry/platform-health/pkg/provider/mock"
)

func TestMock(t *testing.T) {
	tests := []struct {
		name   string
		mock   *mock.Mock
		expect ph.Status
	}{
		{
			name: "HealthyComponent",
			mock: &mock.Mock{
				Name: "TestComponent",
			},
			expect: ph.Status_HEALTHY,
		},
		{
			name: "HealthyComponentWithSleep",
			mock: &mock.Mock{
				Name:  "TestComponent",
				Sleep: 50 * time.Millisecond,
			},
			expect: ph.Status_HEALTHY,
		},

		{
			name: "UnhealthyComponent",
			mock: &mock.Mock{
				Name:   "TestComponent",
				Health: ph.Status_UNHEALTHY,
			},
			expect: ph.Status_UNHEALTHY,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.mock.SetDefaults()

			start := time.Now()
			result := tt.mock.GetHealth(context.Background())
			duration := time.Since(start)

			assert.Equal(t, tt.expect, result.GetStatus())
			assert.Greater(t, duration, tt.mock.Sleep)
		})
	}
}
