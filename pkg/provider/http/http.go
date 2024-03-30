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

	ph "github.com/isometry/platform-health/pkg/platform_health"
	"github.com/isometry/platform-health/pkg/provider"
	tlsProvider "github.com/isometry/platform-health/pkg/provider/tls"
	"github.com/isometry/platform-health/pkg/utils"
)

const TypeHTTP = "http"

type HTTP struct {
	Name     string        `mapstructure:"name"`
	URL      string        `mapstructure:"url"`
	Method   string        `mapstructure:"method" default:"HEAD"`
	Timeout  time.Duration `mapstructure:"timeout" default:"10s"`
	Insecure bool          `mapstructure:"insecure"`
	Status   []int         `mapstructure:"status" default:"[200]"` // expected status
	Detail   bool          `mapstructure:"detail"`
}

var certPool *x509.CertPool = nil

func init() {
	provider.Register(TypeHTTP, new(HTTP))
	if systemCertPool, err := x509.SystemCertPool(); err == nil {
		certPool = systemCertPool
	}
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

func (i *HTTP) SetDefaults() {
	defaults.SetDefaults(i)
}

func (i *HTTP) GetType() string {
	return TypeHTTP
}

func (i *HTTP) GetName() string {
	return i.Name
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
