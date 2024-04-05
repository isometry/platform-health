# Jenkins Provider

The Jenkins Provider extends the platform-health server to enable monitoring the health of Jenkins jobs. It does this by
interacting with a Jenkins controller via HTTP requests to the specified URL and reporting on the success or failure of
this operation based upon the retrieved Job's last build health.

## Usage

Once the Jenkins Provider is configured, any query to the platform-health server will trigger validation of the
configured Jenkins job(s). The server will attempt to send an HTTP request to each defined Jenkins job, and it will
report each job as "healthy" if the request is successful and the job health status matches one of the expected values,
or "unhealthy" otherwise.

## Configuration

The Jenkins Provider is configured through the platform-health server's configuration file, with component instances
listed under the `jenkins` key.

* `job` (optional): The name of the Jenkins job, used to identify the service in the health reports.
* `url` (required): The URL of the Jenkins controller service to monitor.
* `timeout` (default: `10s`): The maximum time to wait for a response before timing out.
* `insecure` (default: `false`): If set to true, allows the Jenkins provider to establish connections even if the HTTP
  certificate of the service is invalid or untrusted. This is useful for testing or in environments where services use
  self-signed certificates. Note that using this option in a production environment is not recommended, as it disables
  important security checks.
* `status` (default: `["Succeeded"]`): The list of desired job statuses that are expected in the response.
* `detail` (default: `false`): If set to true, the provider will return detailed information about the Jenkins job.
* `credentials` (optional): The credentials required to authenticate with the Jenkins controller. This is an array of
  credential objects which aims at populating the `username` and `token` fields. Multiple credentials providers can be used.
  If multiple credentials providers are used, the `username` and `token` fields are populated by merging the provider results.

## Credentials provider support

|             Supported             | Provider      | Description                                                                                 |
|:---------------------------------:|---------------|---------------------------------------------------------------------------------------------|
|               @TODO               | `environment` | Reads the credentials from environment variables.                                           |
|               @TODO               | `vault`       | Reads the credentials from HashiCorp Vault. Currently supported engines are `kv` v1 and v2. |

### Example

```yaml
jenkins:
  - job: example
    url: https://jenkins.com
    detail: true
```

### Example with credentials (environment variables)

```yaml
jenkins:
  - job: example
    url: https://jenkins.com
    detail: true
    credentials:
      - provider: environment
        username: JENKINS_USERNAME
        token: JENKINS_TOKEN
```

### Example with credentials (HashiCorp Vault)

```yaml
jenkins:
  - job: example
    url: https://jenkins.com
    detail: true
    # Optional credentials block if Jenkins controller access does not require authentication
    credentials:
      - provider: vault
        address: https://vault.com
        engine: kv
        version: 2
        # Optional credentials block if VAULT_TOKEN is not set
        # credentials:
        #   - provider: environment
        #     token: ALTERNATE_VAULT_TOKEN
        username:
          # address: X (optional override)
          # engine: Y (optional override)
          # version: Z (optional override)
          path: secret/data/jenkins
          field: credentials
          key: username
        token:
          path: secret/data/jenkins
          field: credentials
          key: token
```

In this example, the platform-health server will attempt to fetch the `example` job status request
to `http://jenkins.com`; it will allow the default `10s` before timing out; it will expect the job status to
be `Succeeded`; it will not establish connections if the HTTP certificate of the service is invalid or untrusted; and it
will provide additional detailed information about the HTTP connection.
