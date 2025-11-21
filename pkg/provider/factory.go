package provider

import (
	"fmt"
	"reflect"

	"github.com/go-viper/mapstructure/v2"
)

// NewInstanceFromConfig creates a provider instance from a registered type and config map.
// It handles reflection-based instantiation, mapstructure decoding with duration support,
// name assignment, and Setup() invocation.
func NewInstanceFromConfig(registeredType reflect.Type, instanceName string, configMap map[string]any) (Instance, error) {
	// Create new instance of the provider type
	instance := reflect.New(registeredType)

	// Decode config into instance
	decoder, err := mapstructure.NewDecoder(&mapstructure.DecoderConfig{
		DecodeHook: mapstructure.StringToTimeDurationHookFunc(),
		Result:     instance.Interface(),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create decoder: %w", err)
	}

	if err := decoder.Decode(configMap); err != nil {
		return nil, fmt.Errorf("failed to decode config: %w", err)
	}

	concreteInstance := instance.Elem().Interface().(Instance)

	// Set name from instance key if not non-empty
	if concreteInstance.GetName() == "" {
		concreteInstance.SetName(instanceName)
	}

	if err := concreteInstance.Setup(); err != nil {
		return nil, fmt.Errorf("setup failed: %w", err)
	}

	return concreteInstance, nil
}
