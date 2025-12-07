package helm_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	chart "helm.sh/helm/v4/pkg/chart/v2"
	"helm.sh/helm/v4/pkg/release/common"
	release "helm.sh/helm/v4/pkg/release/v1"

	"github.com/isometry/platform-health/pkg/checks"
	ph "github.com/isometry/platform-health/pkg/platform_health"
	"github.com/isometry/platform-health/pkg/provider"
	"github.com/isometry/platform-health/pkg/provider/helm"
	"github.com/isometry/platform-health/pkg/provider/helm/client"
)

// getTestdataPath returns the path to the testdata directory
func getTestdataPath(t *testing.T) string {
	t.Helper()
	_, filename, _, ok := runtime.Caller(0)
	require.True(t, ok, "failed to get caller info")
	return filepath.Join(filepath.Dir(filename), "testdata")
}

// loadManifestFixture loads a manifest YAML file from testdata
func loadManifestFixture(t *testing.T, filename string) string {
	t.Helper()
	path := filepath.Join(getTestdataPath(t), filename)
	data, err := os.ReadFile(path)
	require.NoError(t, err, "failed to read fixture %s", filename)
	return string(data)
}

// setupMockFactory sets up a mock factory with the given release and error
func setupMockFactory(t *testing.T, rel *release.Release, err error) {
	t.Helper()
	client.ClientFactory = &client.MockHelmFactory{
		Runner: &client.MockStatusRunner{
			Release: rel,
			Err:     err,
		},
	}

	t.Cleanup(func() {
		client.ClientFactory = &client.DefaultHelmFactory{}
	})
}

// testRelease creates a test release with the given status
func testRelease(name string, status common.Status) *release.Release {
	return &release.Release{
		Name:      name,
		Namespace: "default",
		Version:   1,
		Info: &release.Info{
			Status:        status,
			FirstDeployed: time.Now().Add(-24 * time.Hour),
			LastDeployed:  time.Now(),
			Description:   "Install complete",
		},
		Chart: &chart.Chart{
			Metadata: &chart.Metadata{
				Name:       "test-chart",
				Version:    "1.0.0",
				AppVersion: "2.0.0",
				Deprecated: false,
			},
			Values: map[string]any{
				"replicas": 1,
				"image": map[string]any{
					"repository": "nginx",
					"tag":        "latest",
				},
				"service": map[string]any{
					"type": "ClusterIP",
					"port": 80,
				},
			},
		},
		Config: map[string]any{
			"replicas": 3,
			"image": map[string]any{
				"tag": "v1.0.0",
			},
		},
		Labels: map[string]string{
			"team": "platform",
			"env":  "prod",
		},
	}
}

func TestHelm_StatusVariants(t *testing.T) {
	tests := []struct {
		name           string
		status         common.Status
		expectedStatus ph.Status
		expectContains string
	}{
		{"deployed", common.StatusDeployed, ph.Status_HEALTHY, ""},
		{"failed", common.StatusFailed, ph.Status_UNHEALTHY, "failed"},
		{"pending_install", common.StatusPendingInstall, ph.Status_UNHEALTHY, "pending-install"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			setupMockFactory(t, testRelease("my-release", tt.status), nil)

			instance := &helm.Component{
				Release:   "my-release",
				Namespace: "default",
			}
			instance.SetName("test-helm")
			require.NoError(t, instance.Setup())

			result := instance.GetHealth(t.Context())
			assert.Equal(t, tt.expectedStatus, result.Status)
			assert.Equal(t, "test-helm", result.Name)
			if tt.expectContains != "" {
				require.NotEmpty(t, result.Messages)
				assert.Contains(t, result.Messages[0], tt.expectContains)
			}
		})
	}
}

func TestHelm_ReleaseNotFound(t *testing.T) {
	setupMockFactory(t, nil, errors.New("release: not found"))

	instance := &helm.Component{
		Release:   "nonexistent",
		Namespace: "default",
	}
	instance.SetName("test-helm")
	require.NoError(t, instance.Setup())

	result := instance.GetHealth(t.Context())
	assert.Equal(t, ph.Status_UNHEALTHY, result.Status)
	require.NotEmpty(t, result.Messages)
	assert.Contains(t, result.Messages[0], "not found")
}

func TestHelm_FactoryError(t *testing.T) {
	client.ClientFactory = &client.MockHelmFactory{
		Runner: nil,
		Err:    errors.New("failed to initialize helm"),
	}
	t.Cleanup(func() {
		client.ClientFactory = &client.DefaultHelmFactory{}
	})

	instance := &helm.Component{
		Release:   "my-release",
		Namespace: "default",
	}
	instance.SetName("test-helm")
	require.NoError(t, instance.Setup())

	result := instance.GetHealth(t.Context())
	assert.Equal(t, ph.Status_UNHEALTHY, result.Status)
	require.NotEmpty(t, result.Messages)
	assert.Contains(t, result.Messages[0], "failed to initialize helm")
}

func TestHelm_Timeout(t *testing.T) {
	slowRunner := &slowStatusRunner{
		delay:   200 * time.Millisecond,
		release: testRelease("my-release", common.StatusDeployed),
	}
	client.ClientFactory = &client.MockHelmFactory{
		Runner: slowRunner,
	}
	t.Cleanup(func() {
		client.ClientFactory = &client.DefaultHelmFactory{}
	})

	instance := &helm.Component{
		Release:   "my-release",
		Namespace: "default",
	}
	instance.SetName("test-helm")
	instance.SetTimeout(50 * time.Millisecond)
	require.NoError(t, instance.Setup())

	// Use GetHealthWithDuration which applies the timeout
	result := provider.GetHealthWithDuration(t.Context(), instance)
	assert.Equal(t, ph.Status_UNHEALTHY, result.Status)
	require.NotEmpty(t, result.Messages)
	assert.Contains(t, result.Messages[0], "context deadline exceeded")
}

func TestSetup_NoDefaultTimeout(t *testing.T) {
	// Timeout is no longer set by Setup() - it's set via config or global flag
	instance := &helm.Component{
		Release:   "my-release",
		Namespace: "default",
	}
	require.NoError(t, instance.Setup())

	// No default timeout - it's now set externally
	assert.Equal(t, time.Duration(0), instance.GetTimeout())
}

func TestGetType(t *testing.T) {
	instance := &helm.Component{}
	assert.Equal(t, "helm", instance.GetType())
}

func TestGetSetName(t *testing.T) {
	instance := &helm.Component{}
	instance.SetName("test")
	assert.Equal(t, "test", instance.GetName())
}

// CEL Tests

func TestCEL_VersionCheck(t *testing.T) {
	rel := testRelease("my-release", common.StatusDeployed)
	rel.Version = 3
	setupMockFactory(t, rel, nil)

	instance := &helm.Component{
		Release:   "my-release",
		Namespace: "default",
	}
	instance.SetName("test-helm")
	require.NoError(t, instance.Setup())
	require.NoError(t, instance.SetChecks([]checks.Expression{
		{Expression: "release.Revision >= 2", Message: "Need at least one upgrade"},
	}))

	result := instance.GetHealth(t.Context())
	assert.Equal(t, ph.Status_HEALTHY, result.Status)
}

func TestCEL_VersionCheckFails(t *testing.T) {
	rel := testRelease("my-release", common.StatusDeployed)
	rel.Version = 1
	setupMockFactory(t, rel, nil)

	instance := &helm.Component{
		Release:   "my-release",
		Namespace: "default",
	}
	instance.SetName("test-helm")
	require.NoError(t, instance.Setup())
	require.NoError(t, instance.SetChecks([]checks.Expression{
		{Expression: "release.Revision >= 2", Message: "Need at least one upgrade"},
	}))

	result := instance.GetHealth(t.Context())
	assert.Equal(t, ph.Status_UNHEALTHY, result.Status)
	require.NotEmpty(t, result.Messages)
	assert.Contains(t, result.Messages[0], "Need at least one upgrade")
}

func TestCEL_ChartVersion(t *testing.T) {
	setupMockFactory(t, testRelease("my-release", common.StatusDeployed), nil)

	instance := &helm.Component{
		Release:   "my-release",
		Namespace: "default",
	}
	instance.SetName("test-helm")
	require.NoError(t, instance.Setup())
	require.NoError(t, instance.SetChecks([]checks.Expression{
		{Expression: "chart.Version == '1.0.0'"},
	}))

	result := instance.GetHealth(t.Context())
	assert.Equal(t, ph.Status_HEALTHY, result.Status)
}

func TestCEL_ConfigValidation(t *testing.T) {
	setupMockFactory(t, testRelease("my-release", common.StatusDeployed), nil)

	instance := &helm.Component{
		Release:   "my-release",
		Namespace: "default",
	}
	instance.SetName("test-helm")
	require.NoError(t, instance.Setup())
	require.NoError(t, instance.SetChecks([]checks.Expression{
		{Expression: "'replicas' in release.Config && release.Config['replicas'] >= 3"},
	}))

	result := instance.GetHealth(t.Context())
	assert.Equal(t, ph.Status_HEALTHY, result.Status)
}

func TestCEL_ConfigValidationFails(t *testing.T) {
	rel := testRelease("my-release", common.StatusDeployed)
	rel.Config["replicas"] = 1
	setupMockFactory(t, rel, nil)

	instance := &helm.Component{
		Release:   "my-release",
		Namespace: "default",
	}
	instance.SetName("test-helm")
	require.NoError(t, instance.Setup())
	require.NoError(t, instance.SetChecks([]checks.Expression{
		{Expression: "release.Config['replicas'] >= 3", Message: "Need at least 3 replicas"},
	}))

	result := instance.GetHealth(t.Context())
	assert.Equal(t, ph.Status_UNHEALTHY, result.Status)
	require.NotEmpty(t, result.Messages)
	assert.Contains(t, result.Messages[0], "Need at least 3 replicas")
}

func TestCEL_NotDeprecated(t *testing.T) {
	setupMockFactory(t, testRelease("my-release", common.StatusDeployed), nil)

	instance := &helm.Component{
		Release:   "my-release",
		Namespace: "default",
	}
	instance.SetName("test-helm")
	require.NoError(t, instance.Setup())
	require.NoError(t, instance.SetChecks([]checks.Expression{
		{Expression: "!chart.Deprecated", Message: "Chart is deprecated"},
	}))

	result := instance.GetHealth(t.Context())
	assert.Equal(t, ph.Status_HEALTHY, result.Status)
}

func TestCEL_LabelCheck(t *testing.T) {
	setupMockFactory(t, testRelease("my-release", common.StatusDeployed), nil)

	instance := &helm.Component{
		Release:   "my-release",
		Namespace: "default",
	}
	instance.SetName("test-helm")
	require.NoError(t, instance.Setup())
	require.NoError(t, instance.SetChecks([]checks.Expression{
		{Expression: "'team' in release.Labels && 'env' in release.Labels"},
	}))

	result := instance.GetHealth(t.Context())
	assert.Equal(t, ph.Status_HEALTHY, result.Status)
}

func TestCEL_ChartValues(t *testing.T) {
	setupMockFactory(t, testRelease("my-release", common.StatusDeployed), nil)

	// Test that Values contains chart defaults (not merged with Config)
	// Config has overrides, Values has chart defaults
	instance := &helm.Component{
		Release:   "my-release",
		Namespace: "default",
	}
	instance.SetName("test-helm")
	require.NoError(t, instance.Setup())
	require.NoError(t, instance.SetChecks([]checks.Expression{
		// Check chart default value
		{Expression: "chart.Values['replicas'] == 1"},
		// Check nested default value
		{Expression: "'image' in chart.Values && chart.Values['image']['tag'] == 'latest'"},
		// Check Config has overrides
		{Expression: "release.Config['replicas'] == 3"},
	}))

	result := instance.GetHealth(t.Context())
	assert.Equal(t, ph.Status_HEALTHY, result.Status)
}

func TestSetup_InvalidCEL(t *testing.T) {
	instance := &helm.Component{
		Release:   "my-release",
		Namespace: "default",
	}
	require.NoError(t, instance.Setup())

	// SetChecks should fail with invalid CEL expression
	err := instance.SetChecks([]checks.Expression{
		{Expression: "invalid cel syntax [[["},
	})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "invalid CEL expression")
}

func TestCEL_Manifests(t *testing.T) {
	rel := testRelease("my-release", common.StatusDeployed)
	rel.Manifest = `---
apiVersion: v1
kind: ConfigMap
metadata:
  name: my-config
data:
  key: value
---
apiVersion: apps/v1
kind: Deployment
metadata:
  name: my-app
spec:
  replicas: 3
`
	setupMockFactory(t, rel, nil)

	instance := &helm.Component{
		Release:   "my-release",
		Namespace: "default",
	}
	instance.SetName("test-helm")
	require.NoError(t, instance.Setup())
	require.NoError(t, instance.SetChecks([]checks.Expression{
		{Expression: "size(release.Manifest) == 2"},
		{Expression: "release.Manifest[0].kind == 'ConfigMap'"},
		{Expression: "release.Manifest[1].kind == 'Deployment'"},
	}))

	result := instance.GetHealth(t.Context())
	assert.Equal(t, ph.Status_HEALTHY, result.Status)
}

func TestCEL_ManifestsEmpty(t *testing.T) {
	rel := testRelease("my-release", common.StatusDeployed)
	rel.Manifest = ""
	setupMockFactory(t, rel, nil)

	instance := &helm.Component{
		Release:   "my-release",
		Namespace: "default",
	}
	instance.SetName("test-helm")
	require.NoError(t, instance.Setup())
	require.NoError(t, instance.SetChecks([]checks.Expression{
		{Expression: "size(release.Manifest) == 0"},
	}))

	result := instance.GetHealth(t.Context())
	assert.Equal(t, ph.Status_HEALTHY, result.Status)
}

func TestCEL_ManifestsFilter(t *testing.T) {
	rel := testRelease("my-release", common.StatusDeployed)
	rel.Manifest = `---
apiVersion: v1
kind: ConfigMap
metadata:
  name: config-1
---
apiVersion: v1
kind: ConfigMap
metadata:
  name: config-2
---
apiVersion: apps/v1
kind: Deployment
metadata:
  name: my-app
`
	setupMockFactory(t, rel, nil)

	instance := &helm.Component{
		Release:   "my-release",
		Namespace: "default",
	}
	instance.SetName("test-helm")
	require.NoError(t, instance.Setup())
	require.NoError(t, instance.SetChecks([]checks.Expression{
		{Expression: "release.Manifest.filter(m, m.kind == 'ConfigMap').size() == 2"},
		{Expression: "release.Manifest.filter(m, m.kind == 'Deployment').size() == 1"},
	}))

	result := instance.GetHealth(t.Context())
	assert.Equal(t, ph.Status_HEALTHY, result.Status)
}

// slowStatusRunner is a mock that delays before returning
type slowStatusRunner struct {
	delay   time.Duration
	release *release.Release
	err     error
}

func (s *slowStatusRunner) Run(ctx context.Context, name string) (*release.Release, error) {
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-time.After(s.delay):
		return s.release, s.err
	}
}

// Fixture-based tests

func TestCEL_ManifestsFixture(t *testing.T) {
	rel := testRelease("my-release", common.StatusDeployed)
	rel.Manifest = loadManifestFixture(t, "manifests-multi.yaml")
	setupMockFactory(t, rel, nil)

	instance := &helm.Component{
		Release:   "my-release",
		Namespace: "default",
	}
	instance.SetName("test-helm")
	require.NoError(t, instance.Setup())
	require.NoError(t, instance.SetChecks([]checks.Expression{
		{Expression: "size(release.Manifest) == 2"},
		{Expression: "release.Manifest[0].kind == 'ConfigMap'"},
		{Expression: "release.Manifest[1].kind == 'Deployment'"},
	}))

	result := instance.GetHealth(t.Context())
	assert.Equal(t, ph.Status_HEALTHY, result.Status)
}

func TestCEL_ManifestsFilterFixture(t *testing.T) {
	rel := testRelease("my-release", common.StatusDeployed)
	rel.Manifest = loadManifestFixture(t, "manifests-filter.yaml")
	setupMockFactory(t, rel, nil)

	instance := &helm.Component{
		Release:   "my-release",
		Namespace: "default",
	}
	instance.SetName("test-helm")
	require.NoError(t, instance.Setup())
	require.NoError(t, instance.SetChecks([]checks.Expression{
		{Expression: "release.Manifest.filter(m, m.kind == 'ConfigMap').size() == 2"},
		{Expression: "release.Manifest.filter(m, m.kind == 'Deployment').size() == 1"},
	}))

	result := instance.GetHealth(t.Context())
	assert.Equal(t, ph.Status_HEALTHY, result.Status)
}
