# Kubernetes Provider

The Kubernetes Provider extends the platform-health server to enable monitoring the health and status of Kubernetes resources. It validates resources using flexible CEL (Common Expression Language) expressions that have full access to the resource's structure.

## Usage

Once the Kubernetes Provider is configured, any query to the platform health server will trigger validation of the configured Kubernetes resource(s). The server will attempt to query the Kubernetes API for each resource, and it will report each resource as "healthy" if the query is successful and all CEL checks pass, or "unhealthy" if the request fails, times out, or any check fails.

## Configuration

The Kubernetes Provider is configured through the platform-health server's configuration file. Each instance is defined with its name as the YAML key.

- `type` (required): Must be `kubernetes`.
- `resource`: The Kubernetes resource to check:
  - `group` (required): The API group of the Kubernetes resource.
  - `version` (optional): The API version of the Kubernetes resource. If not specified, the API server's preferred version is used automatically.
  - `kind` (required): The kind of the Kubernetes resource.
  - `namespace`: The namespace of the Kubernetes resource.
  - `name` (optional): The name of a specific Kubernetes resource. Mutually exclusive with `labelSelector`.
  - `labelSelector` (optional): Select resources by label selector using Kubernetes native syntax (e.g., `app=nginx,env=prod`). Supports equality (`=`, `==`, `!=`) and set-based (`in`, `notin`) operators. When multiple resources match, each is checked and results are aggregated. Mutually exclusive with `name`. If neither `name` nor `labelSelector` is specified, all resources of the kind in the namespace are selected.
- `checks`: A list of CEL expressions to validate the resource. Each check has:
  - `expression` (required): A CEL expression that must evaluate to `true` for the resource to be healthy.
  - `errorMessage` (optional): Custom error message when the check fails.
- `kstatus` (default: `true`): Whether to evaluate resource health using the [kstatus](https://github.com/kubernetes-sigs/cli-utils/tree/master/pkg/kstatus) library. When enabled, resources must reach "Current" status to be considered healthy. A `Detail_KStatus` is included in the response with status, message, and conditions.
- `timeout` (default: `10s`): Timeout for the Kubernetes API request.

Many common resource kinds (see [common/generator.go](common/generator.go)) are internally mapped to the correct `group` if that option is left at default.

For queries to succeed, the platform-health server must be run in a context with appropriate access privileges to list and get the monitored resources. Running "in-cluster", this means an appropriate service account, role and role binding must be configured.

## CEL Expression Context

The CEL context differs based on the selection mode:

**Single-resource mode** (when `name` is specified):

- `resource`: The Kubernetes resource as a map, with access to `resource.metadata`, `resource.spec`, and `resource.status`.

**Selector mode** (when `name` is not specified):

- `items`: Array of all matched Kubernetes resources. Each item is a map with the same structure as `resource` above.

### Example CEL Expressions (Single-Resource Mode)

```cel
// Check deployment has all replicas ready
resource.status.readyReplicas >= resource.spec.replicas

// Check for Available condition
resource.status.conditions.exists(c, c.type == 'Available' && c.status == 'True')

// Check specific label value
resource.metadata.labels['app'] == 'myapp'

// Check minimum replicas
resource.status.readyReplicas >= 3

// Check annotation exists
'prometheus.io/scrape' in resource.metadata.annotations

// Combined checks
resource.status.readyReplicas >= 1 && resource.status.updatedReplicas == resource.spec.replicas
```

### Example CEL Expressions (Selector Mode)

```cel
// Minimum number of resources
items.size() >= 3

// All resources must be running
items.all(r, r.status.phase == 'Running')

// At least N resources must be ready
items.filter(r, r.status.conditions.exists(c, c.type == 'Ready' && c.status == 'True')).size() >= 2

// All deployments fully scaled
items.all(r, r.status.readyReplicas >= r.spec.replicas)

// Check total replica count across all deployments
items.map(r, r.status.readyReplicas).sum() >= 10
```

## Examples

### Basic Deployment Health Check

```yaml
my-app:
  type: kubernetes
  resource:
    kind: Deployment
    name: my-app
    namespace: production
  checks:
    - expression: "resource.status.readyReplicas >= resource.spec.replicas"
      errorMessage: "Not all replicas are ready"
```

### Condition-Based Check

```yaml
my-app:
  type: kubernetes
  kind: Deployment
  resource: my-app
  checks:
    - expression: "resource.status.conditions.exists(c, c.type == 'Available' && c.status == 'True')"
      errorMessage: "Deployment is not available"
```

### Multiple Validation Checks

```yaml
my-app:
  type: kubernetes
  kind: Deployment
  resource: my-app
  checks:
    - expression: "resource.status.readyReplicas >= 1"
      errorMessage: "No replicas ready"
    - expression: "resource.status.updatedReplicas == resource.spec.replicas"
      errorMessage: "Rolling update in progress"
    - expression: "resource.metadata.labels['version'] == 'v2'"
      errorMessage: "Expected version v2"
```

### StatefulSet Check

```yaml
my-database:
  type: kubernetes
  kind: StatefulSet
  resource: my-database
  checks:
    - expression: "resource.status.readyReplicas == resource.spec.replicas"
      errorMessage: "StatefulSet not fully ready"
```

### Service Existence Check

For resources without status conditions, simply omit the `checks` field to verify existence only:

```yaml
my-service:
  type: kubernetes
  kind: Service
  resource: my-service
  namespace: default
```

### Pod Phase Check

```yaml
my-pod:
  type: kubernetes
  kind: Pod
  resource: my-pod
  checks:
    - expression: "resource.status.phase == 'Running'"
      errorMessage: "Pod is not running"
```

### ConfigMap Content Validation

```yaml
app-config:
  type: kubernetes
  kind: ConfigMap
  resource: app-config
  checks:
    - expression: "'database_url' in resource.data"
      errorMessage: "Missing database_url in ConfigMap"
```

### Disable kstatus Evaluation

For resources where kstatus evaluation is not appropriate (e.g., custom resources without standard status conditions), disable it:

```yaml
my-custom-resource:
  type: kubernetes
  resource:
    group: example.com
    version: v1
    kind: MyCustomResource
    name: my-custom-resource
  kstatus: false
  checks:
    - expression: "resource.status.ready == true"
      errorMessage: "Custom resource is not ready"
```

### All Resources in Scope

Select all resources of a kind in a namespace by omitting both `name` and `labelSelector`. Each resource is checked individually and results are aggregated (worst status wins):

```yaml
all-system-deployments:
  type: kubernetes
  resource:
    kind: Deployment
    namespace: kube-system
```

### Label Selector - Multiple Resources

Select resources matching a label selector. CEL checks use `items` for cardinality and collection-based validation:

```yaml
vault-pods:
  type: kubernetes
  resource:
    kind: Pod
    namespace: vault
    labelSelector: "app.kubernetes.io/name=vault"
  checks:
    - expression: "items.size() >= 3"
      errorMessage: "Less than 3 vault pods found"
    - expression: "items.all(r, r.status.phase == 'Running')"
      errorMessage: "Not all pods are running"
```

### Label Selector with Multiple Conditions

Use comma-separated conditions for AND logic, or set-based operators for OR logic on values:

```yaml
app-deployments:
  type: kubernetes
  resource:
    kind: Deployment
    namespace: production
    labelSelector: "app.kubernetes.io/part-of=myapp,tier in (frontend,backend)"
  checks:
    - expression: "items.all(r, r.status.readyReplicas >= r.spec.replicas)"
      errorMessage: "Not all deployments are fully ready"
```

### Component Selection with Label Selector

When using `--component` (`-c`) flag, you can select individual discovered resources by name:

```bash
# Check all matched resources
phc -c vault-pods

# Check specific discovered resource
phc -c vault-pods/vault-0

# Check multiple specific resources
phc -c vault-pods/vault-0 -c vault-pods/vault-1
```

## Response Details

When `kstatus: true` (the default), the health check response includes a `Detail_KStatus` containing:

- `status`: The kstatus result (e.g., "Current", "InProgress", "Failed")
- `message`: Human-readable status message
- `conditions`: Array of Kubernetes conditions from the resource (only included when status is not "Current", for debugging):
  - `type`: Condition type (e.g., "Available", "Progressing")
  - `status`: Condition status ("True", "False", "Unknown")
  - `reason`: Machine-readable reason for the condition
  - `message`: Human-readable message about the condition
