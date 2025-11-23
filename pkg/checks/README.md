# Checks Package

The checks package provides CEL (Common Expression Language) expression evaluation for health check validation. It is used by providers to validate responses and resources using powerful, flexible expressions.

## Supported Providers

The following providers support CEL expressions:

- [`rest`](../provider/rest): Full response with JSON parsing
- [`kubernetes`](../provider/kubernetes): Resource metadata, spec, status
- [`helm`](../provider/helm): Release info, chart metadata, values

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

CEL expressions must evaluate to a boolean (`true` for healthy, `false` for unhealthy). Expressions have access to provider-specific context variables (e.g., `response` for REST, `resource` for Kubernetes).

## Common CEL Patterns

### Simple Field Validation

```yaml
checks:
  - expr: "data.ready == true"
    message: "Service not ready"
```

### Nested Field Access

```yaml
checks:
  - expr: "data.services.database.connected == true"
    message: "Database not connected"
```

### Numeric Comparisons

```yaml
checks:
  - expr: "data.activeConnections < 1000"
    message: "Too many active connections"
```

### Array Operations

```yaml
checks:
  - expr: "size(data.errors) == 0"
    message: "System has reported errors"
  - expr: "size(data.items) > 0"
    message: "No items in response"
```

### String Operations

```yaml
checks:
  - expr: 'data.message.contains("SUCCESS")'
    message: "Success message not found"
  - expr: 'data.version.startsWith("2.")'
    message: "Wrong API version"
```

### Logical Operations

```yaml
checks:
  - expr: "data.value >= 200 && data.value < 300"
    message: "Value outside expected range"
  - expr: 'data.state == "active" || data.state == "standby"'
    message: "Service in unexpected state"
```

### Regex Pattern Matching

```yaml
checks:
  - expr: 'data.id.matches("\\d{3}-\\d{2}-\\d{4}")'
    message: "Invalid format in response"
  - expr: 'data.status.matches("(?i)success|ok|healthy")'
    message: "No success indicator found"
```

### Existence Checks (Lists)

```yaml
checks:
  - expr: "data.conditions.exists(c, c.type == 'Ready' && c.status == 'True')"
    message: "Ready condition not met"
```

### Map Key Checks

```yaml
checks:
  - expr: "'required_key' in data.config"
    message: "Required key missing from config"
```

## Security

- CEL expressions are validated at configuration load time to catch syntax errors early
- CEL expressions are sandboxed and cannot execute arbitrary code or access the filesystem
- Expression evaluation is deterministic and side-effect free
