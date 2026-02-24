# DNS Provider

The DNS Provider extends the platform-health server to enable DNS lookups and response validation using CEL (Common Expression Language) expressions.

## Usage

Once the DNS Provider is configured, any query to the platform health server will trigger DNS resolution for the configured hostname(s). The server will report each instance as "healthy" if the DNS query succeeds and all CEL checks pass, or "unhealthy" otherwise.

## Configuration

The DNS Provider is configured through the platform-health server's configuration file. Each instance is defined with its name as the YAML key under `components`.

- `type` (required): Must be `dns`.
- `spec`: Provider-specific configuration:
  - `host` (required): The hostname to query.
  - `server` (optional): DNS server IP address (no port). If not specified, uses the system resolver.
  - `port` (default: `0`): DNS port. `0` means auto-select: 53 for plain DNS, 853 for DoT.
  - `serverName` (optional): TLS server name for certificate verification. Required when using an IP address with DNS-over-TLS.
  - `type` (default: `A`): DNS record type (A, AAAA, CNAME, MX, TXT, NS, SOA, SRV, PTR, ANY).
  - `timeout` (default: `5s`): Query timeout.
  - `tls` (default: `false`): Use DNS-over-TLS (DoT).
  - `dnssec` (default: `false`): Enable DNSSEC validation (sets DO flag).
  - `detail` (default: `false`): Include detailed DNS response in output.
- `checks`: A list of CEL expressions to validate the DNS response. Each check has:
  - `check` (required): A CEL expression that must evaluate to `true` for the response to be healthy.
  - `message` (optional): Custom error message when the check fails.

## CEL Check Context

Two variables are exposed to CEL expressions:

### `records`

List of DNS records returned by the query:

- `type`: Record type string (e.g., "A", "CNAME", "MX")
- `value`: String representation of the record value
- `ttl`: TTL in seconds
- `priority`: MX/SRV priority (when applicable)
- `weight`: SRV weight (when applicable)
- `port`: SRV port (when applicable)
- `target`: CNAME/MX/SRV/NS target (when applicable)

### `dnssec`

DNSSEC validation status:

- `dnssec.enabled`: Whether the DO flag was set in the query (bool)
- `dnssec.authenticated`: Whether the AD flag is set in the response (bool)

## Examples

### Simple DNS Resolution Check

```yaml
components:
  google-dns:
    type: dns
    spec:
      host: google.com
```

### Custom DNS Server with Record Type

```yaml
components:
  api-cname:
    type: dns
    spec:
      host: api.example.com
      server: "10.0.0.1"
      type: CNAME
    checks:
      - check: size(records) > 0
        message: No CNAME records found
```

### DNS-over-TLS with DNSSEC Validation

```yaml
components:
  secure-lookup:
    type: dns
    spec:
      host: cloudflare.com
      server: "1.1.1.1"             # Port auto-selected as 853 due to tls: true
      serverName: cloudflare-dns.com  # Required when using IP address with DoT
      tls: true
      dnssec: true
    checks:
      - check: dnssec.authenticated
        message: DNSSEC validation failed
      - check: records.exists(r, r.type == "A")
        message: No A records found
```

### MX Record Validation

```yaml
components:
  mail-config:
    type: dns
    spec:
      host: example.com
      type: MX
    checks:
      - check: records.all(r, r.priority <= 20)
        message: MX priority too high
```

### Multiple Checks

```yaml
components:
  comprehensive-check:
    type: dns
    spec:
      host: example.com
      type: A
    checks:
      - check: size(records) >= 2
        message: Expected at least 2 A records for redundancy
      - check: records.all(r, r.ttl >= 60)
        message: TTL too low
```
