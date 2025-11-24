package provider_test

import (
	"context"
	"testing"
	"testing/synctest"
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
			name: "AllHealthy",
			instances: []provider.Instance{
				&mock.Mock{Health: ph.Status_HEALTHY},
				&mock.Mock{Health: ph.Status_HEALTHY},
			},
			expected: ph.Status_HEALTHY,
		},
		{
			name: "OneUnhealthy",
			instances: []provider.Instance{
				&mock.Mock{Health: ph.Status_UNHEALTHY},
				&mock.Mock{Health: ph.Status_HEALTHY},
			},
			expected: ph.Status_UNHEALTHY,
		},
		{
			name: "AllUnhealthy",
			instances: []provider.Instance{
				&mock.Mock{Health: ph.Status_UNHEALTHY},
				&mock.Mock{Health: ph.Status_UNHEALTHY},
			},
			expected: ph.Status_UNHEALTHY,
		},
		{
			name: "LoopFirstPriority",
			instances: []provider.Instance{
				&mock.Mock{Health: ph.Status_LOOP_DETECTED},
				&mock.Mock{Health: ph.Status_UNHEALTHY},
				&mock.Mock{Health: ph.Status_HEALTHY},
			},
			expected: ph.Status_LOOP_DETECTED,
		},
		{
			name: "LoopLastPriority",
			instances: []provider.Instance{
				&mock.Mock{Health: ph.Status_HEALTHY},
				&mock.Mock{Health: ph.Status_UNHEALTHY},
				&mock.Mock{Health: ph.Status_LOOP_DETECTED},
			},
			expected: ph.Status_LOOP_DETECTED,
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

// TestCheckVaryingDelays uses synctest to verify status aggregation
// when providers complete at different times. With synctest, the delays
// execute instantly while still testing the concurrent aggregation logic.
func TestCheckVaryingDelays(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		instances := []provider.Instance{
			&mock.Mock{Name: "fast", Health: ph.Status_HEALTHY, Sleep: 100 * time.Millisecond},
			&mock.Mock{Name: "medium", Health: ph.Status_UNHEALTHY, Sleep: 500 * time.Millisecond},
			&mock.Mock{Name: "slow", Health: ph.Status_HEALTHY, Sleep: time.Second},
		}

		responses, status := provider.Check(t.Context(), instances)

		// Status should be UNHEALTHY (highest severity)
		assert.Equal(t, ph.Status_UNHEALTHY, status)
		assert.Len(t, responses, 3)

		// Verify all responses are present
		names := make(map[string]bool)
		for _, r := range responses {
			names[r.GetName()] = true
		}
		assert.True(t, names["fast"])
		assert.True(t, names["medium"])
		assert.True(t, names["slow"])
	})
}

// TestCheckTimeout verifies that providers correctly return UNHEALTHY
// when context times out. With synctest, we can test a 5-minute timeout
// scenario instantly.
func TestCheckTimeout(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		ctx, cancel := context.WithTimeout(t.Context(), time.Second)
		defer cancel()

		instances := []provider.Instance{
			&mock.Mock{Name: "slow", Health: ph.Status_HEALTHY, Sleep: 5 * time.Minute},
		}

		responses, status := provider.Check(ctx, instances)

		assert.Equal(t, ph.Status_UNHEALTHY, status)
		assert.Len(t, responses, 1)
		assert.Contains(t, responses[0].Message, "deadline exceeded")
	})
}
