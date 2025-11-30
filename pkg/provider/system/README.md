# System Provider

The System Provider extends the platform-health server to enable hierarchical grouping of related health checks. It does this by defining a named container that holds other provider instances as sub-components, aggregating their health status into a single result.

## Usage

Once the System Provider is configured, any query to the platform health server will trigger validation of all sub-components within the system. The system is reported "healthy" if-and-only-if all of its sub-components are "healthy". The worst status among sub-components propagates up to the system level.

Sub-components appear nested in the system's response, making it easy to identify the relationship in health reports.

## Configuration

The System Provider is configured through the platform-health server's configuration file. Each instance is defined with its name as the YAML key under `components`.

- `kind` (required): Must be `system`.
- `components`: A map of sub-components. Each sub-component is defined with its name as the key and must include a `kind` field specifying its provider type.

## Examples

### FluxCD System Check

```yaml
components:
  fluxcd:
    kind: system
    components:
      source-controller:
        kind: kubernetes
        spec:
          kind: Deployment
          name: source-controller
          namespace: flux-system
      kustomize-controller:
        kind: kubernetes
        spec:
          kind: Deployment
          name: kustomize-controller
          namespace: flux-system
      helm-controller:
        kind: kubernetes
        spec:
          kind: Deployment
          name: helm-controller
          namespace: flux-system
```

In this example, the System Provider creates a `fluxcd` system containing three Kubernetes deployment checks. The `fluxcd` system will be reported "healthy" only if all three controllers are running.

### Nested Systems

Systems can be nested to create deeper hierarchies:

```yaml
components:
  infrastructure:
    kind: system
    components:
      monitoring:
        kind: system
        components:
          prometheus:
            kind: http
            spec:
              url: https://prometheus.example.com/health
          grafana:
            kind: http
            spec:
              url: https://grafana.example.com/api/health
      database:
        kind: tcp
        spec:
          host: postgres.example.com
          port: 5432
```

This creates a hierarchy where `infrastructure` contains a `monitoring` subsystem and a database check. The nested structure is reflected in the response, with each system aggregating its sub-components' status.
