# aws-oidc Chart Updates Plan

Addresses: #20, #26, #27. Branch: `feat/oidc-access-strategy`.

---

## 1. Ship WorkspaceAccessStrategy (#20)

Two access strategy templates under `templates/access-strategy/`:

**`oauth-access-strategy.yaml`** — OAuth flow (Dex/oauth2-proxy/authmiddleware).
**`bearer-access-strategy.yaml`** — Bearer token flow.

Both adapted from the former `samples/oidc/` strategy files:
- `$DOMAIN` replaced with `.Values.domain`
- Workspace template vars use backtick escaping (same pattern as hyperpod chart)
- `metadata.namespace` set to `.Values.accessStrategy.namespace`

**`namespace.yaml`** — Creates the target namespace, gated on `createNamespace`.

**Values:**
```yaml
accessStrategy:
  createOAuth: true
  createBearer: false       # opt-in: requires authmiddleware.enableBearerAuth
  createNamespace: false    # set true if namespace doesn't already exist
  namespace: jupyter-k8s-system
```

Default namespace is `jupyter-k8s-system` (the operator's shared namespace, which
the webhook allows for cross-namespace access strategy references).

**Validation** in `validations.tpl`:
- `accessStrategy.createBearer` requires `authmiddleware.enableBearerAuth`

---

## 2. Make githubRbac namespace configurable (#26)

Replaced hardcoded `namespace: default` with `{{ .Values.githubRbac.namespace }}` in:
- `github-rbac/group-role.yaml`
- `github-rbac/group-rolebinding.yaml`
- `github-rbac/user-role.yaml`
- `github-rbac/user-rolebinding.yaml`

**New value:**
```yaml
githubRbac:
  namespace: default
```

ClusterRole/ClusterRoleBinding are cluster-scoped — no change needed.

---

## 3. Default githubRbac.orgs from github.orgs (#27)

Templates that iterate over orgs now resolve:
```
{{- $orgs := .Values.githubRbac.orgs | default .Values.github.orgs }}
```

Affected:
- `github-rbac/group-rolebinding.yaml`
- `github-rbac/group-clusterrolebinding.yaml`

---

## 4. Helm tests

New test files in `test/helm/aws-oidc/`:
- `access_strategy_test.go` — strategy rendering, gating, namespace, template
  vars, bearer validation failure
- `github_rbac_test.go` — namespace configurability, orgs defaulting/override

---

## 5. Makefile / samples cleanup

- Deleted `samples/oidc/workspace_access_strategy{,_bearer}.yaml` (chart is
  the single source of truth)
- `apply-sample-oidc` simplified to `kubectl apply -k samples/oidc` (no more
  envsubst pipeline)
- `delete-sample-oidc` simplified to `kubectl delete -k samples/oidc`
- `deploy-aws-oidc` now sets `accessStrategy.createBearer=true`
- `helm-test-aws-oidc` now sets `accessStrategy.createBearer=true`
- Sample workspace manifests updated to reference `namespace: jupyter-k8s-system`
- `kustomization.yaml` and `README.md` updated
