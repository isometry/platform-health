package provider_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	ph "github.com/isometry/platform-health/pkg/platform_health"
	"github.com/isometry/platform-health/pkg/provider"
	"github.com/isometry/platform-health/pkg/provider/mock"
)

func TestCheckAll(t *testing.T) {
	tests := []struct {
		name      string
		instances []provider.Instance
		expected  ph.Status
	}{
		{
			name: "All healthy",
			instances: []provider.Instance{
				&mock.Mock{Health: ph.Status_HEALTHY},
				&mock.Mock{Health: ph.Status_HEALTHY},
				&mock.Mock{Health: ph.Status_HEALTHY},
			},
			expected: ph.Status_HEALTHY,
		},
		{
			name: "One unhealthy",
			instances: []provider.Instance{
				&mock.Mock{Health: ph.Status_HEALTHY},
				&mock.Mock{Health: ph.Status_UNHEALTHY},
				&mock.Mock{Health: ph.Status_HEALTHY},
			},
			expected: ph.Status_UNHEALTHY,
		},
		{
			name: "All unhealthy",
			instances: []provider.Instance{
				&mock.Mock{Health: ph.Status_UNHEALTHY},
				&mock.Mock{Health: ph.Status_UNHEALTHY},
				&mock.Mock{Health: ph.Status_UNHEALTHY},
			},
			expected: ph.Status_UNHEALTHY,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, actual := provider.Check(context.Background(), tt.instances)
			assert.Equal(t, tt.expected, actual)
		})
	}
}

func TestServiceWithDuration(t *testing.T) {
	instance := &mock.Mock{
		Name:   "test",
		Health: ph.Status_HEALTHY,
		Sleep:  10 * time.Millisecond,
	}

	result := provider.GetHealthWithDuration(context.Background(), instance)

	assert.Equal(t, instance.GetType(), result.GetType())
	assert.Equal(t, instance.GetName(), result.GetName())
	assert.Equal(t, instance.Health, result.GetStatus())
	assert.NotZero(t, result.GetDuration())
}
