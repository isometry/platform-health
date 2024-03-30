# Kubernetes Provider

The Kubernetes Provider extends the platform-health server to enable monitoring the health and status of Kubernetes resources. It does this by checking the existence and readiness of the specified Kubernetes resources, and reporting on the success or failure of these operations.

## Usage

Once the Kubernetes Provider is configured, any query to the platform health server will trigger validation of the configured Kubernetes resource(s). The server will attempt to query the Kubernetes API for each resource, and it will report each resource as "healthy" if the query is successful and the condition matches expectations, or "unhealthy" if the request fails or times out, or if the resource does not exist or the status condition does not match.

## Configuration

The Kubernetes Provider is configured through the platform-health server's configuration file, with list of instances under the `kubernetes` key.

* `group` (default: `apps`): The group of the Kubernetes resource.
* `version` (default: `v1`): The version of the Kubernetes resource.
* `kind` (default: `deployment`): The kind of the Kubernetes resource.
* `name` (required): The name of the Kubernetes resource.
* `namespace` (default: `default`): The namespace of the Kubernetes resource.
* `condition` (default: `null`): A condition to check on the Kubernetes resource. This is an object with two properties:
  * `type` (default: `Available`): The type of the condition.
  * `status` (default: `"True"`): The status of the condition.

Please note that the `condition` option is only applicable to Kubernetes resources that have conditions, such as `deployment`, `pod`, etc. For other resources, such as `service`, `secret`, etc., the `condition` option should not be specified, and the Kubernetes Provider will only check the existence of the resource.

Many common resource kinds (see [common/resources.go](common/resources.go)) are internally mapped to the correct `group` and `version` if those options are left at default.

For queries to succeed, the platform-health server must be run in a context with appropriate access privileges to list and get the monitored resources. Running "in-cluster", this means an appropriate service account, role and role binding must be configured.

### Example

```yaml
kubernetes:
  - group: apps # default
    version: v1 # default
    kind: deployment
    name: example-deployment
    namespace: default # default
    condition:
      type: Available
      status: "True"
```

In this example, the Kubernetes Provider will check the existence and readiness of a Deployment named `example-deployment` in the `default` namespace. It will report the service as "unhealthy" if the `Available` condition of the Deployment is not `True`.
