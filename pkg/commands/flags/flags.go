package flags

import (
	"log/slog"
	"maps"
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
		defaultVal := false
		if f.DefaultValue != nil {
			defaultVal = f.DefaultValue.(bool)
		}
		flagSet.BoolP(flagName, f.Shorthand, defaultVal, f.Usage)
	case "count":
		flagSet.CountP(flagName, f.Shorthand, f.Usage)
	case "int":
		defaultVal := 0
		if f.DefaultValue != nil {
			defaultVal = f.DefaultValue.(int)
		}
		flagSet.IntP(flagName, f.Shorthand, defaultVal, f.Usage)
	case "string":
		defaultVal := ""
		if f.DefaultValue != nil {
			defaultVal = f.DefaultValue.(string)
		}
		flagSet.StringP(flagName, f.Shorthand, defaultVal, f.Usage)
	case "stringSlice":
		var defaultVal []string
		if f.DefaultValue != nil {
			defaultVal = f.DefaultValue.([]string)
		}
		flagSet.StringSliceP(flagName, f.Shorthand, defaultVal, f.Usage)
	case "intSlice":
		var defaultVal []int
		if f.DefaultValue != nil {
			defaultVal = f.DefaultValue.([]int)
		}
		flagSet.IntSliceP(flagName, f.Shorthand, defaultVal, f.Usage)
	case "duration":
		var defaultVal time.Duration
		if f.DefaultValue != nil {
			switch v := f.DefaultValue.(type) {
			case time.Duration:
				defaultVal = v
			case string:
				var err error
				defaultVal, err = time.ParseDuration(v)
				if err != nil {
					slog.Warn("invalid duration default value", "flag", flagName, "value", v, "error", err)
				}
			}
		}
		flagSet.DurationP(flagName, f.Shorthand, defaultVal, f.Usage)
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
		maps.Copy(result, fs)
	}
	return result
}

// BindFlags binds all command flags to Viper without namespace prefix.
// This includes local flags and inherited persistent flags from parent commands.
// All flags are accessible directly by name (e.g., viper.GetBool("flat")).
func BindFlags(cmd *cobra.Command) {
	cmd.Flags().VisitAll(func(f *pflag.Flag) {
		_ = viper.BindPFlag(f.Name, f)
	})
}

// ConfigPaths returns the config-path and config-name values from viper.
// Use this to call config.Load with consistent settings.
func ConfigPaths() (paths []string, name string) {
	return viper.GetStringSlice("config-path"), viper.GetString("config-name")
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
		"compact": {
			Kind:         "bool",
			DefaultValue: false,
			Usage:        "compact JSON output",
		},
	}
}
