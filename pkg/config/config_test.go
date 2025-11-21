package config

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/isometry/platform-health/pkg/provider"
	"github.com/isometry/platform-health/pkg/provider/mock"
	"github.com/isometry/platform-health/pkg/utils"
)

func init() {
	log = utils.ContextLogger(context.TODO())
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
		config   *concreteConfig
		expected int // expected number of instances
	}{
		{
			name:     "EmptyConfig",
			config:   &concreteConfig{},
			expected: 0,
		},
		{
			name: "SingleProvider",
			config: &concreteConfig{
				"mock": []provider.Instance{
					&mock.Mock{Name: "comp1"},
					&mock.Mock{Name: "comp2"},
				},
			},
			expected: 2,
		},
		{
			name: "MultipleProviders",
			config: &concreteConfig{
				"mock": []provider.Instance{
					&mock.Mock{Name: "a"},
					&mock.Mock{Name: "b"},
				},
				"other": []provider.Instance{
					&mock.Mock{Name: "c"},
				},
			},
			expected: 3,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			instances := tt.config.GetInstances()
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
				"comp1": map[string]any{"type": "mock", "name": "1"},
				"comp2": map[string]any{"type": "mock", "name": "2"},
			},
			expectedProviders: 1,
			expectedTotal:     2,
		},
		{
			name: "Multiple Providers",
			abstract: abstractConfig{
				"a": map[string]any{"type": "mock", "name": "a"},
				"b": map[string]any{"type": "mock", "name": "b"},
			},
			expectedProviders: 1,
			expectedTotal:     2,
		},
		{
			name: "Unknown Provider",
			abstract: abstractConfig{
				"test": map[string]any{"type": "nonexistent", "name": "test"},
			},
			expectedProviders: 0,
			expectedTotal:     0,
		},
		{
			name: "Duration Parsing",
			abstract: abstractConfig{
				"duration-test": map[string]any{
					"type":  "mock",
					"name":  "duration-test",
					"sleep": "5s",
				},
			},
			expectedProviders: 1,
			expectedTotal:     1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.abstract.harden()
			assert.Equal(t, tt.expectedProviders, len(*result), "providers count mismatch")
			assert.Equal(t, tt.expectedTotal, result.totalInstances(), "total instances mismatch")
		})
	}
}

func TestHardenSetName(t *testing.T) {
	abstract := abstractConfig{
		"myinstance": map[string]any{"type": "mock"},
	}

	result := abstract.harden()

	// Check instance name is set from key
	instance := findInstanceByName((*result)["mock"], "myinstance")
	assert.NotNil(t, instance)
	assert.Equal(t, "myinstance", instance.GetName())
}

func TestHardenDurationParsing(t *testing.T) {
	abstract := abstractConfig{
		"test": map[string]any{
			"type":  "mock",
			"name":  "test",
			"sleep": "5s",
		},
	}

	result := abstract.harden()
	assert.Equal(t, 1, len((*result)["mock"]))

	instance := findInstanceByName((*result)["mock"], "test").(*mock.Mock)
	assert.Equal(t, 5*time.Second, instance.Sleep)
}

func TestCountByProvider(t *testing.T) {
	config := &concreteConfig{
		"mock": []provider.Instance{
			&mock.Mock{Name: "a"},
			&mock.Mock{Name: "b"},
			&mock.Mock{Name: "c"},
		},
	}

	counts := config.countByProvider()
	assert.Equal(t, 3, counts["mock"])
}

func TestTotalInstances(t *testing.T) {
	tests := []struct {
		name     string
		config   *concreteConfig
		expected int
	}{
		{
			name:     "Empty",
			config:   &concreteConfig{},
			expected: 0,
		},
		{
			name: "Multiple Providers",
			config: &concreteConfig{
				"mock": []provider.Instance{
					&mock.Mock{},
					&mock.Mock{},
				},
				"other": []provider.Instance{
					&mock.Mock{},
					&mock.Mock{},
				},
			},
			expected: 4,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, tt.config.totalInstances())
		})
	}
}
