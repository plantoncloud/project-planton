# CloudNativePG Operator

Installs CloudNativePG — the CNCF PostgreSQL operator — from the official Helm chart. The operator reconciles `Cluster` custom resources into highly available PostgreSQL: streaming replication, automated failover with a safe primary election, rolling updates, declarative roles and storage, and plugin-based backups. This component installs the ENGINE; the databases themselves are declared with KubernetesPostgres resources — one per PostgreSQL cluster. One installation per cluster: the CRDs are cluster-scoped and the webhook service name is baked into the webhook certificate, so the release name is fixed to `cnpg`.

## What Gets Created

When you deploy this Cloud Resource, the IaC module provisions:

- **Helm Release** (`cnpg`) -- the CloudNativePG operator Deployment, its mutating/validating webhooks, and its RBAC (ClusterRoles when cluster-wide, namespace-scoped when fenced)
- **CRDs** -- Cluster, ScheduledBackup, Backup, Pooler, Database, and companions — stamped `helm.sh/resource-policy: keep` unconditionally, so uninstalling the release never cascade-deletes the databases behind them
- **Namespace** (optional) -- created with standard governance labels when `createNamespace` is true (`cnpg-system` is the upstream convention)

## Before You Deploy

### Planton Setup

- **Kubernetes Provider Connection** -- an active connection in the Connect module with credentials for the target cluster. Map it as the default for your environment, or specify it explicitly when creating the Cloud Resource.

### Kubernetes Cluster

- For the operator PodMonitor: the Prometheus operator CRDs — the release fails to install without them.
- For backups: nothing here. Object-store backups are the Barman Cloud plugin's job, installed into this operator's namespace by a separate resource ([CNPG Barman Cloud Plugin](/cloud-catalog/kubernetes-cnpg-barman-cloud-plugin)), which needs cert-manager.

## Deploy

### Console

Open the deployment store, find **CloudNativePG Operator**, and click **Deploy**. The creation wizard walks you through placement, the chart pin and CRD dial, operator runtime, the watch scope, operator configuration, observability, image sourcing, and scheduling. Start from the **Standard** preset in the [Presets](#presets) tab.

### CLI

Create a manifest and apply it:

```yaml
apiVersion: kubernetes.planton.dev/v1alpha1
kind: KubernetesCloudNativePgOperator
metadata:
  name: cnpg
  org: acme-corp
  env: prod
spec:
  namespace:
    value: cnpg-system
  createNamespace: true
  chartVersion: "0.29.0"
  monitoring:
    podMonitorEnabled: true
  priorityClassName: system-cluster-critical
```

```shell
planton apply -f cnpg-operator.yaml
```

This creates the `cnpg-system` namespace, installs the operator release, and enables the operator's PodMonitor — the production control-plane posture. Declare databases with KubernetesPostgres resources afterwards, and a CNPG Barman Cloud Plugin beside the operator before any of them declares a backup. A Stack Job tracks the provisioning in real time.

### InfraChart

When deploying as part of a multi-resource environment, reference the namespace instead of hardcoding it:

```yaml
spec:
  namespace:
    valueFrom:
      kind: KubernetesNamespace
      name: cnpg-namespace
      fieldPath: spec.name
  createNamespace: false
```

The InfraPipeline creates the namespace first, then installs the operator into it in dependency order. A CNPG Barman Cloud Plugin node referencing this resource's `namespace` output follows it, and backup-declaring databases follow the plugin.

## Key Configuration

These are the most important decisions when configuring the operator. Explore the full field reference in the [API Explorer](#api-explorer) tab.

**One installation per cluster** -- the CRDs are cluster-scoped and the webhook service name is baked into the webhook certificate; a second installation would fight over both. The release name is fixed to `cnpg`. When CloudNativePG is ALREADY on the cluster (a self-hosted platform operator installs one for its own database; `kubectl get deploy -A -l app.kubernetes.io/name=cloudnative-pg` tells), do not declare this kind — declare only what the cluster is missing, usually the CNPG Barman Cloud Plugin with the resident operator's namespace. A CloudNativePG uninstalled by a non-Helm owner can leave cluster-scoped CRDs, webhooks, and RBAC behind with that owner's labels; an install then fails Helm's ownership check ("managed-by must equal Helm") until those leftovers are deleted.

**The chart pin governs** -- chart and operator versions move separately (chart `0.29.0` ships operator `1.30.0`). Pick versions from the served chart index; editing the pin later IS the upgrade.

**CRDs survive uninstall, unconditionally** -- the chart stamps `helm.sh/resource-policy: keep` on every CRD, so removing the release never takes the databases with it. This kind deliberately offers no dial to weaken that posture.

**Backups are a separate resource** -- CloudNativePG's built-in object-store support is deprecated upstream; the Barman Cloud plugin is what makes every KubernetesPostgres backup block function, and it is its own chart, pin, and dependency set. Declare a [CNPG Barman Cloud Plugin](/cloud-catalog/kubernetes-cnpg-barman-cloud-plugin) into this operator's namespace before the first database declares a backup; without it the operator parks that database in an unknown-plugin phase.

**Standbys are not throughput** -- extra operator replicas are leader-elected warm standbys that shorten the operator's own failover; `maxConcurrentReconciles` is the throughput dial for control planes managing many databases.

**The watch fence is silent on the outside** -- a fenced operator (`watch.clusterWide: false` plus a namespace list) never reconciles a database outside the fence, with no error anywhere. Cluster-wide is the normal posture.

**Keep the operator's priority above workloads** -- databases stop failing over without their operator; `system-cluster-critical` is the conventional choice.

**The escape hatch** -- `helmValues` carries additional chart values as a YAML document, merged LAST — never the substitute for typed fields, and never a place for secrets.

## Outputs and Dependencies

### What This Component Consumes

| Dependency | Field | ValueFromRef Path |
|------------|-------|-------------------|
| **KubernetesNamespace** | `namespace` | `spec.name` |

### What This Component Provides

After provisioning, `status.outputs` contains values that downstream Cloud Resources can consume via ValueFromRef:

| Output | Description | Common Downstream Use |
|--------|-------------|----------------------|
| `namespace` | Installation namespace | The CNPG Barman Cloud Plugin's `namespace` references it — the plugin must live where the operator does |
| `release_name` | Operator Helm release name (always `cnpg`) | Debugging the release (`helm status`) |

## Common Patterns

Browse the [Presets](#presets) tab for ready-to-deploy configurations.

**Standard** -- the operator alone, cluster-wide, pinned and prioritized for a production control plane. Databases run and fail over; their backup blocks need the CNPG Barman Cloud Plugin beside it. Start from the **Standard** preset.

**Backup-capable cluster** -- this operator plus a CNPG Barman Cloud Plugin referencing its namespace (cert-manager first): the production database-fleet posture, declared as two resources in one chart.

## Works With

- [**PostgreSQL**](/cloud-catalog/kubernetes-postgres) -- the databases this operator reconciles; each one composes against the CRDs installed here.
- [**CNPG Barman Cloud Plugin**](/cloud-catalog/kubernetes-cnpg-barman-cloud-plugin) -- the object-store backup engine, installed into this operator's namespace; every KubernetesPostgres backup block depends on it.
- [**Kubernetes Namespace**](/cloud-catalog/kubernetes-namespace) -- the placement target (`cnpg-system` by convention), permanent while the CRDs are kept.
