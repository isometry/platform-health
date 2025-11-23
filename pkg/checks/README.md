# Checks Package

The checks package provides CEL (Common Expression Language) expression evaluation for health check validation. It is used by providers to validate responses and resources using powerful, flexible expressions.

## Supported Providers

The following providers support CEL expressions:

- [`rest`](../provider/rest): Full response with JSON parsing
- [`kubernetes`](../provider/kubernetes): Resource metadata, spec, status
- [`helm`](../provider/helm): Release info, chart metadata, values

## Debugging CEL Expressions

Use `ph context` to inspect the evaluation context available to your expressions:

```bash
# View context for a configured component
ph context my-app

# View context for ad-hoc provider
ph context http --url https://api.example.com/health

# Output as YAML for readability
ph context my-app -o yaml
```

## CEL Expression Syntax

CEL expressions must evaluate to a boolean (`true` for healthy, `false` for unhealthy). Expressions have access to provider-specific context variables (e.g., `response` for REST, `resource` for Kubernetes).

## Common CEL Patterns

### Simple Field Validation

```yaml
checks:
  - expression: "data.ready == true"
    errorMessage: "Service not ready"
```

### Nested Field Access

```yaml
checks:
  - expression: "data.services.database.connected == true"
    errorMessage: "Database not connected"
```

### Numeric Comparisons

```yaml
checks:
  - expression: "data.activeConnections < 1000"
    errorMessage: "Too many active connections"
```

### Array Operations

```yaml
checks:
  - expression: "size(data.errors) == 0"
    errorMessage: "System has reported errors"
  - expression: "size(data.items) > 0"
    errorMessage: "No items in response"
```

### String Operations

```yaml
checks:
  - expression: 'data.message.contains("SUCCESS")'
    errorMessage: "Success message not found"
  - expression: 'data.version.startsWith("2.")'
    errorMessage: "Wrong API version"
```

### Logical Operations

```yaml
checks:
  - expression: "data.value >= 200 && data.value < 300"
    errorMessage: "Value outside expected range"
  - expression: 'data.state == "active" || data.state == "standby"'
    errorMessage: "Service in unexpected state"
```

### Regex Pattern Matching

```yaml
checks:
  - expression: 'data.id.matches("\\d{3}-\\d{2}-\\d{4}")'
    errorMessage: "Invalid format in response"
  - expression: 'data.status.matches("(?i)success|ok|healthy")'
    errorMessage: "No success indicator found"
```

### Existence Checks (Lists)

```yaml
checks:
  - expression: "data.conditions.exists(c, c.type == 'Ready' && c.status == 'True')"
    errorMessage: "Ready condition not met"
```

### Map Key Checks

```yaml
checks:
  - expression: "'required_key' in data.config"
    errorMessage: "Required key missing from config"
```

## Security

- CEL expressions are validated at configuration load time to catch syntax errors early
- CEL expressions are sandboxed and cannot execute arbitrary code or access the filesystem
- Expression evaluation is deterministic and side-effect free
