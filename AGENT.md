# jupyter-k8s-aws Developer Guide

> **Note:** `CLAUDE.md` is a symlink to this file (`AGENT.md`).
> Always edit `AGENT.md` directly — never update `CLAUDE.md`.

## Project Overview

AWS-specific plugin sidecar and Helm charts for
[jupyter-k8s](https://github.com/jupyter-infra/jupyter-k8s) deployments.
Provides the SSM-based remote access sidecar (`aws-plugin`) and two
deployment charts (`aws-hyperpod`, `aws-traefik-dex`).

Module path: `github.com/jupyter-infra/jupyter-k8s-aws`

Depends on [jupyter-k8s-plugin](https://github.com/jupyter-infra/jupyter-k8s-plugin)
for the plugin contract (HTTP server, request/response types).
No dependency on `jupyter-k8s` (the operator).

## Architecture

The `aws-plugin` binary is an HTTP sidecar that implements the
[jupyter-k8s-plugin](https://github.com/jupyter-infra/jupyter-k8s-plugin) contract.
It runs alongside the jupyter-k8s operator in the same pod.

- **SSM remote access**: Registers/deregisters workspace pods as SSM managed instances and
  creates SSM sessions for VS Code remote and web-UI connections.
- **Plugin HTTP server** (`internal/awsplugin/server.go`): Routes requests from the operator's
  plugin client to AWS SDK handlers (SSM client, remote access actions).

### Helm Charts

**`aws-hyperpod`**: 
This chart relies on bearer-token access, and requires the awsplugin to run as sidecar of the controller. It deploys:
- Traefik as router (TLS termination in an AWS ALB with AWS ACM)
- Authmiddleware for Workspace access control
- Rotator and Kubernetes secret for Authmiddleware JWT seed
Configures a workspace AccessStrategy for both remote access and WebUI, and related templates.

**`aws-traefik-dex`**:
This chart relies on GitHub OIDC to control Workspace access, and DOES NOT need the awsplugin to run as a sidecar of the controller. It deploys:
- Traefik as router (TLS termination in traefik pod with LetsEncrypt TLS certificates)
- Dex as OIDC identity provider for GitHub OAuth authentication
- OAuth2-proxy for cookies management
- Authmiddleware for Workspace access control
- Rotator and Kubernetes secret for Authmiddleware JWT seed
It does not configure a default workspace AccessStrategy.

### Deployment Dependencies

The controller chart and its images (operator, auth-middleware, rotator) live in
[jupyter-k8s](https://github.com/jupyter-infra/jupyter-k8s). This repo only builds the
aws-plugin image. The `deploy-controller` / `deploy-controller-with-plugin` targets
delegate to the sibling jupyter-k8s checkout to deploy the controller.

> **Note:** During the migration period, jupyter-k8s still contains its own copy of the
> aws-plugin source under `images/aws-plugin/`. Its `load-images-aws` target rebuilds and
> pushes that copy, overwriting the image this repo pushes. Until `images/aws-plugin/` is
> removed from jupyter-k8s, the image running in the cluster comes from the jupyter-k8s
> build, not this repo.

## Repository Layout

| Directory                 | Purpose                                                          |
|---------------------------|------------------------------------------------------------------|
| `cmd/aws-plugin/`         | SSM sidecar binary entry point                                   |
| `internal/awsplugin/`     | AWS SDK handlers (SSM client, remote access routes, initializer) |
| `charts/aws-hyperpod/`    | Helm chart: plugin + SSM + web UI for SageMaker HyperPod        |
| `charts/aws-traefik-dex/` | Helm chart: Traefik + Dex OAuth (no Go, Helm only)               |
| `Dockerfile`              | Container image for the aws-plugin sidecar                       |
| `samples/hyperpod/`       | Sample HyperPod workspaces (JupyterLab, Code Editor)             |
| `samples/oidc/`           | Sample OIDC workspaces with access strategies (OAuth, bearer)    |
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

The controller chart lives in [jupyter-k8s](https://github.com/jupyter-infra/jupyter-k8s).
Controller deployment targets delegate to that repo via `CONTROLLER_DIR` (default: `../jupyter-k8s`).

**Controller (from jupyter-k8s):**
- Deploy controller (no plugin): `make deploy-controller`
- Deploy controller with aws-plugin sidecar: `make deploy-controller-with-plugin`

**Guided charts (this repo):**
- Setup EKS connection: `make setup-aws`
- Deploy traefik-dex: `make deploy-aws-traefik-dex`
- Deploy hyperpod: `make deploy-aws-hyperpod`
- Undeploy all: `make undeploy-aws`

The two guided charts cannot be deployed on the same cluster.
- `aws-traefik-dex` uses OIDC (GitHub OAuth) for access — does NOT need the aws-plugin sidecar.
  Use `make deploy-controller` + `make deploy-aws-traefik-dex`.
- `aws-hyperpod` uses bearer-token access — requires the aws-plugin sidecar.
  Use `make deploy-controller-with-plugin` + `make deploy-aws-hyperpod`.

Deployment targets read configuration from `.env` (copy `.env.example` to get started).

### End-to-End Testing

There is no local Kind cluster support — an AWS EKS cluster is required.

**For `aws-traefik-dex` (OIDC):**
1. Configure `.env` with `AWS_REGION`, `EKS_CLUSTER_NAME`, and OIDC variables
2. `make setup-aws`
3. `make deploy-controller`
4. `make deploy-aws-traefik-dex`
5. `make apply-sample-oidc`
6. Verify workspaces: `kubectl get workspaces`
7. Open the access URL in a browser and complete the GitHub OAuth flow
8. Clean up: `make delete-sample-oidc`

**For `aws-hyperpod` (bearer-token + SSM):**
1. Configure `.env` with `AWS_REGION`, `EKS_CLUSTER_NAME`, and HyperPod variables
2. `make setup-aws`
3. `make deploy-controller-with-plugin` (builds, pushes the aws-plugin image, and deploys the controller with it as sidecar)
4. `make deploy-aws-hyperpod`
5. `make apply-sample-hyperpod WS_USER=<username>`
6. Verify workspaces: `kubectl get workspaces`
7. `make bearer-token WS_NAME=<name>` and open the returned URL
8. Clean up: `make delete-sample-hyperpod WS_USER=<username>`

### Samples

Sample workspace manifests for testing against a deployed cluster.

**OIDC workspaces** (requires `aws-traefik-dex` chart deployed):
- Apply: `make apply-sample-oidc` (reads `TRAEFIK_DEX_DOMAIN` or `HYPERPOD_DOMAIN` from `.env`)
- Delete: `make delete-sample-oidc`
- Creates workspaces with OAuth and bearer-token access strategies (public + private variants)

**HyperPod workspaces** (requires `aws-hyperpod` chart deployed):
- Apply: `make apply-sample-hyperpod WS_USER=<username>`
- Delete: `make delete-sample-hyperpod WS_USER=<username>`
- Creates JupyterLab and Code Editor workspaces (public + private variants)

**Connection tokens** (for bearer-auth workspaces):
- Web UI URL: `make bearer-token WS_NAME=<name> [WS_NAMESPACE=default]`
- VS Code remote URL: `make vscode-token WS_NAME=<name> [WS_NAMESPACE=default]`

### Before Submitting a PR
- `make build`
- `make lint`
- `make test`
- `make helm-lint`
- `make helm-test`

## Notes

- Default container runtime is Finch (configurable via `CONTAINER_TOOL` in Makefile)
- Uses golangci-lint v2 for Go linting
- No local Kind cluster support — all deployment/testing is against AWS EKS
- Controller deployment (`deploy-controller*`) requires a sibling jupyter-k8s checkout (see `CONTROLLER_DIR`)
