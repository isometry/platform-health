package helm

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"log/slog"
	"time"

	"github.com/google/cel-go/cel"
	"github.com/mcuadros/go-defaults"
	"go.yaml.in/yaml/v3"
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
	provider.BaseInstanceWithChecks `mapstructure:",squash"`

	Name      string        `mapstructure:"-"`
	Release   string        `mapstructure:"release"`
	Namespace string        `mapstructure:"namespace"`
	Timeout   time.Duration `mapstructure:"timeout" default:"5s"`
}

var _ provider.InstanceWithChecks = (*Helm)(nil)

func init() {
	provider.Register(TypeHelm, new(Helm))
}

func (i *Helm) LogValue() slog.Value {
	logAttr := []slog.Attr{
		slog.String("name", i.Name),
		slog.String("release", i.Release),
		slog.String("namespace", i.Namespace),
		slog.Any("timeout", i.Timeout),
		slog.Int("checks", len(i.GetChecks())),
	}
	return slog.GroupValue(logAttr...)
}

func (i *Helm) Setup() error {
	defaults.SetDefaults(i)
	return i.SetupChecks(celConfig)
}

// GetCheckConfig returns the Helm provider's CEL variable declarations.
func (i *Helm) GetCheckConfig() *checks.CEL {
	return celConfig
}

// GetCheckContext fetches the Helm release and returns the CEL evaluation context.
func (i *Helm) GetCheckContext(ctx context.Context) (map[string]any, error) {
	log := utils.ContextLogger(ctx, slog.String("provider", TypeHelm), slog.Any("instance", i))

	statusRunner, err := client.ClientFactory.GetStatusRunner(i.Namespace, log)
	if err != nil {
		return nil, fmt.Errorf("failed to get status runner: %w", err)
	}

	resultChan := make(chan releaseResult)
	go func() {
		rel, err := statusRunner.Run(i.Release)
		resultChan <- releaseResult{release: rel, err: err}
	}()

	var rel *release.Release
	select {
	case <-time.After(i.Timeout):
		return nil, fmt.Errorf("timeout")
	case result := <-resultChan:
		if result.err != nil {
			return nil, result.err
		}
		rel = result.release
	}

	releaseMap, chartMap := releaseToMaps(rel)
	return map[string]any{
		"release": releaseMap,
		"chart":   chartMap,
	}, nil
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

	// Get check context (fetches release)
	checkCtx, err := i.GetCheckContext(ctx)
	if err != nil {
		return component.Unhealthy(err.Error())
	}

	// Check release status from context
	releaseMap := checkCtx["release"].(map[string]any)
	status := releaseMap["Status"].(string)
	if status != string(client.StatusDeployed) {
		return component.Unhealthy(fmt.Sprintf("expected status 'deployed'; actual status '%s'", status))
	}

	// Apply CEL checks
	if err := i.EvaluateChecks(checkCtx); err != nil {
		return component.Unhealthy(err.Error())
	}

	return component.Healthy()
}

// releaseToMaps converts a release.Release to separate release and chart maps for CEL evaluation
// unmarshalManifest parses a multi-document YAML manifest string into structured objects.
func unmarshalManifest(manifestStr string) []map[string]any {
	if manifestStr == "" {
		return []map[string]any{}
	}

	var result []map[string]any
	decoder := yaml.NewDecoder(bytes.NewReader([]byte(manifestStr)))

	for {
		var doc map[string]any
		if err := decoder.Decode(&doc); err != nil {
			if err == io.EOF {
				break
			}
			// Skip malformed documents
			continue
		}
		if len(doc) > 0 {
			result = append(result, doc)
		}
	}

	return result
}

func releaseToMaps(rel *release.Release) (releaseMap map[string]any, chartMap map[string]any) {
	releaseMap = map[string]any{
		"Name":      rel.Name,
		"Namespace": rel.Namespace,
		"Revision":  rel.Version, // Renamed from Version for Helm idiom
		"Config":    rel.Config,  // User overrides
		"Manifest":  unmarshalManifest(rel.Manifest),
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
