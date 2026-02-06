package output

import (
	"os"

	"github.com/spf13/viper"
	"golang.org/x/term"

	"github.com/isometry/platform-health/internal/cliflags"
)

// Config holds configuration for formatting health check output.
type Config struct {
	Format     string // output format (json, junit, etc.)
	Flat       bool
	Quiet      int
	Compact    bool
	Colorize   bool           // colorize output (for supported formats)
	Colors     ResolvedColors // resolved ANSI color codes
	Components []string       // requested components for filtering flat output
}

// ConfigFromViper creates a Config from viper settings.
// This is the standard way to build Config for commands that use
// the common output flags (flat, quiet, compact, component, output-format).
func ConfigFromViper(v *viper.Viper) Config {
	format := v.GetString("output-format")
	if format == "" {
		format = cliflags.DefaultFormat
	}

	// Load color configuration with defaults.
	// Error is intentionally ignored: invalid color config falls back to defaults.
	colorCfg := DefaultColorConfig()
	_ = v.UnmarshalKey("colors", &colorCfg)

	return Config{
		Format:     format,
		Flat:       v.GetBool("flat"),
		Quiet:      v.GetInt("quiet"),
		Compact:    v.GetBool("compact"),
		Colorize:   shouldColorize(v.GetString("color")),
		Colors:     colorCfg.Resolve(),
		Components: v.GetStringSlice("component"),
	}
}

// shouldColorize determines whether to colorize output based on the color flag value.
func shouldColorize(colorFlag string) bool {
	switch colorFlag {
	case "always":
		return true
	case "never":
		return false
	default: // "auto"
		return term.IsTerminal(int(os.Stdout.Fd()))
	}
}
