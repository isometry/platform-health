package flags

import (
	"fmt"
	"net"
	"strconv"
	"strings"

	"github.com/spf13/viper"
	"google.golang.org/protobuf/encoding/protojson"

	ph "github.com/isometry/platform-health/pkg/platform_health"
)

// OutputConfig holds configuration for formatting health check output
type OutputConfig struct {
	Flat       bool
	Quiet      int
	Compact    bool
	Components []string // requested components for filtering flat output
}

// OutputConfigFromViper creates an OutputConfig from viper settings.
// This is the standard way to build OutputConfig for commands that use
// the common output flags (flat, quiet, compact, component).
func OutputConfigFromViper() OutputConfig {
	return OutputConfig{
		Flat:       viper.GetBool("flat"),
		Quiet:      viper.GetInt("quiet"),
		Compact:    viper.GetBool("compact"),
		Components: viper.GetStringSlice("component"),
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
		status.Components = status.Flatten(status.Name)
		if len(cfg.Components) > 0 {
			status.Components = filterToRequested(status.Components, cfg.Components)
		}
	}

	opts := protojson.MarshalOptions{}
	if !cfg.Compact {
		opts.Multiline = true
		opts.Indent = "  "
	}
	pjson, err := opts.Marshal(status)
	if err != nil {
		return err
	}

	fmt.Println(string(pjson))

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
