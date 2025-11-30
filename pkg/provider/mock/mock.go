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

const ProviderKind = "mock"

var _ provider.InstanceWithChecks = (*Component)(nil)

var celConfig = checks.NewCEL(
	cel.Variable("mock", cel.MapType(cel.StringType, cel.DynType)),
)

type Component struct {
	provider.Base
	provider.BaseWithChecks

	InstanceName string        // Instance name (exported for test struct literals)
	Health       ph.Status     `mapstructure:"health" default:"HEALTHY"`
	Sleep        time.Duration `mapstructure:"sleep" default:"1ns"`
}

func init() {
	provider.Register(ProviderKind, new(Component))
}

func (c *Component) Setup() error {
	defaults.SetDefaults(c)
	return nil
}

// SetChecks sets and compiles CEL expressions.
func (c *Component) SetChecks(exprs []checks.Expression) error {
	return c.SetChecksAndCompile(exprs, celConfig)
}

func (c *Component) GetKind() string {
	return ProviderKind
}

func (c *Component) GetName() string {
	return c.InstanceName
}

func (c *Component) SetName(name string) {
	c.InstanceName = name
}

func (c *Component) GetHealth(ctx context.Context) *ph.HealthCheckResponse {
	// simulate a delay, respecting context cancellation
	select {
	case <-time.After(c.Sleep):
		// normal completion
	case <-ctx.Done():
		return &ph.HealthCheckResponse{
			Kind:     ProviderKind,
			Name:     c.GetName(),
			Status:   ph.Status_UNHEALTHY,
			Messages: []string{ctx.Err().Error()},
		}
	}

	component := &ph.HealthCheckResponse{
		Kind:   ProviderKind,
		Name:   c.GetName(),
		Status: c.Health,
	}

	// Evaluate CEL checks if configured
	checkCtx, err := c.GetCheckContext(ctx)
	if err != nil {
		component.Status = ph.Status_UNHEALTHY
		component.Messages = append(component.Messages, err.Error())
		return component
	}

	if msgs := c.EvaluateChecks(ctx, checkCtx); len(msgs) > 0 {
		component.Status = ph.Status_UNHEALTHY
		component.Messages = append(component.Messages, msgs...)
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
