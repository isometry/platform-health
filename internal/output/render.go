package output

import (
	"fmt"
	"strings"

	ph "github.com/isometry/platform-health/pkg/platform_health"
)

// FormatAndPrint handles common output formatting for health check responses.
func FormatAndPrint(status *ph.HealthCheckResponse, cfg Config) error {
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

// filterToRequested filters flattened components to only show explicitly requested ones.
func filterToRequested(components []*ph.HealthCheckResponse, requested []string) []*ph.HealthCheckResponse {
	var filtered []*ph.HealthCheckResponse
	for _, c := range components {
		if isRequestedComponent(c.Name, requested) {
			filtered = append(filtered, c)
		}
	}
	return filtered
}

// isRequestedComponent checks if a component name matches any requested path.
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

// filterUnhealthy recursively filters components to keep only non-HEALTHY ones.
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
