package client

import (
	"context"

	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
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
	if err := q.Validate(); err != nil {
		return nil, err
	}

	mapping, err := f.ResolveMapping(q)
	if err != nil {
		return nil, err
	}

	ri := f.resourceInterface(mapping, q.Namespace, false)
	return ri.Get(ctx, q.Name, metav1.GetOptions{})
}

// List fetches resources matching a label selector.
// Handles cluster-scoped, all-namespaces, and namespaced queries automatically.
func (f *ResourceFetcher) List(ctx context.Context, q ResourceQuery) (*unstructured.UnstructuredList, error) {
	if err := q.Validate(); err != nil {
		return nil, err
	}

	mapping, err := f.ResolveMapping(q)
	if err != nil {
		return nil, err
	}

	opts := metav1.ListOptions{LabelSelector: q.LabelSelector}
	ri := f.resourceInterface(mapping, q.Namespace, true)
	return ri.List(ctx, opts)
}

// resourceInterface returns the appropriate dynamic.ResourceInterface for the given mapping and namespace.
// For List operations, pass forList=true to handle AllNamespaces correctly.
func (f *ResourceFetcher) resourceInterface(mapping *meta.RESTMapping, namespace string, forList bool) dynamic.ResourceInterface {
	gvr := mapping.Resource
	// Cluster-scoped resources always use cluster-wide interface
	if mapping.Scope.Name() == meta.RESTScopeNameRoot {
		return f.clients.Dynamic.Resource(gvr)
	}
	// For List: empty or AllNamespaces uses cluster-wide
	// For Get: only empty namespace uses cluster-wide
	if namespace == "" || (forList && namespace == AllNamespaces) {
		return f.clients.Dynamic.Resource(gvr)
	}
	return f.clients.Dynamic.Resource(gvr).Namespace(namespace)
}
