// Package rest provides a REST API health check provider with response
// validation capabilities using CEL (Common Expression Language).
//
// The REST provider supports:
//   - HTTP/HTTPS requests with configurable methods and bodies
//   - TLS certificate verification
//   - Response validation using CEL expressions
//   - JSON response parsing and field validation
//   - Status code, header, and body validation via CEL
//
// CEL Expression Context:
// The following variables are available in CEL expressions:
//
// Request context:
//   - request.method (string): HTTP method (GET, POST, etc.)
//   - request.body (string): Request body text
//   - request.headers (map[string]string): Request headers (lowercase keys)
//   - request.url (string): Target URL
//
// Response context:
//   - response.json (map[string]any): Parsed JSON response body (nil if not JSON)
//   - response.body (string): Raw response body as text
//   - response.status (int): HTTP status code
//   - response.headers (map[string]string): Response headers (lowercase keys)
//
// Example CEL expressions:
//   - response.status == 200                                       // Status code validation
//   - response.status >= 200 && response.status < 300             // Status range validation
//   - response.json.status == "healthy"                            // JSON field validation
//   - response.headers["x-health-status"] == "ok"                 // Header validation
//   - response.body.contains("success")                            // Body text validation
//   - response.headers["content-type"].contains("application/json") // Content type validation
//   - request.method == "POST" && response.status == 201          // Method-specific validation
//   - response.headers["content-type"] == request.headers["accept"] // Content negotiation
package rest

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/google/cel-go/cel"
	"github.com/mcuadros/go-defaults"
	"github.com/spf13/viper"

	"github.com/isometry/platform-health/pkg/checks"
	"github.com/isometry/platform-health/pkg/commands/flags"
	ph "github.com/isometry/platform-health/pkg/platform_health"
	"github.com/isometry/platform-health/pkg/provider"
	"github.com/isometry/platform-health/pkg/utils"
)

const (
	TypeREST    = "rest"
	maxBodySize = 10 * 1024 * 1024 // 10MB max response size
)

// Request represents HTTP request configuration
type Request struct {
	URL     string            `mapstructure:"url"`
	Method  string            `mapstructure:"method" default:"GET"`
	Body    string            `mapstructure:"body"`
	Headers map[string]string `mapstructure:"headers"`
}

// REST provider extends HTTP provider with response validation capabilities
type REST struct {
	provider.BaseCELProvider `mapstructure:",squash"`

	Name     string        `mapstructure:"-"`
	Request  Request       `mapstructure:"request"`
	Insecure bool          `mapstructure:"insecure"`
	Timeout  time.Duration `mapstructure:"timeout" default:"10s"`
}

// Compile-time interface checks
var (
	_ provider.CELCapable       = (*REST)(nil)
	_ provider.FlagConfigurable = (*REST)(nil)
)

var certPool *x509.CertPool

// CEL configuration for REST provider
var celConfig = checks.NewCEL(
	cel.Variable("request", cel.MapType(cel.StringType, cel.DynType)),
	cel.Variable("response", cel.MapType(cel.StringType, cel.DynType)),
)

func init() {
	provider.Register(TypeREST, new(REST))
	if systemCertPool, err := x509.SystemCertPool(); err == nil {
		certPool = systemCertPool
	}
}

func (i *REST) LogValue() slog.Value {
	logAttr := []slog.Attr{
		slog.String("name", i.Name),
		slog.String("url", i.Request.URL),
		slog.String("method", i.Request.Method),
		slog.Any("timeout", i.Timeout),
		slog.Bool("insecure", i.Insecure),
		slog.Bool("hasRequestBody", i.Request.Body != ""),
		slog.Int("requestHeaders", len(i.Request.Headers)),
		slog.Int("checks", len(i.GetChecks())),
	}
	return slog.GroupValue(logAttr...)
}

func (i *REST) Setup() error {
	defaults.SetDefaults(i)
	return i.SetupCEL(celConfig)
}

// GetCELConfig returns the REST provider's CEL variable declarations.
func (i *REST) GetCELConfig() *checks.CEL {
	return celConfig
}

// GetCELContext performs the HTTP request and returns the CEL evaluation context.
func (i *REST) GetCELContext(ctx context.Context) (map[string]any, error) {
	ctx, cancel := context.WithTimeout(ctx, i.Timeout)
	defer cancel()

	response, body, err := i.executeHTTPRequest(ctx)
	if err != nil {
		return nil, err
	}
	defer func() { _ = response.Body.Close() }()

	return i.buildCELContext(response, body), nil
}

// GetProviderFlags returns flag definitions for CLI configuration.
func (i *REST) GetProviderFlags() flags.FlagValues {
	return flags.FlagValues{
		"url": {
			Kind:         "string",
			DefaultValue: "",
			Usage:        "target URL",
		},
		"method": {
			Kind:         "string",
			DefaultValue: "GET",
			Usage:        "HTTP method",
		},
		"body": {
			Kind:         "string",
			DefaultValue: "",
			Usage:        "request body",
		},
		"insecure": {
			Kind:         "bool",
			DefaultValue: false,
			Usage:        "skip TLS verification",
		},
		"timeout": {
			Kind:         "duration",
			DefaultValue: 10 * time.Second,
			Usage:        "request timeout",
		},
	}
}

// ConfigureFromFlags applies Viper values to the provider.
func (i *REST) ConfigureFromFlags(v *viper.Viper) error {
	i.Request.URL = v.GetString(TypeREST + ".url")
	i.Request.Method = v.GetString(TypeREST + ".method")
	i.Request.Body = v.GetString(TypeREST + ".body")
	i.Insecure = v.GetBool(TypeREST + ".insecure")
	i.Timeout = v.GetDuration(TypeREST + ".timeout")

	if i.Request.URL == "" {
		return fmt.Errorf("url is required")
	}
	return nil
}

func (i *REST) GetType() string {
	return TypeREST
}

func (i *REST) GetName() string {
	return i.Name
}

func (i *REST) SetName(name string) {
	i.Name = name
}

func (i *REST) GetHealth(ctx context.Context) *ph.HealthCheckResponse {
	log := utils.ContextLogger(ctx, slog.String("provider", TypeREST), slog.Any("instance", i))
	log.Debug("checking")

	component := &ph.HealthCheckResponse{
		Type: TypeREST,
		Name: i.Name,
	}
	defer component.LogStatus(log)

	// Get CEL context (executes HTTP request)
	celCtx, err := i.GetCELContext(ctx)
	if err != nil {
		return component.Unhealthy(err.Error())
	}

	// Apply CEL checks
	if err := i.EvaluateCEL(celCtx); err != nil {
		return component.Unhealthy(err.Error())
	}

	return component.Healthy()
}

// executeHTTPRequest performs a single HTTP request and returns response with body
func (i *REST) executeHTTPRequest(ctx context.Context) (*http.Response, []byte, error) {
	// Create request with optional body
	var bodyReader io.Reader
	if i.Request.Body != "" {
		bodyReader = strings.NewReader(i.Request.Body)
	}

	request, err := http.NewRequestWithContext(ctx, i.Request.Method, i.Request.URL, bodyReader)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create request: %w", err)
	}

	// Set custom headers from configuration
	for key, value := range i.Request.Headers {
		request.Header.Set(key, value)
	}

	// Set default Content-Type for POST/PUT with body if not already set
	if i.Request.Body != "" && (i.Request.Method == "POST" || i.Request.Method == "PUT") {
		if request.Header.Get("Content-Type") == "" {
			request.Header.Set("Content-Type", "application/json")
		}
	}

	// Configure HTTP client with TLS
	client := &http.Client{Timeout: i.Timeout}
	tlsConf := &tls.Config{
		ServerName: request.URL.Hostname(),
		RootCAs:    certPool,
	}
	if i.Insecure {
		tlsConf.InsecureSkipVerify = true
	}
	client.Transport = &http.Transport{TLSClientConfig: tlsConf}

	// Execute request
	response, err := client.Do(request)
	if err != nil {
		// Enhanced error handling for TLS issues
		switch {
		case errors.As(err, new(x509.CertificateInvalidError)):
			return nil, nil, fmt.Errorf("certificate invalid")
		case errors.As(err, new(x509.HostnameError)):
			return nil, nil, fmt.Errorf("hostname mismatch")
		case errors.As(err, new(x509.UnknownAuthorityError)):
			return nil, nil, fmt.Errorf("unknown authority")
		default:
			return nil, nil, err
		}
	}

	// Read body with size limit
	limitedReader := io.LimitReader(response.Body, maxBodySize)
	body, err := io.ReadAll(limitedReader)
	if err != nil {
		return nil, nil, errors.Join(fmt.Errorf("failed to read response body: %w", err), response.Body.Close())
	}

	return response, body, nil
}

// buildCELContext creates CEL evaluation context from HTTP request and response
func (i *REST) buildCELContext(response *http.Response, body []byte) map[string]any {
	bodyText := string(body)

	// Parse JSON body if possible (ignore error, jsonData remains nil for non-JSON)
	var jsonData any
	_ = json.Unmarshal(body, &jsonData)

	// Build response headers map with lowercase keys
	respHeaders := make(map[string]string, len(response.Header))
	for key, values := range response.Header {
		if len(values) > 0 {
			respHeaders[strings.ToLower(key)] = values[0]
		}
	}

	// Build request headers map with lowercase keys
	reqHeaders := make(map[string]string, len(i.Request.Headers))
	for key, value := range i.Request.Headers {
		reqHeaders[strings.ToLower(key)] = value
	}

	return map[string]any{
		"request": map[string]any{
			"method":  i.Request.Method,
			"body":    i.Request.Body,
			"headers": reqHeaders,
			"url":     i.Request.URL,
		},
		"response": map[string]any{
			"json":    jsonData,
			"body":    bodyText,
			"status":  response.StatusCode,
			"headers": respHeaders,
		},
	}
}
