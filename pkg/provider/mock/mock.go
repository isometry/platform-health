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
	Name   string        `mapstructure:"-"`
	Health ph.Status     `mapstructure:"health" default:"1"`
	Sleep  time.Duration `mapstructure:"sleep" default:"1ns"`
}

func init() {
	provider.Register(TypeMock, new(Mock))
}

func (i *Mock) Setup() error {
	defaults.SetDefaults(i)

	return nil
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

	return component
}
