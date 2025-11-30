package flags

import (
	"log/slog"
	"maps"
	"runtime"
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

// BindFlags binds all command flags to the given viper instance.
// This includes local flags and inherited persistent flags from parent commands.
// All flags are accessible directly by name (e.g., v.GetBool("flat")).
func BindFlags(cmd *cobra.Command, v *viper.Viper) {
	cmd.Flags().VisitAll(func(f *pflag.Flag) {
		_ = v.BindPFlag(f.Name, f)
	})
}

// ConfigPaths returns the config-path and config-name values from the given viper.
// Use this to call config.Load with consistent settings.
func ConfigPaths(v *viper.Viper) (paths []string, name string) {
	return v.GetStringSlice("config-path"), v.GetString("config-name")
}

// Common flag definitions that can be reused across commands

// ConfigFlags returns flags for configuration file settings
func ConfigFlags() FlagValues {
	return FlagValues{
		"config-path": {
			Kind:         "stringSlice",
			DefaultValue: []string{".", "/config"},
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
		"output-format": {
			Shorthand:    "o",
			Kind:         "string",
			DefaultValue: DefaultFormat,
			Usage:        "output format (json, junit, yaml)",
		},
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
		"color": {
			Kind:         "string",
			DefaultValue: "auto",
			Usage:        "colorize output: auto, always, never",
		},
	}
}

// FailFastFlags returns flags for fail-fast behavior
func FailFastFlags() FlagValues {
	return FlagValues{
		"fail-fast": {
			Shorthand:    "F",
			Kind:         "bool",
			DefaultValue: false,
			Usage:        "cancel remaining checks after first failure",
		},
	}
}

// ParallelismFlags returns flags for parallelism control
func ParallelismFlags() FlagValues {
	return FlagValues{
		"parallelism": {
			Shorthand:    "j",
			Kind:         "int",
			DefaultValue: runtime.GOMAXPROCS(0),
			Usage:        "max concurrent health checks (0 = GOMAXPROCS, -1 = unlimited)",
		},
	}
}

// TimeoutFlags returns flags for timeout control
func TimeoutFlags() FlagValues {
	return FlagValues{
		"timeout": {
			Shorthand:    "t",
			Kind:         "duration",
			DefaultValue: 10 * time.Second,
			Usage:        "timeout for health check operations",
		},
	}
}
