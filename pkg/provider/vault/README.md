# Vault Provider

The Vault Provider extends the platform-health server to enable monitoring the health of [HashiCorp Vault](https://www.vaultproject.io/) servers. It does this by querying the [sys/health](https://developer.hashicorp.com/vault/api-docs/system/health) endpoint and validating that it returns `initialized: true` and `sealed: false` for "healthy", and "unhealthy" otherwise.

## Usage

Once the Vault Provider is configured, any query to the platform-health server will trigger validation of the configured Vault service(s). The server will attempt to send an HTTP request to each service, and it will report each service as "healthy" if the request is successful and the server reports as initialized and unsealed, or "unhealthy" otherwise.

## Configuration

The Vault Provider is configured through the platform-health server's configuration file, with component instances listed under the `vault` key.

* `name` (required): The name of the Vault service instance, used to identify the service in the health reports.
* `address` (required): The address of the Vault instance in standard `VAULT_ADDR` format.
* `timeout` (default: `1s`): The maximum time to wait for a response before timing out.
* `insecure` (default: `false`): If set to true, allows the Vault provider to establish connections even if the TLS certificate of the service is invalid or untrusted. This is useful for testing or in environments where services use self-signed certificates. Note that using this option in a production environment is not recommended, as it disables important security checks.

### Example

```yaml
vault:
  - name: example
    address: https://vault.example.com
```

In this example, the platform-health server will validate that the Vault cluster running at `https://vault.example.com` is up, initialized and unsealed.
