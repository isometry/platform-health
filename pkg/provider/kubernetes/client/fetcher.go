package client

import (
	"context"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/client-go/dynamic"
)

// ResourceFetcher provides unified Kubernetes resource operations
// with proper handling of cluster-scoped vs namespaced resources.
type ResourceFetcher struct {
	clients *KubeClients
}

// NewFetcher creates a ResourceFetcher bound to the given clients
func NewFetcher(clients *KubeClients) *ResourceFetcher {
	return &ResourceFetcher{clients: clients}
}

// ResolveMapping converts a query to a REST mapping.
// If Version is empty, the preferred version is auto-discovered.
func (f *ResourceFetcher) ResolveMapping(q ResourceQuery) (*meta.RESTMapping, error) {
	gk := q.GroupKind()
	if q.Version != "" {
		return f.clients.Mapper.RESTMapping(gk, q.Version)
	}
	return f.clients.Mapper.RESTMapping(gk)
}

// Get fetches a single resource by name.
// Handles cluster-scoped vs namespaced resources automatically.
func (f *ResourceFetcher) Get(ctx context.Context, q ResourceQuery) (*unstructured.Unstructured, error) {
	mapping, err := f.ResolveMapping(q)
	if err != nil {
		return nil, err
	}

	ri := f.resourceInterface(mapping, q.Namespace)
	return ri.Get(ctx, q.Name, metav1.GetOptions{})
}

// List fetches resources matching a label selector.
// Handles cluster-scoped, all-namespaces, and namespaced queries automatically.
func (f *ResourceFetcher) List(ctx context.Context, q ResourceQuery) (*unstructured.UnstructuredList, error) {
	mapping, err := f.ResolveMapping(q)
	if err != nil {
		return nil, err
	}

	opts := metav1.ListOptions{LabelSelector: q.LabelSelector}
	ri := f.scopedResourceInterface(mapping, q.Namespace)
	return ri.List(ctx, opts)
}

// resourceInterface returns the appropriate interface for Get operations.
// Cluster-scoped resources or empty namespace use cluster-wide interface.
func (f *ResourceFetcher) resourceInterface(mapping *meta.RESTMapping, namespace string) dynamic.ResourceInterface {
	gvr := mapping.Resource
	if namespace == "" || mapping.Scope.Name() == meta.RESTScopeNameRoot {
		return f.clients.Dynamic.Resource(gvr)
	}
	return f.clients.Dynamic.Resource(gvr).Namespace(namespace)
}

// scopedResourceInterface returns the appropriate interface for List operations.
// Handles cluster-scoped, all-namespaces, and specific namespace cases.
func (f *ResourceFetcher) scopedResourceInterface(mapping *meta.RESTMapping, namespace string) dynamic.ResourceInterface {
	gvr := mapping.Resource
	// Cluster-scoped resources always use cluster-wide interface
	if mapping.Scope.Name() == meta.RESTScopeNameRoot {
		return f.clients.Dynamic.Resource(gvr)
	}
	// All-namespaces or empty namespace uses cluster-wide interface
	if namespace == AllNamespaces || namespace == "" {
		return f.clients.Dynamic.Resource(gvr)
	}
	// Specific namespace
	return f.clients.Dynamic.Resource(gvr).Namespace(namespace)
}
