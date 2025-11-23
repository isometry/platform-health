package provider

import (
	"reflect"

	"github.com/spf13/viper"

	"github.com/isometry/platform-health/pkg/commands/flags"
)

// FlagConfigurable indicates a provider can be configured via CLI flags.
// Providers implementing this interface enable ad-hoc checks without config files
// via `ph check <provider> --flags...` and `ph context <provider> --flags...`.
type FlagConfigurable interface {
	// GetProviderFlags returns flag definitions for CLI generation.
	// The returned flags are used to dynamically build subcommands.
	GetProviderFlags() flags.FlagValues

	// ConfigureFromFlags applies Viper values to the provider.
	// Called after flags are parsed to configure the provider instance.
	ConfigureFromFlags(v *viper.Viper) error
}

// GetFlagConfigurableProviders returns a list of provider types that implement FlagConfigurable.
func GetFlagConfigurableProviders() []string {
	mu.RLock()
	defer mu.RUnlock()

	var configurable []string
	for name, providerType := range Providers {
		// Create a new instance to check interface
		instance := reflect.New(providerType.Elem()).Interface()
		if _, ok := instance.(FlagConfigurable); ok {
			configurable = append(configurable, name)
		}
	}
	return configurable
}

// IsFlagConfigurable checks if a provider instance implements FlagConfigurable.
func IsFlagConfigurable(instance Instance) bool {
	_, ok := instance.(FlagConfigurable)
	return ok
}

// AsFlagConfigurable returns the instance as FlagConfigurable if it implements the interface.
// Returns nil if the instance does not implement FlagConfigurable.
func AsFlagConfigurable(instance Instance) FlagConfigurable {
	if fc, ok := instance.(FlagConfigurable); ok {
		return fc
	}
	return nil
}
