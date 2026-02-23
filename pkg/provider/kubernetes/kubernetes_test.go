package kubernetes_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	k8sruntime "k8s.io/apimachinery/pkg/runtime"

	itestutil "github.com/isometry/platform-health/internal/testutil"
	"github.com/isometry/platform-health/pkg/checks"
	ph "github.com/isometry/platform-health/pkg/platform_health"
	"github.com/isometry/platform-health/pkg/provider/kubernetes"
	"github.com/isometry/platform-health/pkg/provider/kubernetes/testutil"
)

// getTestdataPath returns the path to the testdata directory
func getTestdataPath(t *testing.T) string {
	t.Helper()
	return itestutil.TestdataPath(t)
}

// loadDeploymentFixture loads a deployment JSON fixture and returns an unstructured object
func loadDeploymentFixture(t *testing.T, filename string) *unstructured.Unstructured {
	t.Helper()
	path := filepath.Join(getTestdataPath(t), filename)
	data, err := os.ReadFile(path)
	require.NoError(t, err, "failed to read fixture %s", filename)

	var obj map[string]any
	require.NoError(t, json.Unmarshal(data, &obj), "failed to unmarshal fixture")

	// Convert float64 to int64 for numeric fields (JSON unmarshals numbers as float64)
	convertNumericFields(obj)

	return &unstructured.Unstructured{Object: obj}
}

// convertNumericFields recursively converts float64 to int64 where appropriate
func convertNumericFields(obj map[string]any) {
	for k, v := range obj {
		switch val := v.(type) {
		case float64:
			// Convert to int64 if it's a whole number
			if val == float64(int64(val)) {
				obj[k] = int64(val)
			}
		case map[string]any:
			convertNumericFields(val)
		case []any:
			for i, item := range val {
				if m, ok := item.(map[string]any); ok {
					convertNumericFields(m)
				} else if f, ok := item.(float64); ok && f == float64(int64(f)) {
					val[i] = int64(f)
				}
			}
		}
	}
}

// loadDeploymentFixtureWithName loads a fixture and customizes name/namespace
func loadDeploymentFixtureWithName(t *testing.T, filename, name, namespace string) *unstructured.Unstructured {
	t.Helper()
	u := loadDeploymentFixture(t, filename)
	u.SetName(name)
	u.SetNamespace(namespace)
	return u
}

// testDeployment creates a test deployment using the testutil builder
func testDeployment(name, namespace string, ready bool) *unstructured.Unstructured {
	b := testutil.NewDeployment(name, namespace)
	if !ready {
		b.Unhealthy()
	}
	return b.Build()
}

// setupMockFactory sets up a mock factory with the given resources
func setupMockFactory(t *testing.T, objects ...k8sruntime.Object) {
	t.Helper()
	testutil.NewMockFactory().WithObjects(objects...).Install(t)
}

func TestCheckByName_Healthy(t *testing.T) {
	setupMockFactory(t, testDeployment("my-app", "default", true))

	instance := &kubernetes.Component{
		Kind:      "Deployment",
		Namespace: "default",
		Name:      "my-app",
	}
	instance.SetName("test-provider")
	require.NoError(t, instance.Setup())

	result := instance.GetHealth(t.Context())
	assert.Equal(t, ph.Status_HEALTHY, result.Status)
	assert.Equal(t, "test-provider", result.Name)
	assert.Equal(t, kubernetes.ProviderType, result.Type)
}

func TestCheckByName_NotFound(t *testing.T) {
	setupMockFactory(t)

	instance := &kubernetes.Component{
		Kind:      "Deployment",
		Namespace: "default",
		Name:      "nonexistent",
	}
	require.NoError(t, instance.Setup())

	result := instance.GetHealth(t.Context())
	assert.Equal(t, ph.Status_UNHEALTHY, result.Status)
}

func TestCheckBySelector_MultipleResources(t *testing.T) {
	setupMockFactory(t,
		testDeployment("app-1", "default", true),
		testDeployment("app-2", "default", true),
		testDeployment("app-3", "default", true),
	)

	// Empty selector = all resources (fake client returns all objects anyway)
	instance := &kubernetes.Component{
		Kind:      "Deployment",
		Namespace: "default",
		// Note: fake client doesn't filter by labelSelector, so use empty selector
	}
	require.NoError(t, instance.Setup())

	result := instance.GetHealth(t.Context())
	assert.Equal(t, ph.Status_HEALTHY, result.Status)
	assert.Equal(t, kubernetes.ProviderType, result.Type)
	assert.Len(t, result.Components, 3)
	for _, comp := range result.Components {
		assert.Equal(t, kubernetes.ProviderType, comp.Type)
	}
}

func TestCheckBySelector_EmptyResult(t *testing.T) {
	setupMockFactory(t)
	// Empty result is HEALTHY by default (use CEL checks to require resources)
	instance := &kubernetes.Component{
		Kind:          "Deployment",
		Namespace:     "default",
		LabelSelector: "app=nonexistent",
	}
	require.NoError(t, instance.Setup())

	result := instance.GetHealth(t.Context())
	assert.Equal(t, ph.Status_HEALTHY, result.Status)
	assert.Len(t, result.Components, 0)
}

func TestCheckBySelector_RequireAtLeastOne(t *testing.T) {
	setupMockFactory(t)

	// Use CEL check to require at least one resource
	instance := &kubernetes.Component{
		Kind:          "Deployment",
		Namespace:     "default",
		LabelSelector: "app=nonexistent",
	}
	require.NoError(t, instance.Setup())
	require.NoError(t, instance.SetChecks([]checks.Expression{
		{Expression: "items.size() >= 1", Message: "No resources found"},
	}))

	result := instance.GetHealth(t.Context())
	assert.Equal(t, ph.Status_UNHEALTHY, result.Status)
	require.NotEmpty(t, result.Messages)
	assert.Contains(t, result.Messages[0], "No resources found")
}

func TestCheckBySelector_EmptySelector(t *testing.T) {
	setupMockFactory(t,
		testDeployment("app-1", "default", true),
		testDeployment("app-2", "default", true),
	)

	// Empty selector = all resources
	instance := &kubernetes.Component{
		Kind:      "Deployment",
		Namespace: "default",
	}
	require.NoError(t, instance.Setup())

	result := instance.GetHealth(t.Context())
	assert.Equal(t, ph.Status_HEALTHY, result.Status)
	assert.Len(t, result.Components, 2)
}

func TestCELChecks_SingleResource(t *testing.T) {
	setupMockFactory(t, testDeployment("my-app", "default", true))

	instance := &kubernetes.Component{
		Kind:      "Deployment",
		Namespace: "default",
		Name:      "my-app",
	}
	require.NoError(t, instance.Setup())
	require.NoError(t, instance.SetChecks([]checks.Expression{
		{Expression: "resource.status.readyReplicas >= resource.spec.replicas"},
	}))

	result := instance.GetHealth(t.Context())
	assert.Equal(t, ph.Status_HEALTHY, result.Status)
}

func TestCELChecks_ItemsList(t *testing.T) {
	setupMockFactory(t,
		testDeployment("app-1", "default", true),
		testDeployment("app-2", "default", true),
		testDeployment("app-3", "default", true),
	)

	instance := &kubernetes.Component{
		Kind:      "Deployment",
		Namespace: "default",
	}
	require.NoError(t, instance.Setup())
	require.NoError(t, instance.SetChecks([]checks.Expression{
		{Expression: "items.size() >= 3", Message: "Need at least 3 deployments"},
	}))

	result := instance.GetHealth(t.Context())
	assert.Equal(t, ph.Status_HEALTHY, result.Status)
}

func TestCELChecks_ItemsListFails(t *testing.T) {
	setupMockFactory(t,
		testDeployment("app-1", "default", true),
	)

	instance := &kubernetes.Component{
		Kind:      "Deployment",
		Namespace: "default",
	}
	require.NoError(t, instance.Setup())
	require.NoError(t, instance.SetChecks([]checks.Expression{
		{Expression: "items.size() >= 3", Message: "Need at least 3 deployments"},
	}))

	result := instance.GetHealth(t.Context())
	assert.Equal(t, ph.Status_UNHEALTHY, result.Status)
	require.NotEmpty(t, result.Messages)
	assert.Contains(t, result.Messages[0], "Need at least 3 deployments")
}

func TestCELChecks_PerItemPasses(t *testing.T) {
	setupMockFactory(t,
		testDeployment("app-1", "default", true),
		testDeployment("app-2", "default", true),
		testDeployment("app-3", "default", true),
	)

	instance := &kubernetes.Component{
		Kind:      "Deployment",
		Namespace: "default",
	}
	require.NoError(t, instance.Setup())
	require.NoError(t, instance.SetChecks([]checks.Expression{
		{Expression: "resource.status.readyReplicas >= resource.spec.replicas", Mode: "each"},
	}))

	result := instance.GetHealth(t.Context())
	assert.Equal(t, ph.Status_HEALTHY, result.Status)
	assert.Len(t, result.Components, 3)
	for _, comp := range result.Components {
		assert.Equal(t, ph.Status_HEALTHY, comp.Status)
	}
}

func TestCELChecks_PerItemFails(t *testing.T) {
	setupMockFactory(t,
		testDeployment("app-1", "default", true),
		testDeployment("app-2", "default", false), // This one will fail the per-item check
		testDeployment("app-3", "default", true),
	)

	instance := &kubernetes.Component{
		Kind:      "Deployment",
		Namespace: "default",
	}
	kstatusDisabled := false
	instance.KStatus = &kstatusDisabled // Disable kstatus to isolate CEL check behavior
	require.NoError(t, instance.Setup())
	require.NoError(t, instance.SetChecks([]checks.Expression{
		{Expression: "resource.status.readyReplicas >= resource.spec.replicas", Message: "Deployment not ready", Mode: "each"},
	}))

	result := instance.GetHealth(t.Context())
	assert.Equal(t, ph.Status_UNHEALTHY, result.Status)
	assert.Len(t, result.Components, 3)

	// Find the failing component and verify failure attribution
	var foundFailure bool
	for _, comp := range result.Components {
		if comp.Name == "app-2" {
			assert.Equal(t, ph.Status_UNHEALTHY, comp.Status)
			require.NotEmpty(t, comp.Messages)
			assert.Contains(t, comp.Messages[0], "Deployment not ready")
			foundFailure = true
		} else {
			assert.Equal(t, ph.Status_HEALTHY, comp.Status)
		}
	}
	assert.True(t, foundFailure, "Expected app-2 to be marked unhealthy")
}

func TestCELChecks_MixedModes(t *testing.T) {
	setupMockFactory(t,
		testDeployment("app-1", "default", true),
		testDeployment("app-2", "default", true),
		testDeployment("app-3", "default", true),
	)

	instance := &kubernetes.Component{
		Kind:      "Deployment",
		Namespace: "default",
	}
	require.NoError(t, instance.Setup())
	require.NoError(t, instance.SetChecks([]checks.Expression{
		// Aggregate check against items list
		{Expression: "items.size() >= 3", Message: "Need at least 3 deployments"},
		// Per-item check against each resource
		{Expression: "resource.status.readyReplicas >= resource.spec.replicas", Message: "Deployment not ready", Mode: "each"},
	}))

	result := instance.GetHealth(t.Context())
	assert.Equal(t, ph.Status_HEALTHY, result.Status)
	assert.Len(t, result.Components, 3)
}

func TestCELChecks_MixedModes_AggregateFailure(t *testing.T) {
	setupMockFactory(t,
		testDeployment("app-1", "default", true),
		testDeployment("app-2", "default", true),
	)

	instance := &kubernetes.Component{
		Kind:      "Deployment",
		Namespace: "default",
	}
	require.NoError(t, instance.Setup())
	require.NoError(t, instance.SetChecks([]checks.Expression{
		// This aggregate check will fail (only 2 items)
		{Expression: "items.size() >= 3", Message: "Need at least 3 deployments"},
		// Per-item checks will pass
		{Expression: "resource.status.readyReplicas >= resource.spec.replicas", Mode: "each"},
	}))

	result := instance.GetHealth(t.Context())
	// Aggregate failure should appear on parent component
	assert.Equal(t, ph.Status_UNHEALTHY, result.Status)
	require.NotEmpty(t, result.Messages)
	assert.Contains(t, result.Messages[0], "Need at least 3 deployments")
}

func TestSetup_MutuallyExclusiveNameAndSelector(t *testing.T) {
	instance := &kubernetes.Component{
		Kind:          "Deployment",
		Name:          "my-app",
		LabelSelector: "app=myapp",
	}
	err := instance.Setup()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "mutually exclusive")
}

// Fixture-based tests

func TestCheckByName_HealthyFixture(t *testing.T) {
	setupMockFactory(t, loadDeploymentFixture(t, "deployment-ready.json"))

	instance := &kubernetes.Component{
		Kind:      "Deployment",
		Namespace: "default",
		Name:      "my-app",
	}
	instance.SetName("test-provider")
	require.NoError(t, instance.Setup())

	result := instance.GetHealth(t.Context())
	assert.Equal(t, ph.Status_HEALTHY, result.Status)
	assert.Equal(t, "test-provider", result.Name)
}

func TestCheckByName_UnhealthyFixture(t *testing.T) {
	setupMockFactory(t, loadDeploymentFixture(t, "deployment-not-ready.json"))

	instance := &kubernetes.Component{
		Kind:      "Deployment",
		Namespace: "default",
		Name:      "my-app",
	}
	instance.SetName("test-provider")
	require.NoError(t, instance.Setup())

	result := instance.GetHealth(t.Context())
	assert.Equal(t, ph.Status_UNHEALTHY, result.Status)
}

func TestCheckBySelector_MultipleFixtures(t *testing.T) {
	setupMockFactory(t,
		loadDeploymentFixtureWithName(t, "deployment-ready.json", "app-1", "default"),
		loadDeploymentFixtureWithName(t, "deployment-ready.json", "app-2", "default"),
		loadDeploymentFixtureWithName(t, "deployment-ready.json", "app-3", "default"),
	)

	instance := &kubernetes.Component{
		Kind:      "Deployment",
		Namespace: "default",
	}
	require.NoError(t, instance.Setup())

	result := instance.GetHealth(t.Context())
	assert.Equal(t, ph.Status_HEALTHY, result.Status)
	assert.Len(t, result.Components, 3)
}

func TestSummarize_AllHealthy(t *testing.T) {
	setupMockFactory(t,
		testDeployment("app-1", "default", true),
		testDeployment("app-2", "default", true),
		testDeployment("app-3", "default", true),
	)

	instance := &kubernetes.Component{
		Kind:      "Deployment",
		Namespace: "default",
		Summarize: true,
	}
	require.NoError(t, instance.Setup())

	result := instance.GetHealth(t.Context())
	assert.Equal(t, ph.Status_HEALTHY, result.Status)
	assert.Empty(t, result.Components, "Summarize mode should not have components")
	assert.Empty(t, result.Messages, "All healthy should have no messages")
}

func TestSummarize_WithErrors(t *testing.T) {
	setupMockFactory(t,
		testDeployment("app-1", "default", true),
		testDeployment("app-2", "default", false), // Unhealthy
		testDeployment("app-3", "default", true),
	)

	instance := &kubernetes.Component{
		Kind:      "Deployment",
		Namespace: "default",
		Summarize: true,
	}
	kstatusDisabled := false
	instance.KStatus = &kstatusDisabled // Disable kstatus to isolate CEL check behavior
	require.NoError(t, instance.Setup())
	require.NoError(t, instance.SetChecks([]checks.Expression{
		{Expression: "resource.status.readyReplicas >= resource.spec.replicas", Message: "Deployment not ready", Mode: "each"},
	}))

	result := instance.GetHealth(t.Context())
	assert.Equal(t, ph.Status_UNHEALTHY, result.Status)
	assert.Empty(t, result.Components, "Summarize mode should not have components")
	require.Len(t, result.Messages, 1, "Should have one error message")
	assert.Contains(t, result.Messages[0], "app-2@default")
	assert.Contains(t, result.Messages[0], "Deployment not ready")
}

func TestSummarize_MultipleErrors(t *testing.T) {
	setupMockFactory(t,
		testDeployment("app-1", "default", false), // Unhealthy
		testDeployment("app-2", "default", false), // Unhealthy
		testDeployment("app-3", "default", true),
	)

	instance := &kubernetes.Component{
		Kind:      "Deployment",
		Namespace: "default",
		Summarize: true,
	}
	kstatusDisabled := false
	instance.KStatus = &kstatusDisabled
	require.NoError(t, instance.Setup())
	require.NoError(t, instance.SetChecks([]checks.Expression{
		{Expression: "resource.status.readyReplicas >= resource.spec.replicas", Message: "Deployment not ready", Mode: "each"},
	}))

	result := instance.GetHealth(t.Context())
	assert.Equal(t, ph.Status_UNHEALTHY, result.Status)
	assert.Empty(t, result.Components)
	require.Len(t, result.Messages, 2, "Should have two error messages")
	// Messages should be for app-1 and app-2
	assert.Contains(t, result.Messages[0], "app-1@default")
	assert.Contains(t, result.Messages[1], "app-2@default")
}

func TestSummarize_False_PreservesComponents(t *testing.T) {
	setupMockFactory(t,
		testDeployment("app-1", "default", true),
		testDeployment("app-2", "default", true),
	)

	instance := &kubernetes.Component{
		Kind:      "Deployment",
		Namespace: "default",
		Summarize: false, // Explicit default
	}
	require.NoError(t, instance.Setup())

	result := instance.GetHealth(t.Context())
	assert.Equal(t, ph.Status_HEALTHY, result.Status)
	assert.Len(t, result.Components, 2, "Default mode should have components")
	assert.Empty(t, result.Messages)
}
