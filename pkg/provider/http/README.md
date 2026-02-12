# HTTP Provider

The HTTP Provider extends the platform-health server to enable health checking of HTTP/HTTPS services with powerful response validation using CEL (Common Expression Language) expressions.

## Features

- HTTP/HTTPS requests with configurable methods (HEAD, GET, POST, PUT, DELETE, etc.)
- Request body and custom headers support
- TLS certificate verification and optional detail extraction
- Response validation using CEL expressions
- JSON response parsing and field validation
- Status code, header, and body validation

## Usage

Once the HTTP Provider is configured, any query to the platform health server will trigger an HTTP request to each configured endpoint. The server will report each instance as "healthy" if the request succeeds and all CEL checks pass, or "unhealthy" otherwise.

## Configuration

The HTTP Provider is configured through the platform-health server's configuration file. Each instance is defined with its name as the YAML key under `components`.

- `type` (required): Must be `http`.
- `timeout` (optional): Per-instance timeout override.
- `spec`: Provider-specific configuration:
  - `url` (required): Target URL.
  - `method` (default: `HEAD`): HTTP method.
  - `body` (optional): Request body.
  - `headers` (optional): Custom HTTP headers as a map.
  - `insecure` (default: `false`): Skip TLS certificate validation.
  - `detail` (default: `false`): Include TLS certificate details in response.
- `checks`: A list of CEL expressions to validate the response. Each check has:
  - `check` (required): A CEL expression that must evaluate to `true` for the response to be healthy.
  - `message` (optional): Custom error message when the check fails.

## CEL Check Context

The following variables are available in CEL expressions:

### Request Context

- `request.method`: HTTP method (string)
- `request.body`: Request body text (string)
- `request.headers`: Request headers as map (lowercase keys)
- `request.url`: Target URL (string)

### Response Context

- `response.status`: HTTP status code (int)
- `response.body`: Raw response body as text (string)
- `response.json`: Parsed JSON response (map, nil if not JSON)
- `response.headers`: Response headers as map (lowercase keys)

### Example CEL Expressions

```cel
// Status code validation
response.status == 200

// Status range validation
response.status >= 200 && response.status < 300

// Multiple accepted statuses
response.status == 200 || response.status == 201

// JSON field validation
response.json.status == "healthy"

// Nested JSON validation
response.json.data.database.connected == true

// Array length validation
size(response.json.items) > 0

// Header validation
response.headers["content-type"].contains("application/json")

// Body text validation
response.body.contains("success")

// Regex pattern matching
response.body.matches("(?i)all systems operational")
```

## Default Health Behavior

When no `checks` are configured, the HTTP provider considers any response with a status code in the range 200-399 as healthy. Status codes 400 and above are reported as unhealthy. To customize this behavior, use CEL checks.

## TLS Details

When `detail: true` is set and the request uses HTTPS, the response will include TLS certificate information:

- Common Name
- Subject Alternative Names (SANs)
- Certificate validity period
- Signature algorithm
- Public key algorithm
- TLS version
- Cipher suite
- Protocol (e.g., "h2" for HTTP/2)
- Certificate chain

## Examples

### Simple Health Check

```yaml
components:
  google:
    type: http
    spec:
      url: https://google.com
```

### API Health Check with JSON Validation

```yaml
components:
  api-health:
    type: http
    timeout: 5s
    spec:
      url: https://api.example.com/health
      method: GET
    checks:
      - check: 'response.status == 200'
        message: "API returned non-200 status"
      - check: 'response.json.status == "healthy"'
        message: "API reports unhealthy status"
      - check: 'response.json.database.connected == true'
        message: "Database connection failed"
```

### Authenticated API Check

```yaml
components:
  auth-api:
    type: http
    timeout: 10s
    spec:
      url: https://api.example.com/v1/status
      method: POST
      body: '{"action": "healthcheck"}'
      headers:
        Authorization: "Bearer ${API_TOKEN}"
        Content-Type: "application/json"
    checks:
      - 'response.status == 200'
      - check: 'response.json.authenticated == true'
        message: "Authentication failed"
```

### DNS-over-HTTPS Check

```yaml
components:
  dns-check:
    type: http
    spec:
      url: https://dns.google.com/resolve?name=example.com&type=A
      method: GET
    checks:
      - check: 'response.status == 200'
        message: "DNS query failed"
      - check: 'response.json.Status == 0'
        message: "DNS lookup returned error"
      - check: 'size(response.json.Answer) > 0'
        message: "No DNS records found"
```
