# Kubernetes MongoDB

## When NOT to Use This

**The operator must already be on the cluster.** This component
declares a database cluster; KubernetesPerconaMongoOperator installs
the ENGINE that reconciles it. The default operator posture watches its
OWN namespace — install the operator in the database's namespace, or
widen its watch. Deploy the operator first, databases after.

Also not the right component when:

- **You want the operator itself** — installing and configuring the
  Percona Operator for MongoDB (watch scope, reconcile concurrency,
  telemetry) is KubernetesPerconaMongoOperator; this component is one
  MongoDB cluster it manages.
- **You want a managed cloud MongoDB** — use a vendor-operated service
  such as MongoDB Atlas; this component is for
  running MongoDB ON the Kubernetes cluster itself.
- **You want a single throwaway pod** — a replica set, an operator,
  and per-member PVCs are the wrong tool for a scratch database that
  lives for an afternoon; run a plain mongo container as an ordinary
  workload instead.
- **You want external exposure baked in** — this component never
  creates a LoadBalancer or a route. The cluster is in-cluster
  plumbing reachable at the exported `kube_endpoint`; to reach it from
  outside, compose a first-class exposure kind against the exported
  service name. (The per-set and mongos `expose` type/annotations
  knobs exist for the managed-cloud LoadBalancer recipes.)
- **You need an operator surface the spec deliberately leaves out** —
  multi-cluster deployments, split horizons, hidden and non-voting
  members, external nodes, Vault integration, hook scripts, sidecar
  containers, custom MongoDB roles (see the research doc). Those are
  reachable today by declaring the raw custom resource through
  KubernetesManifest.

## Overview

**KubernetesMongodb** declares one production-grade MongoDB cluster
reconciled by the Percona Operator for MongoDB. The spec renders a
`psmdb.percona.com/v1` PerconaServerMongoDB custom resource — so one
resource carries the whole database story: replica sets with automated
failover (a new primary is elected in seconds when the current one
dies), optional sharding (mongos routers + config servers, each
declared replica set becoming a shard), scheduled
logical/physical/incremental backups with point-in-time recovery via
Percona Backup for MongoDB, TLS, and declarative users.

**The server is Percona Server for MongoDB** — a fully
MongoDB-compatible open-source distribution (every driver, tool, and
query works unchanged) with enterprise-grade features under an open
license.

**Topology**: `replica_sets` declares the data-bearing sets. Without
sharding, exactly one replica set — 3 members is the production shape
(automated failover needs a majority); 1 is a development posture that
requires `unsafe.replset_size`. With `sharding` enabled, EVERY declared
replica set becomes a shard behind the mongos routers, and clients
connect to mongos.

**The naming contract**: every object the operator creates derives from
`metadata.name` — member pods (`<name>-<rs>-0..N`), the per-replica-set
headless Services (`<name>-<rs>`), the mongos Service (`<name>-mongos`,
sharding only), and the system-users Secret (`<name>-secrets`,
operator-generated passwords for the built-in accounts — key
`MONGODB_DATABASE_ADMIN_PASSWORD` is the admin password). Drivers
discover every member through the headless Service; connect with
`?replicaSet=<rs>` so the driver follows failovers.

**Key design points:**

- **TLS is preferTLS by default** — operator-generated certificates
  out of the box; point `tls.issuer` at a cert-manager (Cluster)Issuer
  for an organization-trusted chain, and move to `requireTLS` for
  production. Disabling TLS entirely REQUIRES `unsafe.tls`.
- **Users are declarative** — the operator creates them, keeps their
  roles reconciled, and manages their password Secrets
  (`<name>-user-<username>`); rotating the Secret rotates the database
  password. Secrets never appear inline in the rendered resource.
- **Backups are PBM + PITR** — named storages (S3 or any S3-compatible
  store via `endpoint_url`, Cloudflare R2 in its own terms by reference to
  the catalog's Cloudflare kinds, GCS, Azure Blob; declared credentials
  materialize as `<name>-backup-<storage>` Secrets; the keyless posture
  exists for real S3 only — GCS always needs a service-account key,
  which a `GcpServiceAccount` can supply by reference, and R2 always
  needs its token's key pair, which a `CloudflareAccountApiToken`
  supplies by reference), five-field cron
  tasks with retention and logical/physical/incremental types, and
  point-in-time recovery archiving oplog chunks to the main storage.
- **Restore is a declaration, not a runbook** — `spec.restore` names a
  backup (a same-namespace Backup object, or a location in one of the
  cluster's declared storages for a backup another cluster took) and an
  optional point in time; the cluster comes up carrying the data. A
  restored database carries the SOURCE cluster's users and passwords,
  so a cluster restoring another's backup points
  `system_users_secret_name` at the source's `<source>-secrets` — back
  that Secret up with the data. A restore that fails stays failed
  (`kubectl get psmdb-restore` shows why; change the declaration to run
  again). The full GKE disaster-recovery resource set — keyed identity,
  bucket with its two roles, operator, source, restore target — is laid
  out in [GUIDE.md](GUIDE.md); it was proven live on both engines.
- **The version is the image** — `image_name` chooses the MongoDB
  version (e.g. `percona/percona-server-mongodb:8.0.19-7` — MongoDB
  8.0); changing it on a live cluster performs a SmartUpdate rolling
  upgrade (the operator orders restarts safely).
- **The log-collector sidecar is off unless declared** — the operator
  runs the fluent-bit sidecar shipping mongod logs only when the
  `log_collector` block is present.
- **Exposure is composed, never embedded** — no ingress block exists
  in the spec.

## Essential Configuration Fields

### Required

- **`spec.namespace`**: namespace for the cluster — literal or a
  KubernetesNamespace reference; the operator must watch it
- **`spec.replica_sets`**: at least one replica set, each with a
  `name` (`rs0` is the upstream convention) and required
  `storage.size` (one PVC per member; grows are applied in place —
  shrinks are rejected)

### Common

- **`replica_sets[].size`**: data-bearing members — 3 is the
  production shape (a majority survives one loss), even numbers waste
  a vote (add the `arbiter` instead), 1 is development-only and
  requires `unsafe.replset_size` (default 3)
- **`spec.image_name`**: the Percona Server for MongoDB image, tag
  form; empty = the module's default for the pinned operator
- **`replica_sets[].resources`**: CPU/memory for every member pod —
  WiredTiger sizes its cache from the memory limit
- **`replica_sets[].mongod_config`**: extra mongod configuration
  merged over the operator's defaults (mongod.conf YAML shape) —
  replication and security essentials are operator-managed
- **`replica_sets[].pod_disruption_budget`**: one of
  `max_unavailable` / `min_available` — the upstream default allows at
  most one member down to voluntary disruptions
- **`replica_sets[].scheduling`**: `anti_affinity_topology_key`
  (`kubernetes.io/hostname` upstream default — one member per node;
  `topology.kubernetes.io/zone` to spread across zones), node
  selector, tolerations, priority class
- **`spec.sharding`**: `enabled` plus required `config_server`
  (metadata replica set, 3 members + small storage) and `mongos`
  (query routers, 3 by default; fewer than 2 requires
  `unsafe.mongos_size`); every declared replica set becomes a shard
- **`spec.tls`**: `mode` (`preferTLS` default; `requireTLS` for
  production; `disabled` requires `unsafe.tls`), `issuer` /
  `issuer_kind` (the cert-manager seam), `cert_validity_duration`
- **`spec.users`**: declarative application users — auth `db`
  (default `admin`), `roles` (name + db), and a password (empty =
  operator-generated into the same `<name>-user-<username>` Secret)
- **`spec.backup`**: named `storages` (S3/S3-compatible, Cloudflare R2,
  GCS, Azure Blob — the first or the one marked `main` receives PITR oplog
  chunks; GCS requires `credentials` — a `service_account_key`, raw or
  a GcpServiceAccount's `key_base64` output, or an
  `existing_secret_name`), `tasks` (five-field cron, `storage_name`,
  `type` logical/physical/incremental/incremental-base, `keep`
  retention), and `pitr` (continuous oplog archiving between backups)
- **`spec.restore`**: `backup_name` (a Backup object here) or
  `backup_source` (`storage_name` + the backup's `destination` path +
  `type`), optional `pitr` (`latest`, or `date` with
  `YYYY-MM-DD HH:MM:SS`), optional `replset_remapping`; requires
  `spec.backup` with the storage the backup lives in — the module copies
  that storage's definition into the restore object, because the operator
  resolves a backup-source restore's storage from the restore itself when
  it syncs the store's metadata into a fresh cluster
- **`spec.system_users_secret_name`**: bring your own system-users
  Secret (the restore-from-another-cluster requirement); empty = the
  operator generates `<name>-secrets`
- **`spec.update_strategy`**: `SmartUpdate` (default) /
  `RollingUpdate` / `OnDelete`
- **`spec.log_collector`**: declare it to turn on the fluent-bit
  sidecar shipping mongod logs (omitted = disabled)
- **`spec.unsafe`**: explicit opt-in to postures the operator
  otherwise rejects — `replset_size`, `mongos_size`, `tls`,
  `backup_if_unhealthy`; development only
- **`spec.pause`**: scale everything to zero, keep the volumes

## Stack Outputs

| Output | Purpose |
|---|---|
| `namespace` | Namespace the cluster runs in |
| `cluster_name` | Name of the PerconaServerMongoDB resource (equals `metadata.name`) — every derived object is prefixed with it |
| `service` | The Service applications connect to — `<name>-mongos` when sharding is enabled, otherwise the first replica set's headless Service (`<name>-<rs>`) |
| `kube_endpoint` | In-cluster endpoint (`<service>.<namespace>.svc.cluster.local:27017`) — for replica-set clusters connect with `?replicaSet=<rs>` so the driver follows failovers |
| `replica_set` | The first replica set's name (the driver's `replicaSet` parameter) — empty for sharded clusters (mongos needs none) |
| `port_forward_command` | Port-forward command for workstation access when no exposure is composed |
| `admin_password_secret` | `{name, key}` of the database-admin password — the operator-managed `<name>-secrets` Secret, key `MONGODB_DATABASE_ADMIN_PASSWORD` (the paired username key is `MONGODB_DATABASE_ADMIN_USER`) |

## Composing in Infra Charts

- **`spec.namespace`** is a foreign key (default kind
  KubernetesNamespace, field path `spec.name`);
  **`storage.storage_class`** references a KubernetesStorageClass
  (`status.outputs.storage_class_name`); **`tls.issuer`** references a
  KubernetesClusterIssuer (`metadata.name`) — the cert-manager seam.
- **Applications consume the outputs**: `kube_endpoint` as the
  connection host (plus the `replica_set` output as the driver's
  `replicaSet` parameter), `admin_password_secret` (or a declared
  user's `<name>-user-<username>` Secret) as env-from references — the
  credential rides the operator-managed Secret, never the manifest.
- **Exposure composes, never embeds**: a first-class exposure kind
  targets the `service` output; the per-set and mongos `expose`
  type/annotations knobs exist for the managed-cloud LoadBalancer
  recipes.
- **The operator is a cluster prerequisite**, not a reference: deploy
  KubernetesPerconaMongoOperator first — in the database's namespace,
  or with its watch widened to cover it.

## Examples

### Development (single member)

```yaml
apiVersion: kubernetes.planton.dev/v1alpha1
kind: KubernetesMongodb
metadata:
  name: dev-db
spec:
  namespace:
    value: percona-mongo # where the operator watches
  replica_sets:
    - name: rs0
      size: 1
      storage:
        size: 5Gi
  unsafe:
    replset_size: true # 1 member has no failover — development only
```

### Production (3-member replica set, declared user, S3 backups + PITR)

```yaml
apiVersion: kubernetes.planton.dev/v1alpha1
kind: KubernetesMongodb
metadata:
  name: orders-db
spec:
  namespace:
    value: percona-mongo
  replica_sets:
    - name: rs0
      size: 3
      storage:
        size: 100Gi
      resources:
        requests:
          cpu: "1"
          memory: 2Gi
        limits:
          cpu: "2"
          memory: 4Gi
      pod_disruption_budget:
        max_unavailable: 1
      scheduling:
        anti_affinity_topology_key: topology.kubernetes.io/zone
  tls:
    mode: requireTLS
  users:
    - name: app
      roles:
        - name: readWrite
          db: orders
  backup:
    storages:
      - name: primary
        s3:
          bucket: acme-mongo-backups
          region: us-west-2
          prefix: orders-db
          access_keys:
            access_key_id: AKIAEXAMPLE
            secret_access_key: <backup-user-secret-key>
    tasks:
      - name: nightly
        schedule: "0 2 * * *" # five-field cron
        storage_name: primary
        keep: 14
    pitr:
      enabled: true # oplog chunks land on the main storage
```

### Sharded (two shards behind mongos)

```yaml
apiVersion: kubernetes.planton.dev/v1alpha1
kind: KubernetesMongodb
metadata:
  name: events-db
spec:
  namespace:
    value: percona-mongo
  replica_sets: # every declared set becomes a shard
    - name: rs0
      size: 3
      storage:
        size: 200Gi
    - name: rs1
      size: 3
      storage:
        size: 200Gi
  sharding:
    enabled: true
    config_server:
      size: 3
      storage:
        size: 5Gi # metadata is small but precious
    mongos:
      size: 3
```

---

© Planton. Licensed under [Apache-2.0](https://github.com/plantonhq/planton/blob/main/LICENSE).
