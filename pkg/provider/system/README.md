# System Provider

The System Provider extends the platform-health server to enable hierarchical grouping of related health checks. It does this by defining a named container that holds other provider instances as children, aggregating their health status into a single result.

## Usage

Once the System Provider is configured, any query to the platform health server will trigger validation of all child components within the system. The system is reported "healthy" if-and-only-if all of its child components are "healthy". The worst status among children propagates up to the system level.

Child instances appear as nested components in the system's response, making it easy to identify the relationship in health reports.

## Configuration

The System Provider is configured through the platform-health server's configuration file. Each instance is defined with its name as the YAML key under `components`.

- `type` (required): Must be `system`.
- `components`: A map of child instances. Each child is defined with its name as the key and must include a `type` field specifying its provider type.

### Example

```yaml
components:
  fluxcd:
    type: system
    components:
      source-controller:
        type: kubernetes
        kind: deployment
        resource: source-controller
        namespace: flux-system
      kustomize-controller:
        type: kubernetes
        kind: deployment
        resource: kustomize-controller
        namespace: flux-system
      helm-controller:
        type: kubernetes
        kind: deployment
        resource: helm-controller
        namespace: flux-system
```

In this example, the System Provider creates a `fluxcd` system containing three Kubernetes deployment checks. The `fluxcd` system will be reported "healthy" only if all three controllers are running.

### Nested Systems

Systems can be nested to create deeper hierarchies:

```yaml
components:
  infrastructure:
    type: system
    components:
      monitoring:
        type: system
        components:
          prometheus:
            type: http
            url: https://prometheus.example.com/health
          grafana:
            type: http
            url: https://grafana.example.com/api/health
      database:
        type: tcp
        host: postgres.example.com
        port: 5432
```

This creates a hierarchy where `infrastructure` contains a `monitoring` subsystem and a database check. The nested structure is reflected in the response, with each system aggregating its children's status.
