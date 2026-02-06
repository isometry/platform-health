// Package http provides an HTTP health check provider with response
// validation capabilities using CEL (Common Expression Language).
//
// The HTTP provider supports:
//   - HTTP/HTTPS requests with configurable methods and bodies
//   - TLS certificate verification and detail extraction
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
package http

import (
	"context"
	"crypto/tls"
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
	"google.golang.org/protobuf/types/known/anypb"

	"github.com/isometry/platform-health/pkg/checks"
	"github.com/isometry/platform-health/pkg/phctx"
	ph "github.com/isometry/platform-health/pkg/platform_health"
	"github.com/isometry/platform-health/pkg/provider"
	tlsprovider "github.com/isometry/platform-health/pkg/provider/tls"
)

const (
	ProviderType   = "http"
	DefaultTimeout = 10 * time.Second
	maxBodySize    = 10 * 1024 * 1024 // 10MB max response size
)

// Component provides HTTP health checks with CEL-based response validation
type Component struct {
	provider.Base
	provider.BaseWithChecks

	URL      string            `mapstructure:"url"`
	Method   string            `mapstructure:"method" default:"HEAD"`
	Body     string            `mapstructure:"body"`
	Headers  map[string]string `mapstructure:"headers"`
	Insecure bool              `mapstructure:"insecure"`
	Detail   bool              `mapstructure:"detail"`

	cachedDetails []*anypb.Any // cached details from GetCheckContext
}

var _ provider.InstanceWithChecks = (*Component)(nil)

// CEL configuration for HTTP provider
var celConfig = checks.NewCEL(
	cel.Variable("request", cel.MapType(cel.StringType, cel.DynType)),
	cel.Variable("response", cel.MapType(cel.StringType, cel.DynType)),
)

var certPool = tlsprovider.CertPool()

func init() {
	provider.Register(ProviderType, new(Component))
}

func (c *Component) LogValue() slog.Value {
	logAttr := []slog.Attr{
		slog.String("name", c.GetName()),
		slog.String("url", c.URL),
		slog.String("method", c.Method),
		slog.Bool("insecure", c.Insecure),
		slog.Bool("detail", c.Detail),
		slog.Bool("hasRequestBody", c.Body != ""),
		slog.Int("requestHeaders", len(c.Headers)),
		slog.Int("checks", len(c.GetChecks())),
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

// GetCheckConfig returns the HTTP provider's CEL variable declarations.
func (c *Component) GetCheckConfig() *checks.CEL {
	return celConfig
}

// GetCheckContext performs the HTTP request and returns the CEL evaluation context.
// Also caches TLS details for use by GetHealth.
func (c *Component) GetCheckContext(ctx context.Context) (map[string]any, error) {
	response, body, err := c.executeHTTPRequest(ctx)
	if err != nil {
		return nil, err
	}
	defer func() { _ = response.Body.Close() }()

	// Cache wrapped TLS details if available
	if response.TLS != nil {
		if detail, err := anypb.New(tlsprovider.Detail(response.TLS)); err == nil {
			c.cachedDetails = []*anypb.Any{detail}
		}
	}

	return c.buildCheckContext(response, body), nil
}

func (c *Component) GetType() string {
	return ProviderType
}

func (c *Component) GetHealth(ctx context.Context) *ph.HealthCheckResponse {
	log := phctx.Logger(ctx, slog.String("provider", ProviderType), slog.Any("instance", c))
	log.Debug("checking")

	component := &ph.HealthCheckResponse{
		Type: ProviderType,
		Name: c.GetName(),
	}
	defer component.LogStatus(log)

	// Get check context (single HTTP request, caches TLS details)
	checkCtx, err := c.GetCheckContext(ctx)
	if err != nil {
		return component.Unhealthy(err.Error())
	}

	// Apply CEL checks
	if msgs := c.EvaluateChecks(ctx, checkCtx); len(msgs) > 0 {
		return component.Unhealthy(msgs...)
	}

	// Add cached details if requested
	if c.Detail {
		component.Details = append(component.Details, c.cachedDetails...)
	}

	return component.Healthy()
}

// executeHTTPRequest performs a single HTTP request and returns response with body
func (c *Component) executeHTTPRequest(ctx context.Context) (*http.Response, []byte, error) {
	// Create request with optional body
	var bodyReader io.Reader
	if c.Body != "" {
		bodyReader = strings.NewReader(c.Body)
	}

	request, err := http.NewRequestWithContext(ctx, c.Method, c.URL, bodyReader)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create request: %w", err)
	}

	// Set custom headers from configuration
	for key, value := range c.Headers {
		request.Header.Set(key, value)
	}

	// Set default Content-Type for POST/PUT with body if not already set
	if c.Body != "" && (c.Method == "POST" || c.Method == "PUT") {
		if request.Header.Get("Content-Type") == "" {
			request.Header.Set("Content-Type", "application/json")
		}
	}

	// Configure HTTP client with TLS
	client := &http.Client{}
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
		return nil, nil, fmt.Errorf("%s", tlsprovider.ClassifyTLSError(err))
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
	reqHeaders := make(map[string]string, len(c.Headers))
	for key, value := range c.Headers {
		reqHeaders[strings.ToLower(key)] = value
	}

	return map[string]any{
		"request": map[string]any{
			"method":  c.Method,
			"body":    c.Body,
			"headers": reqHeaders,
			"url":     c.URL,
		},
		"response": map[string]any{
			"json":    jsonData,
			"body":    bodyText,
			"status":  response.StatusCode,
			"headers": respHeaders,
		},
	}
}
