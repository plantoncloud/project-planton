# MongoDB

Deploys a production-grade MongoDB cluster reconciled by the Percona Operator for MongoDB. One resource carries the whole database story: replica sets with automated failover (a new primary is elected in seconds), optional sharding (mongos routers + config servers, every declared replica set becoming a shard), declarative users with operator-managed password Secrets, scheduled logical/physical/incremental backups with point-in-time recovery via Percona Backup for MongoDB, and TLS. The server is Percona Server for MongoDB — a fully MongoDB-compatible open-source distribution, so every driver, tool, and query works unchanged. The cluster is in-cluster plumbing by design — external exposure is composed from first-class exposure kinds, never embedded here.

## What Gets Created

When you deploy this Cloud Resource, the IaC module provisions:

- **PerconaServerMongoDB** — a `psmdb.percona.com/v1` custom resource named `metadata.name`. The operator derives every object from it: member pods (`{name}-{rs}-0..N`), the per-replica-set headless Services (`{name}-{rs}`), the mongos Service (`{name}-mongos`, sharding only), and the system-users Secret (`{name}-secrets`, operator-generated passwords for the built-in accounts)
- **Credential Secrets** — every declared password or key materializes as a deterministic Secret (`{name}-user-{username}` for declarative users, per-storage Secrets for backup-store credentials); the keyless S3 posture needs none
- **PerconaServerMongoDBRestore** (optional) — when `restore` is declared, a run named after the declaration that replays the named backup (and, with PITR, the archived oplog) into the cluster once its members are up
- **Namespace** (optional) — created with standard governance labels when `createNamespace` is true

Applications connect through the SERVICES, never a pod: for replica-set clusters, the driver's `replicaSet` parameter makes it follow failovers automatically.

## Before You Deploy

### Planton Setup

- **Kubernetes Provider Connection** — an active connection in the Connect module with credentials for the target cluster. Map it as the default for your environment, or specify it explicitly when creating the Cloud Resource.

### Kubernetes Cluster

- **Percona MongoDB operator** installed and running — deploy the **Percona Operator for MongoDB** component first. The operator must WATCH the database's namespace: its default posture watches only its own namespace, and a cluster declared outside the watched set is silently never reconciled. Install the operator beside the database, or widen its watch scope.
- **A storage class** capable of dynamic provisioning for the declared sizes; volume expansion support if you plan to grow storage in place.
- For keyless S3 backups: cloud-side ambient AWS identity on the member pods (EKS IRSA or node instance-profile credentials). GCS backups always need a service-account key — a `GcpServiceAccount` with a user-managed key, granted `roles/storage.objectAdmin` on the bucket, supplies it by reference.
- For organization-trusted TLS: cert-manager with a ClusterIssuer (or a namespaced Issuer in the database's namespace) referenced by `tls.issuer`; otherwise the operator self-generates certificates.

## Deploy

### Console

Open the deployment store, find **MongoDB**, and click **Deploy**. The creation wizard walks you through placement (with the operator-watch check), the server image, the replica-set topology, sharding, TLS, users, backup storages with their credential postures, backup schedules, point-in-time recovery, operations, and the explicit unsafe opt-ins. Start from the **Replica Set** preset in the [Presets](#presets) tab.

### CLI

Create a manifest and apply it:

```yaml
apiVersion: kubernetes.planton.dev/v1alpha1
kind: KubernetesMongodb
metadata:
  name: orders-db
  org: acme-corp
  env: prod
spec:
  namespace:
    value: databases
  createNamespace: true
  replicaSets:
    - name: rs0
      size: 3
      storage:
        size: 50Gi
  users:
    - name: orders-svc
      roles:
        - name: readWrite
          db: orders
  backup:
    storages:
      - name: s3-primary
        s3:
          bucket: acme-mongo-backups
          region: us-west-2
    tasks:
      - name: nightly
        schedule: "0 2 * * *"
        storageName: s3-primary
        keep: 14
    pitr:
      enabled: true
```

```shell
planton apply -f mongodb.yaml
```

This creates a three-member replica set with automated failover, a declared application user with an operator-managed password Secret, a nightly logical backup to S3 (keyless — the pods' ambient AWS identity) pruned after 14 runs, and continuous oplog archiving for point-in-time recovery. A Stack Job tracks the provisioning in real time.

### InfraChart

When deploying as part of a multi-resource environment, wire placement and storage to resources managed by other Cloud Resources:

```yaml
spec:
  namespace:
    valueFrom:
      kind: KubernetesNamespace
      name: databases-namespace
      fieldPath: spec.name
  replicaSets:
    - name: rs0
      size: 3
      storage:
        size: 50Gi
        storageClass:
          valueFrom:
            kind: KubernetesStorageClass
            name: fast-ssd
            fieldPath: status.outputs.storage_class_name
```

The InfraPipeline deploys the referenced resources first, then provisions the MongoDB cluster against them.

## Key Configuration

These are the most important decisions when configuring a MongoDB cluster. Explore the full field reference in the [API Explorer](#api-explorer) tab.

**Quorum decides availability** — failover is an election, and elections need a strict majority. 3 data-bearing members is the production shape (one loss still leaves 2 of 3 voting); even counts waste a vote — pair 2 data members with an arbiter (votes, holds no data) instead. Sizes below 3 are rejected by the operator unless `unsafe.replsetSize` explicitly opts in (development only).

**The image tag IS the MongoDB version** — there is no separate version field. Pin a tag like `percona/percona-server-mongodb:8.0.19-7` (MongoDB 8.0); changing it on a live cluster performs a SmartUpdate rolling upgrade — secondaries restart first, then the primary steps down, no manual failover.

**Storage only grows** — the operator provisions one PVC per member; size increases apply to live volumes, shrinks are rejected. WiredTiger sizes its cache from the container memory LIMIT, so resource limits affect correctness, not just cost.

**Shard only when one set no longer fits** — a single well-sized replica set carries most production workloads. With `sharding` enabled, every declared replica set becomes a shard, the config servers (3 members, a few Gi) track chunk placement, and clients connect through the mongos routers with no `replicaSet` parameter.

**Backups are a deliberate choice, not a default** — the omitted backup block means NO backups. Declare named storage destinations (S3/S3-compatible, GCS, or Azure Blob per destination), schedule tasks against them by name with FIVE-field cron expressions (`"0 2 * * *"` is daily at 02:00), and enable PITR to archive the oplog continuously so a restore can land BETWEEN backups. With multiple storages, exactly one carries `main: true` — PITR chunks and restore metadata land there. One `prefix` per cluster (PBM keeps its metadata under it); the source and its restore target deliberately share one, two live clusters never do. PITR starts archiving only after the first backup succeeds, and its recovery point is the last closed chunk (`oplogSpanMin`). Logical backups restore anywhere; physical backups are much faster at scale; incrementals build on an `incremental-base`.

**Credential posture per backend** — S3 on real AWS supports the keyless posture (omit `accessKeys`; the pods' IRSA or instance-profile identity signs). GCS does NOT: Percona Backup for MongoDB authenticates to Google Cloud Storage with a service-account key only, so every GCS storage declares `credentials` — the key itself (raw, or a `GcpServiceAccount`'s `key_base64` output wired by reference) or the name of a Secret already carrying it. Grant that account `roles/storage.objectAdmin` AND `roles/storage.legacyBucketReader` on the bucket, and rotate the key like a password (recreate the `GcpServiceAccount` key and re-apply). S3-COMPATIBLE endpoints (MinIO, Ceph RGW, GCS over its S3-interoperability endpoint with HMAC keys) always need declared keys — nothing ambient can mint their credentials. Every declared credential is stored as a managed-secret reference, never plaintext.

**Restore is declared on the cluster that receives the data** — `restore` names a backup (a Backup object in this namespace, or the backup's location inside one of the cluster's own declared storages when another cluster took it) and optionally a point in time (`latest`, or a `date`). The operator waits for the members and their PBM agents, syncs the store's metadata, then replays the backup into the running cluster — the module copies the storage definition into the restore object because the operator resolves a backup-source restore's storage from the restore itself during that sync. A restore that fails stays failed (`kubectl get psmdb-restore` shows the error; fix the cause and change the declaration to run again). Two rules make a cross-cluster restore work: the receiving cluster declares the SAME storage the backup lives in, and it points `systemUsersSecretName` at the source cluster's `{source}-secrets` — a restored database carries the source's users and passwords, and the operator must log in with them. Back that Secret up with the data.

**TLS is a ladder** — `disabled` → `allowTLS` → `preferTLS` (the upstream default) → `requireTLS` (the production posture). Each step is a rolling, non-destructive change. Point `tls.issuer` at a cert-manager (Cluster)Issuer for an organization-trusted chain; `disabled` requires the explicit `unsafe.tls` opt-in.

**Users are declarative** — the operator creates them, reconciles their roles, and manages their password Secrets (`{name}-user-{username}`); rotating the referenced secret rotates the database password. Leave a password empty and the operator generates one nobody ever sees.

**Exposure is composed, never embedded** — the cluster is reachable in-cluster at the exported `kube_endpoint`. The per-set `expose` block exists for the managed-cloud LoadBalancer recipes; to reach the database from outside, compose a first-class exposure kind.

## Outputs and Dependencies

### What This Component Consumes

| Dependency | Field | ValueFromRef Path |
|------------|-------|-------------------|
| **KubernetesNamespace** | `namespace` | `spec.name` |
| **KubernetesStorageClass** | `replicaSets[].storage.storageClass` / `sharding.configServer.storage.storageClass` | `status.outputs.storage_class_name` |
| **KubernetesClusterIssuer** | `tls.issuer` (or a namespaced KubernetesIssuer when `issuerKind: Issuer`) | `metadata.name` |

### What This Component Provides

After provisioning, `status.outputs` contains values that applications and downstream resources can consume:

| Output | Description | Common Downstream Use |
|--------|-------------|----------------------|
| `namespace` | Namespace the cluster runs in | Application deployment manifests |
| `cluster_name` | PerconaServerMongoDB name — every derived object is prefixed with it | Debugging (`kubectl get psmdb`) |
| `service` | The Service applications connect to — `{name}-mongos` when sharded, otherwise the first replica set's headless Service (`{name}-{rs}`) | The connection host |
| `kube_endpoint` | In-cluster endpoint (`{service}.{namespace}.svc.cluster.local:27017`) | Application connection strings in the same cluster |
| `replica_set` | The first replica set's name (the driver's `replicaSet` parameter) — empty for sharded clusters | `mongodb://user:pass@{endpoint}/?replicaSet={rs}` so the driver follows failovers |
| `port_forward_command` | Ready-to-run `kubectl port-forward` command | Local access when no exposure is composed |
| `admin_password_secret` | Secret key holding the database-admin password (the operator-managed `{name}-secrets` Secret, key `MONGODB_DATABASE_ADMIN_PASSWORD`; the paired username key is `MONGODB_DATABASE_ADMIN_USER`) | Break-glass administrative access via `secretKeyRef` |

## Common Patterns

Browse the [Presets](#presets) tab for ready-to-deploy configurations.

**Single instance (development)** — one member with the `unsafe.replsetSize` opt-in: no failover, no replication, right for a laptop namespace and never production. Start from the **Single Instance** preset.

**Production replica set** — three members with zone anti-affinity, a disruption budget, declared users, nightly backups, and PITR. Start from the **Replica Set** preset.

**GKE replica set with GCS backups and disaster recovery** — three members spread over three nodes, nightly logical backups plus PITR into a GCS bucket with the key wired by reference from a `GcpServiceAccount`, and a documented restore target that declares the same storage, a `restore` block, and the source's system-users Secret. Start from the **GKE Replica Set with GCS Backups** preset; the full resource set is in the guide.

**Sharded at scale** — enable `sharding`, declare multiple replica sets (each becomes a shard), size the config servers at 3 members, and keep 2+ mongos routers for query-path availability. Plan the shard key before sharding collections.

## Works With

- [**Percona Operator for MongoDB**](/cloud-catalog/kubernetes-percona-mongo-operator) — the prerequisite: reconciles the PerconaServerMongoDB resource and must watch the database's namespace
- [**Kubernetes Namespace**](/cloud-catalog/kubernetes-namespace) — the placement target
- [**Kubernetes StorageClass**](/cloud-catalog/kubernetes-storage-class) — backs the member and config-server volumes
- [**Cert Manager Cluster Issuer**](/cloud-catalog/kubernetes-cluster-issuer) — the cert-manager seam for organization-trusted TLS
- [**PostgreSQL**](/cloud-catalog/kubernetes-postgres) — the relational sibling in a typical polyglot data layer
