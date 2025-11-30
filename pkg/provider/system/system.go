package system

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	"github.com/isometry/platform-health/pkg/phctx"
	ph "github.com/isometry/platform-health/pkg/platform_health"
	"github.com/isometry/platform-health/pkg/provider"
)

const ProviderKind = "system"

var _ provider.Container = (*Component)(nil)

type Component struct {
	provider.Base
	provider.BaseContainer
}

func init() {
	provider.Register(ProviderKind, new(Component))
}

func (c *Component) LogValue() slog.Value {
	logAttr := []slog.Attr{
		slog.String("name", c.GetName()),
		slog.Int("components", len(c.GetComponents())),
	}
	return slog.GroupValue(logAttr...)
}

func (c *Component) Setup() error {
	return c.ResolveComponents()
}

func (c *Component) GetKind() string {
	return ProviderKind
}

func (c *Component) GetHealth(ctx context.Context) *ph.HealthCheckResponse {
	log := phctx.Logger(ctx, slog.String("provider", ProviderKind), slog.Any("instance", c))
	log.Debug("checking")

	component := &ph.HealthCheckResponse{
		Kind: ProviderKind,
		Name: c.GetName(),
	}
	defer component.LogStatus(log)

	// Check for component filtering from context
	componentPaths := phctx.ComponentPathsFromContext(ctx)
	subComponents := c.GetComponents()
	var invalidComponents []string

	if len(componentPaths) > 0 {
		subComponents, invalidComponents = c.filterChildren(componentPaths)
		// Clear component paths from context - they've been consumed at this level
		ctx = phctx.ContextWithComponentPaths(ctx, nil)
	}

	// Return error for invalid components
	if len(invalidComponents) > 0 {
		component.Status = ph.Status_UNHEALTHY
		component.Messages = append(component.Messages, fmt.Sprintf("invalid components: %s", strings.Join(invalidComponents, ", ")))
		return component
	}

	// Check sub-components (filtered or all)
	componentResults, aggregateStatus := provider.Check(ctx, subComponents)
	component.Status = aggregateStatus
	component.Components = componentResults

	// Fail-fast triggered if enabled and something failed
	if phctx.FailFastFromContext(ctx) && aggregateStatus > ph.Status_HEALTHY {
		component.FailFastTriggered = true
		component.Messages = append(component.Messages, "Results may be incomplete due to fail-fast mode")
	}

	return component
}

// filterChildren filters resolved children based on component paths
// Returns the filtered children and any invalid component names
func (c *Component) filterChildren(paths phctx.ComponentPaths) ([]provider.Instance, []string) {
	// Build map for quick lookup
	childMap := make(map[string]provider.Instance)
	for _, child := range c.GetComponents() {
		childMap[child.GetName()] = child
	}

	// Track which children to include and their sub-components
	type matchedChild struct {
		instance      provider.Instance
		subComponents phctx.ComponentPaths
	}
	matched := make(map[string]*matchedChild)
	var invalidComponents []string

	for _, path := range paths {
		if len(path) == 0 {
			continue
		}
		childName := path[0]

		child, ok := childMap[childName]
		if !ok {
			invalidComponents = append(invalidComponents, strings.Join(path, "/"))
			continue
		}

		if _, exists := matched[childName]; !exists {
			matched[childName] = &matchedChild{instance: child}
		}

		// If there are deeper components, track them
		if len(path) > 1 {
			matched[childName].subComponents = append(matched[childName].subComponents, path[1:])
		}
	}

	// Build result slice
	result := make([]provider.Instance, 0, len(matched))
	for _, mc := range matched {
		if len(mc.subComponents) > 0 {
			// Wrap child with component filtering for nested systems
			result = append(result, &filteredChild{
				Instance: mc.instance,
				subPaths: mc.subComponents,
			})
		} else {
			result = append(result, mc.instance)
		}
	}

	return result, invalidComponents
}

// filteredChild wraps a provider.Instance to pass sub-component paths via context
type filteredChild struct {
	provider.Instance
	subPaths phctx.ComponentPaths
}

func (f *filteredChild) GetHealth(ctx context.Context) *ph.HealthCheckResponse {
	ctx = phctx.ContextWithComponentPaths(ctx, f.subPaths)
	return f.Instance.GetHealth(ctx)
}
