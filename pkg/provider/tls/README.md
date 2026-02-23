# TLS Provider

The TLS Provider extends the platform-health server to enable monitoring the health and status of TLS services. It validates TLS connections using flexible CEL (Common Expression Language) expressions that have full access to the connection's certificate and protocol details.

## Usage

Once the TLS Provider is configured, any query to the platform health server will trigger validation of the configured TLS service(s). The server will attempt to establish a TLS connection to each instance, and it will report each instance as "healthy" if the connection is successful and all CEL checks pass, or "unhealthy" if the connection fails, times out, or any check fails. If the `detail` option is set to true, the server will also return detailed information about the TLS connection.

### Ad-hoc Check

```bash
# Basic TLS check
ph check tls --host example.com --port 443

# Check with CEL expression
ph check tls --host example.com --port 443 --check='"example.com" in tls.subjectAltNames'
```

### Context Inspection

Use `ph context` to inspect the available CEL variables before writing expressions:

```bash
ph context tls --host example.com --port 443
```

## Configuration

The TLS Provider is configured through the platform-health server's configuration file. Each instance is defined with its name as the YAML key under `components`.

- `type` (required): Must be `tls`.
- `timeout` (optional): Per-instance timeout override.
- `spec`: Provider-specific configuration:
  - `host` (required): The hostname or IP address of the TLS service to monitor.
  - `port` (default: `443`): The port number of the TLS service to monitor.
  - `insecure` (default: `false`): Allows connections even if the TLS certificate is invalid or untrusted. Useful for testing or self-signed certificates. Not recommended for production.
  - `minValidity` (default: `24h`): The minimum validity period for the TLS certificate. If the remaining validity is less than this value, the service will be reported as "unhealthy".
  - `subjectAltNames` (default: `[]`): Subject Alternate Names which must be present on the presented certificate.
  - `detail` (default: `false`): Include detailed information about the TLS connection in the response.
- `checks`: A list of CEL expressions to validate the TLS connection. Each check has:
  - `check` (required): A CEL expression that must evaluate to `true` for the connection to be healthy.
  - `message` (optional): Custom error message when the check fails.

## CEL Check Context

The TLS provider exposes a `tls` variable containing connection and certificate details:

- `tls.verified`: Whether the certificate chain is verified by the system CA pool (bool). This is evaluated regardless of the `insecure` setting, allowing you to monitor trust status even when allowing untrusted connections.
- `tls.commonName`: Certificate subject common name (string)
- `tls.subjectAltNames`: List of DNS names from Subject Alternative Names (list of strings)
- `tls.chain`: Certificate chain issuers from leaf to root (list of strings)
- `tls.validUntil`: Certificate expiration timestamp (timestamp)
- `tls.signatureAlgorithm`: Certificate signature algorithm (string, e.g., "SHA256-RSA")
- `tls.publicKeyAlgorithm`: Certificate public key algorithm (string, e.g., "RSA")
- `tls.version`: TLS protocol version (string, e.g., "TLS 1.3")
- `tls.cipherSuite`: Negotiated cipher suite (string)
- `tls.protocol`: ALPN negotiated protocol (string, e.g., "h2")
- `tls.serverName`: Server name used for connection (string)
- `tls.port`: Port number used for connection (int)

### Example CEL Expressions

```cel
// Check certificate is verified by system CA pool
tls.verified

// Expect self-signed certificate (not verified)
!tls.verified

// Check for specific SAN
"example.com" in tls.subjectAltNames

// Check for wildcard certificate
tls.subjectAltNames.exists(san, san.startsWith("*."))

// Verify TLS version is 1.3
tls.version == "TLS 1.3"

// Check certificate uses strong signature algorithm
tls.signatureAlgorithm.contains("SHA256") || tls.signatureAlgorithm.contains("SHA384")

// Verify ALPN negotiated HTTP/2
tls.protocol == "h2"

// Check cipher suite contains ECDHE for forward secrecy
tls.cipherSuite.contains("ECDHE")

// Verify certificate chain depth
tls.chain.size() <= 3

// Combined check: modern TLS with HTTP/2
tls.version == "TLS 1.3" && tls.protocol == "h2"
```

## Examples

### Basic TLS Check

```yaml
components:
  example:
    type: tls
    timeout: 1s
    spec:
      host: tls.example.com
      port: 465
      insecure: false
      minValidity: 336h # 14 days
      detail: true
```

In this example, the TLS Provider will establish a TLS connection to `tls.example.com` on port 465, it will wait for 1s before timing out, it will provide detailed information about the TLS connection, it will report the service as "unhealthy" if the remaining validity of the certificate is less than 14 days, and it will not establish connections if the TLS certificate of the service is invalid or untrusted.

### TLS with CEL Checks

```yaml
components:
  secure-api:
    type: tls
    spec:
      host: api.example.com
      port: 443
    checks:
      - check: 'tls.version == "TLS 1.3"'
        message: "Must use TLS 1.3"
      - check: '"api.example.com" in tls.subjectAltNames'
        message: "Certificate must include api.example.com SAN"
```

### Certificate Validation

```yaml
components:
  production-cert:
    type: tls
    spec:
      host: prod.example.com
      port: 443
      minValidity: 720h  # 30 days
    checks:
      - check: 'tls.signatureAlgorithm.contains("SHA256")'
        message: "Certificate must use SHA-256 or stronger"
      - check: 'tls.chain.size() <= 3'
        message: "Certificate chain too deep"
```

### Modern TLS Requirements

```yaml
components:
  modern-endpoint:
    type: tls
    spec:
      host: modern.example.com
      port: 443
    checks:
      - check: 'tls.version == "TLS 1.3"'
        message: "TLS 1.3 required"
      - check: 'tls.protocol == "h2"'
        message: "HTTP/2 required"
      - check: 'tls.cipherSuite.contains("CHACHA20") || tls.cipherSuite.contains("AES_256_GCM")'
        message: "Strong cipher required"
```

### Self-Signed Certificate Monitoring

Use `insecure: true` to allow connections to services with self-signed certificates, while using CEL to verify the expected trust status:

```yaml
components:
  internal-service:
    type: tls
    spec:
      host: internal.example.com
      port: 443
      insecure: true  # Allow connection to proceed
    checks:
      - check: '!tls.verified'
        message: "Expected self-signed certificate"
      - check: 'tls.commonName == "internal.example.com"'
        message: "Unexpected certificate CN"
```

This separates connection behavior (`insecure`) from trust evaluation (`tls.verified`), allowing you to monitor self-signed services while still validating certificate properties.
