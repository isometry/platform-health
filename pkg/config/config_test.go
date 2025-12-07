package config

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/isometry/platform-health/pkg/phctx"
	"github.com/isometry/platform-health/pkg/provider"
	"github.com/isometry/platform-health/pkg/provider/mock"
)

func init() {
	log = phctx.Logger(context.Background())
}

// testContext creates a context with a viper instance for testing
func testContext(t *testing.T) context.Context {
	v := phctx.NewViper()
	return phctx.ContextWithViper(t.Context(), v)
}

// findInstanceByName finds an instance by name in a slice
func findInstanceByName(instances []provider.Instance, name string) provider.Instance {
	for _, inst := range instances {
		if inst.GetName() == name {
			return inst
		}
	}
	return nil
}

func TestGetInstances(t *testing.T) {
	tests := []struct {
		name     string
		result   *LoadResult
		expected int // expected number of instances
	}{
		{
			name:     "EmptyConfig",
			result:   &LoadResult{config: make(concreteConfig)},
			expected: 0,
		},
		{
			name: "SingleProvider",
			result: &LoadResult{
				config: concreteConfig{
					"mock": {
						&mock.Component{InstanceName: "comp1"},
						&mock.Component{InstanceName: "comp2"},
					},
				},
			},
			expected: 2,
		},
		{
			name: "MultipleProviders",
			result: &LoadResult{
				config: concreteConfig{
					"mock": {
						&mock.Component{InstanceName: "a"},
						&mock.Component{InstanceName: "b"},
					},
					"other": {
						&mock.Component{InstanceName: "c"},
					},
				},
			},
			expected: 3,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			instances := tt.result.GetInstances()
			assert.Equal(t, tt.expected, len(instances))
		})
	}
}

func TestHarden(t *testing.T) {
	tests := []struct {
		name              string
		abstract          abstractConfig
		expectedProviders int
		expectedTotal     int
	}{
		{
			name:              "Empty Config",
			abstract:          abstractConfig{},
			expectedProviders: 0,
			expectedTotal:     0,
		},
		{
			name: "Single Provider Multiple Instances",
			abstract: abstractConfig{
				"comp1": map[string]any{"type": "mock", "spec": map[string]any{}},
				"comp2": map[string]any{"type": "mock", "spec": map[string]any{}},
			},
			expectedProviders: 1,
			expectedTotal:     2,
		},
		{
			name: "Multiple Providers",
			abstract: abstractConfig{
				"a": map[string]any{"type": "mock", "spec": map[string]any{}},
				"b": map[string]any{"type": "mock", "spec": map[string]any{}},
			},
			expectedProviders: 1,
			expectedTotal:     2,
		},
		{
			name: "Unknown Provider",
			abstract: abstractConfig{
				"test": map[string]any{"type": "nonexistent", "spec": map[string]any{}},
			},
			expectedProviders: 0,
			expectedTotal:     0,
		},
		{
			name: "Duration Parsing",
			abstract: abstractConfig{
				"duration-test": map[string]any{
					"type": "mock",
					"spec": map[string]any{
						"sleep": "5s",
					},
				},
			},
			expectedProviders: 1,
			expectedTotal:     1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			instances, _ := tt.abstract.harden(false)
			assert.Equal(t, tt.expectedProviders, len(instances), "providers count mismatch")
			// Count total instances
			total := 0
			for _, insts := range instances {
				total += len(insts)
			}
			assert.Equal(t, tt.expectedTotal, total, "total instances mismatch")
		})
	}
}

func TestHardenSetName(t *testing.T) {
	abstract := abstractConfig{
		"myinstance": map[string]any{"type": "mock", "spec": map[string]any{}},
	}

	result, _ := abstract.harden(false)

	// Check instance name is set from key
	instance := findInstanceByName(result["mock"], "myinstance")
	assert.NotNil(t, instance)
	assert.Equal(t, "myinstance", instance.GetName())
}

func TestHardenDurationParsing(t *testing.T) {
	abstract := abstractConfig{
		"test": map[string]any{
			"type": "mock",
			"spec": map[string]any{
				"sleep": "5s",
			},
		},
	}

	result, _ := abstract.harden(false)
	assert.Equal(t, 1, len(result["mock"]))

	instance := findInstanceByName(result["mock"], "test").(*mock.Component)
	assert.Equal(t, 5*time.Second, instance.Sleep)
}

func TestCountByProvider(t *testing.T) {
	result := &LoadResult{
		config: concreteConfig{
			"mock": {
				&mock.Component{InstanceName: "a"},
				&mock.Component{InstanceName: "b"},
				&mock.Component{InstanceName: "c"},
			},
		},
	}

	counts := result.countByProvider()
	assert.Equal(t, 3, counts["mock"])
}

func TestTotalInstances(t *testing.T) {
	tests := []struct {
		name     string
		result   *LoadResult
		expected int
	}{
		{
			name:     "Empty",
			result:   &LoadResult{config: make(concreteConfig)},
			expected: 0,
		},
		{
			name: "Multiple Providers",
			result: &LoadResult{
				config: concreteConfig{
					"mock": {
						&mock.Component{},
						&mock.Component{},
					},
					"other": {
						&mock.Component{},
						&mock.Component{},
					},
				},
			},
			expected: 4,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, tt.result.totalInstances())
		})
	}
}

// TestUnknownComponentKeysFixtures tests unknown key detection using testdata fixtures
func TestUnknownComponentKeysFixtures(t *testing.T) {
	testdataPath := getTestdataPath()

	tests := []struct {
		name            string
		configFile      string
		strict          bool
		expectErrors    int
		expectInstances int
	}{
		{
			name:            "Unknown component key in strict mode",
			configFile:      "unknown_component_key",
			strict:          true,
			expectErrors:    1,
			expectInstances: 1, // instance is still created despite unknown key
		},
		{
			name:            "Unknown component key in non-strict mode",
			configFile:      "unknown_component_key",
			strict:          false,
			expectErrors:    0, // only warning, no error
			expectInstances: 1,
		},
		{
			name:            "Multiple unknown keys in strict mode",
			configFile:      "multiple_unknown_keys",
			strict:          true,
			expectErrors:    2, // two unknown keys
			expectInstances: 1,
		},
		{
			name:            "Missing required key 'type' in strict mode",
			configFile:      "missing_type",
			strict:          true,
			expectErrors:    1,
			expectInstances: 0,
		},
		{
			name:            "Missing required key 'type' in non-strict mode",
			configFile:      "missing_type",
			strict:          false,
			expectErrors:    0, // only warning
			expectInstances: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := Load(testContext(t), []string{testdataPath}, tt.configFile, tt.strict)
			assert.NoError(t, err, "Load should not return error")
			assert.Equal(t, tt.expectErrors, len(result.ValidationErrors), "validation error count mismatch")
			assert.Equal(t, tt.expectInstances, len(result.GetInstances()), "instance count mismatch")
		})
	}
}

// TestUnknownSpecKeysFixtures tests unknown spec key detection using testdata fixtures
func TestUnknownSpecKeysFixtures(t *testing.T) {
	testdataPath := getTestdataPath()

	tests := []struct {
		name            string
		configFile      string
		strict          bool
		expectErrors    int
		expectInstances int
	}{
		{
			name:            "Unknown spec key in strict mode",
			configFile:      "unknown_spec_key",
			strict:          true,
			expectErrors:    1,
			expectInstances: 1, // instance is still created
		},
		{
			name:            "Unknown spec key in non-strict mode",
			configFile:      "unknown_spec_key",
			strict:          false,
			expectErrors:    0, // only warning, no error
			expectInstances: 1,
		},
		{
			name:            "Valid spec keys only",
			configFile:      "valid_spec_keys",
			strict:          true,
			expectErrors:    0,
			expectInstances: 1,
		},
		{
			name:            "Mix of valid and unknown spec keys",
			configFile:      "mixed_spec_keys",
			strict:          true,
			expectErrors:    1,
			expectInstances: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := Load(testContext(t), []string{testdataPath}, tt.configFile, tt.strict)
			assert.NoError(t, err, "Load should not return error")
			assert.Equal(t, tt.expectErrors, len(result.ValidationErrors), "validation error count mismatch")
			assert.Equal(t, tt.expectInstances, len(result.GetInstances()), "instance count mismatch")
		})
	}
}
