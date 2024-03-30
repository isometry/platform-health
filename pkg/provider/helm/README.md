# Helm Provider

The Helm Provider extends the platform-health server to enable monitoring the status of Helm releases. It does this by interacting with the Helm API to check the status of the specified Helm release.

## Usage

Once the Helm Provider is configured, any query to the platform health server will trigger validation of the configured Helm release(s). The server will attempt to check the status of each Helm release, and it will report each release as "healthy" if the Helm release exists and is in `deployed` state, or "unhealthy" otherwise.

## Configuration

The Helm Provider is configured through the platform-health server's configuration file, with component instances listed under the `helm` key.

* `name` (required): The name of the Helm release, used to identify the release in the health reports.
* `chart` (required): The chart of the Helm release to monitor.
* `namespace` (required): The namespace of the Helm release to monitor.
* `timeout` (default: `5s`): The maximum time to wait for a status check to be completed before timing out.

For queries to succeed, the platform-health server must be run in a context with appropriate access privileges to list and get the `Secret` resources that Helm uses internally to track releases. Running "in-cluster", this means an appropriate service account, role and role binding must be configured.

### Example

```yaml
helm:
  - name: example
    chart: example-chart
    namespace: example-namespace
    timeout: 5s
```

In this example, the Helm Provider will check the status of the Helm release named "example" in the "example-namespace" namespace, using the "example-chart" chart, and it will wait for 5s before timing out. It will not include detailed information about the Helm release in the health reports.
