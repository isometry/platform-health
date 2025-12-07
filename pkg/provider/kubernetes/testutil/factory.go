package testutil

import (
	"testing"

	"k8s.io/apimachinery/pkg/api/meta"
	k8sruntime "k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic/fake"
	"k8s.io/client-go/rest"

	"github.com/isometry/platform-health/pkg/provider/kubernetes/client"
)

// MockFactoryBuilder creates mock client factories for testing.
type MockFactoryBuilder struct {
	mapper        meta.RESTMapper
	objects       []k8sruntime.Object
	gvrToListKind map[schema.GroupVersionResource]string
}

// NewMockFactory creates a new MockFactoryBuilder with the standard mapper.
func NewMockFactory() *MockFactoryBuilder {
	return &MockFactoryBuilder{
		mapper:        StandardMapper(),
		gvrToListKind: standardGVRToListKind(),
	}
}

// standardGVRToListKind returns the default GVR to list kind mapping.
func standardGVRToListKind() map[schema.GroupVersionResource]string {
	return map[schema.GroupVersionResource]string{
		{Group: "apps", Version: "v1", Resource: "deployments"}:            "DeploymentList",
		{Group: "apps", Version: "v1", Resource: "statefulsets"}:           "StatefulSetList",
		{Group: "apps", Version: "v1", Resource: "daemonsets"}:             "DaemonSetList",
		{Group: "apps", Version: "v1", Resource: "replicasets"}:            "ReplicaSetList",
		{Group: "", Version: "v1", Resource: "pods"}:                       "PodList",
		{Group: "", Version: "v1", Resource: "configmaps"}:                 "ConfigMapList",
		{Group: "", Version: "v1", Resource: "secrets"}:                    "SecretList",
		{Group: "", Version: "v1", Resource: "namespaces"}:                 "NamespaceList",
		{Group: "", Version: "v1", Resource: "services"}:                   "ServiceList",
		{Group: "policy", Version: "v1", Resource: "poddisruptionbudgets"}: "PodDisruptionBudgetList",
	}
}

// WithMapper sets a custom RESTMapper.
func (b *MockFactoryBuilder) WithMapper(mapper meta.RESTMapper) *MockFactoryBuilder {
	b.mapper = mapper
	return b
}

// WithObjects adds objects to the fake client.
func (b *MockFactoryBuilder) WithObjects(objs ...k8sruntime.Object) *MockFactoryBuilder {
	b.objects = append(b.objects, objs...)
	return b
}

// WithGVRToListKind adds a GVR to list kind mapping (for custom resources).
func (b *MockFactoryBuilder) WithGVRToListKind(gvr schema.GroupVersionResource, listKind string) *MockFactoryBuilder {
	if b.gvrToListKind == nil {
		b.gvrToListKind = make(map[schema.GroupVersionResource]string)
	}
	b.gvrToListKind[gvr] = listKind
	return b
}

// Build creates the KubeClients without installing globally.
func (b *MockFactoryBuilder) Build() *client.KubeClients {
	scheme := k8sruntime.NewScheme()
	fakeClient := fake.NewSimpleDynamicClientWithCustomListKinds(scheme, b.gvrToListKind, b.objects...)

	return &client.KubeClients{
		Config:  &rest.Config{},
		Dynamic: fakeClient,
		Mapper:  b.mapper,
	}
}

// Install sets the global ClientFactory and registers cleanup.
func (b *MockFactoryBuilder) Install(t *testing.T) {
	t.Helper()
	clients := b.Build()

	client.ClientFactory = &client.MockFactory{
		Clients: clients,
	}

	t.Cleanup(func() {
		client.ClientFactory = &client.DefaultFactory{}
	})
}

// InstallWithClients sets the global ClientFactory and returns the clients for direct use.
func (b *MockFactoryBuilder) InstallWithClients(t *testing.T) *client.KubeClients {
	t.Helper()
	clients := b.Build()

	client.ClientFactory = &client.MockFactory{
		Clients: clients,
	}

	t.Cleanup(func() {
		client.ClientFactory = &client.DefaultFactory{}
	})

	return clients
}
