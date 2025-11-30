# Kubernetes Provider

The Kubernetes Provider extends the platform-health server to enable monitoring the health and status of Kubernetes resources. It validates resources using flexible CEL (Common Expression Language) expressions that have full access to the resource's structure.

## Usage

Once the Kubernetes Provider is configured, any query to the platform health server will trigger validation of the configured Kubernetes resource(s). The server will attempt to query the Kubernetes API for each resource, and it will report each resource as "healthy" if the query is successful and all CEL checks pass, or "unhealthy" if the request fails, times out, or any check fails.

## Configuration

The Kubernetes Provider is configured through the platform-health server's configuration file. Each instance is defined with its name as the YAML key under `components`.

- `kind` (required): Must be `kubernetes`.
- `spec`: Provider-specific configuration:
  - `group` (optional): The API group of the Kubernetes resource. Many common kinds auto-map to their correct group.
  - `version` (optional): The API version of the Kubernetes resource. If not specified, the API server's preferred version is used automatically.
  - `kind` (required): The kind of the Kubernetes resource (e.g., "Deployment", "Pod").
  - `namespace` (optional): The namespace of the Kubernetes resource. Use `"*"` to select resources across all namespaces (only valid with `labelSelector`, not `name`).
  - `name` (optional): The name of a specific Kubernetes resource. Mutually exclusive with `labelSelector`.
  - `labelSelector` (optional): Select resources by label selector using Kubernetes native syntax (e.g., `app=nginx,env=prod`). Supports equality (`=`, `==`, `!=`) and set-based (`in`, `notin`) operators. When multiple resources match, each is checked and results are aggregated. Mutually exclusive with `name`. If neither `name` nor `labelSelector` is specified, all resources of the kind in the namespace are selected.
  - `kstatus` (default: `true`): Whether to evaluate resource health using the [kstatus](https://github.com/kubernetes-sigs/cli-utils/tree/master/pkg/kstatus) library. When enabled, resources must reach "Current" status to be considered healthy.
  - `timeout` (default: `10s`): Timeout for the Kubernetes API request.
- `checks`: A list of CEL expressions to validate the resource. Each check has:
  - `check` (required): A CEL expression that must evaluate to `true` for the resource to be healthy.
  - `message` (optional): Custom error message when the check fails.

Many common resource kinds (see [common/generator.go](common/generator.go)) are internally mapped to the correct `group` if that option is left at default.

For queries to succeed, the platform-health server must be run in a context with appropriate access privileges to list and get the monitored resources. Running "in-cluster", this means an appropriate service account, role and role binding must be configured.

## CEL Check Context

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

## Dynamic Resource Lookup with `kubernetes.Get()`

The Kubernetes provider includes a custom CEL function `kubernetes.Get()` that enables dynamic lookup of other Kubernetes resources during check evaluation. This is useful for validating cross-resource dependencies.

### Function Signature

```cel
kubernetes.Get({"kind": "...", ...}) -> map | list | null
```

### Parameters

| Key | Required | Default | Description |
|-----|----------|---------|-------------|
| `kind` | Yes | - | Resource kind (case-insensitive, e.g., `"pod"`, `"Deployment"`) |
| `name` | Conditional | - | Resource name (required if no `labelSelector`) |
| `labelSelector` | Conditional | - | Label selector (required if no `name`) |
| `namespace` | No | `""` | Namespace (empty for cluster-scoped resources) |
| `group` | No | *auto* | API group (auto-resolved for common kinds) |
| `version` | No | *auto* | API version (auto-discovered by REST mapper) |

**Note:** `name` and `labelSelector` are mutually exclusive.

### Return Values

- **Single mode** (`name` provided): Returns the resource as a map, or `null` if not found
- **List mode** (`labelSelector` provided): Returns a list of matching resources (possibly empty)

### Auto-Resolution

- **Group**: Common resource kinds (Deployment, Pod, ConfigMap, etc.) automatically resolve to their correct API group
- **Version**: If not specified, uses the API server's preferred version

### kubernetes.Get() Examples

```cel
// Check if a PodDisruptionBudget exists for this deployment
kubernetes.Get({"kind": "poddisruptionbudget", "namespace": resource.metadata.namespace, "name": resource.metadata.name}) != null

// Check if a ConfigMap has a required key
kubernetes.Get({"kind": "configmap", "namespace": "production", "name": "app-config"}).data["DATABASE_URL"] != ""

// Cluster-scoped resource lookup (omit namespace)
kubernetes.Get({"kind": "namespace", "name": "production"}) != null

// List mode: verify all related pods are running
kubernetes.Get({"kind": "pod", "namespace": resource.metadata.namespace, "labelSelector": "app=nginx"}).all(p, p.status.phase == "Running")

// Dynamic namespace from current resource
kubernetes.Get({"kind": "secret", "namespace": resource.metadata.namespace, "name": "db-creds"}) != null

// Explicit group for CRDs
kubernetes.Get({"group": "cert-manager.io", "kind": "Certificate", "namespace": "prod", "name": "my-cert"}) != null
```

## Examples

### Basic Deployment Health Check

```yaml
components:
  my-app:
    kind: kubernetes
    spec:
      kind: Deployment
      name: my-app
      namespace: production
    checks:
      - check: "resource.status.readyReplicas >= resource.spec.replicas"
        message: "Not all replicas are ready"
```

### Condition-Based Check

```yaml
components:
  my-app:
    kind: kubernetes
    spec:
      kind: Deployment
      name: my-app
    checks:
      - check: "resource.status.conditions.exists(c, c.type == 'Available' && c.status == 'True')"
        message: "Deployment is not available"
```

### Multiple Validation Checks

```yaml
components:
  my-app:
    kind: kubernetes
    spec:
      kind: Deployment
      name: my-app
    checks:
      - check: "resource.status.readyReplicas >= 1"
        message: "No replicas ready"
      - check: "resource.status.updatedReplicas == resource.spec.replicas"
        message: "Rolling update in progress"
      - check: "resource.metadata.labels['version'] == 'v2'"
        message: "Expected version v2"
```

### StatefulSet Check

```yaml
components:
  my-database:
    kind: kubernetes
    spec:
      kind: StatefulSet
      name: my-database
    checks:
      - check: "resource.status.readyReplicas == resource.spec.replicas"
        message: "StatefulSet not fully ready"
```

### Service Existence Check

For resources without status conditions, simply omit the `checks` field to verify existence only:

```yaml
components:
  my-service:
    kind: kubernetes
    spec:
      kind: Service
      name: my-service
      namespace: default
```

### Pod Phase Check

```yaml
components:
  my-pod:
    kind: kubernetes
    spec:
      kind: Pod
      name: my-pod
    checks:
      - check: "resource.status.phase == 'Running'"
        message: "Pod is not running"
```

### ConfigMap Content Validation

```yaml
components:
  app-config:
    kind: kubernetes
    spec:
      kind: ConfigMap
      name: app-config
    checks:
      - check: "'database_url' in resource.data"
        message: "Missing database_url in ConfigMap"
```

### Cross-Resource Validation with kubernetes.Get()

Validate that related resources exist or have specific properties:

```yaml
components:
  my-deployment:
    kind: kubernetes
    spec:
      kind: Deployment
      name: my-app
      namespace: production
    checks:
      # Verify PDB exists for this deployment
      - check: 'kubernetes.Get({"kind": "poddisruptionbudget", "namespace": resource.metadata.namespace, "name": resource.metadata.name}) != null'
        message: "Missing PodDisruptionBudget for deployment"

      # Verify ConfigMap has required database URL
      - check: 'kubernetes.Get({"kind": "configmap", "namespace": "production", "name": "app-config"}).data["DATABASE_URL"] != ""'
        message: "ConfigMap missing DATABASE_URL"
```

### Disable kstatus Evaluation

For resources where kstatus evaluation is not appropriate (e.g., custom resources without standard status conditions), disable it:

```yaml
components:
  my-custom-resource:
    kind: kubernetes
    spec:
      group: example.com
      version: v1
      kind: MyCustomResource
      name: my-custom-resource
      kstatus: false
    checks:
      - check: "resource.status.ready == true"
        message: "Custom resource is not ready"
```

### All Resources in Scope

Select all resources of a kind in a namespace by omitting both `name` and `labelSelector`. Each resource is checked individually and results are aggregated (worst status wins):

```yaml
components:
  all-system-deployments:
    kind: kubernetes
    spec:
      kind: Deployment
      namespace: kube-system
```

**Note:** Empty results (no matching resources) are considered HEALTHY by default. Use CEL checks to require resources:

```yaml
components:
  require-deployments:
    kind: kubernetes
    spec:
      kind: Deployment
      namespace: production
      labelSelector: "app=myapp"
    checks:
      - check: "items.size() >= 1"
        message: "No deployments found"
```

### All Resources Across All Namespaces

Use `namespace: "*"` to select resources across all namespaces:

```yaml
components:
  cluster-wide-pods:
    kind: kubernetes
    spec:
      kind: Pod
      namespace: "*"
      labelSelector: "app.kubernetes.io/part-of=myapp"
    checks:
      - check: "items.size() >= 1"
        message: "No pods found cluster-wide"
```

**Note:** All-namespaces mode requires `labelSelector`; it cannot be used with `name`.

### Label Selector - Multiple Resources

Select resources matching a label selector. CEL checks use `items` for cardinality and collection-based validation:

```yaml
components:
  vault-pods:
    kind: kubernetes
    spec:
      kind: Pod
      namespace: vault
      labelSelector: "app.kubernetes.io/name=vault"
    checks:
      - check: "items.size() >= 3"
        message: "Less than 3 vault pods found"
      - check: "items.all(r, r.status.phase == 'Running')"
        message: "Not all pods are running"
```

### Label Selector with Multiple Conditions

Use comma-separated conditions for AND logic, or set-based operators for OR logic on values:

```yaml
components:
  app-deployments:
    kind: kubernetes
    spec:
      kind: Deployment
      namespace: production
      labelSelector: "app.kubernetes.io/part-of=myapp,tier in (frontend,backend)"
    checks:
      - check: "items.all(r, r.status.readyReplicas >= r.spec.replicas)"
        message: "Not all deployments are fully ready"
```

### Component Selection with Label Selector

When using `--component` (`-c`) flag, you can select individual discovered resources by name:

```bash
# Check all matched resources
ph check -c vault-pods

# Check specific discovered resource
ph check -c vault-pods/vault-0

# Check multiple specific resources
ph check -c vault-pods/vault-0 -c vault-pods/vault-1
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
