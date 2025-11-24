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

const TypeMock = "mock"

var _ provider.InstanceWithChecks = (*Mock)(nil)

var celConfig = checks.NewCEL(
	cel.Variable("mock", cel.MapType(cel.StringType, cel.DynType)),
)

type Mock struct {
	provider.BaseWithChecks `mapstructure:",squash"`

	Name   string        `mapstructure:"-"`
	Health ph.Status     `mapstructure:"health" default:"HEALTHY"`
	Sleep  time.Duration `mapstructure:"sleep" default:"1ns"`
}

func init() {
	provider.Register(TypeMock, new(Mock))
}

func (i *Mock) Setup() error {
	defaults.SetDefaults(i)

	return i.SetupChecks(celConfig)
}

func (i *Mock) GetType() string {
	return TypeMock
}

func (i *Mock) GetName() string {
	return i.Name
}

func (i *Mock) SetName(name string) {
	i.Name = name
}

func (i *Mock) GetHealth(ctx context.Context) *ph.HealthCheckResponse {
	// simulate a delay
	time.Sleep(i.Sleep)

	component := &ph.HealthCheckResponse{
		Type:   TypeMock,
		Name:   i.Name,
		Status: i.Health,
	}

	// Evaluate CEL checks if configured
	checkCtx, err := i.GetCheckContext(ctx)
	if err != nil {
		component.Status = ph.Status_UNHEALTHY
		component.Message = err.Error()
		return component
	}

	if err := i.EvaluateChecks(checkCtx); err != nil {
		component.Status = ph.Status_UNHEALTHY
		component.Message = err.Error()
	}

	return component
}

// InstanceWithChecks implementation

func (i *Mock) GetCheckConfig() *checks.CEL {
	return celConfig
}

func (i *Mock) GetCheckContext(ctx context.Context) (map[string]any, error) {
	return map[string]any{
		"mock": map[string]any{
			"health": i.Health.String(),
			"sleep":  i.Sleep.String(),
		},
	}, nil
}
