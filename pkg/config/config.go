package config

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"path/filepath"

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

// LoadResult wraps config with validation errors from loading
type LoadResult struct {
	config           concreteConfig
	ValidationErrors []error
	v                *viper.Viper // viper instance for config reloading
}

var log *slog.Logger

// Load loads and validates configuration from the specified paths.
// If strict is true, validation errors are collected; otherwise invalid instances are skipped with warnings.
func Load(ctx context.Context, configPaths []string, configName string, strict bool) (*LoadResult, error) {
	log = phctx.Logger(ctx)

	result := &LoadResult{
		config: make(concreteConfig),
		v:      phctx.Viper(ctx), // use viper from context (has :: delimiter)
	}

	if err := result.initialize(configPaths, configName, strict); err != nil {
		return nil, err
	}
	return result, nil
}

// HasErrors returns true if any validation errors were collected.
func (r *LoadResult) HasErrors() bool {
	return len(r.ValidationErrors) > 0
}

// GetInstances returns a flat slice of all loaded provider instances.
func (r *LoadResult) GetInstances() []provider.Instance {
	flatInstances := make([]provider.Instance, 0, r.totalInstances())

	for _, instances := range r.config {
		flatInstances = append(flatInstances, instances...)
	}

	return flatInstances
}

// initialize sets up provider configuration using viper
func (r *LoadResult) initialize(configPaths []string, configName string, strict bool) (err error) {
	log.Debug("initializing server configuration")

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
				log.Info("no configuration file found - using defaults")
			} else {
				log.Error("failed to read config", "error", err)
				return err
			}
		} else {
			log.Info("read config", slog.String("file", r.v.ConfigFileUsed()))
			r.v.WatchConfig()
			r.v.OnConfigChange(func(e fsnotify.Event) {
				log.Debug("config change")
				if err = r.v.ReadInConfig(); err != nil {
					log.Error("failed to read config", "error", err)
					return
				}
				if err = r.update(strict); err != nil {
					log.Error("failed to load config", "error", err)
				}

				log.Info("config reloaded", slog.Any("instances", r.countByProvider()))
			})
		}
	}

	if err := r.update(strict); err != nil {
		log.Error("failed to load config", "error", err)
		return err
	}

	log.Info("config loaded", slog.Any("instances", r.countByProvider()))

	return nil
}

func (r *LoadResult) update(strict bool) error {
	raw := make(map[string]any)

	if err := r.v.Unmarshal(&raw); err != nil {
		log.Error("failed to unmarshal config", "error", err)
		return err
	}

	basePath := "."
	if configFile := r.v.ConfigFileUsed(); configFile != "" {
		basePath = filepath.Dir(configFile)
	}

	processed, err := ProcessIncludes(raw, basePath, nil)
	if err != nil {
		log.Error("failed to process includes", "error", err)
		return fmt.Errorf("failed to process includes: %w", err)
	}

	components, ok := processed["components"].(map[string]any)
	if !ok {
		return fmt.Errorf("config must have a 'components' key containing all component definitions")
	}

	abstract := abstractConfig(components)
	r.config, r.ValidationErrors = abstract.harden(strict)

	return nil
}

func (a abstractConfig) harden(strict bool) (concreteConfig, []error) {
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
	}

	return concrete, validationErrors
}

func (r *LoadResult) totalInstances() (count int) {
	for _, instances := range r.config {
		count += len(instances)
	}
	return count
}

func (r *LoadResult) countByProvider() map[string]int {
	counts := make(map[string]int)

	for providerType, instances := range r.config {
		counts[providerType] = len(instances)
	}

	return counts
}
