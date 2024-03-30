# gRPC Provider

The gRPC Provider extends the platform-health server to enable monitoring of arbitrary external gRPC servers implementing the [gRPC Health Checking Protocol](https://grpc.io/docs/guides/health-checking/).

## Usage

Once the gRPC Provider is configured, any query to the platform-health server will trigger validation of the configured gRPC service(s). The server will attempt to establish a connection to each configured component, and it will report component as "healthy" if the connection is successful and the service reports "SERVING", or "unhealthy" otherwise.

## Configuration

The gRPC Provider is configured through the platform-health server's configuration file, with component instances listed under the `grpc` key.

* `name` (required): The name of the gRPC service instance, used to identify the service in the health reports.
* `host` (required): The hostname or IP address of the gRPC service to monitor.
* `port` (default: `8080`): The port number of the gRPC service to monitor.
* `service` (default: `""`): The service on the target gRPC service to monitor.
* `tls` (default: `false`, unless `port` is `443`): Enable TLS for the gRPC dialer.
* `insecure` (default: `false`): Disable certificate validation when TLS is enabled.

### Example

```yaml
grpc:
  - name: example
    host: grpc.example.com
    port: 443
    service: "foo"
```

In this example, the gRPC Provider will establish a connection to `grpc.example.com` on port 443 (which automatically enables TLS mode), returning "healthy" only if the "foo" service reports "SERVING".
