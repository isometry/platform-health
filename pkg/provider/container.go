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
		// Extract provider kind for error context
		providerKind := extractKind(instanceConfig)

		// Collect validation warnings
		var validationWarnings []error
		instance, err := ResolveComponentConfig(instanceName, instanceConfig, &validationWarnings)

		// Wrap and collect validation warnings
		for _, warning := range validationWarnings {
			b.resolutionErrors = append(b.resolutionErrors,
				NewInstanceError(providerKind, instanceName, warning))
		}

		if err != nil {
			// Check if it's just a warning about unused keys (instance still created)
			var unusedWarning *UnusedKeysWarning
			if errors.As(err, &unusedWarning) {
				// Instance was created successfully, but has unused spec keys
				b.resolutionErrors = append(b.resolutionErrors,
					NewInstanceError(providerKind, instanceName, err))
				// Continue to add the instance (don't skip)
			} else {
				// Real error - instance creation failed
				b.resolutionErrors = append(b.resolutionErrors,
					NewInstanceError(providerKind, instanceName, err))
				continue
			}
		}

		b.resolved = append(b.resolved, instance)
	}

	return nil // errors collected in resolutionErrors, not returned
}

// extractKind extracts the provider kind from raw config for error reporting.
func extractKind(rawConfig any) string {
	if configMap, ok := rawConfig.(map[string]any); ok {
		if kindVal, hasKind := configMap["kind"].(string); hasKind {
			return kindVal
		}
	}
	return "unknown"
}
