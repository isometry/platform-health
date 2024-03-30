# Satellite Provider

The Satellite Provider extends the platform-health server to enable delegation of health checks to another platform-health instance. It does this by querying the configured instance(s) and encapsulating the results.

## Usage

Once the Satellite Provider is configured, any query to the platform health server will trigger validation of the configured Satellite instances. A instance is reported "healthy" if-and-only-if the satellite instance reports all of *its* instances as "healthy".

## Configuration

The Satellite Provider is configured through the platform-health server's configuration file, with list of instances under the `satellite` key.

* `name` (required): The name of the Satellite service instance, used to identify the service in the health reports.
* `host` (required): The hostname or IP address of the Satellite service to monitor.
* `port` (default: `8080`): The port number of the Satellite service to monitor.
* `tls` (default: `false`, unless `port` is `443`): Enable TLS for the gRPC dialer.
* `insecure` (default: `false`): Disable certificate validation when TLS is enabled.

### Example

```yaml
satellite:
  - name: example
    host: satellite.example.com
    port: 8080
```

In this example, the Satellite Provider will return the health of the platform-health server and its instances running on `satellite.example.com:8080`.
