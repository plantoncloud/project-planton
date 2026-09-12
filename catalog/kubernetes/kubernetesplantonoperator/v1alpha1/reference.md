# KubernetesPlantonOperator

> Generated from the protobuf schema by `make generate-reference` -- do not
> edit by hand. To change a fact on this page, change the proto field comment
> or validation rule it is derived from, then regenerate.

**apiVersion**: `kubernetes.planton.dev/v1alpha1`

**Guide**: [GUIDE.md](../GUIDE.md) -- authored operational judgment for this component: conventions, trade-offs, and what pairs well with it.

**KubernetesPlantonOperatorSpec** installs the Planton operator — the
lifecycle manager that reconciles a `PlantonPlatform` declaration into a
running self-hosted Planton (control plane, console, identity server,
databases, in-cluster runner, secrets manager) — from the official
`planton-operator` Helm chart (OCI, ghcr.io/plantonhq/charts) as a real
Helm release, byte-identical to a hand-installed one.

This component installs the MANAGER only. Installing the operator alone
deploys NO platform — the operator never auto-creates a PlantonPlatform;
each platform is declared with a KubernetesPlantonPlatform resource,
which the operator reconciles. One operator serves EVERY platform on the
cluster (it watches all namespaces).

ONE OPERATOR PER CLUSTER: the operator enforces this itself at startup —
it scans for sibling operator Deployments (matched by the chart's
`app.kubernetes.io/name: planton-operator` + `control-plane:
controller-manager` labels) and refuses to start beside one, naming the
remedy in its log. Adding more PLATFORMS to a cluster that already runs
the operator is normal and needs no second operator — declare more
KubernetesPlantonPlatform resources instead. The Helm release name is
fixed to "planton-operator" for the same singleton reason.

THE CHART OWNS ITS DEFINITIONS: the `PlantonPlatform` and
`PlantonIdentityProvider` CRDs are ordinary resources of the release,
rendered from the operator's own source, so a `chart_version` upgrade
carries the matching schema with the operator, and an uninstall keeps
them by default (`crds.keep_on_uninstall`) — destroying this resource
never cascade-deletes PlantonPlatform declarations or the platforms
behind them. The modules map the two `crds` dials onto the chart's own
values and apply nothing else: no second copy of the schema exists
anywhere.

DESTROY: removing the operator leaves every platform RUNNING but
unmanaged — spec edits stop taking effect until an operator returns
and adopts them. Platform deletion still completes without the
operator (teardown is Kubernetes garbage collection of the
declaration's owner-referenced objects), so the ordering is
operational hygiene, not a hard constraint. The kept definitions are
what make all of this safe: declarations survive the operator's absence.

## Example

```yaml
# Full-surface hack manifest for the offline plan/preview proofs — every
# typed field exercised with realistic values. The operator itself needs
# almost nothing: it is a single small manager pod whose real work is
# reconciling PlantonPlatform declarations (deployed separately as
# KubernetesPlantonPlatform resources).
apiVersion: kubernetes.planton.dev/v1alpha1
kind: KubernetesPlantonOperator
metadata:
  name: planton-operator
spec:
  namespace:
    value: planton-operator
  create_namespace: true
  chart_version: 0.15.0
  crds:
    install: true
    keep_on_uninstall: true
  replicas: 2
  leader_election: true
  resources:
    requests:
      cpu: 10m
      memory: 256Mi
    limits:
      cpu: 500m
      memory: 512Mi
  service_account:
    annotations:
      example.com/team: platform
  common_labels:
    team: platform
  pod_annotations:
    example.com/inject: "false"
  node_selector:
    kubernetes.io/os: linux
  tolerations:
    - key: platform
      operator: Exists
      effect: NoSchedule
  image_pull_secrets:
    - mirror-pull
  image:
    repository: ghcr.io/plantonhq/planton/operator
    tag: v0.15.0
```

## Spec Fields

| Path | Type | Required | Default | References |
|---|---|---|---|---|
| `spec.namespace` | `string \| valueFrom` | yes |  | KubernetesNamespace (`spec.name`) |
| `spec.createNamespace` | `bool` |  |  |  |
| `spec.chartVersion` | `string` |  | `0.15.0` |  |
| `spec.replicas` | `int32` |  | `1` |  |
| `spec.leaderElection` | `bool` |  | `true` |  |
| `spec.resources` | `ContainerResources` |  |  |  |
| `spec.resources.limits` | `CpuMemory` |  |  |  |
| `spec.resources.limits.cpu` | `string` |  |  |  |
| `spec.resources.limits.memory` | `string` |  |  |  |
| `spec.resources.requests` | `CpuMemory` |  |  |  |
| `spec.resources.requests.cpu` | `string` |  |  |  |
| `spec.resources.requests.memory` | `string` |  |  |  |
| `spec.serviceAccount` | `KubernetesPlantonOperatorServiceAccount` |  |  |  |
| `spec.serviceAccount.create` | `bool` |  | `true` |  |
| `spec.serviceAccount.name` | `string` |  |  |  |
| `spec.serviceAccount.annotations` | `map<string, string>` |  |  |  |
| `spec.commonLabels` | `map<string, string>` |  |  |  |
| `spec.podAnnotations` | `map<string, string>` |  |  |  |
| `spec.nodeSelector` | `map<string, string>` |  |  |  |
| `spec.tolerations` | `[]WorkloadToleration` |  |  |  |
| `spec.tolerations[].key` | `string` |  |  |  |
| `spec.tolerations[].operator` | `string` |  |  |  |
| `spec.tolerations[].value` | `string` |  |  |  |
| `spec.tolerations[].effect` | `string` |  |  |  |
| `spec.tolerations[].tolerationSeconds` | `int64` |  |  |  |
| `spec.imagePullSecrets` | `[]string` |  |  |  |
| `spec.image` | `KubernetesPlantonOperatorImage` |  |  |  |
| `spec.image.repository` | `string` |  |  |  |
| `spec.image.tag` | `string` |  |  |  |
| `spec.helmValues` | `string` |  |  |  |
| `spec.crds` | `KubernetesPlantonOperatorCrds` |  |  |  |
| `spec.crds.install` | `bool` |  | `true` |  |
| `spec.crds.keepOnUninstall` | `bool` |  | `true` |  |

## Field Details

### spec.namespace

`string | valueFrom` · required

The namespace to install the operator into ("planton-operator" is the
convention). Accepts a literal namespace name or a reference to a
KubernetesNamespace resource. This is only the manager's home —
platforms live in their own namespaces.

- references: KubernetesNamespace (`spec.name`)
- rule: {"required":true}
- rule: write as {value: <literal>} or {valueFrom: {kind: KubernetesNamespace, name: <that resource's name>, fieldPath: spec.name}} -- a bare string does not parse

### spec.createNamespace

`bool`

When true, the namespace is created (with the standard Planton
governance labels) before the release is installed, and deleted with
the resource. When false, the namespace must already exist.

### spec.chartVersion

`string` · optional (explicit presence)

The planton-operator chart version to install (e.g. "0.15.0"). The chart
and the operator image share one version line: chart 0.15.0 runs
operator v0.15.0. Defaults to the version this catalog release was
validated against; pin a different version only for change control.
Versions must exist as published charts at oci://ghcr.io/plantonhq/charts.
Charts older than 0.8.0 do not own their definitions and are refused
at plan time: the `crds` dials would have nothing to act on.

- default: `0.15.0`
- rule: chart version must be an exact semver like "0.15.0" — ranges are not reproducible

### spec.replicas

`int32` · optional (explicit presence)

Operator replica count. Chart default: 1. Extra replicas are
leader-elected warm standbys that shorten failover of the OPERATOR
itself — they add no reconciliation throughput.

- default: `1`
- rule: {"int32":{"gte":1}}

### spec.leaderElection

`bool` · optional (explicit presence)

Leader election for the operator's controller manager. Chart default:
true. Required whenever replicas > 1; disable only on
single-replica resource-constrained clusters where the lease traffic
matters.

- default: `true`

### spec.resources

`ContainerResources`

Operator container resources. Empty = the chart's defaults (requests
10m/256Mi, limits 500m/512Mi) — comfortable for the reconcile loops of
several platforms; the heavy lifting happens in the platforms' own
workloads, never in the operator.

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

### spec.serviceAccount

`KubernetesPlantonOperatorServiceAccount`

The operator's Kubernetes ServiceAccount. Empty = the chart creates
one named after the release. The operator talks only to the Kubernetes
API — it holds no cloud credentials, so annotations are rarely needed
here (cloud identity for platform workloads is configured on the
KubernetesPlantonPlatform resource instead).

### spec.serviceAccount.create

`bool` · optional (explicit presence)

Create the ServiceAccount with the release. Chart default: true. Set
false to bring your own (name it below).

- default: `true`

### spec.serviceAccount.name

`string`

ServiceAccount name. Empty = derived from the release name.

### spec.serviceAccount.annotations

`map<string, string>`

Annotations on the ServiceAccount.

### spec.commonLabels

`map<string, string>`

Extra labels merged into every resource the chart renders — fleet
conventions, cost attribution, policy selectors.

### spec.podAnnotations

`map<string, string>`

Annotations added to the operator pod.

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

`KubernetesPlantonOperatorImage`

Override the operator image (registry mirrors, air-gapped clusters).
Empty = the chart default (ghcr.io/plantonhq/planton/operator at the
chart's app version). The mirror must be digest-identical to the
official image — the operator and the definitions the chart renders
are one build.

### spec.image.repository

`string`

Image repository (e.g. "my-mirror.example.com/planton/operator").

### spec.image.tag

`string`

Image tag. Empty = the chart's app version.

### spec.helmValues

`string`

Escape hatch: additional chart values as a YAML document, merged LAST
over everything the typed fields render (Helm `-f` semantics,
identical on both engines). For the chart surface beyond the typed
fields (affinity, health-probe port, image pull policy) — never the
substitute for them. `nameOverride`/`fullnameOverride` are
deliberately not honored knobs: renaming the Deployment takes it out
of the operator's own one-per-cluster startup guard's view. Do not
put secrets here.

### spec.crds

`KubernetesPlantonOperatorCrds`

CRD lifecycle. The chart owns its two definitions as release resources;
these dials map onto the chart's `crds.enabled` and `crds.keep` values.

### spec.crds.install

`bool` · optional (explicit presence)

Install the `PlantonPlatform` and `PlantonIdentityProvider` definitions
with the release. Default TRUE. Set false ONLY when another release or
a GitOps tool already owns them on this cluster: with the definitions
absent the operator cannot start, so this is a bring-your-own-CRDs arm,
never a lighter install.

- default: `true`

### spec.crds.keepOnUninstall

`bool` · optional (explicit presence)

Keep the definitions (and therefore every PlantonPlatform declaration
and every platform behind it) when the resource is destroyed. Default
TRUE: deleting them cascades to every self-hosted Planton on the
cluster, a destructive act that must be an explicit false. A later
install under the same release adopts kept definitions.

- default: `true`

## Outputs

Reference an output from another manifest as `valueFrom: {kind: KubernetesPlantonOperator, name: <resource-name>, fieldPath: status.outputs.<output>}`.

| Output | Type | Description |
|---|---|---|
| `status.outputs.namespace` | `string` | Namespace the operator runs in. |
| `status.outputs.release_name` | `string` | Helm release name of the operator (fixed: "planton-operator" — one installation per cluster, enforced by the operator's own startup guard). |

## References

Fields that can point at another resource's outputs:

| Field | Kind | Output |
|---|---|---|
| `spec.namespace` | KubernetesNamespace | `spec.name` |

## See Also

- [Overview](../README.md)
