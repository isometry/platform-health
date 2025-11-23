package helm

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/cel-go/cel"
	"github.com/mcuadros/go-defaults"
	release "helm.sh/helm/v4/pkg/release/v1"

	"github.com/isometry/platform-health/pkg/checks"
	ph "github.com/isometry/platform-health/pkg/platform_health"
	"github.com/isometry/platform-health/pkg/provider"
	"github.com/isometry/platform-health/pkg/provider/helm/client"
	"github.com/isometry/platform-health/pkg/utils"
)

const TypeHelm = "helm"

// CEL configuration for Helm provider
var celConfig = checks.NewCEL(
	cel.Variable("release", cel.MapType(cel.StringType, cel.DynType)),
	cel.Variable("chart", cel.MapType(cel.StringType, cel.DynType)),
)

type Helm struct {
	Name      string              `mapstructure:"-"`
	Release   string              `mapstructure:"release"`
	Namespace string              `mapstructure:"namespace"`
	Timeout   time.Duration       `mapstructure:"timeout" default:"5s"`
	Checks    []checks.Expression `mapstructure:"checks"`

	// Compiled CEL evaluator (cached after Setup)
	evaluator *checks.Evaluator
}

func init() {
	provider.Register(TypeHelm, new(Helm))
}

func (i *Helm) LogValue() slog.Value {
	logAttr := []slog.Attr{
		slog.String("name", i.Name),
		slog.String("release", i.Release),
		slog.String("namespace", i.Namespace),
		slog.Any("timeout", i.Timeout),
		slog.Int("checks", len(i.Checks)),
	}
	return slog.GroupValue(logAttr...)
}

func (i *Helm) Setup() error {
	defaults.SetDefaults(i)

	// Pre-compile CEL evaluator if checks exist
	if len(i.Checks) > 0 {
		evaluator, err := celConfig.NewEvaluator(i.Checks)
		if err != nil {
			return fmt.Errorf("invalid CEL expression: %w", err)
		}
		i.evaluator = evaluator
	}

	return nil
}

func (i *Helm) GetType() string {
	return TypeHelm
}

func (i *Helm) GetName() string {
	return i.Name
}

func (i *Helm) SetName(name string) {
	i.Name = name
}

// releaseResult holds the result of a status check
type releaseResult struct {
	release *release.Release
	err     error
}

func (i *Helm) GetHealth(ctx context.Context) *ph.HealthCheckResponse {
	log := utils.ContextLogger(ctx, slog.String("provider", TypeHelm), slog.Any("instance", i))
	log.Debug("checking")

	component := &ph.HealthCheckResponse{
		Type: TypeHelm,
		Name: i.Name,
	}
	defer component.LogStatus(log)

	statusRunner, err := client.ClientFactory.GetStatusRunner(i.Namespace, log)
	if err != nil {
		return component.Unhealthy(err.Error())
	}

	resultChan := make(chan releaseResult)
	go func() {
		rel, err := statusRunner.Run(i.Release)
		resultChan <- releaseResult{release: rel, err: err}
	}()

	var rel *release.Release
	select {
	case <-time.After(i.Timeout):
		return component.Unhealthy("timeout")
	case result := <-resultChan:
		if result.err != nil {
			return component.Unhealthy(result.err.Error())
		}
		rel = result.release
	}

	// Check status
	if rel.Info.Status != client.StatusDeployed {
		return component.Unhealthy(fmt.Sprintf("expected status 'deployed'; actual status '%s'", rel.Info.Status))
	}

	// Apply CEL checks with release context
	if len(i.Checks) > 0 {
		if i.evaluator == nil {
			evaluator, err := celConfig.NewEvaluator(i.Checks)
			if err != nil {
				return component.Unhealthy(fmt.Sprintf("failed to compile CEL programs: %v", err))
			}
			i.evaluator = evaluator
		}

		// Convert release to maps for CEL evaluation
		releaseMap, chartMap := releaseToMaps(rel)
		celCtx := map[string]any{
			"release": releaseMap,
			"chart":   chartMap,
		}

		if err := i.evaluator.Evaluate(celCtx); err != nil {
			return component.Unhealthy(err.Error())
		}
	}

	return component.Healthy()
}

// releaseToMaps converts a release.Release to separate release and chart maps for CEL evaluation
func releaseToMaps(rel *release.Release) (releaseMap map[string]any, chartMap map[string]any) {
	releaseMap = map[string]any{
		"Name":      rel.Name,
		"Namespace": rel.Namespace,
		"Revision":  rel.Version, // Renamed from Version for Helm idiom
		"Config":    rel.Config,  // User overrides
		"Manifest":  rel.Manifest,
		"Labels":    rel.Labels,
	}

	// Add Info fields directly to release (flattened for cleaner access)
	if rel.Info != nil {
		releaseMap["Status"] = string(rel.Info.Status)
		releaseMap["FirstDeployed"] = rel.Info.FirstDeployed
		releaseMap["LastDeployed"] = rel.Info.LastDeployed
		releaseMap["Deleted"] = rel.Info.Deleted
		releaseMap["Description"] = rel.Info.Description
		releaseMap["Notes"] = rel.Info.Notes
	}

	// Build chart map with flattened metadata and default values (Helm-idiomatic)
	chartMap = map[string]any{}
	if rel.Chart != nil {
		if rel.Chart.Metadata != nil {
			meta := rel.Chart.Metadata
			chartMap = map[string]any{
				"Name":        meta.Name,
				"Version":     meta.Version,
				"AppVersion":  meta.AppVersion,
				"Description": meta.Description,
				"Deprecated":  meta.Deprecated,
				"KubeVersion": meta.KubeVersion,
				"Type":        meta.Type,
				"Annotations": meta.Annotations,
			}
		}
		chartMap["Values"] = rel.Chart.Values // Chart default values
	}

	return releaseMap, chartMap
}
