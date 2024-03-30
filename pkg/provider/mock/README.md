# Mock Provider

The Mock Provider extends the platform-health server to support internal testing.

## Configuration

* `name` (required): The name of the Mock service instance, used to identify the service in the health reports.
* `health` (default: `HEALTHY`): The health state of the Mock service.
* `sleep` (default: `1ns`): The delay in returning Mock service status.
