package config

import (
	"context"
	"slices"
	"strings"
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

func TestGetInstances(t *testing.T) {
	tests := []struct {
		name     string
		config   *concreteConfig
		expected []provider.Instance
	}{
		{
			name:     "EmptyConfig",
			config:   &concreteConfig{},
			expected: []provider.Instance{},
		},
		{
			name: "PopulatedConfig",
			config: &concreteConfig{
				"provider1": []provider.Instance{
					&mock.Mock{Name: "1"},
					&mock.Mock{Name: "2"},
				},
				"provider2": []provider.Instance{
					&mock.Mock{Name: "3"},
				},
			},
			expected: []provider.Instance{
				&mock.Mock{Name: "1"},
				&mock.Mock{Name: "2"},
				&mock.Mock{Name: "3"},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			instances := tt.config.GetInstances()
			slices.SortFunc(instances, func(a, b provider.Instance) int {
				return strings.Compare(a.GetName(), b.GetName())
			})
			assert.Equal(t, tt.expected, instances)
		})
	}
}

func TestHarden(t *testing.T) {
	tests := []struct {
		name     string
		abstract abstractConfig
		expected concreteConfig
	}{
		{
			name:     "Empty Config",
			abstract: abstractConfig{},
			expected: concreteConfig{},
		},
		{
			name: "Simple Config",
			abstract: abstractConfig{
				"mock": []any{
					map[string]any{"Name": "1"},
					map[string]any{"Name": "2"},
				},
			},
			expected: concreteConfig{
				"mock": []provider.Instance{
					&mock.Mock{Name: "1", Health: 1, Sleep: 1},
					&mock.Mock{Name: "2", Health: 1, Sleep: 1},
				},
			},
		},
		{
			name: "Invalid Config",
			abstract: abstractConfig{
				"mock": "invalid",
			},
			expected: concreteConfig{},
		},
		{
			name: "Unknown Provider",
			abstract: abstractConfig{
				"unknown": []any{
					map[string]any{"Name": "1"},
				},
			},
			expected: concreteConfig{},
		},
		{
			name: "Duration Parsing",
			abstract: abstractConfig{
				"mock": []any{
					map[string]any{
						"name":  "duration-test",
						"sleep": "5s",
					},
				},
			},
			expected: concreteConfig{
				"mock": []provider.Instance{
					&mock.Mock{Name: "duration-test", Health: 1, Sleep: 5 * time.Second},
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.abstract.harden()
			assert.Equal(t, &tt.expected, result)
		})
	}
}
