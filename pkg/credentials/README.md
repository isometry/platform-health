# Credentials

* [[Schema](#schema)]
    * [Common keywords](#common-keywords)

* [[Supported credentials](#supported-credentials)]
    * `Kubernetes`
        * [Secret](#kubernetes-secret)
        * [ConfigMap](#kubernetes-configmap)
    * `Hashicorp`
        * [Vault](#hashicorp-vault)
    * `AWS`
        * [Systems Manager](#aws-systems-manager)
        * [Secrets Manager](#aws-secrets-manager)

## Schema

| Type         | Description            | Fields                     | Docs                   |
|--------------|------------------------|----------------------------|------------------------|
| `k8s_secret` | Kubernetes `Secret`    | `namespace`, `name`, `key` | [link](#K8S_SECRET.md) |
| `k8s_cm`     | Kubernetes `ConfigMap` | `namespace`, `name`, `key` | [link](#K8S_CM.md)     |
| `vault`      | Hashicorp `Vault`      | `engine`, `mount`, `keys`  | [link](#VAULT.md)      |
| `aws_ssm`    | AWS `Systems Manager`  | `decryption`, `keys`       | [link](#SSM.md)        |
| `aws_sm`     | AWS `Secrets Manager`  | `path`, `key`              | [link](#SM.md)         |

### Common keywords

> [!NOTE]
> All fetched credentials are automatically shared with other services via an internal `secret bus`.
> However, it is sometimes desirable to be able to consume and expose environment variables to downstream services.
> This is by default not active but can be enabled by using `expose` field.

| Keyword  | Description                                                                                                                                        |
|----------|----------------------------------------------------------------------------------------------------------------------------------------------------|
| `path`   | The path to the secret.                                                                                                                            |
| `key`    | If provided, the secret will be parsed and the value matching the `key` will be extracted.                                                         |
| `target` | The variable name to set the value to in the secret bus and environment variable when applicable.                                                  |
| `expose` | Whether to expose the value as an environment variable. (Defaults to `false`)                                                                      |
| `keys`   | A list of `path`, `key`, `target`, `expose` (and other additional fields when applicable) items allowing multiple secrets to be fetched/e at once. |

```yaml
...
credentials: # [list of credential providers]
  - <type>: # identifier (e.g. vault, aws_ssm, aws_sm)
      <foo>: <bar> # dynamic fields appropriate for the credential provider
      <baz>: <qux>
  ...
  - <type>:
      <foo>: <bar>
      <baz>: <qux> 
```

> [!NOTE]  
> Credential types can be daisy-chained together to provide a more complex credential resolution mechanism.
> For example:
> ```yaml
> ...
> credentials:
>   - k8s_secret:
>       namespace: default
>       name: my-secret
>       keys:
>         - key: vault-token
>           target: VAULT_TOKEN
>           expose: true
>         - key: vault-addr
>           target: VAULT_ADDR
>           expose: true
>   - vault:
>       engine: kv2
>       mount: secret
>       keys:
>         - path: /aws/creds
>           key: access-key-id
>           target: AWS_ACCESS_KEY_ID
>           expose: true
>         - path: /aws/creds
>           key: secret-access-key
>           target: AWS_SECRET_ACCESS_KEY
>           expose: true
>   - k8s_cm:
>       namespace: default
>       name: my-configmap
>       key: region
>       target: AWS_REGION
>       expose: true
>   - aws_ssm:
>       decryption: true
>       path: /secret
>       key: ssm-secret
>       target: MY_APP_SECRET
> ```

## Supported credentials

### `Kubernetes Secret`

<details>
    <summary><i>expand me! ✨</i></summary>

```yaml
...
credentials:
  - k8s_secret:
      namespace: default
      name: my-secret
      key: secret
      target: MY_SECRET
      expose: true
```

</details>

### `Kubernetes ConfigMap`

<details>
    <summary><i>expand me! ✨</i></summary>

```yaml
...
credentials:
  - k8s_cm:
      namespace: default
      name: my-secret
      key: secret
      target: MY_SECRET
      expose: true
```

</details>

### `Hashicorp Vault`

<details>
    <summary><i>expand me! ✨</i></summary>

```yaml
...
credentials:
  - vault:
      # address: <value> # Defaults to VAULT_ADDR - @TODO(paulo) WIP
      # token_var: <var> # Defaults to VAULT_TOKEN - @TODO(paulo) WIP
      engine: kv2
      mount: secret
      keys:
        - path: /path
          key: user
          target: MY_USER
          expose: true
        - path: /path
          key: secret
    target: MY_SECRET
```

</details>

### `AWS Systems Manager`

<details> 
    <summary><i>expand me! ✨</i></summary>

```yaml
...
credentials:
  - aws_ssm:
      decryption: true
      keys:
        - path: /path
          key: user
          target: MY_USER
        - path: /path
          key: secret
          target: MY_SECRET
```

</details>

### `AWS Secrets Manager`

<details> 
    <summary><i>expand me! ✨</i></summary>

```yaml
...
credentials:
  - aws_sm:
      path: /path
      key: secret
      target: MY_SECRET
```

</details>
