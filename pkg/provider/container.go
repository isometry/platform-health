package provider

import (
	"errors"
)

// BaseContainer provides reusable container handling for providers with nested components.
// It manages component storage, resolution, and error collection.
type BaseContainer struct {
	components       map[string]any // raw config from SetComponents
	resolved         []Instance
	resolutionErrors []error
}

// SetComponents stores raw component config. Called before Setup().
func (b *BaseContainer) SetComponents(config map[string]any) {
	b.components = config
}

// GetComponents returns resolved child instances. Available after ResolveComponents().
func (b *BaseContainer) GetComponents() []Instance {
	return b.resolved
}

// ComponentErrors returns validation errors from resolution.
func (b *BaseContainer) ComponentErrors() []error {
	return b.resolutionErrors
}

// ResolveComponents resolves all child components from stored config.
// Call this from the embedding provider's Setup() method.
func (b *BaseContainer) ResolveComponents() error {
	b.resolved = make([]Instance, 0)
	b.resolutionErrors = nil

	for instanceName, instanceConfig := range b.components {
		// Extract provider type for error context
		providerType := ExtractType(instanceConfig)

		// Collect validation warnings
		var validationWarnings []error
		instance, err := ResolveComponentConfig(instanceName, instanceConfig, &validationWarnings)

		// Wrap and collect validation warnings
		for _, warning := range validationWarnings {
			b.resolutionErrors = append(b.resolutionErrors,
				NewInstanceError(providerType, instanceName, warning))
		}

		if err != nil {
			b.resolutionErrors = append(b.resolutionErrors,
				NewInstanceError(providerType, instanceName, err))

			// Skip instance only if it's a real error (not just unused keys warning)
			var unusedWarning *UnusedKeysWarning
			if !errors.As(err, &unusedWarning) {
				continue
			}
		}

		b.resolved = append(b.resolved, instance)
	}

	if len(b.resolved) == 0 && len(b.resolutionErrors) > 0 {
		return errors.Join(b.resolutionErrors...)
	}

	return nil
}
