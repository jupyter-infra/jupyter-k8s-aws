# Testing: aws-oidc Guided Chart

This chart deploys the OSS routing stack: Traefik reverse proxy, Dex OIDC provider with GitHub OAuth,
OAuth2-proxy, authmiddleware, and JWT key rotation. It does NOT use `--plugin-endpoints` — there are
no plugins. It exercises the k8s-native bearer token path (HMAC JWT via Kubernetes Secret).

## Prerequisites

- An EKS cluster (the OSS chart does not require Pod Identity / IRSA)
- `.env` file with `AWS_REGION`, `EKS_CLUSTER_NAME`, domain, GitHub OAuth, and EFS values (see `.env.example`)
- kubectl context pointing at the target cluster
- A domain with DNS pointing to the cluster (for Traefik ingress + Let's Encrypt)

```bash
make setup-aws                        # configure kubectl context from .env
```

**Default cluster:** `jupyter-k8s-cluster` in `us-west-2` (`.env` defaults).
If your cluster has a different name (e.g. `jupyter-deploy-eks-*`), update
`EKS_CLUSTER_NAME` in `.env` before running any deploy targets.

To switch kubectl context without re-running full setup:

```bash
make kubectl-aws                      # switch kubectl context
```

## Clean Slate

> **⚠️  DESTRUCTIVE — This deletes ALL workspaces, uninstalls ALL Helm releases,
> and removes CRDs from the cluster. Only run this on a development/sandbox cluster.
> Never run against a shared or production cluster. There is no undo.**

If there's an existing deployment, tear it down in dependency order:

```bash
kubectl delete workspaces --all --all-namespaces --wait
helm uninstall jupyter-k8s-aws-oidc -n jupyter-k8s-router 2>/dev/null; true
helm uninstall jupyter-k8s -n jupyter-k8s-system 2>/dev/null; true
kubectl delete crd workspaces.workspace.jupyter.org \
  workspaceaccessstrategies.workspace.jupyter.org \
  workspacetemplates.workspace.jupyter.org 2>/dev/null; true
```

Note: The Helm release name depends on how the cluster was deployed:

- `jd apply` (terraform): release name is `jupyter-k8s-aws-oidc`
- `make deploy-aws-oidc`: release name is `jk8-aws-oidc`

Check with `helm list -n jupyter-k8s-router` before uninstalling.

## Build and Validate

```bash
make build                            # compile Go binaries
make helm-lint                        # lint all charts
make helm-test                        # render + run Go tests against rendered output
```

Shorthand for all pre-PR checks:

```bash
make release                          # runs: build lint test helm-lint helm-test
```

## Deploy

The controller (from the sibling `jupyter-k8s` repo) must be deployed first — it installs the CRDs:

```bash
make deploy-controller                # operator chart (no aws-plugin sidecar)
make deploy-aws-oidc                  # OSS guided chart (reads .env for domain, GitHub, EFS)
```

If `make deploy-controller` fails (e.g. missing `jupyter-k8s` checkout), you can install
just the CRDs as a fallback so `deploy-aws-oidc` can proceed:

```bash
kubectl apply -f ../jupyter-k8s/config/crd/bases/
```

### Helm release name conflict

If the cluster was previously deployed via `jd` (terraform), the existing Helm release is
named `jupyter-k8s-aws-oidc`. The Makefile uses `jk8-aws-oidc`. Deploying with the Makefile
will fail with resource ownership errors. In this case, upgrade the existing release directly:

```bash
helm upgrade jupyter-k8s-aws-oidc ./charts/aws-oidc -n jupyter-k8s-router --reuse-values
```

**Checks:**

1. **Controller pod has 1 container** (no sidecar):
   ```bash
   kubectl get pods -n jupyter-k8s-system -l control-plane=controller-manager \
     -o jsonpath='{.items[0].spec.containers[*].name}'
   ```
   Expected: `manager` (no `plugin-aws`)

2. **No `--plugin-endpoints` in manager args**:
   ```bash
   kubectl describe pod -n jupyter-k8s-system -l control-plane=controller-manager | grep plugin-endpoints
   ```
   Expected: empty

3. **Manager logs** show no plugin-related errors:
   ```bash
   kubectl logs -n jupyter-k8s-system -l control-plane=controller-manager -c manager --tail=20
   ```

4. **Full routing stack is running**:

   ```bash
   kubectl get pods -n jupyter-k8s-router
   ```

   Expect: traefik, oauth2-proxy, dex, authmiddleware, and completed rotator jobs.

## Create and Test Workspaces

```bash
make apply-sample-oidc
kubectl get workspaces -n default
kubectl wait --for=condition=Available workspaces --all -n default --timeout=300s
```

### Verify Bearer Token Flow (Web UI)

```bash
make bearer-token WS_NAME=<workspace-name>
```

Should return a URL with a JWT token. Open in browser — the bearer auth middleware creates
a session JWT and sets a cookie, then routes through Traefik to the workspace.

### Verify GitHub OAuth Flow

Access a workspace URL directly in browser (without bearer token):
- Should redirect to Dex -> GitHub login -> workspace
- Only users in the configured GitHub org/team should be granted access

### Verify JWT Key Rotation

Check that the rotator CronJob is running and the secret has multiple keys:
```bash
kubectl get cronjob -n jupyter-k8s-router
kubectl get secret authmiddleware-secrets -n jupyter-k8s-router -o jsonpath='{.data}' | jq 'keys'
```

## Cleanup

```bash
make delete-sample-oidc
```

No errors should appear in controller logs.

## Failure Modes

| Symptom | Likely cause |
|---------|-------------|
| Authmiddleware CrashLoopBackOff | JWT secret not found — check `secretName` matches between auth deployment and rotator |
| Certificate not issued | cert-manager not installed, or DNS not pointing to cluster |
| OAuth redirect fails | GitHub OAuth app callback URL mismatch with domain, or Dex configmap has wrong issuer URL |
| Bearer token returns 403 | `enableBearerAuth` not set to true, or RBAC missing for `bearertokenreviews` |
| Rotator job fails | ServiceAccount missing RBAC to update the JWT secret |
| Resource ownership error on deploy | Helm release name mismatch — see "Helm release name conflict" above |
