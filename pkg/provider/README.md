# Platform Health Providers

Platform Health Providers are extensions to the platform-health server. They enable the server to report on the health and status of a variety of external systems. This extensibility allows the platform-health server to be a versatile tool for monitoring and maintaining the health of your entire platform.

## Core Interfaces

To create a new provider, there are a few requirements:

- **Implement the [`provider.Instance`](provider.go) interface**:

```go
type Instance interface {
    GetType() string
    GetName() string
    SetName(string)
    GetTimeout() time.Duration
    SetTimeout(time.Duration)
    GetHealth(context.Context) *ph.HealthCheckResponse
    Setup() error
}
```

Methods:

- `GetType()`: Returns provider type (e.g., "tcp", "http", "system", etc.)
- `GetName()`: Returns instance name (from config key)
- `SetName()`: Sets the instance name
- `GetTimeout()`: Returns the per-instance timeout override (0 means use parent context deadline)
- `SetTimeout()`: Sets the per-instance timeout override
- `GetHealth()`: Performs the actual health check
- `Setup()`: Sets default configuration and initializes the instance

* **Register with the internal registry**: Providers must register themselves with the platform-health server's internal registry. This is done with a call to [`provider.Register`](registry.go) in an `init()` function. The `init()` function is automatically called when the program starts, registering the provider before the server begins handling requests.

* **Include via blank import**: To include the provider in the server, it must be imported using a blank import statement (i.e., `_ path/to/module`) in the [main command](../../cmd/ph).

By following these guidelines, you can extend the platform-health server to interact with any external system, making it a powerful tool for platform health monitoring.

## Optional Interfaces

### InstanceWithChecks

Providers can implement the `InstanceWithChecks` interface to support CEL (Common Expression Language) expressions for health checks:

```go
type InstanceWithChecks interface {
    GetCheckConfig() *checks.CEL
    GetCheckContext(ctx context.Context) (map[string]any, error)
    GetChecks() []checks.Expression
    SetChecks([]checks.Expression) error
}
```

Methods:

- `GetCheckConfig()`: Returns CEL configuration for the provider
- `GetCheckContext()`: Returns evaluation context map for CEL expressions
- `GetChecks()`: Returns configured CEL expressions
- `SetChecks()`: Sets and compiles CEL expressions; returns an error if any expression is invalid

This enables:

- Custom validation logic via CEL expressions
- Context inspection via `ph context` command
- Rich evaluation contexts with provider-specific data

### BaseWithChecks

The `BaseWithChecks` struct provides reusable CEL handling that can be embedded by providers:

```go
type BaseWithChecks struct {
    checks   []checks.Expression
    compiled []*checks.Check
}
```

Embed this in your provider to get default implementations of `GetChecks()`, `HasChecks()`, `EvaluateChecks()`, and `EvaluateChecksByMode()`. In your provider's `SetChecks()` method, call `SetChecksAndCompile(exprs, celConfig)` to compile the CEL expressions against your provider's CEL configuration.

### Container

Providers that group related child health checks implement the `Container` interface:

```go
type Container interface {
    SetComponents(config map[string]any)
    GetComponents() []Instance
    ComponentErrors() []error
}
```

Methods:

- `SetComponents()`: Stores raw component configuration (called by factory before Setup)
- `GetComponents()`: Returns resolved child instances (available after Setup)
- `ComponentErrors()`: Returns validation errors from component resolution

The `BaseContainer` struct provides a default implementation. Call `ResolveComponents()` from your provider's `Setup()` method to resolve child instances from stored config.
