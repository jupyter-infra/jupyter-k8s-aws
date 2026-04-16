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
