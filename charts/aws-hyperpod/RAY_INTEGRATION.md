# Ray Cluster Integration (`ray-integration` WorkspaceIntegrationTemplate)

Design notes and operational reference for `templates/ray-integration-template.yaml`.
The template carries only short pointers back to the sections here.

## Overview

An admin installs this `WorkspaceIntegrationTemplate` once per cluster (chart-managed);
each customer only supplies the RayCluster **name** via their Workspace's
`integrationTemplateRefs[].parameters`. The RayCluster must be in the workspace's own
namespace. The template injects a `ray-sidecar` that joins the RayCluster as a
zero-resource client node, plus the env/volumes the workspace's Ray client needs to attach.

## Studio dependency — name & parameter are a public contract

The metadata `name: ray-integration` and the declared parameter `rayClusterName` are a
**stable API**. The SageMaker Studio Ray extension references this template by the literal
name `ray-integration` and fills `rayClusterName` by name when it wires a workspace's
`integrationTemplateRefs[]`. Renaming the template or the parameter silently breaks the
Studio attach flow (the ref resolves to nothing). Do not rename either without coordinating
a change on the Studio extension side.

## Helm escaping

The operator's own Go-template expressions are wrapped in Helm's backtick idiom
(`` {{`{{ ... }}`}} ``) so Helm renders them through verbatim. Helm evaluates the whole
file first (comments included), so only the chart-level `.Values` / `.Release` references
are Helm's; everything else is the operator's, evaluated at reconcile time.

## Version matching (customer responsibility)

Ray requires the client (workspace) and cluster to match on **Ray version AND Python
version** to the patch level. The sidecar image is resolved from the RayCluster head image,
so the cluster head image must match the workspace image's Ray/Python.
`public.ecr.aws/sagemaker/sagemaker-distribution` ships both JupyterLab and a matching Ray;
the simplest guarantee is to run the RayCluster head on the same Ray/Python as the workspace
image. **Last tested against Ray 2.55.0 / Python 3.11.**

On a mismatch there is **no admission error** — `ray.init()` fails at runtime in the
workspace with a version-mismatch message (e.g. "Version mismatch: The cluster was started
with Ray _x_ / Python _y_, this process is Ray _a_ / Python _b_"). The workspace pod stays
healthy (the statusProbe is report-only); only the Ray attach fails. Fix by aligning the
RayCluster head image with the workspace image.

## GPU workspaces

This integration is a **connection broker** — Ray compute runs on the RayCluster's worker
pods, never on the workspace pod (the sidecar joins `--num-cpus=0 --num-gpus=0`). A GPU on
the workspace pod is therefore for **local notebook use only**; steer Ray GPU work to GPU
RayCluster workers via `@ray.remote(num_gpus=...)`.

- **`RAY_ACCEL_ENV_VAR_OVERRIDE_ON_ZERO=0`** (set in the workspace container's `mergeEnv`):
  the local broker node is `num-gpus=0`, so current Ray blanks `CUDA_VISIBLE_DEVICES` in the
  driver process on `ray.init()` — hiding a physical GPU from local (non-Ray) notebook code
  (`torch.cuda.is_available()` flips to `False`). Setting this to `0` adopts Ray's future
  no-override behavior now. No-op on CPU-only workspaces; required for GPU workspaces.
- **CUDA match**: the version-match discipline above extends to CUDA — when tensors move
  between a GPU workspace and GPU RayCluster workers, the workspace image and the worker
  image must be CUDA/driver compatible, not just Ray/Python compatible.

## Object store / shared memory sizing

Ray's object store (plasma) lives in `/dev/shm`. Containers default `/dev/shm` to 64MB, so
without intervention Ray spills to `/tmp` (slow) and warns. In a container Ray's auto-sizing
is non-deterministic (it may read the cgroup limit OR the whole node's RAM), so we **pin** the
object store explicitly and keep three values coupled and consistent:

```
--object-store-memory  ==  /dev/shm emptyDir sizeLimit  <=  sidecar memory limit (minus overhead)
```

Defaults use a modest **1Gi** object store appropriate for a connection broker. Raise all three
together (`rayIntegration.objectStoreMemory` + `rayIntegration.devShmSizeLimit` +
`rayIntegration.sidecar.resources.limits.memory`) for workloads that pull large Ray Data
results back into the workspace — **GPU notebooks that pull large tensors/Ray Data blocks
back into the driver typically want 4-8Gi.**

## `parameters`

Declares the contract the workspace fills in. Every `.Parameters.X` the template references
MUST be declared here: the validating webhook rejects a template that references an undeclared
parameter (author typo) and rejects a referencing workspace that omits a declared one.
`rayClusterName` is customer-supplied on the Workspace's `integrationTemplateRefs[].parameters`.

## `shareProcessNamespace`

The workspace container must see the Ray process that runs in the ray-sidecar (the workspace
attaches to the local Ray session the sidecar starts). A shared PID namespace makes the
sidecar's processes visible to — and signalable by — the workspace container. Both run as
uid 1000 (see the sidecar securityContext), so the cross-container signal/attach works without
granting root.

## `resourceRefs`

Fetch the customer-named RayCluster at reconcile time so the template can read live values from
it (head image, head service name, GCS port). `metadata.name` supports templates. Each entry
has a `name` handle, referenced as `` {{`{{ resource "<name>" "<jsonpath>" }}`}} ``. The
RayCluster always resolves in the workspace's own namespace; cross-namespace references are not
supported.

## `statusProbe`

The operator periodically execs `ray status` in the workspace container and records the verdict
in `workspace.status.integrationStatuses[]`. It is **report-only** (named `statusProbe`, not
`readinessProbe`, because it never gates the pod or restarts it) — it surfaces integration
health only. `ray status` exits 0 only when the local Ray session can reach the cluster's GCS,
so it catches data-plane failures (e.g. a GCS reconnect loop) that a control-plane RayCluster
`.status` check would miss.

## The `ray-sidecar` container

**Image** is resolved from the RayCluster head container, guaranteeing the sidecar's Ray binary
matches the cluster it joins (workspace↔cluster version match is still the customer's
responsibility — see above).

**`command: ["/bin/sh", "-c"]`** (not `/bin/bash`): the sidecar inherits the customer's
RayCluster head image, which we do not control. Slim/distroless Ray images (e.g.
`rayproject/ray:*-slim`) ship without `/bin/bash` and would CrashLoopBackOff on
`/bin/bash: not found`. `/bin/sh` is present on effectively all images, and the args use no
bash-specific syntax.

**`ray start` args** — join the RayCluster as a zero-resource client node:

- `--address=<head-svc>.<ns>.svc.cluster.local:<gcs-port>` — head service DNS name and GCS port
  are read from RayCluster `.status`, which KubeRay owns and keeps correct even if naming/port
  conventions change. The service DNS (not the pod IP) is stable across head restarts, enabling
  auto-reconnect.
- `--num-cpus=0 --num-gpus=0` — the sidecar contributes NO compute and advertises no GPUs, so the
  Ray scheduler never places tasks/actors on the workspace pod. It exists only to host the local
  Ray session the workspace attaches to.
- `--object-store-memory` — EXPLICITLY pin the plasma object store (see sizing section). We do NOT
  let Ray auto-size it: inside a container Ray may read either the cgroup limit OR the whole node's
  RAM (non-deterministic), and if it reads node RAM it sizes the store far larger than the
  sidecar's limit and OOM-kills it. Pinning makes the footprint deterministic.
- `--temp-dir=/tmp/ray` — explicit shared session dir (see `ray-tmp` volume); the workspace
  attaches via this dir, so it must be deterministic.
- `--block` — keep ray in the foreground so the container stays alive; on exit k8s restarts it and
  it re-resolves the head DNS (this is how head-restart auto-recovery works).

**Retry loop**: `ray start` can fail transiently — most commonly when the sidecar starts before
the RayCluster head's GCS is reachable (early attach). Rather than exit and lean on the kubelet's
CrashLoopBackOff (which escalates the restart delay up to ~5m and reads as a crashing container),
we retry in-container on a fixed 10s cadence. `--block` means a successful start never returns, so
the loop body only runs on failure. Bounded at 30 attempts (~5m) so a genuinely bad reference
(wrong cluster name, permanent misconfig) eventually exits with a clear message instead of
spinning forever.

**`resources`**: non-zero requests (a 0 request makes the sidecar first to be evicted and gives
the scheduler no signal). The memory limit must be `>= objectStoreMemory + raylet overhead
(~256Mi) + the /tmp/ray tmpfs` — see the sizing section for the coupling.

**`securityContext`**: `runAsUser`/`runAsGroup` must match the workspace image's uid so both
containers share the `/tmp/ray` session dir. `sagemaker-distribution` (sagemaker-user) and
`rayproject/ray` (ray) both use uid 1000, so the defaults work today — but it is image coupling.
If a custom workspace image runs as a different uid, override
`rayIntegration.sidecar.runAsUser`/`runAsGroup` in values.yaml or the workspace cannot read the
sidecar's session files and `ray.init()` fails. `runAsNonRoot`, `allowPrivilegeEscalation: false`,
and `drop: [ALL]` are defense-in-depth (the sidecar needs no Linux capabilities).

**No `readinessProbe`** on the sidecar (deliberate): Ray connectivity is reported by the
`statusProbe` (operator-run, report-only). A container `readinessProbe` here would instead GATE
pod readiness on Ray-cluster health — a RayCluster outage would flip the pod NotReady and drop the
Workspace to `Available=False` even though JupyterLab itself is fine. The sidecar exposes no
service ports, so readiness gating buys nothing; keep Ray health non-gating and let `statusProbe`
be the single source of integration health.

## Volumes

- **`ray-tmp`** — tmpfs shared session dir (Ray session/temp IO is latency-sensitive). Shared
  between the sidecar and the workspace container so the workspace's `ray.init()` discovers the
  sidecar's local Ray session. Cap bounds the tmpfs against the pod memory limit.
- **`ray-dshm`** — tmpfs `/dev/shm` sized to match `objectStoreMemory`. Container `/dev/shm`
  defaults to 64MB, which is too small for Ray's plasma object store and triggers the "object
  store using /tmp instead of /dev/shm" warning + degraded performance. `medium: Memory` means
  this counts against container memory, which is why the sidecar memory limit is sized to cover it
  plus the raylet. Keep `devShmSizeLimit >= objectStoreMemory` or Ray fails to start with "object
  store size exceeds the capacity of /dev/shm"; raise both together.

## Workspace container modifications (`primaryContainerModifications`)

- **volumeMounts** — mount the SAME `ray-tmp` session dir into the workspace container so
  `ray.init()` (`RAY_ADDRESS=auto`) finds the local session the sidecar created, and the SAME
  enlarged `ray-dshm` `/dev/shm` (the workspace client materializes Ray Data blocks locally when
  results are pulled back into the notebook).
- **`RAY_ADDRESS=auto`** — `ray.init()` with no arguments attaches to the local session in
  `/tmp/ray` (created by the sidecar) instead of bootstrapping a new cluster. This is the single
  most important env var — it is what makes `ray.init()` succeed from the workspace.
- **`RAY_ACCEL_ENV_VAR_OVERRIDE_ON_ZERO=0`** — see the GPU workspaces section.
- **`RAY_HEAD_IMAGE`, `RAY_CLUSTER_NAME`** — informational only (handy for debugging
  version-match issues). Safe to omit.
