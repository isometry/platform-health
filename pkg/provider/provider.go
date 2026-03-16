package provider

import (
	"context"
	"slices"
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
	// GetTimeout returns the per-instance timeout override (0 means use parent context deadline)
	GetTimeout() time.Duration
	// SetTimeout sets the per-instance timeout override
	SetTimeout(time.Duration)
	// GetHealth checks and returns the instance
	GetHealth(context.Context) *ph.HealthCheckResponse
	// Setup sets the default values for the instance and validates specification.
	// Returns an error if the specification is invalid.
	Setup() error
}

// Base provides common functionality for all providers.
// Providers should embed this struct to get default implementations of
// GetName, SetName, GetTimeout, SetTimeout, and order/always accessors.
type Base struct {
	name    string
	timeout time.Duration
	order   int
	always  bool
}

func (b *Base) GetName() string            { return b.name }
func (b *Base) SetName(name string)        { b.name = name }
func (b *Base) GetTimeout() time.Duration  { return b.timeout }
func (b *Base) SetTimeout(t time.Duration) { b.timeout = t }
func (b *Base) GetOrder() int              { return b.order }
func (b *Base) SetOrder(o int)             { b.order = o }
func (b *Base) GetAlways() bool            { return b.always }
func (b *Base) SetAlways(a bool)           { b.always = a }

// Config is the interface through which the provider configuration is retrieved.
type Config interface {
	GetInstances() []Instance
}

// Container is implemented by providers that contain nested components.
// This enables recursive validation and generic config handling.
type Container interface {
	// SetComponents accepts the raw components config from YAML.
	// Called during config loading, before Setup().
	SetComponents(config map[string]any)

	// GetComponents returns the resolved child component instances.
	// Available after Setup() has been called.
	GetComponents() []Instance

	// ComponentErrors returns validation errors from child component resolution.
	ComponentErrors() []error
}

// AsContainer returns the instance as Container if it implements it.
// Returns nil if the instance does not implement Container.
func AsContainer(i Instance) Container {
	if c, ok := i.(Container); ok {
		return c
	}
	return nil
}

// orderOf returns the order of an instance via type assertion, defaulting to 0.
func orderOf(i Instance) int {
	if o, ok := i.(interface{ GetOrder() int }); ok {
		return o.GetOrder()
	}
	return 0
}

// alwaysOf returns the always flag of an instance via type assertion, defaulting to false.
func alwaysOf(i Instance) bool {
	if a, ok := i.(interface{ GetAlways() bool }); ok {
		return a.GetAlways()
	}
	return false
}

// instanceGroup holds instances that share the same order value.
type instanceGroup struct {
	order     int
	instances []Instance
}

// groupByOrder partitions instances into groups sorted by order (ascending).
// Instances with the same order value are placed in the same group.
func groupByOrder(instances []Instance) []instanceGroup {
	orderMap := make(map[int][]Instance)
	for _, inst := range instances {
		o := orderOf(inst)
		orderMap[o] = append(orderMap[o], inst)
	}

	groups := make([]instanceGroup, 0, len(orderMap))
	for order, insts := range orderMap {
		groups = append(groups, instanceGroup{order: order, instances: insts})
	}
	slices.SortFunc(groups, func(a, b instanceGroup) int {
		return a.order - b.order
	})
	return groups
}

// checkGroup runs health checks for a single group of instances.
// always-flagged instances use the parent ctx (not the errgroup's derived context)
// so they survive fail-fast cancellation and don't trigger it.
func checkGroup(ctx context.Context, instances []Instance, failFast bool, limit int) ([]*ph.HealthCheckResponse, bool) {
	instanceChan := make(chan *ph.HealthCheckResponse, len(instances))

	g, gctx := errgroup.WithContext(ctx)
	if limit > 0 {
		g.SetLimit(limit)
	}

	for _, instance := range instances {
		always := alwaysOf(instance)
		g.Go(func() error {
			// always instances get parent ctx so they survive fail-fast cancellation
			checkCtx := gctx
			if always {
				checkCtx = ctx
			}
			result := GetHealthWithDuration(checkCtx, instance)
			instanceChan <- result
			if failFast && !always && result.Status > ph.Status_HEALTHY {
				return context.Canceled
			}
			return nil
		})
	}

	var failFastTriggered bool
	go func() {
		if err := g.Wait(); err != nil {
			failFastTriggered = true
		}
		close(instanceChan)
	}()

	response := make([]*ph.HealthCheckResponse, 0, len(instances))
	for result := range instanceChan {
		response = append(response, result)
	}
	return response, failFastTriggered
}

func Check(ctx context.Context, instances []Instance) (response []*ph.HealthCheckResponse, status ph.Status) {
	failFast := phctx.FailFastFromContext(ctx)
	limit := phctx.ParallelismLimit(phctx.ParallelismFromContext(ctx))

	groups := groupByOrder(instances)

	response = make([]*ph.HealthCheckResponse, 0, len(instances))
	status = ph.Status_HEALTHY
	failFastTriggered := false

	for _, group := range groups {
		var groupInstances []Instance
		if failFastTriggered {
			// After fail-fast, only run always instances
			for _, inst := range group.instances {
				if alwaysOf(inst) {
					groupInstances = append(groupInstances, inst)
				}
			}
			if len(groupInstances) == 0 {
				continue
			}
		} else {
			groupInstances = group.instances
		}

		groupResp, triggered := checkGroup(ctx, groupInstances, failFast, limit)
		for _, result := range groupResp {
			response = append(response, result)
			if result.Status.Number() > status.Number() {
				status = result.Status
			}
		}
		if triggered {
			failFastTriggered = true
		}
	}

	return response, status
}

func GetHealthWithDuration(ctx context.Context, instance Instance) *ph.HealthCheckResponse {
	// Apply per-instance timeout if configured
	if timeout := instance.GetTimeout(); timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, timeout)
		defer cancel()
	}

	start := time.Now()
	response := instance.GetHealth(ctx)
	if response != nil {
		response.Duration = durationpb.New(time.Since(start))
	}
	return response
}
