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
	"github.com/spf13/pflag"
	"google.golang.org/protobuf/types/known/anypb"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/isometry/platform-health/pkg/commands/flags"
	ph "github.com/isometry/platform-health/pkg/platform_health"
	"github.com/isometry/platform-health/pkg/platform_health/details"
	"github.com/isometry/platform-health/pkg/provider"
	"github.com/isometry/platform-health/pkg/utils"
)

const TypeTLS = "tls"

type TLS struct {
	Name        string        `mapstructure:"-"`
	Host        string        `mapstructure:"host"`
	Port        int           `mapstructure:"port" default:"443"`
	Timeout     time.Duration `mapstructure:"timeout" default:"5s"`
	Insecure    bool          `mapstructure:"insecure"`
	MinValidity time.Duration `mapstructure:"minValidity" default:"24h"`
	SANs        []string      `mapstructure:"subjectAltNames"`
	Detail      bool          `mapstructure:"detail"`
}

// Compile-time interface check
var _ provider.FlagConfigurable = (*TLS)(nil)

type VerificationStatus struct {
	UnknownAuthority bool
	HostnameMismatch bool
}

var certPool *x509.CertPool = nil

func init() {
	provider.Register(TypeTLS, new(TLS))
	if systemCertPool, err := x509.SystemCertPool(); err == nil {
		certPool = systemCertPool
	}
}

// GetProviderFlags returns flag definitions for CLI configuration.
func (i *TLS) GetProviderFlags() flags.FlagValues {
	return flags.FlagValues{
		"host": {
			Kind:  "string",
			Usage: "target hostname",
		},
		"port": {
			Kind:         "int",
			DefaultValue: 443,
			Usage:        "target port",
		},
		"timeout": {
			Kind:         "duration",
			DefaultValue: "5s",
			Usage:        "connection timeout",
		},
		"insecure": {
			Kind:  "bool",
			Usage: "skip certificate verification",
		},
		"min-validity": {
			Kind:         "duration",
			DefaultValue: "24h",
			Usage:        "minimum certificate validity",
		},
		"sans": {
			Kind:  "stringSlice",
			Usage: "expected Subject Alternative Names",
		},
		"detail": {
			Kind:  "bool",
			Usage: "include TLS connection details",
		},
	}
}

// ConfigureFromFlags applies flag values to the provider.
func (i *TLS) ConfigureFromFlags(fs *pflag.FlagSet) error {
	var errs []error
	var err error

	if i.Host, err = fs.GetString("host"); err != nil {
		errs = append(errs, err)
	}
	if i.Port, err = fs.GetInt("port"); err != nil {
		errs = append(errs, err)
	}
	if i.Timeout, err = fs.GetDuration("timeout"); err != nil {
		errs = append(errs, err)
	}
	if i.Insecure, err = fs.GetBool("insecure"); err != nil {
		errs = append(errs, err)
	}
	if i.MinValidity, err = fs.GetDuration("min-validity"); err != nil {
		errs = append(errs, err)
	}
	if i.SANs, err = fs.GetStringSlice("sans"); err != nil {
		errs = append(errs, err)
	}
	if i.Detail, err = fs.GetBool("detail"); err != nil {
		errs = append(errs, err)
	}

	if len(errs) > 0 {
		return fmt.Errorf("flag errors: %w", errors.Join(errs...))
	}

	if i.Host == "" {
		return fmt.Errorf("host is required")
	}
	return nil
}

func (i *TLS) LogValue() slog.Value {
	logAttr := []slog.Attr{
		slog.String("name", i.Name),
		slog.String("host", i.Host),
		slog.Int("port", i.Port),
		slog.Any("timeout", i.Timeout),
	}
	return slog.GroupValue(logAttr...)
}

func (i *TLS) Setup() error {
	defaults.SetDefaults(i)

	return nil
}

func (i *TLS) GetType() string {
	return TypeTLS
}

func (i *TLS) GetName() string {
	return i.Name
}

func (i *TLS) SetName(name string) {
	i.Name = name
}

func (i *TLS) GetHealth(ctx context.Context) *ph.HealthCheckResponse {
	log := utils.ContextLogger(ctx, slog.String("provider", TypeTLS), slog.Any("instance", i))
	log.Debug("checking")

	ctx, cancel := context.WithTimeout(ctx, i.Timeout)
	defer cancel()

	component := &ph.HealthCheckResponse{
		Type: TypeTLS,
		Name: i.Name,
	}
	defer component.LogStatus(log)

	dialer := &net.Dialer{}

	address := net.JoinHostPort(i.Host, fmt.Sprint(i.Port))
	conn, err := dialer.DialContext(ctx, "tcp", address)
	if err != nil {
		return component.Unhealthy(err.Error())
	}
	defer func() { _ = conn.Close() }()

	tlsConf := &tls.Config{
		ServerName: i.Host,
		RootCAs:    certPool,
	}
	if i.Insecure {
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
	if i.Detail {
		if detail, err := anypb.New(Detail(&connectionState)); err != nil {
			return component.Unhealthy(err.Error())
		} else {
			component.Details = append(component.Details, detail)
		}
	}

	if time.Until(connectionState.PeerCertificates[0].NotAfter) < i.MinValidity {
		return component.Unhealthy(fmt.Sprintf("certificate expires: %s", connectionState.PeerCertificates[0].NotAfter))
	}

	if len(i.SANs) > 0 {
		for _, san := range i.SANs {
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
