# jupyter-k8s-aws Developer Guide

## Project Overview

AWS-specific plugin sidecar and Helm charts for
[jupyter-k8s](https://github.com/jupyter-infra/jupyter-k8s) deployments.
Provides the SSM-based remote access sidecar (`aws-plugin`) and two
deployment charts (`aws-hyperpod`, `aws-traefik-dex`).

Module path: `github.com/jupyter-infra/jupyter-k8s-aws`

Depends on [jupyter-k8s-plugin](https://github.com/jupyter-infra/jupyter-k8s-plugin)
for the plugin contract (HTTP server, request/response types).
No dependency on `jupyter-k8s` (the operator).

## Repository Layout

| Directory                 | Purpose                                                          |
|---------------------------|------------------------------------------------------------------|
| `cmd/aws-plugin/`         | SSM sidecar binary entry point                                   |
| `internal/awsplugin/`     | AWS SDK handlers (SSM client, remote access routes, initializer) |
| `charts/aws-hyperpod/`    | Helm chart: plugin + SSM + web UI for SageMaker HyperPod        |
| `charts/aws-traefik-dex/` | Helm chart: Traefik + Dex OAuth (no Go, Helm only)               |
| `images/aws-plugin/`      | Dockerfile for SSM sidecar                                       |
| `images/jupyter-uv/`      | Reference app image for E2E tests (not published)                |
| `test/helm/`              | Helm chart unit tests (Ginkgo)                                   |
| `scripts/`                | Deployment helper scripts                                        |

## Development

### Prerequisites
- Go 1.24+
- golangci-lint v2.4+
- Helm 3
- Container tool: Finch or Docker

### Common Tasks
- Build sidecar: `make build`
- Lint Go: `make lint`
- Lint with auto-fix: `make lint-fix`
- Unit tests: `make test`
- Functional tests: `make test-functional`
- Helm lint: `make helm-lint`
- Helm tests: `make helm-test`
- Build container image: `make image-build`
- Download/tidy deps: `make deps`

### AWS Deployment
- Setup EKS connection: `make setup-aws`
- Deploy both charts: `make deploy-aws`
- Deploy traefik-dex only: `make deploy-aws-traefik-dex`
- Deploy hyperpod only: `make deploy-aws-hyperpod`
- Undeploy all: `make undeploy-aws`

Deployment targets read configuration from `.env` (copy `.env.example` to get started).

### Before Submitting a PR
- `make build`
- `make lint`
- `make test`
- `make helm-lint`
- `make helm-test`
