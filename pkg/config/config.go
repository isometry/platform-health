package config

import (
	"context"
	"fmt"
	"log/slog"
	"reflect"

	"github.com/fsnotify/fsnotify"
	"github.com/go-viper/mapstructure/v2"
	"github.com/spf13/viper"

	"github.com/isometry/platform-health/pkg/provider"
	"github.com/isometry/platform-health/pkg/utils"
)

type abstractConfig map[string]any
type concreteConfig map[string][]provider.Instance

type flagPrefix string

func (f flagPrefix) ViperKey(flag string) string {
	return fmt.Sprintf("%s.%s", f, flag)
}

var FlagPrefix = flagPrefix("server")

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

func (c *abstractConfig) harden() *concreteConfig {
	concrete := concreteConfig{}

	for typeName, instances := range *c {
		if typeName == string(FlagPrefix) {
			// skip bound server flags
			continue
		}

		log := log.With(slog.String("provider", typeName))

		providerType, ok := provider.Providers[typeName]
		if !ok {
			log.Warn("skipping unknown provider")
			continue
		}

		log.Debug("loading instances")
		abstractInstances, ok := instances.([]any)
		if !ok {
			log.Warn("invalid provider configuration")
			continue
		}

		concrete[typeName] = make([]provider.Instance, 0, len(abstractInstances))

		for i, abstractInstance := range abstractInstances {
			instance := reflect.New(providerType)

			if err := mapstructure.Decode(abstractInstance, instance.Interface()); err != nil {
				log.Warn("failed to decode instance", slog.Int("index", i), slog.Any("error", err))
				continue
			}

			concreteInstance := instance.Elem().Interface().(provider.Instance)
			concreteInstance.SetDefaults()

			concrete[typeName] = append(concrete[typeName], concreteInstance)
		}
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

	for provider, instances := range *c {
		counts[provider] = len(instances)
	}

	return counts
}
