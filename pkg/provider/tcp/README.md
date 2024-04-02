# TCP Provider

The TCP Provider extends the platform-health server to enable monitoring the health and status of basic TCP services. It does this by establishing a TCP connection to the specified host and port, and reporting on the success or failure of this operation.

## Usage

Once the TCP Provider is configured, any query to the platform health server will trigger validation of the configured TCP service(s). The server will attempt to establish a TCP connection to each service, and it will report each component as "healthy" if the connection is successful, or "unhealthy" otherwise.

## Configuration

The TCP Provider is configured through the platform-health server's configuration file, with list of instances under the `services.tcp` key.

* `name` (required): The name of the TCP service instance, used to identify the service in the health reports.
* `host` (required): The hostname or IP address of the TCP service to monitor.
* `port` (default: `80`): The port number of the TCP service to monitor.
* `invert` (default: `false`): Reverse logic to report "unhealthy" if port is open and "healthy" if it is closed.
* `timeout` (default: `1s`): The maximum time to wait for a connection to be established before timing out.

### Example

```yaml
tcp:
  - name: example
    host: example.com
    port: 80
    timeout: 1s
```

In this example, the TCP Provider will establish a TCP connection to example.com on port 80 and it will wait for 1s before timing out.
