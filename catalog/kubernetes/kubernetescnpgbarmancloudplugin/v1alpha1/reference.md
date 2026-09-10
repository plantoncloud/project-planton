# KubernetesCnpgBarmanCloudPlugin

> Generated from the protobuf schema by `make generate-reference` -- do not
> edit by hand. To change a fact on this page, change the proto field comment
> or validation rule it is derived from, then regenerate.

**apiVersion**: `kubernetes.planton.dev/v1alpha1`

**Guide**: [GUIDE.md](../GUIDE.md) -- authored operational judgment for this component: conventions, trade-offs, and what pairs well with it.

**KubernetesCnpgBarmanCloudPluginSpec** installs the Barman Cloud plugin
for CloudNativePG from the official Helm chart (`plugin-barman-cloud` at
https://cloudnative-pg.github.io/charts). The plugin is the object-store
backup path for every PostgreSQL cluster the operator manages: WAL
archiving, scheduled base backups, and restores to S3, S3-compatible
stores, GCS, and Azure Blob. CloudNativePG's built-in object-store
support is deprecated upstream, so a KubernetesPostgres `backup` block
(or an object-store `bootstrap.recovery`) renders its Cluster against
THIS plugin and does nothing without it -- the operator parks such a
Cluster in the phase "Cluster cannot proceed to reconciliation due to an
unknown plugin being required" and never creates its instances.

ONE PLUGIN PER CLUSTER, IN THE OPERATOR'S NAMESPACE. CloudNativePG
discovers plugins through Services labeled `cnpg.io/pluginName` in its
OWN namespace only (a plugin anywhere else is invisible to it), and the
chart fixes that Service's name to `barman-cloud` because it is baked
into the plugin's TLS certificate. So the plugin is a per-namespace
singleton by construction, and the Helm release name is fixed to
"plugin-barman-cloud" -- a second declaration in the same namespace
would fight the first over those fixed-name objects.

WHO INSTALLS IT. Beside a CloudNativePG declared with the catalog's
KubernetesCloudNativePgOperator: this resource, with `namespace`
referencing that operator resource. Beside a CloudNativePG someone else
installed (Helm, GitOps, a platform operator that manages its own copy):
this resource with the operator's namespace as a literal -- unless that
installer also owns a plugin toggle, in which case enable it THERE; two
owners of the singleton collide.

REQUIRES cert-manager on the cluster (KubernetesCertManager): the chart
renders a self-signed Issuer and two Certificates for the operator <->
plugin gRPC TLS unconditionally, and the release fails to install without
it. Requires a CloudNativePG at 1.26 or later (1.27 or later strongly
recommended upstream for its plugin error reporting); the catalog
operator's default chart ships 1.30.0.

The typed fields below cover the chart's meaningful configuration
surface; `helm_values` remains as the escape hatch for chart values
beyond them (merged last, Helm `-f` semantics, identical on both
engines) -- a safety valve, never the primary interface.

## Example

```yaml
# Full-surface test manifest: exercises every typed arm of the spec so the
# offline plan proofs cover what the live lanes may not. Not a realistic
# production shape -- see presets for those.
#
# The namespace is a literal here (the offline plan/preview input has no
# operator resource to resolve against); the install scenario proves the
# reference form live.
apiVersion: kubernetes.planton.dev/v1alpha1
kind: KubernetesCnpgBarmanCloudPlugin
metadata:
  name: hack-barman-plugin
spec:
  namespace:
    value: hack-cnpg-system
  createNamespace: false
  chartVersion: "0.7.0"
  crds:
    install: true
  replicas: 2
  resources:
    requests:
      cpu: 50m
      memory: 64Mi
    limits:
      cpu: 200m
      memory: 256Mi
  image:
    repository: mirror.example.com/cloudnative-pg/plugin-barman-cloud
    tag: v0.13.0
  sidecarImage:
    repository: mirror.example.com/cloudnative-pg/plugin-barman-cloud-sidecar
    tag: v0.13.0
  imagePullSecrets:
    - hack-registry-credentials
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
  helmValues: |
    additionalArgs:
      - --log-level=info
    service:
      name: renamed-by-mistake
```

## Spec Fields

| Path | Type | Required | Default | References |
|---|---|---|---|---|
| `spec.namespace` | `string \| valueFrom` | yes |  | KubernetesCloudNativePgOperator (`status.outputs.namespace`) |
| `spec.createNamespace` | `bool` |  |  |  |
| `spec.chartVersion` | `string` |  | `0.7.0` |  |
| `spec.crds` | `KubernetesCnpgBarmanCloudPluginCrds` |  |  |  |
| `spec.crds.install` | `bool` |  | `true` |  |
| `spec.replicas` | `int32` |  | `1` |  |
| `spec.resources` | `ContainerResources` |  |  |  |
| `spec.resources.limits` | `CpuMemory` |  |  |  |
| `spec.resources.limits.cpu` | `string` |  |  |  |
| `spec.resources.limits.memory` | `string` |  |  |  |
| `spec.resources.requests` | `CpuMemory` |  |  |  |
| `spec.resources.requests.cpu` | `string` |  |  |  |
| `spec.resources.requests.memory` | `string` |  |  |  |
| `spec.image` | `KubernetesCnpgBarmanCloudPluginImage` |  |  |  |
| `spec.image.repository` | `string` |  |  |  |
| `spec.image.tag` | `string` |  |  |  |
| `spec.sidecarImage` | `KubernetesCnpgBarmanCloudPluginImage` |  |  |  |
| `spec.sidecarImage.repository` | `string` |  |  |  |
| `spec.sidecarImage.tag` | `string` |  |  |  |
| `spec.imagePullSecrets` | `[]string` |  |  |  |
| `spec.priorityClassName` | `string` |  |  |  |
| `spec.nodeSelector` | `map<string, string>` |  |  |  |
| `spec.tolerations` | `[]WorkloadToleration` |  |  |  |
| `spec.tolerations[].key` | `string` |  |  |  |
| `spec.tolerations[].operator` | `string` |  |  |  |
| `spec.tolerations[].value` | `string` |  |  |  |
| `spec.tolerations[].effect` | `string` |  |  |  |
| `spec.tolerations[].tolerationSeconds` | `int64` |  |  |  |
| `spec.helmValues` | `string` |  |  |  |

## Field Details

### spec.namespace

`string | valueFrom` · required

Namespace to install the plugin into. This MUST be the namespace the
CloudNativePG operator runs in: the operator only considers plugin
Services in its own namespace, so a plugin elsewhere is never
discovered and every backup-declaring database stays unreconciled.
Reference the operator resource (a bare `valueFrom` with the resource
name resolves to its namespace output), or pass the literal namespace
of a CloudNativePG someone else installed ("cnpg-system" is the
upstream convention).

- references: KubernetesCloudNativePgOperator (`status.outputs.namespace`)
- rule: {"required":true}
- rule: write as {value: <literal>} or {valueFrom: {kind: KubernetesCloudNativePgOperator, name: <that resource's name>, fieldPath: status.outputs.namespace}} -- a bare string does not parse

### spec.createNamespace

`bool`

When true, the namespace is created (with the standard Planton
governance labels) before installing and deleted with the resource.
Almost always false here: the namespace is the OPERATOR's and already
exists (created by the operator resource or by whoever installed the
resident CloudNativePG). True is only correct when this resource is
the first thing to claim the namespace -- and then deleting the plugin
deletes the operator's namespace with it.

### spec.chartVersion

`string` · optional (explicit presence)

Helm chart version to install (e.g. "0.7.0", which ships plugin
v0.13.0 -- chart and app versions move separately; the chart pin
governs). Pin deliberately; upgrades re-run the release with the new
chart. Pick versions from the chart repository's index
(`helm search repo`): the served chart is the contract. Plugin
v0.13.0 requires CloudNativePG 1.26 or later and upstream strongly
recommends 1.27 or later (it reports plugin errors on the Cluster's
status instead of only in logs); check the plugin's release notes
when moving the pin against an older operator.

- default: `0.7.0`

### spec.crds

`KubernetesCnpgBarmanCloudPluginCrds`

The plugin's custom resource definition (`ObjectStore`, the object
store a KubernetesPostgres backup block points at) lifecycle.

### spec.crds.install

`bool` · optional (explicit presence)

Install the `ObjectStore` CRD with the release. Chart default: true.
Disable only when something else manages it. The chart stamps the CRD
with `helm.sh/resource-policy: keep`, so uninstalling the release
NEVER deletes the ObjectStore resources databases point at (and the
backup configuration they carry) -- the upstream safety posture, kept
as-is. A later install with the SAME release name and namespace adopts
the kept CRD; that adoption is one more reason the release name is
fixed.

- default: `true`

### spec.replicas

`int32` · optional (explicit presence)

Plugin replica count. Chart default: 1. The chart deploys with the
Recreate strategy (the plugin does not support rolling updates yet),
and extra replicas are leader-elected standbys that shorten failover of
the PLUGIN itself -- they add no backup throughput.

- default: `1`
- rule: {"int32":{"gte":1}}

### spec.resources

`ContainerResources`

Plugin container resources. Empty = no requests/limits (the chart
ships none by default). The plugin process itself is light; the
backup work runs in a sidecar the plugin injects into each PostgreSQL
instance pod, sized by the database's own resources.

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

### spec.image

`KubernetesCnpgBarmanCloudPluginImage`

Override the plugin image (registry mirrors, air-gapped clusters).
Empty = the chart default (ghcr.io/cloudnative-pg/plugin-barman-cloud
at the chart's app version).

### spec.image.repository

`string`

Image repository including its registry (e.g.
"my-mirror.example.com/cloudnative-pg/plugin-barman-cloud").

### spec.image.tag

`string`

Image tag. Empty = the chart's app version.

### spec.sidecarImage

`KubernetesCnpgBarmanCloudPluginImage`

Override the SIDECAR image -- the container the plugin injects into
every PostgreSQL instance pod to run the actual archiving and backup
commands. Empty = the chart default
(ghcr.io/cloudnative-pg/plugin-barman-cloud-sidecar at the chart's
app version). Mirror BOTH images for an air-gapped cluster: the
database pods pull this one, not the plugin's.

### spec.sidecarImage.repository

`string`

Image repository including its registry (e.g.
"my-mirror.example.com/cloudnative-pg/plugin-barman-cloud").

### spec.sidecarImage.tag

`string`

Image tag. Empty = the chart's app version.

### spec.imagePullSecrets

`[]string`

Names of image-pull secrets (in the installation namespace) for
pulling the plugin image from a private mirror. The sidecar image is
pulled by the DATABASE pods in their own namespaces -- give those
clusters their own pull secrets (KubernetesPostgres) when the sidecar
is mirrored too.

### spec.priorityClassName

`string`

PriorityClass for the plugin pod. Backups and WAL archiving stop
silently-for-a-while when the plugin is evicted (the databases keep
running, the archive falls behind) -- keep it at the operator's
priority.

### spec.nodeSelector

`map<string, string>`

Node selector for the plugin pod.

### spec.tolerations

`[]WorkloadToleration`

Tolerations for the plugin pod.

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

### spec.helmValues

`string`

Escape hatch: additional chart values as a YAML document, merged LAST
over everything the typed fields render (Helm `-f` semantics,
identical on both engines). For the chart surface beyond the typed
fields (additionalArgs, additionalEnv, update strategy, security
contexts, topology spread, host network, certificate durations, ...)
-- never the substitute for them. The chart's fixed identities
(`service.name`, the name overrides) are re-pinned after this merge:
the Service name is baked into the plugin's TLS certificate and the
release name is what a re-install adopts the kept CRD through, so
neither is configurable here. Do not put secrets here.

## Outputs

Reference an output from another manifest as `valueFrom: {kind: KubernetesCnpgBarmanCloudPlugin, name: <resource-name>, fieldPath: status.outputs.<output>}`.

| Output | Type | Description |
|---|---|---|
| `status.outputs.namespace` | `string` | Namespace the plugin runs in -- the CloudNativePG operator's namespace, by requirement. |
| `status.outputs.release_name` | `string` | Helm release name of the plugin (fixed: "plugin-barman-cloud" -- one plugin per operator namespace). |
| `status.outputs.plugin_name` | `string` | The CNPG-I plugin identifier a Cluster names in its `plugins` list (fixed: "barman-cloud.cloudnative-pg.io"). KubernetesPostgres renders it whenever a backup block or an object-store recovery is declared. |

## References

Fields that can point at another resource's outputs:

| Field | Kind | Output |
|---|---|---|
| `spec.namespace` | KubernetesCloudNativePgOperator | `status.outputs.namespace` |

## See Also

- [Overview](../README.md)
