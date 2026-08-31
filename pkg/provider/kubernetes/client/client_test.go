package client

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const testKubeconfig = `apiVersion: v1
kind: Config
clusters:
- name: test
  cluster:
    server: https://127.0.0.1:6443
contexts:
- name: test
  context:
    cluster: test
    user: test
- name: other
  context:
    cluster: test
    user: test
current-context: test
users:
- name: test
  user:
    token: test-token
`

// The client-go defaults of 5 QPS and 10 burst are shared by every component
// on a context, so an unset rate limit is what turns a large estate into scan
// timeouts reported as UNHEALTHY.
func TestGetKubeConfigSetsRateLimits(t *testing.T) {
	path := filepath.Join(t.TempDir(), "kubeconfig")
	require.NoError(t, os.WriteFile(path, []byte(testKubeconfig), 0o600))
	t.Setenv("KUBECONFIG", path)
	t.Setenv("KUBERNETES_SERVICE_HOST", "")
	t.Setenv("KUBERNETES_SERVICE_PORT", "")

	tests := []struct {
		name    string
		context string
	}{
		{name: "current context", context: ""},
		{name: "context override", context: "other"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config, err := GetKubeConfig(tt.context)
			require.NoError(t, err)
			assert.Equal(t, float32(kubeQPS), config.QPS)
			assert.Equal(t, kubeBurst, config.Burst)
			assert.Greater(t, config.QPS, float32(5), "must exceed the client-go default")
		})
	}
}
