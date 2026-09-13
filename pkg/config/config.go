package config

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"path/filepath"
	"slices"
	"sync"

	"github.com/fsnotify/fsnotify"
	"github.com/spf13/viper"

	"github.com/isometry/platform-health/pkg/phctx"
	"github.com/isometry/platform-health/pkg/provider"
)

// abstractConfig holds raw YAML before provider type resolution
// Structure: map[instanceName]instanceConfigWithTypeAndSpec
// Each instance config must have a "type" field specifying the provider type
// Provider-specific config goes under "spec" key
// Framework fields (type, checks, includes, components) stay at top level
type abstractConfig map[string]any

// concreteConfig holds resolved provider instances
// Structure: map[providerType][]provider.Instance
type concreteConfig map[string][]provider.Instance

// LoadResult wraps config with validation errors from loading.
// It is safe for concurrent use; the config reload callback (from fsnotify)
// writes under a write lock, while GetInstances and friends read under a read lock.
type LoadResult struct {
	mu               sync.RWMutex
	config           concreteConfig
	validationErrors []error
	v                *viper.Viper   // viper instance for config reloading
	log              *slog.Logger   // logger captured at Load time
}

// Load loads and validates configuration from the specified paths.
// If strict is true, validation errors are collected; otherwise invalid instances are skipped with warnings.
func Load(ctx context.Context, configPaths []string, configName string, strict bool) (*LoadResult, error) {
	result := &LoadResult{
		config: make(concreteConfig),
		v:      phctx.Viper(ctx), // use viper from context (has :: delimiter)
		log:    phctx.Logger(ctx),
	}

	if err := result.initialize(configPaths, configName, strict); err != nil {
		return nil, err
	}
	return result, nil
}

// HasErrors returns true if any validation errors were collected.
func (r *LoadResult) HasErrors() bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return len(r.validationErrors) > 0
}

// ValidationErrors returns a copy of the validation errors.
func (r *LoadResult) ValidationErrors() []error {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return slices.Clone(r.validationErrors)
}

// EnforceStrict logs all validation errors and returns an error if any exist.
// Used in strict mode to abort on configuration errors.
func (r *LoadResult) EnforceStrict(log *slog.Logger) error {
	errs := r.ValidationErrors()
	if len(errs) == 0 {
		return nil
	}
	for _, e := range errs {
		log.Error("configuration error", slog.Any("error", e))
	}
	return fmt.Errorf("configuration validation failed with %d error(s)", len(errs))
}

// GetInstances returns a flat slice of all loaded provider instances.
func (r *LoadResult) GetInstances() []provider.Instance {
	r.mu.RLock()
	defer r.mu.RUnlock()

	flatInstances := make([]provider.Instance, 0, r.totalInstancesLocked())

	for _, instances := range r.config {
		flatInstances = append(flatInstances, instances...)
	}

	return flatInstances
}

// initialize sets up provider configuration using viper
func (r *LoadResult) initialize(configPaths []string, configName string, strict bool) (err error) {
	r.log.Debug("initializing server configuration")

	// r.v is already set from context with :: delimiter

	if configName != "" {
		if configPaths == nil {
			configPaths = []string{"."}
		}
		for _, configPath := range configPaths {
			r.v.AddConfigPath(configPath)
		}
		r.v.SetConfigName(configName)

		if err = r.v.ReadInConfig(); err != nil {
			if _, ok := err.(viper.ConfigFileNotFoundError); ok {
				r.log.Info("no configuration file found - using defaults")
			} else {
				r.log.Error("failed to read config", "error", err)
				return err
			}
		} else {
			r.log.Info("read config", slog.String("file", r.v.ConfigFileUsed()))
			r.v.WatchConfig()
			r.v.OnConfigChange(func(e fsnotify.Event) {
				r.log.Debug("config change")
				if err := r.v.ReadInConfig(); err != nil {
					r.log.Error("failed to read config", "error", err)
					return
				}

				r.mu.Lock()
				prevConfig := r.config
				prevErrors := r.validationErrors
				r.mu.Unlock()

				if err := r.update(strict); err != nil {
					r.log.Error("failed to load config", "error", err)
					r.mu.Lock()
					r.config = prevConfig
					r.validationErrors = prevErrors
					r.mu.Unlock()
					return
				}

				r.mu.RLock()
				hasErrors := len(r.validationErrors) > 0
				r.mu.RUnlock()

				if strict && hasErrors {
					for _, e := range r.ValidationErrors() {
						r.log.Error("configuration error", slog.Any("error", e))
					}
					r.log.Warn("config reload rejected due to strict validation errors")
					r.mu.Lock()
					r.config = prevConfig
					r.validationErrors = prevErrors
					r.mu.Unlock()
					return
				}

				r.log.Info("config reloaded", slog.Any("instances", r.countByProvider()))
			})
		}
	}

	if err := r.update(strict); err != nil {
		r.log.Error("failed to load config", "error", err)
		return err
	}

	r.log.Info("config loaded", slog.Any("instances", r.countByProvider()))

	return nil
}

func (r *LoadResult) update(strict bool) error {
	raw := make(map[string]any)

	if err := r.v.Unmarshal(&raw); err != nil {
		r.log.Error("failed to unmarshal config", "error", err)
		return err
	}

	basePath := "."
	if configFile := r.v.ConfigFileUsed(); configFile != "" {
		basePath = filepath.Dir(configFile)
	}

	processed, err := ProcessIncludes(raw, basePath, nil)
	if err != nil {
		r.log.Error("failed to process includes", "error", err)
		return fmt.Errorf("failed to process includes: %w", err)
	}

	components, ok := processed["components"].(map[string]any)
	if !ok {
		return fmt.Errorf("config must have a 'components' key containing all component definitions")
	}

	abstract := abstractConfig(components)
	config, validationErrors := abstract.harden(r.log, strict)

	r.mu.Lock()
	r.config, r.validationErrors = config, validationErrors
	r.mu.Unlock()

	return nil
}

func (a abstractConfig) harden(log *slog.Logger, strict bool) (concreteConfig, []error) {
	concrete := make(concreteConfig)
	var validationErrors []error

	for instanceName, instanceConfig := range a {
		instanceLog := log.With(slog.String("instance", instanceName))

		providerType := provider.ExtractType(instanceConfig)

		var validationWarnings []error
		instance, err := provider.ResolveComponentConfig(instanceName, instanceConfig, &validationWarnings)

		for _, warning := range validationWarnings {
			if strict {
				validationErrors = append(validationErrors, provider.NewInstanceError(providerType, instanceName, warning))
			} else {
				instanceLog.Warn(warning.Error())
			}
		}

		if err != nil {
			var unusedWarning *provider.UnusedKeysWarning
			if errors.As(err, &unusedWarning) {
				if strict {
					validationErrors = append(validationErrors, provider.NewInstanceError(providerType, instanceName, err))
				} else {
					instanceLog.Warn(unusedWarning.Error())
				}
				// Continue to add the instance (don't skip)
			} else {
				// Real error - instance creation failed
				instErr := provider.NewInstanceError(providerType, instanceName, err)
				if strict {
					validationErrors = append(validationErrors, instErr)
				} else {
					instanceLog.Warn("failed to create instance", slog.Any("error", err))
				}
				continue
			}
		}

		if concrete[providerType] == nil {
			concrete[providerType] = make([]provider.Instance, 0, 1)
		}
		instanceLog.Debug("loaded instance", slog.String("type", providerType))
		concrete[providerType] = append(concrete[providerType], instance)

		if container := provider.AsContainer(instance); container != nil {
			for _, err := range container.ComponentErrors() {
				if strict {
					validationErrors = append(validationErrors, err)
				} else {
					instanceLog.Warn(err.Error())
				}
			}
		}
	}

	return concrete, validationErrors
}

// totalInstancesLocked returns the total instance count. Caller must hold r.mu.
func (r *LoadResult) totalInstancesLocked() (count int) {
	for _, instances := range r.config {
		count += len(instances)
	}
	return count
}

func (r *LoadResult) countByProvider() map[string]int {
	r.mu.RLock()
	defer r.mu.RUnlock()

	counts := make(map[string]int)

	for providerType, instances := range r.config {
		counts[providerType] = len(instances)
	}

	return counts
}
