package client

import (
	"fmt"

	"k8s.io/apimachinery/pkg/runtime/schema"
)

// AllNamespaces is the special value for namespace to query all namespaces
const AllNamespaces = "*"

// QueryMode indicates whether a query should Get a single resource or List multiple
type QueryMode int

const (
	// QueryModeGet fetches a single resource by name
	QueryModeGet QueryMode = iota
	// QueryModeList fetches resources matching a label selector
	QueryModeList
)

// ResourceQuery encapsulates parameters for Kubernetes resource lookups.
// It supports both single-resource Gets (by name) and multi-resource Lists (by selector).
type ResourceQuery struct {
	Group         string // API group (e.g., "apps", "" for core)
	Version       string // API version (e.g., "v1"); empty for auto-discovery
	Kind          string // Resource kind (e.g., "Deployment", "Pod")
	Namespace     string // Namespace; empty for default, AllNamespaces for cluster-wide
	Name          string // Resource name; mutually exclusive with LabelSelector
	LabelSelector string // Label selector; mutually exclusive with Name
}

// Mode returns whether this is a Get (by name) or List (by selector) query
func (q ResourceQuery) Mode() QueryMode {
	if q.Name != "" {
		return QueryModeGet
	}
	return QueryModeList
}

// GroupKind returns the schema.GroupKind for REST mapping lookups
func (q ResourceQuery) GroupKind() schema.GroupKind {
	return schema.GroupKind{Group: q.Group, Kind: q.Kind}
}

// Validate checks for invalid field combinations
func (q ResourceQuery) Validate() error {
	if q.Kind == "" {
		return fmt.Errorf("kind is required")
	}
	if q.Name != "" && q.LabelSelector != "" {
		return fmt.Errorf("name and labelSelector are mutually exclusive")
	}
	if q.Name != "" && q.Namespace == AllNamespaces {
		return fmt.Errorf("cannot get by name across all namespaces")
	}
	return nil
}
