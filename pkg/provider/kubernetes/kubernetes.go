//go:generate go run ./common/generator.go

package kubernetes

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/google/cel-go/cel"
	"github.com/mcuadros/go-defaults"
	"google.golang.org/protobuf/types/known/anypb"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/discovery/cached/memory"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/restmapper"
	"sigs.k8s.io/cli-utils/pkg/kstatus/status"

	"github.com/isometry/platform-health/pkg/checks"
	ph "github.com/isometry/platform-health/pkg/platform_health"
	"github.com/isometry/platform-health/pkg/platform_health/details"
	"github.com/isometry/platform-health/pkg/provider"
	"github.com/isometry/platform-health/pkg/utils"
)

const TypeKubernetes = "kubernetes"

// CEL configuration for Kubernetes provider
var celConfig = checks.NewCEL(
	cel.Variable("resource", cel.MapType(cel.StringType, cel.DynType)),
)

// Resource represents a Kubernetes resource to check
type Resource struct {
	Group     string `mapstructure:"group" default:"apps"`
	Version   string `mapstructure:"version"` // Optional: if empty, uses API server's preferred version
	Kind      string `mapstructure:"kind" default:"deployment"`
	Namespace string `mapstructure:"namespace" default:"default"`
	Name      string `mapstructure:"name"`
}

type Kubernetes struct {
	Name     string              `mapstructure:"name"`
	Resource Resource            `mapstructure:"resource"`
	Checks   []checks.Expression `mapstructure:"checks"`
	KStatus  *bool               `mapstructure:"kstatus"`
	Detail   bool                `mapstructure:"detail"`
	Timeout  time.Duration       `mapstructure:"timeout" default:"10s"`

	// Compiled CEL evaluator (cached after Setup)
	evaluator *checks.Evaluator
}

func init() {
	provider.Register(TypeKubernetes, new(Kubernetes))
}

func (i *Kubernetes) LogValue() slog.Value {
	logAttr := []slog.Attr{
		slog.String("name", i.Name),
		slog.String("group", i.Resource.Group),
		slog.String("kind", i.Resource.Kind),
		slog.String("namespace", i.Resource.Namespace),
		slog.String("resourceName", i.Resource.Name),
		slog.Any("timeout", i.Timeout),
		slog.Int("checks", len(i.Checks)),
		slog.Bool("detail", i.Detail),
	}
	if i.Resource.Version != "" {
		logAttr = append(logAttr, slog.String("version", i.Resource.Version))
	}
	return slog.GroupValue(logAttr...)
}

func (i *Kubernetes) Setup() error {
	defaults.SetDefaults(i)

	// Default kstatus to true if not set
	if i.KStatus == nil {
		kstatusDefault := true
		i.KStatus = &kstatusDefault
	}

	// Pre-compile CEL evaluator if checks exist (using package-level cache)
	if len(i.Checks) > 0 {
		evaluator, err := celConfig.NewEvaluator(i.Checks)
		if err != nil {
			return fmt.Errorf("invalid CEL expression: %w", err)
		}
		i.evaluator = evaluator
	}

	return nil
}

func (i *Kubernetes) GetType() string {
	return TypeKubernetes
}

func (i *Kubernetes) GetName() string {
	if i.Name != "" {
		return i.Name
	}
	return fmt.Sprintf("%s/%s", i.Resource.Kind, i.Resource.Name)
}

func (i *Kubernetes) GetHealth(ctx context.Context) *ph.HealthCheckResponse {
	log := utils.ContextLogger(ctx, slog.String("provider", TypeKubernetes), slog.Any("instance", i))
	log.Debug("checking")

	component := &ph.HealthCheckResponse{
		Type: TypeKubernetes,
		Name: i.GetName(),
	}
	defer component.LogStatus(log)

	config, err := utils.GetKubeConfig()
	if err != nil {
		return component.Unhealthy(err.Error())
	}

	config.Timeout = i.Timeout

	client, err := dynamic.NewForConfig(config)
	if err != nil {
		return component.Unhealthy(err.Error())
	}

	dc, _ := discovery.NewDiscoveryClientForConfig(config)
	mapper := restmapper.NewDeferredDiscoveryRESTMapper(memory.NewMemCacheClient(dc))

	// Default group based on kind for common resources
	group := i.Resource.Group
	if group == "apps" { // default value
		k := strings.ToLower(i.Resource.Kind)
		if g, ok := commonKindToGroup[k]; ok {
			group = g
		}
	}

	gk := schema.GroupKind{
		Group: group,
		Kind:  i.Resource.Kind,
	}

	// Use explicit version if provided, otherwise let RESTMapper discover preferred version
	var mapping *meta.RESTMapping
	if i.Resource.Version != "" {
		mapping, err = mapper.RESTMapping(gk, i.Resource.Version)
	} else {
		mapping, err = mapper.RESTMapping(gk)
	}
	if err != nil {
		return component.Unhealthy(err.Error())
	}

	gvr := mapping.Resource

	blob, err := client.Resource(gvr).Namespace(i.Resource.Namespace).Get(ctx, i.Resource.Name, metav1.GetOptions{})

	if err != nil {
		return component.Unhealthy(err.Error())
	}

	// Apply kstatus evaluation if enabled
	if *i.KStatus {
		result, err := status.Compute(blob)
		if err != nil {
			return component.Unhealthy(err.Error())
		}

		// Build kstatus detail
		kstatusDetail := &details.Detail_KStatus{
			Status:  result.Status.String(),
			Message: result.Message,
		}

		// Only include conditions if not Current (for debugging)
		if result.Status != status.CurrentStatus {
			if statusMap, ok := blob.Object["status"].(map[string]any); ok {
				if conditionsRaw, ok := statusMap["conditions"].([]any); ok {
					for _, condRaw := range conditionsRaw {
						if cond, ok := condRaw.(map[string]any); ok {
							kstatusDetail.Conditions = append(kstatusDetail.Conditions, &details.Condition{
								Type:    getString(cond, "type"),
								Status:  getString(cond, "status"),
								Reason:  getString(cond, "reason"),
								Message: getString(cond, "message"),
							})
						}
					}
				}
			}
		}

		// Append detail to component if Detail is enabled
		if i.Detail {
			if detail, err := anypb.New(kstatusDetail); err != nil {
				return component.Unhealthy(err.Error())
			} else {
				component.Details = append(component.Details, detail)
			}
		}

		if result.Status != status.CurrentStatus {
			msg := result.Message
			if msg == "" {
				msg = fmt.Sprintf("kstatus: %s", result.Status)
			}
			return component.Unhealthy(msg)
		}
	}

	// Apply CEL checks if configured
	if len(i.Checks) > 0 {
		// Ensure CEL evaluator is compiled
		if i.evaluator == nil {
			evaluator, err := celConfig.NewEvaluator(i.Checks)
			if err != nil {
				return component.Unhealthy(fmt.Sprintf("failed to compile CEL programs: %v", err))
			}
			i.evaluator = evaluator
		}

		// Pass the raw resource object to CEL for full access
		celCtx := map[string]any{
			"resource": blob.Object,
		}

		if err := i.evaluator.Evaluate(celCtx); err != nil {
			return component.Unhealthy(err.Error())
		}
	}

	return component.Healthy()
}

// getString safely extracts a string value from a map
func getString(m map[string]any, key string) string {
	if v, ok := m[key].(string); ok {
		return v
	}
	return ""
}
