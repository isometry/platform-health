# Mock Provider

The Mock Provider extends the platform-health server to support internal testing.

## Configuration

Each instance is defined with its name as the YAML key.

* `type` (required): Must be `mock`.
* `health` (default: `HEALTHY`): The health state of the Mock service.
* `sleep` (default: `1ns`): The delay in returning Mock service status.

### Example

```yaml
test-service:
  type: mock
  health: HEALTHY
  sleep: 100ms
```
