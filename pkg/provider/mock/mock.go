package mock

import (
	"context"
	"time"

	"github.com/mcuadros/go-defaults"

	ph "github.com/isometry/platform-health/pkg/platform_health"
	"github.com/isometry/platform-health/pkg/provider"
)

const TypeMock = "mock"

type Mock struct {
	Name   string        `mapstructure:"name"`
	Health ph.Status     `mapstructure:"health" default:"2"`
	Sleep  time.Duration `mapstructure:"sleep" default:"1ns"`
}

func init() {
	provider.Register(TypeMock, new(Mock))
}

func (i *Mock) SetDefaults() {
	defaults.SetDefaults(i)
}

func (i *Mock) GetType() string {
	return TypeMock
}

func (i *Mock) GetName() string {
	return i.Name
}

func (i *Mock) GetHealth(ctx context.Context) *ph.HealthCheckResponse {
	// simulate a delay
	time.Sleep(i.Sleep)

	component := &ph.HealthCheckResponse{
		Type:   i.GetType(),
		Name:   i.GetName(),
		Status: i.Health,
	}

	return component
}
