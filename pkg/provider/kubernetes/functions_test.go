package kubernetes_test

import (
	"context"
	"testing"

	"github.com/google/cel-go/cel"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	k8sruntime "k8s.io/apimachinery/pkg/runtime"

	"github.com/isometry/platform-health/pkg/checks"
	ph "github.com/isometry/platform-health/pkg/platform_health"
	"github.com/isometry/platform-health/pkg/provider/kubernetes"
	"github.com/isometry/platform-health/pkg/provider/kubernetes/testutil"
)

// setupExtendedMockFactory sets up a mock factory with extended resources
func setupExtendedMockFactory(t *testing.T, objects ...k8sruntime.Object) {
	t.Helper()
	testutil.NewMockFactory().WithObjects(objects...).Install(t)
}

// testPodDisruptionBudget creates a test PDB using the testutil builder
func testPodDisruptionBudget(name, namespace string) *unstructured.Unstructured {
	return testutil.NewPodDisruptionBudget(name, namespace).Build()
}

// testConfigMap creates a test ConfigMap using the testutil builder
func testConfigMap(name, namespace string, data map[string]string) *unstructured.Unstructured {
	return testutil.NewConfigMap(name, namespace).WithData(data).Build()
}

// testNamespace creates a test Namespace using the testutil builder
func testNamespace(name string) *unstructured.Unstructured {
	return testutil.NewNamespace(name).Build()
}

// testDeploymentSimple creates a simple test deployment using the testutil builder
func testDeploymentSimple(name, namespace string) *unstructured.Unstructured {
	return testutil.NewDeployment(name, namespace).Build()
}

func TestKubernetesGet_ExistsCheck(t *testing.T) {
	// Test the common use case: check if a PDB exists for a deployment
	setupExtendedMockFactory(t,
		testDeploymentSimple("my-app", "production"),
		testPodDisruptionBudget("my-app", "production"),
	)

	instance := &kubernetes.Component{
		Kind:      "Deployment",
		Namespace: "production",
		Name:      "my-app",
	}
	instance.SetName("test-provider")
	require.NoError(t, instance.Setup())

	// Check: PDB exists for this deployment
	err := instance.SetChecks([]checks.Expression{
		{Expression: `kubernetes.Get({"kind": "poddisruptionbudget", "namespace": resource.metadata.namespace, "name": resource.metadata.name}) != null`},
	})
	require.NoError(t, err)

	result := instance.GetHealth(t.Context())
	assert.Equal(t, ph.Status_HEALTHY, result.Status)
}

func TestKubernetesGet_NotFound(t *testing.T) {
	// Test that kubernetes.Get returns null when resource doesn't exist
	setupExtendedMockFactory(t,
		testDeploymentSimple("my-app", "production"),
		// No PDB created
	)

	instance := &kubernetes.Component{
		Kind:      "Deployment",
		Namespace: "production",
		Name:      "my-app",
	}
	instance.SetName("test-provider")
	require.NoError(t, instance.Setup())

	// Check: PDB exists for this deployment (should fail)
	err := instance.SetChecks([]checks.Expression{
		{
			Expression: `kubernetes.Get({"kind": "poddisruptionbudget", "namespace": resource.metadata.namespace, "name": resource.metadata.name}) != null`,
			Message:    "Missing PodDisruptionBudget for deployment",
		},
	})
	require.NoError(t, err)

	result := instance.GetHealth(t.Context())
	assert.Equal(t, ph.Status_UNHEALTHY, result.Status)
	assert.Contains(t, result.Messages, "Missing PodDisruptionBudget for deployment")
}

func TestKubernetesGet_ConfigMapData(t *testing.T) {
	// Test accessing data within a fetched resource
	setupExtendedMockFactory(t,
		testDeploymentSimple("my-app", "production"),
		testConfigMap("app-config", "production", map[string]string{
			"DATABASE_URL": "postgres://localhost:5432/db",
			"LOG_LEVEL":    "info",
		}),
	)

	instance := &kubernetes.Component{
		Kind:      "Deployment",
		Namespace: "production",
		Name:      "my-app",
	}
	instance.SetName("test-provider")
	require.NoError(t, instance.Setup())

	// Check: ConfigMap has required key
	err := instance.SetChecks([]checks.Expression{
		{Expression: `kubernetes.Get({"kind": "configmap", "namespace": "production", "name": "app-config"}).data["DATABASE_URL"] != ""`},
	})
	require.NoError(t, err)

	result := instance.GetHealth(t.Context())
	assert.Equal(t, ph.Status_HEALTHY, result.Status)
}

func TestKubernetesGet_ClusterScoped(t *testing.T) {
	// Test looking up cluster-scoped resources (namespace omitted)
	setupExtendedMockFactory(t,
		testDeploymentSimple("my-app", "production"),
		testNamespace("production"),
	)

	instance := &kubernetes.Component{
		Kind:      "Deployment",
		Namespace: "production",
		Name:      "my-app",
	}
	instance.SetName("test-provider")
	require.NoError(t, instance.Setup())

	// Check: Namespace exists
	err := instance.SetChecks([]checks.Expression{
		{Expression: `kubernetes.Get({"kind": "namespace", "name": "production"}) != null`},
	})
	require.NoError(t, err)

	result := instance.GetHealth(t.Context())
	assert.Equal(t, ph.Status_HEALTHY, result.Status)
}

func TestKubernetesGet_GroupAutoResolution(t *testing.T) {
	// Test that group is auto-resolved from commonKindToGroup
	setupExtendedMockFactory(t,
		testDeploymentSimple("my-app", "production"),
		testPodDisruptionBudget("my-app", "production"),
	)

	instance := &kubernetes.Component{
		Kind:      "Deployment",
		Namespace: "production",
		Name:      "my-app",
	}
	instance.SetName("test-provider")
	require.NoError(t, instance.Setup())

	// Check: Using kind only (group should auto-resolve from commonKindToGroup)
	err := instance.SetChecks([]checks.Expression{
		// "poddisruptionbudget" should auto-resolve to group "policy"
		{Expression: `kubernetes.Get({"kind": "poddisruptionbudget", "namespace": "production", "name": "my-app"}) != null`},
	})
	require.NoError(t, err)

	result := instance.GetHealth(t.Context())
	assert.Equal(t, ph.Status_HEALTHY, result.Status)
}

func TestKubernetesGet_CacheHit(t *testing.T) {
	// Test that cache works by using same resource multiple times
	setupExtendedMockFactory(t,
		testDeploymentSimple("my-app", "production"),
		testConfigMap("app-config", "production", map[string]string{
			"KEY1": "value1",
			"KEY2": "value2",
		}),
	)

	instance := &kubernetes.Component{
		Kind:      "Deployment",
		Namespace: "production",
		Name:      "my-app",
	}
	instance.SetName("test-provider")
	require.NoError(t, instance.Setup())

	// Multiple checks accessing the same ConfigMap should use cache
	err := instance.SetChecks([]checks.Expression{
		{Expression: `kubernetes.Get({"kind": "configmap", "namespace": "production", "name": "app-config"}).data["KEY1"] == "value1"`},
		{Expression: `kubernetes.Get({"kind": "configmap", "namespace": "production", "name": "app-config"}).data["KEY2"] == "value2"`},
	})
	require.NoError(t, err)

	result := instance.GetHealth(t.Context())
	assert.Equal(t, ph.Status_HEALTHY, result.Status)
}

func TestKubernetesGet_MissingKind(t *testing.T) {
	// Test that missing kind returns an error
	ctx := context.Background()
	cache := kubernetes.NewResourceCache()

	clients := testutil.NewMockFactory().Build()
	binding := kubernetes.KubernetesGetBinding(ctx, clients, cache)

	// Create CEL environment with the declaration
	env, err := cel.NewEnv(
		kubernetes.KubernetesGetDeclaration(),
	)
	require.NoError(t, err)

	// Compile expression with missing kind
	ast, issues := env.Compile(`kubernetes.Get({"name": "test"})`)
	require.Nil(t, issues.Err())

	// Extend environment with runtime binding
	extendedEnv, err := env.Extend(binding)
	require.NoError(t, err)

	prg, err := extendedEnv.Program(ast)
	require.NoError(t, err)

	_, _, err = prg.Eval(map[string]any{})
	// CEL propagates function errors as evaluation errors
	require.Error(t, err)
	assert.Contains(t, err.Error(), "missing required key")
}

func TestKubernetesGet_MutuallyExclusiveNameAndSelector(t *testing.T) {
	// Test that name and labelSelector are mutually exclusive
	ctx := context.Background()
	cache := kubernetes.NewResourceCache()

	clients := testutil.NewMockFactory().Build()
	binding := kubernetes.KubernetesGetBinding(ctx, clients, cache)

	env, err := cel.NewEnv(
		kubernetes.KubernetesGetDeclaration(),
	)
	require.NoError(t, err)

	ast, issues := env.Compile(`kubernetes.Get({"kind": "pod", "name": "test", "labelSelector": "app=test"})`)
	require.Nil(t, issues.Err())

	// Extend environment with runtime binding
	extendedEnv, err := env.Extend(binding)
	require.NoError(t, err)

	prg, err := extendedEnv.Program(ast)
	require.NoError(t, err)

	_, _, err = prg.Eval(map[string]any{})
	// CEL propagates function errors as evaluation errors
	require.Error(t, err)
	assert.Contains(t, err.Error(), "mutually exclusive")
}
