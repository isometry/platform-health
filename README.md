# Platform Health

Lightweight & extensible platform health monitoring.

## Overview

Platform Health is a simple client/server system for lightweight health monitoring of platform components and systems.

The Platform Health client (`phc`) sends a gRPC health check request to a Platform Health server which is configured to probe a set of network services. Probes run asynchronously on the server (subject to configurable timeouts), with the accumulated response returned to the client.

## Providers

Probes use a compile-time [provider plugin system](pkg/provider) that supports extension to monitoring of arbitrary services. Integrated providers include:

* [`system`](pkg/provider/system): Hierarchical grouping of related health checks with status aggregation
* [`satellite`](pkg/provider/satellite): A separate satellite instance of the Platform Health server
* [`tcp`](pkg/provider/tcp): TCP connectivity checks
* [`tls`](pkg/provider/tls): TLS handshake and certificate verification
* [`http`](pkg/provider/http): HTTP(S) queries with status code and certificate verification
* [`rest`](pkg/provider/rest): REST API health checks with CEL-based response validation
* [`grpc`](pkg/provider/grpc): gRPC Health v1 service status checks
* [`kubernetes`](pkg/provider/kubernetes): Kubernetes resource existence and readiness
* [`helm`](pkg/provider/helm): Helm release existence and deployment status
* [`vault`](pkg/provider/vault): [Vault](https://www.vaultproject.io/) cluster initialization and seal status

Each provider implements the `Instance` interface, with the health of each instance obtained asynchronously, and contributing to the overall response.

## Installation

### macOS/Linux

```bash
brew install isometry/tap/platform-health
```

```console
$ phs -l & sleep 1 && phc && kill %1
{"status":"HEALTHY", "duration":"0.000004833s"}
```

### Kubernetes

#### Install via `helm` chart

```console
helm upgrade \
    --install platform-health \
    -n platform-health --create-namespace \
    oci://ghcr.io/isometry/charts/platform-health
```

#### Install via `kubectl`

```bash
kubectl create configmap platform-health --from-file=platform-health.yaml=/dev/stdin <<-EOF
  ssh@localhost:
    type: tcp
    host: localhost
    port: 22
  gmail:
    type: tls
    host: smtp.gmail.com
    port: 465
  google:
    type: http
    url: https://google.com
EOF

kubectl create deployment platform-health --image ghcr.io/isometry/platform-health:latest --port=8080

kubectl patch deployment platform-health --patch-file=/dev/stdin <<-EOF
  spec:
    template:
      spec:
        volumes:
          - name: config
            configMap:
              name: platform-health
        containers:
          - name: platform-health
            args:
              - -vv
            volumeMounts:
              - name: config
                mountPath: /config
EOF

kubectl create service loadbalancer platform-health --tcp=8080:8080
```

## Usage

### Client

```bash
# Check all components
phc

# Check specific components
phc -c google -c github

# Check with hierarchical path (system/component)
phc -c fluxcd/source-controller

# Connect to remote server
phc prod:8080 -c google
```

### One-Shot Mode

Run health checks once and exit without starting a server:

```bash
phs -o
# or
phs --one-shot

# Check specific components only
phs -o -c google -c fluxcd/source-controller
```

This is useful for:
- Validating configuration files
- Local health check verification
- CI/CD pipeline integration
- Testing specific components

## Configuration

The Platform Health server reads a simple configuration file, defaulting to `platform-health.yaml` with the following structure:

```yaml
<instance-name>:
  type: <provider-type>
  <provider-specific-config>
```

### Example

The following configuration will monitor that /something/ is listening on `tcp/22` of localhost; validate connectivity and TLS handshake to the Gmail SSL mail-submission port; and validate that Google is accessible and returning a 200 status code:

```yaml
ssh@localhost:
  type: tcp
  host: localhost
  port: 22
gmail:
  type: tls
  host: smtp.gmail.com
  port: 465
google:
  type: http
  url: https://google.com
api-health:
  type: rest
  request:
    url: https://api.example.com/health
    method: GET
  checks:
    - expression: 'response.status == 200'
      errorMessage: "Expected HTTP 200"
    - expression: 'response.json.status == "healthy"'
      errorMessage: "Service unhealthy"
```

### Hierarchical Grouping

Use the `system` provider to group related checks:

```yaml
fluxcd:
  type: system
  components:
    source-controller:
      type: kubernetes
      resource:
        kind: deployment
        namespace: flux-system
        name: source-controller
    kustomize-controller:
      type: kubernetes
      resource:
        kind: deployment
        namespace: flux-system
        name: kustomize-controller
```

The system is reported "healthy" only if all child components are healthy.
