// Package ssh provides an SSH protocol health check provider.
//
// The SSH provider performs protocol-level handshake(s) to verify SSH server availability
// and capture host key fingerprints for security verification. This enables MITM detection
// through host key fingerprint pinning and algorithm compliance checks.
//
// # CEL Variables
//
// The provider exposes the following variables for CEL check expressions:
//
//	ssh.hostKey    map[string]string    Map of algorithm name to SHA256 fingerprint
//	ssh.host       string               Target host
//	ssh.port       int                  Target port
//
// # Example Configuration
//
//	bastion:
//	  type: ssh
//	  spec:
//	    host: bastion.example.com
//	    port: 22
//	    algorithms:
//	      - ssh-ed25519
//	      - ecdsa-sha2-nistp256
//	  checks:
//	    - check: '"ssh-ed25519" in ssh.hostKey'
//	      message: "Server must support ED25519"
//	    - check: 'ssh.hostKey["ssh-ed25519"] == "SHA256:abc123..."'
//	      message: "Host key mismatch - possible MITM attack"
package ssh

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"strings"
	"time"

	"github.com/google/cel-go/cel"
	"github.com/mcuadros/go-defaults"
	"golang.org/x/crypto/ssh"

	"github.com/isometry/platform-health/pkg/checks"
	"github.com/isometry/platform-health/pkg/phctx"
	ph "github.com/isometry/platform-health/pkg/platform_health"
	"github.com/isometry/platform-health/pkg/provider"
)

const (
	ProviderType   = "ssh"
	DefaultTimeout = 5 * time.Second
)

// CEL configuration for SSH provider
var celConfig = checks.NewCEL(
	cel.Variable("ssh", cel.MapType(cel.StringType, cel.DynType)),
)

// Component represents an SSH health check instance.
type Component struct {
	provider.Base
	provider.BaseWithChecks

	Host       string   `mapstructure:"host"`
	Port       int      `mapstructure:"port" default:"22"`
	Algorithms []string `mapstructure:"algorithms"`
}

var _ provider.InstanceWithChecks = (*Component)(nil)

func init() {
	provider.Register(ProviderType, new(Component))
}

func (c *Component) LogValue() slog.Value {
	logAttr := []slog.Attr{
		slog.String("name", c.GetName()),
		slog.String("host", c.Host),
		slog.Int("port", c.Port),
	}
	if len(c.Algorithms) > 0 {
		logAttr = append(logAttr, slog.Any("algorithms", c.Algorithms))
	}
	return slog.GroupValue(logAttr...)
}

func (c *Component) Setup() error {
	if c.GetTimeout() == 0 {
		c.SetTimeout(DefaultTimeout)
	}
	defaults.SetDefaults(c)

	if c.Host == "" {
		return fmt.Errorf("host is required")
	}

	return nil
}

// SetChecks sets and compiles CEL expressions.
func (c *Component) SetChecks(exprs []checks.Expression) error {
	return c.SetChecksAndCompile(exprs, celConfig)
}

func (c *Component) GetType() string {
	return ProviderType
}

// GetCheckConfig returns the SSH provider's CEL variable declarations.
func (c *Component) GetCheckConfig() *checks.CEL {
	return celConfig
}

func (c *Component) address() string {
	return net.JoinHostPort(c.Host, fmt.Sprint(c.Port))
}

// GetCheckContext performs SSH handshake(s) and returns the CEL evaluation context.
// Returns {"ssh": {hostKey: {algorithm: fingerprint, ...}, host, port}}.
// If Algorithms is specified, performs one handshake per algorithm to collect all fingerprints.
func (c *Component) GetCheckContext(ctx context.Context) (map[string]any, error) {
	hostKeys := make(map[string]string)

	algorithms := c.Algorithms
	if len(algorithms) == 0 {
		// Default: single handshake, server chooses algorithm
		algorithms = []string{""}
	}

	for _, algo := range algorithms {
		keyType, fingerprint, err := c.probeHostKey(ctx, algo)
		if err != nil {
			return nil, err
		}
		hostKeys[keyType] = fingerprint
	}

	return map[string]any{
		"ssh": map[string]any{
			"hostKey": hostKeys,
			"host":    c.Host,
			"port":    c.Port,
		},
	}, nil
}

// probeHostKey performs an SSH handshake and returns the host key type and fingerprint.
// If algorithm is non-empty, restricts the handshake to that specific algorithm.
func (c *Component) probeHostKey(ctx context.Context, algorithm string) (keyType, fingerprint string, err error) {
	dialer := &net.Dialer{}

	conn, err := dialer.DialContext(ctx, "tcp", c.address())
	if err != nil {
		return "", "", fmt.Errorf("tcp dial: %w", err)
	}
	defer func() { _ = conn.Close() }()

	if deadline, ok := ctx.Deadline(); ok {
		_ = conn.SetDeadline(deadline)
	}

	config := &ssh.ClientConfig{
		User: "probe",
		Auth: []ssh.AuthMethod{
			ssh.Password(""), // Empty password - will fail but that's expected
		},
		HostKeyCallback: func(hostname string, remote net.Addr, key ssh.PublicKey) error {
			keyType = key.Type()
			fingerprint = ssh.FingerprintSHA256(key)
			return nil // Accept any key for probing
		},
	}

	// Restrict to specific algorithm if specified
	if algorithm != "" {
		config.HostKeyAlgorithms = []string{algorithm}
	}

	// Perform SSH handshake - authentication will fail, but we capture the host key
	sshConn, _, _, err := ssh.NewClientConn(conn, c.address(), config)
	if err != nil {
		// If we captured the host key, the handshake succeeded (auth failed as expected)
		if keyType == "" {
			return "", "", fmt.Errorf("ssh handshake: %w", err)
		}
		// Handshake succeeded, auth failed - this is expected for probing
	}

	if sshConn != nil {
		_ = sshConn.Close()
	}

	return keyType, fingerprint, nil
}

func (c *Component) GetHealth(ctx context.Context) *ph.HealthCheckResponse {
	log := phctx.Logger(ctx, slog.String("provider", ProviderType), slog.Any("instance", c))
	log.Debug("checking")

	component := &ph.HealthCheckResponse{
		Type: ProviderType,
		Name: c.GetName(),
	}
	defer component.LogStatus(log)

	// Get check context (single SSH handshake)
	checkCtx, err := c.GetCheckContext(ctx)
	if err != nil {
		if label, ok := classifySSHError(err); ok {
			return component.Unhealthy(label)
		}
		return component.Unhealthy(err.Error())
	}

	// Apply CEL checks
	if msgs := c.EvaluateChecks(ctx, checkCtx); len(msgs) > 0 {
		return component.Unhealthy(msgs...)
	}

	return component.Healthy()
}

// classifySSHError classifies an SSH connection error, returning a short label
// for known error types or false if the error is not classifiable.
func classifySSHError(err error) (string, bool) {
	if err == nil {
		return "", false
	}

	switch {
	case errors.Is(err, context.DeadlineExceeded):
		return "connection timeout", true
	case errors.Is(err, context.Canceled):
		return "connection canceled", true
	case strings.Contains(err.Error(), "connection refused"):
		return "connection refused", true
	case strings.Contains(err.Error(), "no route to host"):
		return "no route to host", true
	case strings.Contains(err.Error(), "network is unreachable"):
		return "network unreachable", true
	case strings.Contains(err.Error(), "i/o timeout"):
		return "connection timeout", true
	default:
		return "", false
	}
}
