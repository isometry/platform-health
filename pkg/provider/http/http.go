package http

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"slices"
	"time"

	"github.com/mcuadros/go-defaults"
	"github.com/spf13/pflag"
	"google.golang.org/protobuf/types/known/anypb"

	"github.com/isometry/platform-health/pkg/commands/flags"
	ph "github.com/isometry/platform-health/pkg/platform_health"
	"github.com/isometry/platform-health/pkg/provider"
	tlsProvider "github.com/isometry/platform-health/pkg/provider/tls"
	"github.com/isometry/platform-health/pkg/utils"
)

const TypeHTTP = "http"

type HTTP struct {
	Name     string        `mapstructure:"-"`
	URL      string        `mapstructure:"url"`
	Method   string        `mapstructure:"method" default:"HEAD"`
	Timeout  time.Duration `mapstructure:"timeout" default:"10s"`
	Insecure bool          `mapstructure:"insecure"`
	Status   []int         `mapstructure:"status" default:"[200]"` // expected status
	Detail   bool          `mapstructure:"detail"`
}

// Compile-time interface check
var _ provider.FlagConfigurable = (*HTTP)(nil)

var certPool *x509.CertPool = nil

func init() {
	provider.Register(TypeHTTP, new(HTTP))
	if systemCertPool, err := x509.SystemCertPool(); err == nil {
		certPool = systemCertPool
	}
}

// GetProviderFlags returns flag definitions for CLI configuration.
func (i *HTTP) GetProviderFlags() flags.FlagValues {
	return flags.FlagValues{
		"url": {
			Kind:  "string",
			Usage: "target URL",
		},
		"method": {
			Kind:         "string",
			DefaultValue: "HEAD",
			Usage:        "HTTP method",
		},
		"timeout": {
			Kind:         "duration",
			DefaultValue: "10s",
			Usage:        "request timeout",
		},
		"insecure": {
			Kind:  "bool",
			Usage: "skip TLS verification",
		},
		"status": {
			Kind:         "intSlice",
			DefaultValue: []int{200},
			Usage:        "expected HTTP status codes",
		},
		"detail": {
			Kind:  "bool",
			Usage: "include TLS connection details",
		},
	}
}

// ConfigureFromFlags applies flag values to the provider.
func (i *HTTP) ConfigureFromFlags(fs *pflag.FlagSet) error {
	var errs []error
	var err error

	if i.URL, err = fs.GetString("url"); err != nil {
		errs = append(errs, err)
	}
	if i.Method, err = fs.GetString("method"); err != nil {
		errs = append(errs, err)
	}
	if i.Timeout, err = fs.GetDuration("timeout"); err != nil {
		errs = append(errs, err)
	}
	if i.Insecure, err = fs.GetBool("insecure"); err != nil {
		errs = append(errs, err)
	}
	if i.Status, err = fs.GetIntSlice("status"); err != nil {
		errs = append(errs, err)
	}
	if i.Detail, err = fs.GetBool("detail"); err != nil {
		errs = append(errs, err)
	}

	if len(errs) > 0 {
		return fmt.Errorf("flag errors: %w", errors.Join(errs...))
	}

	if i.URL == "" {
		return fmt.Errorf("url is required")
	}
	return nil
}

func (i *HTTP) LogValue() slog.Value {
	logAttr := []slog.Attr{
		slog.String("name", i.Name),
		slog.String("url", i.URL),
		slog.Any("status", i.Status),
		slog.Any("timeout", i.Timeout),
		slog.Bool("insecure", i.Insecure),
		slog.Bool("detail", i.Detail),
	}
	return slog.GroupValue(logAttr...)
}

func (i *HTTP) Setup() error {
	defaults.SetDefaults(i)

	return nil
}

func (i *HTTP) GetType() string {
	return TypeHTTP
}

func (i *HTTP) GetName() string {
	return i.Name
}

func (i *HTTP) SetName(name string) {
	i.Name = name
}

func (i *HTTP) GetHealth(ctx context.Context) *ph.HealthCheckResponse {
	log := utils.ContextLogger(ctx, slog.String("provider", TypeHTTP), slog.Any("instance", i))
	log.Debug("checking")

	ctx, cancel := context.WithTimeout(ctx, i.Timeout)
	defer cancel()

	component := &ph.HealthCheckResponse{
		Type: TypeHTTP,
		Name: i.Name,
	}
	defer component.LogStatus(log)

	request, err := http.NewRequestWithContext(ctx, i.Method, i.URL, nil)
	if err != nil {
		log.Error("failed to create request", "error", err.Error())
		return component.Unhealthy(err.Error())
	}

	client := &http.Client{Timeout: i.Timeout}
	tlsConf := &tls.Config{
		ServerName: request.URL.Hostname(),
		RootCAs:    certPool,
	}
	if i.Insecure {
		tlsConf.InsecureSkipVerify = true
	}
	client.Transport = &http.Transport{TLSClientConfig: tlsConf}

	response, err := client.Do(request)
	if err != nil {
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

	if i.Detail && response.TLS != nil {
		if detail, err := anypb.New(tlsProvider.Detail(response.TLS)); err != nil {
			return component.Unhealthy(err.Error())
		} else {
			component.Details = append(component.Details, detail)
		}
	}

	if !slices.Contains[[]int, int](i.Status, response.StatusCode) {
		return component.Unhealthy(fmt.Sprintf("expected status %d; actual status %d", i.Status, response.StatusCode))
	}
	_ = response.Body.Close()

	return component.Healthy()
}
