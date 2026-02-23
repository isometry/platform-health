# Mock Provider

The Mock Provider extends the platform-health server to support internal testing.

## Configuration

The Mock Provider is configured through the platform-health server's configuration file. Each instance is defined with its name as the YAML key under `components`.

- `type` (required): Must be `mock`.
- `spec`: Provider-specific configuration:
  - `health` (default: `HEALTHY`): The health state of the Mock service.
  - `sleep` (default: `1ns`): The delay in returning Mock service status.

## Examples

### Basic Mock Check

```yaml
components:
  test-service:
    type: mock
    spec:
      health: HEALTHY
      sleep: 100ms
```
