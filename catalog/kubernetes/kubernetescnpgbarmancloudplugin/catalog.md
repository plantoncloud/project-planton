# CNPG Barman Cloud Plugin

Installs the Barman Cloud plugin for CloudNativePG from the official Helm chart — the object-store backup engine every KubernetesPostgres backup block and object-store recovery runs through (WAL archiving, scheduled base backups, restores to S3, S3-compatible stores, GCS, and Azure Blob). CloudNativePG's built-in object-store support is deprecated upstream; this plugin is the backup path. It installs into the operator's namespace — the operator only discovers plugins there — as one fixed-name release, `plugin-barman-cloud`, one per operator.

## What Gets Created

When you deploy this Cloud Resource, the IaC module provisions:

- **Helm Release** (`plugin-barman-cloud`) -- the plugin Deployment, its ServiceAccount and RBAC, the fixed-name gRPC Service `barman-cloud` labeled `cnpg.io/pluginName` (how the operator finds it), and the config ConfigMap carrying the sidecar image the plugin injects into every PostgreSQL instance pod
- **cert-manager Issuer and Certificates** -- a self-signed Issuer plus server and client Certificates for the operator↔plugin gRPC TLS, rendered by the chart unconditionally; cert-manager must be on the cluster
- **CRD** -- ObjectStore (`objectstores.barmancloud.cnpg.io`), stamped `helm.sh/resource-policy: keep`, so uninstalling the plugin never deletes the ObjectStore resources databases point at
- **Namespace** (optional) -- created only when `createNamespace` is true; normally false, because the namespace is the operator's and already exists

## Before You Deploy

### Planton Setup

- **Kubernetes Provider Connection** -- an active connection in the Connect module with credentials for the target cluster. Map it as the default for your environment, or specify it explicitly when creating the Cloud Resource.

### Kubernetes Cluster

- **CloudNativePG on the cluster** -- a deployed [CloudNativePG Operator](/cloud-catalog/kubernetes-cloud-native-pg-operator) (reference it), or a resident operator installed by another hand (name its namespace). Plugin v0.13.0 needs CloudNativePG 1.26 or later; 1.27 or later is strongly recommended upstream.
- **cert-manager on the cluster** -- a deployed [Cert Manager](/cloud-catalog/kubernetes-cert-manager); the chart's Certificates never issue without it and the install rolls back.
- **Not already installed by the operator's own installer** -- a self-hosted platform operator that offers a plugin toggle owns the plugin; enable it there instead of declaring this kind.

## Deploy

### Console

Open the deployment store, find **CNPG Barman Cloud Plugin**, and click **Deploy**. The creation wizard walks you through placement (the operator's namespace), the chart pin and CRD dial, plugin runtime, image sourcing for the plugin and its sidecar, and scheduling. Start from the **Standard** preset in the [Presets](#presets) tab.

### CLI

Create a manifest and apply it:

```yaml
apiVersion: kubernetes.planton.dev/v1alpha1
kind: KubernetesCnpgBarmanCloudPlugin
metadata:
  name: cnpg-barman-plugin
  org: acme-corp
  env: prod
spec:
  namespace:
    value: cnpg-system
  chartVersion: "0.7.0"
  priorityClassName: system-cluster-critical
```

```shell
planton apply -f cnpg-barman-plugin.yaml
```

This installs the plugin into `cnpg-system` beside the CloudNativePG operator already running there, registers it with the operator, and establishes the ObjectStore CRD — from then on every KubernetesPostgres backup block on the cluster works. A Stack Job tracks the provisioning in real time.

### InfraChart

When deploying as part of a multi-resource environment, reference the operator instead of hardcoding its namespace:

```yaml
spec:
  namespace:
    valueFrom:
      kind: KubernetesCloudNativePgOperator
      name: cnpg
      fieldPath: status.outputs.namespace
```

The InfraPipeline installs the operator first, then the plugin into its namespace, then any database whose backup block depends on it — the reference is the ordering edge.

## Key Configuration

These are the most important decisions when configuring the plugin. Explore the full field reference in the [API Explorer](#api-explorer) tab.

**The namespace is the operator's, full stop** -- CloudNativePG discovers plugins only in its own namespace. `namespace` defaults its reference onto the operator resource's namespace output; a literal names a resident operator's namespace (`kubectl get deploy -A -l app.kubernetes.io/name=cloudnative-pg`). Anywhere else, the plugin runs and nothing ever finds it.

**One plugin per operator** -- the chart fixes the Service name (baked into the TLS certificate), the TLS Secrets, and the config ConfigMap; the release name is fixed to `plugin-barman-cloud`. Two installers collide. Whoever installed CloudNativePG should install its plugin: if that installer offers a toggle, use it and skip this kind.

**Install it before the first backup block** -- a KubernetesPostgres declaring `backup` or an object-store `bootstrap.recovery` on a cluster without the plugin is parked by the operator in the phase "Cluster cannot proceed to reconciliation due to an unknown plugin being required" — visible in `kubectl get cluster`, invisible to an apply waiting on Ready. In a chart, give backup-declaring databases a `dependsOn` edge to this resource.

**The chart pin governs** -- chart and plugin versions move separately (chart `0.7.0` ships plugin `v0.13.0`). Pick versions from the served chart index; editing the pin later IS the upgrade. Keep the operator at 1.26 or later (1.27 or later recommended).

**The ObjectStore CRD survives uninstall** -- the chart stamps `helm.sh/resource-policy: keep` on it, so removing the plugin never takes the ObjectStore resources (and the backup configuration they carry) with it. A re-install with the same release name adopts the kept CRD.

**Two images for air-gapped clusters** -- `image` is the plugin pod's; `sidecarImage` is what every PostgreSQL instance pod pulls (published through the plugin's config ConfigMap). Mirror both; the sidecar's pull secrets belong on the databases.

**Keep the plugin's priority with the operator's** -- while the plugin is evicted, WAL archiving falls behind on every database; `system-cluster-critical` is the conventional choice.

**The escape hatch** -- `helmValues` carries additional chart values as a YAML document, merged over the typed fields — never the substitute for them, and never a place for secrets. The fixed identities (`service.name`, the name overrides) are re-pinned after it.

## Outputs and Dependencies

### What This Component Consumes

| Dependency | Field | ValueFromRef Path |
|------------|-------|-------------------|
| **KubernetesCloudNativePgOperator** | `namespace` | `status.outputs.namespace` |

### What This Component Provides

After provisioning, `status.outputs` contains values that downstream Cloud Resources can consume via ValueFromRef:

| Output | Description | Common Downstream Use |
|--------|-------------|----------------------|
| `namespace` | The operator's namespace the plugin runs in | Debugging and composition |
| `release_name` | Plugin Helm release name (always `plugin-barman-cloud`) | Debugging the release (`helm status`) |
| `plugin_name` | The CNPG-I identifier a Cluster's `plugins` list names (always `barman-cloud.cloudnative-pg.io`) | Reading a Cluster's plugin wiring against the fact rather than a remembered string |

## Common Patterns

Browse the [Presets](#presets) tab for ready-to-deploy configurations.

**Standard** -- the plugin beside the catalog's operator, `namespace` by reference, prioritized with the operator. Start from the **Standard** preset.

**Beside a resident operator** -- the plugin beside a CloudNativePG someone else installed without a plugin, `namespace` as the literal operator namespace — the shape of a self-hosted platform's management cluster whose installer offers no plugin toggle. Start from the **Beside Resident Operator** preset.

## Works With

- [**CloudNativePG Operator**](/cloud-catalog/kubernetes-cloud-native-pg-operator) -- the engine the plugin registers with; reference it for the namespace.
- [**Cert Manager**](/cloud-catalog/kubernetes-cert-manager) -- the chart's TLS issuer; deploy it first.
- [**PostgreSQL**](/cloud-catalog/kubernetes-postgres) -- whose backup blocks and object-store recoveries run through this plugin.
