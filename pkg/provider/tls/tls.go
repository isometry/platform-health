package tls

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"slices"
	"time"

	"github.com/google/cel-go/cel"
	"github.com/mcuadros/go-defaults"
	"google.golang.org/protobuf/types/known/anypb"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/isometry/platform-health/pkg/checks"
	"github.com/isometry/platform-health/pkg/phctx"
	ph "github.com/isometry/platform-health/pkg/platform_health"
	"github.com/isometry/platform-health/pkg/platform_health/details"
	"github.com/isometry/platform-health/pkg/provider"
)

const (
	ProviderType   = "tls"
	DefaultTimeout = 5 * time.Second
)

// CEL configuration for TLS provider
var celConfig = checks.NewCEL(
	cel.Variable("tls", cel.MapType(cel.StringType, cel.DynType)),
)

type Component struct {
	provider.Base
	provider.BaseWithChecks

	Host        string        `mapstructure:"host"`
	Port        int           `mapstructure:"port" default:"443"`
	Insecure    bool          `mapstructure:"insecure"`
	MinValidity time.Duration `mapstructure:"minValidity" default:"24h"`
	SANs        []string      `mapstructure:"subjectAltNames"`
	Detail      bool          `mapstructure:"detail"`

	cachedDetails []*anypb.Any // cached details from GetCheckContext
}

type VerificationStatus struct {
	UnknownAuthority bool
	HostnameMismatch bool
}

var _ provider.InstanceWithChecks = (*Component)(nil)

var certPool *x509.CertPool = nil

func init() {
	provider.Register(ProviderType, new(Component))
	if systemCertPool, err := x509.SystemCertPool(); err == nil {
		certPool = systemCertPool
	}
}

// CertPool returns the system certificate pool, or nil if unavailable.
func CertPool() *x509.CertPool { return certPool }

func (c *Component) LogValue() slog.Value {
	logAttr := []slog.Attr{
		slog.String("name", c.GetName()),
		slog.String("host", c.Host),
		slog.Int("port", c.Port),
	}
	return slog.GroupValue(logAttr...)
}

func (c *Component) Setup() error {
	if c.GetTimeout() == 0 {
		c.SetTimeout(DefaultTimeout)
	}
	defaults.SetDefaults(c)
	return nil
}

// SetChecks sets and compiles CEL expressions.
func (c *Component) SetChecks(exprs []checks.Expression) error {
	return c.SetChecksAndCompile(exprs, celConfig)
}

func (c *Component) GetType() string {
	return ProviderType
}

// GetCheckConfig returns the TLS provider's CEL variable declarations.
func (c *Component) GetCheckConfig() *checks.CEL {
	return celConfig
}

// GetCheckContext performs a TLS handshake and returns the CEL evaluation context.
// Returns {"tls": tlsConnectionData} containing all TLS connection details.
// Also caches TLS details for use by GetHealth.
func (c *Component) GetCheckContext(ctx context.Context) (map[string]any, error) {
	dialer := &net.Dialer{}
	address := net.JoinHostPort(c.Host, fmt.Sprint(c.Port))
	conn, err := dialer.DialContext(ctx, "tcp", address)
	if err != nil {
		return nil, fmt.Errorf("TCP connection to %s: %w", address, err)
	}
	defer func() { _ = conn.Close() }()

	tlsConf := &tls.Config{
		ServerName: c.Host,
		RootCAs:    certPool,
	}
	if c.Insecure {
		tlsConf.InsecureSkipVerify = true
	}

	tlsConn := tls.Client(conn, tlsConf)
	if err := tlsConn.HandshakeContext(ctx); err != nil {
		return nil, fmt.Errorf("TLS handshake with %s:%d: %w", c.Host, c.Port, err)
	}
	defer func() { _ = tlsConn.Close() }()

	state := tlsConn.ConnectionState()

	// Cache wrapped details
	if detail, err := anypb.New(Detail(&state)); err == nil {
		c.cachedDetails = []*anypb.Any{detail}
	}

	// Determine if certificate chain would be verified by system CA pool
	// (regardless of insecure setting)
	verified := false
	if len(state.PeerCertificates) > 0 {
		opts := x509.VerifyOptions{
			Roots:         certPool,
			Intermediates: x509.NewCertPool(),
			DNSName:       c.Host,
		}
		for _, cert := range state.PeerCertificates[1:] {
			opts.Intermediates.AddCert(cert)
		}
		_, err := state.PeerCertificates[0].Verify(opts)
		verified = (err == nil)
	}

	// Build certificate chain
	chain := make([]string, 0, len(state.PeerCertificates))
	for _, cert := range state.PeerCertificates {
		chain = append(chain, cert.Issuer.CommonName)
	}

	return map[string]any{
		"tls": map[string]any{
			"verified":           verified,
			"commonName":         state.PeerCertificates[0].Subject.CommonName,
			"subjectAltNames":    state.PeerCertificates[0].DNSNames,
			"chain":              chain,
			"validUntil":         state.PeerCertificates[0].NotAfter,
			"signatureAlgorithm": state.PeerCertificates[0].SignatureAlgorithm.String(),
			"publicKeyAlgorithm": state.PeerCertificates[0].PublicKeyAlgorithm.String(),
			"version":            tls.VersionName(state.Version),
			"cipherSuite":        tls.CipherSuiteName(state.CipherSuite),
			"protocol":           state.NegotiatedProtocol,
			"serverName":         c.Host,
			"port":               c.Port,
		},
	}, nil
}

func (c *Component) GetHealth(ctx context.Context) *ph.HealthCheckResponse {
	log := phctx.Logger(ctx, slog.String("provider", ProviderType), slog.Any("instance", c))
	log.Debug("checking")

	component := &ph.HealthCheckResponse{
		Type: ProviderType,
		Name: c.GetName(),
	}
	defer component.LogStatus(log)

	// Get check context (single TLS handshake, caches details)
	checkCtx, err := c.GetCheckContext(ctx)
	if err != nil {
		return component.Unhealthy(ClassifyTLSError(err))
	}

	// Extract TLS data for traditional checks
	tlsData, ok := checkCtx["tls"].(map[string]any)
	if !ok {
		return component.Unhealthy(fmt.Sprintf("invalid TLS context: expected map[string]any, got %T", checkCtx["tls"]))
	}

	// Add cached details if requested
	if c.Detail {
		component.Details = append(component.Details, c.cachedDetails...)
	}

	// Check certificate validity
	validUntil, ok := tlsData["validUntil"].(time.Time)
	if !ok {
		return component.Unhealthy("missing certificate validity")
	}
	if time.Until(validUntil) < c.MinValidity {
		return component.Unhealthy(fmt.Sprintf("certificate expires: %s", validUntil))
	}

	// Check SANs
	if len(c.SANs) > 0 {
		sans, _ := tlsData["subjectAltNames"].([]string) // empty slice if missing is OK
		for _, san := range c.SANs {
			if !slices.Contains(sans, san) {
				return component.Unhealthy(fmt.Sprintf("expected SAN %s not found in certificate", san))
			}
		}
	}

	// Apply CEL checks
	if msgs := c.EvaluateChecks(ctx, checkCtx); len(msgs) > 0 {
		return component.Unhealthy(msgs...)
	}

	return component.Healthy()
}

// ClassifyTLSError returns a user-friendly error message for TLS handshake errors.
func ClassifyTLSError(err error) string {
	switch {
	case errors.As(err, new(x509.CertificateInvalidError)):
		return "certificate invalid"
	case errors.As(err, new(x509.HostnameError)):
		return "hostname mismatch"
	case errors.As(err, new(x509.UnknownAuthorityError)):
		return "unknown authority"
	default:
		return err.Error()
	}
}

// Detail builds a Detail_TLS from a TLS connection state.
func Detail(state *tls.ConnectionState) (detail *details.Detail_TLS) {
	detail = &details.Detail_TLS{
		CommonName:         state.PeerCertificates[0].Subject.CommonName,
		SubjectAltNames:    state.PeerCertificates[0].DNSNames,
		ValidUntil:         timestamppb.New(state.PeerCertificates[0].NotAfter),
		SignatureAlgorithm: state.PeerCertificates[0].SignatureAlgorithm.String(),
		PublicKeyAlgorithm: state.PeerCertificates[0].PublicKeyAlgorithm.String(),
		Version:            tls.VersionName(state.Version),
		CipherSuite:        tls.CipherSuiteName(state.CipherSuite),
		Protocol:           state.NegotiatedProtocol,
	}
	chain := make([]string, 0, len(state.PeerCertificates))
	for _, cert := range state.PeerCertificates {
		chain = append(chain, cert.Issuer.CommonName)
	}
	detail.Chain = chain

	return detail
}
