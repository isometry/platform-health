package system

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	ph "github.com/isometry/platform-health/pkg/platform_health"
	"github.com/isometry/platform-health/pkg/provider"
	"github.com/isometry/platform-health/pkg/server"
	"github.com/isometry/platform-health/pkg/utils"
)

const ProviderType = "system"

type Component struct {
	Name string `mapstructure:"-"`
	// Components holds sub-components' provider configs: map[instanceName]configWithType
	Components map[string]any `mapstructure:"components"`
	// resolved holds the concrete provider instances after Setup()
	resolved []provider.Instance
}

func init() {
	provider.Register(ProviderType, new(Component))
}

func (c *Component) LogValue() slog.Value {
	logAttr := []slog.Attr{
		slog.String("name", c.Name),
		slog.Int("components", len(c.resolved)),
	}
	return slog.GroupValue(logAttr...)
}

func (c *Component) Setup() error {
	// Resolve all sub-components
	c.resolved = make([]provider.Instance, 0)

	for instanceName, instanceConfig := range c.Components {
		// Convert instance config to map
		configMap, ok := instanceConfig.(map[string]any)
		if !ok {
			continue
		}

		// Extract provider type from config
		providerType, ok := configMap["type"].(string)
		if !ok {
			continue
		}

		// Look up provider type in registry
		registeredType, ok := provider.Providers[providerType]
		if !ok {
			continue // Unknown provider type, skip
		}

		instance, err := provider.NewInstanceFromConfig(registeredType, instanceName, configMap)
		if err != nil {
			continue
		}

		c.resolved = append(c.resolved, instance)
	}

	return nil
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

// GetResolved returns the resolved sub-components.
func (c *Component) GetResolved() []provider.Instance {
	return c.resolved
}

func (c *Component) GetHealth(ctx context.Context) *ph.HealthCheckResponse {
	log := utils.ContextLogger(ctx, slog.String("provider", ProviderType), slog.Any("instance", c))
	log.Debug("checking")

	component := &ph.HealthCheckResponse{
		Type: ProviderType,
		Name: c.Name,
	}
	defer component.LogStatus(log)

	// Check for component filtering from context
	componentPaths := server.ComponentPathsFromContext(ctx)
	subComponents := c.resolved
	var invalidComponents []string

	if len(componentPaths) > 0 {
		subComponents, invalidComponents = c.filterChildren(componentPaths)
		// Clear component paths from context - they've been consumed at this level
		ctx = server.ContextWithComponentPaths(ctx, nil)
	}

	// Return error for invalid components
	if len(invalidComponents) > 0 {
		component.Status = ph.Status_UNHEALTHY
		component.Message = fmt.Sprintf("invalid components: %s", strings.Join(invalidComponents, ", "))
		return component
	}

	// Check sub-components (filtered or all)
	componentResults, aggregateStatus := provider.Check(ctx, subComponents)
	component.Status = aggregateStatus
	component.Components = componentResults

	return component
}

// filterChildren filters resolved children based on component paths
// Returns the filtered children and any invalid component names
func (c *Component) filterChildren(paths server.ComponentPaths) ([]provider.Instance, []string) {
	// Build map for quick lookup
	childMap := make(map[string]provider.Instance)
	for _, child := range c.resolved {
		childMap[child.GetName()] = child
	}

	// Track which children to include and their sub-components
	type matchedChild struct {
		instance      provider.Instance
		subComponents server.ComponentPaths
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
	subPaths server.ComponentPaths
}

func (f *filteredChild) GetHealth(ctx context.Context) *ph.HealthCheckResponse {
	ctx = server.ContextWithComponentPaths(ctx, f.subPaths)
	return f.Instance.GetHealth(ctx)
}
