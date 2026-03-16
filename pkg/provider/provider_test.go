package provider_test

import (
	"context"
	"slices"
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
		names := responseNames(responses)
		assert.Contains(t, names, "fast")
		assert.Contains(t, names, "medium")
		assert.Contains(t, names, "slow")
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

// TestCheckDefaultOrderSingleGroup verifies that all instances with default
// order=0 behave identically to the previous single-group implementation.
func TestCheckDefaultOrderSingleGroup(t *testing.T) {
	instances := []provider.Instance{
		mock.Healthy("a"),
		mock.Unhealthy("b"),
		mock.Healthy("c"),
	}

	responses, status := provider.Check(t.Context(), instances)

	assert.Equal(t, ph.Status_UNHEALTHY, status)
	assert.Len(t, responses, 3)
}

// TestCheckSequentialGroupExecution verifies groups run in order.
// Group -10 completes before group 0.
func TestCheckSequentialGroupExecution(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		// Track execution order via timing
		instances := []provider.Instance{
			mock.Healthy("first", mock.WithOrder(-10), mock.WithSleep(10*time.Millisecond)),
			mock.Healthy("second", mock.WithOrder(0), mock.WithSleep(10*time.Millisecond)),
			mock.Healthy("third", mock.WithOrder(10), mock.WithSleep(10*time.Millisecond)),
		}

		responses, status := provider.Check(t.Context(), instances)

		assert.Equal(t, ph.Status_HEALTHY, status)
		require.Len(t, responses, 3)

		// Verify responses arrive in group order (sequential execution)
		assert.Equal(t, "first", responses[0].GetName())
		assert.Equal(t, "second", responses[1].GetName())
		assert.Equal(t, "third", responses[2].GetName())
	})
}

// TestCheckAlwaysSurvivesWithinGroupFailFast verifies that an always instance
// within a group completes even when fail-fast cancels the errgroup context.
func TestCheckAlwaysSurvivesWithinGroupFailFast(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		instances := []provider.Instance{
			// Fast unhealthy triggers fail-fast
			mock.Unhealthy("trigger", mock.WithSleep(10*time.Millisecond)),
			// always=true instance with longer sleep should still complete
			mock.Healthy("monitor", mock.WithAlways(true), mock.WithSleep(100*time.Millisecond)),
			// Non-always slow instance may be cancelled
			mock.Healthy("slow", mock.WithSleep(5*time.Second)),
		}

		ctx := phctx.ContextWithFailFast(t.Context(), true)
		responses, status := provider.Check(ctx, instances)

		assert.Equal(t, ph.Status_UNHEALTHY, status)

		names := responseNames(responses)
		assert.Contains(t, names, "trigger")
		requireResponseStatus(t, responses, "monitor", ph.Status_HEALTHY)
	})
}

// TestCheckAlwaysDoesNotTriggerFailFast verifies that an unhealthy always
// instance does not cancel sibling instances.
func TestCheckAlwaysDoesNotTriggerFailFast(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		instances := []provider.Instance{
			// Unhealthy always instance should NOT trigger fail-fast
			mock.Unhealthy("always-unhealthy", mock.WithAlways(true), mock.WithSleep(10*time.Millisecond)),
			// This healthy instance should complete normally
			mock.Healthy("normal", mock.WithSleep(100*time.Millisecond)),
		}

		ctx := phctx.ContextWithFailFast(t.Context(), true)
		responses, status := provider.Check(ctx, instances)

		assert.Equal(t, ph.Status_UNHEALTHY, status)
		assert.Len(t, responses, 2)
		requireResponseStatus(t, responses, "normal", ph.Status_HEALTHY)
	})
}

// TestCheckCrossGroupFailFastWithAlways verifies that after fail-fast triggers
// in an earlier group, only always instances in later groups execute.
func TestCheckCrossGroupFailFastWithAlways(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		instances := []provider.Instance{
			// Group -10: unhealthy triggers fail-fast
			mock.Unhealthy("infra", mock.WithOrder(-10), mock.WithSleep(10*time.Millisecond)),
			// Group 0: only monitor (always=true) should run
			mock.Healthy("app", mock.WithOrder(0), mock.WithSleep(10*time.Millisecond)),
			mock.Healthy("monitor", mock.WithOrder(0), mock.WithAlways(true), mock.WithSleep(10*time.Millisecond)),
		}

		ctx := phctx.ContextWithFailFast(t.Context(), true)
		responses, status := provider.Check(ctx, instances)

		assert.Equal(t, ph.Status_UNHEALTHY, status)

		names := responseNames(responses)
		assert.Contains(t, names, "infra")
		assert.Contains(t, names, "monitor")
		// "app" should NOT be present — it's non-always in a later group after fail-fast
		assert.NotContains(t, names, "app")
	})
}

// TestCheckMultipleAlwaysAcrossOrderedGroups verifies that always instances
// at different order levels all execute, even after fail-fast.
func TestCheckMultipleAlwaysAcrossOrderedGroups(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		instances := []provider.Instance{
			// Group -10: triggers fail-fast, plus an always
			mock.Unhealthy("fail", mock.WithOrder(-10), mock.WithSleep(10*time.Millisecond)),
			mock.Healthy("dns", mock.WithOrder(-10), mock.WithAlways(true), mock.WithSleep(10*time.Millisecond)),
			// Group 0: only always should run
			mock.Healthy("app", mock.WithOrder(0), mock.WithSleep(10*time.Millisecond)),
			mock.Healthy("monitoring", mock.WithOrder(0), mock.WithAlways(true), mock.WithSleep(10*time.Millisecond)),
			// Group 10: only always should run
			mock.Healthy("cleanup", mock.WithOrder(10), mock.WithSleep(10*time.Millisecond)),
			mock.Healthy("alerting", mock.WithOrder(10), mock.WithAlways(true), mock.WithSleep(10*time.Millisecond)),
		}

		ctx := phctx.ContextWithFailFast(t.Context(), true)
		responses, status := provider.Check(ctx, instances)

		assert.Equal(t, ph.Status_UNHEALTHY, status)

		// All always instances + the failing trigger should be present
		names := responseNames(responses)
		assert.Contains(t, names, "fail")
		assert.Contains(t, names, "dns")
		assert.Contains(t, names, "monitoring")
		assert.Contains(t, names, "alerting")
		// Non-always in later groups should be absent
		assert.NotContains(t, names, "app")
		assert.NotContains(t, names, "cleanup")
	})
}

// helpers

func responseNames(responses []*ph.HealthCheckResponse) []string {
	names := make([]string, len(responses))
	for i, r := range responses {
		names[i] = r.GetName()
	}
	return names
}

func requireResponseStatus(t *testing.T, responses []*ph.HealthCheckResponse, name string, status ph.Status) {
	t.Helper()
	idx := slices.IndexFunc(responses, func(r *ph.HealthCheckResponse) bool {
		return r.GetName() == name
	})
	require.GreaterOrEqual(t, idx, 0, "response %q not found", name)
	assert.Equal(t, status, responses[idx].Status)
}
