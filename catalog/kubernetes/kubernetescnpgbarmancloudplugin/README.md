# Kubernetes CNPG Barman Cloud Plugin

## When NOT to Use This

**One plugin per operator namespace, and only in the operator's
namespace.** CloudNativePG discovers plugins through Services labeled
`cnpg.io/pluginName` in its OWN namespace only, and the chart fixes that
Service's name to `barman-cloud` (baked into the plugin's TLS
certificate). A plugin installed anywhere else is invisible to the
operator; a second one in the same namespace fights the first over the
fixed-name Service, Secrets, and ConfigMap. The Helm release name is
therefore fixed to `plugin-barman-cloud` and never derives from
`metadata.name`.

Also not the right component when:

- **The thing that installed CloudNativePG also owns a plugin toggle** —
  a self-hosted platform operator that installs CloudNativePG for its own
  database and can install the plugin too. Enable it THERE: two owners of
  the singleton collide on the fixed names. This kind is for clusters
  where the operator's installer offers no plugin (the catalog's
  KubernetesCloudNativePgOperator, a bare Helm or GitOps install).
- **You want to configure a backup** — this component installs the
  ENGINE that backups run through. WHERE a database's backups land, how
  often, and how long they are kept is declared on each
  KubernetesPostgres (`backup`, `bootstrap.recovery`).
- **You want the operator** — that is KubernetesCloudNativePgOperator.
  This kind assumes one is already running in `namespace` (the
  catalog's, referenced; or a resident one, named by literal).

## Overview

**KubernetesCnpgBarmanCloudPlugin** installs the Barman Cloud plugin for
CloudNativePG from the official Helm chart (`plugin-barman-cloud` at
`https://cloudnative-pg.github.io/charts`). The plugin is CloudNativePG's
object-store backup path: WAL archiving, scheduled base backups, and
restores to S3, S3-compatible stores, GCS, and Azure Blob. The operator's
built-in object-store support is deprecated upstream, so every
KubernetesPostgres backup block and object-store recovery renders its
Cluster against THIS plugin.

Without the plugin on the cluster, such a Cluster never reconciles: the
operator parks it in the phase "Cluster cannot proceed to reconciliation
due to an unknown plugin being required" (`kubectl get cluster` shows it;
the Planton apply waits on Ready until it times out). Install the plugin
BEFORE the first backup block, not after.

The typed spec covers the chart's meaningful configuration surface, with
a `helm_values` escape hatch (merged with Helm `-f` semantics, identical
on both engines) for anything beyond it.

**Key design points:**

- **Its own release, its own kind** — the plugin is a different chart
  with a different pin than the operator (chart `0.7.0` = plugin
  `v0.13.0`; the operator chart `0.29.0` = CloudNativePG `1.30.0`), a
  different dependency (it needs cert-manager; the operator does not),
  and on many clusters a different owner. Upstream forbids folding it
  into the operator's release, so one kind per release is also the
  honest grain: "install the operator" and "add backups to the cluster"
  are declared, upgraded, and destroyed independently.
- **The namespace IS the operator's** — `spec.namespace` is annotated
  onto KubernetesCloudNativePgOperator's `namespace` output, so a bare
  `valueFrom` naming the operator resource encodes the constraint and
  orders the plugin after the operator. A literal namespace names a
  resident operator someone else installed.
- **Discoverable by construction** — the chart stamps the plugin's
  Service with the `cnpg.io/pluginName: barman-cloud.cloudnative-pg.io`
  label the operator's plugin controller watches; a Cluster names the
  plugin by that identifier (exported as `plugin_name`).
- **Fixed identities re-pinned after the escape hatch** — the Service
  name and the name overrides are closed after `helm_values` merges, so
  no values document can rename what the operator handshake, the E2E
  verifier, and the import recipes key on.
- **ObjectStores survive uninstall by construction** — the chart stamps
  `helm.sh/resource-policy: keep` on the ObjectStore CRD, so uninstalling
  the plugin never deletes the ObjectStore resources databases point at.
  A later install with the same release name and namespace adopts the
  kept CRD — one more reason the release name is fixed.
- **The install waits for real readiness** — both engines install
  atomically (600s timeout) with cleanup on fail: without cert-manager
  the chart's Certificates never issue, and the deploy fails with a
  clear rollback instead of a plugin that silently never registers.

## Essential Configuration Fields

### Required

- **`spec.namespace`**: the CloudNativePG operator's namespace — a
  reference to the operator resource (default kind
  KubernetesCloudNativePgOperator, field path
  `status.outputs.namespace`) or the literal namespace of a resident
  operator (`cnpg-system` is the upstream convention)

### Common

- **`spec.create_namespace`**: almost always false — the namespace is
  the operator's and already exists; true is only correct when this
  resource is the first to claim it (and then destroying the plugin
  deletes the namespace)
- **`spec.chart_version`**: pinned chart version (default `0.7.0`, which
  ships plugin v0.13.0 — chart and app versions move separately; the
  chart pin governs). Plugin v0.13.0 needs CloudNativePG 1.26 or later;
  upstream strongly recommends 1.27 or later for its plugin error
  reporting on the Cluster status
- **`spec.crds`**: `install` (chart default true) — disable only when
  something else manages the ObjectStore CRD; it carries the keep policy
  either way
- **`spec.replicas`**: plugin replica count (chart default 1; the chart
  deploys with the Recreate strategy, extras are leader-elected warm
  standbys)
- **`spec.resources`**: plugin container resources (the chart ships none
  by default; the plugin is light — the backup work runs in a sidecar
  inside each database pod, sized by the database)
- **`spec.image` / `spec.sidecar_image` / `spec.image_pull_secrets`**:
  registry mirrors and air-gapped clusters. Mirror BOTH images — the
  sidecar is what the DATABASE pods pull (published through the plugin's
  config ConfigMap and injected into every instance pod), so its pull
  secrets belong on the KubernetesPostgres resources, not here
- **`spec.priority_class_name` / `node_selector` / `tolerations`**:
  scheduling for the plugin pod — archiving falls behind while the
  plugin is evicted; keep it at the operator's priority
- **`spec.helm_values`**: escape hatch for chart values beyond the typed
  fields (`additionalArgs`, `additionalEnv`, update strategy, security
  contexts, topology spread, host network, certificate durations, ...)
  — never the primary interface. The fixed identities (`service.name`,
  the name overrides) are re-pinned after it.

## Environment Injection

The plugin carries NO cloud identity of its own: backups authenticate as
the DATABASE pods, so the keyless posture (EKS IRSA / GKE Workload
Identity / AKS Workload Identity) is declared per KubernetesPostgres —
its `workload_identity` field annotates each cluster's own
ServiceAccount, and its backup block's keyless arm points at the store.
This component is identical on every environment Kubernetes runs in.

| Environment | This component | Where backup identity lives |
|---|---|---|
| Any cluster, catalog operator | `namespace` by reference to the operator resource | `KubernetesPostgres.spec.workload_identity` + the backup block's keyless arm, per database |
| Any cluster, resident operator (Helm, GitOps, a platform operator without a plugin toggle) | `namespace` as the literal operator namespace | same |

## Stack Outputs

| Output | Purpose |
|---|---|
| `namespace` | Namespace the plugin runs in — the operator's namespace |
| `release_name` | Helm release name of the plugin (always `plugin-barman-cloud`) |
| `plugin_name` | The CNPG-I identifier a Cluster's `plugins` list names (always `barman-cloud.cloudnative-pg.io`) |

## Composing in Infra Charts

- **`spec.namespace`** is a foreign key (default kind
  KubernetesCloudNativePgOperator, field path `status.outputs.namespace`)
  — the reference is the ordering edge: the operator (and its CRDs)
  exists before the plugin registers with it.
- **Registry prerequisites**: KubernetesCertManager (the chart's TLS
  issuer) and KubernetesCloudNativePgOperator (the engine it registers
  with). On a cluster where one or both are resident, declare them as
  such rather than installing a second copy.
- **KubernetesPostgres resources need no reference to this component** —
  they name the plugin by its fixed identifier inside their Cluster. In a
  chart, give backup-declaring databases a `depends_on` edge to the
  plugin so the first reconcile finds it.
- **The chain of three**: KubernetesCertManager →
  KubernetesCloudNativePgOperator → this component → KubernetesPostgres
  (with a backup block).

## Examples

### Beside the catalog's operator (by reference)

```yaml
apiVersion: kubernetes.planton.dev/v1alpha1
kind: KubernetesCnpgBarmanCloudPlugin
metadata:
  name: cnpg-barman-plugin
spec:
  namespace:
    valueFrom:
      name: cnpg # a KubernetesCloudNativePgOperator resource; its namespace output
```

### Beside a resident CloudNativePG (by literal)

```yaml
apiVersion: kubernetes.planton.dev/v1alpha1
kind: KubernetesCnpgBarmanCloudPlugin
metadata:
  name: cnpg-barman-plugin
spec:
  namespace:
    value: cnpg-system # the resident operator's namespace: kubectl get deploy -A -l app.kubernetes.io/name=cloudnative-pg
```

### Air-gapped (both images mirrored, resources, scheduling)

```yaml
apiVersion: kubernetes.planton.dev/v1alpha1
kind: KubernetesCnpgBarmanCloudPlugin
metadata:
  name: cnpg-barman-plugin
spec:
  namespace:
    valueFrom:
      name: cnpg
  image:
    repository: mirror.example.com/cloudnative-pg/plugin-barman-cloud
  sidecar_image:
    repository: mirror.example.com/cloudnative-pg/plugin-barman-cloud-sidecar
  image_pull_secrets:
    - mirror-pull # the plugin pod's; database pods carry their own
  resources:
    requests:
      cpu: 50m
      memory: 64Mi
    limits:
      cpu: 200m
      memory: 256Mi
  priority_class_name: system-cluster-critical
```

---

© Planton. Licensed under [Apache-2.0](https://github.com/plantonhq/planton/blob/main/LICENSE).
