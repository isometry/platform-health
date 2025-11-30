package mock

import (
	ph "github.com/isometry/platform-health/pkg/platform_health"
)

// Healthy creates a healthy mock with given name
func Healthy(name string) *Component {
	return &Component{InstanceName: name, Health: ph.Status_HEALTHY}
}

// Unhealthy creates an unhealthy mock with given name
func Unhealthy(name string) *Component {
	return &Component{InstanceName: name, Health: ph.Status_UNHEALTHY}
}
