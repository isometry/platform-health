package provider_test

import (
	"context"
	"testing"
	"testing/synctest"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/isometry/platform-health/pkg/phctx"
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
				mock.Healthy("a"),
				mock.Healthy("b"),
			},
			expected: ph.Status_HEALTHY,
		},
		{
			name: "OneUnhealthy",
			instances: []provider.Instance{
				mock.Unhealthy("a"),
				mock.Healthy("b"),
			},
			expected: ph.Status_UNHEALTHY,
		},
		{
			name: "AllUnhealthy",
			instances: []provider.Instance{
				mock.Unhealthy("a"),
				mock.Unhealthy("b"),
			},
			expected: ph.Status_UNHEALTHY,
		},
		{
			name: "LoopFirstPriority",
			instances: []provider.Instance{
				mock.New("a", mock.WithHealth(ph.Status_LOOP_DETECTED)),
				mock.Unhealthy("b"),
				mock.Healthy("c"),
			},
			expected: ph.Status_LOOP_DETECTED,
		},
		{
			name: "LoopLastPriority",
			instances: []provider.Instance{
				mock.Healthy("a"),
				mock.Unhealthy("b"),
				mock.New("c", mock.WithHealth(ph.Status_LOOP_DETECTED)),
			},
			expected: ph.Status_LOOP_DETECTED,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, actual := provider.Check(t.Context(), tt.instances)
			assert.Equal(t, tt.expected, actual)
		})
	}
}

func TestServiceWithDuration(t *testing.T) {
	instance := mock.Healthy("test", mock.WithSleep(10*time.Millisecond))

	result := provider.GetHealthWithDuration(t.Context(), instance)

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
			mock.Healthy("fast", mock.WithSleep(100*time.Millisecond)),
			mock.Unhealthy("medium", mock.WithSleep(500*time.Millisecond)),
			mock.Healthy("slow", mock.WithSleep(time.Second)),
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
			mock.Healthy("slow", mock.WithSleep(5*time.Minute)),
		}

		responses, status := provider.Check(ctx, instances)

		assert.Equal(t, ph.Status_UNHEALTHY, status)
		assert.Len(t, responses, 1)
		require.NotEmpty(t, responses[0].Messages)
		assert.Contains(t, responses[0].Messages[0], "deadline exceeded")
	})
}

// TestCheckParallelismOne verifies that parallelism=1 doesn't cause deadlock
// when checking nested providers (e.g., system provider with children).
func TestCheckParallelismOne(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		// Create a "system-like" provider that internally checks multiple children
		// by simulating what the system provider does
		instances := []provider.Instance{
			mock.Healthy("child1", mock.WithSleep(10*time.Millisecond)),
			mock.Healthy("child2", mock.WithSleep(10*time.Millisecond)),
			mock.Unhealthy("child3", mock.WithSleep(10*time.Millisecond)),
		}

		// Set parallelism to 1 - should still complete without deadlock
		ctx := phctx.ContextWithParallelism(t.Context(), 1)

		responses, status := provider.Check(ctx, instances)

		assert.Equal(t, ph.Status_UNHEALTHY, status)
		assert.Len(t, responses, 3)
	})
}

// TestCheckParallelismZero verifies that parallelism=0 uses GOMAXPROCS
func TestCheckParallelismZero(t *testing.T) {
	instances := []provider.Instance{
		mock.Healthy("test1"),
		mock.Healthy("test2"),
	}

	ctx := phctx.ContextWithParallelism(t.Context(), 0)
	responses, status := provider.Check(ctx, instances)

	assert.Equal(t, ph.Status_HEALTHY, status)
	assert.Len(t, responses, 2)
}

// TestCheckParallelismUnlimited verifies that parallelism=-1 (unlimited) works
func TestCheckParallelismUnlimited(t *testing.T) {
	instances := []provider.Instance{
		mock.Healthy("test1"),
		mock.Healthy("test2"),
		mock.Healthy("test3"),
	}

	ctx := phctx.ContextWithParallelism(t.Context(), -1)
	responses, status := provider.Check(ctx, instances)

	assert.Equal(t, ph.Status_HEALTHY, status)
	assert.Len(t, responses, 3)
}
