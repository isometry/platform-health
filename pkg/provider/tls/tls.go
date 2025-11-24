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

	"github.com/mcuadros/go-defaults"
	"google.golang.org/protobuf/types/known/anypb"
	"google.golang.org/protobuf/types/known/timestamppb"

	ph "github.com/isometry/platform-health/pkg/platform_health"
	"github.com/isometry/platform-health/pkg/platform_health/details"
	"github.com/isometry/platform-health/pkg/provider"
	"github.com/isometry/platform-health/pkg/utils"
)

const ProviderType = "tls"

type Component struct {
	Name        string        `mapstructure:"-"`
	Host        string        `mapstructure:"host"`
	Port        int           `mapstructure:"port" default:"443"`
	Timeout     time.Duration `mapstructure:"timeout" default:"5s"`
	Insecure    bool          `mapstructure:"insecure"`
	MinValidity time.Duration `mapstructure:"minValidity" default:"24h"`
	SANs        []string      `mapstructure:"subjectAltNames"`
	Detail      bool          `mapstructure:"detail"`
}

type VerificationStatus struct {
	UnknownAuthority bool
	HostnameMismatch bool
}

var certPool *x509.CertPool = nil

func init() {
	provider.Register(ProviderType, new(Component))
	if systemCertPool, err := x509.SystemCertPool(); err == nil {
		certPool = systemCertPool
	}
}

func (c *Component) LogValue() slog.Value {
	logAttr := []slog.Attr{
		slog.String("name", c.Name),
		slog.String("host", c.Host),
		slog.Int("port", c.Port),
		slog.Any("timeout", c.Timeout),
	}
	return slog.GroupValue(logAttr...)
}

func (c *Component) Setup() error {
	defaults.SetDefaults(c)

	return nil
}

func (c *Component) GetType() string {
	return ProviderType
}

func (c *Component) GetName() string {
	return c.Name
}

func (c *Component) SetName(name string) {
	c.Name = name
}

func (c *Component) GetHealth(ctx context.Context) *ph.HealthCheckResponse {
	log := utils.ContextLogger(ctx, slog.String("provider", ProviderType), slog.Any("instance", c))
	log.Debug("checking")

	ctx, cancel := context.WithTimeout(ctx, c.Timeout)
	defer cancel()

	component := &ph.HealthCheckResponse{
		Type: ProviderType,
		Name: c.Name,
	}
	defer component.LogStatus(log)

	dialer := &net.Dialer{}

	address := net.JoinHostPort(c.Host, fmt.Sprint(c.Port))
	conn, err := dialer.DialContext(ctx, "tcp", address)
	if err != nil {
		return component.Unhealthy(err.Error())
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
		switch {
		case errors.As(err, new(x509.CertificateInvalidError)):
			return component.Unhealthy("certificate invalid")
		case errors.As(err, new(x509.HostnameError)):
			return component.Unhealthy("hostname mismatch")
		case errors.As(err, new(x509.UnknownAuthorityError)):
			return component.Unhealthy("unknown authority")
		default:
			return component.Unhealthy(err.Error())
		}
	}
	defer func() { _ = tlsConn.Close() }()

	connectionState := tlsConn.ConnectionState()
	if c.Detail {
		if detail, err := anypb.New(Detail(&connectionState)); err != nil {
			return component.Unhealthy(err.Error())
		} else {
			component.Details = append(component.Details, detail)
		}
	}

	if time.Until(connectionState.PeerCertificates[0].NotAfter) < c.MinValidity {
		return component.Unhealthy(fmt.Sprintf("certificate expires: %s", connectionState.PeerCertificates[0].NotAfter))
	}

	if len(c.SANs) > 0 {
		for _, san := range c.SANs {
			if !slices.Contains[[]string, string](connectionState.PeerCertificates[0].DNSNames, san) {
				return component.Unhealthy(fmt.Sprintf("expected SAN %s not found in certificate", san))
			}
		}
	}

	return component.Healthy()
}

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
