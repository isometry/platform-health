# Platform Health

Lightweight & extensible platform health monitoring.

## Overview

Platform Health is a simple client/server system for lightweight health monitoring of platform components and systems.

The Platform Health client (`phc`) sends a gRPC health check request to a Platform Health server which is configured to probe a set of network services. Probes run asynchronously on the server (subject to configurable timeouts), with the accumulated response returned to the client.

## Providers

Probes use a compile-time [provider plugin system](pkg/provider) that supports extension to monitoring of arbitrary services. Integrated providers include:

* [`satellite`](pkg/provider/satellite): A separate satellite instance of the Platform Health server
* [`tcp`](pkg/provider/tcp): TCP connectivity checks
* [`tls`](pkg/provider/tls): TLS handshake and certificate verification
* [`http`](pkg/provider/http): HTTP(S) queries with status code and certificate verification
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

TODO: Helm chart :-)

```bash
kubectl create configmap platform-health --from-file=platform-health.yaml=/dev/stdin <<-EOF
  tcp:
    - name: ssh@localhost
      host: localhost
      port: 22
  tls:
    - name: gmail
      host: smtp.gmail.com
      port: 465
  http:
    - name: google
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

## Configuration

The Platform Health server reads a simple configuration file, defaulting to `platform-health.yaml` with the following structure:

```yaml
<provider>: [<instance>, …]
```

### Example

The following configuration will monitor that /something/ is listening on `tcp/22` of localhost; validate connectivity and TLS handshake to the Gmail SSL mail-submission port; and validate that Google is accessible and returning a 200 status code:

```yaml
tcp:
  - name: ssh@localhost
    host: localhost
    port: 22
tls:
  - name: gmail
    host: smtp.gmail.com
    port: 465
http:
  - name: google
    url: https://google.com
```
