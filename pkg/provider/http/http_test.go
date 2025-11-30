package http_test

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/isometry/platform-health/pkg/checks"
	ph "github.com/isometry/platform-health/pkg/platform_health"
	httpProvider "github.com/isometry/platform-health/pkg/provider/http"
)

func init() {
	slog.SetLogLoggerLevel(slog.LevelError)
}

func TestHTTPProvider_JSONValidation(t *testing.T) {
	tests := []struct {
		name           string
		responseBody   map[string]any
		checks         []checks.Expression
		expectedStatus ph.Status
		expectedMsg    string
	}{
		{
			name: "simple JSON field validation - success",
			responseBody: map[string]any{
				"status": "healthy",
			},
			checks: []checks.Expression{
				{
					Expression: `response.json.status == "healthy"`,
					Message:    "service is not healthy",
				},
			},
			expectedStatus: ph.Status_HEALTHY,
		},
		{
			name: "simple JSON field validation - failure",
			responseBody: map[string]any{
				"status": "unhealthy",
			},
			checks: []checks.Expression{
				{
					Expression: `response.json.status == "healthy"`,
					Message:    "service is not healthy",
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
			checks: []checks.Expression{
				{
					Expression: `response.json.data.database.connected == true`,
					Message:    "database not connected",
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
			checks: []checks.Expression{
				{
					Expression: `response.json.status == "ok"`,
					Message:    "status not ok",
				},
				{
					Expression: `response.json.uptime > 0`,
					Message:    "uptime is zero",
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
			checks: []checks.Expression{
				{
					Expression: `response.json.status == "ok"`,
					Message:    "status not ok",
				},
				{
					Expression: `response.json.uptime > 0`,
					Message:    "uptime is zero",
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
			checks: []checks.Expression{
				{
					Expression: `response.status == 200`,
					Message:    "unexpected status code",
				},
			},
			expectedStatus: ph.Status_HEALTHY,
		},
		{
			name: "array length validation",
			responseBody: map[string]any{
				"items": []any{"a", "b", "c"},
			},
			checks: []checks.Expression{
				{
					Expression: `size(response.json.items) == 3`,
					Message:    "wrong number of items",
				},
			},
			expectedStatus: ph.Status_HEALTHY,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
			defer cancel()

			// Create test server
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusOK)
				_ = json.NewEncoder(w).Encode(tt.responseBody)
			}))

			t.Cleanup(func() {
				server.Close()
			})

			// Create HTTP provider instance
			instance := &httpProvider.Component{
				URL:    server.URL,
				Method: "GET",
			}
			instance.SetName("test-service")
			instance.SetTimeout(5 * time.Second)
			require.NoError(t, instance.Setup())
			if len(tt.checks) > 0 {
				require.NoError(t, instance.SetChecks(tt.checks))
			}

			// Execute health check
			result := instance.GetHealth(ctx)

			// Assert results
			assert.NotNil(t, result)
			assert.Equal(t, tt.expectedStatus, result.GetStatus())
			if tt.expectedMsg != "" {
				require.NotEmpty(t, result.Messages)
				assert.Contains(t, result.Messages[0], tt.expectedMsg)
			}
		})
	}
}

func TestHTTPProvider_POSTWithBody(t *testing.T) {
	tests := []struct {
		name           string
		requestBody    string
		expectedMethod string
		expectedBody   string
		responseBody   map[string]any
		checks         []checks.Expression
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
			checks: []checks.Expression{
				{
					Expression: `response.json.authenticated == true`,
					Message:    "authentication failed",
				},
			},
			expectedStatus: ph.Status_HEALTHY,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
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

			t.Cleanup(func() {
				server.Close()
			})

			// Create HTTP provider instance
			instance := &httpProvider.Component{
				URL:    server.URL,
				Method: "POST",
				Body:   tt.requestBody,
			}
			instance.SetName("test-service")
			instance.SetTimeout(5 * time.Second)
			require.NoError(t, instance.Setup())
			if len(tt.checks) > 0 {
				require.NoError(t, instance.SetChecks(tt.checks))
			}

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

func TestHTTPProvider_StatusCodeValidation(t *testing.T) {
	tests := []struct {
		name           string
		serverStatus   int
		checks         []checks.Expression
		expectedResult ph.Status
	}{
		{
			name:         "status match - single expected",
			serverStatus: 200,
			checks: []checks.Expression{
				{
					Expression: `response.status == 200`,
					Message:    "expected status 200",
				},
			},
			expectedResult: ph.Status_HEALTHY,
		},
		{
			name:         "status match - multiple expected",
			serverStatus: 201,
			checks: []checks.Expression{
				{
					Expression: `response.status >= 200 && response.status < 300`,
					Message:    "expected 2xx status",
				},
			},
			expectedResult: ph.Status_HEALTHY,
		},
		{
			name:         "status mismatch",
			serverStatus: 500,
			checks: []checks.Expression{
				{
					Expression: `response.status == 200`,
					Message:    "expected status 200",
				},
			},
			expectedResult: ph.Status_UNHEALTHY,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
			defer cancel()

			// Create test server
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tt.serverStatus)
				_, _ = w.Write([]byte("{}"))
			}))

			t.Cleanup(func() {
				server.Close()
			})

			// Create HTTP provider instance
			instance := &httpProvider.Component{
				URL:    server.URL,
				Method: "GET",
			}
			instance.SetName("test-service")
			instance.SetTimeout(5 * time.Second)
			require.NoError(t, instance.Setup())
			if len(tt.checks) > 0 {
				require.NoError(t, instance.SetChecks(tt.checks))
			}

			// Execute health check
			result := instance.GetHealth(ctx)

			// Assert results
			assert.NotNil(t, result)
			assert.Equal(t, tt.expectedResult, result.GetStatus())
		})
	}
}

func TestHTTPProvider_CombinedValidation(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
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

	t.Cleanup(func() {
		server.Close()
	})

	// Create HTTP provider with CEL validation
	instance := &httpProvider.Component{
		URL:    server.URL,
		Method: "GET",
	}
	instance.SetName("test-service")
	instance.SetTimeout(5 * time.Second)
	require.NoError(t, instance.Setup())
	require.NoError(t, instance.SetChecks([]checks.Expression{
		{
			Expression: `response.status == 200`,
			Message:    "expected status 200",
		},
		{
			Expression: `response.body.matches('"status":\\s*"healthy"')`,
			Message:    "status pattern not found",
		},
		{
			Expression: `response.json.status == "healthy"`,
			Message:    "service unhealthy",
		},
		{
			Expression: `response.json.checks.database == "ok"`,
			Message:    "database check failed",
		},
		{
			Expression: `response.status == 200`,
			Message:    "unexpected status code",
		},
		{
			Expression: `response.headers["content-type"] == "application/json"`,
			Message:    "wrong content type",
		},
	}))

	// Execute health check
	result := instance.GetHealth(ctx)

	// Assert all validations pass
	assert.NotNil(t, result)
	assert.Equal(t, ph.Status_HEALTHY, result.GetStatus())
}

func TestHTTPProvider_ContentTypeValidation(t *testing.T) {
	tests := []struct {
		name           string
		contentType    string
		checks         []checks.Expression
		expectedStatus ph.Status
	}{
		{
			name:        "content type matches JSON",
			contentType: "application/json",
			checks: []checks.Expression{
				{
					Expression: `response.headers["content-type"] == "application/json"`,
					Message:    "Expected JSON content type",
				},
			},
			expectedStatus: ph.Status_HEALTHY,
		},
		{
			name:        "content type contains JSON",
			contentType: "application/json; charset=utf-8",
			checks: []checks.Expression{
				{
					Expression: `response.headers["content-type"].contains("application/json")`,
					Message:    "Expected JSON content type",
				},
			},
			expectedStatus: ph.Status_HEALTHY,
		},
		{
			name:        "content type mismatch",
			contentType: "text/html",
			checks: []checks.Expression{
				{
					Expression: `response.headers["content-type"] == "application/json"`,
					Message:    "Expected JSON content type",
				},
			},
			expectedStatus: ph.Status_UNHEALTHY,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
			defer cancel()

			// Create test server
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", tt.contentType)
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write([]byte(`{"status":"ok"}`))
			}))

			t.Cleanup(func() {
				server.Close()
			})

			// Create HTTP provider instance
			instance := &httpProvider.Component{
				URL:    server.URL,
				Method: "GET",
			}
			instance.SetName("test-service")
			instance.SetTimeout(5 * time.Second)
			require.NoError(t, instance.Setup())
			if len(tt.checks) > 0 {
				require.NoError(t, instance.SetChecks(tt.checks))
			}

			// Execute health check
			result := instance.GetHealth(ctx)

			// Assert results
			assert.NotNil(t, result)
			assert.Equal(t, tt.expectedStatus, result.GetStatus())
		})
	}
}

func TestHTTPProvider_ErrorCases(t *testing.T) {
	t.Run("invalid CEL expression syntax", func(t *testing.T) {
		instance := &httpProvider.Component{
			URL:    "http://localhost",
			Method: "GET",
		}
		instance.SetName("test-service")
		instance.SetTimeout(5 * time.Second)
		require.NoError(t, instance.Setup())

		// SetChecks should fail with invalid CEL expression
		err := instance.SetChecks([]checks.Expression{
			{
				Expression: `invalid syntax here!!!`,
				Message:    "validation failed",
			},
		})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "invalid CEL expression")
	})
}

func TestHTTPProvider_RequestContextValidation(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
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

	t.Cleanup(func() {
		server.Close()
	})

	// Create HTTP provider with request context validation
	instance := &httpProvider.Component{
		URL:    server.URL,
		Method: "POST",
		Body:   `{"test":"data"}`,
		Headers: map[string]string{
			"Accept":        "application/json",
			"Authorization": "Bearer token123",
		},
	}
	instance.SetName("test-service")
	instance.SetTimeout(5 * time.Second)
	require.NoError(t, instance.Setup())
	require.NoError(t, instance.SetChecks([]checks.Expression{
		{
			Expression: `request.method == "POST"`,
			Message:    "request method validation failed",
		},
		{
			Expression: `request.body.contains("test")`,
			Message:    "request body validation failed",
		},
		{
			Expression: `request.headers["accept"] == "application/json"`,
			Message:    "request accept header missing",
		},
		{
			Expression: `response.headers["content-type"] == request.headers["accept"]`,
			Message:    "content negotiation failed",
		},
		{
			Expression: `response.json.echo_method == request.method`,
			Message:    "echoed method doesn't match request",
		},
		{
			Expression: `request.url == "` + server.URL + `"`,
			Message:    "request URL validation failed",
		},
	}))

	// Execute health check
	result := instance.GetHealth(ctx)

	// Assert all validations pass
	assert.NotNil(t, result)
	assert.Equal(t, ph.Status_HEALTHY, result.GetStatus())

	// Verify headers were sent (including lowercase normalization works)
	assert.NotEmpty(t, receivedHeaders.Get("Authorization"))
}

func TestHTTPProvider_HEADMethod(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()

	var receivedMethod string

	// Create test server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedMethod = r.Method
		w.WriteHeader(http.StatusOK)
	}))

	t.Cleanup(func() {
		server.Close()
	})

	// Create HTTP provider instance with default method (HEAD)
	instance := &httpProvider.Component{
		URL: server.URL,
	}
	instance.SetName("test-service")
	instance.SetTimeout(5 * time.Second)
	require.NoError(t, instance.Setup())
	require.NoError(t, instance.SetChecks([]checks.Expression{
		{
			Expression: `response.status == 200`,
			Message:    "expected status 200",
		},
	}))

	// Execute health check
	result := instance.GetHealth(ctx)

	// Assert HEAD method was used (default)
	assert.Equal(t, "HEAD", receivedMethod)
	assert.NotNil(t, result)
	assert.Equal(t, ph.Status_HEALTHY, result.GetStatus())
}
