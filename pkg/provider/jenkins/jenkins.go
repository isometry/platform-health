package jenkins

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"github.com/bndr/gojenkins"
	ph "github.com/isometry/platform-health/pkg/platform_health"
	"github.com/isometry/platform-health/pkg/provider"
	"github.com/isometry/platform-health/pkg/utils"
	"github.com/mcuadros/go-defaults"
	"log/slog"
	"net/http"
	"net/url"
	"slices"
	"time"
)

const TypeJenkins = "jenkins"

type JobStatus = string

const (
	JobStatusSuccess JobStatus = "SUCCESS"
	JobStatusFailure           = "FAILURE"
	JobStatusAborted           = "ABORTED"
	JobStatusUnknown           = "UNKNOWN"
)

type Jenkins struct {
	client *gojenkins.Jenkins

	Job      string        `mapstructure:"job"`
	Url      string        `mapstructure:"url"`
	Timeout  time.Duration `mapstructure:"timeout" default:"10s"`
	Insecure bool          `mapstructure:"insecure"`
	Status   []JobStatus   `mapstructure:"status" default:"[SUCCESS]"`
	Detail   bool          `mapstructure:"detail"`
}

var certPool *x509.CertPool = nil

func init() {
	provider.Register(TypeJenkins, new(Jenkins))
	if systemCertPool, err := x509.SystemCertPool(); err == nil {
		certPool = systemCertPool
	}
}

func (j *Jenkins) LogValue() slog.Value {
	logAttr := []slog.Attr{
		slog.String("job", j.Job),
		slog.String("url", j.Url),
		slog.Any("status", j.Status),
		slog.Any("timeout", j.Timeout),
		slog.Bool("insecure", j.Insecure),
		slog.Bool("detail", j.Detail),
	}
	return slog.GroupValue(logAttr...)
}

func (j *Jenkins) SetDefaults() {
	defaults.SetDefaults(j)
}

func (j *Jenkins) GetType() string {
	return TypeJenkins
}

func (j *Jenkins) GetName() string {
	u, _ := url.Parse(j.Url)
	return fmt.Sprintf("%s/%s", u.Hostname(), j.Job)
}

func (j *Jenkins) GetHealth(ctx context.Context) *ph.HealthCheckResponse {
	log := utils.ContextLogger(ctx, slog.String("provider", TypeJenkins), slog.Any("instance", j))
	log.Debug("checking Jenkins job status...")

	ctx, cancel := context.WithTimeout(ctx, j.Timeout)
	defer cancel()

	component := &ph.HealthCheckResponse{
		Type: TypeJenkins,
		Name: j.GetName(),
	}

	defer component.LogStatus(log)

	parsedUrl, err := url.Parse(j.Url)
	if err != nil {
		log.Error("failed to parse URL", "error", err.Error())
		return component.Unhealthy(err.Error())
	}
	client := &http.Client{Timeout: j.Timeout}
	tlsConf := &tls.Config{
		ServerName: parsedUrl.Hostname(),
		RootCAs:    certPool,
	}

	if j.Insecure {
		tlsConf.InsecureSkipVerify = true
	}
	client.Transport = &http.Transport{TLSClientConfig: tlsConf}

	j.client, err = gojenkins.CreateJenkins(client, j.Url, "", "").
		Init(ctx)
	if err != nil {
		log.Error("failed to create Jenkins client", "error", err.Error())
		return component.Unhealthy(err.Error())
	}

	job, err := j.client.GetJob(ctx, j.Job)
	if err != nil {
		log.Error("failed to get job", "error", err.Error())
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

	build, err := job.GetLastBuild(ctx)
	if err != nil {
		log.Error("failed to get last build", "error", err.Error())
		return component.Unhealthy(err.Error())
	}

	status := JobStatusUnknown
	if build != nil {
		status = build.GetResult()
	}

	if !slices.Contains(j.Status, status) {
		return component.Unhealthy(fmt.Sprintf("expected %v. got job status %s", j.Status, status))
	}
	return component.Healthy()
}
