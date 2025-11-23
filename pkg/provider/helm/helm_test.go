package helm_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	chart "helm.sh/helm/v4/pkg/chart/v2"
	"helm.sh/helm/v4/pkg/release/common"
	release "helm.sh/helm/v4/pkg/release/v1"

	"github.com/isometry/platform-health/pkg/checks"
	ph "github.com/isometry/platform-health/pkg/platform_health"
	"github.com/isometry/platform-health/pkg/provider/helm"
	"github.com/isometry/platform-health/pkg/provider/helm/client"
)

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

			provider := &helm.Helm{
				Name:      "test-helm",
				Release:   "my-release",
				Namespace: "default",
			}
			require.NoError(t, provider.Setup())

			result := provider.GetHealth(context.Background())
			assert.Equal(t, tt.expectedStatus, result.Status)
			assert.Equal(t, "test-helm", result.Name)
			if tt.expectContains != "" {
				assert.Contains(t, result.Message, tt.expectContains)
			}
		})
	}
}

func TestHelm_ReleaseNotFound(t *testing.T) {
	setupMockFactory(t, nil, errors.New("release: not found"))

	provider := &helm.Helm{
		Name:      "test-helm",
		Release:   "nonexistent",
		Namespace: "default",
	}
	require.NoError(t, provider.Setup())

	result := provider.GetHealth(context.Background())
	assert.Equal(t, ph.Status_UNHEALTHY, result.Status)
	assert.Contains(t, result.Message, "not found")
}

func TestHelm_FactoryError(t *testing.T) {
	client.ClientFactory = &client.MockHelmFactory{
		Runner: nil,
		Err:    errors.New("failed to initialize helm"),
	}
	t.Cleanup(func() {
		client.ClientFactory = &client.DefaultHelmFactory{}
	})

	provider := &helm.Helm{
		Name:      "test-helm",
		Release:   "my-release",
		Namespace: "default",
	}
	require.NoError(t, provider.Setup())

	result := provider.GetHealth(context.Background())
	assert.Equal(t, ph.Status_UNHEALTHY, result.Status)
	assert.Contains(t, result.Message, "failed to initialize helm")
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

	provider := &helm.Helm{
		Name:      "test-helm",
		Release:   "my-release",
		Namespace: "default",
		Timeout:   50 * time.Millisecond,
	}
	require.NoError(t, provider.Setup())

	result := provider.GetHealth(context.Background())
	assert.Equal(t, ph.Status_UNHEALTHY, result.Status)
	assert.Equal(t, "timeout", result.Message)
}

func TestSetup_DefaultTimeout(t *testing.T) {
	provider := &helm.Helm{
		Release:   "my-release",
		Namespace: "default",
	}
	require.NoError(t, provider.Setup())

	assert.Equal(t, 5*time.Second, provider.Timeout)
}

func TestGetType(t *testing.T) {
	provider := &helm.Helm{}
	assert.Equal(t, "helm", provider.GetType())
}

func TestGetSetName(t *testing.T) {
	provider := &helm.Helm{}
	provider.SetName("test")
	assert.Equal(t, "test", provider.GetName())
}

// CEL Tests

func TestCEL_VersionCheck(t *testing.T) {
	rel := testRelease("my-release", common.StatusDeployed)
	rel.Version = 3
	setupMockFactory(t, rel, nil)

	provider := &helm.Helm{
		Name:      "test-helm",
		Release:   "my-release",
		Namespace: "default",
		Checks: []checks.Expression{
			{Expression: "release.Revision >= 2", ErrorMessage: "Need at least one upgrade"},
		},
	}
	require.NoError(t, provider.Setup())

	result := provider.GetHealth(context.Background())
	assert.Equal(t, ph.Status_HEALTHY, result.Status)
}

func TestCEL_VersionCheckFails(t *testing.T) {
	rel := testRelease("my-release", common.StatusDeployed)
	rel.Version = 1
	setupMockFactory(t, rel, nil)

	provider := &helm.Helm{
		Name:      "test-helm",
		Release:   "my-release",
		Namespace: "default",
		Checks: []checks.Expression{
			{Expression: "release.Revision >= 2", ErrorMessage: "Need at least one upgrade"},
		},
	}
	require.NoError(t, provider.Setup())

	result := provider.GetHealth(context.Background())
	assert.Equal(t, ph.Status_UNHEALTHY, result.Status)
	assert.Contains(t, result.Message, "Need at least one upgrade")
}

func TestCEL_ChartVersion(t *testing.T) {
	setupMockFactory(t, testRelease("my-release", common.StatusDeployed), nil)

	provider := &helm.Helm{
		Name:      "test-helm",
		Release:   "my-release",
		Namespace: "default",
		Checks: []checks.Expression{
			{Expression: "chart.Version == '1.0.0'"},
		},
	}
	require.NoError(t, provider.Setup())

	result := provider.GetHealth(context.Background())
	assert.Equal(t, ph.Status_HEALTHY, result.Status)
}

func TestCEL_ConfigValidation(t *testing.T) {
	setupMockFactory(t, testRelease("my-release", common.StatusDeployed), nil)

	provider := &helm.Helm{
		Name:      "test-helm",
		Release:   "my-release",
		Namespace: "default",
		Checks: []checks.Expression{
			{Expression: "'replicas' in release.Config && release.Config['replicas'] >= 3"},
		},
	}
	require.NoError(t, provider.Setup())

	result := provider.GetHealth(context.Background())
	assert.Equal(t, ph.Status_HEALTHY, result.Status)
}

func TestCEL_ConfigValidationFails(t *testing.T) {
	rel := testRelease("my-release", common.StatusDeployed)
	rel.Config["replicas"] = 1
	setupMockFactory(t, rel, nil)

	provider := &helm.Helm{
		Name:      "test-helm",
		Release:   "my-release",
		Namespace: "default",
		Checks: []checks.Expression{
			{Expression: "release.Config['replicas'] >= 3", ErrorMessage: "Need at least 3 replicas"},
		},
	}
	require.NoError(t, provider.Setup())

	result := provider.GetHealth(context.Background())
	assert.Equal(t, ph.Status_UNHEALTHY, result.Status)
	assert.Contains(t, result.Message, "Need at least 3 replicas")
}

func TestCEL_NotDeprecated(t *testing.T) {
	setupMockFactory(t, testRelease("my-release", common.StatusDeployed), nil)

	provider := &helm.Helm{
		Name:      "test-helm",
		Release:   "my-release",
		Namespace: "default",
		Checks: []checks.Expression{
			{Expression: "!chart.Deprecated", ErrorMessage: "Chart is deprecated"},
		},
	}
	require.NoError(t, provider.Setup())

	result := provider.GetHealth(context.Background())
	assert.Equal(t, ph.Status_HEALTHY, result.Status)
}

func TestCEL_LabelCheck(t *testing.T) {
	setupMockFactory(t, testRelease("my-release", common.StatusDeployed), nil)

	provider := &helm.Helm{
		Name:      "test-helm",
		Release:   "my-release",
		Namespace: "default",
		Checks: []checks.Expression{
			{Expression: "'team' in release.Labels && 'env' in release.Labels"},
		},
	}
	require.NoError(t, provider.Setup())

	result := provider.GetHealth(context.Background())
	assert.Equal(t, ph.Status_HEALTHY, result.Status)
}

func TestCEL_ChartValues(t *testing.T) {
	setupMockFactory(t, testRelease("my-release", common.StatusDeployed), nil)

	// Test that Values contains chart defaults (not merged with Config)
	// Config has overrides, Values has chart defaults
	provider := &helm.Helm{
		Name:      "test-helm",
		Release:   "my-release",
		Namespace: "default",
		Checks: []checks.Expression{
			// Check chart default value
			{Expression: "chart.Values['replicas'] == 1"},
			// Check nested default value
			{Expression: "'image' in chart.Values && chart.Values['image']['tag'] == 'latest'"},
			// Check Config has overrides
			{Expression: "release.Config['replicas'] == 3"},
		},
	}
	require.NoError(t, provider.Setup())

	result := provider.GetHealth(context.Background())
	assert.Equal(t, ph.Status_HEALTHY, result.Status)
}

func TestSetup_InvalidCEL(t *testing.T) {
	provider := &helm.Helm{
		Release:   "my-release",
		Namespace: "default",
		Checks: []checks.Expression{
			{Expression: "invalid cel syntax [[["},
		},
	}
	err := provider.Setup()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "invalid CEL expression")
}

// slowStatusRunner is a mock that delays before returning
type slowStatusRunner struct {
	delay   time.Duration
	release *release.Release
	err     error
}

func (s *slowStatusRunner) Run(name string) (*release.Release, error) {
	time.Sleep(s.delay)
	return s.release, s.err
}
