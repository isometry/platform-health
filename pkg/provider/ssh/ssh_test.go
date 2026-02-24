package ssh_test

import (
	"crypto/ecdsa"
	"crypto/ed25519"
	"crypto/elliptic"
	"crypto/rand"
	"fmt"
	"log/slog"
	"net"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	gossh "golang.org/x/crypto/ssh"

	"github.com/isometry/platform-health/pkg/checks"
	ph "github.com/isometry/platform-health/pkg/platform_health"
	"github.com/isometry/platform-health/pkg/provider"
	"github.com/isometry/platform-health/pkg/provider/ssh"
)

func init() {
	slog.SetLogLoggerLevel(slog.LevelError)
}

// mockSSHServer starts a mock SSH server on a random port with the given host keys.
// It performs key exchange and then rejects authentication, mimicking real SSH servers.
func mockSSHServer(t *testing.T, signers ...gossh.Signer) int {
	t.Helper()

	config := &gossh.ServerConfig{
		NoClientAuth: false,
		PasswordCallback: func(conn gossh.ConnMetadata, password []byte) (*gossh.Permissions, error) {
			return nil, fmt.Errorf("authentication rejected")
		},
	}
	for _, signer := range signers {
		config.AddHostKey(signer)
	}

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	t.Cleanup(func() { _ = listener.Close() })

	go func() {
		for {
			conn, err := listener.Accept()
			if err != nil {
				return // listener closed
			}
			go func(c net.Conn) {
				defer func() { _ = c.Close() }()
				// Perform SSH handshake; auth will fail, which is expected
				_, _, _, _ = gossh.NewServerConn(c, config)
			}(conn)
		}
	}()

	return listener.Addr().(*net.TCPAddr).Port
}

// generateED25519Signer creates an ed25519 SSH host key signer.
func generateED25519Signer(t *testing.T) gossh.Signer {
	t.Helper()
	_, priv, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)
	signer, err := gossh.NewSignerFromKey(priv)
	require.NoError(t, err)
	return signer
}

// generateECDSASigner creates an ECDSA P-256 SSH host key signer.
func generateECDSASigner(t *testing.T) gossh.Signer {
	t.Helper()
	priv, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)
	signer, err := gossh.NewSignerFromKey(priv)
	require.NoError(t, err)
	return signer
}

func TestComponent_Setup(t *testing.T) {
	c := &ssh.Component{Host: "example.com"}
	err := c.Setup()
	require.NoError(t, err)

	// Check defaults
	assert.Equal(t, 22, c.Port)
	assert.Equal(t, ssh.DefaultTimeout, c.GetTimeout())
}

func TestComponent_Setup_MissingHost(t *testing.T) {
	c := &ssh.Component{}
	err := c.Setup()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "host is required")
}

func TestComponent_GetType(t *testing.T) {
	c := &ssh.Component{}
	assert.Equal(t, "ssh", c.GetType())
}

func TestComponent_SetChecks(t *testing.T) {
	c := &ssh.Component{}

	// Valid check
	err := c.SetChecks([]checks.Expression{
		{Expression: `"ssh-ed25519" in ssh.hostKey`},
	})
	require.NoError(t, err)
	assert.True(t, c.HasChecks())

	// Invalid check
	err = c.SetChecks([]checks.Expression{
		{Expression: `invalid syntax [`},
	})
	assert.Error(t, err)
}

func TestComponent_ProviderInterface(t *testing.T) {
	instance := &ssh.Component{
		Host: "example.com",
	}
	instance.SetName("test-instance")
	require.NoError(t, instance.Setup())

	assert.Equal(t, ssh.ProviderType, instance.GetType())
	assert.Equal(t, "test-instance", instance.GetName())

	instance.SetName("renamed")
	assert.Equal(t, "renamed", instance.GetName())
}

func TestComponent_GetHealth(t *testing.T) {
	ed25519Signer := generateED25519Signer(t)
	ecdsaSigner := generateECDSASigner(t)

	tests := []struct {
		name     string
		signers  []gossh.Signer
		algos    []string
		timeout  time.Duration
		expected ph.Status
	}{
		{
			name:     "Healthy - default algorithm",
			signers:  []gossh.Signer{ed25519Signer},
			timeout:  5 * time.Second,
			expected: ph.Status_HEALTHY,
		},
		{
			name:     "Healthy - specific algorithm",
			signers:  []gossh.Signer{ed25519Signer, ecdsaSigner},
			algos:    []string{"ssh-ed25519"},
			timeout:  5 * time.Second,
			expected: ph.Status_HEALTHY,
		},
		{
			name:     "Healthy - multiple algorithms",
			signers:  []gossh.Signer{ed25519Signer, ecdsaSigner},
			algos:    []string{"ssh-ed25519", "ecdsa-sha2-nistp256"},
			timeout:  5 * time.Second,
			expected: ph.Status_HEALTHY,
		},
		{
			name:     "Unhealthy - connection refused",
			timeout:  2 * time.Second,
			expected: ph.Status_UNHEALTHY,
		},
		{
			name:     "Unhealthy - timeout",
			signers:  []gossh.Signer{ed25519Signer},
			timeout:  time.Nanosecond,
			expected: ph.Status_UNHEALTHY,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			port := 0
			if len(tt.signers) > 0 {
				port = mockSSHServer(t, tt.signers...)
			} else {
				// Use a port with nothing listening for connection refused
				port = 65432
			}

			instance := &ssh.Component{
				Host:       "127.0.0.1",
				Port:       port,
				Algorithms: tt.algos,
			}
			instance.SetName(tt.name)
			instance.SetTimeout(tt.timeout)
			require.NoError(t, instance.Setup())

			result := provider.GetHealthWithDuration(t.Context(), instance)

			assert.NotNil(t, result)
			assert.Equal(t, ssh.ProviderType, result.GetType())
			assert.Equal(t, tt.name, result.GetName())
			assert.Equal(t, tt.expected, result.GetStatus())
		})
	}
}

func TestComponent_GetCheckContext(t *testing.T) {
	ed25519Signer := generateED25519Signer(t)
	ecdsaSigner := generateECDSASigner(t)

	t.Run("single algorithm (server default)", func(t *testing.T) {
		port := mockSSHServer(t, ed25519Signer)

		instance := &ssh.Component{
			Host: "127.0.0.1",
			Port: port,
		}
		instance.SetName("check-context-default")
		instance.SetTimeout(5 * time.Second)
		require.NoError(t, instance.Setup())

		ctx, err := instance.GetCheckContext(t.Context())
		require.NoError(t, err)

		sshCtx, ok := ctx["ssh"].(map[string]any)
		require.True(t, ok, "ssh should be a map")

		// Verify hostKey map
		hostKey, ok := sshCtx["hostKey"].(map[string]string)
		require.True(t, ok, "hostKey should be map[string]string")
		assert.Len(t, hostKey, 1)
		assert.Contains(t, hostKey, "ssh-ed25519")
		assert.Contains(t, hostKey["ssh-ed25519"], "SHA256:")

		// Verify host and port
		assert.Equal(t, "127.0.0.1", sshCtx["host"])
		assert.Equal(t, port, sshCtx["port"])
	})

	t.Run("multiple algorithms", func(t *testing.T) {
		port := mockSSHServer(t, ed25519Signer, ecdsaSigner)

		instance := &ssh.Component{
			Host:       "127.0.0.1",
			Port:       port,
			Algorithms: []string{"ssh-ed25519", "ecdsa-sha2-nistp256"},
		}
		instance.SetName("check-context-multi")
		instance.SetTimeout(5 * time.Second)
		require.NoError(t, instance.Setup())

		ctx, err := instance.GetCheckContext(t.Context())
		require.NoError(t, err)

		sshCtx := ctx["ssh"].(map[string]any)
		hostKey := sshCtx["hostKey"].(map[string]string)
		assert.Len(t, hostKey, 2)
		assert.Contains(t, hostKey, "ssh-ed25519")
		assert.Contains(t, hostKey, "ecdsa-sha2-nistp256")
	})

	t.Run("connection refused", func(t *testing.T) {
		instance := &ssh.Component{
			Host: "127.0.0.1",
			Port: 65432,
		}
		instance.SetName("check-context-refused")
		instance.SetTimeout(2 * time.Second)
		require.NoError(t, instance.Setup())

		_, err := instance.GetCheckContext(t.Context())
		assert.Error(t, err)
	})
}

func TestComponent_CELChecks(t *testing.T) {
	ed25519Signer := generateED25519Signer(t)
	ed25519Fingerprint := gossh.FingerprintSHA256(ed25519Signer.PublicKey())

	tests := []struct {
		name     string
		checks   []checks.Expression
		expected ph.Status
	}{
		{
			name: "Check passes - algorithm present",
			checks: []checks.Expression{
				{Expression: `"ssh-ed25519" in ssh.hostKey`, Message: "ED25519 not supported"},
			},
			expected: ph.Status_HEALTHY,
		},
		{
			name: "Check passes - fingerprint match",
			checks: []checks.Expression{
				{Expression: fmt.Sprintf(`ssh.hostKey["ssh-ed25519"] == "%s"`, ed25519Fingerprint), Message: "Fingerprint mismatch"},
			},
			expected: ph.Status_HEALTHY,
		},
		{
			name: "Check passes - host and port",
			checks: []checks.Expression{
				{Expression: `ssh.host == "127.0.0.1"`, Message: "Wrong host"},
			},
			expected: ph.Status_HEALTHY,
		},
		{
			name: "Check fails - missing algorithm",
			checks: []checks.Expression{
				{Expression: `"ecdsa-sha2-nistp256" in ssh.hostKey`, Message: "ECDSA not supported"},
			},
			expected: ph.Status_UNHEALTHY,
		},
		{
			name: "Check fails - fingerprint mismatch",
			checks: []checks.Expression{
				{Expression: `ssh.hostKey["ssh-ed25519"] == "SHA256:bogus"`, Message: "Fingerprint mismatch"},
			},
			expected: ph.Status_UNHEALTHY,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			port := mockSSHServer(t, ed25519Signer)

			instance := &ssh.Component{
				Host: "127.0.0.1",
				Port: port,
			}
			instance.SetName(tt.name)
			instance.SetTimeout(5 * time.Second)
			require.NoError(t, instance.SetChecks(tt.checks))
			require.NoError(t, instance.Setup())

			result := instance.GetHealth(t.Context())

			assert.NotNil(t, result)
			assert.Equal(t, tt.expected, result.GetStatus())
		})
	}
}
