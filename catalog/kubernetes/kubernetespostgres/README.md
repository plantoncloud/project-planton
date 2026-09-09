# Kubernetes Postgres

## When NOT to Use This

**The operator must already be on the cluster.** This component declares
a database; KubernetesCloudNativePgOperator installs the ENGINE that
reconciles it (with `barman_cloud_plugin.enabled` when backups are
declared). Deploy the operator first, databases after.

Also not the right component when:

- **You want the operator itself** — installing and configuring
  CloudNativePG (watch scope, reconcile concurrency, the backup plugin)
  is KubernetesCloudNativePgOperator; this component is one PostgreSQL
  cluster it manages.
- **You want external exposure baked in** — this component never creates
  a LoadBalancer or a route. The cluster is in-cluster plumbing
  reachable at the exported `kube_endpoint`; to reach it from outside,
  compose a first-class exposure kind (a KubernetesService of type
  LoadBalancer selecting the primary's pods, or a TCP route on a
  Gateway) against the exported service names.
- **You need a CloudNativePG surface the spec deliberately leaves out**
  — tablespaces, replica (distributed) clusters, the PgBouncer Pooler,
  LDAP authentication, image catalogs (see the research doc for the full
  list and reasons). Those are reachable today by declaring the raw
  custom resources through KubernetesManifest.

## Overview

**KubernetesPostgres** declares a production-grade PostgreSQL cluster
reconciled by CloudNativePG. The spec renders a `postgresql.cnpg.io/v1`
Cluster custom resource — and, when backups are declared, the companion
Barman Cloud `ObjectStore` and `ScheduledBackup` resources — so one
resource carries the whole database story: instances and streaming
replication with automated failover, storage (with a separate WAL volume
when I/O isolation matters), PostgreSQL configuration, bootstrap (fresh
initdb, restore from a backup, physical replication from an existing
server, or logical import), declarative roles, continuous WAL archiving
plus scheduled base backups to S3/GCS/Azure-Blob/S3-compatible stores,
TLS, and monitoring.

**The naming contract**: every object CloudNativePG creates derives from
`metadata.name` — instance pods (`<name>-1`, `<name>-2`, ...), the
traffic Services (`<name>-rw` primary read-write, `<name>-ro` replicas
only, `<name>-r` any instance), and the credential Secrets
(`<name>-app`, `<name>-superuser`). Applications connect through the
SERVICES, never a pod: after a failover the `-rw` Service re-points to
the new primary automatically.

**Key design points:**

- **Backups are plugin-based** — the backup block renders a Barman Cloud
  `ObjectStore` resource plus the Cluster's plugin wiring (WAL archiving
  starts immediately) and one `ScheduledBackup` per declared schedule.
  CloudNativePG's built-in object-store support is deprecated upstream
  and deliberately not modeled. The operator must be installed with
  `barman_cloud_plugin.enabled`.
- **Secrets never appear inline** — every declared credential (owner
  password, role passwords, superuser password, external-cluster
  passwords, object-store keys) materializes as a deterministic
  Kubernetes Secret; the rendered custom resources carry only
  secretKeyRef pointers. Keyless backup arms need no Secret at all.
- **Bootstrap is how the cluster is born** — exactly one method (initdb
  / recovery / pg_basebackup), immutable after creation; the operator
  ignores changes after the first reconcile.
- **Exposure is composed, never embedded** — no ingress block exists in
  the spec; external reachability is a separate first-class kind wired
  against the exported service names.

## Essential Configuration Fields

### Required

- **`spec.namespace`**: namespace for the cluster — literal or a
  KubernetesNamespace reference
- **`spec.storage.size`**: PGDATA volume size (one PVC per instance;
  sizes can only grow — the operator rejects shrinks)
- **`spec.bootstrap`**: exactly one of `initdb` (fresh empty database —
  the standard path), `recovery` (restore from an object-store backup —
  disaster recovery, cloning, PITR; `database`/`owner`/
  `owner_secret_name` carry the source's application credential into
  the recovered cluster), or `pg_basebackup` (physical
  streaming from a declared external cluster — same-major-version
  migration)

### Common

- **`spec.instances`**: one primary plus (instances − 1) streaming
  replicas; 1 for development, 2 for automated failover, 3 is the
  production convention (default 1)
- **`spec.image_name`**: PostgreSQL image, tag or digest form (e.g.
  `ghcr.io/cloudnative-pg/postgresql:17.5`); empty = the operator's
  default for its release
- **`spec.storage.storage_class`**: literal class name or a
  KubernetesStorageClass reference; `spec.wal_storage` adds a dedicated
  WAL volume (set at creation) for write-heavy I/O isolation
- **`spec.resources`**: per-instance CPU/memory — the operator derives
  PostgreSQL memory tuning hints from the limits
- **`spec.postgresql`**: `parameters` (postgresql.conf — merged over the
  operator's defaults; restart-requiring changes roll out
  automatically), `pg_hba` / `pg_ident` rules, `shared_preload_libraries`,
  `enable_alter_system`, and `synchronous` (quorum or priority
  synchronous replication — the zero-data-loss posture; `number` must be
  below the instance count)
- **`spec.bootstrap.initdb`**: database/owner names, optional declared
  `owner_password` (otherwise operator-generated), `data_checksums`
  (cannot be enabled later — set true for new production clusters),
  encoding/locale, post-init SQL, and `import` for logical migration
  from any reachable PostgreSQL (RDS, Cloud SQL, self-managed)
- **`spec.roles`**: declaratively managed roles — attributes, membership,
  connection limits, and (when a password is declared) a watched
  credential Secret whose rotation rotates the database password
- **`spec.superuser`**: disabled by default (upstream posture — password
  blanked); enable only when something genuinely needs superuser SQL
- **`spec.backup`**: the `object_store` (destination path + S3 / GCS /
  Azure-Blob backend with keyless XOR declared-key credentials, WAL and
  base-backup tuning), `retention_policy` (`30d` / `8w` / `6m`), and
  `schedules` — six-field cron (seconds first), `immediate` for
  protection from day one
- **`spec.workload_identity`**: keyless cloud identity for the instance
  pods' ServiceAccount — pair with the backup block's keyless arm (table
  below)
- **`spec.certificates`**: `server_tls_secret` (a KubernetesCertificate
  reference — the cert-manager seam) with `server_ca_secret`, or
  `server_alt_dns_names` on the operator-generated certificate; empty =
  the operator self-signs per cluster
- **`spec.monitoring`**: the per-instance Prometheus exporter (port
  9187) — TLS and default-query knobs
- **`spec.scheduling`**: `anti_affinity_type` (`preferred` default;
  `required` is the production posture — instances stay Pending unless
  separate nodes exist), `topology_key` (zones instead of nodes),
  node selector, tolerations, priority class
- **`spec.update_strategy`**: how rolling updates treat the primary —
  `unsupervised`/`supervised`, `restart`/`switchover`
- **`spec.enable_pdb`**: PodDisruptionBudget protecting the primary from
  voluntary eviction (default true)

## Environment Injection

How backups reach the object store keylessly, per host cloud: the
`workload_identity` arm annotates the cluster's ServiceAccount (the
identity the instance pods run as), and the backup backend's `keyless`
flag tells the Barman Cloud plugin to use that ambient identity.

| Environment | `workload_identity` arm | Backup backend arm | Declared-credential alternative |
|---|---|---|---|
| EKS / AWS S3 | `eks.role_arn` — the `eks.amazonaws.com/role-arn` annotation (IRSA) | `s3.keyless: true` | `s3.access_keys` — materialized as the `<name>-backup-creds` Secret |
| GKE / GCS | `gke.service_account_email` — the `iam.gke.io/gcp-service-account` annotation | `gcs.keyless: true` | `gcs.service_account_key_json` |
| AKS / Azure Blob | `aks.client_id` (+ optional `tenant_id`) — the `azure.workload.identity/*` annotations | `azure_blob.keyless: true` + `storage_account` (identifies the endpoint) | `connection_string` XOR `storage_account` + `storage_key` |
| S3-compatible (MinIO, R2, Ceph RGW, ...) | — | `s3.endpoint_url` + `access_keys` (keyless is spec-rejected: it only mints AWS credentials) | `endpoint_ca_pem` for self-signed endpoints |

The cloud-side half of each keyless contract (IRSA trust policy, GCP WI
binding, Entra federated credential) is written against the cluster's
own ServiceAccount — CloudNativePG names it after the cluster, in the
cluster's namespace — so it is one binding per cluster, a recovery
target included. On GKE the GCP service account needs
`roles/storage.objectAdmin` AND `roles/storage.legacyBucketReader` on
the bucket: Barman calls `storage.buckets.get` before every WAL archive
and `objectAdmin` alone 403s every archive (live-caught; the cluster
reports healthy while `ContinuousArchiving` stays false). The whole
GKE disaster-recovery resource set — identity, binding, bucket, operator
plugin, source cluster, recovery target — is laid out in
[GUIDE.md](GUIDE.md).

## Stack Outputs

| Output | Purpose |
|---|---|
| `namespace` | Namespace the cluster runs in |
| `cluster_name` | Name of the Cluster resource (equals `metadata.name`) — every derived object is prefixed with it |
| `rw_service` | Read-write Service (`<name>-rw`) — always points at the current primary; the Service applications write through |
| `ro_service` | Read-only Service (`<name>-ro`) — replicas only; the read-scaling handle |
| `r_service` | Any-instance read Service (`<name>-r`) — every ready instance including the primary |
| `kube_endpoint` | In-cluster endpoint of the read-write Service (`<name>-rw.<namespace>.svc.cluster.local:5432`) |
| `port_forward_command` | Port-forward command for workstation access when no exposure is composed |
| `username_secret` | `{name, key}` of the application user's name (the `<name>-app` Secret's `username` key) |
| `password_secret` | `{name, key}` of the application user's password — the same Secret also carries ready-made `uri` / `jdbc-uri` connection strings |
| `superuser_secret_name` | Superuser credential Secret (`<name>-superuser`) — populated only when superuser access is enabled |

## Composing in Infra Charts

- **`spec.namespace`** is a foreign key (default kind
  KubernetesNamespace, field path `spec.name`);
  **`storage.storage_class`** references a KubernetesStorageClass
  (`status.outputs.storage_class_name`);
  **`certificates.server_tls_secret`** references a
  KubernetesCertificate (`status.outputs.secret_name`) — the
  cert-manager seam; **`workload_identity`** arms reference AwsIamRole /
  GcpServiceAccount / AzureUserAssignedIdentity outputs.
- **Applications consume the outputs**: `kube_endpoint` as the
  connection host, `username_secret` / `password_secret` as env-from
  references — the credential rides the operator-managed Secret, never
  the manifest.
- **Exposure composes, never embeds**: a KubernetesService of type
  LoadBalancer (or a TCP route on a Gateway) targets the `rw_service`
  output; put externally reachable hostnames in
  `certificates.server_alt_dns_names` so the server certificate covers
  them.
- **The operator is a cluster prerequisite**, not a reference: deploy
  KubernetesCloudNativePgOperator (plugin enabled for backups) before
  any database.

## Examples

### Development (single instance, small)

```yaml
apiVersion: kubernetes.planton.dev/v1alpha1
kind: KubernetesPostgres
metadata:
  name: dev-db
spec:
  namespace:
    value: dev-db
  create_namespace: true
  instances: 1
  storage:
    size: 5Gi
  bootstrap:
    initdb:
      database: app
```

### Production (3 instances, synchronous replication, keyless S3 backups)

```yaml
apiVersion: kubernetes.planton.dev/v1alpha1
kind: KubernetesPostgres
metadata:
  name: orders-db
spec:
  namespace:
    value: orders
  create_namespace: true
  instances: 3
  storage:
    size: 100Gi
  wal_storage:
    size: 20Gi
  resources:
    requests:
      cpu: "1"
      memory: 2Gi
    limits:
      cpu: "2"
      memory: 4Gi
  postgresql:
    synchronous:
      method: any
      number: 1
  bootstrap:
    initdb:
      database: orders
      data_checksums: true
  backup:
    object_store:
      destination_path: s3://acme-pg-backups/orders-db
      s3:
        region: us-west-2
        keyless: true # IRSA — no stored keys
    retention_policy: 30d
    schedules:
      - name: daily
        schedule: "0 0 2 * * *" # six fields — seconds first
        immediate: true
  workload_identity:
    eks:
      role_arn:
        value: arn:aws:iam::111111111111:role/orders-db-backups
  scheduling:
    anti_affinity_type: required
```

### Clone / disaster recovery (restore from another cluster's backups)

```yaml
apiVersion: kubernetes.planton.dev/v1alpha1
kind: KubernetesPostgres
metadata:
  name: orders-db-clone
spec:
  namespace:
    value: orders-staging
  create_namespace: true
  instances: 1
  storage:
    size: 100Gi
  bootstrap:
    recovery:
      object_store:
        destination_path: s3://acme-pg-backups/orders-db # the SOURCE archive
        s3:
          region: us-west-2
          keyless: true
      source_server_name: orders-db
      recovery_target:
        target_time: "2026-07-20T06:00:00Z" # PITR — omit for full recovery
      # The recovered data carries the source's roles and passwords. Name
      # the source's application database and owner, and bring the source's
      # app Secret (kept alive by a KubernetesSecret / ExternalSecret) so
      # this cluster hands out credentials that actually work. Omit
      # owner_secret_name for a clone that gets a fresh password.
      database: orders
      owner: orders
      owner_secret_name: orders-db-app
  workload_identity:
    eks:
      role_arn:
        value: arn:aws:iam::111111111111:role/orders-db-backups
```

---

© Planton. Licensed under [Apache-2.0](https://github.com/plantonhq/planton/blob/main/LICENSE).
