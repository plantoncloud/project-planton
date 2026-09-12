# KubernetesCloudNativePgOperator Pulumi Module

Installs CloudNativePG from the official Helm chart
(`https://cloudnative-pg.github.io/charts`) as ONE Helm release. The
typed spec renders into chart values in `module/values.go`; the
`helm_values` escape hatch merges LAST over them with Helm `-f`
semantics (maps deep-merge, later document wins, lists replace) — the
exact semantic twin of the Terraform module's `helm_release` with
`values = [typed, helm_values]`.

## What the Module Creates

1. **Namespace** (optional) — created with the standard governance
   labels when `create_namespace` is true; otherwise the namespace must
   already exist
2. **Helm Release `cnpg`** — the operator chart (`cloudnative-pg`,
   pinned default 0.29.0 = operator 1.30.0). The release name is FIXED:
   the operator registers cluster-scoped CRDs and webhooks whose service
   name is baked into the chart (`cnpg-webhook-service` — embedded in
   the webhook certificate and not configurable), so one installation
   per cluster is an upstream constraint and the name never derives from
   `metadata.name`

The Barman Cloud backup plugin is NOT rendered here. It is a separate
chart, pin, and dependency set, installed into this release's namespace
by its own kind (KubernetesCnpgBarmanCloudPlugin); upstream forbids
folding it into the operator's release.

## Rendering Notes

- **Chart-default-matching values render only on divergence** —
  `crds.create` (only on explicit opt-out), `config.clusterWide` (only
  when fencing), monitoring flags (only when on) — the rendered values
  stay minimal on both engines.
- **The typed watch field OWNS `WATCH_NAMESPACE`** — a user entry under
  that key in `operator_config` is always stripped; the key renders only
  from `watch.namespaces` (comma-joined) when `cluster_wide` is false.
  Spec CEL rules guarantee namespaces are present exactly when the fence
  is on.
- **The chart folds three typed concerns into one `config` block** —
  `clusterWide`, `data` (the operator's ConfigMap entries), and
  `maxConcurrentReconciles` — rendered together so the block appears at
  most once.
- **No `fullnameOverride`** — the chart hard-codes the names that matter
  (the webhook service is `cnpg-webhook-service` regardless of release
  name); there is nothing for an override to pin.
- **CRD keep is unconditional** — the chart stamps
  `helm.sh/resource-policy: keep` on every CRD, so uninstalling never
  cascade-deletes the Cluster resources (and the databases behind them);
  no keep knob is needed or modeled.

## Wait / Atomic Posture

The release installs with `Atomic` + `CleanupOnFail` and a 600s timeout,
waiting for readiness. An operator that never becomes ready (a
PodMonitor rendered without the Prometheus operator CRDs is THE classic
install failure) fails THIS deploy with a readiness timeout instead of
surfacing later as Cluster resources that mysteriously never reconcile.

## Usage

```shell
planton pulumi up --manifest e2e/manifest.yaml --module-dir <path-to-this-module>
```

## Outputs

| Output | Description |
|---|---|
| `namespace` | Namespace the operator runs in — the plugin kind's `namespace` references it |
| `release_name` | Helm release name of the operator (fixed `cnpg` — one installation per cluster) |

## Module Structure

- `main.go`: entrypoint that calls the module
- `module/main.go`: namespace → operator release → output exports
- `module/values.go`: typed-spec → chart values rendering (CRD
  lifecycle, sizing, the config block with the WATCH_NAMESPACE
  precedence, telemetry, scheduling, image) and the escape-hatch merge
- `module/locals.go`: resolved namespace and chart version — kept in
  lockstep with the Terraform module's `locals.tf`
- `module/vars.go`: chart identity, pinned default version (0.29.0 =
  operator 1.30.0), the fixed release name, the 600s timeout
- `module/helpers.go`: shared shape renderers (resources, tolerations,
  the Helm `-f` merge)
