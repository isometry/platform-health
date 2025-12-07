package provider_test

import (
	"context"
	"testing"
	"time"

	"github.com/spf13/pflag"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	ph "github.com/isometry/platform-health/pkg/platform_health"
	"github.com/isometry/platform-health/pkg/provider"
)

// testProviderWithPointers is a test provider with various pointer fields.
type testProviderWithPointers struct {
	provider.Base

	Name      string         `mapstructure:"name"`
	PtrBool   *bool          `mapstructure:"ptr-bool"`
	PtrString *string        `mapstructure:"ptr-string"`
	PtrInt    *int           `mapstructure:"ptr-int"`
	Duration  *time.Duration `mapstructure:"ptr-duration"`
}

func (t *testProviderWithPointers) GetType() string { return "test-ptr" }
func (t *testProviderWithPointers) Setup() error    { return nil }
func (t *testProviderWithPointers) GetHealth(context.Context) *ph.HealthCheckResponse {
	return &ph.HealthCheckResponse{Status: ph.Status_HEALTHY}
}

func TestProviderFlags_PointerTypes(t *testing.T) {
	instance := &testProviderWithPointers{}

	flags := provider.ProviderFlags(instance)

	// Verify pointer fields are detected with correct underlying types
	assert.Contains(t, flags, "ptr-bool", "should have ptr-bool flag")
	assert.Contains(t, flags, "ptr-string", "should have ptr-string flag")
	assert.Contains(t, flags, "ptr-int", "should have ptr-int flag")
	assert.Contains(t, flags, "ptr-duration", "should have ptr-duration flag")

	// Verify the flag kinds are correct (unwrapped from pointer)
	assert.Equal(t, "bool", flags["ptr-bool"].Kind)
	assert.Equal(t, "string", flags["ptr-string"].Kind)
	assert.Equal(t, "int", flags["ptr-int"].Kind)
	assert.Equal(t, "duration", flags["ptr-duration"].Kind)
}

func TestConfigureFromFlags_PointerBool(t *testing.T) {
	tests := []struct {
		name     string
		flagVal  string
		expected bool
	}{
		{"true", "true", true},
		{"false", "false", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			instance := &testProviderWithPointers{}

			// Create and set up flags
			fs := pflag.NewFlagSet("test", pflag.ContinueOnError)
			flags := provider.ProviderFlags(instance)
			flags.Register(fs, false)

			// Set the flag value
			err := fs.Set("ptr-bool", tt.flagVal)
			require.NoError(t, err)

			// Configure from flags
			err = provider.ConfigureFromFlags(instance, fs)
			require.NoError(t, err)

			// Verify pointer was set correctly
			require.NotNil(t, instance.PtrBool, "pointer should not be nil")
			assert.Equal(t, tt.expected, *instance.PtrBool)
		})
	}
}

func TestConfigureFromFlags_PointerString(t *testing.T) {
	instance := &testProviderWithPointers{}

	fs := pflag.NewFlagSet("test", pflag.ContinueOnError)
	flags := provider.ProviderFlags(instance)
	flags.Register(fs, false)

	err := fs.Set("ptr-string", "hello")
	require.NoError(t, err)

	err = provider.ConfigureFromFlags(instance, fs)
	require.NoError(t, err)

	require.NotNil(t, instance.PtrString)
	assert.Equal(t, "hello", *instance.PtrString)
}

func TestConfigureFromFlags_PointerInt(t *testing.T) {
	instance := &testProviderWithPointers{}

	fs := pflag.NewFlagSet("test", pflag.ContinueOnError)
	flags := provider.ProviderFlags(instance)
	flags.Register(fs, false)

	err := fs.Set("ptr-int", "42")
	require.NoError(t, err)

	err = provider.ConfigureFromFlags(instance, fs)
	require.NoError(t, err)

	require.NotNil(t, instance.PtrInt)
	assert.Equal(t, 42, *instance.PtrInt)
}

func TestConfigureFromFlags_PointerDuration(t *testing.T) {
	instance := &testProviderWithPointers{}

	fs := pflag.NewFlagSet("test", pflag.ContinueOnError)
	flags := provider.ProviderFlags(instance)
	flags.Register(fs, false)

	err := fs.Set("ptr-duration", "5s")
	require.NoError(t, err)

	err = provider.ConfigureFromFlags(instance, fs)
	require.NoError(t, err)

	require.NotNil(t, instance.Duration)
	assert.Equal(t, 5*time.Second, *instance.Duration)
}

// testProviderWithPtrDefault tests pointer fields with default values.
type testProviderWithPtrDefault struct {
	provider.Base

	PtrBool *bool `mapstructure:"ptr-bool" default:"true"`
}

func (t *testProviderWithPtrDefault) GetType() string { return "test-ptr-default" }
func (t *testProviderWithPtrDefault) Setup() error    { return nil }
func (t *testProviderWithPtrDefault) GetHealth(context.Context) *ph.HealthCheckResponse {
	return &ph.HealthCheckResponse{Status: ph.Status_HEALTHY}
}

func TestProviderFlags_PointerWithDefault(t *testing.T) {
	instance := &testProviderWithPtrDefault{}

	flags := provider.ProviderFlags(instance)

	// Verify default value is parsed correctly for pointer
	assert.Contains(t, flags, "ptr-bool")
	assert.Equal(t, true, flags["ptr-bool"].DefaultValue)
}
