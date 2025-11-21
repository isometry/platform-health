# HTTP Provider

The HTTP Provider extends the platform-health server to enable monitoring the health of HTTP services. It does this by sending an HTTP request to the specified URL and reporting on the success or failure of this operation based upon the status code of the response.

## Usage

Once the HTTP Provider is configured, any query to the platform-health server will trigger validation of the configured HTTP(S) service(s). The server will attempt to send an HTTP request to each service, and it will report each service as "healthy" if the request is successful and the status code matches one of the expected status codes, or "unhealthy" otherwise.

## Configuration

The HTTP Provider is configured through the platform-health server's configuration file. Each instance is defined with its name as the YAML key.

* `type` (required): Must be `http`.
* `url` (required): The URL of the HTTP service to monitor.
* `method` (default: `HEAD`): The HTTP method to use for the request.
* `timeout` (default: `10s`): The maximum time to wait for a response before timing out.
* `insecure` (default: `false`): If set to true, allows the HTTP provider to establish connections even if the TLS certificate of the service is invalid or untrusted. This is useful for testing or in environments where services use self-signed certificates. Note that using this option in a production environment is not recommended, as it disables important security checks.
* `status` (default: `[200]`): The list of HTTP status codes that are expected in the response.
* `detail` (default: `false`): If set to true, the provider will return detailed information about the HTTP connection.

### Example

```yaml
example:
  type: http
  url: https://example.com
  method: GET
  detail: true
```

In this example, the platform-health server will send a `GET` request to `https://example.com`; it will allow the default `10s` before timing out; it will expect the HTTP status code to be `200`; it will not establish connections if the HTTP certificate of the service is invalid or untrusted; and it will provide additional detailed information about the HTTP connection.
