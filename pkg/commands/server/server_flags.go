package server

import (
	"github.com/isometry/platform-health/pkg/config"
	"github.com/isometry/platform-health/pkg/utils"
	"github.com/spf13/pflag"
	"github.com/spf13/viper"
)

type flagValue struct {
	shorthand    string
	kind         string
	variable     any
	defaultValue any
	noOptDefault string
	usage        string
}

type flagValues map[string]flagValue

var serverFlags = flagValues{
	"listen": {
		shorthand:    "l",
		kind:         "string",
		variable:     &listenHost,
		defaultValue: "",
		noOptDefault: "localhost",
		usage:        "listen on host (default all interfaces)",
	},
	"port": {
		shorthand:    "p",
		kind:         "int",
		variable:     &listenPort,
		defaultValue: 8080,
		usage:        "listen on port",
	},
	"config-path": {
		shorthand:    "C",
		kind:         "stringSlice",
		variable:     &configPaths,
		defaultValue: []string{"/config", "."},
		usage:        "configuration paths",
	},
	"config-name": {
		shorthand:    "c",
		kind:         "string",
		variable:     &configName,
		defaultValue: "platform-health",
		usage:        "configuration name",
	},
	"no-grpc-health-v1": {
		shorthand:    "H",
		kind:         "bool",
		variable:     &noGrpcHealthV1,
		defaultValue: false,
		usage:        "disable gRPC Health v1",
	},
	"grpc-reflection": {
		shorthand:    "R",
		kind:         "bool",
		variable:     &grpcReflection,
		defaultValue: false,
		usage:        "enable gRPC reflection",
	},
	"json": {
		shorthand:    "j",
		kind:         "bool",
		variable:     &jsonOutput,
		defaultValue: !utils.IsTTY(),
		usage:        "json logs",
	},
	"debug": {
		shorthand:    "d",
		kind:         "bool",
		variable:     &debugMode,
		defaultValue: false,
		usage:        "debug mode",
	},
	"verbosity": {
		shorthand:    "v",
		kind:         "count",
		variable:     &verbosity,
		defaultValue: 0,
		usage:        "verbose output",
	},
}

func (f flagValues) register(flagSet *pflag.FlagSet, sort bool) {
	for flagName, flag := range f {
		flag.buildFlag(flagSet, flagName)
	}

	flagSet.SortFlags = sort
}

func (f *flagValue) buildFlag(flagSet *pflag.FlagSet, flagName string) error {
	switch f.kind {
	case "bool":
		flagSet.BoolVarP(f.variable.(*bool), flagName, f.shorthand, f.defaultValue.(bool), f.usage)
	case "count":
		flagSet.CountVarP(f.variable.(*int), flagName, f.shorthand, f.usage)
	case "int":
		flagSet.IntVarP(f.variable.(*int), flagName, f.shorthand, f.defaultValue.(int), f.usage)
	case "string":
		flagSet.StringVarP(f.variable.(*string), flagName, f.shorthand, f.defaultValue.(string), f.usage)
	case "stringSlice":
		flagSet.StringSliceVarP(f.variable.(*[]string), flagName, f.shorthand, f.defaultValue.([]string), f.usage)
	}

	flag := flagSet.Lookup(flagName)

	if f.noOptDefault != "" {
		flag.NoOptDefVal = f.noOptDefault
	}

	viperKey := config.FlagPrefix.ViperKey(flagName)

	// Update pflag bound variables with viper values if set
	if viper.Get(viperKey) != nil {
		if err := flag.Value.Set(viper.GetString(viperKey)); err != nil {
			return err
		}
	}

	return nil
}
