package client

import (
	"encoding/base64"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestDefaultHelmFactory_GetStatusRunner_InClusterContextOverride verifies that
// a context override is rejected up front when running in-cluster, before any
// ConfigFlags resolution is attempted. This is the one DefaultHelmFactory path
// that does not require a live cluster or kubeconfig to exercise.
func TestDefaultHelmFactory_GetStatusRunner_InClusterContextOverride(t *testing.T) {
	t.Setenv("KUBERNETES_SERVICE_HOST", "127.0.0.1")
	t.Setenv("KUBERNETES_SERVICE_PORT", "6443")

	factory := &DefaultHelmFactory{}
	runner, err := factory.GetStatusRunner("some-context", "default", nil)

	assert.Nil(t, runner)
	assert.ErrorContains(t, err, "context override not supported when running in-cluster")
}

const fixtureKubeconfigTemplate = `
apiVersion: v1
kind: Config
current-context: current
clusters:
- name: cluster-current
  cluster:
    server: https://current.example.com:6443
- name: cluster-cert
  cluster:
    server: https://target-cert.example.com:6443
    certificate-authority-data: %s
- name: cluster-token
  cluster:
    server: https://target-token.example.com:6443
users:
- name: user-current
  user:
    token: current-context-token
- name: user-cert
  user:
    client-certificate-data: %s
    client-key-data: %s
- name: user-token
  user:
    token: target-context-token
contexts:
- name: current
  context:
    cluster: cluster-current
    user: user-current
- name: target-cert
  context:
    cluster: cluster-cert
    user: user-cert
- name: target-token
  context:
    cluster: cluster-token
    user: user-token
`

// writeFixtureKubeconfig writes a kubeconfig with three contexts: the current
// one, a client-certificate context carrying inline CA/cert/key data, and a
// bearer-token context. All contexts point at different, fake servers so a
// resolved value can only have come from the context that was actually asked
// for.
func writeFixtureKubeconfig(t *testing.T) string {
	t.Helper()

	caData := base64.StdEncoding.EncodeToString([]byte("fake-ca-data"))
	certData := base64.StdEncoding.EncodeToString([]byte("fake-cert-data"))
	keyData := base64.StdEncoding.EncodeToString([]byte("fake-key-data"))
	content := fmt.Sprintf(fixtureKubeconfigTemplate, caData, certData, keyData)

	path := filepath.Join(t.TempDir(), "kubeconfig")
	require.NoError(t, os.WriteFile(path, []byte(content), 0o600))
	return path
}

// TestConfigFlagsFor_ResolvesNamedContextInlineCredentials is the regression
// test for the credential forwarding bug. It calls configFlagsFor, the exact
// function GetStatusRunner uses to build its ConfigFlags, and asserts that
// the flags it returns resolve the named context's own inline CertData,
// KeyData and CAData rather than falling back to the current context.
// Before the fix, GetStatusRunner hand-copied only APIServer, BearerToken
// and CAFile, none of which can carry inline data, so a client-certificate
// context like this one had no usable credential and silently fell back to
// whatever the current context happened to be.
func TestConfigFlagsFor_ResolvesNamedContextInlineCredentials(t *testing.T) {
	t.Setenv("KUBECONFIG", writeFixtureKubeconfig(t))

	kubeConfig := configFlagsFor("target-cert", "default")

	restConfig, err := kubeConfig.ToRESTConfig()
	require.NoError(t, err)

	assert.Equal(t, "https://target-cert.example.com:6443", restConfig.Host)
	assert.Equal(t, []byte("fake-cert-data"), restConfig.CertData)
	assert.Equal(t, []byte("fake-key-data"), restConfig.KeyData)
	assert.Equal(t, []byte("fake-ca-data"), restConfig.CAData)
	assert.Empty(t, restConfig.BearerToken)
}

// TestConfigFlagsFor_ResolvesNamedContextBearerToken covers the other
// credential shape through the same seam: a bearer-token context must still
// resolve its own token, not the current context's, confirming the fix does
// not regress the token-based case the old hand-copied BearerToken field
// used to cover.
func TestConfigFlagsFor_ResolvesNamedContextBearerToken(t *testing.T) {
	t.Setenv("KUBECONFIG", writeFixtureKubeconfig(t))

	kubeConfig := configFlagsFor("target-token", "default")

	restConfig, err := kubeConfig.ToRESTConfig()
	require.NoError(t, err)

	assert.Equal(t, "https://target-token.example.com:6443", restConfig.Host)
	assert.Equal(t, "target-context-token", restConfig.BearerToken)
	assert.NotEqual(t, "current-context-token", restConfig.BearerToken)
}
