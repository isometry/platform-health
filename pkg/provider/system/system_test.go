package system_test

import (
	"log/slog"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"testing/synctest"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.yaml.in/yaml/v3"

	"github.com/isometry/platform-health/pkg/phctx"
	ph "github.com/isometry/platform-health/pkg/platform_health"
	"github.com/isometry/platform-health/pkg/provider/mock"
	"github.com/isometry/platform-health/pkg/provider/system"
)

// getTestdataPath returns the path to the testdata directory
func getTestdataPath(t *testing.T) string {
	t.Helper()
	_, filename, _, ok := runtime.Caller(0)
	require.True(t, ok, "failed to get caller info")
	return filepath.Join(filepath.Dir(filename), "testdata")
}

// loadComponentsFixture loads a components YAML fixture
func loadComponentsFixture(t *testing.T, filename string) map[string]any {
	t.Helper()
	path := filepath.Join(getTestdataPath(t), filename)
	data, err := os.ReadFile(path)
	require.NoError(t, err, "failed to read fixture %s", filename)

	var result map[string]any
	require.NoError(t, yaml.Unmarshal(data, &result), "failed to unmarshal fixture")
	return result
}

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
					"type": "mock",
					"spec": map[string]any{
						"health": ph.Status_HEALTHY,
					},
				},
			},
			expected:   ph.Status_HEALTHY,
			childCount: 1,
		},
		{
			name: "SingleUnhealthyComponent",
			components: map[string]any{
				"test": map[string]any{
					"type": "mock",
					"spec": map[string]any{
						"health": ph.Status_UNHEALTHY,
					},
				},
			},
			expected:   ph.Status_UNHEALTHY,
			childCount: 1,
		},
		{
			name: "MixedHealthComponents",
			components: map[string]any{
				"healthy": map[string]any{
					"type": "mock",
					"spec": map[string]any{
						"health": ph.Status_HEALTHY,
					},
				},
				"unhealthy": map[string]any{
					"type": "mock",
					"spec": map[string]any{
						"health": ph.Status_UNHEALTHY,
					},
				},
			},
			expected:   ph.Status_UNHEALTHY,
			childCount: 2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := &system.Component{}
			s.SetName("TestSystem")
			s.SetComponents(tt.components)
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

func TestSystemUnknownProviderReturnsError(t *testing.T) {
	s := &system.Component{}
	s.SetName("TestSystem")
	s.SetComponents(map[string]any{
		"test": map[string]any{
			"type": "nonexistent",
			"spec": map[string]any{},
		},
	})

	// Setup collects errors in ComponentErrors (doesn't return error)
	err := s.Setup()
	require.NoError(t, err)

	// ComponentErrors should contain the error for unknown provider
	require.Len(t, s.ComponentErrors(), 1)
	assert.Contains(t, s.ComponentErrors()[0].Error(), "nonexistent")
	assert.Contains(t, s.ComponentErrors()[0].Error(), "unknown provider type")

	// GetHealth should still work but return HEALTHY with no children
	result := s.GetHealth(t.Context())
	assert.NotNil(t, result)
	assert.Equal(t, ph.Status_HEALTHY, result.GetStatus())
	assert.Equal(t, 0, len(result.GetComponents()))
}

func TestSystemChildName(t *testing.T) {
	s := &system.Component{}
	s.SetName("ParentSystem")
	s.SetComponents(map[string]any{
		"child": map[string]any{
			"type": "mock",
			"name": "child",
		},
	})
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
	s := &system.Component{}
	s.SetName("OuterSystem")
	s.SetComponents(map[string]any{
		"inner": map[string]any{
			"type": "system",
			"name": "inner",
			"components": map[string]any{
				"leaf": map[string]any{
					"type": "mock",
					"name": "leaf",
					"spec": map[string]any{
						"health": ph.Status_HEALTHY,
					},
				},
			},
		},
	})
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
	s := &system.Component{}
	s.SetName("Test")

	assert.Equal(t, system.ProviderType, s.GetType())
	assert.Equal(t, "Test", s.GetName())
}

func TestSystemParallelismOneNoDeadlock(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		s := &system.Component{}
		s.SetName("test-system")
		s.SetComponents(map[string]any{
			"child1": map[string]any{
				"type": "mock",
				"spec": map[string]any{
					"health": ph.Status_HEALTHY,
					"sleep":  10 * time.Millisecond,
				},
			},
			"child2": map[string]any{
				"type": "mock",
				"spec": map[string]any{
					"health": ph.Status_HEALTHY,
					"sleep":  10 * time.Millisecond,
				},
			},
			"child3": map[string]any{
				"type": "mock",
				"spec": map[string]any{
					"health": ph.Status_UNHEALTHY,
					"sleep":  10 * time.Millisecond,
				},
			},
		})
		require.NoError(t, s.Setup())

		ctx := phctx.ContextWithParallelism(t.Context(), 1)
		result := s.GetHealth(ctx)

		assert.Equal(t, ph.Status_UNHEALTHY, result.Status)
		assert.Len(t, result.Components, 3)
	})
}

// Fixture-based tests

func TestSystemGetHealthFixtures(t *testing.T) {
	tests := []struct {
		name       string
		fixture    string
		expected   ph.Status
		childCount int
	}{
		{
			name:       "EmptySystemFixture",
			fixture:    "components-empty.yaml",
			expected:   ph.Status_HEALTHY,
			childCount: 0,
		},
		{
			name:       "SingleHealthyFixture",
			fixture:    "components-single-healthy.yaml",
			expected:   ph.Status_HEALTHY,
			childCount: 1,
		},
		{
			name:       "SingleUnhealthyFixture",
			fixture:    "components-single-unhealthy.yaml",
			expected:   ph.Status_UNHEALTHY,
			childCount: 1,
		},
		{
			name:       "MixedHealthFixture",
			fixture:    "components-mixed.yaml",
			expected:   ph.Status_UNHEALTHY,
			childCount: 2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := &system.Component{}
			s.SetName("TestSystem")
			s.SetComponents(loadComponentsFixture(t, tt.fixture))
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

func TestSystemNestedSystemsFixture(t *testing.T) {
	s := &system.Component{}
	s.SetName("OuterSystem")
	s.SetComponents(loadComponentsFixture(t, "components-nested.yaml"))
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
