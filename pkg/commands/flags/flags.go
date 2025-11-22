package flags

import (
	"time"

	"github.com/isometry/platform-health/pkg/config"
	"github.com/spf13/pflag"
	"github.com/spf13/viper"
)

// FlagValue represents a single flag definition with metadata
type FlagValue struct {
	Shorthand    string
	Kind         string
	Variable     any
	DefaultValue any
	NoOptDefault string
	Usage        string
}

// FlagValues is a map of flag names to their definitions
type FlagValues map[string]FlagValue

// Register adds all flags in the set to the given pflag.FlagSet
func (f FlagValues) Register(flagSet *pflag.FlagSet, sort bool) {
	for flagName, flag := range f {
		_ = flag.BuildFlag(flagSet, flagName)
	}
	flagSet.SortFlags = sort
}

// BuildFlag creates a pflag from the FlagValue definition and binds to Viper
func (f *FlagValue) BuildFlag(flagSet *pflag.FlagSet, flagName string) error {
	switch f.Kind {
	case "bool":
		flagSet.BoolVarP(f.Variable.(*bool), flagName, f.Shorthand, f.DefaultValue.(bool), f.Usage)
	case "count":
		flagSet.CountVarP(f.Variable.(*int), flagName, f.Shorthand, f.Usage)
	case "int":
		flagSet.IntVarP(f.Variable.(*int), flagName, f.Shorthand, f.DefaultValue.(int), f.Usage)
	case "string":
		flagSet.StringVarP(f.Variable.(*string), flagName, f.Shorthand, f.DefaultValue.(string), f.Usage)
	case "stringSlice":
		flagSet.StringSliceVarP(f.Variable.(*[]string), flagName, f.Shorthand, f.DefaultValue.([]string), f.Usage)
	case "duration":
		flagSet.DurationVarP(f.Variable.(*time.Duration), flagName, f.Shorthand, f.DefaultValue.(time.Duration), f.Usage)
	}

	flag := flagSet.Lookup(flagName)

	if f.NoOptDefault != "" {
		flag.NoOptDefVal = f.NoOptDefault
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

// Merge combines multiple FlagValues maps into one
func Merge(flagSets ...FlagValues) FlagValues {
	result := make(FlagValues)
	for _, fs := range flagSets {
		for k, v := range fs {
			result[k] = v
		}
	}
	return result
}

// Common flag definitions that can be reused across commands

// ConfigFlags returns flags for configuration file settings
func ConfigFlags(configPaths *[]string, configName *string) FlagValues {
	return FlagValues{
		"config-path": {
			Kind:         "stringSlice",
			Variable:     configPaths,
			DefaultValue: []string{"/config", "."},
			Usage:        "configuration paths",
		},
		"config-name": {
			Kind:         "string",
			Variable:     configName,
			DefaultValue: "platform-health",
			Usage:        "configuration name",
		},
	}
}

// ComponentFlags returns flags for component filtering
func ComponentFlags(components *[]string) FlagValues {
	return FlagValues{
		"component": {
			Shorthand:    "c",
			Kind:         "stringSlice",
			Variable:     components,
			DefaultValue: []string{},
			Usage:        "component(s) to check",
		},
	}
}

// OutputFlags returns flags for output formatting
func OutputFlags(flat *bool, quiet *int) FlagValues {
	return FlagValues{
		"flat": {
			Shorthand:    "f",
			Kind:         "bool",
			Variable:     flat,
			DefaultValue: false,
			Usage:        "flat output",
		},
		"quiet": {
			Shorthand:    "q",
			Kind:         "count",
			Variable:     quiet,
			DefaultValue: 0,
			Usage:        "quiet output (-q: hide healthy, -qq: summary only, -qqq: exit code only)",
		},
	}
}
