# Contributing to jupyter-k8s-aws

## Prerequisites

- Go 1.24+
- golangci-lint v2.4+
- Helm 3
- Container tool: Finch or Docker

## Getting Started

Clone this repo and the operator repo as siblings, then install dependencies:

```bash
git clone https://github.com/jupyter-infra/jupyter-k8s-aws.git
git clone https://github.com/jupyter-infra/jupyter-k8s.git
cd jupyter-k8s-aws
make deps
```

The operator repo is needed for controller deployment (`make deploy-controller` /
`make deploy-controller-with-plugin`). The path defaults to `../jupyter-k8s` and
can be overridden with `CONTROLLER_DIR` in `.env` or on the command line.

Copy `.env.example` to `.env` and fill in your AWS cluster configuration.

> **Note:** During the migration period, jupyter-k8s still contains its own copy of the
> aws-plugin source under `images/aws-plugin/`. Its `load-images-aws` target rebuilds and
> pushes that copy, overwriting the image this repo pushes. Until `images/aws-plugin/` is
> removed from jupyter-k8s, the image running in the cluster comes from the jupyter-k8s
> build, not this repo.

## Development Workflow

| Command                | Description                                      |
|------------------------|--------------------------------------------------|
| `make build`           | Build all Go binaries                            |
| `make test`            | Run unit tests with coverage                     |
| `make test-functional` | Run functional tests (build-tagged)              |
| `make lint`            | Run golangci-lint                                |
| `make lint-fix`        | Run golangci-lint with auto-fix                  |
| `make helm-lint`       | Lint all Helm charts                             |
| `make helm-test`       | Run Helm unit tests (Ginkgo)                     |
| `make image-build`     | Build aws-plugin container image locally         |
| `make deps`            | Download and tidy Go dependencies                |

## Testing against AWS

`deploy-aws-oidc` uses `--reuse-values` to preserve the Helm release state that
[jupyter-deploy](https://github.com/jupyter-infra/jupyter-deploy) terraform initially
configured (domain, OAuth, storage, etc.). Contributors only need cluster connection
info in `.env`.

You will need:

- [jupyter-deploy](https://github.com/jupyter-infra/jupyter-deploy) CLI installed
- A deployed `tf-aws-eks-oidc` project (creates the EKS cluster + full chart config)

```bash
cp .env.example .env
# Edit .env with your cluster's AWS_REGION and EKS_CLUSTER_NAME

# Option A: via make
make setup-aws

# Option B: via jd
jd cluster login   # from your jd project directory
```

### Create test workspaces

```bash
kubectl apply -k samples/oidc   # create sample workspaces
kubectl delete -k samples/oidc  # remove them
```

### Iterate on charts

```bash
make deploy-aws-oidc                                          # upgrade with existing values
make deploy-aws-oidc HELM_EXTRA_ARGS="--set webApp.imageTag=dev"  # override specific values
```

### Iterate on the controller

```bash
make deploy-controller                  # deploy without aws-plugin sidecar
make deploy-controller-with-plugin      # deploy with aws-plugin sidecar
```

### Teardown

Run `jd down` in your jupyter-deploy project to destroy the cluster and all
resources. The next `jd up` re-creates everything from scratch.

### CI E2E tests

Chart changes pushed to `main` (under `charts/`) automatically trigger an E2E
workflow that upgrades the chart on a persistent EKS cluster in a dedicated CI
AWS account. The cluster is provisioned and maintained by project maintainers
using [`jupyter-deploy-tf-aws-eks-oidc`](https://github.com/jupyter-infra/jupyter-deploy).
The same check gates chart releases — a chart cannot be staged to GHCR unless
e2e passes. A weekly canary run detects cluster drift independently of code
changes. See [`.github/workflows/e2e.yml`](.github/workflows/e2e.yml).

## Before Submitting a PR

```bash
make build
make lint
make test
make helm-lint
make helm-test
```

All must pass. CI runs the same checks on every pull request.

## License

By contributing, you agree that your contributions will be licensed under the MIT License.
