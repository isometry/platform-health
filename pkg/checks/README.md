# Checks Package

The checks package provides CEL (Common Expression Language) expression evaluation for health check validation. It is used by providers to validate responses and resources using powerful, flexible expressions.

## Debugging CEL Expressions

Use `ph context` to inspect the evaluation context available to your CEL expressions:

```bash
# View context for a configured component
ph context my-app

# View context for ad-hoc provider
ph context kubernetes --kind deployment --namespace default --name my-app

# Output as YAML for readability
ph context my-app -o yaml
```

## CEL Expression Syntax

CEL expressions must evaluate to a boolean (`true` for healthy, `false` for unhealthy). Expressions have access to provider-specific context variables (e.g., `response` for HTTP, `resource` for Kubernetes).

### Check Modes

Each check can optionally specify a `mode` field:

- **default** (no `mode` field): The expression is evaluated once against the full provider context.
- **`mode: "each"`**: The expression is evaluated per-item for providers that return collections (e.g., Kubernetes with label selectors). Each item is evaluated independently, and failures reference the specific item.

```yaml
checks:
  - check: "resource.status.readyReplicas >= resource.spec.replicas"
    message: "Not all replicas ready"
    mode: "each"
```

## Built-in Functions

The following custom functions are available in all CEL expressions:

- `time.Now()` - Returns the current timestamp
- `time.Since(timestamp)` - Returns the duration elapsed since the given timestamp
- `time.Until(timestamp)` - Returns the duration until the given timestamp

```yaml
checks:
  - check: "time.Until(tls.validUntil) > duration('168h')"
    message: "Certificate expires within 7 days"
```

## Standard Extensions

The following CEL extension libraries are available:

- **Strings**: Additional string functions (`charAt`, `indexOf`, `replace`, `split`, `substring`, `trim`, `upperAscii`, `lowerAscii`)
- **Lists**: List manipulation functions (`slice`, `flatten`)
- **Encoders**: Base64 encoding/decoding (`base64.encode`, `base64.decode`)
- **Math**: Math functions (`math.greatest`, `math.least`)
- **Sets**: Set operations (`sets.contains`, `sets.intersects`, `sets.equivalent`)
- **Bindings**: Variable binding via `cel.bind()`

## Common CEL Patterns

> **Note:** The examples below use `data` as a generic illustrative placeholder variable. Actual CEL context variables are provider-specific — for example, `response` for [HTTP](../provider/http), `resource` for [Kubernetes](../provider/kubernetes), `tls` for [TLS](../provider/tls), `health` for [Vault](../provider/vault), and `release`/`chart` for [Helm](../provider/helm). See each provider's README for its available CEL variables, or use `ph context` to inspect the evaluation context.

### Simple Field Validation

```yaml
checks:
  - check: "data.ready == true"
    message: "Service not ready"
```

### Nested Field Access

```yaml
checks:
  - check: "data.services.database.connected == true"
    message: "Database not connected"
```

### Numeric Comparisons

```yaml
checks:
  - check: "data.activeConnections < 1000"
    message: "Too many active connections"
```

### Array Operations

```yaml
checks:
  - check: "size(data.errors) == 0"
    message: "System has reported errors"
  - check: "size(data.items) > 0"
    message: "No items in response"
```

### String Operations

```yaml
checks:
  - check: 'data.message.contains("SUCCESS")'
    message: "Success message not found"
  - check: 'data.version.startsWith("2.")'
    message: "Wrong API version"
```

### Logical Operations

```yaml
checks:
  - check: "data.value >= 200 && data.value < 300"
    message: "Value outside expected range"
  - check: 'data.state == "active" || data.state == "standby"'
    message: "Service in unexpected state"
```

### Regex Pattern Matching

```yaml
checks:
  - check: 'data.id.matches("\\d{3}-\\d{2}-\\d{4}")'
    message: "Invalid format in response"
  - check: 'data.status.matches("(?i)success|ok|healthy")'
    message: "No success indicator found"
```

### Existence Checks (Lists)

```yaml
checks:
  - check: "data.conditions.exists(c, c.type == 'Ready' && c.status == 'True')"
    message: "Ready condition not met"
```

### Map Key Checks

```yaml
checks:
  - check: "'required_key' in data.config"
    message: "Required key missing from config"
```

## Security

- CEL expressions are validated at configuration load time to catch syntax errors early
- CEL expressions are sandboxed and cannot execute arbitrary code or access the filesystem
- Expression evaluation is deterministic and side-effect free
