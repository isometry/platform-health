//go:generate go run ./common/generator.go

package kubernetes

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	"github.com/google/cel-go/cel"
	"github.com/mcuadros/go-defaults"
	"google.golang.org/protobuf/types/known/anypb"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"sigs.k8s.io/cli-utils/pkg/kstatus/status"

	"github.com/isometry/platform-health/pkg/checks"
	"github.com/isometry/platform-health/pkg/phctx"
	ph "github.com/isometry/platform-health/pkg/platform_health"
	"github.com/isometry/platform-health/pkg/platform_health/details"
	"github.com/isometry/platform-health/pkg/provider"
	"github.com/isometry/platform-health/pkg/provider/kubernetes/client"
)

const ProviderType = "kubernetes"

// AllNamespaces is re-exported from client for convenience
const AllNamespaces = client.AllNamespaces

type Component struct {
	provider.Base
	provider.BaseWithChecks

	Context       string `mapstructure:"context"`
	Group         string `mapstructure:"group"`
	Version       string `mapstructure:"version"` // Optional: if empty, uses API server's preferred version
	Kind          string `mapstructure:"kind"`
	Namespace     string `mapstructure:"namespace"`
	Name          string `mapstructure:"name"`
	LabelSelector string `mapstructure:"labelSelector"` // Mutually exclusive with Name
	KStatus       *bool  `mapstructure:"kstatus" default:"true"`
	Detail        bool   `mapstructure:"detail"`
	Summarize     bool   `mapstructure:"summarize"` // In labelSelector mode, accumulate errors into messages instead of sub-components
}

var _ provider.InstanceWithChecks = (*Component)(nil)

// CEL configuration for Kubernetes provider
var celConfig = checks.NewCEL(
	cel.Variable("resource", cel.MapType(cel.StringType, cel.DynType)),
	cel.Variable("items", cel.ListType(cel.MapType(cel.StringType, cel.DynType))),
	KubernetesGetDeclaration(),
).WithIterationKeys("items", "resource")

func init() {
	provider.Register(ProviderType, new(Component))
}

func (c *Component) LogValue() slog.Value {
	logAttr := []slog.Attr{
		slog.String("name", c.GetName()),
		slog.String("group", c.Group),
		slog.String("kind", c.Kind),
		slog.String("namespace", c.Namespace),
		slog.Int("checks", len(c.GetChecks())),
		slog.Bool("detail", c.Detail),
	}
	if c.Context != "" {
		logAttr = append(logAttr, slog.String("context", c.Context))
	}
	if c.Name != "" {
		logAttr = append(logAttr, slog.String("resourceName", c.Name))
	}
	if c.LabelSelector != "" {
		logAttr = append(logAttr, slog.String("labelSelector", c.LabelSelector))
	}
	if c.Version != "" {
		logAttr = append(logAttr, slog.String("version", c.Version))
	}
	return slog.GroupValue(logAttr...)
}

func (c *Component) Setup() error {
	defaults.SetDefaults(c)

	// defaults.SetDefaults doesn't allocate pointer types, so handle *bool default
	if c.KStatus == nil {
		t := true
		c.KStatus = &t
	}

	// Default group based on kind for common resources
	if c.Group == "" {
		k := strings.ToLower(c.Kind)
		if g, ok := commonKindToGroup[k]; ok {
			c.Group = g
		}
	}

	// Delegate field validation to ResourceQuery
	return c.buildQuery().Validate()
}

// SetChecks sets and compiles CEL expressions.
func (c *Component) SetChecks(exprs []checks.Expression) error {
	return c.SetChecksAndCompile(exprs, celConfig)
}

// GetCheckConfig returns the Kubernetes provider's CEL variable declarations.
func (c *Component) GetCheckConfig() *checks.CEL {
	return celConfig
}

// buildQuery creates a ResourceQuery from the component's configuration.
func (c *Component) buildQuery() client.ResourceQuery {
	return client.ResourceQuery{
		Group:         c.Group,
		Version:       c.Version,
		Kind:          c.Kind,
		Namespace:     c.Namespace,
		Name:          c.Name,
		LabelSelector: c.LabelSelector,
	}
}

// GetCheckContext fetches the Kubernetes resource(s) and returns the CEL evaluation context.
// For single resource (by name): returns {"resource": resourceMap}
// For multiple resources (by selector): returns {"items": []resourceMap}
func (c *Component) GetCheckContext(ctx context.Context) (map[string]any, error) {
	clients, err := client.ClientFactory.GetClients(c.Context)
	if err != nil {
		return nil, fmt.Errorf("kubernetes client for context %q: %w", c.Context, err)
	}

	fetcher := clients.NewFetcher()
	query := c.buildQuery()

	// Branch based on Name vs selector mode
	if query.Mode() == client.QueryModeGet {
		blob, err := fetcher.Get(ctx, query)
		if err != nil {
			return nil, fmt.Errorf("get %+v: %w", query, err)
		}
		return map[string]any{
			"resource": blob.Object,
		}, nil
	}

	// Selector mode - list resources
	list, err := fetcher.List(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("list %+v: %w", query, err)
	}

	var items []any
	for idx := range list.Items {
		items = append(items, list.Items[idx].Object)
	}

	return map[string]any{
		"items": items,
	}, nil
}

func (c *Component) GetType() string {
	return ProviderType
}

func (c *Component) GetHealth(ctx context.Context) *ph.HealthCheckResponse {
	log := phctx.Logger(ctx, slog.String("provider", ProviderType), slog.Any("instance", c))
	log.Debug("checking")

	component := &ph.HealthCheckResponse{
		Type: ProviderType,
		Name: c.GetName(),
	}
	defer component.LogStatus(log)

	clients, err := client.ClientFactory.GetClients(c.Context)
	if err != nil {
		return component.Unhealthy(err.Error())
	}

	fetcher := clients.NewFetcher()
	query := client.ResourceQuery{
		Group:         c.Group,
		Version:       c.Version,
		Kind:          c.Kind,
		Namespace:     c.Namespace,
		Name:          c.Name,
		LabelSelector: c.LabelSelector,
	}

	mapping, err := fetcher.ResolveMapping(query)
	if err != nil {
		return component.Unhealthy(err.Error())
	}

	// Branch based on Name vs selector mode (empty selector = all resources)
	if query.Mode() == client.QueryModeList {
		return c.checkBySelector(ctx, clients, fetcher, query, mapping, component, log)
	}

	return c.checkByName(ctx, clients, fetcher, query, component)
}

// checkByName checks a single resource by name
func (c *Component) checkByName(ctx context.Context, clients *client.KubeClients, fetcher *client.ResourceFetcher, query client.ResourceQuery, component *ph.HealthCheckResponse) *ph.HealthCheckResponse {
	blob, err := fetcher.Get(ctx, query)
	if err != nil {
		return component.Unhealthy(err.Error())
	}

	// Apply kstatus evaluation
	c.applyKStatus(blob, component)
	if component.Status == ph.Status_UNHEALTHY {
		return component
	}

	// Apply CEL checks with resource context
	celCtx := map[string]any{
		"resource": blob.Object,
	}

	// Create resource cache and runtime binding for kubernetes.Get
	cache := NewResourceCache()
	binding := KubernetesGetBinding(ctx, clients, cache)

	if msgs := c.EvaluateChecks(ctx, celCtx, binding); len(msgs) > 0 {
		return component.Unhealthy(msgs...)
	}

	return component
}

// checkBySelector lists resources matching the label selector and checks each
func (c *Component) checkBySelector(ctx context.Context, clients *client.KubeClients, fetcher *client.ResourceFetcher, query client.ResourceQuery, mapping *meta.RESTMapping, component *ph.HealthCheckResponse, log *slog.Logger) *ph.HealthCheckResponse {
	list, err := fetcher.List(ctx, query)
	if err != nil {
		return component.Unhealthy(err.Error())
	}

	// Filter by component paths if specified
	componentPaths := phctx.ComponentPathsFromContext(ctx)
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
	var summarizedMessages []string
	var items []any
	worstStatus := ph.Status_HEALTHY

	// Get pre-compiled per-item checks
	eachChecks := c.Checks(checks.ModeEach)

	// Create resource cache and runtime binding for kubernetes.Get
	cache := NewResourceCache()
	binding := KubernetesGetBinding(ctx, clients, cache)

	for idx := range list.Items {
		item := &list.Items[idx]
		resourceName := item.GetName()
		resourceNS := item.GetNamespace()

		childComponent := &ph.HealthCheckResponse{
			Name: resourceName,
			// Kind omitted - inherits from parent in hierarchical view, populated by Flatten() for flat mode
		}

		result := c.applyKStatus(item, childComponent)

		// Apply per-item checks (mode: each)
		if len(eachChecks) > 0 {
			failFast := phctx.FailFastFromContext(ctx)
			celCtx := map[string]any{"resource": item.Object}
			for _, check := range eachChecks {
				if msg, err := check.Evaluate(celCtx, binding); err != nil {
					result.Status = ph.Status_UNHEALTHY
					result.Messages = append(result.Messages, err.Error())
					if failFast {
						break
					}
				} else if msg != "" {
					result.Status = ph.Status_UNHEALTHY
					result.Messages = append(result.Messages, msg)
					if failFast {
						break
					}
				}
			}
		}

		if c.Summarize {
			// Summarize mode: collect error messages instead of sub-components
			if result.Status > ph.Status_HEALTHY {
				resourceID := formatResourceID(resourceName, resourceNS)
				if len(result.Messages) > 0 {
					for _, msg := range result.Messages {
						summarizedMessages = append(summarizedMessages, msg+": "+resourceID)
					}
				} else {
					// No specific message but unhealthy, add status
					summarizedMessages = append(summarizedMessages, result.Status.String()+": "+resourceID)
				}
			}
		} else {
			components = append(components, result)
		}

		items = append(items, item.Object)

		if result.Status > worstStatus {
			worstStatus = result.Status
		}

		childLog := log.With(slog.String("resource", resourceName))
		result.LogStatus(childLog)
	}

	component.Status = worstStatus
	if c.Summarize {
		component.Messages = summarizedMessages
	} else {
		component.Components = components
	}

	// Apply default CEL checks against items list (selector mode)
	celCtx := map[string]any{"items": items}
	if msgs := c.EvaluateChecksByMode(ctx, checks.ModeDefault, celCtx, binding); len(msgs) > 0 {
		return component.Unhealthy(msgs...)
	}

	return component
}

// applyKStatus applies kstatus evaluation to a single resource
func (c *Component) applyKStatus(blob *unstructured.Unstructured, component *ph.HealthCheckResponse) *ph.HealthCheckResponse {
	if !*c.KStatus {
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
							Type:    client.GetString(cond, "type"),
							Status:  client.GetString(cond, "status"),
							Reason:  client.GetString(cond, "reason"),
							Message: client.GetString(cond, "message"),
						})
					}
				}
			}
		}
	}

	// Append detail to component if Detail is enabled
	if c.Detail {
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

// formatResourceID creates a human-readable resource identifier for summarized output.
// Format: {name}@{namespace} for namespaced resources, or just {name} for cluster-scoped resources.
func formatResourceID(name, namespace string) string {
	if namespace == "" {
		return name
	}
	return name + "@" + namespace
}
