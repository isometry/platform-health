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

	"github.com/isometry/platform-health/pkg/checks"
	"github.com/isometry/platform-health/pkg/phctx"
	ph "github.com/isometry/platform-health/pkg/platform_health"
	"github.com/isometry/platform-health/pkg/provider"
)

const (
	ProviderType = "rest"
	maxBodySize  = 10 * 1024 * 1024 // 10MB max response size
)

// Component provider extends HTTP provider with response validation capabilities
type Component struct {
	provider.BaseWithChecks `mapstructure:",squash"`

	Name     string        `mapstructure:"-"`
	Request  Request       `mapstructure:"request" flag:",inline"`
	Insecure bool          `mapstructure:"insecure"`
	Timeout  time.Duration `mapstructure:"timeout" default:"10s"`
}

// Request represents HTTP request configuration
type Request struct {
	URL     string            `mapstructure:"url"`
	Method  string            `mapstructure:"method" default:"GET"`
	Body    string            `mapstructure:"body"`
	Headers map[string]string `mapstructure:"headers"`
}

var _ provider.InstanceWithChecks = (*Component)(nil)

var certPool *x509.CertPool

// CEL configuration for REST provider
var celConfig = checks.NewCEL(
	cel.Variable("request", cel.MapType(cel.StringType, cel.DynType)),
	cel.Variable("response", cel.MapType(cel.StringType, cel.DynType)),
)

func init() {
	provider.Register(ProviderType, new(Component))
	if systemCertPool, err := x509.SystemCertPool(); err == nil {
		certPool = systemCertPool
	}
}

func (c *Component) LogValue() slog.Value {
	logAttr := []slog.Attr{
		slog.String("name", c.Name),
		slog.String("url", c.Request.URL),
		slog.String("method", c.Request.Method),
		slog.Any("timeout", c.Timeout),
		slog.Bool("insecure", c.Insecure),
		slog.Bool("hasRequestBody", c.Request.Body != ""),
		slog.Int("requestHeaders", len(c.Request.Headers)),
		slog.Int("checks", len(c.GetChecks())),
	}
	return slog.GroupValue(logAttr...)
}

func (c *Component) Setup() error {
	defaults.SetDefaults(c)
	return c.SetupChecks(celConfig)
}

// GetCheckConfig returns the REST provider's CEL variable declarations.
func (c *Component) GetCheckConfig() *checks.CEL {
	return celConfig
}

// GetCheckContext performs the HTTP request and returns the CEL evaluation context.
func (c *Component) GetCheckContext(ctx context.Context) (map[string]any, error) {
	ctx, cancel := context.WithTimeout(ctx, c.Timeout)
	defer cancel()

	response, body, err := c.executeHTTPRequest(ctx)
	if err != nil {
		return nil, err
	}
	defer func() { _ = response.Body.Close() }()

	return c.buildCheckContext(response, body), nil
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

	component := &ph.HealthCheckResponse{
		Type: ProviderType,
		Name: c.Name,
	}
	defer component.LogStatus(log)

	// Get check context (executes HTTP request)
	checkCtx, err := c.GetCheckContext(ctx)
	if err != nil {
		return component.Unhealthy(err.Error())
	}

	// Apply CEL checks
	if err := c.EvaluateChecks(checkCtx); err != nil {
		return component.Unhealthy(err.Error())
	}

	return component.Healthy()
}

// executeHTTPRequest performs a single HTTP request and returns response with body
func (c *Component) executeHTTPRequest(ctx context.Context) (*http.Response, []byte, error) {
	// Create request with optional body
	var bodyReader io.Reader
	if c.Request.Body != "" {
		bodyReader = strings.NewReader(c.Request.Body)
	}

	request, err := http.NewRequestWithContext(ctx, c.Request.Method, c.Request.URL, bodyReader)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create request: %w", err)
	}

	// Set custom headers from configuration
	for key, value := range c.Request.Headers {
		request.Header.Set(key, value)
	}

	// Set default Content-Type for POST/PUT with body if not already set
	if c.Request.Body != "" && (c.Request.Method == "POST" || c.Request.Method == "PUT") {
		if request.Header.Get("Content-Type") == "" {
			request.Header.Set("Content-Type", "application/json")
		}
	}

	// Configure HTTP client with TLS
	client := &http.Client{Timeout: c.Timeout}
	tlsConf := &tls.Config{
		ServerName: request.URL.Hostname(),
		RootCAs:    certPool,
	}
	if c.Insecure {
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

// buildCheckContext creates CEL evaluation context from HTTP request and response
func (c *Component) buildCheckContext(response *http.Response, body []byte) map[string]any {
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
	reqHeaders := make(map[string]string, len(c.Request.Headers))
	for key, value := range c.Request.Headers {
		reqHeaders[strings.ToLower(key)] = value
	}

	return map[string]any{
		"request": map[string]any{
			"method":  c.Request.Method,
			"body":    c.Request.Body,
			"headers": reqHeaders,
			"url":     c.Request.URL,
		},
		"response": map[string]any{
			"json":    jsonData,
			"body":    bodyText,
			"status":  response.StatusCode,
			"headers": respHeaders,
		},
	}
}
