package flags

import (
	"fmt"
	"net"
	"os"
	"strconv"
	"strings"

	"github.com/spf13/viper"
	"golang.org/x/term"

	ph "github.com/isometry/platform-health/pkg/platform_health"
)

// OutputConfig holds configuration for formatting health check output
type OutputConfig struct {
	Format     string // output format (json, junit, etc.)
	Flat       bool
	Quiet      int
	Compact    bool
	Colorize   bool           // colorize output (for supported formats)
	Colors     ResolvedColors // resolved ANSI color codes
	Components []string       // requested components for filtering flat output
}

// OutputConfigFromViper creates an OutputConfig from viper settings.
// This is the standard way to build OutputConfig for commands that use
// the common output flags (flat, quiet, compact, component, output-format).
func OutputConfigFromViper(v *viper.Viper) OutputConfig {
	format := v.GetString("output-format")
	if format == "" {
		format = DefaultFormat
	}

	// Load color configuration with defaults
	colorCfg := DefaultColorConfig()
	_ = v.UnmarshalKey("colors", &colorCfg)

	return OutputConfig{
		Format:     format,
		Flat:       v.GetBool("flat"),
		Quiet:      v.GetInt("quiet"),
		Compact:    v.GetBool("compact"),
		Colorize:   shouldColorize(v.GetString("color")),
		Colors:     colorCfg.Resolve(),
		Components: v.GetStringSlice("component"),
	}
}

// shouldColorize determines whether to colorize output based on the color flag value
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

// FormatAndPrintStatus handles common output formatting for health check responses
func FormatAndPrintStatus(status *ph.HealthCheckResponse, cfg OutputConfig) error {
	switch {
	case cfg.Quiet > 2:
		return status.IsHealthy()
	case cfg.Quiet > 1:
		status.Components = nil
	case cfg.Quiet > 0:
		status.Components = filterUnhealthy(status.Components)
	}

	if cfg.Flat {
		status.Components = status.Flatten(status.Name, status.Type)
		if len(cfg.Components) > 0 {
			status.Components = filterToRequested(status.Components, cfg.Components)
		}
	}

	formatter, ok := GetFormatter(cfg.Format)
	if !ok {
		return fmt.Errorf("unknown output format %q (available: %s)", cfg.Format, strings.Join(FormatNames(), ", "))
	}

	output, err := formatter.Format(status, cfg)
	if err != nil {
		return err
	}

	fmt.Println(string(output))

	return status.IsHealthy()
}

// filterToRequested filters flattened components to only show explicitly requested ones
func filterToRequested(components []*ph.HealthCheckResponse, requested []string) []*ph.HealthCheckResponse {
	var filtered []*ph.HealthCheckResponse
	for _, c := range components {
		if isRequestedComponent(c.Name, requested) {
			filtered = append(filtered, c)
		}
	}
	return filtered
}

// isRequestedComponent checks if a component name matches any requested path
func isRequestedComponent(name string, requested []string) bool {
	for _, req := range requested {
		// Exact match
		if name == req {
			return true
		}
		// Component is a child of requested (e.g., "foo/bar" matches request for "foo")
		if strings.HasPrefix(name, req+"/") {
			return true
		}
	}
	return false
}

// ParseHostPort parses a host:port string into separate host and port values
func ParseHostPort(arg string) (host string, port int, err error) {
	var portStr string
	host, portStr, err = net.SplitHostPort(arg)
	if err != nil {
		return "", 0, err
	}
	port, err = strconv.Atoi(portStr)
	if err != nil {
		return "", 0, err
	}
	return host, port, nil
}

// filterUnhealthy recursively filters components to keep only non-HEALTHY ones
func filterUnhealthy(components []*ph.HealthCheckResponse) []*ph.HealthCheckResponse {
	var filtered []*ph.HealthCheckResponse
	for _, c := range components {
		if c.Status != ph.Status_HEALTHY {
			c.Components = filterUnhealthy(c.Components)
			filtered = append(filtered, c)
		}
	}
	return filtered
}
