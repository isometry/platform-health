package system

import (
	"context"
	"log/slog"

	ph "github.com/isometry/platform-health/pkg/platform_health"
	"github.com/isometry/platform-health/pkg/provider"
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

	// Check all child components
	childResults, aggregateStatus := provider.Check(ctx, s.resolved)
	component.Status = aggregateStatus
	component.Components = childResults

	return component
}
