# Helm Provider

The Helm Provider extends the platform-health server to enable monitoring the status of Helm releases. It does this by interacting with the Helm API to check the status of the specified Helm release.

## Usage

Once the Helm Provider is configured, any query to the platform health server will trigger validation of the configured Helm release(s). The server will attempt to check the status of each Helm release, and it will report each release as "healthy" if the Helm release exists and is in `deployed` state, or "unhealthy" otherwise.

## Configuration

The Helm Provider is configured through the platform-health server's configuration file. Each instance is defined with its name as the YAML key under `components`.

- `type` (required): Must be `helm`.
- `release` (required): The name of the Helm release to monitor.
- `namespace` (required): The namespace of the Helm release to monitor.
- `timeout` (default: `5s`): The maximum time to wait for a status check to be completed before timing out.
- `checks` (optional): List of CEL expressions for custom health validation. Each check has:
  - `expr` (required): A CEL expression that must evaluate to `true` for the release to be healthy.
  - `message` (optional): Custom error message when the check fails.

For queries to succeed, the platform-health server must be run in a context with appropriate access privileges to list and get the `Secret` resources that Helm uses internally to track releases. Running "in-cluster", this means an appropriate service account, role and role binding must be configured.

### Example

```yaml
components:
  example:
    type: helm
    release: example-chart
    namespace: example-namespace
    timeout: 5s
```

In this example, the Helm Provider will check the status of the Helm release named "example-chart" in the "example-namespace" namespace, and it will wait for 5s before timing out.

## CEL Expressions

The `checks` field accepts a list of CEL (Common Expression Language) expressions for custom health validation. Each expression must evaluate to `true` for the release to be considered healthy.

### Available Context

The following variables are available in CEL expressions:

#### Release Properties
- `release.Name` - Release name
- `release.Namespace` - Release namespace
- `release.Revision` - Release revision number (int)
- `release.Status` - Release status (string: "deployed", "failed", etc.)
- `release.FirstDeployed` - First deployment timestamp
- `release.LastDeployed` - Last deployment timestamp
- `release.Deleted` - Deletion timestamp
- `release.Description` - Release description
- `release.Notes` - Chart NOTES.txt content
- `release.Manifest` - Rendered manifest content
- `release.Labels` - Release labels (map)
- `release.Config` - User-provided value overrides (map)

#### Chart Properties
- `chart.Name` - Chart name
- `chart.Version` - Chart version
- `chart.AppVersion` - Application version
- `chart.Description` - Chart description
- `chart.Deprecated` - Whether chart is deprecated (bool)
- `chart.KubeVersion` - Required Kubernetes version
- `chart.Type` - Chart type
- `chart.Annotations` - Chart annotations (map)
- `chart.Values` - Chart default values (map)

### Example with CEL Checks

```yaml
components:
  my-app:
    type: helm
    release: my-app
    namespace: production
    timeout: 10s
    checks:
      - expr: "release.Revision >= 2"
        message: "Release must have at least one upgrade"
      - expr: "!chart.Deprecated"
        message: "Chart is deprecated"
      - expr: "'team' in release.Labels && 'env' in release.Labels"
        message: "Release must have team and env labels"
      - expr: "release.Config['replicas'] >= 3"
        message: "Production must have at least 3 replicas"
```

This example validates that:
- The release has been upgraded at least once
- The chart is not deprecated
- Required labels are present
- The replica count meets production requirements
