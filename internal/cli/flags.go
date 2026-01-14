package cli

import (
	"maps"
	"runtime"
	"time"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"github.com/spf13/viper"

	"github.com/isometry/platform-health/pkg/provider"
)

// Merge combines multiple FlagValues maps into one
func Merge(flagSets ...provider.FlagValues) provider.FlagValues {
	result := make(provider.FlagValues)
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
func ConfigFlags() provider.FlagValues {
	return provider.FlagValues{
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
func ComponentFlags() provider.FlagValues {
	return provider.FlagValues{
		"component": {
			Shorthand:    "c",
			Kind:         "stringSlice",
			DefaultValue: []string{},
			Usage:        "component(s) to check",
		},
	}
}

// OutputFlags returns flags for output formatting
func OutputFlags() provider.FlagValues {
	return provider.FlagValues{
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
func FailFastFlags() provider.FlagValues {
	return provider.FlagValues{
		"fail-fast": {
			Shorthand:    "F",
			Kind:         "bool",
			DefaultValue: false,
			Usage:        "cancel remaining checks after first failure",
		},
	}
}

// ParallelismFlags returns flags for parallelism control
func ParallelismFlags() provider.FlagValues {
	return provider.FlagValues{
		"parallelism": {
			Shorthand:    "j",
			Kind:         "int",
			DefaultValue: runtime.GOMAXPROCS(0),
			Usage:        "max concurrent health checks (0 = GOMAXPROCS, -1 = unlimited)",
		},
	}
}

// TimeoutFlags returns flags for timeout control
func TimeoutFlags() provider.FlagValues {
	return provider.FlagValues{
		"timeout": {
			Shorthand:    "t",
			Kind:         "duration",
			DefaultValue: 10 * time.Second,
			Usage:        "timeout for health check operations",
		},
	}
}
