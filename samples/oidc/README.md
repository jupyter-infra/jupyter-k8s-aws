### Prerequisites

The `aws-oidc` chart must be deployed first — it ships the
`WorkspaceAccessStrategy` resources that these sample workspaces reference.

### Usage
- set the value of `OIDC_DOMAIN` or `HYPERPOD_DOMAIN` in your `.env` file
- run `make apply-sample-oidc`

### Clean-up
- run `make delete-sample-oidc` or `kubectl delete -k samples/oidc`
