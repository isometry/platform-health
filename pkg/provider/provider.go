package provider

import (
	"context"
	"time"

	"golang.org/x/sync/errgroup"
	"google.golang.org/protobuf/types/known/durationpb"

	"github.com/isometry/platform-health/pkg/phctx"
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
	failFast := phctx.FailFastFromContext(ctx)
	limit := phctx.ParallelismLimit(phctx.ParallelismFromContext(ctx))

	instanceChan := make(chan *ph.HealthCheckResponse, len(instances))

	g, gctx := errgroup.WithContext(ctx)
	if limit > 0 {
		g.SetLimit(limit)
	}
	// if limit < 0, don't call SetLimit (unlimited)

	for _, instance := range instances {
		g.Go(func() error {
			result := GetHealthWithDuration(gctx, instance)
			instanceChan <- result
			if failFast && result.Status > ph.Status_HEALTHY {
				return context.Canceled
			}
			return nil
		})
	}

	go func() {
		_ = g.Wait() // error is context.Canceled on fail-fast; results collected via channel
		close(instanceChan)
	}()

	response = make([]*ph.HealthCheckResponse, 0, len(instances))
	status = ph.Status_HEALTHY
	for result := range instanceChan {
		response = append(response, result)
		if result.Status.Number() > status.Number() {
			status = result.Status
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
