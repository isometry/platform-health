// Package testutil provides test utilities for the kubernetes provider.
package testutil

import (
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/restmapper"
)

// MapperBuilder creates REST mappers with configurable resources.
type MapperBuilder struct {
	groups []*restmapper.APIGroupResources
}

// NewMapperBuilder creates a new MapperBuilder.
func NewMapperBuilder() *MapperBuilder {
	return &MapperBuilder{}
}

// WithAppsV1 adds apps/v1 resources (Deployment, StatefulSet, DaemonSet, ReplicaSet).
func (b *MapperBuilder) WithAppsV1() *MapperBuilder {
	b.groups = append(b.groups, &restmapper.APIGroupResources{
		Group: metav1.APIGroup{
			Name: "apps",
			Versions: []metav1.GroupVersionForDiscovery{
				{GroupVersion: "apps/v1", Version: "v1"},
			},
		},
		VersionedResources: map[string][]metav1.APIResource{
			"v1": {
				{Name: "deployments", Namespaced: true, Kind: "Deployment"},
				{Name: "statefulsets", Namespaced: true, Kind: "StatefulSet"},
				{Name: "daemonsets", Namespaced: true, Kind: "DaemonSet"},
				{Name: "replicasets", Namespaced: true, Kind: "ReplicaSet"},
			},
		},
	})
	return b
}

// WithCoreV1 adds core v1 resources (Pod, ConfigMap, Secret, Namespace, Service).
func (b *MapperBuilder) WithCoreV1() *MapperBuilder {
	b.groups = append(b.groups, &restmapper.APIGroupResources{
		Group: metav1.APIGroup{
			Name: "",
			Versions: []metav1.GroupVersionForDiscovery{
				{GroupVersion: "v1", Version: "v1"},
			},
		},
		VersionedResources: map[string][]metav1.APIResource{
			"v1": {
				{Name: "pods", Namespaced: true, Kind: "Pod"},
				{Name: "configmaps", Namespaced: true, Kind: "ConfigMap"},
				{Name: "secrets", Namespaced: true, Kind: "Secret"},
				{Name: "namespaces", Namespaced: false, Kind: "Namespace"},
				{Name: "services", Namespaced: true, Kind: "Service"},
			},
		},
	})
	return b
}

// WithPolicyV1 adds policy/v1 resources (PodDisruptionBudget).
func (b *MapperBuilder) WithPolicyV1() *MapperBuilder {
	b.groups = append(b.groups, &restmapper.APIGroupResources{
		Group: metav1.APIGroup{
			Name: "policy",
			Versions: []metav1.GroupVersionForDiscovery{
				{GroupVersion: "policy/v1", Version: "v1"},
			},
		},
		VersionedResources: map[string][]metav1.APIResource{
			"v1": {
				{Name: "poddisruptionbudgets", Namespaced: true, Kind: "PodDisruptionBudget"},
			},
		},
	})
	return b
}

// WithResource adds a custom resource to the mapper.
func (b *MapperBuilder) WithResource(group, version, resource, kind string, namespaced bool) *MapperBuilder {
	// Find existing group or create new one
	for _, g := range b.groups {
		if g.Group.Name == group {
			for v := range g.VersionedResources {
				if v == version {
					g.VersionedResources[v] = append(g.VersionedResources[v],
						metav1.APIResource{Name: resource, Namespaced: namespaced, Kind: kind})
					return b
				}
			}
		}
	}

	// Create new group
	gv := group
	if gv == "" {
		gv = version
	} else {
		gv = group + "/" + version
	}
	b.groups = append(b.groups, &restmapper.APIGroupResources{
		Group: metav1.APIGroup{
			Name: group,
			Versions: []metav1.GroupVersionForDiscovery{
				{GroupVersion: gv, Version: version},
			},
		},
		VersionedResources: map[string][]metav1.APIResource{
			version: {{Name: resource, Namespaced: namespaced, Kind: kind}},
		},
	})
	return b
}

// Build creates the RESTMapper.
func (b *MapperBuilder) Build() meta.RESTMapper {
	return restmapper.NewDiscoveryRESTMapper(b.groups)
}

// StandardMapper returns a mapper with common resources for testing.
// Includes: apps/v1 (Deployment), core/v1 (Pod, ConfigMap, Secret, Namespace).
func StandardMapper() meta.RESTMapper {
	return NewMapperBuilder().
		WithAppsV1().
		WithCoreV1().
		WithPolicyV1().
		Build()
}
