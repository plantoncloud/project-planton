# PostgreSQL

Deploys a production-grade PostgreSQL cluster reconciled by CloudNativePG — the CNCF PostgreSQL operator. One resource carries the whole database story: instances with streaming replication and automated failover, storage with an optional dedicated WAL volume, PostgreSQL server configuration, bootstrap (fresh initdb, restore from a backup, physical replication from an existing server, or logical import from RDS/Cloud SQL/anything reachable), declarative roles, continuous WAL archiving plus scheduled base backups to S3/GCS/Azure-Blob/Cloudflare-R2/S3-compatible stores, TLS, and monitoring. The cluster is in-cluster plumbing by design — external exposure is composed from first-class exposure kinds, never embedded here.

## What Gets Created

When you deploy this Cloud Resource, the IaC module provisions:

- **CloudNativePG Cluster** — a `postgresql.cnpg.io/v1` Cluster custom resource. The operator derives every object from `metadata.name`: instance pods (`{name}-1`, `{name}-2`, ...), the traffic Services (`{name}-rw` primary read-write, `{name}-ro` replicas only, `{name}-r` any instance), one PVC per instance (plus a WAL PVC when declared), and the credential Secrets (`{name}-app`, and `{name}-superuser` when superuser access is enabled)
- **Barman Cloud ObjectStore** (when backups are declared) — the object-store descriptor WAL archiving and base backups land in; recovery bootstraps render a second one (`{name}-recovery-source`) to read from
- **ScheduledBackup resources** (one per declared schedule) — the periodic base backups that make point-in-time recovery real
- **Namespace** (optional) — created with standard governance labels when `createNamespace` is true

Applications connect through the SERVICES, never a pod: after a failover the `-rw` Service re-points to the new primary automatically.

## Before You Deploy

### Planton Setup

- **Kubernetes Provider Connection** — an active connection in the Connect module with credentials for the target cluster. Map it as the default for your environment, or specify it explicitly when creating the Cloud Resource.

### Kubernetes Cluster

- **CloudNativePG operator** installed and running — deploy the **Kubernetes CloudNativePG Operator** component first. For backups, the operator must be installed with its Barman Cloud plugin enabled (CloudNativePG's built-in object-store support is deprecated upstream and deliberately not modeled here).
- **A storage class** capable of dynamic provisioning for the declared sizes; volume expansion support if you plan to grow storage in place.
- For keyless backups: cloud-side identity (an IRSA role, GKE Workload Identity binding, or AKS federated credential) written against the instance pods' ServiceAccount — wired through the `workloadIdentity` field.
- For organization-trusted TLS: cert-manager and a **Cert Manager Certificate** resource to reference; otherwise the operator self-signs per cluster.

## Deploy

### Console

Open the deployment store, find **PostgreSQL**, and click **Deploy**. The creation wizard walks you through placement, instances and storage, how the cluster is born (bootstrap), server configuration, roles, backups with their credential posture, superuser access, TLS, monitoring, and scheduling. Start from the **Production HA** preset in the [Presets](#presets) tab.

### CLI

Create a manifest and apply it:

```yaml
apiVersion: kubernetes.planton.dev/v1alpha1
kind: KubernetesPostgres
metadata:
  name: app-database
  org: acme-corp
  env: prod
spec:
  namespace:
    value: databases
  createNamespace: true
  instances: 3
  storage:
    size: 50Gi
  postgresql:
    synchronous:
      method: any
      number: 1
  backup:
    objectStore:
      destinationPath: s3://acme-db-backups/prod/app-database
      s3:
        region: us-west-2
        keyless: true
    retentionPolicy: 30d
    schedules:
      - name: nightly
        schedule: "0 0 2 * * *"
        immediate: true
```

```shell
planton apply -f postgres.yaml
```

This creates a three-instance cluster (one primary, two streaming replicas) with quorum synchronous replication, continuous WAL archiving to S3, and a nightly base backup pruned after 30 days. A Stack Job tracks the provisioning in real time.

### InfraChart

When deploying as part of a multi-resource environment, wire placement, storage, and TLS to resources managed by other Cloud Resources:

```yaml
spec:
  namespace:
    valueFrom:
      kind: KubernetesNamespace
      name: databases-namespace
      fieldPath: spec.name
  storage:
    size: 50Gi
    storageClass:
      valueFrom:
        kind: KubernetesStorageClass
        name: fast-ssd
        fieldPath: status.outputs.storage_class_name
```

The InfraPipeline deploys the referenced resources first, then provisions the PostgreSQL cluster against them.

## Key Configuration

These are the most important decisions when configuring a PostgreSQL cluster. Explore the full field reference in the [API Explorer](#api-explorer) tab.

**Instances and failover** — `instances: 1` is a single point of failure suitable for development; 2 gives automated failover; 3 is the production convention (failover capacity even during maintenance). Add quorum synchronous replication (`postgresql.synchronous`) for the zero-data-loss posture — transactions wait for standbys before committing, and the `dataDurability` dial decides whether writes block (`required`) or the requirement relaxes (`preferred`) when standbys are unavailable.

**Storage only grows** — the operator provisions one PVC per instance and never lets sizes shrink. A dedicated `walStorage` volume puts the sequential WAL writes on their own disk — the standard I/O-isolation move for write-heavy workloads, and a creation-time decision (adding it later re-creates the instances).

**Bootstrap is how the cluster is born — and immutable after creation.** Fresh initdb is the standard path (with optional data checksums, which cannot be turned on later). Recovery restores from an object-store backup, with point-in-time targets — the disaster-recovery and clone path; because the restored data carries the source's roles and passwords, a recovery names the source's application database and owner and brings the source's app Secret (`ownerSecretName`) so the credential it hands out is the one the data accepts — back that Secret up with the archive. pg_basebackup streams physically from an existing same-major-version server. initdb's `import` block does a logical migration from anything reachable (RDS, Cloud SQL, self-managed) declared as an external cluster.

**Backups are a deliberate choice, not a default** — the backup block starts WAL archiving immediately and each schedule renders a ScheduledBackup; at least one schedule is what makes point-in-time recovery real. Schedules are SIX-field cron expressions (seconds first — `"0 0 2 * * *"` is daily at 02:00); set `immediate: true` on one of them, because WAL alone restores nothing without a base backup to replay onto. One `destinationPath` per cluster, forever: Barman refuses to archive into a path that already holds another cluster's WAL (a cluster recreated under the same path, or a recovered cluster writing back to the path it restored from), and the failure is quiet — the cluster reports healthy while its `ContinuousArchiving` condition stays false and no backup ever lands. Check `kubectl get cluster <name> -o jsonpath='{.status.conditions}'` after the first deploy.

**Credential posture per backend** — each backend offers keyless (IRSA / GKE Workload Identity / AKS Workload Identity, paired with the `workloadIdentity` field) or declared keys stored as managed secrets. The keyless identity is bound to the ServiceAccount the operator names after the cluster (`<name>` in the cluster's namespace) — one cloud-side binding per cluster, a recovery target included. On GKE the GCP service account needs TWO bucket roles, `roles/storage.objectAdmin` AND `roles/storage.legacyBucketReader`: Barman calls `storage.buckets.get` before every WAL archive, and `objectAdmin` alone fails every archive with a 403 (live-caught). Cloudflare R2 has its own arm (`r2`), declared in R2's terms — the bucket's account and jurisdiction and a Cloudflare API token, each by reference to the catalog's `CloudflareR2Bucket` and `CloudflareAccountApiToken` — and the module composes the jurisdiction's endpoint, region `auto`, the token as an S3 key pair, and the plugin sidecar's `when_required` checksum posture (boto3 otherwise attaches data-integrity checksums an S3-compatible store may reject); R2 has no keyless posture from any cluster. A token scoped to the single bucket with `Workers R2 Storage Bucket Item Write` is enough for archiving, base backups, and recovery (live-proven). Cloudflare refuses to delete a bucket that still holds objects and the provider has no force-destroy, so a retired archive bucket is emptied over the S3 API before its `CloudflareR2Bucket` is destroyed. Other S3-compatible endpoints (MinIO, Ceph RGW, DigitalOcean Spaces) use the `endpointUrl` override and authenticate with access keys — the keyless posture only mints AWS credentials.

**Superuser stays off** — the upstream default blanks the `postgres` password and everything runs through the application owner role. Enable it only when something genuinely needs superuser SQL, and the operator maintains the `{name}-superuser` Secret.

**TLS by composition** — by default the operator self-signs a CA and server certificate per cluster. Point `certificates.serverTlsSecret` at a **Cert Manager Certificate** to serve an organization-trusted chain (the CA Secret rides along in `serverCaSecret`).

**Exposure is composed, never embedded** — the cluster is reachable in-cluster at the exported `kube_endpoint`. To reach it from outside, compose a first-class exposure kind; this component never creates one.

**Updates on your terms** — replicas always update first; `updateStrategy` decides whether the primary restarts in place or a replica is promoted first (`switchover`), and whether the operator proceeds automatically or waits for a manual promotion (`supervised` — change-window control).

## Outputs and Dependencies

### What This Component Consumes

| Field | References | Purpose |
|-------|-----------|---------|
| `spec.namespace` | KubernetesNamespace (`spec.name`) | The namespace the cluster runs in |
| `spec.storage.storageClass` / `spec.walStorage.storageClass` | KubernetesStorageClass (`status.outputs.storage_class_name`) | The storage class backing the data / WAL PVCs |
| `spec.certificates.serverTlsSecret` | KubernetesCertificate (`status.outputs.secret_name`) | A cert-manager-issued server certificate |

### What This Component Provides

After provisioning, `status.outputs` contains values that applications and downstream resources can consume:

| Output | Description | Common Downstream Use |
|--------|-------------|----------------------|
| `namespace` | Namespace the cluster runs in | Application deployment manifests |
| `cluster_name` | Cluster resource name — every derived object is prefixed with it | Debugging (`kubectl cnpg status`) |
| `rw_service` | Read-write Service (`{name}-rw`) — always points at the current primary | The Service applications write through |
| `ro_service` | Read-only Service (`{name}-ro`) — replicas only | Read scaling for reporting/analytics traffic |
| `r_service` | Any-instance Service (`{name}-r`) — every ready instance | Reads that tolerate the primary |
| `kube_endpoint` | In-cluster endpoint (`{name}-rw.{namespace}.svc.cluster.local:5432`) | Application connection strings in the same cluster |
| `port_forward_command` | Ready-to-run `kubectl port-forward` command | Local access when no exposure is composed |
| `username_secret` | Secret key holding the application user's name (`{name}-app` Secret, `username` key) | Application pod env via `secretKeyRef` |
| `password_secret` | Secret key holding the application user's password — the same Secret also carries ready-made `uri` / `jdbc-uri` connection strings | Application pod env via `secretKeyRef` |
| `superuser_secret_name` | The `{name}-superuser` Secret — populated only when superuser access is enabled | Break-glass administrative access |

## Common Patterns

Browse the [Presets](#presets) tab for ready-to-deploy configurations.

**Dev single instance** — the smallest useful cluster: one instance, small storage, a fresh `app` database with an operator-generated password, no backups. Start from the **Dev Single Instance** preset.

**Production HA on EKS** — three instances with quorum synchronous replication (zero data loss on failover), a dedicated WAL volume, hard anti-affinity, and continuous backups landing keylessly in S3 via IRSA, pruned after 30 days. Start from the **Production HA** preset.

**Production HA on GKE with disaster recovery** — three instances under hard anti-affinity (the autoscaler adds a node per instance), continuous backups landing keylessly in GCS through Workload Identity, and a documented recovery target that bootstraps from the archive with the source's credentials. Start from the **GKE Production HA with GCS Backups** preset; the full resource set (identity, binding, bucket, operator plugin, source, recovery target) is in the guide.

**Production HA on GKE with Cloudflare R2 backups** — the same HA posture with the archive outside Google: an R2 bucket and an API token declared from the catalog, the database referencing both, nothing S3-shaped typed. Start from the **GKE Production HA with Cloudflare R2 Backups** preset; the resource set (bucket, token, operator plugin, source, recovery target) is in the guide.

**S3-compatible backups** — a highly available cluster whose backups land in an S3-compatible store (in-cluster MinIO, Ceph RGW, DigitalOcean Spaces) via the `endpointUrl` override with declared access keys. Start from the **S3-Compatible Backups** preset.

## Works With

- **Kubernetes CloudNativePG Operator** — the prerequisite: reconciles the Cluster resource; its Barman Cloud plugin powers the backup story.
- **Kubernetes Namespace** — the placement target.
- **Kubernetes StorageClass** — backs the data and WAL volumes.
- **Cert Manager Certificate** — the cert-manager seam for organization-trusted server TLS.
- **Kubernetes Valkey** — the cache beside the database in a typical application stack.
