package provider_test

import (
	"reflect"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/isometry/platform-health/pkg/provider"
	"github.com/isometry/platform-health/pkg/provider/mock"
)

func TestRegisterAuto(t *testing.T) {
	tests := []struct {
		name     string
		provider provider.Instance
	}{
		{
			name:     mock.ProviderType,
			provider: new(mock.Component),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			registeredProvider, ok := provider.Providers[tt.name]
			assert.True(t, ok, "Provider was not auto-registered")
			assert.Equal(t, reflect.TypeOf(tt.provider), registeredProvider)
		})
	}
}

func TestRegisterManual(t *testing.T) {
	tests := []struct {
		name     string
		provider provider.Instance
	}{
		{
			name:     "mock_manual",
			provider: new(mock.Component),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			provider.Register(tt.name, tt.provider)

			registeredProvider, ok := provider.Providers[tt.name]
			assert.True(t, ok, "Provider was not registered")
			assert.Equal(t, reflect.TypeOf(tt.provider), registeredProvider)
		})
	}
}
