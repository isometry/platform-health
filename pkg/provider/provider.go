package provider

import (
	"context"
	"sync"
	"time"

	"google.golang.org/protobuf/types/known/durationpb"

	ph "github.com/isometry/platform-health/pkg/platform_health"
)

// Instance is the interface that must be implemented by all providers.
type Instance interface {
	// GetType returns the provider type of the instance
	GetType() string
	// GetName returns the name of the instance
	GetName() string
	// SetName sets the name of the instance
	SetName(string)
	// GetHealth checks and returns the instance
	GetHealth(context.Context) *ph.HealthCheckResponse
	// Setup sets the default values for the instance and validates specification.
	// Returns an error if the specification is invalid.
	Setup() error
}

// Config is the interface through which the provider configuration is retrieved.
type Config interface {
	GetInstances() []Instance
}

func Check(ctx context.Context, instances []Instance) (response []*ph.HealthCheckResponse, status ph.Status) {
	var wg sync.WaitGroup
	instanceChan := make(chan *ph.HealthCheckResponse, len(instances))

	for _, instance := range instances {
		wg.Go(func() {
			instanceChan <- GetHealthWithDuration(ctx, instance)
		})
	}

	go func() {
		wg.Wait()
		close(instanceChan)
	}()

	response = make([]*ph.HealthCheckResponse, 0, len(instances))
	status = ph.Status_HEALTHY
	for instance := range instanceChan {
		response = append(response, instance)

		if instance.Status.Number() > status.Number() {
			status = instance.Status
		}
	}

	return response, status
}

func GetHealthWithDuration(ctx context.Context, instance Instance) *ph.HealthCheckResponse {
	start := time.Now()
	response := instance.GetHealth(ctx)
	if response != nil {
		response.Duration = durationpb.New(time.Since(start))
	}
	return response
}
