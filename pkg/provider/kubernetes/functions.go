package kubernetes

import (
	"context"
	"fmt"
	"strings"
	"sync"

	"github.com/google/cel-go/cel"
	"github.com/google/cel-go/common/types"
	"github.com/google/cel-go/common/types/ref"
	"github.com/google/cel-go/common/types/traits"
	apierrors "k8s.io/apimachinery/pkg/api/errors"

	"github.com/isometry/platform-health/pkg/provider/kubernetes/client"
)

// KubernetesGetDeclaration returns the CEL type declaration for kubernetes.Get.
// This declares the function signature for type checking but does not provide
// the implementation - that is injected at runtime via KubernetesGetBinding().
//
// Parameters (via map):
//   - kind (required): Resource kind (case-insensitive)
//   - name (conditional): Resource name (required if no labelSelector)
//   - labelSelector (conditional): Label selector (required if no name)
//   - namespace (optional): Namespace (empty for cluster-scoped)
//   - group (optional): API group (auto-resolved via commonKindToGroup)
//   - version (optional): API version (auto-discovered by REST mapper)
//
// Returns:
//   - Single mode (name): map[string]any or null if not found
//   - List mode (labelSelector): list of maps (possibly empty)
//
// Security: queries are scoped by the Kubernetes service account's RBAC permissions.
// Ensure the service account follows least-privilege principles, as CEL expressions
// in configuration files can access any resource the service account can read.
func KubernetesGetDeclaration() cel.EnvOption {
	return cel.Function("kubernetes.Get",
		cel.Overload("kubernetes_get_resource_map",
			[]*cel.Type{cel.MapType(cel.StringType, cel.StringType)},
			cel.DynType, // Returns map (single) or list (labelSelector)
		),
	)
}

// ResourceCache caches resources within a single CEL evaluation.
// This prevents duplicate API calls when the same resource is referenced
// multiple times in check expressions.
type ResourceCache struct {
	mu    sync.RWMutex
	cache map[string]ref.Val
}

// NewResourceCache creates a new resource cache for a single evaluation cycle.
func NewResourceCache() *ResourceCache {
	return &ResourceCache{cache: make(map[string]ref.Val)}
}

// KubernetesGetBinding creates the runtime function binding with K8s client bound via closure.
// This is used with env.Extend() to create an extended CEL environment at evaluation time
// that includes the implementation for kubernetes.Get with the provided context and clients.
func KubernetesGetBinding(ctx context.Context, clients *client.KubeClients, cache *ResourceCache) cel.EnvOption {
	return cel.Function("kubernetes.Get",
		cel.Overload("kubernetes_get_resource_map",
			[]*cel.Type{cel.MapType(cel.StringType, cel.StringType)},
			cel.DynType,
			cel.UnaryBinding(func(arg ref.Val) ref.Val {
				return kubernetesGetImpl(ctx, clients, cache, arg)
			}),
		),
	)
}

// kubernetesGetImpl is the implementation of kubernetes.Get
func kubernetesGetImpl(ctx context.Context, clients *client.KubeClients, cache *ResourceCache, arg ref.Val) ref.Val {
	params, ok := arg.(traits.Mapper)
	if !ok {
		return types.NewErr("kubernetes.Get: expected map argument, got %T", arg)
	}

	// Extract required key: kind
	kind, err := getStringKey(params, "kind")
	if err != nil {
		return types.NewErr("kubernetes.Get: %v", err)
	}

	// Extract conditional keys: name OR labelSelector (one required)
	name := getStringKeyOrDefault(params, "name", "")
	labelSelector := getStringKeyOrDefault(params, "labelSelector", "")
	if name == "" && labelSelector == "" {
		return types.NewErr("kubernetes.Get: either 'name' or 'labelSelector' is required")
	}
	if name != "" && labelSelector != "" {
		return types.NewErr("kubernetes.Get: 'name' and 'labelSelector' are mutually exclusive")
	}

	// Extract optional keys with auto-resolution
	namespace := getStringKeyOrDefault(params, "namespace", "")
	group := getStringKeyOrDefault(params, "group", "")
	version := getStringKeyOrDefault(params, "version", "")

	// Auto-resolve group from commonKindToGroup if not provided
	if group == "" {
		if resolvedGroup, exists := commonKindToGroup[strings.ToLower(kind)]; exists {
			group = resolvedGroup
		}
	}

	// Build cache key
	var key string
	if name != "" {
		key = fmt.Sprintf("get:%s/%s/%s/%s/%s", group, version, kind, namespace, name)
	} else {
		key = fmt.Sprintf("list:%s/%s/%s/%s/%s", group, version, kind, namespace, labelSelector)
	}

	// Check cache
	cache.mu.RLock()
	if val, exists := cache.cache[key]; exists {
		cache.mu.RUnlock()
		return val
	}
	cache.mu.RUnlock()

	// Build query
	query := client.ResourceQuery{
		Group:         group,
		Version:       version,
		Kind:          kind,
		Namespace:     namespace,
		Name:          name,
		LabelSelector: labelSelector,
	}

	// Fetch resource(s)
	fetcher := clients.NewFetcher()
	var result ref.Val
	if query.Mode() == client.QueryModeGet {
		result = fetchSingleResource(ctx, fetcher, query)
	} else {
		result = fetchResourceList(ctx, fetcher, query)
	}

	// Cache result
	cache.mu.Lock()
	cache.cache[key] = result
	cache.mu.Unlock()

	return result
}

func getStringKey(m traits.Mapper, key string) (string, error) {
	val := m.Get(types.String(key))
	if types.IsUnknownOrError(val) {
		return "", fmt.Errorf("missing required key %q", key)
	}
	str, ok := val.(types.String)
	if !ok {
		return "", fmt.Errorf("key %q must be a string, got %T", key, val)
	}
	return string(str), nil
}

func getStringKeyOrDefault(m traits.Mapper, key, defaultVal string) string {
	val := m.Get(types.String(key))
	if types.IsUnknownOrError(val) {
		return defaultVal
	}
	if str, ok := val.(types.String); ok {
		return string(str)
	}
	return defaultVal
}

func fetchSingleResource(ctx context.Context, fetcher *client.ResourceFetcher, query client.ResourceQuery) ref.Val {
	resource, err := fetcher.Get(ctx, query)
	if err != nil {
		if apierrors.IsNotFound(err) {
			return types.NullValue
		}
		return types.NewErr("kubernetes.Get: %v", err)
	}
	return types.NewDynamicMap(types.DefaultTypeAdapter, resource.Object)
}

func fetchResourceList(ctx context.Context, fetcher *client.ResourceFetcher, query client.ResourceQuery) ref.Val {
	list, err := fetcher.List(ctx, query)
	if err != nil {
		// List operations don't have "not found" - empty result is normal
		// Return error for actual failures (permissions, network, etc.)
		return types.NewErr("kubernetes.Get: %v", err)
	}

	items := make([]any, len(list.Items))
	for i, item := range list.Items {
		items[i] = item.Object
	}

	return types.NewDynamicList(types.DefaultTypeAdapter, items)
}
