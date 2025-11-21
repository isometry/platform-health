# REST Provider

The REST Provider extends the platform-health server to enable monitoring of RESTful API services with advanced response validation using CEL (Common Expression Language). It validates HTTP responses using powerful expressions that can check JSON content, response headers, status codes, and text patterns.

## Usage

Once the REST Provider is configured, any query to the platform-health server will trigger validation of the configured REST API service(s). The server will send an HTTP request to each service, validate the response status code, and then apply CEL expressions against the response. The service is reported as "healthy" if all validations pass, or "unhealthy" otherwise.

## Configuration

The REST Provider is configured through the platform-health server's configuration file. Each instance is defined with its name as the YAML key.

- `type` (required): Must be `rest`.
- `request` (required): HTTP request configuration with the following fields:
  - `url` (required): The URL of the REST service to monitor.
  - `method` (default: `GET`): The HTTP method to use (GET, POST, PUT, etc.).
  - `body` (optional): Request body to send with POST/PUT requests.
  - `headers` (optional): Map of custom HTTP headers to send with the request (e.g., `{"Authorization": "Bearer token", "Content-Type": "application/json"}`).
- `timeout` (default: `10s`): The maximum time to wait for a response before timing out.
- `insecure` (default: `false`): If set to true, allows the REST provider to establish connections even if the TLS certificate of the service is invalid or untrusted. This is useful for testing or in environments where services use self-signed certificates. Note that using this option in a production environment is not recommended, as it disables important security checks.
- `checks` (optional): List of CEL expressions to validate against the response. Each expression consists of:
  - `expression`: A CEL expression that must evaluate to a boolean. Has access to `request.method`, `request.body`, `request.headers`, `request.url`, `response.json` (parsed JSON), `response.body` (raw text), `response.status` (HTTP status code), and `response.headers` (response headers).
  - `errorMessage`: Custom error message to return if the expression fails.

## Validation Flow

The REST Provider validates responses using CEL expressions. All checks are evaluated in order, and validation stops at the first failure, making the process efficient.

**Note**: Status code validation is now done through CEL expressions. For example:
- Single status: `response.status == 200`
- Multiple statuses: `response.status >= 200 && response.status < 300`
- Specific codes: `response.status == 200 || response.status == 201`

## CEL Expression Context

CEL expressions have access to both `request` and `response` objects:

**Request Context:**
- `request.method`: HTTP method (string)
- `request.body`: Request body as text (string)
- `request.headers`: Request headers (map[string]string, lowercase keys)
- `request.url`: Target URL (string)

**Response Context:**
- `response.json`: Parsed JSON response (null if response is not valid JSON)
- `response.body`: Raw response body as a string
- `response.status`: HTTP status code (int)
- `response.headers`: Response headers (map[string]string, lowercase keys)

## Examples

### Basic JSON Validation

```yaml
api-health:
  type: rest
  request:
    url: https://api.example.com/health
    method: GET
  timeout: 10s
  checks:
    - expression: 'response.status == 200'
      errorMessage: "Expected HTTP 200 status"
    - expression: 'response.json.status == "healthy"'
      errorMessage: "API reports unhealthy status"
    - expression: "response.json.database.connected == true"
      errorMessage: "Database connection failed"
    - expression: "response.json.uptime > 0"
      errorMessage: "Service uptime is zero"
```

In this example, the platform-health server will send a `GET` request to `https://api.example.com/health`, validating that the HTTP status is `200` and that the JSON response contains a `status` field with value `"healthy"`, a nested `database.connected` field with value `true`, and an `uptime` field greater than zero.

### POST with Request Body

```yaml
auth-check:
  type: rest
  request:
    url: https://api.example.com/auth/login
    method: POST
    body: '{"username":"healthcheck","password":"test123"}'
  checks:
    - expression: 'response.status == 200 || (response.status == 401 && response.json.error == "invalid_credentials")'
      errorMessage: "Unexpected authentication response"
```

In this example, the provider sends a POST request with credentials, accepting either a successful login (200) or a specific authentication failure (401 with expected error message).

### Custom Headers (Authentication, Content-Type)

```yaml
authenticated-api:
  type: rest
  request:
    url: https://api.example.com/v1/status
    method: GET
    headers:
      Authorization: "Bearer eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9..."
      X-API-Key: "my-secret-key-123"
  checks:
    - expression: 'response.status == 200'
      errorMessage: "API authentication failed"
    - expression: 'response.json.authenticated == true'
      errorMessage: "Not authenticated"
```

In this example, the provider includes custom headers for API authentication, allowing health checks on protected endpoints.

### Text/HTML Validation with Regex

```yaml
status-page:
  type: rest
  request:
    url: https://status.example.com
    method: GET
  checks:
    - expression: 'response.status == 200'
      errorMessage: "Expected HTTP 200 status"
    - expression: 'response.body.matches("(?i)all systems (operational|normal|healthy)")'
      errorMessage: "Status page doesn't show operational state"
```

In this example, the provider validates that the HTML response contains the text "all systems operational", "all systems normal", or "all systems healthy" (case-insensitive) using CEL's `matches()` function.

### Inverted Pattern Matching (Error Detection)

```yaml
error-detection:
  type: rest
  request:
    url: https://monitor.example.com/status
    method: GET
  checks:
    - expression: 'response.status == 200'
      errorMessage: "Expected HTTP 200 status"
    - expression: '!response.body.matches("(?i)(error|critical|down|failed)")'
      errorMessage: "Error keywords detected in response"
```

In this example, the provider fails if it finds any of the error-related keywords in the response, making it useful for detecting unexpected error states.

### Content-Type Validation

```yaml
json-api:
  type: rest
  request:
    url: https://api.example.com/v1/health
    method: GET
  checks:
    - expression: 'response.status == 200'
      errorMessage: "Expected HTTP 200 status"
    - expression: 'response.headers["content-type"].contains("application/json")'
      errorMessage: "Expected JSON response"
    - expression: 'response.json.ready == true'
      errorMessage: "Service not ready"
```

In this example, the provider validates that the Content-Type header contains "application/json" before checking the JSON content.

### Comprehensive Validation

```yaml
comprehensive-check:
  type: rest
  request:
    url: https://api.example.com/status
    method: GET
  timeout: 15s
  checks:
    - expression: 'response.headers["content-type"] == "application/json"'
      errorMessage: "Wrong content type"
    - expression: 'response.body.matches("\"status\":\\s*\"ok\"")'
      errorMessage: "Status pattern not found"
    - expression: 'response.json.status == "ok"'
      errorMessage: "Service status not ok"
    - expression: 'response.json.checks.database == "ok"'
      errorMessage: "Database check failed"
    - expression: 'response.json.checks.cache == "ok"'
      errorMessage: "Cache check failed"
    - expression: 'response.headers["Content-Type"] == "application/json"'
      errorMessage: "Unexpected content type"
```

In this example, the provider combines Content-Type validation, regex pattern matching, and multiple JSON field checks to thoroughly verify the service health.

## Security Considerations

- Response bodies are limited to 10MB to prevent memory exhaustion.
- CEL expressions are validated at configuration load time to prevent runtime errors.
- TLS certificate validation is enabled by default (use `insecure: true` only for testing).
- CEL expressions are sandboxed and cannot execute arbitrary code or access the filesystem.

For general CEL expression syntax and patterns, see [pkg/checks/README.md](../../checks/README.md).
