# .github — CI Workflows

## Workflows

| Workflow | Trigger | Purpose |
|----------|---------|---------|
| `ci.yml` | push/PR | Lint (golangci-lint), unit tests, helm lint, helm tests |
| `release-plugin.yml` | `workflow_dispatch` | Orchestrator: stage → promote → release (aws-plugin image) |
| `release-chart.yml` | `workflow_dispatch` | Orchestrator: stage → promote → release (either chart) |
| `release-stage-plugin-image.yml` | `workflow_call` / `workflow_dispatch` | Build multi-arch image, push to staging GHCR |
| `release-promote-plugin-image.yml` | `workflow_call` / `workflow_dispatch` | Promote image: crane copy staging → production |
| `release-stage-chart.yml` | `workflow_call` / `workflow_dispatch` | Package chart, push OCI to staging GHCR |
| `release-promote-chart.yml` | `workflow_call` / `workflow_dispatch` | Promote chart: helm pull staging, push production |

## Artifacts (independently versioned)

| Artifact | GHCR path | Git tag format |
|----------|-----------|----------------|
| aws-plugin image | `ghcr.io/jupyter-infra/jupyter-k8s-aws-plugin` | `plugin/vX.Y.Z` |
| aws-hyperpod chart | `oci://ghcr.io/jupyter-infra/charts/jupyter-k8s-aws-hyperpod` | `aws-hyperpod/vX.Y.Z` |
| aws-oidc chart | `oci://ghcr.io/jupyter-infra/charts/jupyter-k8s-aws-oidc` | `aws-oidc/vX.Y.Z` |

## Release Flow

No automated E2E gate — requires real EKS clusters. The `dry_run` flag stages without
promoting, giving a manual verification checkpoint.

### Plugin image
```
workflow_dispatch (version + dry_run)
  → validate (semver, tag uniqueness, branch check)
  → lint + test (golangci-lint, make test)              [parallel]
  → stage-image (multi-arch build → staging GHCR)
  → [manual verification on EKS]
  → promote-image (crane copy → production GHCR)        [skipped if dry_run]
  → release (git tag plugin/vX.Y.Z + GitHub Release)    [skipped if dry_run]
```

### Chart (parametrized: aws-hyperpod or aws-oidc)
```
workflow_dispatch (chart + version + dry_run)
  → validate (semver, tag uniqueness, branch check)
  → helm-lint + helm-test (make helm-lint, make helm-test-<chart>)  [parallel]
  → stage-chart (dependency build → package → staging GHCR)
  → [manual verification on EKS]
  → promote-chart (helm pull/push → production GHCR)    [skipped if dry_run]
  → release (git tag <chart>/vX.Y.Z + GitHub Release)   [skipped if dry_run]
```

## Testing Workflow Changes

`workflow_dispatch` only fires on the default branch. To iterate from a feature branch,
create a temporary push-triggered workflow:

```yaml
# .github/workflows/test-<name>.yml  — DO NOT merge to main
name: Test workflow (temporary)
on:
  push:
    branches: [your-branch]
permissions:
  contents: read
  packages: write
jobs:
  test:
    uses: ./.github/workflows/release-stage-plugin-image.yml
    with:
      version: v0.1.0-rc.1
      short_sha: ""
    permissions:
      contents: read
      packages: write
```

- Use pre-release versions (e.g. `v0.1.0-rc.1`) to avoid colliding with real releases.
- Each sub-workflow supports both `workflow_call` and `workflow_dispatch`, so after merging
  you can trigger each step individually from the Actions UI.
- Remove test workflows before merging to main.

## Registries

| Namespace | Visibility | Purpose |
|-----------|-----------|---------|
| `ghcr.io/jupyter-infra/staging/` | Private | Pre-release validation |
| `ghcr.io/jupyter-infra/` | Public | Production artifacts |
| `ghcr.io/jupyter-infra/staging/charts/` | Private | Chart staging |
| `ghcr.io/jupyter-infra/charts/` | Public | Chart production |
