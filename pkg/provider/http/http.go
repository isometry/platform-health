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
	"google.golang.org/protobuf/types/known/anypb"

	"github.com/isometry/platform-health/pkg/phctx"
	ph "github.com/isometry/platform-health/pkg/platform_health"
	"github.com/isometry/platform-health/pkg/provider"
	tlsProvider "github.com/isometry/platform-health/pkg/provider/tls"
)

const ProviderType = "http"

type Component struct {
	Name     string        `mapstructure:"-"`
	URL      string        `mapstructure:"url"`
	Method   string        `mapstructure:"method" default:"HEAD"`
	Timeout  time.Duration `mapstructure:"timeout" default:"10s"`
	Insecure bool          `mapstructure:"insecure"`
	Status   []int         `mapstructure:"status" default:"[200]"` // expected status
	Detail   bool          `mapstructure:"detail"`
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
		slog.String("url", c.URL),
		slog.Any("status", c.Status),
		slog.Any("timeout", c.Timeout),
		slog.Bool("insecure", c.Insecure),
		slog.Bool("detail", c.Detail),
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
	log := phctx.Logger(ctx, slog.String("provider", ProviderType), slog.Any("instance", c))
	log.Debug("checking")

	ctx, cancel := context.WithTimeout(ctx, c.Timeout)
	defer cancel()

	component := &ph.HealthCheckResponse{
		Type: ProviderType,
		Name: c.Name,
	}
	defer component.LogStatus(log)

	request, err := http.NewRequestWithContext(ctx, c.Method, c.URL, nil)
	if err != nil {
		log.Error("failed to create request", "error", err.Error())
		return component.Unhealthy(err.Error())
	}

	client := &http.Client{Timeout: c.Timeout}
	tlsConf := &tls.Config{
		ServerName: request.URL.Hostname(),
		RootCAs:    certPool,
	}
	if c.Insecure {
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

	if c.Detail && response.TLS != nil {
		if detail, err := anypb.New(tlsProvider.Detail(response.TLS)); err != nil {
			return component.Unhealthy(err.Error())
		} else {
			component.Details = append(component.Details, detail)
		}
	}

	if !slices.Contains[[]int, int](c.Status, response.StatusCode) {
		return component.Unhealthy(fmt.Sprintf("expected status %d; actual status %d", c.Status, response.StatusCode))
	}
	_ = response.Body.Close()

	return component.Healthy()
}
