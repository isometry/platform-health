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
	"reflect"
	"strings"
	"time"

	"github.com/google/cel-go/cel"
	"github.com/mcuadros/go-defaults"

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
	Method  string            `mapstructure:"method" default:"GET"`
	Body    string            `mapstructure:"body"`
	Headers map[string]string `mapstructure:"headers"`
}

// REST provider extends HTTP provider with response validation capabilities
type REST struct {
	Name     string          `mapstructure:"name"`
	URL      string          `mapstructure:"url"`
	Request  Request         `mapstructure:"request"`
	Timeout  time.Duration   `mapstructure:"timeout" default:"10s"`
	Insecure bool            `mapstructure:"insecure"`
	Checks   []CELExpression `mapstructure:"checks"`

	// Compiled validation components (cached after SetDefaults)
	celEnv      *cel.Env
	celPrograms []cel.Program
}

// CELExpression represents a CEL expression validation rule
type CELExpression struct {
	Expression   string `mapstructure:"expression"`
	ErrorMessage string `mapstructure:"errorMessage"`
}

var certPool *x509.CertPool = nil

func init() {
	provider.Register(TypeREST, new(REST))
	if systemCertPool, err := x509.SystemCertPool(); err == nil {
		certPool = systemCertPool
	}
}

func (i *REST) LogValue() slog.Value {
	logAttr := []slog.Attr{
		slog.String("name", i.Name),
		slog.String("url", i.URL),
		slog.String("method", i.Request.Method),
		slog.Any("timeout", i.Timeout),
		slog.Bool("insecure", i.Insecure),
		slog.Bool("hasRequestBody", i.Request.Body != ""),
		slog.Int("requestHeaders", len(i.Request.Headers)),
		slog.Int("checks", len(i.Checks)),
	}
	return slog.GroupValue(logAttr...)
}

func (i *REST) SetDefaults() {
	defaults.SetDefaults(i)

	// Pre-compile CEL programs if checks exist
	if len(i.Checks) > 0 {
		if err := i.compileCELPrograms(); err != nil {
			// Log error but don't fail - will be caught during GetHealth
			slog.Error("failed to compile CEL programs", "error", err.Error())
		}
	}
}

// compileCELPrograms pre-compiles all CEL expressions for efficient evaluation
func (i *REST) compileCELPrograms() error {
	// Create CEL environment
	env, err := cel.NewEnv(
		cel.Variable("request", cel.MapType(cel.StringType, cel.DynType)),
		cel.Variable("response", cel.MapType(cel.StringType, cel.DynType)),
	)
	if err != nil {
		return fmt.Errorf("failed to create CEL environment: %w", err)
	}

	// Pre-compile all expressions
	programs := make([]cel.Program, len(i.Checks))
	for idx, expr := range i.Checks {
		ast, issues := env.Compile(expr.Expression)
		if issues != nil && issues.Err() != nil {
			return fmt.Errorf("CEL expression [%d] compilation error: %w", idx, issues.Err())
		}
		if ast.OutputType() != cel.BoolType {
			return fmt.Errorf("CEL expression [%d] must return boolean", idx)
		}
		program, err := env.Program(ast)
		if err != nil {
			return fmt.Errorf("failed to create CEL program [%d]: %w", idx, err)
		}
		programs[idx] = program
	}

	// Only update instance fields after successful compilation
	i.celEnv = env
	i.celPrograms = programs
	return nil
}

func (i *REST) GetType() string {
	return TypeREST
}

func (i *REST) GetName() string {
	return i.Name
}

func (i *REST) GetHealth(ctx context.Context) *ph.HealthCheckResponse {
	log := utils.ContextLogger(ctx, slog.String("provider", TypeREST), slog.Any("instance", i))
	log.Debug("checking")

	ctx, cancel := context.WithTimeout(ctx, i.Timeout)
	defer cancel()

	component := &ph.HealthCheckResponse{
		Type: TypeREST,
		Name: i.Name,
	}
	defer component.LogStatus(log)

	// Execute single HTTP request
	response, body, err := i.executeHTTPRequest(ctx)
	if err != nil {
		return component.Unhealthy(err.Error())
	}
	defer func() { _ = response.Body.Close() }()

	// Apply CEL checks if configured
	if len(i.Checks) > 0 {
		if err := i.validateCEL(response, body); err != nil {
			return component.Unhealthy(err.Error())
		}
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

	request, err := http.NewRequestWithContext(ctx, i.Request.Method, i.URL, bodyReader)
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

// validateCEL evaluates CEL expressions against response using cached programs
func (i *REST) validateCEL(response *http.Response, body []byte) error {
	if len(i.Checks) == 0 {
		return nil
	}

	// Ensure CEL programs are compiled
	if i.celEnv == nil || len(i.celPrograms) == 0 {
		if err := i.compileCELPrograms(); err != nil {
			return fmt.Errorf("failed to compile CEL programs: %w", err)
		}
	}

	// Build CEL context with request and response
	celCtx, err := i.buildCELContext(response, body)
	if err != nil {
		return fmt.Errorf("failed to build CEL context: %w", err)
	}

	// Evaluate each pre-compiled CEL program
	for idx, program := range i.celPrograms {
		result, _, err := program.Eval(celCtx)
		if err != nil {
			return fmt.Errorf("CEL expression [%d] failed to evaluate: %w", idx, err)
		}

		// Convert result to native boolean
		value, err := result.ConvertToNative(reflect.TypeOf(false))
		if err != nil {
			return fmt.Errorf("CEL expression [%d] result conversion failed: %w", idx, err)
		}

		// Check if result is boolean true
		if boolResult, ok := value.(bool); !ok || !boolResult {
			if i.Checks[idx].ErrorMessage != "" {
				return fmt.Errorf("%s", i.Checks[idx].ErrorMessage)
			}
			return fmt.Errorf("CEL expression failed: %s", i.Checks[idx].Expression)
		}
	}

	return nil
}

// buildCELContext creates CEL evaluation context from HTTP request and response
func (i *REST) buildCELContext(response *http.Response, body []byte) (map[string]any, error) {
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
			"url":     i.URL,
		},
		"response": map[string]any{
			"json":    jsonData,
			"body":    bodyText,
			"status":  response.StatusCode,
			"headers": respHeaders,
		},
	}, nil
}
