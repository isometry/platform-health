package mock

import (
	"time"

	ph "github.com/isometry/platform-health/pkg/platform_health"
)

// Option configures a mock Component.
type Option func(*Component)

// WithSleep sets the sleep duration for the mock.
func WithSleep(d time.Duration) Option {
	return func(c *Component) { c.Sleep = d }
}

// WithHealth sets the health status for the mock.
func WithHealth(s ph.Status) Option {
	return func(c *Component) { c.Health = s }
}

// New creates a mock component with the given name and options.
// Defaults to HEALTHY status if not specified via WithHealth.
func New(name string, opts ...Option) *Component {
	c := &Component{Health: ph.Status_HEALTHY}
	c.SetName(name)
	for _, opt := range opts {
		opt(c)
	}
	return c
}

// Healthy creates a healthy mock with the given name and options.
func Healthy(name string, opts ...Option) *Component {
	return New(name, opts...)
}

// Unhealthy creates an unhealthy mock with the given name and options.
func Unhealthy(name string, opts ...Option) *Component {
	return New(name, append([]Option{WithHealth(ph.Status_UNHEALTHY)}, opts...)...)
}
