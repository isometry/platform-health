package flags

import (
	"time"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"github.com/spf13/viper"
)

// FlagValue represents a single flag definition with metadata
type FlagValue struct {
	Shorthand    string
	Kind         string
	DefaultValue any
	NoOptDefault string
	Usage        string
}

// FlagValues is a map of flag names to their definitions
type FlagValues map[string]FlagValue

// Register adds all flags in the set to the given pflag.FlagSet
func (f FlagValues) Register(flagSet *pflag.FlagSet, sort bool) {
	for flagName, flag := range f {
		flag.BuildFlag(flagSet, flagName)
	}
	flagSet.SortFlags = sort
}

// BuildFlag creates a pflag from the FlagValue definition
func (f *FlagValue) BuildFlag(flagSet *pflag.FlagSet, flagName string) {
	switch f.Kind {
	case "bool":
		flagSet.BoolP(flagName, f.Shorthand, f.DefaultValue.(bool), f.Usage)
	case "count":
		flagSet.CountP(flagName, f.Shorthand, f.Usage)
	case "int":
		flagSet.IntP(flagName, f.Shorthand, f.DefaultValue.(int), f.Usage)
	case "string":
		flagSet.StringP(flagName, f.Shorthand, f.DefaultValue.(string), f.Usage)
	case "stringSlice":
		flagSet.StringSliceP(flagName, f.Shorthand, f.DefaultValue.([]string), f.Usage)
	case "duration":
		flagSet.DurationP(flagName, f.Shorthand, f.DefaultValue.(time.Duration), f.Usage)
	}

	if f.NoOptDefault != "" {
		flag := flagSet.Lookup(flagName)
		flag.NoOptDefVal = f.NoOptDefault
	}
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

// BindFlags binds all command flags to Viper with an optional namespace prefix.
// For example, namespace "server" makes flag "port" accessible as "server.port"
// and via env var PH_SERVER_PORT (assuming PH_ prefix is set).
func BindFlags(cmd *cobra.Command, namespace string) {
	bindFlagSet := func(fs *pflag.FlagSet) {
		fs.VisitAll(func(f *pflag.Flag) {
			key := f.Name
			if namespace != "" {
				key = namespace + "." + f.Name
			}
			_ = viper.BindPFlag(key, f)
		})
	}

	bindFlagSet(cmd.Flags())
	bindFlagSet(cmd.PersistentFlags())
}

// Common flag definitions that can be reused across commands

// ConfigFlags returns flags for configuration file settings
func ConfigFlags() FlagValues {
	return FlagValues{
		"config-path": {
			Kind:         "stringSlice",
			DefaultValue: []string{"/config", "."},
			Usage:        "configuration paths",
		},
		"config-name": {
			Kind:         "string",
			DefaultValue: "platform-health",
			Usage:        "configuration name",
		},
	}
}

// ComponentFlags returns flags for component filtering
func ComponentFlags() FlagValues {
	return FlagValues{
		"component": {
			Shorthand:    "c",
			Kind:         "stringSlice",
			DefaultValue: []string{},
			Usage:        "component(s) to check",
		},
	}
}

// OutputFlags returns flags for output formatting
func OutputFlags() FlagValues {
	return FlagValues{
		"flat": {
			Shorthand:    "f",
			Kind:         "bool",
			DefaultValue: false,
			Usage:        "flat output",
		},
		"quiet": {
			Shorthand:    "q",
			Kind:         "count",
			DefaultValue: 0,
			Usage:        "quiet output (-q: hide healthy, -qq: summary only, -qqq: exit code only)",
		},
	}
}
