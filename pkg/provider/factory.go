package provider

import (
	"fmt"
	"reflect"
	"time"

	"github.com/go-viper/mapstructure/v2"
	"github.com/spf13/pflag"

	"github.com/isometry/platform-health/pkg/checks"
)

// KnownComponentKeys defines valid keys at the component configuration level.
// The boolean value indicates whether the key is required (true) or optional (false).
// Key existence alone indicates validity; unknown keys will generate warnings.
var KnownComponentKeys = map[string]bool{
	"type":       true,  // required: provider type
	"spec":       false, // optional: provider-specific configuration
	"checks":     false, // optional: CEL expressions
	"components": false, // optional: nested children (Container providers)
	"timeout":    false, // optional: per-instance timeout override
	"includes":   false, // optional: include other configuration files
}

// Option configures instance creation.
type Option func(*instanceConfig) error

// instanceConfig holds configuration for instance creation.
type instanceConfig struct {
	name       string
	spec       map[string]any
	checks     []checks.Expression
	components map[string]any
	flags      *pflag.FlagSet
	timeout    time.Duration
	hasTimeout bool
}

// WithName sets the instance name.
func WithName(name string) Option {
	return func(c *instanceConfig) error {
		c.name = name
		return nil
	}
}

// WithSpec sets provider-specific configuration (decoded via mapstructure).
func WithSpec(spec map[string]any) Option {
	return func(c *instanceConfig) error {
		c.spec = spec
		return nil
	}
}

// WithChecks sets CEL check expressions.
// Returns an error during NewInstance if the provider doesn't implement InstanceWithChecks.
func WithChecks(exprs []checks.Expression) Option {
	return func(c *instanceConfig) error {
		c.checks = exprs
		return nil
	}
}

// WithComponents sets nested component configuration.
// Returns an error during NewInstance if the provider doesn't implement Container.
func WithComponents(components map[string]any) Option {
	return func(c *instanceConfig) error {
		c.components = components
		return nil
	}
}

// WithFlags configures the provider from CLI flags.
// This is mutually exclusive with WithSpec - flags take precedence.
func WithFlags(fs *pflag.FlagSet) Option {
	return func(c *instanceConfig) error {
		c.flags = fs
		return nil
	}
}

// WithTimeout sets a per-instance timeout override.
// When set, GetHealthWithDuration creates a child context with this timeout.
func WithTimeout(timeout time.Duration) Option {
	return func(c *instanceConfig) error {
		c.timeout = timeout
		c.hasTimeout = true
		return nil
	}
}

// New creates a raw provider instance without configuration or Setup().
// Use this for capability detection (e.g., checking interface implementations).
// For configured instances, use NewInstance instead.
func New(providerType string) (Instance, error) {
	mu.RLock()
	registeredType, ok := Providers[providerType]
	mu.RUnlock()
	if !ok {
		return nil, fmt.Errorf("unknown provider type: %q", providerType)
	}

	instance := reflect.New(registeredType.Elem())
	return instance.Interface().(Instance), nil
}

// NewInstance creates a provider instance with the given options.
// Returns an error if the provider type is unknown or configuration fails.
// If no name is provided via WithName, defaults to the provider type.
func NewInstance(providerType string, opts ...Option) (Instance, error) {
	mu.RLock()
	registeredType, ok := Providers[providerType]
	mu.RUnlock()
	if !ok {
		return nil, fmt.Errorf("unknown provider type: %q", providerType)
	}

	// Apply options
	cfg := &instanceConfig{}
	for _, opt := range opts {
		if err := opt(cfg); err != nil {
			return nil, err
		}
	}

	// Default name to type if not provided
	name := cfg.name
	if name == "" {
		name = providerType
	}

	// Create instance (pointer to struct)
	instance := reflect.New(registeredType.Elem())
	concreteInstance := instance.Interface().(Instance)
	concreteInstance.SetName(name)

	// Track unused keys for warning
	var unusedKeys []string

	// Configure from flags OR spec (flags take precedence)
	if cfg.flags != nil {
		if err := ConfigureFromFlags(concreteInstance, cfg.flags); err != nil {
			return nil, fmt.Errorf("failed to configure from flags: %w", err)
		}
	} else if cfg.spec != nil {
		// Decode ONLY spec via mapstructure (not components)
		var metadata mapstructure.Metadata
		decoder, err := mapstructure.NewDecoder(&mapstructure.DecoderConfig{
			DecodeHook: mapstructure.StringToTimeDurationHookFunc(),
			Result:     instance.Interface(),
			Metadata:   &metadata,
		})
		if err != nil {
			return nil, fmt.Errorf("failed to create decoder: %w", err)
		}
		if err := decoder.Decode(cfg.spec); err != nil {
			return nil, fmt.Errorf("failed to decode config: %w", err)
		}
		unusedKeys = metadata.Unused
	}

	// Handle components via Container interface (bypasses mapstructure)
	if cfg.components != nil {
		container := AsContainer(concreteInstance)
		if container == nil {
			return nil, fmt.Errorf("components specified but provider %q does not implement Container", providerType)
		}
		container.SetComponents(cfg.components)
	}

	// Run Setup (providers set their default timeout here)
	if err := concreteInstance.Setup(); err != nil {
		return nil, fmt.Errorf("setup failed: %w", err)
	}

	// Apply explicit timeout override (after Setup, so it wins over provider defaults)
	if cfg.hasTimeout {
		concreteInstance.SetTimeout(cfg.timeout)
	}

	// Validate and apply checks (requires InstanceWithChecks)
	if len(cfg.checks) > 0 {
		checkProvider := AsInstanceWithChecks(concreteInstance)
		if checkProvider == nil {
			return nil, fmt.Errorf("checks specified but provider %q does not implement InstanceWithChecks", providerType)
		}
		if err := checkProvider.SetChecks(cfg.checks); err != nil {
			return nil, fmt.Errorf("failed to set checks: %w", err)
		}
	}

	// Return warning if there were unused keys in spec (caller decides warn vs error)
	if len(unusedKeys) > 0 {
		return concreteInstance, &UnusedKeysWarning{Keys: unusedKeys}
	}

	return concreteInstance, nil
}

// validateComponentConfig validates raw config and returns typed map with provider type.
// Populates validationErrors for unknown keys.
func validateComponentConfig(rawConfig any, validationErrors *[]error) (configMap map[string]any, providerType string, err error) {
	configMap, ok := rawConfig.(map[string]any)
	if !ok {
		return nil, "unknown", fmt.Errorf("invalid configuration: expected map")
	}

	providerType = "unknown"
	if typeVal, hasType := configMap["type"].(string); hasType {
		providerType = typeVal
	}

	// Validate component-level keys
	for key := range configMap {
		if _, valid := KnownComponentKeys[key]; !valid {
			if validationErrors != nil {
				*validationErrors = append(*validationErrors,
					fmt.Errorf("unknown key %q in component configuration", key))
			}
		}
	}

	// Check required keys
	for key, required := range KnownComponentKeys {
		if required {
			if _, present := configMap[key]; !present {
				return nil, providerType, fmt.Errorf("missing required key %q", key)
			}
		}
	}

	return configMap, providerType, nil
}

// buildInstanceOptions creates Option slice from validated config map.
func buildInstanceOptions(name string, configMap map[string]any) ([]Option, error) {
	opts := []Option{WithName(name)}

	if specMap, hasSpec := configMap["spec"].(map[string]any); hasSpec {
		opts = append(opts, WithSpec(specMap))
	}
	if components, hasComponents := configMap["components"].(map[string]any); hasComponents {
		opts = append(opts, WithComponents(components))
	}
	if timeoutRaw, hasTimeout := configMap["timeout"]; hasTimeout {
		timeout, err := ParseDuration(timeoutRaw)
		if err != nil {
			return nil, fmt.Errorf("invalid timeout: %w", err)
		}
		opts = append(opts, WithTimeout(timeout))
	}
	if checksRaw, hasChecks := configMap["checks"]; hasChecks {
		exprs, err := checks.ParseConfig(checksRaw)
		if err != nil {
			return nil, fmt.Errorf("invalid checks configuration: %w", err)
		}
		opts = append(opts, WithChecks(exprs))
	}

	return opts, nil
}

// ResolveComponentConfig validates raw component configuration and creates a provider instance.
// It validates component-level keys, checks for required fields, and builds the appropriate options.
//
// Returns:
//   - (Instance, nil): Successfully created with no warnings
//   - (Instance, UnusedKeysWarning): Created successfully but has unknown spec keys
//   - (nil, error): Failed to create - validation errors or creation failure
//
// The validationErrors slice is populated with all validation issues found (for reporting),
// even if the instance was created successfully.
func ResolveComponentConfig(name string, rawConfig any, validationErrors *[]error) (Instance, error) {
	configMap, providerType, err := validateComponentConfig(rawConfig, validationErrors)
	if err != nil {
		return nil, err
	}

	opts, err := buildInstanceOptions(name, configMap)
	if err != nil {
		return nil, err
	}

	return NewInstance(providerType, opts...)
}

// ParseDuration parses a duration from various formats:
//   - string: "10s", "1m", etc. (Go duration format)
//   - int/float: interpreted as seconds
//   - time.Duration: used directly
func ParseDuration(v any) (time.Duration, error) {
	switch val := v.(type) {
	case string:
		return time.ParseDuration(val)
	case time.Duration:
		return val, nil
	case int:
		return time.Duration(val) * time.Second, nil
	case int64:
		return time.Duration(val) * time.Second, nil
	case float64:
		return time.Duration(val * float64(time.Second)), nil
	default:
		return 0, fmt.Errorf("unsupported duration type: %T", v)
	}
}

// ExtractType extracts the "type" field from a raw component configuration.
// Returns "unknown" if the field is missing or not a string.
func ExtractType(rawConfig any) string {
	if configMap, ok := rawConfig.(map[string]any); ok {
		if typeVal, hasType := configMap["type"].(string); hasType {
			return typeVal
		}
	}
	return "unknown"
}
