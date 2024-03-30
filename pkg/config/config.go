package config

import (
	"context"
	"fmt"
	"log/slog"
	"reflect"
	"strings"

	"github.com/fsnotify/fsnotify"
	"github.com/mitchellh/mapstructure"
	"github.com/spf13/viper"

	"github.com/isometry/platform-health/pkg/provider"
	"github.com/isometry/platform-health/pkg/utils"
)

const ServerFlagPrefix = "server"

type abstractConfig map[string]any
type concreteConfig map[string][]provider.Instance

func New(ctx context.Context, configPaths []string, configName string) (*concreteConfig, error) {
	conf := &concreteConfig{}
	if err := conf.initialize(ctx, configPaths, configName); err != nil {
		return nil, err
	}
	return conf, nil
}

// function to initialize the configuration using viper
func (c *concreteConfig) initialize(ctx context.Context, configPaths []string, configName string) (err error) {
	log := utils.ContextLogger(ctx, slog.String("context", "config"))
	log.Debug("initializing config")

	viper.AutomaticEnv() // read in environment variables that match bound variables
	viper.SetEnvKeyReplacer(strings.NewReplacer(".", "_", "-", "_"))

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
				log.Info("No configuration file found - using defaults")
			} else {
				log.Error("error reading config", "error", err)
				return err
			}
		} else {
			log = log.With(slog.String("configFile", viper.ConfigFileUsed()))
			viper.WatchConfig()
			viper.OnConfigChange(func(e fsnotify.Event) {
				log.Debug("config change")
				if err = viper.ReadInConfig(); err != nil {
					log.Error("error reading config", "error", err)
					return
				}
				concreteConf, err := UnmarshalConfig(ctx)
				if err != nil {
					log.Error("failed to update config", "error", err)
					return
				}
				*c = *concreteConf
				log.Info("config updated", slog.Any("instances", concreteConf.InstanceCounts()))
			})
		}
	}

	concreteConf, err := UnmarshalConfig(ctx)
	if err != nil {
		log.Error("failed to load config", "error", err)
		return err
	}

	*c = *concreteConf
	log.Info("config loaded", slog.Any("instances", concreteConf.InstanceCounts()))

	return nil
}

func UnmarshalConfig(ctx context.Context) (*concreteConfig, error) {
	log := utils.ContextLogger(ctx, slog.String("context", "config"))
	log.Debug("reading config", "configFile", viper.ConfigFileUsed())

	abstract := make(abstractConfig)
	if err := viper.Unmarshal(&abstract); err != nil {
		log.Error("error unmarshalling config", "error", err)
		return nil, err
	}

	return abstract.Harden(ctx)
}

func (c *concreteConfig) count() (count int) {
	for _, instances := range *c {
		count += len(instances)
	}
	return count
}

func (c *concreteConfig) GetInstances() []provider.Instance {
	all := make([]provider.Instance, 0, c.count())
	for _, instances := range *c {
		all = append(all, instances...)
	}
	return all
}

func (c *abstractConfig) Harden(ctx context.Context) (*concreteConfig, error) {
	log := utils.ContextLogger(ctx, slog.String("context", "config"))
	concrete := concreteConfig{}
	for typeName, instances := range *c {
		if typeName == ServerFlagPrefix {
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
				log.Warn(fmt.Sprintf("failed to load instance[%d]: %v", i, err))
				continue
			}

			concreteInstance := instance.Elem().Interface().(provider.Instance)
			concreteInstance.SetDefaults()

			concrete[typeName] = append(concrete[typeName], concreteInstance)
		}
	}

	return &concrete, nil
}

func (c *concreteConfig) InstanceCounts() map[string]int {
	counts := make(map[string]int)
	for typeName, instances := range *c {
		counts[typeName] = len(instances)
	}
	return counts
}
