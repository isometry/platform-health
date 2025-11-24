package kubernetes_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic/fake"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/restmapper"

	"github.com/isometry/platform-health/pkg/checks"
	ph "github.com/isometry/platform-health/pkg/platform_health"
	"github.com/isometry/platform-health/pkg/provider"
	"github.com/isometry/platform-health/pkg/provider/kubernetes"
	"github.com/isometry/platform-health/pkg/provider/kubernetes/client"
)

// testMapper creates a RESTMapper for testing
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
				},
			},
		},
	})
}

// testDeployment creates a test deployment resource
// kstatus requires specific fields: observedGeneration, replicas, availableReplicas, updatedReplicas, conditions
func testDeployment(name, namespace string, ready bool) *unstructured.Unstructured {
	availableStatus := "True"
	progressingStatus := "True"
	var availableReplicas, updatedReplicas int64 = 3, 3
	if !ready {
		availableStatus = "False"
		progressingStatus = "False"
		availableReplicas = 0
		updatedReplicas = 0
	}
	return &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": "apps/v1",
			"kind":       "Deployment",
			"metadata": map[string]any{
				"name":       name,
				"namespace":  namespace,
				"generation": int64(1),
			},
			"spec": map[string]any{
				"replicas": int64(3),
			},
			"status": map[string]any{
				"observedGeneration": int64(1),
				"replicas":           int64(3),
				"readyReplicas":      availableReplicas,
				"availableReplicas":  availableReplicas,
				"updatedReplicas":    updatedReplicas,
				"conditions": []any{
					map[string]any{
						"type":   "Available",
						"status": availableStatus,
						"reason": "MinimumReplicasAvailable",
					},
					map[string]any{
						"type":   "Progressing",
						"status": progressingStatus,
						"reason": "NewReplicaSetAvailable",
					},
				},
			},
		},
	}
}

// setupMockFactory sets up a mock factory with the given resources
func setupMockFactory(t *testing.T, objects ...runtime.Object) {
	t.Helper()
	scheme := runtime.NewScheme()

	// Register list kinds for the fake client
	gvrToListKind := map[schema.GroupVersionResource]string{
		{Group: "apps", Version: "v1", Resource: "deployments"}: "DeploymentList",
		{Group: "", Version: "v1", Resource: "pods"}:            "PodList",
	}

	fakeClient := fake.NewSimpleDynamicClientWithCustomListKinds(scheme, gvrToListKind, objects...)

	client.ClientFactory = &client.MockFactory{
		Clients: &client.KubeClients{
			Config:  &rest.Config{},
			Dynamic: fakeClient,
			Mapper:  testMapper(),
		},
	}

	t.Cleanup(func() {
		client.ClientFactory = &client.DefaultFactory{}
	})
}

func TestCheckByName_Healthy(t *testing.T) {
	setupMockFactory(t, testDeployment("my-app", "default", true))

	provider := &kubernetes.Kubernetes{
		Name: "test-provider",
		Resource: kubernetes.Resource{
			Kind:      "Deployment",
			Namespace: "default",
			Name:      "my-app",
		},
	}
	require.NoError(t, provider.Setup())

	result := provider.GetHealth(context.Background())
	assert.Equal(t, ph.Status_HEALTHY, result.Status)
	assert.Equal(t, "test-provider", result.Name)
}

func TestCheckByName_NotFound(t *testing.T) {
	setupMockFactory(t)

	provider := &kubernetes.Kubernetes{
		Resource: kubernetes.Resource{
			Kind:      "Deployment",
			Namespace: "default",
			Name:      "nonexistent",
		},
	}
	require.NoError(t, provider.Setup())

	result := provider.GetHealth(context.Background())
	assert.Equal(t, ph.Status_UNHEALTHY, result.Status)
}

func TestCheckBySelector_MultipleResources(t *testing.T) {
	setupMockFactory(t,
		testDeployment("app-1", "default", true),
		testDeployment("app-2", "default", true),
		testDeployment("app-3", "default", true),
	)

	// Empty selector = all resources (fake client returns all objects anyway)
	provider := &kubernetes.Kubernetes{
		Resource: kubernetes.Resource{
			Kind:      "Deployment",
			Namespace: "default",
			// Note: fake client doesn't filter by labelSelector, so use empty selector
		},
	}
	require.NoError(t, provider.Setup())

	result := provider.GetHealth(context.Background())
	assert.Equal(t, ph.Status_HEALTHY, result.Status)
	assert.Len(t, result.Components, 3)
}

func TestCheckBySelector_EmptyResult(t *testing.T) {
	setupMockFactory(t)

	// Empty result is HEALTHY by default (use CEL checks to require resources)
	provider := &kubernetes.Kubernetes{
		Resource: kubernetes.Resource{
			Kind:          "Deployment",
			Namespace:     "default",
			LabelSelector: "app=nonexistent",
		},
	}
	require.NoError(t, provider.Setup())

	result := provider.GetHealth(context.Background())
	assert.Equal(t, ph.Status_HEALTHY, result.Status)
	assert.Len(t, result.Components, 0)
}

func TestCheckBySelector_RequireAtLeastOne(t *testing.T) {
	setupMockFactory(t)

	// Use CEL check to require at least one resource
	provider := &kubernetes.Kubernetes{
		Resource: kubernetes.Resource{
			Kind:          "Deployment",
			Namespace:     "default",
			LabelSelector: "app=nonexistent",
		},
		BaseInstanceWithChecks: provider.BaseInstanceWithChecks{Checks: []checks.Expression{
			{Expression: "items.size() >= 1", Message: "No resources found"},
		}},
	}
	require.NoError(t, provider.Setup())

	result := provider.GetHealth(context.Background())
	assert.Equal(t, ph.Status_UNHEALTHY, result.Status)
	assert.Contains(t, result.Message, "No resources found")
}

func TestCheckBySelector_EmptySelector(t *testing.T) {
	setupMockFactory(t,
		testDeployment("app-1", "default", true),
		testDeployment("app-2", "default", true),
	)

	// Empty selector = all resources
	provider := &kubernetes.Kubernetes{
		Resource: kubernetes.Resource{
			Kind:      "Deployment",
			Namespace: "default",
		},
	}
	require.NoError(t, provider.Setup())

	result := provider.GetHealth(context.Background())
	assert.Equal(t, ph.Status_HEALTHY, result.Status)
	assert.Len(t, result.Components, 2)
}

func TestCELChecks_SingleResource(t *testing.T) {
	setupMockFactory(t, testDeployment("my-app", "default", true))

	provider := &kubernetes.Kubernetes{
		Resource: kubernetes.Resource{
			Kind:      "Deployment",
			Namespace: "default",
			Name:      "my-app",
		},
		BaseInstanceWithChecks: provider.BaseInstanceWithChecks{Checks: []checks.Expression{
			{Expression: "resource.status.readyReplicas >= resource.spec.replicas"},
		}},
	}
	require.NoError(t, provider.Setup())

	result := provider.GetHealth(context.Background())
	assert.Equal(t, ph.Status_HEALTHY, result.Status)
}

func TestCELChecks_ItemsList(t *testing.T) {
	setupMockFactory(t,
		testDeployment("app-1", "default", true),
		testDeployment("app-2", "default", true),
		testDeployment("app-3", "default", true),
	)

	provider := &kubernetes.Kubernetes{
		Resource: kubernetes.Resource{
			Kind:      "Deployment",
			Namespace: "default",
		},
		BaseInstanceWithChecks: provider.BaseInstanceWithChecks{Checks: []checks.Expression{
			{Expression: "items.size() >= 3", Message: "Need at least 3 deployments"},
		}},
	}
	require.NoError(t, provider.Setup())

	result := provider.GetHealth(context.Background())
	assert.Equal(t, ph.Status_HEALTHY, result.Status)
}

func TestCELChecks_ItemsListFails(t *testing.T) {
	setupMockFactory(t,
		testDeployment("app-1", "default", true),
	)

	provider := &kubernetes.Kubernetes{
		Resource: kubernetes.Resource{
			Kind:      "Deployment",
			Namespace: "default",
		},
		BaseInstanceWithChecks: provider.BaseInstanceWithChecks{Checks: []checks.Expression{
			{Expression: "items.size() >= 3", Message: "Need at least 3 deployments"},
		}},
	}
	require.NoError(t, provider.Setup())

	result := provider.GetHealth(context.Background())
	assert.Equal(t, ph.Status_UNHEALTHY, result.Status)
	assert.Contains(t, result.Message, "Need at least 3 deployments")
}

func TestSetup_MutuallyExclusiveNameAndSelector(t *testing.T) {
	provider := &kubernetes.Kubernetes{
		Resource: kubernetes.Resource{
			Kind:          "Deployment",
			Name:          "my-app",
			LabelSelector: "app=myapp",
		},
	}
	err := provider.Setup()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "mutually exclusive")
}
