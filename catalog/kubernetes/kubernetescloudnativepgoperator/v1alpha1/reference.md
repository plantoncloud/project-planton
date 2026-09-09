# KubernetesCloudNativePgOperator

> Generated from the protobuf schema by `make generate-reference` -- do not
> edit by hand. To change a fact on this page, change the proto field comment
> or validation rule it is derived from, then regenerate.

**apiVersion**: `kubernetes.planton.dev/v1alpha1`

**Guide**: [GUIDE.md](../GUIDE.md) -- authored operational judgment for this component: conventions, trade-offs, and what pairs well with it.

**KubernetesCloudNativePgOperatorSpec** installs CloudNativePG — the CNCF
PostgreSQL operator — from the official Helm chart (`cloudnative-pg` at
https://cloudnative-pg.github.io/charts). The operator reconciles
`Cluster` custom resources into highly available PostgreSQL clusters:
streaming replication, automated failover with a safe primary election,
rolling updates, declarative roles and storage, and plugin-based backups.

This component installs and configures the ENGINE. The databases
themselves are declared with KubernetesPostgres resources — one per
PostgreSQL cluster — which the operator reconciles.

ONE INSTALLATION PER CLUSTER: the operator registers cluster-scoped CRDs
and mutating/validating webhooks whose service name is fixed by the chart
(it is baked into the webhook certificate), so a second installation
would fight over both. The Helm release name is therefore fixed to
"cnpg".

BACKUPS ARE PLUGIN-BASED: CloudNativePG delegates object-store backups to
the Barman Cloud plugin (its built-in object-store support is deprecated
upstream and scheduled for removal). Enable `barman_cloud_plugin` here to
install the plugin alongside the operator; KubernetesPostgres backup
blocks then declare WHERE backups land. The plugin's internal TLS is
issued by cert-manager, so the plugin arm requires cert-manager on the
cluster (KubernetesCertManager).

A CLUSTER THAT ALREADY RUNS CLOUDNATIVEPG — installed by the Planton
operator, by Helm, by GitOps — cannot take a second copy (see above),
and a resident operator that came without the plugin cannot back
anything up. `install_operator: false` is the posture for that cluster:
this resource then manages ONLY the plugin, beside whichever CloudNativePG
is already there, and every KubernetesPostgres on the cluster gains
object-store backups.

The typed fields below cover the chart's meaningful configuration
surface; `helm_values` remains as the escape hatch for chart values
beyond them (merged last, Helm `-f` semantics, identical on both
engines) — a safety valve, never the primary interface.

## Example

```yaml
# Full-surface test manifest: exercises every typed arm of the spec so the
# offline plan proofs cover what the live lanes may not. Not a realistic
# production shape — see presets for those.
#
# watch.clusterWide is true here, so watch.namespaces stays empty (the spec
# CEL rules reject namespaces on a cluster-wide operator). The
# WATCH_NAMESPACE entry in operatorConfig below is DELIBERATE: it proves
# the typed watch field owns that key — the modules strip it.
apiVersion: kubernetes.planton.dev/v1alpha1
kind: KubernetesCloudNativePgOperator
metadata:
  name: hack-cnpg
spec:
  namespace:
    value: hack-cnpg-system
  createNamespace: true
  chartVersion: "0.29.0"
  crds:
    install: true
  replicas: 2
  resources:
    requests:
      cpu: 100m
      memory: 128Mi
    limits:
      cpu: "1"
      memory: 512Mi
  watch:
    clusterWide: true
  operatorConfig:
    INHERITED_ANNOTATIONS: categories
    INHERITED_LABELS: environment,workload
    WATCH_NAMESPACE: stripped-by-typed-watch
  maxConcurrentReconciles: 20
  barmanCloudPlugin:
    enabled: true
    chartVersion: "0.7.0"
    resources:
      requests:
        cpu: 50m
        memory: 64Mi
      limits:
        memory: 256Mi
  monitoring:
    podMonitorEnabled: true
    grafanaDashboard: true
  priorityClassName: system-cluster-critical
  nodeSelector:
    kubernetes.io/os: linux
  tolerations:
    - key: node-role.kubernetes.io/control-plane
      operator: Exists
      effect: NoSchedule
    - key: dedicated
      operator: Equal
      value: databases
      effect: NoExecute
      tolerationSeconds: 300
  imagePullSecrets:
    - hack-registry-credentials
  image:
    repository: mirror.example.com/cloudnative-pg/cloudnative-pg
    tag: 1.30.0
  helmValues: |
    webhook:
      livenessProbe:
        initialDelaySeconds: 5
```

## Spec Fields

| Path | Type | Required | Default | References |
|---|---|---|---|---|
| `spec.namespace` | `string \| valueFrom` | yes |  | KubernetesNamespace (`spec.name`) |
| `spec.createNamespace` | `bool` |  |  |  |
| `spec.chartVersion` | `string` |  | `0.29.0` |  |
| `spec.crds` | `KubernetesCloudNativePgOperatorCrds` |  |  |  |
| `spec.crds.install` | `bool` |  | `true` |  |
| `spec.replicas` | `int32` |  | `1` |  |
| `spec.resources` | `ContainerResources` |  |  |  |
| `spec.resources.limits` | `CpuMemory` |  |  |  |
| `spec.resources.limits.cpu` | `string` |  |  |  |
| `spec.resources.limits.memory` | `string` |  |  |  |
| `spec.resources.requests` | `CpuMemory` |  |  |  |
| `spec.resources.requests.cpu` | `string` |  |  |  |
| `spec.resources.requests.memory` | `string` |  |  |  |
| `spec.watch` | `KubernetesCloudNativePgOperatorWatch` |  |  |  |
| `spec.watch.clusterWide` | `bool` |  | `true` |  |
| `spec.watch.namespaces` | `[]string` |  |  |  |
| `spec.operatorConfig` | `map<string, string>` |  |  |  |
| `spec.maxConcurrentReconciles` | `int32` |  | `10` |  |
| `spec.barmanCloudPlugin` | `KubernetesCloudNativePgOperatorBarmanPlugin` |  |  |  |
| `spec.barmanCloudPlugin.enabled` | `bool` |  |  |  |
| `spec.barmanCloudPlugin.chartVersion` | `string` |  | `0.7.0` |  |
| `spec.barmanCloudPlugin.resources` | `ContainerResources` |  |  |  |
| `spec.barmanCloudPlugin.resources.limits` | `CpuMemory` |  |  |  |
| `spec.barmanCloudPlugin.resources.limits.cpu` | `string` |  |  |  |
| `spec.barmanCloudPlugin.resources.limits.memory` | `string` |  |  |  |
| `spec.barmanCloudPlugin.resources.requests` | `CpuMemory` |  |  |  |
| `spec.barmanCloudPlugin.resources.requests.cpu` | `string` |  |  |  |
| `spec.barmanCloudPlugin.resources.requests.memory` | `string` |  |  |  |
| `spec.monitoring` | `KubernetesCloudNativePgOperatorMonitoring` |  |  |  |
| `spec.monitoring.podMonitorEnabled` | `bool` |  |  |  |
| `spec.monitoring.grafanaDashboard` | `bool` |  |  |  |
| `spec.priorityClassName` | `string` |  |  |  |
| `spec.nodeSelector` | `map<string, string>` |  |  |  |
| `spec.tolerations` | `[]WorkloadToleration` |  |  |  |
| `spec.tolerations[].key` | `string` |  |  |  |
| `spec.tolerations[].operator` | `string` |  |  |  |
| `spec.tolerations[].value` | `string` |  |  |  |
| `spec.tolerations[].effect` | `string` |  |  |  |
| `spec.tolerations[].tolerationSeconds` | `int64` |  |  |  |
| `spec.imagePullSecrets` | `[]string` |  |  |  |
| `spec.image` | `KubernetesCloudNativePgOperatorImage` |  |  |  |
| `spec.image.repository` | `string` |  |  |  |
| `spec.image.tag` | `string` |  |  |  |
| `spec.helmValues` | `string` |  |  |  |
| `spec.installOperator` | `bool` |  | `true` |  |

## Field Details

### spec.namespace

`string | valueFrom` · required

Namespace to install the operator into ("cnpg-system" is the upstream
convention). Accepts a literal namespace name or a reference to a
KubernetesNamespace resource.

- references: KubernetesNamespace (`spec.name`)
- rule: {"required":true}
- rule: write as {value: <literal>} or {valueFrom: {kind: KubernetesNamespace, name: <that resource's name>, fieldPath: spec.name}} -- a bare string does not parse

### spec.createNamespace

`bool`

When true, the namespace is created (with the standard Planton
governance labels) before installing and deleted with the resource.
When false, the namespace must already exist.

### spec.chartVersion

`string` · optional (explicit presence)

Helm chart version to install (e.g. "0.29.0", which ships operator
1.30.0 — chart and app versions move separately; the chart pin
governs). Pin deliberately; upgrades re-run the release with the new
chart. Pick versions from the chart repository's index
(`helm search repo`): the served chart is the contract — the upstream
source tree's Chart.yaml can claim a version at a tag that was never
served.

- default: `0.29.0`

### spec.crds

`KubernetesCloudNativePgOperatorCrds`

CloudNativePG custom resource definitions (Cluster, ScheduledBackup,
Backup, Pooler, Database, ...) lifecycle.

### spec.crds.install

`bool` · optional (explicit presence)

Install the CRDs with the release. Chart default: true. Disable only
when something else manages them. The chart stamps every CRD with
`helm.sh/resource-policy: keep` unconditionally, so uninstalling the
release NEVER cascade-deletes the Cluster resources (and the
databases behind them) — the upstream safety posture, kept as-is.

- default: `true`

### spec.replicas

`int32` · optional (explicit presence)

Operator replica count. Chart default: 1. Extra replicas are
leader-elected warm standbys that shorten failover of the OPERATOR
itself — they add no reconciliation throughput.

- default: `1`
- rule: {"int32":{"gte":1}}

### spec.resources

`ContainerResources`

Operator container resources. Empty = no requests/limits (the chart
ships none by default).

### spec.resources.limits

`CpuMemory`

The resource limits for the container.
Specify the maximum amount of CPU and memory that the container can use.

### spec.resources.limits.cpu

`string`

### spec.resources.limits.memory

`string`

### spec.resources.requests

`CpuMemory`

The resource requests for the container.
Specify the minimum amount of CPU and memory that the container is guaranteed.

### spec.resources.requests.cpu

`string`

### spec.resources.requests.memory

`string`

### spec.watch

`KubernetesCloudNativePgOperatorWatch`

What the operator watches: the whole cluster (chart default) or a
fenced set of namespaces.

- rule: watch namespaces only apply when cluster_wide is explicitly false — a cluster-wide operator (the default) already watches everything
- rule: a namespace-fenced operator (cluster_wide false) needs at least one namespace to watch

### spec.watch.clusterWide

`bool` · optional (explicit presence)

Watch the whole cluster. Chart default: true — the normal posture;
RBAC is created as ClusterRoles. Set false to fence the operator
into specific namespaces (namespace-scoped RBAC).

- default: `true`

### spec.watch.namespaces

`[]string`

Namespaces to watch when cluster_wide is false (rendered as the
WATCH_NAMESPACE operator-config entry).

### spec.operatorConfig

`map<string, string>`

Operator configuration entries (the chart's `config.data` map —
INHERITED_ANNOTATIONS, INHERITED_LABELS, PULL_SECRET_NAME, ...; see
the operator-configuration page of the upstream documentation for the
full vocabulary). Namespace scoping has its own typed field above —
WATCH_NAMESPACE set here would be overwritten by it.

### spec.maxConcurrentReconciles

`int32` · optional (explicit presence)

Maximum number of Cluster resources reconciled concurrently. Chart
default: 10. Raise on control planes managing many databases.

- default: `10`
- rule: {"int32":{"gte":1}}

### spec.barmanCloudPlugin

`KubernetesCloudNativePgOperatorBarmanPlugin`

The Barman Cloud backup plugin — the object-store backup path for
every KubernetesPostgres on the cluster. Deployed as its own set of
resources beside the operator release (upstream forbids folding the
plugin into the operator's Helm release — the two would fight over
shared resource ownership).

### spec.barmanCloudPlugin.enabled

`bool`

Deploy the plugin. REQUIRES cert-manager on the cluster
(KubernetesCertManager): the plugin's operator↔sidecar TLS
certificates are cert-manager Certificates, and the install fails
without it. Without the plugin, KubernetesPostgres backup blocks
cannot function.

### spec.barmanCloudPlugin.chartVersion

`string` · optional (explicit presence)

Plugin chart version to install (e.g. "0.7.0", which ships plugin
v0.13.0). Pin deliberately.

- default: `0.7.0`

### spec.barmanCloudPlugin.resources

`ContainerResources`

Plugin container resources. Empty = no requests/limits (the chart
ships none by default).

### spec.barmanCloudPlugin.resources.limits

`CpuMemory`

The resource limits for the container.
Specify the maximum amount of CPU and memory that the container can use.

### spec.barmanCloudPlugin.resources.limits.cpu

`string`

### spec.barmanCloudPlugin.resources.limits.memory

`string`

### spec.barmanCloudPlugin.resources.requests

`CpuMemory`

The resource requests for the container.
Specify the minimum amount of CPU and memory that the container is guaranteed.

### spec.barmanCloudPlugin.resources.requests.cpu

`string`

### spec.barmanCloudPlugin.resources.requests.memory

`string`

### spec.monitoring

`KubernetesCloudNativePgOperatorMonitoring`

The operator's own Prometheus telemetry (reconcile-loop metrics) and
the upstream Grafana dashboard.

### spec.monitoring.podMonitorEnabled

`bool`

Create a PodMonitor for the operator's own metrics. Requires the
Prometheus operator CRDs on the cluster — the release FAILS to
install without them.

### spec.monitoring.grafanaDashboard

`bool`

Ship the upstream Grafana dashboard as a ConfigMap labeled for the
Grafana sidecar to auto-load.

### spec.priorityClassName

`string`

PriorityClass for the operator pod. Databases stop failing over
without their operator — keep it above workload priority.

### spec.nodeSelector

`map<string, string>`

Node selector for the operator pod.

### spec.tolerations

`[]WorkloadToleration`

Tolerations for the operator pod.

### spec.tolerations[].key

`string`

Taint key to tolerate. Empty key with operator "Exists" tolerates every taint.

### spec.tolerations[].operator

`string`

How key/value match: "Equal" (default — value must match too) or "Exists"
(key presence alone matches).

- rule: Toleration operator must be either "Equal" or "Exists"

### spec.tolerations[].value

`string`

Taint value to match when operator is "Equal".

### spec.tolerations[].effect

`string`

Which taint effect is tolerated: "NoSchedule", "PreferNoSchedule", or
"NoExecute". Empty tolerates all effects for the key.

- rule: Toleration effect must be one of "NoSchedule", "PreferNoSchedule", or "NoExecute"

### spec.tolerations[].tolerationSeconds

`int64` · optional (explicit presence)

For "NoExecute" taints only: how many seconds already-running pods stay bound
after the taint appears. Unset means tolerate forever.

### spec.imagePullSecrets

`[]string`

Names of image-pull secrets (in the installation namespace) for
pulling the operator image from a private mirror.

### spec.image

`KubernetesCloudNativePgOperatorImage`

Override the operator image (registry mirrors, air-gapped clusters).
Empty = the chart default (ghcr.io/cloudnative-pg/cloudnative-pg at
the chart's app version).

### spec.image.repository

`string`

Image repository (e.g. "my-mirror.example.com/cloudnative-pg").

### spec.image.tag

`string`

Image tag. Empty = the chart's app version.

### spec.helmValues

`string`

Escape hatch: additional chart values as a YAML document, merged LAST
over everything the typed fields render (Helm `-f` semantics,
identical on both engines). For the chart surface beyond the typed
fields (webhook tuning, update strategy, security contexts, topology
spread, host network, ...) — never the substitute for them. Do not
put secrets here.

### spec.installOperator

`bool` · optional (explicit presence)

Install the operator release. Default true. Set false on a cluster
that ALREADY runs CloudNativePG (the Planton operator installs one for
the platform's own database; a Helm or GitOps install counts too):
the operator's CRDs and webhooks are cluster singletons, so a second
copy would fight the resident one — instead this resource manages only
the Barman Cloud plugin (`barman_cloud_plugin.enabled` is then
required), installing it into `namespace` beside the resident
operator, and every KubernetesPostgres on the cluster gains
object-store backups. The operator-shaping fields (`crds`, `replicas`,
`resources`, `watch`, `operator_config`, `max_concurrent_reconciles`,
`monitoring`, `priority_class_name`, `node_selector`, `tolerations`,
`image_pull_secrets`, `image`, `helm_values`) describe a release this
resource does not own in that posture and must stay unset. Destroying
a plugin-only declaration removes the plugin and leaves the resident
operator running (live-proven). Deciding which posture applies: `kubectl
get deploy -A -l app.kubernetes.io/name=cloudnative-pg` — a hit means
plugin-only. Beware the reverse case too: a CloudNativePG that was
UNINSTALLED by a non-Helm owner (the Planton operator's, a raw
manifest) can leave its cluster-scoped CRDs, webhooks, and RBAC behind
with that owner's labels, and a full install here then fails at the
Helm ownership check ("managed-by must equal Helm") — delete the
leftovers first; nothing here adopts them.

- default: `true`

## Validation Rules

- `spec.plugin_only_requires_plugin`: install_operator false means this resource manages only the Barman Cloud plugin beside a CloudNativePG that is already on the cluster — enable barman_cloud_plugin, or drop install_operator to install the operator itself
- `spec.plugin_only_no_operator_config`: with install_operator false the operator release is not managed here, so operator-shaping fields are dead configuration — remove crds, resources, watch, operator_config, monitoring, priority_class_name, node_selector, tolerations, image_pull_secrets, image, and helm_values (the plugin's own knobs live under barman_cloud_plugin)

## Outputs

Reference an output from another manifest as `valueFrom: {kind: KubernetesCloudNativePgOperator, name: <resource-name>, fieldPath: status.outputs.<output>}`.

| Output | Type | Description |
|---|---|---|
| `status.outputs.namespace` | `string` | Namespace the operator (and the plugin, when enabled) runs in. |
| `status.outputs.release_name` | `string` | Helm release name of the operator (fixed: "cnpg" — one installation per cluster). Empty in the plugin-only posture (install_operator false): the operator on the cluster is someone else's, and this resource never claims a handle it does not own. |
| `status.outputs.barman_plugin_release_name` | `string` | Helm release name of the Barman Cloud plugin when enabled; empty otherwise. KubernetesPostgres backup blocks depend on this plugin being present. |

## References

Fields that can point at another resource's outputs:

| Field | Kind | Output |
|---|---|---|
| `spec.namespace` | KubernetesNamespace | `spec.name` |

## See Also

- [Overview](../README.md)
