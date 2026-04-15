# Contributing to jupyter-k8s-aws

## Prerequisites

- Go 1.24+
- golangci-lint v2.4+
- Helm 3
- Container tool: Finch or Docker

## Getting Started

Clone the repo and download dependencies:

```bash
git clone https://github.com/jupyter-infra/jupyter-k8s-aws.git
cd jupyter-k8s-aws
make deps
```

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
