package config

import (
	"context"
	"log/slog"

	"github.com/fsnotify/fsnotify"
	"github.com/spf13/viper"

	"github.com/isometry/platform-health/pkg/provider"
	"github.com/isometry/platform-health/pkg/utils"
)

// abstractConfig holds raw YAML before provider type resolution
// Structure: map[instanceName]instanceConfigWithType
// Each instance config must have a "type" field specifying the provider type
type abstractConfig map[string]any

// concreteConfig holds resolved provider instances
// Structure: map[providerType][]provider.Instance
type concreteConfig map[string][]provider.Instance

var log *slog.Logger

func Load(ctx context.Context, configPaths []string, configName string) (*concreteConfig, error) {
	log = utils.ContextLogger(ctx)

	conf := &concreteConfig{}
	if err := conf.initialize(configPaths, configName); err != nil {
		return nil, err
	}
	return conf, nil
}

func (c *concreteConfig) GetInstances() []provider.Instance {
	flatInstances := make([]provider.Instance, 0, c.totalInstances())

	for _, instances := range *c {
		flatInstances = append(flatInstances, instances...)
	}

	return flatInstances
}

// function to initialize provider configuration using viper
func (c *concreteConfig) initialize(configPaths []string, configName string) (err error) {
	log.Debug("initializing server configuration")

	if configName != "" {
		if configPaths == nil {
			configPaths = []string{"."}
		}
		for _, configPath := range configPaths {
			viper.AddConfigPath(configPath)
		}
		viper.SetConfigName(configName)

		if err = viper.ReadInConfig(); err != nil {
			if _, ok := err.(viper.ConfigFileNotFoundError); ok {
				log.Info("no configuration file found - using defaults")
			} else {
				log.Error("failed to read config", "error", err)
				return err
			}
		} else {
			log.Info("read config", slog.String("file", viper.ConfigFileUsed()))
			viper.WatchConfig()
			viper.OnConfigChange(func(e fsnotify.Event) {
				log.Debug("config change")
				if err = viper.ReadInConfig(); err != nil {
					log.Error("failed to read config", "error", err)
					return
				}
				if err = c.update(); err != nil {
					log.Error("failed to load config", "error", err)
				}

				log.Info("config reloaded", slog.Any("instances", c.countByProvider()))
			})
		}
	}

	if err := c.update(); err != nil {
		log.Error("failed to load config", "error", err)
		return err

	}

	log.Info("config loaded", slog.Any("instances", c.countByProvider()))

	return nil
}

func (c *concreteConfig) update() error {
	abstract := make(abstractConfig)

	if err := viper.Unmarshal(&abstract); err != nil {
		log.Error("failed to unmarshal config", "error", err)
		return err
	}

	*c = *abstract.harden()

	return nil
}

func (a abstractConfig) harden() *concreteConfig {
	concrete := make(concreteConfig)

	// Iterate over instances
	for instanceName, instanceConfig := range a {
		instanceLog := log.With(slog.String("instance", instanceName))

		// Convert instance config to map
		configMap, ok := instanceConfig.(map[string]any)
		if !ok {
			instanceLog.Warn("invalid instance configuration: expected map")
			continue
		}

		// Extract provider type from config
		providerType, ok := configMap["type"].(string)
		if !ok {
			instanceLog.Warn("missing or invalid 'type' field")
			continue
		}

		// Check if this is a registered provider type
		registeredType, isRegistered := provider.Providers[providerType]
		if !isRegistered {
			instanceLog.Warn("unknown provider type, skipping", slog.String("type", providerType))
			continue
		}

		// Initialize provider slice if needed
		if concrete[providerType] == nil {
			concrete[providerType] = make([]provider.Instance, 0)
		}

		instance, err := provider.NewInstanceFromConfig(registeredType, instanceName, configMap)
		if err != nil {
			instanceLog.Warn("failed to create instance", slog.Any("error", err))
			continue
		}

		instanceLog.Debug("loaded instance", slog.String("type", providerType))
		concrete[providerType] = append(concrete[providerType], instance)
	}

	return &concrete
}

func (c *concreteConfig) totalInstances() (count int) {
	for _, instances := range *c {
		count += len(instances)
	}
	return count
}

func (c *concreteConfig) countByProvider() map[string]int {
	counts := make(map[string]int)

	for providerType, instances := range *c {
		counts[providerType] = len(instances)
	}

	return counts
}
