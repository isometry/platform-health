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

const ProviderType = "helm"

// CEL configuration for Helm provider
var celConfig = checks.NewCEL(
	cel.Variable("release", cel.MapType(cel.StringType, cel.DynType)),
	cel.Variable("chart", cel.MapType(cel.StringType, cel.DynType)),
)

type Component struct {
	provider.BaseWithChecks `mapstructure:",squash"`

	Name      string        `mapstructure:"-"`
	Release   string        `mapstructure:"release"`
	Namespace string        `mapstructure:"namespace"`
	Timeout   time.Duration `mapstructure:"timeout" default:"5s"`
}

var _ provider.InstanceWithChecks = (*Component)(nil)

func init() {
	provider.Register(ProviderType, new(Component))
}

func (c *Component) LogValue() slog.Value {
	logAttr := []slog.Attr{
		slog.String("name", c.Name),
		slog.String("release", c.Release),
		slog.String("namespace", c.Namespace),
		slog.Any("timeout", c.Timeout),
		slog.Int("checks", len(c.GetChecks())),
	}
	return slog.GroupValue(logAttr...)
}

func (c *Component) Setup() error {
	defaults.SetDefaults(c)
	return c.SetupChecks(celConfig)
}

// GetCheckConfig returns the Helm provider's CEL variable declarations.
func (c *Component) GetCheckConfig() *checks.CEL {
	return celConfig
}

// GetCheckContext fetches the Helm release and returns the CEL evaluation context.
func (c *Component) GetCheckContext(ctx context.Context) (map[string]any, error) {
	ctx, cancel := context.WithTimeout(ctx, c.Timeout)
	defer cancel()

	log := utils.ContextLogger(ctx, slog.String("provider", ProviderType), slog.Any("instance", c))

	statusRunner, err := client.ClientFactory.GetStatusRunner(c.Namespace, log)
	if err != nil {
		return nil, fmt.Errorf("failed to get status runner: %w", err)
	}

	rel, err := statusRunner.Run(ctx, c.Release)
	if err != nil {
		return nil, err
	}

	releaseMap, chartMap := releaseToMaps(rel)
	return map[string]any{
		"release": releaseMap,
		"chart":   chartMap,
	}, nil
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

func (c *Component) GetHealth(ctx context.Context) *ph.HealthCheckResponse {
	log := utils.ContextLogger(ctx, slog.String("provider", ProviderType), slog.Any("instance", c))
	log.Debug("checking")

	component := &ph.HealthCheckResponse{
		Type: ProviderType,
		Name: c.Name,
	}
	defer component.LogStatus(log)

	// Get check context (fetches release)
	checkCtx, err := c.GetCheckContext(ctx)
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
	if err := c.EvaluateChecks(checkCtx); err != nil {
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
