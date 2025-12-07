package client

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	k8sruntime "k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic/fake"
	"k8s.io/client-go/restmapper"
)

// testMapper creates a RESTMapper with deployments and pods
func testMapper() meta.RESTMapper {
	return restmapper.NewDiscoveryRESTMapper([]*restmapper.APIGroupResources{
		{
			Group: metav1.APIGroup{
				Name: "apps",
				Versions: []metav1.GroupVersionForDiscovery{
					{GroupVersion: "apps/v1", Version: "v1"},
				},
			},
			VersionedResources: map[string][]metav1.APIResource{
				"v1": {
					{Name: "deployments", Namespaced: true, Kind: "Deployment"},
				},
			},
		},
		{
			Group: metav1.APIGroup{
				Name: "",
				Versions: []metav1.GroupVersionForDiscovery{
					{GroupVersion: "v1", Version: "v1"},
				},
			},
			VersionedResources: map[string][]metav1.APIResource{
				"v1": {
					{Name: "pods", Namespaced: true, Kind: "Pod"},
					{Name: "namespaces", Namespaced: false, Kind: "Namespace"},
				},
			},
		},
	})
}

// testDeployment creates a test deployment
func testDeployment(name, namespace string) *unstructured.Unstructured {
	return &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": "apps/v1",
			"kind":       "Deployment",
			"metadata": map[string]any{
				"name":      name,
				"namespace": namespace,
				"labels": map[string]any{
					"app": name,
				},
			},
		},
	}
}

// testNamespace creates a test namespace (cluster-scoped)
func testNamespace(name string) *unstructured.Unstructured {
	return &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": "v1",
			"kind":       "Namespace",
			"metadata": map[string]any{
				"name": name,
				"labels": map[string]any{
					"env": "test",
				},
			},
		},
	}
}

func newTestFetcher(objects ...k8sruntime.Object) *ResourceFetcher {
	scheme := k8sruntime.NewScheme()
	gvrToListKind := map[schema.GroupVersionResource]string{
		{Group: "apps", Version: "v1", Resource: "deployments"}: "DeploymentList",
		{Group: "", Version: "v1", Resource: "pods"}:            "PodList",
		{Group: "", Version: "v1", Resource: "namespaces"}:      "NamespaceList",
	}
	fakeClient := fake.NewSimpleDynamicClientWithCustomListKinds(scheme, gvrToListKind, objects...)

	clients := &KubeClients{
		Dynamic: fakeClient,
		Mapper:  testMapper(),
	}
	return NewFetcher(clients)
}

func TestResourceFetcher_ResolveMapping(t *testing.T) {
	fetcher := newTestFetcher()

	t.Run("with version", func(t *testing.T) {
		q := ResourceQuery{Group: "apps", Version: "v1", Kind: "Deployment"}
		mapping, err := fetcher.ResolveMapping(q)
		require.NoError(t, err)
		assert.Equal(t, "deployments", mapping.Resource.Resource)
		assert.Equal(t, "apps", mapping.Resource.Group)
	})

	t.Run("without version auto-discovers", func(t *testing.T) {
		q := ResourceQuery{Group: "apps", Kind: "Deployment"}
		mapping, err := fetcher.ResolveMapping(q)
		require.NoError(t, err)
		assert.Equal(t, "v1", mapping.Resource.Version)
	})

	t.Run("unknown kind returns error", func(t *testing.T) {
		q := ResourceQuery{Group: "apps", Kind: "Unknown"}
		_, err := fetcher.ResolveMapping(q)
		assert.Error(t, err)
	})
}

func TestResourceFetcher_Get(t *testing.T) {
	dep := testDeployment("nginx", "default")
	fetcher := newTestFetcher(dep)

	t.Run("get by name", func(t *testing.T) {
		q := ResourceQuery{
			Group:     "apps",
			Kind:      "Deployment",
			Namespace: "default",
			Name:      "nginx",
		}
		result, err := fetcher.Get(context.Background(), q)
		require.NoError(t, err)
		assert.Equal(t, "nginx", result.GetName())
		assert.Equal(t, "default", result.GetNamespace())
	})

	t.Run("get not found", func(t *testing.T) {
		q := ResourceQuery{
			Group:     "apps",
			Kind:      "Deployment",
			Namespace: "default",
			Name:      "nonexistent",
		}
		_, err := fetcher.Get(context.Background(), q)
		assert.Error(t, err)
	})
}

func TestResourceFetcher_Get_ClusterScoped(t *testing.T) {
	ns := testNamespace("my-namespace")
	fetcher := newTestFetcher(ns)

	t.Run("get cluster-scoped resource", func(t *testing.T) {
		q := ResourceQuery{
			Kind: "Namespace",
			Name: "my-namespace",
		}
		result, err := fetcher.Get(context.Background(), q)
		require.NoError(t, err)
		assert.Equal(t, "my-namespace", result.GetName())
	})
}

func TestResourceFetcher_List(t *testing.T) {
	dep1 := testDeployment("nginx", "default")
	dep2 := testDeployment("redis", "default")
	dep3 := testDeployment("mysql", "other")
	fetcher := newTestFetcher(dep1, dep2, dep3)

	t.Run("list in namespace", func(t *testing.T) {
		q := ResourceQuery{
			Group:     "apps",
			Kind:      "Deployment",
			Namespace: "default",
		}
		result, err := fetcher.List(context.Background(), q)
		require.NoError(t, err)
		assert.Len(t, result.Items, 2)
	})

	t.Run("list all namespaces", func(t *testing.T) {
		q := ResourceQuery{
			Group:     "apps",
			Kind:      "Deployment",
			Namespace: AllNamespaces,
		}
		result, err := fetcher.List(context.Background(), q)
		require.NoError(t, err)
		assert.Len(t, result.Items, 3)
	})

	t.Run("list with selector", func(t *testing.T) {
		q := ResourceQuery{
			Group:         "apps",
			Kind:          "Deployment",
			Namespace:     "default",
			LabelSelector: "app=nginx",
		}
		result, err := fetcher.List(context.Background(), q)
		require.NoError(t, err)
		assert.Len(t, result.Items, 1)
		assert.Equal(t, "nginx", result.Items[0].GetName())
	})
}

func TestResourceFetcher_List_ClusterScoped(t *testing.T) {
	ns1 := testNamespace("ns-one")
	ns2 := testNamespace("ns-two")
	fetcher := newTestFetcher(ns1, ns2)

	t.Run("list cluster-scoped resources", func(t *testing.T) {
		q := ResourceQuery{
			Kind:          "Namespace",
			LabelSelector: "env=test",
		}
		result, err := fetcher.List(context.Background(), q)
		require.NoError(t, err)
		assert.Len(t, result.Items, 2)
	})
}

func TestKubeClients_NewFetcher(t *testing.T) {
	clients := &KubeClients{
		Dynamic: nil,
		Mapper:  testMapper(),
	}
	fetcher := clients.NewFetcher()
	assert.NotNil(t, fetcher)
	assert.Equal(t, clients, fetcher.clients)
}
