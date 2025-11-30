package flags

import "github.com/mgutz/ansi"

// StatusColors holds color names for each status value.
type StatusColors struct {
	Healthy   string `mapstructure:"healthy"`
	Unhealthy string `mapstructure:"unhealthy"`
	Unknown   string `mapstructure:"unknown"`
	Loop      string `mapstructure:"loop"`
}

// ColorConfig holds semantic color names for YAML output colorization.
// Color names follow mgutz/ansi format: "red", "green+b" (bold), "white+d" (dim), etc.
type ColorConfig struct {
	Status   StatusColors `mapstructure:"status"`
	Name     string       `mapstructure:"name"`
	Kind     string       `mapstructure:"kind"`
	Message  string       `mapstructure:"message"`
	Duration string       `mapstructure:"duration"`
}

// ResolvedColors holds pre-computed ANSI escape codes.
type ResolvedColors struct {
	Reset           string
	StatusHealthy   string
	StatusUnhealthy string
	StatusUnknown   string
	StatusLoop      string
	Name            string
	Kind            string
	Message         string
	Duration        string
}

// DefaultColorConfig returns the default color configuration.
func DefaultColorConfig() ColorConfig {
	return ColorConfig{
		Status: StatusColors{
			Healthy:   "green",
			Unhealthy: "red",
			Unknown:   "yellow",
			Loop:      "yellow",
		},
		Name:     "cyan",
		Kind:     "blue",
		Message:  "white+d",
		Duration: "magenta",
	}
}

// Resolve converts semantic color names to ANSI escape codes.
func (c ColorConfig) Resolve() ResolvedColors {
	return ResolvedColors{
		Reset:           ansi.ColorCode("reset"),
		StatusHealthy:   ansi.ColorCode(c.Status.Healthy),
		StatusUnhealthy: ansi.ColorCode(c.Status.Unhealthy),
		StatusUnknown:   ansi.ColorCode(c.Status.Unknown),
		StatusLoop:      ansi.ColorCode(c.Status.Loop),
		Name:            ansi.ColorCode(c.Name),
		Kind:            ansi.ColorCode(c.Kind),
		Message:         ansi.ColorCode(c.Message),
		Duration:        ansi.ColorCode(c.Duration),
	}
}
