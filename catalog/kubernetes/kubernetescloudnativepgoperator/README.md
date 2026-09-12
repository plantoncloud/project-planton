# Kubernetes Cloud Native PG Operator

## When NOT to Use This

**One installation per cluster.** The operator registers cluster-scoped
CRDs and mutating/validating webhooks whose service name is fixed by the
chart (`cnpg-webhook-service` — baked into the webhook certificate), so
a second installation would fight over both. The Helm release name is
therefore fixed to `cnpg` and never derives from `metadata.name`.

Also not the right component when:

- **You want a database** — this component installs and configures the
  ENGINE. The databases themselves are declared with KubernetesPostgres
  resources — one per PostgreSQL cluster — which the operator
  reconciles. "Install the operator" and "declare a database" are
  different lifecycles: platform teams own the first, application teams
  own the second.
- **You want ad-hoc operator actions** — promoting a replica, taking a
  one-off backup, hibernating a cluster are day-2 operations against the
  installed operator (the `cnpg` kubectl plugin or the CRDs directly),
  not installation configuration.

## Overview

**KubernetesCloudNativePgOperator** installs CloudNativePG — the CNCF
PostgreSQL operator — from the official Helm chart (`cloudnative-pg` at
`https://cloudnative-pg.github.io/charts`). The operator reconciles
`Cluster` custom resources into highly available PostgreSQL clusters:
streaming replication, automated failover with a safe primary election,
rolling updates, declarative roles and storage, and plugin-based
backups.

Backups are a SEPARATE BLOCK: CloudNativePG delegates object-store
backups to the Barman Cloud plugin (its built-in object-store support is
deprecated upstream and scheduled for removal), and the plugin is its own
chart, pin, and dependency set. The catalog installs it with its own kind
-- [KubernetesCnpgBarmanCloudPlugin](../kubernetescnpgbarmancloudplugin/)
-- declared in THIS operator's namespace (reference this resource's
`namespace` output). KubernetesPostgres backup blocks then declare WHERE
backups land. A backup-declaring database on a cluster without the plugin
never reconciles: the operator parks it in an unknown-plugin phase.

The typed spec covers the chart's meaningful configuration surface, with
a `helm_values` escape hatch (merged last, Helm `-f` semantics,
identical on both engines) for anything beyond it.

**Key design points:**

- **One release, one kind** — the operator release (`cnpg`) is all this
  component renders. The Barman Cloud plugin is a separate release in
  the same namespace (upstream forbids folding it into the operator's
  release — the two would fight over shared resource ownership) and a
  separate kind in the catalog, so "install the operator" and "add
  backups to the cluster" are declared, upgraded, and destroyed
  independently.
- **Databases survive uninstall by construction** — the chart stamps
  `helm.sh/resource-policy: keep` on every CRD unconditionally, so
  uninstalling the release never cascade-deletes the Cluster resources
  (and the databases behind them). The upstream safety posture, kept
  as-is; there is no destructive opt-out to misconfigure.
- **Extra replicas are warm standbys, not capacity** — the operator
  leader-elects; a second replica shortens failover of the OPERATOR
  itself and adds no reconciliation throughput
  (`max_concurrent_reconciles` is the throughput knob).
- **The install waits for real readiness** — both engines install
  atomically (600s timeout) with cleanup on fail: a PodMonitor rendered
  without the Prometheus operator CRDs fails THIS deploy with a clear
  rollback instead of surfacing later as Cluster resources that
  mysteriously never reconcile.

## Essential Configuration Fields

### Required

- **`spec.namespace`**: installation namespace (`cnpg-system` is the
  upstream convention) — literal or a KubernetesNamespace reference

### Common

- **`spec.create_namespace`**: create (and own) the namespace with the
  release — usually true for a dedicated `cnpg-system` namespace
- **`spec.chart_version`**: pinned chart version (default `0.29.0`,
  which ships operator 1.30.0 — chart and app versions move separately;
  the chart pin governs)
- **`spec.crds`**: `install` (chart default true) — disable only when
  something else manages the CRDs; every CRD carries the keep policy
  either way
- **`spec.replicas`**: operator replica count (chart default 1; extras
  are leader-elected warm standbys)
- **`spec.resources`**: operator container resources (the chart ships
  none by default)
- **`spec.watch`**: `cluster_wide` (chart default true — ClusterRole
  RBAC, the normal posture) or false with `namespaces` to fence the
  operator into specific namespaces (namespace-scoped RBAC)
- **`spec.operator_config`**: the chart's `config.data` map —
  `INHERITED_ANNOTATIONS`, `INHERITED_LABELS`, `PULL_SECRET_NAME`, ...
  (namespace scoping has its own typed field above; a `WATCH_NAMESPACE`
  entry here is always stripped in favor of it)
- **`spec.max_concurrent_reconciles`**: Cluster resources reconciled
  concurrently (chart default 10) — raise on control planes managing
  many databases
- **`spec.monitoring`**: `pod_monitor_enabled` (the operator's OWN
  reconcile-loop metrics; requires the Prometheus operator CRDs — the
  release fails to install without them) and `grafana_dashboard` (the
  upstream dashboard as a sidecar-labeled ConfigMap)
- **`spec.priority_class_name` / `node_selector` / `tolerations`**:
  scheduling for the operator pod — databases stop failing over without
  their operator; keep it above workload priority
- **`spec.image` / `spec.image_pull_secrets`**: operator image override
  for registry mirrors and air-gapped clusters (empty = the chart
  default, ghcr.io/cloudnative-pg/cloudnative-pg at the chart's app
  version)
- **`spec.helm_values`**: escape hatch for chart values beyond the typed
  fields (webhook tuning, update strategy, security contexts, topology
  spread, host network, ...) — never the primary interface.

A cluster that ALREADY runs CloudNativePG (a self-hosted platform
operator installs one; check with `kubectl get deploy -A -l
app.kubernetes.io/name=cloudnative-pg`) cannot take a second copy —
declare only what it is missing, usually the plugin kind. See
[GUIDE.md](GUIDE.md) for the leftover-CRD trap a non-Helm uninstall
leaves behind.

## Environment Injection

The operator itself carries NO cloud identity: backups authenticate as
the DATABASE pods, so the keyless posture (EKS IRSA / GKE Workload
Identity / AKS Workload Identity) is declared per KubernetesPostgres —
its `workload_identity` field annotates each cluster's own
ServiceAccount. This component is identical on every environment
Kubernetes runs in.

| Environment | This component | Where backups live |
|---|---|---|
| Any cluster, no backups | operator release only | — |
| Any cluster, object-store backups | operator release + a KubernetesCnpgBarmanCloudPlugin in its namespace (cert-manager required by the plugin) | `KubernetesPostgres.spec.backup` + `workload_identity`, per database |

## Stack Outputs

| Output | Purpose |
|---|---|
| `namespace` | Namespace the operator runs in — the plugin kind's `namespace` references it |
| `release_name` | Helm release name of the operator (always `cnpg`) |

## Composing in Infra Charts

- **`spec.namespace`** is a foreign key (default kind
  KubernetesNamespace, field path `spec.name`).
- **KubernetesPostgres resources need no reference to this component** —
  they compose against the CRDs it installs; deploy the operator first,
  the databases after.
- **Backups are a chain of three**: an infra chart deploying
  KubernetesCertManager → this component → KubernetesCnpgBarmanCloudPlugin
  (its `namespace` referencing this resource) → KubernetesPostgres (with
  a backup block) lands the whole story in dependency order; the
  plugin's reference onto this resource IS the ordering edge.

## Examples

### Minimal (operator only, upstream defaults)

```yaml
apiVersion: kubernetes.planton.dev/v1alpha1
kind: KubernetesCloudNativePgOperator
metadata:
  name: cnpg
spec:
  namespace:
    value: cnpg-system
  create_namespace: true
```

### Backup-capable (the plugin kind beside it; cert-manager on the cluster)

```yaml
apiVersion: kubernetes.planton.dev/v1alpha1
kind: KubernetesCloudNativePgOperator
metadata:
  name: cnpg
spec:
  namespace:
    value: cnpg-system
  create_namespace: true
---
apiVersion: kubernetes.planton.dev/v1alpha1
kind: KubernetesCnpgBarmanCloudPlugin
metadata:
  name: cnpg-barman-plugin
spec:
  namespace:
    valueFrom:
      name: cnpg # the operator's namespace output -- the plugin MUST live there
```

### Production posture (standbys, resources, telemetry, scheduling)

```yaml
apiVersion: kubernetes.planton.dev/v1alpha1
kind: KubernetesCloudNativePgOperator
metadata:
  name: cnpg
spec:
  namespace:
    value: cnpg-system
  create_namespace: true
  replicas: 2 # leader-elected warm standby
  resources:
    requests:
      cpu: 100m
      memory: 256Mi
    limits:
      cpu: "1"
      memory: 512Mi
  max_concurrent_reconciles: 20
  monitoring:
    pod_monitor_enabled: true # requires the Prometheus operator CRDs
    grafana_dashboard: true
  priority_class_name: system-cluster-critical
```

---

© Planton. Licensed under [Apache-2.0](https://github.com/plantonhq/planton/blob/main/LICENSE).
