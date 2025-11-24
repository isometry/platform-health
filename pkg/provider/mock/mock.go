package mock

import (
	"context"
	"time"

	"github.com/google/cel-go/cel"
	"github.com/mcuadros/go-defaults"

	"github.com/isometry/platform-health/pkg/checks"
	ph "github.com/isometry/platform-health/pkg/platform_health"
	"github.com/isometry/platform-health/pkg/provider"
)

const ProviderType = "mock"

var _ provider.InstanceWithChecks = (*Component)(nil)

var celConfig = checks.NewCEL(
	cel.Variable("mock", cel.MapType(cel.StringType, cel.DynType)),
)

type Component struct {
	provider.BaseWithChecks `mapstructure:",squash"`

	Name   string        `mapstructure:"-"`
	Health ph.Status     `mapstructure:"health" default:"HEALTHY"`
	Sleep  time.Duration `mapstructure:"sleep" default:"1ns"`
}

func init() {
	provider.Register(ProviderType, new(Component))
}

func (c *Component) Setup() error {
	defaults.SetDefaults(c)

	return c.SetupChecks(celConfig)
}

func (c *Component) GetType() string {
	return ProviderType
}

func (c *Component) GetName() string {
	return c.Name
}

func (c *Component) SetName(name string) {
	c.Name = name
}

func (c *Component) GetHealth(ctx context.Context) *ph.HealthCheckResponse {
	// simulate a delay, respecting context cancellation
	select {
	case <-time.After(c.Sleep):
		// normal completion
	case <-ctx.Done():
		return &ph.HealthCheckResponse{
			Type:    ProviderType,
			Name:    c.Name,
			Status:  ph.Status_UNHEALTHY,
			Message: ctx.Err().Error(),
		}
	}

	component := &ph.HealthCheckResponse{
		Type:   ProviderType,
		Name:   c.Name,
		Status: c.Health,
	}

	// Evaluate CEL checks if configured
	checkCtx, err := c.GetCheckContext(ctx)
	if err != nil {
		component.Status = ph.Status_UNHEALTHY
		component.Message = err.Error()
		return component
	}

	if err := c.EvaluateChecks(checkCtx); err != nil {
		component.Status = ph.Status_UNHEALTHY
		component.Message = err.Error()
	}

	return component
}

// InstanceWithChecks implementation

func (c *Component) GetCheckConfig() *checks.CEL {
	return celConfig
}

func (c *Component) GetCheckContext(ctx context.Context) (map[string]any, error) {
	return map[string]any{
		"mock": map[string]any{
			"health": c.Health.String(),
			"sleep":  c.Sleep.String(),
		},
	}, nil
}
