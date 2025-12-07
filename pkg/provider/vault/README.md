# Vault Provider

The Vault Provider extends the platform-health server to enable monitoring the health of [HashiCorp Vault](https://www.vaultproject.io/) servers. It validates Vault health using flexible CEL (Common Expression Language) expressions that have full access to the [sys/health](https://developer.hashicorp.com/vault/api-docs/system/health) endpoint response.

## Usage

Once the Vault Provider is configured, any query to the platform health server will trigger validation of the configured Vault service(s). The server will query the health endpoint and report the service as "healthy" if it returns `initialized: true` and `sealed: false` and all CEL checks pass, or "unhealthy" otherwise.

### Ad-hoc Check

```bash
# Basic Vault health check
ph check vault --address https://vault.example.com

# Check with CEL expression
ph check vault --address https://vault.example.com --check='!health.Standby'
```

### Context Inspection

Use `ph context` to inspect the available CEL variables before writing expressions:

```bash
ph context vault --address https://vault.example.com
```

## Configuration

The Vault Provider is configured through the platform-health server's configuration file. Each instance is defined with its name as the YAML key under `components`.

- `type` (required): Must be `vault`.
- `timeout` (default: `1s`): The maximum time to wait for a response before timing out.
- `spec`: Provider-specific configuration:
  - `address` (required): The address of the Vault instance in standard `VAULT_ADDR` format.
  - `insecure` (default: `false`): Allows connections even if the TLS certificate is invalid or untrusted. Not recommended for production.
- `checks`: A list of CEL expressions to validate the Vault health. Each check has:
  - `check` (required): A CEL expression that must evaluate to `true` for the service to be healthy.
  - `message` (optional): Custom error message when the check fails.

## CEL Check Context

The Vault provider exposes a `health` variable containing all fields from the sys/health endpoint:

- `health.Initialized`: Whether Vault has been initialized (bool)
- `health.Sealed`: Whether Vault is sealed (bool)
- `health.Standby`: Whether Vault is in standby mode (bool)
- `health.PerformanceStandby`: Whether Vault is a performance standby (bool)
- `health.ReplicationPerformanceMode`: Replication performance mode (string)
- `health.ReplicationDRMode`: Disaster recovery replication mode (string)
- `health.ServerTimeUTC`: Server time in UTC (int64, Unix timestamp)
- `health.Version`: Vault version (string)
- `health.ClusterName`: Name of the Vault cluster (string)
- `health.ClusterID`: Unique identifier for the cluster (string)

### Example CEL Expressions

```cel
// Check Vault is not in standby mode
!health.Standby

// Verify specific Vault version
health.Version.startsWith("1.15")

// Check for minimum version (simple string comparison)
health.Version >= "1.14.0"

// Ensure active node (not standby or performance standby)
!health.Standby && !health.PerformanceStandby

// Verify cluster name matches expected
health.ClusterName == "production-vault"

// Check DR replication is disabled
health.ReplicationDRMode == "" || health.ReplicationDRMode == "disabled"
```

## Examples

### Basic Health Check

```yaml
components:
  example:
    type: vault
    spec:
      address: https://vault.example.com
```

In this example, the platform-health server will validate that the Vault cluster running at `https://vault.example.com` is up, initialized and unsealed.

### Active Node Validation

```yaml
components:
  vault-active:
    type: vault
    spec:
      address: https://vault.example.com
    checks:
      - check: '!health.Standby'
        message: "Vault node is in standby mode"
```

### Version Requirements

```yaml
components:
  vault-prod:
    type: vault
    spec:
      address: https://vault.example.com
    checks:
      - check: 'health.Version.startsWith("1.15")'
        message: "Vault must be version 1.15.x"
```

### Cluster Validation

```yaml
components:
  vault-cluster:
    type: vault
    spec:
      address: https://vault.example.com
    checks:
      - check: 'health.ClusterName == "production-vault"'
        message: "Must be connected to production cluster"
      - check: '!health.Standby && !health.PerformanceStandby'
        message: "Must be an active node"
```

### DR Replication Check

```yaml
components:
  vault-dr:
    type: vault
    spec:
      address: https://vault-dr.example.com
    checks:
      - check: 'health.ReplicationDRMode == "secondary"'
        message: "DR cluster must be in secondary mode"
```
