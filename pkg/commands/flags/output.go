package flags

import (
	"fmt"
	"net"
	"strconv"

	"google.golang.org/protobuf/encoding/protojson"

	ph "github.com/isometry/platform-health/pkg/platform_health"
)

// FormatAndPrintStatus handles common output formatting for health check responses
func FormatAndPrintStatus(status *ph.HealthCheckResponse, flat bool, quiet int) error {
	switch {
	case quiet > 2:
		return status.IsHealthy()
	case quiet > 1:
		status.Components = nil
	case quiet > 0:
		status.Components = filterUnhealthy(status.Components)
	}

	if flat {
		status.Components = status.Flatten(status.Name)
	}

	pjson, err := protojson.Marshal(status)
	if err != nil {
		return err
	}

	fmt.Println(string(pjson))

	return status.IsHealthy()
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
