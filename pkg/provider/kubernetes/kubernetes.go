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
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	"sigs.k8s.io/cli-utils/pkg/kstatus/status"

	"github.com/isometry/platform-health/pkg/checks"
	ph "github.com/isometry/platform-health/pkg/platform_health"
	"github.com/isometry/platform-health/pkg/platform_health/details"
	"github.com/isometry/platform-health/pkg/provider"
	"github.com/isometry/platform-health/pkg/provider/kubernetes/client"
	"github.com/isometry/platform-health/pkg/server"
	"github.com/isometry/platform-health/pkg/utils"
)

const TypeKubernetes = "kubernetes"

// AllNamespaces is the special value for namespace to query all namespaces
const AllNamespaces = "*"

// CEL configuration for Kubernetes provider
var celConfig = checks.NewCEL(
	cel.Variable("resource", cel.MapType(cel.StringType, cel.DynType)),
	cel.Variable("items", cel.ListType(cel.MapType(cel.StringType, cel.DynType))),
)

type Kubernetes struct {
	provider.BaseWithChecks `mapstructure:",squash"`

	Name     string        `mapstructure:"-"`
	Resource Resource      `mapstructure:"resource" flag:",inline"`
	KStatus  *bool         `mapstructure:"kstatus"`
	Detail   bool          `mapstructure:"detail"`
	Timeout  time.Duration `mapstructure:"timeout" default:"10s"`
}

var _ provider.InstanceWithChecks = (*Kubernetes)(nil)

// Resource represents a Kubernetes resource to check
type Resource struct {
	Group         string `mapstructure:"group"`
	Version       string `mapstructure:"version"` // Optional: if empty, uses API server's preferred version
	Kind          string `mapstructure:"kind"`
	Namespace     string `mapstructure:"namespace"`
	Name          string `mapstructure:"name"`
	LabelSelector string `mapstructure:"labelSelector"` // Mutually exclusive with Name
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
		slog.Any("timeout", i.Timeout),
		slog.Int("checks", len(i.GetChecks())),
		slog.Bool("detail", i.Detail),
	}
	if i.Resource.Name != "" {
		logAttr = append(logAttr, slog.String("resourceName", i.Resource.Name))
	}
	if i.Resource.LabelSelector != "" {
		logAttr = append(logAttr, slog.String("labelSelector", i.Resource.LabelSelector))
	}
	if i.Resource.Version != "" {
		logAttr = append(logAttr, slog.String("version", i.Resource.Version))
	}
	return slog.GroupValue(logAttr...)
}

func (i *Kubernetes) Setup() error {
	defaults.SetDefaults(i)

	// Validate mutually exclusive Name/LabelSelector
	if i.Resource.Name != "" && i.Resource.LabelSelector != "" {
		return fmt.Errorf("resource.name and resource.labelSelector are mutually exclusive")
	}

	// Validate that name + all-namespaces is invalid
	if i.Resource.Name != "" && i.Resource.Namespace == AllNamespaces {
		return fmt.Errorf("cannot get resource by name across all namespaces; use labelSelector instead")
	}

	// Default kstatus to true if not set
	if i.KStatus == nil {
		kstatusDefault := true
		i.KStatus = &kstatusDefault
	}

	return i.SetupChecks(celConfig)
}

// GetCheckConfig returns the Kubernetes provider's CEL variable declarations.
func (i *Kubernetes) GetCheckConfig() *checks.CEL {
	return celConfig
}

// GetCheckContext fetches the Kubernetes resource(s) and returns the CEL evaluation context.
// For single resource (by name): returns {"resource": resourceMap}
// For multiple resources (by selector): returns {"items": []resourceMap}
func (i *Kubernetes) GetCheckContext(ctx context.Context) (map[string]any, error) {
	ctx, cancel := context.WithTimeout(ctx, i.Timeout)
	defer cancel()

	clients, err := client.ClientFactory.GetClients()
	if err != nil {
		return nil, err
	}

	dynClient := clients.Dynamic
	mapper := clients.Mapper

	// Default group based on kind for common resources
	group := i.Resource.Group
	if group == "" {
		k := strings.ToLower(i.Resource.Kind)
		if g, ok := commonKindToGroup[k]; ok {
			group = g
		}
	}

	gk := schema.GroupKind{
		Group: group,
		Kind:  i.Resource.Kind,
	}

	var mapping *meta.RESTMapping
	if i.Resource.Version != "" {
		mapping, err = mapper.RESTMapping(gk, i.Resource.Version)
	} else {
		mapping, err = mapper.RESTMapping(gk)
	}
	if err != nil {
		return nil, err
	}

	gvr := mapping.Resource

	// Branch based on Name vs selector mode
	if i.Resource.Name != "" {
		blob, err := dynClient.Resource(gvr).Namespace(i.Resource.Namespace).Get(ctx, i.Resource.Name, metav1.GetOptions{})
		if err != nil {
			return nil, err
		}
		return map[string]any{
			"resource": blob.Object,
		}, nil
	}

	// Selector mode - list resources
	listOpts := metav1.ListOptions{
		LabelSelector: i.Resource.LabelSelector,
	}

	var list *unstructured.UnstructuredList
	if mapping.Scope.Name() == meta.RESTScopeNameRoot {
		list, err = dynClient.Resource(gvr).List(ctx, listOpts)
	} else if i.Resource.Namespace == AllNamespaces {
		list, err = dynClient.Resource(gvr).List(ctx, listOpts)
	} else {
		list, err = dynClient.Resource(gvr).Namespace(i.Resource.Namespace).List(ctx, listOpts)
	}
	if err != nil {
		return nil, err
	}

	var items []any
	for idx := range list.Items {
		items = append(items, list.Items[idx].Object)
	}

	return map[string]any{
		"items": items,
	}, nil
}

func (i *Kubernetes) GetType() string {
	return TypeKubernetes
}

func (i *Kubernetes) GetName() string {
	return i.Name
}

func (i *Kubernetes) SetName(name string) {
	i.Name = name
}

func (i *Kubernetes) GetHealth(ctx context.Context) *ph.HealthCheckResponse {
	log := utils.ContextLogger(ctx, slog.String("provider", TypeKubernetes), slog.Any("instance", i))
	log.Debug("checking")

	component := &ph.HealthCheckResponse{
		Type: TypeKubernetes,
		Name: i.GetName(),
	}
	defer component.LogStatus(log)

	clients, err := client.ClientFactory.GetClients()
	if err != nil {
		return component.Unhealthy(err.Error())
	}

	clients.Config.Timeout = i.Timeout
	client := clients.Dynamic
	mapper := clients.Mapper

	// Default group based on kind for common resources
	group := i.Resource.Group
	if group == "" {
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

	// Branch based on Name vs selector mode (empty selector = all resources)
	if i.Resource.Name == "" {
		return i.checkBySelector(ctx, client, gvr, mapping, component, log)
	}

	return i.checkByName(ctx, client, gvr, component)
}

// checkByName checks a single resource by name
func (i *Kubernetes) checkByName(ctx context.Context, client dynamic.Interface, gvr schema.GroupVersionResource, component *ph.HealthCheckResponse) *ph.HealthCheckResponse {
	blob, err := client.Resource(gvr).Namespace(i.Resource.Namespace).Get(ctx, i.Resource.Name, metav1.GetOptions{})
	if err != nil {
		return component.Unhealthy(err.Error())
	}

	// Apply kstatus evaluation
	i.applyKStatus(blob, component)
	if component.Status == ph.Status_UNHEALTHY {
		return component
	}

	// Apply CEL checks with resource context
	celCtx := map[string]any{
		"resource": blob.Object,
	}

	if err := i.EvaluateChecks(celCtx); err != nil {
		return component.Unhealthy(err.Error())
	}

	return component
}

// checkBySelector lists resources matching the label selector and checks each
func (i *Kubernetes) checkBySelector(ctx context.Context, client dynamic.Interface, gvr schema.GroupVersionResource, mapping *meta.RESTMapping, component *ph.HealthCheckResponse, log *slog.Logger) *ph.HealthCheckResponse {
	listOpts := metav1.ListOptions{
		LabelSelector: i.Resource.LabelSelector,
	}

	// Handle cluster-scoped vs namespaced resources
	var list *unstructured.UnstructuredList
	var err error
	if mapping.Scope.Name() == meta.RESTScopeNameRoot {
		// Cluster-scoped resources (e.g., nodes, namespaces)
		list, err = client.Resource(gvr).List(ctx, listOpts)
	} else if i.Resource.Namespace == AllNamespaces {
		// All namespaces mode
		list, err = client.Resource(gvr).List(ctx, listOpts)
	} else {
		// Specific namespace
		list, err = client.Resource(gvr).Namespace(i.Resource.Namespace).List(ctx, listOpts)
	}
	if err != nil {
		return component.Unhealthy(err.Error())
	}

	// Filter by component paths if specified
	componentPaths := server.ComponentPathsFromContext(ctx)
	if len(componentPaths) > 0 {
		requestedNames := make(map[string]bool)
		for _, paths := range componentPaths {
			if len(paths) > 0 {
				requestedNames[paths[0]] = true
			}
		}

		var filtered []unstructured.Unstructured
		for _, item := range list.Items {
			if requestedNames[item.GetName()] {
				filtered = append(filtered, item)
				delete(requestedNames, item.GetName())
			}
		}

		// Error if requested components not found
		if len(requestedNames) > 0 {
			var invalid []string
			for name := range requestedNames {
				invalid = append(invalid, name)
			}
			return component.Unhealthy(fmt.Sprintf("invalid components: %v", invalid))
		}

		list.Items = filtered
	}

	// Check each matched resource
	var components []*ph.HealthCheckResponse
	worstStatus := ph.Status_HEALTHY

	for idx := range list.Items {
		item := &list.Items[idx]
		resourceName := item.GetName()

		childComponent := &ph.HealthCheckResponse{
			Type: TypeKubernetes,
			Name: resourceName,
		}

		result := i.applyKStatus(item, childComponent)
		components = append(components, result)

		if result.Status > worstStatus {
			worstStatus = result.Status
		}

		childLog := log.With(slog.String("resource", resourceName))
		result.LogStatus(childLog)
	}

	component.Status = worstStatus
	component.Components = components

	// Apply CEL checks against items list (selector mode)
	var items []any
	for idx := range list.Items {
		items = append(items, list.Items[idx].Object)
	}

	celCtx := map[string]any{
		"items": items,
	}

	if err := i.EvaluateChecks(celCtx); err != nil {
		return component.Unhealthy(err.Error())
	}

	return component
}

// applyKStatus applies kstatus evaluation to a single resource
func (i *Kubernetes) applyKStatus(blob *unstructured.Unstructured, component *ph.HealthCheckResponse) *ph.HealthCheckResponse {
	if !*i.KStatus {
		return component.Healthy()
	}

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

	return component.Healthy()
}

// getString safely extracts a string value from a map
func getString(m map[string]any, key string) string {
	if v, ok := m[key].(string); ok {
		return v
	}
	return ""
}
