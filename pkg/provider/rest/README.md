# REST Provider

The REST Provider extends the platform-health server to enable monitoring of RESTful API services with advanced response validation using CEL (Common Expression Language). It validates HTTP responses using powerful expressions that can check JSON content, response headers, status codes, and text patterns.

## Usage

Once the REST Provider is configured, any query to the platform-health server will trigger validation of the configured REST API service(s). The server will send an HTTP request to each service, validate the response status code, and then apply CEL expressions against the response. The service is reported as "healthy" if all validations pass, or "unhealthy" otherwise.

## Configuration

The REST Provider is configured through the platform-health server's configuration file, with component instances listed under the `rest` key.

- `name` (required): The name of the REST service instance, used to identify the service in the health reports.
- `url` (required): The URL of the REST service to monitor.
- `request` (optional): HTTP request configuration with the following fields:
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
rest:
  - name: api-health
    url: https://api.example.com/health
    request:
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
rest:
  - name: auth-check
    url: https://api.example.com/auth/login
    request:
      method: POST
      body: '{"username":"healthcheck","password":"test123"}'
    checks:
      - expression: 'response.status == 200 || (response.status == 401 && response.json.error == "invalid_credentials")'
        errorMessage: "Unexpected authentication response"
```

In this example, the provider sends a POST request with credentials, accepting either a successful login (200) or a specific authentication failure (401 with expected error message).

### Custom Headers (Authentication, Content-Type)

```yaml
rest:
  - name: authenticated-api
    url: https://api.example.com/v1/status
    request:
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
rest:
  - name: status-page
    url: https://status.example.com
    request:
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
rest:
  - name: error-detection
    url: https://monitor.example.com/status
    request:
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
rest:
  - name: json-api
    url: https://api.example.com/v1/health
    request:
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
rest:
  - name: comprehensive-check
    url: https://api.example.com/status
    request:
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

## CEL Expression Examples

### Simple Field Validation

```yaml
checks:
  - expression: "response.json.ready == true"
    errorMessage: "Service not ready"
```

### Nested Field Access

```yaml
checks:
  - expression: "response.json.services.database.connected == true"
    errorMessage: "Database not connected"
```

### Numeric Comparisons

```yaml
checks:
  - expression: "response.json.activeConnections < 1000"
    errorMessage: "Too many active connections"
```

### Array Operations

```yaml
checks:
  - expression: "size(response.json.errors) == 0"
    errorMessage: "System has reported errors"
  - expression: "size(response.json.items) > 0"
    errorMessage: "No items in response"
```

### String Operations

```yaml
checks:
  - expression: 'response.body.contains("SUCCESS")'
    errorMessage: "Success message not found"
  - expression: 'response.json.version.startsWith("2.")'
    errorMessage: "Wrong API version"
  - expression: 'response.headers["content-type"].contains("json")'
    errorMessage: "Expected JSON content type"
```

### Logical Operations

```yaml
checks:
  - expression: "response.status >= 200 && response.status < 300"
    errorMessage: "Status code outside success range"
  - expression: 'response.json.state == "active" || response.json.state == "standby"'
    errorMessage: "Service in unexpected state"
```

### Regex Pattern Matching

```yaml
checks:
  - expression: 'response.body.matches("\\d{3}-\\d{2}-\\d{4}")'
    errorMessage: "Invalid format in response"
  - expression: 'response.body.matches("(?i)success|ok|healthy")'
    errorMessage: "No success indicator found"
```

### Header Validation

```yaml
checks:
  - expression: 'response.headers["X-API-Version"].startsWith("v2")'
    errorMessage: "Wrong API version in header"
  - expression: 'response.headers["Cache-Control"] == "no-cache"'
    errorMessage: "Unexpected cache control"
```

## Security Considerations

- Response bodies are limited to 10MB to prevent memory exhaustion.
- CEL expressions are validated at configuration load time to prevent runtime errors.
- TLS certificate validation is enabled by default (use `insecure: true` only for testing).
- CEL expressions are sandboxed and cannot execute arbitrary code or access the filesystem.

## Migration Notes

### From bodyMatch

If you were using the `bodyMatch` field in a previous version, you can migrate to CEL expressions:

**Old (bodyMatch):**

```yaml
bodyMatch:
  pattern: "healthy"
```

**New (CEL):**

```yaml
checks:
  - expression: 'response.body.contains("healthy")'
    errorMessage: "Healthy status not found"
```

**Old (inverted bodyMatch):**

```yaml
bodyMatch:
  pattern: "error"
  invert: true
```

**New (CEL with negation):**

```yaml
checks:
  - expression: '!response.body.contains("error")'
    errorMessage: "Error found in response"
```

### From status field

The `status` field has been removed in favor of CEL expressions for status code validation:

**Old:**

```yaml
status: [200]
```

**New:**

```yaml
checks:
  - expression: 'response.status == 200'
    errorMessage: "Expected HTTP 200 status"
```

**Old (multiple status codes):**

```yaml
status: [200, 201, 202]
```

**New (using range):**

```yaml
checks:
  - expression: 'response.status >= 200 && response.status < 300'
    errorMessage: "Expected 2xx status"
```

**New (using specific codes):**

```yaml
checks:
  - expression: 'response.status == 200 || response.status == 201 || response.status == 202'
    errorMessage: "Expected 200, 201, or 202 status"
```
