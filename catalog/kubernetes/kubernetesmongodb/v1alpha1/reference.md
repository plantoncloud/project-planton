# KubernetesMongodb

> Generated from the protobuf schema by `make generate-reference` -- do not
> edit by hand. To change a fact on this page, change the proto field comment
> or validation rule it is derived from, then regenerate.

**apiVersion**: `kubernetes.planton.dev/v1alpha1`

**Guide**: [GUIDE.md](../GUIDE.md) -- authored operational judgment for this component: conventions, trade-offs, and what pairs well with it.

**KubernetesMongodbSpec** declares a production-grade MongoDB cluster
reconciled by the Percona Operator for MongoDB
(KubernetesPerconaMongoOperator must be on the cluster). The spec
renders a `psmdb.percona.com/v1` PerconaServerMongoDB custom
resource: replica sets with automated failover (a new primary is
elected in seconds when the current one dies), optional SHARDING
(mongos routers + config servers, each declared replica set becoming
a shard), scheduled logical/physical/incremental backups with
point-in-time recovery via Percona Backup for MongoDB, TLS, and
declarative users.

The server is Percona Server for MongoDB — a fully MongoDB-compatible
open-source distribution (every driver, tool, and query works
unchanged) with enterprise-grade features under an open license.

TOPOLOGY: `replica_sets` declares the data-bearing sets. Without
sharding, exactly one replica set (3 members is the production
shape). With `sharding` enabled, EVERY declared replica set becomes a
shard behind the mongos routers, and clients connect to mongos.

NAMING CONTRACT: every object derives from `metadata.name` — pods
(`<name>-<rs>-0..N`), the per-replica-set headless Services
(`<name>-<rs>`), the mongos Service (`<name>-mongos`, sharding only),
and the system-users Secret (`<name>-secrets`, operator-generated
passwords for the built-in accounts — unless `system_users_secret_name`
points at one you bring).

DISASTER RECOVERY IS TWO DECLARATIONS: `backup` on the cluster that
writes (storages + scheduled tasks + PITR oplog archiving), and
`restore` on the cluster that reads — a fresh cluster declares the
same storage, names the backup to restore from, and comes up carrying
the data. The restored data includes the SOURCE cluster's users and
their passwords, so the restore target must also reference the
source's system-users Secret (`system_users_secret_name`); back that
Secret up alongside the data or the operator cannot log in to what it
restored.

EXPOSURE IS COMPOSED, never embedded: the cluster is in-cluster
plumbing reachable at the exported `kube_endpoint`. The per-set
`expose` block exists for the managed-cloud LoadBalancer recipes and
cross-cluster topologies, documented per environment.

DELIBERATELY NOT MODELED (reachable via the operator's own upstream
surface where noted, recorded in the research doc): multi-cluster
deployments, split horizons, hidden and non-voting members, external
nodes, Vault integration, hook scripts, sidecar containers, custom
MongoDB roles.

## Example

```yaml
# Full-surface manifest for offline module proofs (tofu validate/plan and
# pulumi preview). Exercises every typed block coherently: a sharded
# cluster (two shards, config servers, mongos routers with an exposed
# Service), per-set tuned mongod config, an arbiter, cert-manager TLS,
# declarative users with roles and declared passwords, PBM backups to an
# S3-compatible (MinIO-style) store with declared keys plus a keyless GCS
# arm, a schedule and PITR, hard anti-affinity with tolerations, PDBs, the
# log-collector sidecar, and image pull secrets. Both engines must render
# identical CRs + credential Secrets from it.
apiVersion: kubernetes.planton.dev/v1alpha1
kind: KubernetesMongodb
metadata:
  name: test-mongodb
spec:
  namespace:
    value: test-mongodb-ns
  create_namespace: true
  image_name: percona/percona-server-mongodb:8.0.19-7
  replica_sets:
    - name: rs0
      size: 3
      storage:
        size: 20Gi
        storage_class:
          value: fast-ssd
      resources:
        requests:
          cpu: 500m
          memory: 1Gi
        limits:
          cpu: 2000m
          memory: 4Gi
      mongod_config: |
        operationProfiling:
          mode: slowOp
          slowOpThresholdMs: 200
      arbiter:
        enabled: true
        size: 1
      pod_disruption_budget:
        max_unavailable: 1
      scheduling:
        anti_affinity_topology_key: topology.kubernetes.io/zone
        node_selector:
          workload: database
        tolerations:
          - key: dedicated
            operator: Equal
            value: database
            effect: NoSchedule
        priority_class_name: database-critical
    - name: rs1
      size: 3
      storage:
        size: 20Gi
      expose:
        enabled: true
        type: ClusterIP
        annotations:
          example.com/team: data
  sharding:
    enabled: true
    balancer_enabled: true
    config_server:
      size: 3
      storage:
        size: 5Gi
      resources:
        requests:
          cpu: 250m
          memory: 512Mi
    mongos:
      size: 2
      resources:
        requests:
          cpu: 250m
          memory: 512Mi
      expose:
        enabled: true
        type: ClusterIP
  tls:
    mode: requireTLS
    issuer:
      value: org-ca-issuer
    issuer_kind: ClusterIssuer
    cert_validity_duration: 2160h
  users:
    - name: appuser
      db: admin
      password: initial-app-password
      roles:
        - name: readWrite
          db: appdb
    - name: reporting
      db: admin
      password: reporting-password
      roles:
        - name: read
          db: appdb
  backup:
    storages:
      - name: minio
        main: true
        s3:
          bucket: mongo-backups
          region: minio
          prefix: test-mongodb
          endpoint_url: http://minio.minio-system.svc:9000
          insecure_skip_tls_verify: true
          access_keys:
            access_key_id: minio-access-key
            secret_access_key: minio-secret-key
      - name: gcs-archive
        gcs:
          bucket: mongo-archive
          prefix: test-mongodb
          # GCS has no keyless arm: PBM authenticates with a service-account
          # key. Here the key already lives in the cluster (an
          # ExternalSecret-synced Secret in the operator's GCS_CLIENT_EMAIL /
          # GCS_PRIVATE_KEY shape); `service_account_key` takes the key
          # itself, raw or as a GcpServiceAccount's key_base64 output.
          credentials:
            existing_secret_name: mongo-archive-gcs-credentials
    tasks:
      - name: nightly
        schedule: "0 3 * * *"
        storage_name: minio
        type: physical
        keep: 7
        delete_from_storage: true
        compression: gzip
    pitr:
      enabled: true
      oplog_span_min: 10
      compression: gzip
  log_collector:
    enabled: true
    resources:
      requests:
        cpu: 50m
        memory: 64Mi
  update_strategy: SmartUpdate
  image_pull_secrets:
    - ghcr-pull-secret
```

## Spec Fields

| Path | Type | Required | Default | References |
|---|---|---|---|---|
| `spec.namespace` | `string \| valueFrom` | yes |  | KubernetesNamespace (`spec.name`) |
| `spec.createNamespace` | `bool` |  |  |  |
| `spec.imageName` | `string` |  |  |  |
| `spec.replicaSets` | `[]KubernetesMongodbReplicaSet` | yes |  |  |
| `spec.replicaSets[].name` | `string` | yes |  |  |
| `spec.replicaSets[].size` | `int32` |  | `3` |  |
| `spec.replicaSets[].storage` | `KubernetesMongodbStorage` | yes |  |  |
| `spec.replicaSets[].storage.size` | `string` | yes |  |  |
| `spec.replicaSets[].storage.storageClass` | `string \| valueFrom` |  |  | KubernetesStorageClass (`status.outputs.storage_class_name`) |
| `spec.replicaSets[].resources` | `ContainerResources` |  |  |  |
| `spec.replicaSets[].resources.limits` | `CpuMemory` |  |  |  |
| `spec.replicaSets[].resources.limits.cpu` | `string` |  |  |  |
| `spec.replicaSets[].resources.limits.memory` | `string` |  |  |  |
| `spec.replicaSets[].resources.requests` | `CpuMemory` |  |  |  |
| `spec.replicaSets[].resources.requests.cpu` | `string` |  |  |  |
| `spec.replicaSets[].resources.requests.memory` | `string` |  |  |  |
| `spec.replicaSets[].mongodConfig` | `string` |  |  |  |
| `spec.replicaSets[].arbiter` | `KubernetesMongodbArbiter` |  |  |  |
| `spec.replicaSets[].arbiter.enabled` | `bool` |  |  |  |
| `spec.replicaSets[].arbiter.size` | `int32` |  | `1` |  |
| `spec.replicaSets[].expose` | `KubernetesMongodbExpose` |  |  |  |
| `spec.replicaSets[].expose.enabled` | `bool` |  |  |  |
| `spec.replicaSets[].expose.type` | `string` |  | `ClusterIP` |  |
| `spec.replicaSets[].expose.annotations` | `map<string, string>` |  |  |  |
| `spec.replicaSets[].podDisruptionBudget` | `KubernetesMongodbPodDisruptionBudget` |  |  |  |
| `spec.replicaSets[].podDisruptionBudget.maxUnavailable` | `int32` |  |  |  |
| `spec.replicaSets[].podDisruptionBudget.minAvailable` | `int32` |  |  |  |
| `spec.replicaSets[].scheduling` | `KubernetesMongodbScheduling` |  |  |  |
| `spec.replicaSets[].scheduling.antiAffinityTopologyKey` | `string` |  |  |  |
| `spec.replicaSets[].scheduling.nodeSelector` | `map<string, string>` |  |  |  |
| `spec.replicaSets[].scheduling.tolerations` | `[]WorkloadToleration` |  |  |  |
| `spec.replicaSets[].scheduling.tolerations[].key` | `string` |  |  |  |
| `spec.replicaSets[].scheduling.tolerations[].operator` | `string` |  |  |  |
| `spec.replicaSets[].scheduling.tolerations[].value` | `string` |  |  |  |
| `spec.replicaSets[].scheduling.tolerations[].effect` | `string` |  |  |  |
| `spec.replicaSets[].scheduling.tolerations[].tolerationSeconds` | `int64` |  |  |  |
| `spec.replicaSets[].scheduling.priorityClassName` | `string` |  |  |  |
| `spec.sharding` | `KubernetesMongodbSharding` |  |  |  |
| `spec.sharding.enabled` | `bool` |  |  |  |
| `spec.sharding.configServer` | `KubernetesMongodbConfigServer` |  |  |  |
| `spec.sharding.configServer.size` | `int32` |  | `3` |  |
| `spec.sharding.configServer.storage` | `KubernetesMongodbStorage` | yes |  |  |
| `spec.sharding.configServer.storage.size` | `string` | yes |  |  |
| `spec.sharding.configServer.storage.storageClass` | `string \| valueFrom` |  |  | KubernetesStorageClass (`status.outputs.storage_class_name`) |
| `spec.sharding.configServer.resources` | `ContainerResources` |  |  |  |
| `spec.sharding.configServer.resources.limits` | `CpuMemory` |  |  |  |
| `spec.sharding.configServer.resources.limits.cpu` | `string` |  |  |  |
| `spec.sharding.configServer.resources.limits.memory` | `string` |  |  |  |
| `spec.sharding.configServer.resources.requests` | `CpuMemory` |  |  |  |
| `spec.sharding.configServer.resources.requests.cpu` | `string` |  |  |  |
| `spec.sharding.configServer.resources.requests.memory` | `string` |  |  |  |
| `spec.sharding.mongos` | `KubernetesMongodbMongos` |  |  |  |
| `spec.sharding.mongos.size` | `int32` |  | `3` |  |
| `spec.sharding.mongos.resources` | `ContainerResources` |  |  |  |
| `spec.sharding.mongos.resources.limits` | `CpuMemory` |  |  |  |
| `spec.sharding.mongos.resources.limits.cpu` | `string` |  |  |  |
| `spec.sharding.mongos.resources.limits.memory` | `string` |  |  |  |
| `spec.sharding.mongos.resources.requests` | `CpuMemory` |  |  |  |
| `spec.sharding.mongos.resources.requests.cpu` | `string` |  |  |  |
| `spec.sharding.mongos.resources.requests.memory` | `string` |  |  |  |
| `spec.sharding.mongos.expose` | `KubernetesMongodbExpose` |  |  |  |
| `spec.sharding.mongos.expose.enabled` | `bool` |  |  |  |
| `spec.sharding.mongos.expose.type` | `string` |  | `ClusterIP` |  |
| `spec.sharding.mongos.expose.annotations` | `map<string, string>` |  |  |  |
| `spec.sharding.balancerEnabled` | `bool` |  | `true` |  |
| `spec.tls` | `KubernetesMongodbTls` |  |  |  |
| `spec.tls.mode` | `string` |  | `preferTLS` |  |
| `spec.tls.issuer` | `string \| valueFrom` |  |  | KubernetesClusterIssuer (`metadata.name`) |
| `spec.tls.issuerKind` | `string` |  | `ClusterIssuer` |  |
| `spec.tls.certValidityDuration` | `string` |  |  |  |
| `spec.users` | `[]KubernetesMongodbUser` |  |  |  |
| `spec.users[].name` | `string` | yes |  |  |
| `spec.users[].db` | `string` |  | `admin` |  |
| `spec.users[].password` | `string` (sensitive) |  |  |  |
| `spec.users[].roles` | `[]KubernetesMongodbUserRole` | yes |  |  |
| `spec.users[].roles[].name` | `string` | yes |  |  |
| `spec.users[].roles[].db` | `string` | yes |  |  |
| `spec.backup` | `KubernetesMongodbBackup` |  |  |  |
| `spec.backup.storages` | `[]KubernetesMongodbBackupStorage` | yes |  |  |
| `spec.backup.storages[].name` | `string` | yes |  |  |
| `spec.backup.storages[].main` | `bool` |  |  |  |
| `spec.backup.storages[].s3` | `KubernetesMongodbS3Storage` |  |  |  |
| `spec.backup.storages[].s3.bucket` | `string` | yes |  |  |
| `spec.backup.storages[].s3.region` | `string` |  |  |  |
| `spec.backup.storages[].s3.prefix` | `string` |  |  |  |
| `spec.backup.storages[].s3.endpointUrl` | `string` |  |  |  |
| `spec.backup.storages[].s3.insecureSkipTlsVerify` | `bool` |  |  |  |
| `spec.backup.storages[].s3.accessKeys` | `KubernetesMongodbS3AccessKeys` |  |  |  |
| `spec.backup.storages[].s3.accessKeys.accessKeyId` | `string` | yes |  |  |
| `spec.backup.storages[].s3.accessKeys.secretAccessKey` | `string` (sensitive) | yes |  |  |
| `spec.backup.storages[].gcs` | `KubernetesMongodbGcsStorage` |  |  |  |
| `spec.backup.storages[].gcs.bucket` | `string` | yes |  |  |
| `spec.backup.storages[].gcs.prefix` | `string` |  |  |  |
| `spec.backup.storages[].gcs.credentials` | `KubernetesMongodbGcsCredentials` | yes |  |  |
| `spec.backup.storages[].gcs.credentials.serviceAccountKey` | `string \| valueFrom` (sensitive) |  |  | GcpServiceAccount (`status.outputs.key_base64`) |
| `spec.backup.storages[].gcs.credentials.existingSecretName` | `string` |  |  |  |
| `spec.backup.storages[].azure` | `KubernetesMongodbAzureStorage` |  |  |  |
| `spec.backup.storages[].azure.container` | `string` | yes |  |  |
| `spec.backup.storages[].azure.prefix` | `string` |  |  |  |
| `spec.backup.storages[].azure.endpointUrl` | `string` |  |  |  |
| `spec.backup.storages[].azure.storageAccount` | `string` | yes |  |  |
| `spec.backup.storages[].azure.accessKey` | `string` (sensitive) | yes |  |  |
| `spec.backup.tasks` | `[]KubernetesMongodbBackupTask` |  |  |  |
| `spec.backup.tasks[].name` | `string` | yes |  |  |
| `spec.backup.tasks[].schedule` | `string` | yes |  |  |
| `spec.backup.tasks[].storageName` | `string` | yes |  |  |
| `spec.backup.tasks[].type` | `string` |  | `logical` |  |
| `spec.backup.tasks[].keep` | `int32` |  |  |  |
| `spec.backup.tasks[].deleteFromStorage` | `bool` |  | `true` |  |
| `spec.backup.tasks[].suspend` | `bool` |  |  |  |
| `spec.backup.tasks[].compression` | `string` |  | `gzip` |  |
| `spec.backup.pitr` | `KubernetesMongodbPitr` |  |  |  |
| `spec.backup.pitr.enabled` | `bool` |  |  |  |
| `spec.backup.pitr.oplogOnly` | `bool` |  |  |  |
| `spec.backup.pitr.oplogSpanMin` | `int32` |  | `10` |  |
| `spec.backup.pitr.compression` | `string` |  | `gzip` |  |
| `spec.updateStrategy` | `string` |  | `SmartUpdate` |  |
| `spec.logCollector` | `KubernetesMongodbLogCollector` |  |  |  |
| `spec.logCollector.enabled` | `bool` |  | `true` |  |
| `spec.logCollector.resources` | `ContainerResources` |  |  |  |
| `spec.logCollector.resources.limits` | `CpuMemory` |  |  |  |
| `spec.logCollector.resources.limits.cpu` | `string` |  |  |  |
| `spec.logCollector.resources.limits.memory` | `string` |  |  |  |
| `spec.logCollector.resources.requests` | `CpuMemory` |  |  |  |
| `spec.logCollector.resources.requests.cpu` | `string` |  |  |  |
| `spec.logCollector.resources.requests.memory` | `string` |  |  |  |
| `spec.unsafe` | `KubernetesMongodbUnsafe` |  |  |  |
| `spec.unsafe.replsetSize` | `bool` |  |  |  |
| `spec.unsafe.mongosSize` | `bool` |  |  |  |
| `spec.unsafe.tls` | `bool` |  |  |  |
| `spec.unsafe.backupIfUnhealthy` | `bool` |  |  |  |
| `spec.pause` | `bool` |  |  |  |
| `spec.imagePullSecrets` | `[]string` |  |  |  |
| `spec.restore` | `KubernetesMongodbRestore` |  |  |  |
| `spec.restore.backupName` | `string` |  |  |  |
| `spec.restore.backupSource` | `KubernetesMongodbRestoreBackupSource` |  |  |  |
| `spec.restore.backupSource.storageName` | `string` | yes |  |  |
| `spec.restore.backupSource.destination` | `string` | yes |  |  |
| `spec.restore.backupSource.type` | `string` |  | `logical` |  |
| `spec.restore.pitr` | `KubernetesMongodbRestorePitr` |  |  |  |
| `spec.restore.pitr.type` | `string` | yes |  |  |
| `spec.restore.pitr.date` | `string` |  |  |  |
| `spec.restore.replsetRemapping` | `map<string, string>` |  |  |  |
| `spec.systemUsersSecretName` | `string` |  |  |  |

## Field Details

### spec.namespace

`string | valueFrom` · required

Namespace to create the MongoDB cluster in. The reconciling
operator must watch this namespace (the default operator posture
watches its OWN namespace — install the operator there, or widen
its watch). Accepts a literal namespace name or a reference to a
KubernetesNamespace resource.

- references: KubernetesNamespace (`spec.name`)
- rule: {"required":true}
- rule: write as {value: <literal>} or {valueFrom: {kind: KubernetesNamespace, name: <that resource's name>, fieldPath: spec.name}} -- a bare string does not parse

### spec.createNamespace

`bool`

When true, the namespace is created (with the standard Planton
governance labels) before the cluster and deleted with the
resource. When false, the namespace must already exist.

### spec.imageName

`string`

Percona Server for MongoDB container image, tag form (e.g.
"percona/percona-server-mongodb:8.0.19-7" — MongoDB 8.0). Empty =
the module's default for the pinned operator. This is how the
MongoDB version is chosen; changing it on a live cluster performs a
SmartUpdate rolling upgrade.

### spec.replicaSets

`[]KubernetesMongodbReplicaSet` · required

The data-bearing replica sets. Exactly one without sharding; with
sharding, each set becomes a shard. 3 members is the production
shape (automated failover needs a majority); 1 is a development
posture that REQUIRES unsafe.replset_size.

- rule: each replica set needs a distinct name
- rule: {"repeated":{"minItems":"1"}}

### spec.replicaSets[].name

`string` · required

Replica set name ("rs0" is the upstream convention for the first).

- rule: replica set name must be a lowercase DNS label (letters, numbers, hyphens)
- rule: {"required":true}

### spec.replicaSets[].size

`int32` · optional (explicit presence)

Data-bearing members. 3 is the production shape (a majority
survives one loss); even numbers waste a vote — add the arbiter
instead. 1 REQUIRES unsafe.replset_size.

- default: `3`
- rule: {"int32":{"gte":1}}

### spec.replicaSets[].storage

`KubernetesMongodbStorage` · required

Storage for every member. Required: the operator provisions one
PVC per member; grows are applied in place (never shrinks).

- rule: {"required":true}

### spec.replicaSets[].storage.size

`string` · required

Volume size as a Kubernetes quantity (e.g. "10Gi"). Grows are
applied to live PVCs; shrinks are rejected.

- rule: size must be a Kubernetes quantity like '10Gi' or '500Mi'
- rule: {"required":true}

### spec.replicaSets[].storage.storageClass

`string | valueFrom`

StorageClass for the PVCs. Empty = the cluster's default class.
Accepts a literal class name or a reference to a
KubernetesStorageClass resource.

- references: KubernetesStorageClass (`status.outputs.storage_class_name`)
- rule: write as {value: <literal>} or {valueFrom: {kind: KubernetesStorageClass, name: <that resource's name>, fieldPath: status.outputs.storage_class_name}} -- a bare string does not parse

### spec.replicaSets[].resources

`ContainerResources`

CPU/memory for every member pod. Empty = no requests/limits (fine
for evaluation, not production — WiredTiger sizes its cache from
the memory limit).

### spec.replicaSets[].resources.limits

`CpuMemory`

The resource limits for the container.
Specify the maximum amount of CPU and memory that the container can use.

### spec.replicaSets[].resources.limits.cpu

`string`

### spec.replicaSets[].resources.limits.memory

`string`

### spec.replicaSets[].resources.requests

`CpuMemory`

The resource requests for the container.
Specify the minimum amount of CPU and memory that the container is guaranteed.

### spec.replicaSets[].resources.requests.cpu

`string`

### spec.replicaSets[].resources.requests.memory

`string`

### spec.replicaSets[].mongodConfig

`string`

Extra mongod configuration merged over the operator's defaults
(YAML, mongod.conf shape — operationProfiling, setParameter,
storage.wiredTiger tuning, ...). Replication and security
essentials are operator-managed.

### spec.replicaSets[].arbiter

`KubernetesMongodbArbiter`

An arbiter: votes in elections but holds no data — majority
elections for an even number of data members at near-zero cost.

### spec.replicaSets[].arbiter.enabled

`bool`

Run arbiters.

### spec.replicaSets[].arbiter.size

`int32` · optional (explicit presence)

Arbiter count. Upstream default: 1 (one is almost always right —
arbiters exist to break ties).

- default: `1`
- rule: {"int32":{"gte":1}}

### spec.replicaSets[].expose

`KubernetesMongodbExpose`

Per-member Services instead of the default headless Service —
the managed-cloud LoadBalancer / cross-cluster recipe surface.

### spec.replicaSets[].expose.enabled

`bool`

Create one Service per member (drivers discover the set through
them). Without it, members are reachable through the headless
Service — the right in-cluster posture.

### spec.replicaSets[].expose.type

`string` · optional (explicit presence)

Service type: ClusterIP (default), NodePort, or LoadBalancer.

- default: `ClusterIP`
- rule: expose type must be ClusterIP, NodePort, or LoadBalancer

### spec.replicaSets[].expose.annotations

`map<string, string>`

Service annotations — the per-cloud LoadBalancer recipe surface.

### spec.replicaSets[].podDisruptionBudget

`KubernetesMongodbPodDisruptionBudget`

PodDisruptionBudget for the members. Omitted = the upstream
default (max one member down to voluntary disruptions).

- rule: declare at most one PDB bound — max_unavailable or min_available

### spec.replicaSets[].podDisruptionBudget.maxUnavailable

`int32`

Maximum members down during voluntary disruptions. Upstream
default: 1 — the majority must survive. Mutually exclusive with
min_available.

- rule: {"int32":{"gte":0}}

### spec.replicaSets[].podDisruptionBudget.minAvailable

`int32`

Minimum members that must stay up. Mutually exclusive with
max_unavailable.

- rule: {"int32":{"gte":0}}

### spec.replicaSets[].scheduling

`KubernetesMongodbScheduling`

Where member pods run: anti-affinity spreading, node selection,
tolerations, and scheduling priority.

### spec.replicaSets[].scheduling.antiAffinityTopologyKey

`string`

Topology key the operator's anti-affinity spreads members across.
Upstream default: kubernetes.io/hostname (one member per node);
use topology.kubernetes.io/zone to spread across zones, or "none"
to disable anti-affinity (development only).

### spec.replicaSets[].scheduling.nodeSelector

`map<string, string>`

Node selector for the member pods.

### spec.replicaSets[].scheduling.tolerations

`[]WorkloadToleration`

Tolerations for the member pods.

### spec.replicaSets[].scheduling.tolerations[].key

`string`

Taint key to tolerate. Empty key with operator "Exists" tolerates every taint.

### spec.replicaSets[].scheduling.tolerations[].operator

`string`

How key/value match: "Equal" (default — value must match too) or "Exists"
(key presence alone matches).

- rule: Toleration operator must be either "Equal" or "Exists"

### spec.replicaSets[].scheduling.tolerations[].value

`string`

Taint value to match when operator is "Equal".

### spec.replicaSets[].scheduling.tolerations[].effect

`string`

Which taint effect is tolerated: "NoSchedule", "PreferNoSchedule", or
"NoExecute". Empty tolerates all effects for the key.

- rule: Toleration effect must be one of "NoSchedule", "PreferNoSchedule", or "NoExecute"

### spec.replicaSets[].scheduling.tolerations[].tolerationSeconds

`int64` · optional (explicit presence)

For "NoExecute" taints only: how many seconds already-running pods stay bound
after the taint appears. Unset means tolerate forever.

### spec.replicaSets[].scheduling.priorityClassName

`string`

PriorityClass for the member pods — databases should outlive
stateless workloads under node pressure.

### spec.sharding

`KubernetesMongodbSharding`

Sharding: mongos routers + a config-server replica set; every
declared replica set becomes a shard. Omitted = a plain replica-set
cluster (the common shape — shard when a single set's write volume
or working set no longer fits).

- rule: sharding needs config_server and mongos declarations

### spec.sharding.enabled

`bool`

Enable sharding: every declared replica set becomes a shard;
clients connect through mongos.

### spec.sharding.configServer

`KubernetesMongodbConfigServer`

The config-server replica set (cluster metadata — which chunks
live on which shard). Required when sharding is enabled.

### spec.sharding.configServer.size

`int32` · optional (explicit presence)

Members. 3 is the production shape. 1 REQUIRES
unsafe.replset_size.

- default: `3`
- rule: {"int32":{"gte":1}}

### spec.sharding.configServer.storage

`KubernetesMongodbStorage` · required

Storage for every config-server member. Required — metadata is
small but precious (a few Gi is the norm).

- rule: {"required":true}

### spec.sharding.configServer.storage.size

`string` · required

Volume size as a Kubernetes quantity (e.g. "10Gi"). Grows are
applied to live PVCs; shrinks are rejected.

- rule: size must be a Kubernetes quantity like '10Gi' or '500Mi'
- rule: {"required":true}

### spec.sharding.configServer.storage.storageClass

`string | valueFrom`

StorageClass for the PVCs. Empty = the cluster's default class.
Accepts a literal class name or a reference to a
KubernetesStorageClass resource.

- references: KubernetesStorageClass (`status.outputs.storage_class_name`)
- rule: write as {value: <literal>} or {valueFrom: {kind: KubernetesStorageClass, name: <that resource's name>, fieldPath: status.outputs.storage_class_name}} -- a bare string does not parse

### spec.sharding.configServer.resources

`ContainerResources`

CPU/memory for every config-server pod.

### spec.sharding.configServer.resources.limits

`CpuMemory`

The resource limits for the container.
Specify the maximum amount of CPU and memory that the container can use.

### spec.sharding.configServer.resources.limits.cpu

`string`

### spec.sharding.configServer.resources.limits.memory

`string`

### spec.sharding.configServer.resources.requests

`CpuMemory`

The resource requests for the container.
Specify the minimum amount of CPU and memory that the container is guaranteed.

### spec.sharding.configServer.resources.requests.cpu

`string`

### spec.sharding.configServer.resources.requests.memory

`string`

### spec.sharding.mongos

`KubernetesMongodbMongos`

The mongos query routers. Required when sharding is enabled.

### spec.sharding.mongos.size

`int32` · optional (explicit presence)

Router count. Upstream default: 3 (2+ keeps the query path
available — a single router is a development posture, though the
pinned operator does not reject it).

- default: `3`
- rule: {"int32":{"gte":1}}

### spec.sharding.mongos.resources

`ContainerResources`

CPU/memory for every mongos pod.

### spec.sharding.mongos.resources.limits

`CpuMemory`

The resource limits for the container.
Specify the maximum amount of CPU and memory that the container can use.

### spec.sharding.mongos.resources.limits.cpu

`string`

### spec.sharding.mongos.resources.limits.memory

`string`

### spec.sharding.mongos.resources.requests

`CpuMemory`

The resource requests for the container.
Specify the minimum amount of CPU and memory that the container is guaranteed.

### spec.sharding.mongos.resources.requests.cpu

`string`

### spec.sharding.mongos.resources.requests.memory

`string`

### spec.sharding.mongos.expose

`KubernetesMongodbExpose`

The mongos client Service (`<name>-mongos`). Type and per-cloud
annotations for the managed-LoadBalancer recipes.

### spec.sharding.mongos.expose.enabled

`bool`

Create one Service per member (drivers discover the set through
them). Without it, members are reachable through the headless
Service — the right in-cluster posture.

### spec.sharding.mongos.expose.type

`string` · optional (explicit presence)

Service type: ClusterIP (default), NodePort, or LoadBalancer.

- default: `ClusterIP`
- rule: expose type must be ClusterIP, NodePort, or LoadBalancer

### spec.sharding.mongos.expose.annotations

`map<string, string>`

Service annotations — the per-cloud LoadBalancer recipe surface.

### spec.sharding.balancerEnabled

`bool` · optional (explicit presence)

The shard balancer (moves chunks between shards to even the load).
Upstream default: enabled.

- default: `true`

### spec.tls

`KubernetesMongodbTls`

TLS posture. Omitted = preferTLS with operator-generated
certificates (the upstream default). Point issuer at a cert-manager
(Cluster)Issuer for an organization-trusted chain; disabling TLS
REQUIRES unsafe.tls.

### spec.tls.mode

`string` · optional (explicit presence)

TLS mode: disabled (REQUIRES unsafe.tls), allowTLS, preferTLS (the
upstream default), or requireTLS (the production posture — clients
must speak TLS).

- default: `preferTLS`
- rule: tls mode must be disabled, allowTLS, preferTLS, or requireTLS

### spec.tls.issuer

`string | valueFrom`

cert-manager issuer for the cluster's certificates instead of the
operator's self-generated ones — the organization-trust seam.
References a KubernetesClusterIssuer (set issuer_kind to "Issuer"
for a namespaced KubernetesIssuer).

- references: KubernetesClusterIssuer (`metadata.name`)
- rule: write as {value: <literal>} or {valueFrom: {kind: KubernetesClusterIssuer, name: <that resource's name>, fieldPath: metadata.name}} -- a bare string does not parse

### spec.tls.issuerKind

`string` · optional (explicit presence)

Kind of the cert-manager issuer named above: "ClusterIssuer" (the
default) or "Issuer" (must live in the database's namespace).

- default: `ClusterIssuer`
- rule: issuer_kind must be ClusterIssuer or Issuer

### spec.tls.certValidityDuration

`string`

Validity of operator-generated certificates (Go duration; upstream
default "2160h" = 90 days).

### spec.users

`[]KubernetesMongodbUser`

Declarative application users: the operator creates them, keeps
their roles reconciled, and manages their password Secrets.

- rule: each user needs a distinct name

### spec.users[].name

`string` · required

Username.

- rule: {"required":true}

### spec.users[].db

`string` · optional (explicit presence)

Authentication database. Upstream default: "admin".

- default: `admin`

### spec.users[].password

`string` · sensitive

Password, materialized as a Kubernetes Secret the operator watches
(`<cluster>-user-<name>`) — rotating the value rotates the
database password. Empty = the operator generates a password into
the same Secret.

### spec.users[].roles

`[]KubernetesMongodbUserRole` · required

Roles granted to the user.

- rule: {"repeated":{"minItems":"1"}}

### spec.users[].roles[].name

`string` · required

Role name (readWrite, read, dbAdmin, clusterAdmin, ...).

- rule: {"required":true}

### spec.users[].roles[].db

`string` · required

Database the role applies to.

- rule: {"required":true}

### spec.backup

`KubernetesMongodbBackup`

Scheduled backups + point-in-time recovery via Percona Backup for
MongoDB. Omitted = no backups (a deliberate choice to make, not a
default to forget).

- rule: with multiple backup storages, mark exactly one main: true — PITR oplog chunks and restore metadata land there (a single storage is main implicitly)

### spec.backup.storages

`[]KubernetesMongodbBackupStorage` · required

Backup storage destinations, referenced by tasks and PITR via
their names. The storage marked main (or the single storage, main
implicitly) is where PITR oplog chunks land.

- rule: each backup storage needs a distinct name
- rule: {"repeated":{"minItems":"1"}}

### spec.backup.storages[].name

`string` · required

Name tasks reference this storage by.

- rule: storage name must be a lowercase DNS label (letters, numbers, hyphens)
- rule: {"required":true}

### spec.backup.storages[].main

`bool`

Mark this storage as the MAIN one (PITR oplog chunks and
un-storaged operations land here). Exactly one storage is main
when several are declared; a single storage is main implicitly.

### spec.backup.storages[].s3

`KubernetesMongodbS3Storage`

AWS S3 — or ANY S3-compatible store (MinIO, Ceph RGW, ...) via
the endpoint_url override.

- rule: an S3-compatible endpoint (endpoint_url) authenticates with access_keys — the keyless posture only mints AWS credentials

### spec.backup.storages[].s3.bucket

`string` · required

Bucket name.

- rule: {"required":true}

### spec.backup.storages[].s3.region

`string`

AWS region of the bucket. Required for real S3; S3-compatible
stores use their expected value (MinIO accepts any).

### spec.backup.storages[].s3.prefix

`string`

Key prefix inside the bucket (a folder for this cluster's
backups). One prefix per cluster: PBM keeps its
backup and oplog metadata under the prefix, and a restore target
declares this same prefix (as a storage of its own) to read the
backups back — so two live clusters must never share one, while the
source and its restore target deliberately do.

### spec.backup.storages[].s3.endpointUrl

`string`

S3-COMPATIBLE ARM: endpoint URL of the store (e.g.
http://minio.minio-system.svc:9000 for in-cluster MinIO). Empty =
real AWS S3.

- rule: endpoint_url must be an http(s) URL (e.g. http://minio.minio-system.svc:9000)

### spec.backup.storages[].s3.insecureSkipTlsVerify

`bool`

Skip TLS verification against a self-signed endpoint (test
environments only).

### spec.backup.storages[].s3.accessKeys

`KubernetesMongodbS3AccessKeys`

Static access keys, materialized as a Kubernetes Secret the PBM
agents read. Omitted = the pods' AMBIENT AWS identity (EKS IRSA on
the cluster's service account, or node instance-profile
credentials) — the keyless posture for real S3.

### spec.backup.storages[].s3.accessKeys.accessKeyId

`string` · required

Access key ID — the public identifier of the key pair, not a
secret. For MinIO this is the access key / username.

- rule: {"required":true}

### spec.backup.storages[].s3.accessKeys.secretAccessKey

`string` · required · sensitive

Secret access key (for MinIO: the secret key / password).

- rule: {"required":true}

### spec.backup.storages[].gcs

`KubernetesMongodbGcsStorage`

Google Cloud Storage (native GCS API).

### spec.backup.storages[].gcs.bucket

`string` · required

Bucket name.

- rule: {"required":true}

### spec.backup.storages[].gcs.prefix

`string`

Key prefix inside the bucket. One prefix per cluster: PBM keeps its
backup and oplog metadata under the prefix, and a restore target
declares this same prefix (as a storage of its own) to read the
backups back — so two live clusters must never share one, while the
source and its restore target deliberately do.

### spec.backup.storages[].gcs.credentials

`KubernetesMongodbGcsCredentials` · required

The credentials the PBM agents present to GCS. REQUIRED: there is
no keyless posture for MongoDB backups on GCS. The operator's CRD
requires a credentials Secret on every gcs storage, and Percona
Backup for MongoDB refuses to build its Google client without a
client email and private key — the pods' GKE Workload Identity is
never consulted. (Both verified at the pinned operator 1.22.0.) The
one identity-free path is HMAC keys over the S3 interoperability
endpoint, which is the `s3` arm with `endpoint_url:
https://storage.googleapis.com`.

- rule: {"required":true}

### spec.backup.storages[].gcs.credentials.serviceAccountKey

`string | valueFrom` · sensitive

A GCP service-account key: the JSON key file's content, either raw
or base64-encoded — the shape a GcpServiceAccount resource exports
as `key_base64` when it is declared with a `user_managed_key`, so
a chart wires the identity and the database in one run. The module
extracts `client_email` and `private_key` into the
`<name>-backup-<storage>` Secret the operator reads; the key file
itself is never rendered into the custom resource. Grant the
account `roles/storage.objectAdmin` AND
`roles/storage.legacyBucketReader` on the bucket (a GcpGcsBucket's
`iam_members`, one entry each): the storage clients read the
bucket's attributes (`storage.buckets.get`) before writing, and
objectAdmin alone does not carry that permission.

- references: GcpServiceAccount (`status.outputs.key_base64`)
- rule: write as {value: <literal>} or {valueFrom: {kind: GcpServiceAccount, name: <that resource's name>, fieldPath: status.outputs.key_base64}} -- a bare string does not parse

### spec.backup.storages[].gcs.credentials.existingSecretName

`string`

The name of an existing Secret in this namespace already in the
operator's shape: keys `GCS_CLIENT_EMAIL` + `GCS_PRIVATE_KEY` (a
service-account key), or `AWS_ACCESS_KEY_ID` +
`AWS_SECRET_ACCESS_KEY` (HMAC keys for the S3-compatible client).
For credentials that already live in the cluster — synced by an
ExternalSecret, or shared with a sibling cluster.

### spec.backup.storages[].azure

`KubernetesMongodbAzureStorage`

Azure Blob Storage.

### spec.backup.storages[].azure.container

`string` · required

Blob container name.

- rule: {"required":true}

### spec.backup.storages[].azure.prefix

`string`

Key prefix inside the container. One prefix per cluster: PBM keeps its
backup and oplog metadata under the prefix, and a restore target
declares this same prefix (as a storage of its own) to read the
backups back — so two live clusters must never share one, while the
source and its restore target deliberately do.

### spec.backup.storages[].azure.endpointUrl

`string`

Endpoint URL override (sovereign clouds, Azurite). Empty = the
account's default blob endpoint.

### spec.backup.storages[].azure.storageAccount

`string` · required

Storage-account name.

- rule: {"required":true}

### spec.backup.storages[].azure.accessKey

`string` · required · sensitive

Storage-account access key, materialized as a Kubernetes Secret
the PBM agents read.

- rule: {"required":true}

### spec.backup.tasks

`[]KubernetesMongodbBackupTask`

Scheduled backup tasks.

- rule: each backup task needs a distinct name

### spec.backup.tasks[].name

`string` · required

Task name.

- rule: task name must be a lowercase DNS label (letters, numbers, hyphens)
- rule: {"required":true}

### spec.backup.tasks[].schedule

`string` · required

Standard FIVE-field cron expression (e.g. "0 2 * * *" = daily at
02:00).

- rule: schedule is a five-field cron expression — e.g. '0 2 * * *' for daily at 02:00
- rule: {"required":true}

### spec.backup.tasks[].storageName

`string` · required

Name of the declared storage this task writes to.

- rule: {"required":true}

### spec.backup.tasks[].type

`string` · optional (explicit presence)

Backup type: logical (BSON dump — the default; restores anywhere),
physical (data-file copy — much faster for large datasets),
incremental (physical delta since the last incremental-base), or
incremental-base (the full physical backup increments build on).

- default: `logical`
- rule: backup type must be logical, physical, incremental, or incremental-base

### spec.backup.tasks[].keep

`int32` · optional (explicit presence)

How many backups this task keeps. Older ones are pruned.

- rule: {"int32":{"gte":1}}

### spec.backup.tasks[].deleteFromStorage

`bool` · optional (explicit presence)

Also delete pruned backups FROM THE STORE (not just the cluster's
records).

- default: `true`

### spec.backup.tasks[].suspend

`bool`

Suspend the task (keeps the declaration, stops the backups).

### spec.backup.tasks[].compression

`string` · optional (explicit presence)

Compression: gzip (the upstream default), snappy, lz4, pgzip,
zstd, s2, or none.

- default: `gzip`
- rule: compression must be one of gzip, snappy, lz4, pgzip, zstd, s2, none

### spec.backup.pitr

`KubernetesMongodbPitr`

Point-in-time recovery: continuously archive oplog chunks so a
restore can land between backups. Requires at least one completed
base backup to be meaningful — PBM starts archiving only after the
first backup succeeds, and a `restore.pitr` on the target replays
exactly these chunks (live-proven: a write made after the backup came
back through them). The recovery point is the last CLOSED chunk
(`oplog_span_min`), so writes in the final span before a total loss
may not be in the archive.

### spec.backup.pitr.enabled

`bool`

Continuously archive oplog chunks to the main storage.

### spec.backup.pitr.oplogOnly

`bool`

Archive oplog ONLY — no base-backup requirement enforcement.
Advanced posture for external base-backup management; leave false.

### spec.backup.pitr.oplogSpanMin

`int32` · optional (explicit presence)

Minutes of oplog per archived chunk. Upstream default: 10 — the
recovery-point objective for a total-cluster loss.

- default: `10`
- rule: {"int32":{"gte":1}}

### spec.backup.pitr.compression

`string` · optional (explicit presence)

Oplog chunk compression: gzip (the upstream default), snappy,
lz4, pgzip, zstd, s2, or none.

- default: `gzip`
- rule: compression must be one of gzip, snappy, lz4, pgzip, zstd, s2, none

### spec.updateStrategy

`string` · optional (explicit presence)

How updates roll across the cluster: SmartUpdate (the operator
orders restarts safely — the upstream default), RollingUpdate
(StatefulSet semantics), or OnDelete.

- default: `SmartUpdate`
- rule: update_strategy must be SmartUpdate, RollingUpdate, or OnDelete

### spec.logCollector

`KubernetesMongodbLogCollector`

The fluent-bit log-collector sidecar shipping mongod logs. Omitted
= DISABLED: the operator runs the sidecar only when this block is
present (and enabled) — declare it to turn log collection on.

### spec.logCollector.enabled

`bool` · optional (explicit presence)

Run the sidecar. Upstream default: true.

- default: `true`

### spec.logCollector.resources

`ContainerResources`

CPU/memory for the sidecar.

### spec.logCollector.resources.limits

`CpuMemory`

The resource limits for the container.
Specify the maximum amount of CPU and memory that the container can use.

### spec.logCollector.resources.limits.cpu

`string`

### spec.logCollector.resources.limits.memory

`string`

### spec.logCollector.resources.requests

`CpuMemory`

The resource requests for the container.
Specify the minimum amount of CPU and memory that the container is guaranteed.

### spec.logCollector.resources.requests.cpu

`string`

### spec.logCollector.resources.requests.memory

`string`

### spec.unsafe

`KubernetesMongodbUnsafe`

Explicit opt-in to unsafe topologies and postures the operator
otherwise REJECTS. Development conveniences — never production.

### spec.unsafe.replsetSize

`bool`

Allow replica sets (and config servers) smaller than 3 members
(single-node development clusters).

### spec.unsafe.mongosSize

`bool`

Rendered onto the CR's unsafeFlags for surface parity, but the
pinned operator (1.22.0) declares this flag without enforcing it —
a single mongos router is never rejected.

### spec.unsafe.tls

`bool`

Allow tls.mode disabled.

### spec.unsafe.backupIfUnhealthy

`bool`

Allow backups to run against an unhealthy cluster.

### spec.pause

`bool`

Pause the cluster: scale everything to zero while keeping the data
volumes (and resume by flipping back).

### spec.imagePullSecrets

`[]string`

Names of image-pull secrets (in the cluster's namespace) for
pulling images from a private registry.

### spec.restore

`KubernetesMongodbRestore`

Restore this cluster from a backup — the disaster-recovery and
clone path. Declared on the cluster that RECEIVES the data: the
operator runs a PerconaServerMongoDBRestore against it once the
members are up. Requires `backup` with the storage the backup lives
in (the PBM agents that perform the restore ride the backup
configuration), and — for a backup taken by ANOTHER cluster —
`system_users_secret_name` pointing at that cluster's system-users
Secret, because a restored database carries the source's users and
passwords. Each distinct declaration runs exactly once; change it
(a different backup, a different point in time) to restore again.

### spec.restore.backupName

`string`

The name of a PerconaServerMongoDBBackup object in this namespace
— a backup THIS cluster (or a sibling in the namespace) took, whose
record the operator still holds. The same-cluster rollback path.

### spec.restore.backupSource

`KubernetesMongodbRestoreBackupSource`

A backup addressed by its location in the store — the path for a
backup ANOTHER cluster took (disaster recovery into a fresh
cluster, cloning an environment), where no Backup object exists
here. Percona's "storage defined on target" recipe: the storage's
configuration and credentials come from this cluster's own
`backup.storages` entry.

### spec.restore.backupSource.storageName

`string` · required

The name of the entry in this cluster's `backup.storages` that holds
the backup — the restore reads through that storage's bucket, prefix,
and credentials. The module copies that storage's definition into the
restore object itself: the operator resolves a backup-source restore's
storage from the restore object (not the cluster) when it syncs the
store's metadata into a fresh cluster — which every disaster-recovery
restore needs — so declaring the storage once here is enough.

- rule: {"required":true}

### spec.restore.backupSource.destination

`string` · required

Full path of the backup inside the store, in the store's native URI
form with the backup name as the LAST segment:
`s3://<bucket>/<prefix>/<backup-name>`, `gs://<bucket>/<prefix>/
<backup-name>`, or `azure://<container>/<prefix>/<backup-name>`.
The backup name is the timestamp PBM stamped when the backup
started (e.g. 2026-09-09T12:00:00Z) — read it from the DESTINATION
column of `kubectl get psmdb-backup` on the source cluster, or list
the store.

- rule: destination is the backup's full path in the store: s3://<bucket>/<prefix>/<backup-name>, gs://..., or azure://... — the backup name (PBM's start timestamp) is the last segment
- rule: {"required":true}

### spec.restore.backupSource.type

`string` · optional (explicit presence)

The backup's type — it must match how the source took it: logical
(the default) or physical. A physical restore additionally requires
this cluster to match the source's topology (same replica-set
count and member count) and, if the source encrypted data at rest,
the same encryption key.

- default: `logical`
- rule: restore type must be logical or physical (incremental chains restore through their physical base)

### spec.restore.pitr

`KubernetesMongodbRestorePitr`

Point-in-time recovery: after replaying the base backup, replay the
archived oplog chunks up to a moment. Requires the source cluster to
have archived oplog (`backup.pitr.enabled`) into the same storage.
Omitted = the base backup alone.

- rule: pitr type date needs `date` as 'YYYY-MM-DD HH:MM:SS' (UTC); type latest takes no date

### spec.restore.pitr.type

`string` · required

`latest` replays every archived oplog chunk (the smallest possible
data loss); `date` stops at the moment in `date`.

- rule: pitr type must be latest (replay everything archived) or date (stop at the moment in `date`)
- rule: {"required":true}

### spec.restore.pitr.date

`string`

The moment to recover to, `YYYY-MM-DD HH:MM:SS` in UTC — the
operator's own format. Required with type `date`; must not be set
with `latest`.

### spec.restore.replsetRemapping

`map<string, string>`

Restore into replica sets named differently from the source's — a
map of source replica-set name to this cluster's replica-set name.
Omitted = names must match (the operator refuses a mismatch).

### spec.systemUsersSecretName

`string`

Bring your own system-users Secret instead of letting the operator
generate `<name>-secrets`: the name of an existing Secret in this
namespace carrying the operator's built-in account keys
(MONGODB_BACKUP_USER/PASSWORD, MONGODB_CLUSTER_ADMIN_USER/PASSWORD,
MONGODB_CLUSTER_MONITOR_USER/PASSWORD, MONGODB_USER_ADMIN_USER/
PASSWORD, MONGODB_DATABASE_ADMIN_USER/PASSWORD). The load-bearing
use is disaster recovery: a cluster restoring another cluster's
backup inherits that cluster's users, so it must authenticate with
that cluster's passwords — point this at the source's
`<source>-secrets` (kept alive by a KubernetesSecret, an
ExternalSecret, or the secret backend). Empty = operator-generated.

## Validation Rules

- `spec.restore_requires_backup`: a restore needs the backup block: the PBM agents that perform it are configured by spec.backup, and the restore's storage must be one of spec.backup.storages
- `spec.restore_storage_declared`: restore.backup_source.storage_name must name one of spec.backup.storages — the restore reads the backup through that storage's configuration and credentials
- `spec.single_replset_without_sharding`: without sharding, declare exactly one replica set — multiple sets only make sense as shards (enable sharding)
- `spec.replset_size_or_unsafe`: a replica set smaller than 3 members cannot elect a majority — the operator rejects it unless unsafe.replset_size explicitly opts in (development only)
- `spec.config_server_size_or_unsafe`: a config server smaller than 3 members cannot elect a majority — the operator rejects it unless unsafe.replset_size explicitly opts in (development only)
- `spec.tls_disabled_or_unsafe`: tls mode disabled is a plaintext development posture — the operator rejects it unless unsafe.tls explicitly opts in

## Outputs

Reference an output from another manifest as `valueFrom: {kind: KubernetesMongodb, name: <resource-name>, fieldPath: status.outputs.<output>}`.

| Output | Type | Description |
|---|---|---|
| `status.outputs.namespace` | `string` | Namespace the cluster runs in. |
| `status.outputs.cluster_name` | `string` | The PerconaServerMongoDB resource name (`metadata.name`) — every operator-created object derives from it. |
| `status.outputs.service` | `string` | The Service applications connect to: `<name>-mongos` when sharding is enabled, otherwise the first replica set's headless Service (`<name>-<rs>` — drivers discover every member through it). |
| `status.outputs.kube_endpoint` | `string` | In-cluster connection endpoint: `<service>.<namespace>.svc.cluster.local:27017`. For replica-set clusters, connect with `mongodb://<user>:<pass>@<endpoint>/?replicaSet=<rs>` so the driver follows failovers. |
| `status.outputs.replica_set` | `string` | The first replica set's name (the driver's replicaSet parameter). Empty for sharded clusters — mongos needs no replicaSet parameter. |
| `status.outputs.port_forward_command` | `string` | kubectl port-forward one-liner for reaching the database from a workstation. |
| `status.outputs.admin_password_secret` | `KubernetesSecretKey` | The Kubernetes Secret key holding the database-admin password (the system-users Secret — the operator-managed `<name>-secrets`, or the one `system_users_secret_name` brought — key MONGODB_DATABASE_ADMIN_PASSWORD; the paired username key is MONGODB_DATABASE_ADMIN_USER). |
| `status.outputs.admin_password_secret.name` | `string` | The name of the Kubernetes Secret. |
| `status.outputs.admin_password_secret.key` | `string` | The key within the Kubernetes Secret. |
| `status.outputs.restore_name` | `string` | Name of the PerconaServerMongoDBRestore object rendered for the spec's `restore` declaration (`<name>-restore-<8 hex>`, the suffix hashing the declaration) — the run whose status tells whether the restore reached ready: `kubectl get psmdb-restore <this> -n <namespace>`. Empty when no restore is declared. |

## References

Fields that can point at another resource's outputs:

| Field | Kind | Output |
|---|---|---|
| `spec.namespace` | KubernetesNamespace | `spec.name` |
| `spec.replicaSets[].storage.storageClass` | KubernetesStorageClass | `status.outputs.storage_class_name` |
| `spec.sharding.configServer.storage.storageClass` | KubernetesStorageClass | `status.outputs.storage_class_name` |
| `spec.tls.issuer` | KubernetesClusterIssuer | `metadata.name` |
| `spec.backup.storages[].gcs.credentials.serviceAccountKey` | GcpServiceAccount | `status.outputs.key_base64` |

## See Also

- [Overview](../README.md)
