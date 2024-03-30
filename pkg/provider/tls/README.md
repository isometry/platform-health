# TLS Provider

The TLS Provider extends the platform-health server to enable monitoring the health and status of TLS services. It does this by establishing a TLS connection to the specified host and port, and reporting on the success or failure of this operation.

## Usage

Once the TLS Provider is configured, any query to the platform health server will trigger validation of the configured TLS service(s). The server will attempt to establish a TLS connection to each instance, and it will report each instance as "healthy" if the connection is successful, or "unhealthy" if the connection fails or times out. If the `detail` option is set to true, the server will also return detailed information about the TLS connection.

## Configuration

The TLS Provider is configured through the platform-health server's configuration file, with list of instances under the `tls` key.

* `name` (required): The name of the TLS service instance, used to identify the service in the health reports.
* `host` (required): The hostname or IP address of the TLS service to monitor.
* `port` (default: 443): The port number of the TLS service to monitor.
* `timeout` (default: 1s): The maximum time to wait for a connection to be established before timing out.
* `insecure` (default: false): If set to true, allows the TLS provider to establish connections even if the TLS certificate of the service is invalid or untrusted. This is useful for testing or in environments where services use self-signed certificates. Note that using this option in a production environment is not recommended, as it disables important security checks.
* `minValidity` (default: 24h): The minimum validity period for the TLS certificate of the service being monitored. If the remaining validity of the certificate is less than this value, the service will be reported as "unhealthy". The value is specified in hours.
* `subjectAltNames` (default: `[]`): Subject Alternate Names which must be present on the presented certificate.
* `detail` (default: false): If set to true, the provider will return detailed information about the TLS connection, such as the common name, subject alternative names, validity period, signature algorithm, public key algorithm, version, cipher suite, and protocol.

### Example

```yaml
tls:
  - name: example
    host: tls.example.com
    port: 465
    timeout: 1s
    insecure: false
    minValidity: 336h # 14 days
    detail: true
```

In this example, the TLS Provider will establish a TLS connection to `tls.example.com` on port 465, it will wait for 1s before timing out, it will provide detailed information about the TLS connection, it will report the service as "unhealthy" if the remaining validity of the certificate is less than 14 days, and it will not establish connections if the TLS certificate of the service is invalid or untrusted.
