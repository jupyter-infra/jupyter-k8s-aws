# jupyter-k8s-aws

AWS-specific plugin sidecar and Helm charts for
[jupyter-k8s](https://github.com/jupyter-infra/jupyter-k8s) deployments.

## Components

| Component | Description |
|-----------|-------------|
| **aws-plugin** | SSM-based remote access sidecar, deployed alongside the jupyter-k8s controller |
| **aws-hyperpod** | Helm chart: plugin + SSM remote access + web UI for SageMaker HyperPod |
| **aws-oidc** | Helm chart: Traefik reverse proxy + Dex OIDC + OAuth2-Proxy (no Go code) |

## Architecture

```
jupyter-k8s ──────────► jupyter-k8s-plugin ◄──────── jupyter-k8s-aws
 (operator)               (shared SDK)            (sidecar + charts)
```

Both `jupyter-k8s` and `jupyter-k8s-aws` import the plugin SDK as a leaf dependency.
There is no direct dependency between the operator and this repository.

## Installation

See [charts/aws-hyperpod/](charts/aws-hyperpod/) or
[charts/aws-oidc/](charts/aws-oidc/) for chart-specific instructions.

## Contributing

See [CONTRIBUTING.md](CONTRIBUTING.md).

## License

[MIT](LICENSE)
