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

const TypeSystem = "system"

type System struct {
	Name string `mapstructure:"-"`
	// Components holds child provider configs: map[instanceName]configWithType
	Components map[string]any `mapstructure:"components"`
	// resolved holds the concrete provider instances after Setup()
	resolved []provider.Instance
}

func init() {
	provider.Register(TypeSystem, new(System))
}

func (s *System) LogValue() slog.Value {
	logAttr := []slog.Attr{
		slog.String("name", s.Name),
		slog.Int("components", len(s.resolved)),
	}
	return slog.GroupValue(logAttr...)
}

func (s *System) Setup() error {
	// Resolve all child components
	s.resolved = make([]provider.Instance, 0)

	for instanceName, instanceConfig := range s.Components {
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

		s.resolved = append(s.resolved, instance)
	}

	return nil
}

func (s *System) GetType() string {
	return TypeSystem
}

func (s *System) GetName() string {
	return s.Name
}

func (s *System) SetName(name string) {
	s.Name = name
}

func (s *System) GetHealth(ctx context.Context) *ph.HealthCheckResponse {
	log := utils.ContextLogger(ctx, slog.String("provider", TypeSystem), slog.Any("instance", s))
	log.Debug("checking")

	component := &ph.HealthCheckResponse{
		Type: TypeSystem,
		Name: s.Name,
	}
	defer component.LogStatus(log)

	// Check for component filtering from context
	componentPaths := server.ComponentPathsFromContext(ctx)
	children := s.resolved
	var invalidComponents []string

	if len(componentPaths) > 0 {
		children, invalidComponents = s.filterChildren(componentPaths)
		// Clear component paths from context - they've been consumed at this level
		ctx = server.ContextWithComponentPaths(ctx, nil)
	}

	// Return error for invalid components
	if len(invalidComponents) > 0 {
		component.Status = ph.Status_UNHEALTHY
		component.Message = fmt.Sprintf("invalid components: %s", strings.Join(invalidComponents, ", "))
		return component
	}

	// Check child components (filtered or all)
	childResults, aggregateStatus := provider.Check(ctx, children)
	component.Status = aggregateStatus
	component.Components = childResults

	return component
}

// filterChildren filters resolved children based on component paths
// Returns the filtered children and any invalid component names
func (s *System) filterChildren(paths server.ComponentPaths) ([]provider.Instance, []string) {
	// Build map for quick lookup
	childMap := make(map[string]provider.Instance)
	for _, child := range s.resolved {
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
