package rest_test

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	ph "github.com/isometry/platform-health/pkg/platform_health"
	restProvider "github.com/isometry/platform-health/pkg/provider/rest"
)

func init() {
	slog.SetLogLoggerLevel(slog.LevelError)
}

func TestRESTProvider_JSONValidation(t *testing.T) {
	tests := []struct {
		name           string
		responseBody   map[string]any
		checks         []restProvider.CELExpression
		expectedStatus ph.Status
		expectedMsg    string
	}{
		{
			name: "simple JSON field validation - success",
			responseBody: map[string]any{
				"status": "healthy",
			},
			checks: []restProvider.CELExpression{
				{
					Expression:   `response.json.status == "healthy"`,
					ErrorMessage: "service is not healthy",
				},
			},
			expectedStatus: ph.Status_HEALTHY,
		},
		{
			name: "simple JSON field validation - failure",
			responseBody: map[string]any{
				"status": "unhealthy",
			},
			checks: []restProvider.CELExpression{
				{
					Expression:   `response.json.status == "healthy"`,
					ErrorMessage: "service is not healthy",
				},
			},
			expectedStatus: ph.Status_UNHEALTHY,
			expectedMsg:    "service is not healthy",
		},
		{
			name: "nested JSON validation - success",
			responseBody: map[string]any{
				"data": map[string]any{
					"database": map[string]any{
						"connected": true,
					},
				},
			},
			checks: []restProvider.CELExpression{
				{
					Expression:   `response.json.data.database.connected == true`,
					ErrorMessage: "database not connected",
				},
			},
			expectedStatus: ph.Status_HEALTHY,
		},
		{
			name: "multiple validations - all pass",
			responseBody: map[string]any{
				"status":  "ok",
				"version": "1.2.3",
				"uptime":  3600,
			},
			checks: []restProvider.CELExpression{
				{
					Expression:   `response.json.status == "ok"`,
					ErrorMessage: "status not ok",
				},
				{
					Expression:   `response.json.uptime > 0`,
					ErrorMessage: "uptime is zero",
				},
			},
			expectedStatus: ph.Status_HEALTHY,
		},
		{
			name: "multiple validations - second fails",
			responseBody: map[string]any{
				"status": "ok",
				"uptime": 0,
			},
			checks: []restProvider.CELExpression{
				{
					Expression:   `response.json.status == "ok"`,
					ErrorMessage: "status not ok",
				},
				{
					Expression:   `response.json.uptime > 0`,
					ErrorMessage: "uptime is zero",
				},
			},
			expectedStatus: ph.Status_UNHEALTHY,
			expectedMsg:    "uptime is zero",
		},
		{
			name: "status code validation",
			responseBody: map[string]any{
				"status": "ok",
			},
			checks: []restProvider.CELExpression{
				{
					Expression:   `response.status == 200`,
					ErrorMessage: "unexpected status code",
				},
			},
			expectedStatus: ph.Status_HEALTHY,
		},
		{
			name: "array length validation",
			responseBody: map[string]any{
				"items": []any{"a", "b", "c"},
			},
			checks: []restProvider.CELExpression{
				{
					Expression:   `size(response.json.items) == 3`,
					ErrorMessage: "wrong number of items",
				},
			},
			expectedStatus: ph.Status_HEALTHY,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()

			// Create test server
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusOK)
				_ = json.NewEncoder(w).Encode(tt.responseBody)
			}))
			defer server.Close()

			// Create REST provider instance
			instance := &restProvider.REST{
				Name: "test-service",
				URL:  server.URL,
				Request: restProvider.Request{
					Method: "GET",
				},
				Timeout: 5 * time.Second,
				Checks:  tt.checks,
			}
			instance.SetDefaults()

			// Execute health check
			result := instance.GetHealth(ctx)

			// Assert results
			assert.NotNil(t, result)
			assert.Equal(t, tt.expectedStatus, result.GetStatus())
			if tt.expectedMsg != "" {
				assert.Contains(t, result.GetMessage(), tt.expectedMsg)
			}
		})
	}
}

func TestRESTProvider_POSTWithBody(t *testing.T) {
	tests := []struct {
		name           string
		requestBody    string
		expectedMethod string
		expectedBody   string
		responseBody   map[string]any
		checks         []restProvider.CELExpression
		expectedStatus ph.Status
	}{
		{
			name:           "POST with JSON body",
			requestBody:    `{"username":"healthcheck","password":"test"}`,
			expectedMethod: "POST",
			expectedBody:   `{"username":"healthcheck","password":"test"}`,
			responseBody: map[string]any{
				"authenticated": true,
				"token":         "abc123",
			},
			checks: []restProvider.CELExpression{
				{
					Expression:   `response.json.authenticated == true`,
					ErrorMessage: "authentication failed",
				},
			},
			expectedStatus: ph.Status_HEALTHY,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()

			// Track received request
			var receivedMethod string
			var receivedBody string

			// Create test server
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				receivedMethod = r.Method

				// Read request body
				var reqBody map[string]any
				if err := json.NewDecoder(r.Body).Decode(&reqBody); err == nil {
					bodyBytes, _ := json.Marshal(reqBody)
					receivedBody = string(bodyBytes)
				}

				// Send response
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusOK)
				_ = json.NewEncoder(w).Encode(tt.responseBody)
			}))
			defer server.Close()

			// Create REST provider instance
			instance := &restProvider.REST{
				Name: "test-service",
				URL:  server.URL,
				Request: restProvider.Request{
					Method: "POST",
					Body:   tt.requestBody,
				},
				Timeout: 5 * time.Second,
				Checks:  tt.checks,
			}
			instance.SetDefaults()

			// Execute health check
			result := instance.GetHealth(ctx)

			// Assert results
			assert.NotNil(t, result)
			assert.Equal(t, tt.expectedStatus, result.GetStatus())
			assert.Equal(t, tt.expectedMethod, receivedMethod)
			assert.JSONEq(t, tt.expectedBody, receivedBody)
		})
	}
}

func TestRESTProvider_StatusCodeValidation(t *testing.T) {
	tests := []struct {
		name           string
		serverStatus   int
		checks         []restProvider.CELExpression
		expectedResult ph.Status
	}{
		{
			name:         "status match - single expected",
			serverStatus: 200,
			checks: []restProvider.CELExpression{
				{
					Expression:   `response.status == 200`,
					ErrorMessage: "expected status 200",
				},
			},
			expectedResult: ph.Status_HEALTHY,
		},
		{
			name:         "status match - multiple expected",
			serverStatus: 201,
			checks: []restProvider.CELExpression{
				{
					Expression:   `response.status >= 200 && response.status < 300`,
					ErrorMessage: "expected 2xx status",
				},
			},
			expectedResult: ph.Status_HEALTHY,
		},
		{
			name:         "status mismatch",
			serverStatus: 500,
			checks: []restProvider.CELExpression{
				{
					Expression:   `response.status == 200`,
					ErrorMessage: "expected status 200",
				},
			},
			expectedResult: ph.Status_UNHEALTHY,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()

			// Create test server
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tt.serverStatus)
				_, _ = w.Write([]byte("{}"))
			}))
			defer server.Close()

			// Create REST provider instance
			instance := &restProvider.REST{
				Name: "test-service",
				URL:  server.URL,
				Request: restProvider.Request{
					Method: "GET",
				},
				Timeout: 5 * time.Second,
				Checks:  tt.checks,
			}
			instance.SetDefaults()

			// Execute health check
			result := instance.GetHealth(ctx)

			// Assert results
			assert.NotNil(t, result)
			assert.Equal(t, tt.expectedResult, result.GetStatus())
		})
	}
}

func TestRESTProvider_CombinedValidation(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Create test server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("X-Custom-Header", "test-value")
		w.WriteHeader(http.StatusOK)
		response := map[string]any{
			"status":  "healthy",
			"version": "2.0.0",
			"checks": map[string]any{
				"database": "ok",
				"cache":    "ok",
			},
		}
		_ = json.NewEncoder(w).Encode(response)
	}))
	defer server.Close()

	// Create REST provider with CEL validation
	instance := &restProvider.REST{
		Name: "test-service",
		URL:  server.URL,
		Request: restProvider.Request{
			Method: "GET",
		},
		Checks: []restProvider.CELExpression{
			{
				Expression:   `response.status == 200`,
				ErrorMessage: "expected status 200",
			},
			{
				Expression:   `response.body.matches('"status":\\s*"healthy"')`,
				ErrorMessage: "status pattern not found",
			},
			{
				Expression:   `response.json.status == "healthy"`,
				ErrorMessage: "service unhealthy",
			},
			{
				Expression:   `response.json.checks.database == "ok"`,
				ErrorMessage: "database check failed",
			},
			{
				Expression:   `response.status == 200`,
				ErrorMessage: "unexpected status code",
			},
			{
				Expression:   `response.headers["content-type"] == "application/json"`,
				ErrorMessage: "wrong content type",
			},
		},
		Timeout: 5 * time.Second,
	}
	instance.SetDefaults()

	// Execute health check
	result := instance.GetHealth(ctx)

	// Assert all validations pass
	assert.NotNil(t, result)
	assert.Equal(t, ph.Status_HEALTHY, result.GetStatus())
}

func TestRESTProvider_ContentTypeValidation(t *testing.T) {
	tests := []struct {
		name           string
		contentType    string
		checks         []restProvider.CELExpression
		expectedStatus ph.Status
	}{
		{
			name:        "content type matches JSON",
			contentType: "application/json",
			checks: []restProvider.CELExpression{
				{
					Expression:   `response.headers["content-type"] == "application/json"`,
					ErrorMessage: "Expected JSON content type",
				},
			},
			expectedStatus: ph.Status_HEALTHY,
		},
		{
			name:        "content type contains JSON",
			contentType: "application/json; charset=utf-8",
			checks: []restProvider.CELExpression{
				{
					Expression:   `response.headers["content-type"].contains("application/json")`,
					ErrorMessage: "Expected JSON content type",
				},
			},
			expectedStatus: ph.Status_HEALTHY,
		},
		{
			name:        "content type mismatch",
			contentType: "text/html",
			checks: []restProvider.CELExpression{
				{
					Expression:   `response.headers["content-type"] == "application/json"`,
					ErrorMessage: "Expected JSON content type",
				},
			},
			expectedStatus: ph.Status_UNHEALTHY,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()

			// Create test server
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", tt.contentType)
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write([]byte(`{"status":"ok"}`))
			}))
			defer server.Close()

			// Create REST provider instance
			instance := &restProvider.REST{
				Name: "test-service",
				URL:  server.URL,
				Request: restProvider.Request{
					Method: "GET",
				},
				Timeout: 5 * time.Second,
				Checks:  tt.checks,
			}
			instance.SetDefaults()

			// Execute health check
			result := instance.GetHealth(ctx)

			// Assert results
			assert.NotNil(t, result)
			assert.Equal(t, tt.expectedStatus, result.GetStatus())
		})
	}
}

func TestRESTProvider_ErrorCases(t *testing.T) {
	tests := []struct {
		name           string
		setupServer    func() *httptest.Server
		checks         []restProvider.CELExpression
		expectedStatus ph.Status
	}{
		{
			name: "invalid CEL expression syntax",
			setupServer: func() *httptest.Server {
				return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					w.WriteHeader(http.StatusOK)
					_, _ = w.Write([]byte(`{"status":"ok"}`))
				}))
			},
			checks: []restProvider.CELExpression{
				{
					Expression:   `invalid syntax here!!!`,
					ErrorMessage: "validation failed",
				},
			},
			expectedStatus: ph.Status_UNHEALTHY,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()

			server := tt.setupServer()
			defer server.Close()

			instance := &restProvider.REST{
				Name: "test-service",
				URL:  server.URL,
				Request: restProvider.Request{
					Method: "GET",
				},
				Timeout: 5 * time.Second,
				Checks:  tt.checks,
			}
			instance.SetDefaults()

			result := instance.GetHealth(ctx)

			assert.NotNil(t, result)
			assert.Equal(t, tt.expectedStatus, result.GetStatus())
		})
	}
}

func TestRESTProvider_RequestContextValidation(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Track received request headers
	var receivedHeaders http.Header

	// Create test server that echoes request info
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedHeaders = r.Header
		w.Header().Set("Content-Type", r.Header.Get("Accept"))
		w.WriteHeader(http.StatusOK)
		response := map[string]any{
			"echo_method": r.Method,
			"echo_path":   r.URL.Path,
		}
		_ = json.NewEncoder(w).Encode(response)
	}))
	defer server.Close()

	// Create REST provider with request context validation
	instance := &restProvider.REST{
		Name: "test-service",
		URL:  server.URL,
		Request: restProvider.Request{
			Method: "POST",
			Body:   `{"test":"data"}`,
			Headers: map[string]string{
				"Accept":        "application/json",
				"Authorization": "Bearer token123",
			},
		},
		Checks: []restProvider.CELExpression{
			{
				Expression:   `request.method == "POST"`,
				ErrorMessage: "request method validation failed",
			},
			{
				Expression:   `request.body.contains("test")`,
				ErrorMessage: "request body validation failed",
			},
			{
				Expression:   `request.headers["accept"] == "application/json"`,
				ErrorMessage: "request accept header missing",
			},
			{
				Expression:   `response.headers["content-type"] == request.headers["accept"]`,
				ErrorMessage: "content negotiation failed",
			},
			{
				Expression:   `response.json.echo_method == request.method`,
				ErrorMessage: "echoed method doesn't match request",
			},
			{
				Expression:   `request.url == "` + server.URL + `"`,
				ErrorMessage: "request URL validation failed",
			},
		},
		Timeout: 5 * time.Second,
	}
	instance.SetDefaults()

	// Execute health check
	result := instance.GetHealth(ctx)

	// Assert all validations pass
	assert.NotNil(t, result)
	assert.Equal(t, ph.Status_HEALTHY, result.GetStatus())

	// Verify headers were sent (including lowercase normalization works)
	assert.NotEmpty(t, receivedHeaders.Get("Authorization"))
}
