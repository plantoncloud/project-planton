# CloudNativePG Operator

Installs CloudNativePG — the CNCF PostgreSQL operator — from the official Helm chart. The operator reconciles `Cluster` custom resources into highly available PostgreSQL: streaming replication, automated failover with a safe primary election, rolling updates, declarative roles and storage, and plugin-based backups. This component installs the ENGINE; the databases themselves are declared with KubernetesPostgres resources — one per PostgreSQL cluster. One installation per cluster: the CRDs are cluster-scoped and the webhook service name is baked into the webhook certificate, so the release name is fixed to `cnpg`.

## What Gets Created

When you deploy this Cloud Resource, the IaC module provisions:

- **Helm Release** (`cnpg`) -- the CloudNativePG operator Deployment, its mutating/validating webhooks, and its RBAC (ClusterRoles when cluster-wide, namespace-scoped when fenced)
- **CRDs** -- Cluster, ScheduledBackup, Backup, Pooler, Database, and companions — stamped `helm.sh/resource-policy: keep` unconditionally, so uninstalling the release never cascade-deletes the databases behind them
- **Barman Cloud plugin** (optional) -- a SECOND Helm release beside the operator (upstream forbids folding them into one) providing object-store backups for every database; its internal TLS is issued by cert-manager
- **Namespace** (optional) -- created with standard governance labels when `createNamespace` is true (`cnpg-system` is the upstream convention)

## Before You Deploy

### Planton Setup

- **Kubernetes Provider Connection** -- an active connection in the Connect module with credentials for the target cluster. Map it as the default for your environment, or specify it explicitly when creating the Cloud Resource.

### Kubernetes Cluster

- For the Barman Cloud backup plugin: **cert-manager on the cluster** (a deployed KubernetesCertManager) — the plugin's operator↔sidecar TLS certificates are cert-manager Certificates, and the install fails without it.
- For the operator PodMonitor: the Prometheus operator CRDs — the release fails to install without them.

## Deploy

### Console

Open the deployment store, find **CloudNativePG Operator**, and click **Deploy**. The creation wizard walks you through placement, the chart pin and CRD dial, operator runtime, the watch scope, operator configuration, the backup plugin, observability, image sourcing, and scheduling. Start from the **Standard** preset in the [Presets](#presets) tab.

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
  barmanCloudPlugin:
    enabled: true
  monitoring:
    podMonitorEnabled: true
  priorityClassName: system-cluster-critical
```

```shell
planton apply -f cnpg-operator.yaml
```

This creates the `cnpg-system` namespace, installs the operator release plus the Barman Cloud backup plugin, and enables the operator's PodMonitor — the backup-capable production posture. Declare databases with KubernetesPostgres resources afterwards. A Stack Job tracks the provisioning in real time.

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
  barmanCloudPlugin:
    enabled: true
```

The InfraPipeline creates the namespace first, then installs the operator into it in dependency order.

## Key Configuration

These are the most important decisions when configuring the operator. Explore the full field reference in the [API Explorer](#api-explorer) tab.

**One installation per cluster** -- the CRDs are cluster-scoped and the webhook service name is baked into the webhook certificate; a second installation would fight over both. The release name is fixed to `cnpg`. When CloudNativePG is ALREADY on the cluster (a self-hosted Planton installs one for its own database; `kubectl get deploy -A -l app.kubernetes.io/name=cloudnative-pg` tells), declare `installOperator: false` with the plugin enabled: this resource then installs only the Barman Cloud plugin beside the resident operator, and destroying it leaves the operator running (live-proven). A CloudNativePG uninstalled by a non-Helm owner can leave cluster-scoped CRDs, webhooks, and RBAC behind with that owner's labels; a full install then fails Helm's ownership check ("managed-by must equal Helm") until those leftovers are deleted.

**The chart pin governs** -- chart and operator versions move separately (chart `0.29.0` ships operator `1.30.0`). Pick versions from the served chart index; editing the pin later IS the upgrade.

**CRDs survive uninstall, unconditionally** -- the chart stamps `helm.sh/resource-policy: keep` on every CRD, so removing the release never takes the databases with it. This kind deliberately offers no dial to weaken that posture.

**The backup plugin is the backup path** -- CloudNativePG's built-in object-store support is deprecated upstream; the Barman Cloud plugin is what makes every KubernetesPostgres backup block function. It installs as its own release, requires cert-manager, and appears as a second release name in the outputs.

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
| `namespace` | Installation namespace | Debugging and composition |
| `release_name` | Operator Helm release name (always `cnpg`) | Debugging the release (`helm status`) |
| `barman_plugin_release_name` | Barman Cloud plugin release name when enabled; empty otherwise | Verifying the backup engine every KubernetesPostgres backup block depends on |

## Common Patterns

Browse the [Presets](#presets) tab for ready-to-deploy configurations.

**Standard** -- the operator alone, cluster-wide, pinned and prioritized for a production control plane. Databases run and fail over; their backup blocks wait for the plugin. Start from the **Standard** preset.

**With Backup Plugin** -- the operator plus the Barman Cloud plugin — the backup-capable posture for production database fleets (cert-manager required first). Start from the **With Backup Plugin** preset.

**Plugin only, beside a resident operator** -- `installOperator: false` plus the plugin: the shape for a cluster that already runs CloudNativePG (Planton self-hosted on GKE is exactly this) and needs its KubernetesPostgres backup blocks to work.

## Works With

- [**PostgreSQL**](/cloud-catalog/kubernetes-postgres) -- the databases this operator reconciles; each one composes against the CRDs installed here, and their backup blocks depend on the Barman plugin.
- [**Cert Manager**](/cloud-catalog/kubernetes-cert-manager) -- the Barman Cloud plugin's TLS issuer; deploy it before enabling the plugin.
- [**Kubernetes Namespace**](/cloud-catalog/kubernetes-namespace) -- the placement target (`cnpg-system` by convention), permanent while the CRDs are kept.
