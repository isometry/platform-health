package provider

import (
	"errors"
	"fmt"
	"reflect"
	"time"

	"github.com/go-viper/mapstructure/v2"
	"github.com/spf13/pflag"

	"github.com/isometry/platform-health/pkg/checks"
)

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
		return nil
	}
}

// New creates a raw provider instance without configuration or Setup().
// Use this for capability detection (e.g., checking interface implementations).
// For configured instances, use NewInstance instead.
func New(kind string) (Instance, error) {
	mu.RLock()
	registeredType, ok := Providers[kind]
	mu.RUnlock()
	if !ok {
		return nil, fmt.Errorf("unknown provider kind: %q", kind)
	}

	instance := reflect.New(registeredType.Elem())
	return instance.Interface().(Instance), nil
}

// NewInstance creates a provider instance with the given options.
// Returns an error if the provider kind is unknown or configuration fails.
// If no name is provided via WithName, defaults to the provider kind.
func NewInstance(kind string, opts ...Option) (Instance, error) {
	mu.RLock()
	registeredType, ok := Providers[kind]
	mu.RUnlock()
	if !ok {
		return nil, fmt.Errorf("unknown provider kind: %q", kind)
	}

	// Apply options
	cfg := &instanceConfig{}
	for _, opt := range opts {
		if err := opt(cfg); err != nil {
			return nil, err
		}
	}

	// Default name to kind if not provided
	name := cfg.name
	if name == "" {
		name = kind
	}

	// Create instance (pointer to struct)
	instance := reflect.New(registeredType.Elem())
	concreteInstance := instance.Interface().(Instance)
	concreteInstance.SetName(name)
	concreteInstance.SetTimeout(cfg.timeout)

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
			return nil, fmt.Errorf("components specified but provider %q does not implement Container", kind)
		}
		container.SetComponents(cfg.components)
	}

	// Run Setup
	if err := concreteInstance.Setup(); err != nil {
		return nil, fmt.Errorf("setup failed: %w", err)
	}

	// Validate and apply checks (requires InstanceWithChecks)
	if len(cfg.checks) > 0 {
		checkProvider := AsInstanceWithChecks(concreteInstance)
		if checkProvider == nil {
			return nil, fmt.Errorf("checks specified but provider %q does not implement InstanceWithChecks", kind)
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
	// Convert to map
	configMap, ok := rawConfig.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("invalid configuration: expected map")
	}

	// Extract provider kind early for accurate error reporting
	providerKind := "unknown"
	if kindVal, hasKind := configMap["kind"].(string); hasKind {
		providerKind = kindVal
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

	// Check required keys are present
	var missingRequired []string
	for key, required := range KnownComponentKeys {
		if required {
			if _, present := configMap[key]; !present {
				missingRequired = append(missingRequired, key)
			}
		}
	}
	if len(missingRequired) > 0 {
		var errs []error
		for _, key := range missingRequired {
			errs = append(errs, fmt.Errorf("missing required key %q", key))
		}
		return nil, errors.Join(errs...)
	}

	// Validate kind is actually a string (presence validated above)
	if _, ok := configMap["kind"].(string); !ok {
		return nil, fmt.Errorf("'kind' must be a string")
	}

	// Build options
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

	// Create instance
	return NewInstance(providerKind, opts...)
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
