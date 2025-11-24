package system_test

import (
	"log/slog"
	"testing"
	"testing/synctest"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/isometry/platform-health/pkg/phctx"
	ph "github.com/isometry/platform-health/pkg/platform_health"
	"github.com/isometry/platform-health/pkg/provider/mock"
	"github.com/isometry/platform-health/pkg/provider/system"
)

func init() {
	slog.SetLogLoggerLevel(slog.LevelError)
}

// findComponent finds a component by name in the slice
func findComponent(components []*ph.HealthCheckResponse, name string) *ph.HealthCheckResponse {
	for _, c := range components {
		if c.GetName() == name {
			return c
		}
	}
	return nil
}

func TestSystemGetHealth(t *testing.T) {
	tests := []struct {
		name       string
		components map[string]any
		expected   ph.Status
		childCount int
	}{
		{
			name:       "EmptySystem",
			components: map[string]any{},
			expected:   ph.Status_HEALTHY,
			childCount: 0,
		},
		{
			name: "SingleHealthyComponent",
			components: map[string]any{
				"test": map[string]any{
					"type":   "mock",
					"name":   "test",
					"health": ph.Status_HEALTHY,
				},
			},
			expected:   ph.Status_HEALTHY,
			childCount: 1,
		},
		{
			name: "SingleUnhealthyComponent",
			components: map[string]any{
				"test": map[string]any{
					"type":   "mock",
					"name":   "test",
					"health": ph.Status_UNHEALTHY,
				},
			},
			expected:   ph.Status_UNHEALTHY,
			childCount: 1,
		},
		{
			name: "MixedHealthComponents",
			components: map[string]any{
				"healthy": map[string]any{
					"type":   "mock",
					"name":   "healthy",
					"health": ph.Status_HEALTHY,
				},
				"unhealthy": map[string]any{
					"type":   "mock",
					"name":   "unhealthy",
					"health": ph.Status_UNHEALTHY,
				},
			},
			expected:   ph.Status_UNHEALTHY,
			childCount: 2,
		},
		{
			name: "UnknownProviderIgnored",
			components: map[string]any{
				"test": map[string]any{
					"type": "nonexistent",
					"name": "test",
				},
			},
			expected:   ph.Status_HEALTHY,
			childCount: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := &system.Component{
				Name:       "TestSystem",
				Components: tt.components,
			}
			s.SetName("TestSystem")
			require.NoError(t, s.Setup())

			result := s.GetHealth(t.Context())

			assert.NotNil(t, result)
			assert.Equal(t, system.ProviderType, result.GetType())
			assert.Equal(t, "TestSystem", result.GetName())
			assert.Equal(t, tt.expected, result.GetStatus())
			assert.Equal(t, tt.childCount, len(result.GetComponents()))
		})
	}
}

func TestSystemChildName(t *testing.T) {
	s := &system.Component{
		Name: "ParentSystem",
		Components: map[string]any{
			"child": map[string]any{
				"type": "mock",
				"name": "child",
			},
		},
	}
	s.SetName("ParentSystem")
	require.NoError(t, s.Setup())

	result := s.GetHealth(t.Context())
	require.Equal(t, 1, len(result.GetComponents()))

	// Check that child has correct name
	child := findComponent(result.GetComponents(), "child")
	require.NotNil(t, child)
	assert.Equal(t, "child", child.GetName())
}

func TestSystemNestedSystems(t *testing.T) {
	// Test nested system structure
	s := &system.Component{
		Name: "OuterSystem",
		Components: map[string]any{
			"inner": map[string]any{
				"type": "system",
				"name": "inner",
				"components": map[string]any{
					"leaf": map[string]any{
						"type":   "mock",
						"name":   "leaf",
						"health": ph.Status_HEALTHY,
					},
				},
			},
		},
	}
	s.SetName("OuterSystem")
	require.NoError(t, s.Setup())

	result := s.GetHealth(t.Context())

	assert.NotNil(t, result)
	assert.Equal(t, ph.Status_HEALTHY, result.GetStatus())
	assert.Equal(t, 1, len(result.GetComponents()))

	// Check inner system
	inner := findComponent(result.GetComponents(), "inner")
	require.NotNil(t, inner)
	assert.Equal(t, system.ProviderType, inner.GetType())
	assert.Equal(t, "inner", inner.GetName())
	assert.Equal(t, 1, len(inner.GetComponents()))

	// Check leaf component
	leaf := findComponent(inner.GetComponents(), "leaf")
	require.NotNil(t, leaf)
	assert.Equal(t, mock.ProviderType, leaf.GetType())
	assert.Equal(t, "leaf", leaf.GetName())
}

func TestSystemInterface(t *testing.T) {
	s := &system.Component{
		Name: "Test",
	}
	s.SetName("Test")

	assert.Equal(t, system.ProviderType, s.GetType())
	assert.Equal(t, "Test", s.GetName())
}

func TestSystemParallelismOneNoDeadlock(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		s := &system.Component{
			Name: "test-system",
			Components: map[string]any{
				"child1": map[string]any{
					"type":   "mock",
					"health": ph.Status_HEALTHY,
					"sleep":  10 * time.Millisecond,
				},
				"child2": map[string]any{
					"type":   "mock",
					"health": ph.Status_HEALTHY,
					"sleep":  10 * time.Millisecond,
				},
				"child3": map[string]any{
					"type":   "mock",
					"health": ph.Status_UNHEALTHY,
					"sleep":  10 * time.Millisecond,
				},
			},
		}
		s.SetName("test-system")
		require.NoError(t, s.Setup())

		ctx := phctx.ContextWithParallelism(t.Context(), 1)
		result := s.GetHealth(ctx)

		assert.Equal(t, ph.Status_UNHEALTHY, result.Status)
		assert.Len(t, result.Components, 3)
	})
}
