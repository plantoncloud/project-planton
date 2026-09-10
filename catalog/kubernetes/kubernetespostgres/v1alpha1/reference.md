# KubernetesPostgres

> Generated from the protobuf schema by `make generate-reference` -- do not
> edit by hand. To change a fact on this page, change the proto field comment
> or validation rule it is derived from, then regenerate.

**apiVersion**: `kubernetes.planton.dev/v1alpha1`

**Guide**: [GUIDE.md](../GUIDE.md) -- authored operational judgment for this component: conventions, trade-offs, and what pairs well with it.

**KubernetesPostgresSpec** declares a production-grade PostgreSQL cluster
reconciled by CloudNativePG (KubernetesCloudNativePgOperator must be on
the cluster). The spec renders a `postgresql.cnpg.io/v1` Cluster custom
resource — and, when backups are declared, the companion Barman Cloud
`ObjectStore` and `ScheduledBackup` resources — so one Planton resource
carries the whole database story: instances and streaming replication
with automated failover, storage (with a separate WAL volume when I/O
isolation matters), PostgreSQL configuration, bootstrap (fresh initdb,
restore from a backup, physical replication from an existing server, or
logical import), declarative roles, continuous WAL archiving plus
scheduled base backups to S3/GCS/Azure-Blob/S3-compatible stores, TLS,
and monitoring.

NAMING CONTRACT: every object CloudNativePG creates derives from
`metadata.name` — instance pods (`<name>-1`, `<name>-2`, ...), the
traffic Services (`<name>-rw` primary read-write, `<name>-ro` replicas
only, `<name>-r` any instance), and the credential Secrets
(`<name>-app`, `<name>-superuser`). Applications connect through the
SERVICES, never a pod: after a failover the -rw Service re-points to the
new primary automatically.

EXPOSURE IS COMPOSED, never embedded: the cluster is in-cluster plumbing
reachable at the exported `kube_endpoint`. To reach it from outside,
compose a first-class exposure kind (KubernetesService of type
LoadBalancer selecting the -rw Service's pods via a managed Service, or
a TCP route on a Gateway) — this component never creates one.

BACKUPS ARE PLUGIN-BASED: the backup block renders a Barman Cloud
`ObjectStore` resource plus the Cluster's plugin wiring (WAL archiving
starts immediately) and one `ScheduledBackup` per declared schedule.
The Barman Cloud plugin must be on the cluster, in the operator's
namespace (KubernetesCnpgBarmanCloudPlugin) — without it the operator
parks a backup-declaring Cluster in the phase "Cluster cannot proceed
to reconciliation due to an unknown plugin being required" and never
creates its instances. CloudNativePG's built-in object-store support is
deprecated upstream and deliberately not modeled here.

## Example

```yaml
# Full-surface manifest for offline module proofs (tofu validate/plan and
# pulumi preview). Exercises every typed block coherently: initdb bootstrap
# with a declared owner password, a three-instance cluster with separate
# WAL storage, tuned PostgreSQL configuration with quorum synchronous
# replication, superuser access with a declared password, managed roles,
# S3-compatible (MinIO-style) backups with declared access keys and two
# schedules, TLS alt names, TLS metrics, hard anti-affinity with
# tolerations, switchover updates, and image pull secrets. Both engines
# must render identical CRs + credential Secrets from it.
apiVersion: kubernetes.planton.dev/v1alpha1
kind: KubernetesPostgres
metadata:
  name: test-postgres
spec:
  namespace:
    value: test-postgres-ns
  createNamespace: true
  instances: 3
  imageName: ghcr.io/cloudnative-pg/postgresql:17.5
  storage:
    size: 20Gi
    storageClass:
      value: fast-ssd
    resizeInUseVolumes: true
  walStorage:
    size: 5Gi
    storageClass:
      value: fast-ssd
  resources:
    limits:
      cpu: 2000m
      memory: 4Gi
    requests:
      cpu: 500m
      memory: 1Gi
  postgresql:
    parameters:
      max_connections: "200"
      shared_buffers: 512MB
      pg_stat_statements.max: "10000"
    pgHba:
      - hostssl app all 10.0.0.0/8 scram-sha-256
    sharedPreloadLibraries:
      - pg_stat_statements
    synchronous:
      method: any
      number: 1
      dataDurability: required
  bootstrap:
    initdb:
      database: appdb
      owner: appuser
      ownerPassword: initial-owner-password
      dataChecksums: true
      encoding: UTF8
      postInitApplicationSql:
        - CREATE EXTENSION IF NOT EXISTS pg_stat_statements;
  superuser:
    enabled: true
    password: super-secret-postgres-password
  roles:
    - name: analyst
      comment: read-only analytics user
      password: analyst-password
      login: true
      inRoles:
        - pg_read_all_data
      connectionLimit: 10
    - name: legacy-batch
      ensure: absent
  backup:
    objectStore:
      destinationPath: s3://backups/test-postgres
      s3:
        region: us-east-1
        endpointUrl: http://minio.minio-system.svc:9000
        accessKeys:
          accessKeyId: minio-access-key
          secretAccessKey: minio-secret-key
      wal:
        compression: zstd
        maxParallel: 4
      data:
        compression: gzip
        jobs: 2
        immediateCheckpoint: true
    retentionPolicy: 30d
    schedules:
      - name: daily
        schedule: 0 0 2 * * *
        immediate: true
      - name: weekly
        schedule: 0 30 4 * * 0
        target: primary
  certificates:
    serverAltDnsNames:
      - postgres.example.com
  monitoring:
    tlsEnabled: true
  scheduling:
    antiAffinityType: required
    topologyKey: topology.kubernetes.io/zone
    tolerations:
      - key: dedicated
        operator: Equal
        value: database
        effect: NoSchedule
  updateStrategy:
    primaryUpdateStrategy: unsupervised
    primaryUpdateMethod: switchover
  enablePdb: true
  imagePullSecrets:
    - ghcr-pull-secret
```

## Spec Fields

| Path | Type | Required | Default | References |
|---|---|---|---|---|
| `spec.namespace` | `string \| valueFrom` | yes |  | KubernetesNamespace (`spec.name`) |
| `spec.createNamespace` | `bool` |  |  |  |
| `spec.instances` | `int32` |  | `1` |  |
| `spec.imageName` | `string` |  |  |  |
| `spec.storage` | `KubernetesPostgresStorage` | yes |  |  |
| `spec.storage.size` | `string` | yes |  |  |
| `spec.storage.storageClass` | `string \| valueFrom` |  |  | KubernetesStorageClass (`status.outputs.storage_class_name`) |
| `spec.storage.resizeInUseVolumes` | `bool` |  | `true` |  |
| `spec.walStorage` | `KubernetesPostgresStorage` |  |  |  |
| `spec.walStorage.size` | `string` | yes |  |  |
| `spec.walStorage.storageClass` | `string \| valueFrom` |  |  | KubernetesStorageClass (`status.outputs.storage_class_name`) |
| `spec.walStorage.resizeInUseVolumes` | `bool` |  | `true` |  |
| `spec.resources` | `ContainerResources` |  |  |  |
| `spec.resources.limits` | `CpuMemory` |  |  |  |
| `spec.resources.limits.cpu` | `string` |  |  |  |
| `spec.resources.limits.memory` | `string` |  |  |  |
| `spec.resources.requests` | `CpuMemory` |  |  |  |
| `spec.resources.requests.cpu` | `string` |  |  |  |
| `spec.resources.requests.memory` | `string` |  |  |  |
| `spec.postgresql` | `KubernetesPostgresServerConfig` |  |  |  |
| `spec.postgresql.parameters` | `map<string, string>` |  |  |  |
| `spec.postgresql.pgHba` | `[]string` |  |  |  |
| `spec.postgresql.pgIdent` | `[]string` |  |  |  |
| `spec.postgresql.sharedPreloadLibraries` | `[]string` |  |  |  |
| `spec.postgresql.synchronous` | `KubernetesPostgresSynchronousReplication` |  |  |  |
| `spec.postgresql.synchronous.method` | `string` |  | `any` |  |
| `spec.postgresql.synchronous.number` | `int32` |  |  |  |
| `spec.postgresql.synchronous.dataDurability` | `string` |  | `required` |  |
| `spec.postgresql.enableAlterSystem` | `bool` |  |  |  |
| `spec.bootstrap` | `KubernetesPostgresBootstrap` |  |  |  |
| `spec.bootstrap.initdb` | `KubernetesPostgresBootstrapInitDb` |  |  |  |
| `spec.bootstrap.initdb.database` | `string` |  | `app` |  |
| `spec.bootstrap.initdb.owner` | `string` |  |  |  |
| `spec.bootstrap.initdb.ownerPassword` | `string` (sensitive) |  |  |  |
| `spec.bootstrap.initdb.dataChecksums` | `bool` |  |  |  |
| `spec.bootstrap.initdb.encoding` | `string` |  | `UTF8` |  |
| `spec.bootstrap.initdb.localeCollate` | `string` |  |  |  |
| `spec.bootstrap.initdb.localeCtype` | `string` |  |  |  |
| `spec.bootstrap.initdb.postInitSql` | `[]string` |  |  |  |
| `spec.bootstrap.initdb.postInitApplicationSql` | `[]string` |  |  |  |
| `spec.bootstrap.initdb.import` | `KubernetesPostgresImport` |  |  |  |
| `spec.bootstrap.initdb.import.type` | `string` | yes |  |  |
| `spec.bootstrap.initdb.import.sourceExternalCluster` | `string` | yes |  |  |
| `spec.bootstrap.initdb.import.databases` | `[]string` | yes |  |  |
| `spec.bootstrap.initdb.import.roles` | `[]string` |  |  |  |
| `spec.bootstrap.initdb.import.schemaOnly` | `bool` |  |  |  |
| `spec.bootstrap.recovery` | `KubernetesPostgresBootstrapRecovery` |  |  |  |
| `spec.bootstrap.recovery.objectStore` | `KubernetesPostgresObjectStore` | yes |  |  |
| `spec.bootstrap.recovery.objectStore.destinationPath` | `string` | yes |  |  |
| `spec.bootstrap.recovery.objectStore.s3` | `KubernetesPostgresS3ObjectStore` |  |  |  |
| `spec.bootstrap.recovery.objectStore.s3.region` | `string` |  |  |  |
| `spec.bootstrap.recovery.objectStore.s3.endpointUrl` | `string` |  |  |  |
| `spec.bootstrap.recovery.objectStore.s3.endpointCaPem` | `string` |  |  |  |
| `spec.bootstrap.recovery.objectStore.s3.keyless` | `bool` |  |  |  |
| `spec.bootstrap.recovery.objectStore.s3.accessKeys` | `KubernetesPostgresS3AccessKeys` |  |  |  |
| `spec.bootstrap.recovery.objectStore.s3.accessKeys.accessKeyId` | `string` | yes |  |  |
| `spec.bootstrap.recovery.objectStore.s3.accessKeys.secretAccessKey` | `string` (sensitive) | yes |  |  |
| `spec.bootstrap.recovery.objectStore.gcs` | `KubernetesPostgresGcsObjectStore` |  |  |  |
| `spec.bootstrap.recovery.objectStore.gcs.keyless` | `bool` |  |  |  |
| `spec.bootstrap.recovery.objectStore.gcs.serviceAccountKeyJson` | `string` (sensitive) |  |  |  |
| `spec.bootstrap.recovery.objectStore.azureBlob` | `KubernetesPostgresAzureBlobObjectStore` |  |  |  |
| `spec.bootstrap.recovery.objectStore.azureBlob.keyless` | `bool` |  |  |  |
| `spec.bootstrap.recovery.objectStore.azureBlob.connectionString` | `string` (sensitive) |  |  |  |
| `spec.bootstrap.recovery.objectStore.azureBlob.storageAccount` | `string` |  |  |  |
| `spec.bootstrap.recovery.objectStore.azureBlob.storageKey` | `string` (sensitive) |  |  |  |
| `spec.bootstrap.recovery.objectStore.r2` | `KubernetesPostgresR2ObjectStore` |  |  |  |
| `spec.bootstrap.recovery.objectStore.r2.accountId` | `string \| valueFrom` | yes |  | CloudflareR2Bucket (`status.outputs.account_id`) |
| `spec.bootstrap.recovery.objectStore.r2.jurisdiction` | `string \| valueFrom` |  |  | CloudflareR2Bucket (`status.outputs.jurisdiction`) |
| `spec.bootstrap.recovery.objectStore.r2.credentials` | `KubernetesPostgresR2Credentials` | yes |  |  |
| `spec.bootstrap.recovery.objectStore.r2.credentials.accessKeyId` | `string \| valueFrom` | yes |  | CloudflareAccountApiToken (`status.outputs.r2_access_key_id`) |
| `spec.bootstrap.recovery.objectStore.r2.credentials.secretAccessKey` | `string \| valueFrom` (sensitive) | yes |  | CloudflareAccountApiToken (`status.outputs.r2_secret_access_key`) |
| `spec.bootstrap.recovery.objectStore.wal` | `KubernetesPostgresWalTuning` |  |  |  |
| `spec.bootstrap.recovery.objectStore.wal.compression` | `string` |  |  |  |
| `spec.bootstrap.recovery.objectStore.wal.maxParallel` | `int32` |  |  |  |
| `spec.bootstrap.recovery.objectStore.data` | `KubernetesPostgresDataTuning` |  |  |  |
| `spec.bootstrap.recovery.objectStore.data.compression` | `string` |  |  |  |
| `spec.bootstrap.recovery.objectStore.data.jobs` | `int32` |  |  |  |
| `spec.bootstrap.recovery.objectStore.data.immediateCheckpoint` | `bool` |  |  |  |
| `spec.bootstrap.recovery.sourceServerName` | `string` | yes |  |  |
| `spec.bootstrap.recovery.recoveryTarget` | `KubernetesPostgresRecoveryTarget` |  |  |  |
| `spec.bootstrap.recovery.recoveryTarget.targetTime` | `string` |  |  |  |
| `spec.bootstrap.recovery.recoveryTarget.targetLsn` | `string` |  |  |  |
| `spec.bootstrap.recovery.recoveryTarget.targetName` | `string` |  |  |  |
| `spec.bootstrap.recovery.recoveryTarget.targetImmediate` | `bool` |  |  |  |
| `spec.bootstrap.recovery.recoveryTarget.backupId` | `string` |  |  |  |
| `spec.bootstrap.recovery.database` | `string` |  |  |  |
| `spec.bootstrap.recovery.owner` | `string` |  |  |  |
| `spec.bootstrap.recovery.ownerSecretName` | `string` |  |  |  |
| `spec.bootstrap.pgBasebackup` | `KubernetesPostgresBootstrapPgBaseBackup` |  |  |  |
| `spec.bootstrap.pgBasebackup.source` | `string` | yes |  |  |
| `spec.externalClusters` | `[]KubernetesPostgresExternalCluster` |  |  |  |
| `spec.externalClusters[].name` | `string` | yes |  |  |
| `spec.externalClusters[].connectionParameters` | `map<string, string>` |  |  |  |
| `spec.externalClusters[].password` | `string` (sensitive) |  |  |  |
| `spec.superuser` | `KubernetesPostgresSuperuser` |  |  |  |
| `spec.superuser.enabled` | `bool` |  |  |  |
| `spec.superuser.password` | `string` (sensitive) |  |  |  |
| `spec.roles` | `[]KubernetesPostgresRole` |  |  |  |
| `spec.roles[].name` | `string` | yes |  |  |
| `spec.roles[].comment` | `string` |  |  |  |
| `spec.roles[].ensure` | `string` |  | `present` |  |
| `spec.roles[].password` | `string` (sensitive) |  |  |  |
| `spec.roles[].disablePassword` | `bool` |  |  |  |
| `spec.roles[].login` | `bool` |  |  |  |
| `spec.roles[].superuser` | `bool` |  |  |  |
| `spec.roles[].createdb` | `bool` |  |  |  |
| `spec.roles[].createrole` | `bool` |  |  |  |
| `spec.roles[].replication` | `bool` |  |  |  |
| `spec.roles[].bypassrls` | `bool` |  |  |  |
| `spec.roles[].inRoles` | `[]string` |  |  |  |
| `spec.roles[].connectionLimit` | `int64` |  | `-1` |  |
| `spec.backup` | `KubernetesPostgresBackup` |  |  |  |
| `spec.backup.objectStore` | `KubernetesPostgresObjectStore` | yes |  |  |
| `spec.backup.objectStore.destinationPath` | `string` | yes |  |  |
| `spec.backup.objectStore.s3` | `KubernetesPostgresS3ObjectStore` |  |  |  |
| `spec.backup.objectStore.s3.region` | `string` |  |  |  |
| `spec.backup.objectStore.s3.endpointUrl` | `string` |  |  |  |
| `spec.backup.objectStore.s3.endpointCaPem` | `string` |  |  |  |
| `spec.backup.objectStore.s3.keyless` | `bool` |  |  |  |
| `spec.backup.objectStore.s3.accessKeys` | `KubernetesPostgresS3AccessKeys` |  |  |  |
| `spec.backup.objectStore.s3.accessKeys.accessKeyId` | `string` | yes |  |  |
| `spec.backup.objectStore.s3.accessKeys.secretAccessKey` | `string` (sensitive) | yes |  |  |
| `spec.backup.objectStore.gcs` | `KubernetesPostgresGcsObjectStore` |  |  |  |
| `spec.backup.objectStore.gcs.keyless` | `bool` |  |  |  |
| `spec.backup.objectStore.gcs.serviceAccountKeyJson` | `string` (sensitive) |  |  |  |
| `spec.backup.objectStore.azureBlob` | `KubernetesPostgresAzureBlobObjectStore` |  |  |  |
| `spec.backup.objectStore.azureBlob.keyless` | `bool` |  |  |  |
| `spec.backup.objectStore.azureBlob.connectionString` | `string` (sensitive) |  |  |  |
| `spec.backup.objectStore.azureBlob.storageAccount` | `string` |  |  |  |
| `spec.backup.objectStore.azureBlob.storageKey` | `string` (sensitive) |  |  |  |
| `spec.backup.objectStore.r2` | `KubernetesPostgresR2ObjectStore` |  |  |  |
| `spec.backup.objectStore.r2.accountId` | `string \| valueFrom` | yes |  | CloudflareR2Bucket (`status.outputs.account_id`) |
| `spec.backup.objectStore.r2.jurisdiction` | `string \| valueFrom` |  |  | CloudflareR2Bucket (`status.outputs.jurisdiction`) |
| `spec.backup.objectStore.r2.credentials` | `KubernetesPostgresR2Credentials` | yes |  |  |
| `spec.backup.objectStore.r2.credentials.accessKeyId` | `string \| valueFrom` | yes |  | CloudflareAccountApiToken (`status.outputs.r2_access_key_id`) |
| `spec.backup.objectStore.r2.credentials.secretAccessKey` | `string \| valueFrom` (sensitive) | yes |  | CloudflareAccountApiToken (`status.outputs.r2_secret_access_key`) |
| `spec.backup.objectStore.wal` | `KubernetesPostgresWalTuning` |  |  |  |
| `spec.backup.objectStore.wal.compression` | `string` |  |  |  |
| `spec.backup.objectStore.wal.maxParallel` | `int32` |  |  |  |
| `spec.backup.objectStore.data` | `KubernetesPostgresDataTuning` |  |  |  |
| `spec.backup.objectStore.data.compression` | `string` |  |  |  |
| `spec.backup.objectStore.data.jobs` | `int32` |  |  |  |
| `spec.backup.objectStore.data.immediateCheckpoint` | `bool` |  |  |  |
| `spec.backup.retentionPolicy` | `string` |  |  |  |
| `spec.backup.schedules` | `[]KubernetesPostgresBackupSchedule` |  |  |  |
| `spec.backup.schedules[].name` | `string` | yes |  |  |
| `spec.backup.schedules[].schedule` | `string` | yes |  |  |
| `spec.backup.schedules[].immediate` | `bool` |  |  |  |
| `spec.backup.schedules[].suspend` | `bool` |  |  |  |
| `spec.backup.schedules[].target` | `string` |  | `prefer-standby` |  |
| `spec.workloadIdentity` | `KubernetesWorkloadIdentity` |  |  |  |
| `spec.workloadIdentity.gke` | `KubernetesWorkloadIdentityGke` |  |  |  |
| `spec.workloadIdentity.gke.serviceAccountEmail` | `string \| valueFrom` | yes |  | GcpServiceAccount (`status.outputs.email`) |
| `spec.workloadIdentity.eks` | `KubernetesWorkloadIdentityEksIrsa` |  |  |  |
| `spec.workloadIdentity.eks.roleArn` | `string \| valueFrom` | yes |  | AwsIamRole (`status.outputs.role_arn`) |
| `spec.workloadIdentity.aks` | `KubernetesWorkloadIdentityAks` |  |  |  |
| `spec.workloadIdentity.aks.clientId` | `string \| valueFrom` | yes |  | AzureUserAssignedIdentity (`status.outputs.client_id`) |
| `spec.workloadIdentity.aks.tenantId` | `string` |  |  |  |
| `spec.certificates` | `KubernetesPostgresCertificates` |  |  |  |
| `spec.certificates.serverTlsSecret` | `string \| valueFrom` |  |  | KubernetesCertificate (`status.outputs.secret_name`) |
| `spec.certificates.serverCaSecret` | `string` |  |  |  |
| `spec.certificates.serverAltDnsNames` | `[]string` |  |  |  |
| `spec.monitoring` | `KubernetesPostgresMonitoring` |  |  |  |
| `spec.monitoring.tlsEnabled` | `bool` |  |  |  |
| `spec.monitoring.disableDefaultQueries` | `bool` |  |  |  |
| `spec.scheduling` | `KubernetesPostgresScheduling` |  |  |  |
| `spec.scheduling.antiAffinityType` | `string` |  | `preferred` |  |
| `spec.scheduling.topologyKey` | `string` |  |  |  |
| `spec.scheduling.nodeSelector` | `map<string, string>` |  |  |  |
| `spec.scheduling.tolerations` | `[]WorkloadToleration` |  |  |  |
| `spec.scheduling.tolerations[].key` | `string` |  |  |  |
| `spec.scheduling.tolerations[].operator` | `string` |  |  |  |
| `spec.scheduling.tolerations[].value` | `string` |  |  |  |
| `spec.scheduling.tolerations[].effect` | `string` |  |  |  |
| `spec.scheduling.tolerations[].tolerationSeconds` | `int64` |  |  |  |
| `spec.scheduling.priorityClassName` | `string` |  |  |  |
| `spec.updateStrategy` | `KubernetesPostgresUpdateStrategy` |  |  |  |
| `spec.updateStrategy.primaryUpdateStrategy` | `string` |  | `unsupervised` |  |
| `spec.updateStrategy.primaryUpdateMethod` | `string` |  | `restart` |  |
| `spec.enablePdb` | `bool` |  | `true` |  |
| `spec.imagePullSecrets` | `[]string` |  |  |  |

## Field Details

### spec.namespace

`string | valueFrom` · required

Namespace to create the PostgreSQL cluster in. Accepts a literal
namespace name or a reference to a KubernetesNamespace resource.

- references: KubernetesNamespace (`spec.name`)
- rule: {"required":true}
- rule: write as {value: <literal>} or {valueFrom: {kind: KubernetesNamespace, name: <that resource's name>, fieldPath: spec.name}} -- a bare string does not parse

### spec.createNamespace

`bool`

When true, the namespace is created (with the standard Planton
governance labels) before the cluster and deleted with the resource.
When false, the namespace must already exist.

### spec.instances

`int32` · optional (explicit presence)

Number of PostgreSQL instances: one primary plus (instances - 1)
streaming replicas. 1 is a single point of failure suitable for
development; 2 gives automated failover; 3 is the production
convention (failover capacity even during maintenance).

- default: `1`
- rule: {"int32":{"gte":1}}

### spec.imageName

`string`

PostgreSQL container image, tag or digest form (e.g.
"ghcr.io/cloudnative-pg/postgresql:17.5"). Empty = the operator's
default image for its release — the recommended posture unless you
need a specific PostgreSQL major/minor or a private mirror. Changing
the image on a live cluster performs a rolling update (minor bumps)
or a major upgrade (major bumps).

### spec.storage

`KubernetesPostgresStorage` · required

Storage for PGDATA — the database files. Required: CloudNativePG
provisions one PVC per instance from this and never lets you shrink
it later (sizes can only grow).

- rule: {"required":true}

### spec.storage.size

`string` · required

Volume size as a Kubernetes quantity (e.g. "10Gi"). Grows are
applied to live PVCs (when the storage class supports expansion);
shrinks are rejected by the operator.

- rule: size must be a Kubernetes quantity like '10Gi' or '500Mi'
- rule: {"required":true}

### spec.storage.storageClass

`string | valueFrom`

StorageClass for the PVCs. Empty = the cluster's default class.
Accepts a literal class name or a reference to a
KubernetesStorageClass resource.

- references: KubernetesStorageClass (`status.outputs.storage_class_name`)
- rule: write as {value: <literal>} or {valueFrom: {kind: KubernetesStorageClass, name: <that resource's name>, fieldPath: status.outputs.storage_class_name}} -- a bare string does not parse

### spec.storage.resizeInUseVolumes

`bool` · optional (explicit presence)

Resize mounted PVCs in place when size grows. Upstream default:
true. Disable for storage classes that cannot expand in-use volumes
(the operator then resizes by recreating instances one at a time).

- default: `true`

### spec.walStorage

`KubernetesPostgresStorage`

Optional dedicated volume for the Write-Ahead Log. Separating WAL
from data puts the sequential WAL writes on their own disk — the
standard I/O-isolation move for write-heavy workloads. Set at
creation; adding it later requires re-creating the instances.

### spec.walStorage.size

`string` · required

Volume size as a Kubernetes quantity (e.g. "10Gi"). Grows are
applied to live PVCs (when the storage class supports expansion);
shrinks are rejected by the operator.

- rule: size must be a Kubernetes quantity like '10Gi' or '500Mi'
- rule: {"required":true}

### spec.walStorage.storageClass

`string | valueFrom`

StorageClass for the PVCs. Empty = the cluster's default class.
Accepts a literal class name or a reference to a
KubernetesStorageClass resource.

- references: KubernetesStorageClass (`status.outputs.storage_class_name`)
- rule: write as {value: <literal>} or {valueFrom: {kind: KubernetesStorageClass, name: <that resource's name>, fieldPath: status.outputs.storage_class_name}} -- a bare string does not parse

### spec.walStorage.resizeInUseVolumes

`bool` · optional (explicit presence)

Resize mounted PVCs in place when size grows. Upstream default:
true. Disable for storage classes that cannot expand in-use volumes
(the operator then resizes by recreating instances one at a time).

- default: `true`

### spec.resources

`ContainerResources`

CPU/memory for every PostgreSQL instance pod. Empty = no
requests/limits (fine for evaluation, not production — the operator
derives PostgreSQL memory tuning hints from the limits).

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

### spec.postgresql

`KubernetesPostgresServerConfig`

The PostgreSQL server configuration: postgresql.conf parameters,
pg_hba/pg_ident rules, preloaded libraries, and synchronous
replication.

### spec.postgresql.parameters

`map<string, string>`

postgresql.conf parameters (e.g. max_connections: "200",
shared_buffers: "256MB"). The operator merges them over its
defaults, validates them, and rolls them out — parameters that
require a restart trigger a rolling restart automatically.
Replication and archiving parameters are operator-managed and
rejected here.

### spec.postgresql.pgHba

`[]string`

Lines appended to pg_hba.conf (client authentication rules),
evaluated before the operator's defaults. The operator always keeps
its own rules for replication and local access.

### spec.postgresql.pgIdent

`[]string`

Lines appended to pg_ident.conf (user name maps).

### spec.postgresql.sharedPreloadLibraries

`[]string`

Libraries added to shared_preload_libraries (e.g. "pg_stat_statements",
"timescaledb"). The library must exist in the image — check the
image's bundled extensions before adding.

### spec.postgresql.synchronous

`KubernetesPostgresSynchronousReplication`

Synchronous replication: transactions wait for replicas before
committing — the zero-data-loss posture. Omitted = asynchronous
replication (a failover may lose the last instants of writes).

### spec.postgresql.synchronous.method

`string` · optional (explicit presence)

How synchronous standbys are selected: "any" (quorum — any `number`
of the standbys acknowledge; the recommended posture) or "first"
(priority order).

- default: `any`
- rule: synchronous method must be 'any' (quorum) or 'first' (priority)

### spec.postgresql.synchronous.number

`int32`

Number of synchronous standbys transactions wait for. Must be lower
than the instance count (a cluster of 3 supports at most 2 — the
primary cannot acknowledge itself).

- rule: {"int32":{"gte":1}}

### spec.postgresql.synchronous.dataDurability

`string` · optional (explicit presence)

What happens when standbys are unavailable: "required" (writes
BLOCK until quorum returns — strict durability, the upstream
default) or "preferred" (the requirement relaxes dynamically —
availability over durability).

- default: `required`
- rule: data_durability must be 'required' (writes block without quorum) or 'preferred' (requirement relaxes when replicas are unavailable)

### spec.postgresql.enableAlterSystem

`bool`

Allow ALTER SYSTEM inside the database. Upstream default: false —
configuration belongs in `parameters` where it is declarative and
survives instance recreation; enable only for debugging.

### spec.bootstrap

`KubernetesPostgresBootstrap`

How the cluster gets its initial state: a fresh empty database
(initdb — the default when omitted), a restore from an object-store
backup (recovery — the disaster-recovery and clone path), physical
streaming from an existing server (pg_basebackup), or initdb plus a
logical import of an existing database (migration from RDS/other
PostgreSQL). Immutable after creation — bootstrap describes how the
cluster was born.

### spec.bootstrap.initdb

`KubernetesPostgresBootstrapInitDb`

A fresh, empty database initialized with initdb — the standard
path for new applications.

### spec.bootstrap.initdb.database

`string` · optional (explicit presence)

Name of the application database to create. Upstream default: "app".

- default: `app`

### spec.bootstrap.initdb.owner

`string`

Name of the role owning the application database. Empty = same as
the database name (the upstream default).

### spec.bootstrap.initdb.ownerPassword

`string` · sensitive

Initial password for the owner role. Empty = the operator generates
one. Either way the credential lands in the `<name>-app` Secret the
outputs point at.

### spec.bootstrap.initdb.dataChecksums

`bool`

Enable data-page checksums (initdb -k): detects silent storage
corruption at a small CPU cost. Upstream default: false; set true
for new production clusters — it cannot be turned on later.

### spec.bootstrap.initdb.encoding

`string` · optional (explicit presence)

Database encoding. Upstream default: UTF8.

- default: `UTF8`

### spec.bootstrap.initdb.localeCollate

`string`

LC_COLLATE for the new database cluster (initdb --lc-collate).
Upstream default: C.

### spec.bootstrap.initdb.localeCtype

`string`

LC_CTYPE for the new database cluster (initdb --lc-ctype).
Upstream default: C.

### spec.bootstrap.initdb.postInitSql

`[]string`

SQL run once as superuser against the `postgres` database right
after creation — extensions, event triggers. Use with care: these
statements run unvalidated.

### spec.bootstrap.initdb.postInitApplicationSql

`[]string`

SQL run once as superuser against the APPLICATION database right
after creation (e.g. CREATE EXTENSION postgis).

### spec.bootstrap.initdb.import

`KubernetesPostgresImport`

Migrate an existing database in during bootstrap: a logical
pg_dump/pg_restore from a declared external cluster — the
cross-version, cross-architecture migration path (works from RDS,
Cloud SQL, self-managed — anything reachable).

- rule: the microservice import shape takes exactly one database — use the monolith shape for several
- rule: importing roles is only available with the monolith shape

### spec.bootstrap.initdb.import.type

`string` · required

Import shape: "microservice" (the listed database is imported INTO
the new application database — the common case) or "monolith"
(databases and roles are recreated one-to-one).

- rule: import type must be 'microservice' (one database into the app database) or 'monolith' (databases and roles recreated one-to-one)
- rule: {"required":true}

### spec.bootstrap.initdb.import.sourceExternalCluster

`string` · required

Name of the declared external cluster to import from.

- rule: {"required":true}

### spec.bootstrap.initdb.import.databases

`[]string` · required

Databases to import. The microservice shape takes exactly one; the
monolith shape takes one or more (or "*" for all).

- rule: {"repeated":{"minItems":"1"}}

### spec.bootstrap.initdb.import.roles

`[]string`

Roles to import (monolith shape only).

### spec.bootstrap.initdb.import.schemaOnly

`bool`

Import schema only — skip table data (a structural clone).

### spec.bootstrap.recovery

`KubernetesPostgresBootstrapRecovery`

Restore from an object-store backup — disaster recovery, cloning,
and point-in-time recovery. The new cluster starts from the backup
and replays WAL to the requested target.

### spec.bootstrap.recovery.objectStore

`KubernetesPostgresObjectStore` · required

The object store holding the source cluster's backups. Rendered as a
SECOND Barman Cloud ObjectStore resource (`<name>-recovery-source`)
— recovery reads from here while the backup block (if any) writes
the new cluster's own backups elsewhere. Never point both at the
same destination path: the new cluster would overwrite the archive
it restored from.

- rule: {"required":true}
- rule: the s3 backend stores at an s3:// destination path (also for S3-compatible stores like MinIO and R2)
- rule: the gcs backend stores at a gs:// destination path
- rule: the azure_blob backend stores at an https:// destination path (https://<account>.blob.core.windows.net/<container>/<path>)
- rule: the r2 backend stores at an s3:// destination path (s3://<bucket>/<path> — R2 is addressed through its S3 API; the bucket name is the CloudflareR2Bucket's bucket_name)

### spec.bootstrap.recovery.objectStore.destinationPath

`string` · required

Where in the store the data lives — the backend's native URI form:
`s3://bucket/path` for S3, Cloudflare R2, and every S3-compatible
store, `gs://bucket/path` for GCS, and
`https://<account>.blob.core.windows.net/<container>/<path>` for
Azure Blob. WAL and base backups are stored under separate folders
beneath it. One path per PostgreSQL cluster, FOREVER: Barman refuses
to archive into a path already holding another cluster's WAL (a
cluster recreated under the same path after a failed attempt, or a
recovered cluster backing up to the path it restored from), and the
failure is quiet — the cluster reports healthy while the
ContinuousArchiving condition stays false and no backup ever lands
(live-caught). A recovered cluster's own backups go to a NEW path.

- rule: {"required":true}

### spec.bootstrap.recovery.objectStore.s3

`KubernetesPostgresS3ObjectStore`

AWS S3 — or ANY S3-compatible store (MinIO, Ceph RGW, DigitalOcean
Spaces, ...) via the endpoint_url override. Cloudflare R2 has its
own arm (`r2`) that composes the catalog's Cloudflare kinds; this
arm still reaches R2 for a hand-carried endpoint and key pair.

- rule: keyless and access_keys are alternative credential postures — set exactly one
- rule: an S3-compatible endpoint (endpoint_url) authenticates with access_keys — the keyless posture only mints AWS credentials

### spec.bootstrap.recovery.objectStore.s3.region

`string`

AWS region of the bucket. Required for real S3; for S3-compatible
stores use the store's expected value (MinIO accepts any; the `r2`
arm pins Cloudflare R2's `auto` itself).

### spec.bootstrap.recovery.objectStore.s3.endpointUrl

`string`

S3-COMPATIBLE ARM: endpoint URL of the store (e.g.
http://minio.minio-system.svc:9000 for in-cluster MinIO). Empty =
real AWS S3. For Cloudflare R2 prefer the `r2` arm, which composes
the endpoint from the bucket's account and jurisdiction.

- rule: endpoint_url must be an http(s) URL (e.g. http://minio.minio-system.svc:9000)

### spec.bootstrap.recovery.objectStore.s3.endpointCaPem

`string`

PEM CA bundle for verifying a self-signed endpoint_url TLS
certificate (materialized as a Secret the plugin reads).

### spec.bootstrap.recovery.objectStore.s3.keyless

`bool`

Keyless posture: the instance pods' AWS identity (IRSA via the
cluster's workload_identity field, or node instance-profile
credentials) authenticates to S3 — no stored keys. Mutually
exclusive with access_keys.

### spec.bootstrap.recovery.objectStore.s3.accessKeys

`KubernetesPostgresS3AccessKeys`

Static access keys, materialized as a Kubernetes Secret the plugin
reads. The declared-credential arm — for S3-compatible stores and
clusters without IRSA.

### spec.bootstrap.recovery.objectStore.s3.accessKeys.accessKeyId

`string` · required

Access key ID — the public identifier of the key pair, not a
secret; only the paired secret access key is a credential. For
MinIO this is the access key / username.

- rule: {"required":true}

### spec.bootstrap.recovery.objectStore.s3.accessKeys.secretAccessKey

`string` · required · sensitive

Secret access key (for MinIO: the secret key / password).

- rule: {"required":true}

### spec.bootstrap.recovery.objectStore.gcs

`KubernetesPostgresGcsObjectStore`

Google Cloud Storage.

- rule: keyless and service_account_key_json are alternative credential postures — set exactly one

### spec.bootstrap.recovery.objectStore.gcs.keyless

`bool`

Keyless posture: the instance pods' GCP identity (GKE Workload
Identity via the cluster's workload_identity field) authenticates
to GCS — no stored key. Mutually exclusive with
service_account_key_json.

THE IDENTITY NEEDS TWO ROLES ON THE BUCKET, not one: Barman Cloud
verifies the archive destination with a bucket-level read
(`storage.buckets.get`) before every WAL archive, and
`roles/storage.objectAdmin` does not carry it — an identity granted
objectAdmin alone fails every archive with "does not have
storage.buckets.get access", the cluster reports
ContinuousArchivingFailing, and never becomes Ready. Grant
`roles/storage.objectAdmin` AND `roles/storage.legacyBucketReader`
(a GcpGcsBucket's `iam_members`, one entry each). Live-verified on
GKE.

### spec.bootstrap.recovery.objectStore.gcs.serviceAccountKeyJson

`string` · sensitive

GCP service-account key (the JSON key file's content), materialized
as a Kubernetes Secret the plugin reads. The declared-credential
arm for non-GKE clusters backing up to GCS.

### spec.bootstrap.recovery.objectStore.azureBlob

`KubernetesPostgresAzureBlobObjectStore`

Azure Blob Storage.

- rule: set exactly one Azure credential posture: keyless, connection_string, or storage_account + storage_key
- rule: storage_key authenticates a specific account — set storage_account with it
- rule: the keyless posture still needs storage_account — it identifies the storage endpoint

### spec.bootstrap.recovery.objectStore.azureBlob.keyless

`bool`

Keyless posture: the instance pods' Azure identity (AKS Workload
Identity via the cluster's workload_identity field, or a managed
identity) authenticates to Blob Storage — no stored secret.
Mutually exclusive with the declared-credential fields.

### spec.bootstrap.recovery.objectStore.azureBlob.connectionString

`string` · sensitive

Storage-account connection string — the all-in-one declared
credential. Mutually exclusive with storage_key.

### spec.bootstrap.recovery.objectStore.azureBlob.storageAccount

`string`

Storage-account name, paired with storage_key. Also required with
keyless (the account identifies the storage endpoint).

### spec.bootstrap.recovery.objectStore.azureBlob.storageKey

`string` · sensitive

Storage-account access key, paired with storage_account.

### spec.bootstrap.recovery.objectStore.r2

`KubernetesPostgresR2ObjectStore`

Cloudflare R2, in R2's own vocabulary: the owning account, the
bucket's jurisdiction, and a Cloudflare credential — each a
reference onto the catalog's CloudflareR2Bucket and
CloudflareAccountApiToken by default. The module performs the S3
translation R2 needs (the jurisdiction's endpoint host, region
`auto`, the token as an S3 key pair); nothing S3-shaped is typed
here.

### spec.bootstrap.recovery.objectStore.r2.accountId

`string | valueFrom` · required

The Cloudflare account that owns the bucket (32 hex characters). By
reference to the bucket resource's `account_id` output, so the arm
follows the bucket; a literal names an account outside the catalog.

- references: CloudflareR2Bucket (`status.outputs.account_id`)
- rule: account_id is the 32-hex-character Cloudflare account id
- rule: {"required":true}
- rule: write as {value: <literal>} or {valueFrom: {kind: CloudflareR2Bucket, name: <that resource's name>, fieldPath: status.outputs.account_id}} -- a bare string does not parse

### spec.bootstrap.recovery.objectStore.r2.jurisdiction

`string | valueFrom`

The bucket's data-residency jurisdiction: `default` (or empty), `eu`,
`fedramp`, or `us`. It selects the S3 host the module composes — a
bucket created in a jurisdiction is unreachable through any other
host — so it must match the bucket exactly; by reference to the
bucket resource's `jurisdiction` output it cannot drift.

- references: CloudflareR2Bucket (`status.outputs.jurisdiction`)
- rule: jurisdiction must be one of "default", "eu", "fedramp", "us" (or empty for default)
- rule: write as {value: <literal>} or {valueFrom: {kind: CloudflareR2Bucket, name: <that resource's name>, fieldPath: status.outputs.jurisdiction}} -- a bare string does not parse

### spec.bootstrap.recovery.objectStore.r2.credentials

`KubernetesPostgresR2Credentials` · required

The Cloudflare credential, as the S3 key pair R2's S3 API
authenticates. Materialized as a Kubernetes Secret the plugin reads;
never plaintext in the rendered resource.

- rule: {"required":true}

### spec.bootstrap.recovery.objectStore.r2.credentials.accessKeyId

`string | valueFrom` · required

The S3 access key id: the API token's id. By reference to the token
resource's `r2_access_key_id` output.

- references: CloudflareAccountApiToken (`status.outputs.r2_access_key_id`)
- rule: {"required":true}
- rule: write as {value: <literal>} or {valueFrom: {kind: CloudflareAccountApiToken, name: <that resource's name>, fieldPath: status.outputs.r2_access_key_id}} -- a bare string does not parse

### spec.bootstrap.recovery.objectStore.r2.credentials.secretAccessKey

`string | valueFrom` · required · sensitive

The S3 secret access key: the SHA-256 of the API token's value. By
reference to the token resource's `r2_secret_access_key` output. Rotates
with the token: a rotated token is a new key pair, and the Secret this
arm materializes follows the reference on the next apply.

- references: CloudflareAccountApiToken (`status.outputs.r2_secret_access_key`)
- rule: {"required":true}
- rule: write as {value: <literal>} or {valueFrom: {kind: CloudflareAccountApiToken, name: <that resource's name>, fieldPath: status.outputs.r2_secret_access_key}} -- a bare string does not parse

### spec.bootstrap.recovery.objectStore.wal

`KubernetesPostgresWalTuning`

WAL archiving tuning: compression and parallelism of the continuous
WAL stream.

### spec.bootstrap.recovery.objectStore.wal.compression

`string`

Compress WAL segments before upload: gzip, bzip2, lz4, snappy, xz,
or zstd. Empty = uncompressed. zstd is the modern
speed/ratio sweet spot.

- rule: WAL compression must be one of gzip, bzip2, lz4, snappy, xz, zstd (or empty for none)

### spec.bootstrap.recovery.objectStore.wal.maxParallel

`int32` · optional (explicit presence)

WAL segments archived (or restored, on a standby) in parallel.
Empty/1 = one at a time; raise when archiving lags write volume.

- rule: {"int32":{"gte":1}}

### spec.bootstrap.recovery.objectStore.data

`KubernetesPostgresDataTuning`

Base-backup tuning: compression and upload parallelism of the
periodic full copies.

### spec.bootstrap.recovery.objectStore.data.compression

`string`

Compress base-backup tarballs: gzip, bzip2, lz4, or snappy. Empty =
uncompressed.

- rule: data compression must be one of gzip, bzip2, lz4, snappy (or empty for none)

### spec.bootstrap.recovery.objectStore.data.jobs

`int32` · optional (explicit presence)

Parallel upload jobs for base backups. Upstream default: 2.

- rule: {"int32":{"gte":1}}

### spec.bootstrap.recovery.objectStore.data.immediateCheckpoint

`bool`

Start the backup with an immediate checkpoint instead of spreading
the checkpoint I/O (faster backup start, heavier I/O spike).

### spec.bootstrap.recovery.sourceServerName

`string` · required

Name the SOURCE cluster's data is stored under in the object store
(its Cluster name, unless its backups declared a server_name
override).

- rule: {"required":true}

### spec.bootstrap.recovery.recoveryTarget

`KubernetesPostgresRecoveryTarget`

Stop replaying WAL at a point in time instead of recovering
everything (PITR). Omitted = full recovery to the archive's end.

- rule: set at most one recovery target selector (target_time, target_lsn, target_name, or target_immediate)

### spec.bootstrap.recovery.recoveryTarget.targetTime

`string`

Recover to this timestamp (RFC3339, e.g. "2026-07-20T06:00:00Z").

### spec.bootstrap.recovery.recoveryTarget.targetLsn

`string`

Recover to this Log Sequence Number.

### spec.bootstrap.recovery.recoveryTarget.targetName

`string`

Recover to a named restore point (pg_create_restore_point).

### spec.bootstrap.recovery.recoveryTarget.targetImmediate

`bool`

Recover to the first consistent state — the shortest possible
recovery.

### spec.bootstrap.recovery.recoveryTarget.backupId

`string`

Restore from this specific backup ID instead of auto-selecting the
closest one before the target.

### spec.bootstrap.recovery.database

`string`

Name of the application database inside the recovered instance —
the one the `<name>-app` credential and the `uri` outputs point
at. Set it to the SOURCE cluster's application database (its
initdb `database`). Empty = the upstream default, `app`.

### spec.bootstrap.recovery.owner

`string`

Name of the role that owns the application database. Set it to the
source cluster's owner role. Empty = same as `database` (the
upstream default).

### spec.bootstrap.recovery.ownerSecretName

`string`

CREDENTIAL CONTINUITY: the recovered data carries the source
cluster's roles and their passwords, so the application credential
this cluster hands out must be the source's. Name an existing
basic-auth Secret in this namespace (`username` + `password` keys —
exactly the shape of the source's `<source>-app` Secret, so keeping
that Secret alive through a KubernetesSecret, an ExternalSecret, or
the secret backend is the whole backup of the credential). The
operator adopts it as this cluster's app Secret and the outputs
point at it. Empty = the operator generates a fresh `<name>-app`
with a NEW password and resets the owner role to match — fine for
a clone that will get its own consumers, wrong for a recovery that
must serve the source's.

### spec.bootstrap.pgBasebackup

`KubernetesPostgresBootstrapPgBaseBackup`

Physical streaming replication from an existing PostgreSQL server
(a declared external cluster) — the binary-identical migration
path (same major version, same architecture).

### spec.bootstrap.pgBasebackup.source

`string` · required

Name of the declared external cluster to stream from. The source
must be the SAME PostgreSQL major version and accept a replication
connection from the new cluster's pods.

- rule: {"required":true}

### spec.externalClusters

`[]KubernetesPostgresExternalCluster`

Connection descriptors for EXTERNAL PostgreSQL servers referenced by
bootstrap methods (pg_basebackup source, initdb import source). Each
entry's declared password materializes as a Kubernetes Secret; the
operator builds the connection string from the parameters.

- rule: each external cluster needs a distinct name

### spec.externalClusters[].name

`string` · required

Name bootstrap methods reference this server by.

- rule: {"required":true}

### spec.externalClusters[].connectionParameters

`map<string, string>`

libpq connection parameters: host, port, user, dbname, sslmode, ...
(never put a password here — declare it below so it lands in a
Secret).

### spec.externalClusters[].password

`string` · sensitive

Password for the connection user, materialized as a Kubernetes
Secret and referenced by the operator — never plaintext in the
rendered Cluster resource.

### spec.superuser

`KubernetesPostgresSuperuser`

The `postgres` superuser posture. Disabled by default (the upstream
default): the superuser password is blanked and everything runs
through the application owner role. Enable only when something
genuinely needs superuser SQL access.

- rule: a superuser password only applies when superuser access is enabled — the operator blanks the password otherwise

### spec.superuser.enabled

`bool`

Enable superuser access: the operator maintains the `postgres`
password in the `<name>-superuser` Secret. Upstream default: false
(password blanked; the Secret is removed).

### spec.superuser.password

`string` · sensitive

Explicit superuser password. Empty = the operator generates one.
Only meaningful with enabled true.

### spec.roles

`[]KubernetesPostgresRole`

Declarative database roles beyond the application owner: the
operator creates them, keeps their attributes reconciled, and (when
a password is declared) manages their credential Secrets.

- rule: each role needs a distinct name
- rule: password and disable_password are mutually exclusive — a role cannot both have a managed password and a NULL password

### spec.roles[].name

`string` · required

Role name.

- rule: {"required":true}

### spec.roles[].comment

`string`

COMMENT ON ROLE text.

### spec.roles[].ensure

`string` · optional (explicit presence)

Ensure the role is "present" (created and kept reconciled — the
default) or "absent" (dropped).

- default: `present`
- rule: ensure must be 'present' or 'absent'

### spec.roles[].password

`string` · sensitive

Password for the role, materialized as a `kubernetes.io/basic-auth`
Secret the operator watches — rotating the value rotates the
database password. Empty with disable_password false = the role has
no managed password.

### spec.roles[].disablePassword

`bool`

Force the role's password to NULL in PostgreSQL (locks out password
logins — for roles authenticated by certificate or used only via
SET ROLE). Mutually exclusive with password.

### spec.roles[].login

`bool`

LOGIN attribute: the role can open connections (a "user"). Roles
without it are privilege groups.

### spec.roles[].superuser

`bool`

SUPERUSER attribute — bypasses every permission check. Rarely the
answer; prefer targeted grants.

### spec.roles[].createdb

`bool`

CREATEDB attribute.

### spec.roles[].createrole

`bool`

CREATEROLE attribute.

### spec.roles[].replication

`bool`

REPLICATION attribute — needed for physical/logical replication
connections. Highly privileged.

### spec.roles[].bypassrls

`bool`

BYPASSRLS attribute — bypasses row-level security policies.

### spec.roles[].inRoles

`[]string`

Existing roles this role is granted membership in.

### spec.roles[].connectionLimit

`int64` · optional (explicit presence)

Maximum concurrent connections for the role. Upstream default: -1
(unlimited).

- default: `-1`
- rule: {"int64":{"gte":"-1"}}

### spec.backup

`KubernetesPostgresBackup`

Continuous backup: WAL archiving plus scheduled base backups to an
object store, via the Barman Cloud plugin. Omitted = no backups (a
deliberate choice to make, not a default to forget). Requires
KubernetesCnpgBarmanCloudPlugin installed in the operator's namespace
BEFORE this block is declared: the Cluster is rendered against that
plugin and the operator will not reconcile it until the plugin is
discovered (phase "unknown plugin being required" in `kubectl get
cluster`).

### spec.backup.objectStore

`KubernetesPostgresObjectStore` · required

The object store backups land in (rendered as a Barman Cloud
ObjectStore resource named after this cluster). WAL archiving into
it starts as soon as the cluster is healthy — the backup schedules
below add the periodic base backups PITR needs.

- rule: {"required":true}
- rule: the s3 backend stores at an s3:// destination path (also for S3-compatible stores like MinIO and R2)
- rule: the gcs backend stores at a gs:// destination path
- rule: the azure_blob backend stores at an https:// destination path (https://<account>.blob.core.windows.net/<container>/<path>)
- rule: the r2 backend stores at an s3:// destination path (s3://<bucket>/<path> — R2 is addressed through its S3 API; the bucket name is the CloudflareR2Bucket's bucket_name)

### spec.backup.objectStore.destinationPath

`string` · required

Where in the store the data lives — the backend's native URI form:
`s3://bucket/path` for S3, Cloudflare R2, and every S3-compatible
store, `gs://bucket/path` for GCS, and
`https://<account>.blob.core.windows.net/<container>/<path>` for
Azure Blob. WAL and base backups are stored under separate folders
beneath it. One path per PostgreSQL cluster, FOREVER: Barman refuses
to archive into a path already holding another cluster's WAL (a
cluster recreated under the same path after a failed attempt, or a
recovered cluster backing up to the path it restored from), and the
failure is quiet — the cluster reports healthy while the
ContinuousArchiving condition stays false and no backup ever lands
(live-caught). A recovered cluster's own backups go to a NEW path.

- rule: {"required":true}

### spec.backup.objectStore.s3

`KubernetesPostgresS3ObjectStore`

AWS S3 — or ANY S3-compatible store (MinIO, Ceph RGW, DigitalOcean
Spaces, ...) via the endpoint_url override. Cloudflare R2 has its
own arm (`r2`) that composes the catalog's Cloudflare kinds; this
arm still reaches R2 for a hand-carried endpoint and key pair.

- rule: keyless and access_keys are alternative credential postures — set exactly one
- rule: an S3-compatible endpoint (endpoint_url) authenticates with access_keys — the keyless posture only mints AWS credentials

### spec.backup.objectStore.s3.region

`string`

AWS region of the bucket. Required for real S3; for S3-compatible
stores use the store's expected value (MinIO accepts any; the `r2`
arm pins Cloudflare R2's `auto` itself).

### spec.backup.objectStore.s3.endpointUrl

`string`

S3-COMPATIBLE ARM: endpoint URL of the store (e.g.
http://minio.minio-system.svc:9000 for in-cluster MinIO). Empty =
real AWS S3. For Cloudflare R2 prefer the `r2` arm, which composes
the endpoint from the bucket's account and jurisdiction.

- rule: endpoint_url must be an http(s) URL (e.g. http://minio.minio-system.svc:9000)

### spec.backup.objectStore.s3.endpointCaPem

`string`

PEM CA bundle for verifying a self-signed endpoint_url TLS
certificate (materialized as a Secret the plugin reads).

### spec.backup.objectStore.s3.keyless

`bool`

Keyless posture: the instance pods' AWS identity (IRSA via the
cluster's workload_identity field, or node instance-profile
credentials) authenticates to S3 — no stored keys. Mutually
exclusive with access_keys.

### spec.backup.objectStore.s3.accessKeys

`KubernetesPostgresS3AccessKeys`

Static access keys, materialized as a Kubernetes Secret the plugin
reads. The declared-credential arm — for S3-compatible stores and
clusters without IRSA.

### spec.backup.objectStore.s3.accessKeys.accessKeyId

`string` · required

Access key ID — the public identifier of the key pair, not a
secret; only the paired secret access key is a credential. For
MinIO this is the access key / username.

- rule: {"required":true}

### spec.backup.objectStore.s3.accessKeys.secretAccessKey

`string` · required · sensitive

Secret access key (for MinIO: the secret key / password).

- rule: {"required":true}

### spec.backup.objectStore.gcs

`KubernetesPostgresGcsObjectStore`

Google Cloud Storage.

- rule: keyless and service_account_key_json are alternative credential postures — set exactly one

### spec.backup.objectStore.gcs.keyless

`bool`

Keyless posture: the instance pods' GCP identity (GKE Workload
Identity via the cluster's workload_identity field) authenticates
to GCS — no stored key. Mutually exclusive with
service_account_key_json.

THE IDENTITY NEEDS TWO ROLES ON THE BUCKET, not one: Barman Cloud
verifies the archive destination with a bucket-level read
(`storage.buckets.get`) before every WAL archive, and
`roles/storage.objectAdmin` does not carry it — an identity granted
objectAdmin alone fails every archive with "does not have
storage.buckets.get access", the cluster reports
ContinuousArchivingFailing, and never becomes Ready. Grant
`roles/storage.objectAdmin` AND `roles/storage.legacyBucketReader`
(a GcpGcsBucket's `iam_members`, one entry each). Live-verified on
GKE.

### spec.backup.objectStore.gcs.serviceAccountKeyJson

`string` · sensitive

GCP service-account key (the JSON key file's content), materialized
as a Kubernetes Secret the plugin reads. The declared-credential
arm for non-GKE clusters backing up to GCS.

### spec.backup.objectStore.azureBlob

`KubernetesPostgresAzureBlobObjectStore`

Azure Blob Storage.

- rule: set exactly one Azure credential posture: keyless, connection_string, or storage_account + storage_key
- rule: storage_key authenticates a specific account — set storage_account with it
- rule: the keyless posture still needs storage_account — it identifies the storage endpoint

### spec.backup.objectStore.azureBlob.keyless

`bool`

Keyless posture: the instance pods' Azure identity (AKS Workload
Identity via the cluster's workload_identity field, or a managed
identity) authenticates to Blob Storage — no stored secret.
Mutually exclusive with the declared-credential fields.

### spec.backup.objectStore.azureBlob.connectionString

`string` · sensitive

Storage-account connection string — the all-in-one declared
credential. Mutually exclusive with storage_key.

### spec.backup.objectStore.azureBlob.storageAccount

`string`

Storage-account name, paired with storage_key. Also required with
keyless (the account identifies the storage endpoint).

### spec.backup.objectStore.azureBlob.storageKey

`string` · sensitive

Storage-account access key, paired with storage_account.

### spec.backup.objectStore.r2

`KubernetesPostgresR2ObjectStore`

Cloudflare R2, in R2's own vocabulary: the owning account, the
bucket's jurisdiction, and a Cloudflare credential — each a
reference onto the catalog's CloudflareR2Bucket and
CloudflareAccountApiToken by default. The module performs the S3
translation R2 needs (the jurisdiction's endpoint host, region
`auto`, the token as an S3 key pair); nothing S3-shaped is typed
here.

### spec.backup.objectStore.r2.accountId

`string | valueFrom` · required

The Cloudflare account that owns the bucket (32 hex characters). By
reference to the bucket resource's `account_id` output, so the arm
follows the bucket; a literal names an account outside the catalog.

- references: CloudflareR2Bucket (`status.outputs.account_id`)
- rule: account_id is the 32-hex-character Cloudflare account id
- rule: {"required":true}
- rule: write as {value: <literal>} or {valueFrom: {kind: CloudflareR2Bucket, name: <that resource's name>, fieldPath: status.outputs.account_id}} -- a bare string does not parse

### spec.backup.objectStore.r2.jurisdiction

`string | valueFrom`

The bucket's data-residency jurisdiction: `default` (or empty), `eu`,
`fedramp`, or `us`. It selects the S3 host the module composes — a
bucket created in a jurisdiction is unreachable through any other
host — so it must match the bucket exactly; by reference to the
bucket resource's `jurisdiction` output it cannot drift.

- references: CloudflareR2Bucket (`status.outputs.jurisdiction`)
- rule: jurisdiction must be one of "default", "eu", "fedramp", "us" (or empty for default)
- rule: write as {value: <literal>} or {valueFrom: {kind: CloudflareR2Bucket, name: <that resource's name>, fieldPath: status.outputs.jurisdiction}} -- a bare string does not parse

### spec.backup.objectStore.r2.credentials

`KubernetesPostgresR2Credentials` · required

The Cloudflare credential, as the S3 key pair R2's S3 API
authenticates. Materialized as a Kubernetes Secret the plugin reads;
never plaintext in the rendered resource.

- rule: {"required":true}

### spec.backup.objectStore.r2.credentials.accessKeyId

`string | valueFrom` · required

The S3 access key id: the API token's id. By reference to the token
resource's `r2_access_key_id` output.

- references: CloudflareAccountApiToken (`status.outputs.r2_access_key_id`)
- rule: {"required":true}
- rule: write as {value: <literal>} or {valueFrom: {kind: CloudflareAccountApiToken, name: <that resource's name>, fieldPath: status.outputs.r2_access_key_id}} -- a bare string does not parse

### spec.backup.objectStore.r2.credentials.secretAccessKey

`string | valueFrom` · required · sensitive

The S3 secret access key: the SHA-256 of the API token's value. By
reference to the token resource's `r2_secret_access_key` output. Rotates
with the token: a rotated token is a new key pair, and the Secret this
arm materializes follows the reference on the next apply.

- references: CloudflareAccountApiToken (`status.outputs.r2_secret_access_key`)
- rule: {"required":true}
- rule: write as {value: <literal>} or {valueFrom: {kind: CloudflareAccountApiToken, name: <that resource's name>, fieldPath: status.outputs.r2_secret_access_key}} -- a bare string does not parse

### spec.backup.objectStore.wal

`KubernetesPostgresWalTuning`

WAL archiving tuning: compression and parallelism of the continuous
WAL stream.

### spec.backup.objectStore.wal.compression

`string`

Compress WAL segments before upload: gzip, bzip2, lz4, snappy, xz,
or zstd. Empty = uncompressed. zstd is the modern
speed/ratio sweet spot.

- rule: WAL compression must be one of gzip, bzip2, lz4, snappy, xz, zstd (or empty for none)

### spec.backup.objectStore.wal.maxParallel

`int32` · optional (explicit presence)

WAL segments archived (or restored, on a standby) in parallel.
Empty/1 = one at a time; raise when archiving lags write volume.

- rule: {"int32":{"gte":1}}

### spec.backup.objectStore.data

`KubernetesPostgresDataTuning`

Base-backup tuning: compression and upload parallelism of the
periodic full copies.

### spec.backup.objectStore.data.compression

`string`

Compress base-backup tarballs: gzip, bzip2, lz4, or snappy. Empty =
uncompressed.

- rule: data compression must be one of gzip, bzip2, lz4, snappy (or empty for none)

### spec.backup.objectStore.data.jobs

`int32` · optional (explicit presence)

Parallel upload jobs for base backups. Upstream default: 2.

- rule: {"int32":{"gte":1}}

### spec.backup.objectStore.data.immediateCheckpoint

`bool`

Start the backup with an immediate checkpoint instead of spreading
the checkpoint I/O (faster backup start, heavier I/O spike).

### spec.backup.retentionPolicy

`string`

How long backups and WAL are kept, as `<n>d|w|m` (e.g. "30d").
Empty = keep forever. Enforced by the plugin after each backup.

- rule: retention_policy must be a positive number of days, weeks, or months — e.g. '30d', '8w', or '6m'

### spec.backup.schedules

`[]KubernetesPostgresBackupSchedule`

Scheduled base backups (each rendered as a ScheduledBackup
resource). At least one schedule is what makes point-in-time
recovery real — WAL alone cannot be replayed without a base backup
to start from.

- rule: each backup schedule needs a distinct name

### spec.backup.schedules[].name

`string` · required

Schedule name (also the ScheduledBackup resource's name suffix:
`<cluster>-<name>`).

- rule: schedule name must be a lowercase DNS label (letters, numbers, hyphens)
- rule: {"required":true}

### spec.backup.schedules[].schedule

`string` · required

Cron expression WITH SECONDS — six fields, not the five Kubernetes
CronJobs use (e.g. "0 0 2 * * *" = daily at 02:00). The leading
field is seconds.

- rule: schedule is a SIX-field cron expression (seconds first) — e.g. '0 0 2 * * *' for daily at 02:00; the five-field Kubernetes form is missing the seconds field
- rule: {"required":true}

### spec.backup.schedules[].immediate

`bool`

Take the first backup immediately on creation instead of waiting
for the first cron tick — recommended: the cluster is unprotected
until its first base backup exists. WAL archiving alone restores
nothing — a recovery needs a base backup to replay WAL onto.

### spec.backup.schedules[].suspend

`bool`

Suspend the schedule (keeps the resource, stops the backups).

### spec.backup.schedules[].target

`string` · optional (explicit presence)

Which instance runs the backup: "prefer-standby" (the upstream
default — keeps load off the primary) or "primary" (required for
single-instance clusters to still take backups... a standby is
preferred, not required, so prefer-standby also works with one
instance).

- default: `prefer-standby`
- rule: target must be 'prefer-standby' or 'primary'

### spec.workloadIdentity

`KubernetesWorkloadIdentity`

Keyless cloud identity for the instance pods' ServiceAccount —
annotates it so backups reach S3 (EKS IRSA), GCS (GKE Workload
Identity), or Azure Blob (AKS Workload Identity) without stored
keys. Pair with the backup block's keyless arm. The ServiceAccount
the operator creates is named after the cluster (`metadata.name`,
in `namespace`), so the cloud-side binding names exactly that pair —
on GKE a GcpGkeWorkloadIdentityBinding with `ksa_name` = this
cluster's name and `ksa_namespace` = its namespace, one per cluster
(a recovery target is another cluster and needs its own). On GKE the
GCP service account needs `roles/storage.objectAdmin` AND
`roles/storage.legacyBucketReader` on the bucket (live-proven; see
the gcs.keyless field).

### spec.workloadIdentity.gke

`KubernetesWorkloadIdentityGke`

GKE Workload Identity: annotate the ServiceAccount with a GCP service account email.

### spec.workloadIdentity.gke.serviceAccountEmail

`string | valueFrom` · required

GCP service account email, e.g. "dns-manager@my-project.iam.gserviceaccount.com".
Applied as the `iam.gke.io/gcp-service-account` annotation. Accepts a literal
email or a reference to a GcpServiceAccount resource's output.

- references: GcpServiceAccount (`status.outputs.email`)
- rule: {"required":true}
- rule: write as {value: <literal>} or {valueFrom: {kind: GcpServiceAccount, name: <that resource's name>, fieldPath: status.outputs.email}} -- a bare string does not parse

### spec.workloadIdentity.eks

`KubernetesWorkloadIdentityEksIrsa`

EKS IRSA: annotate the ServiceAccount with an AWS IAM role ARN.

### spec.workloadIdentity.eks.roleArn

`string | valueFrom` · required

AWS IAM role ARN, e.g. "arn:aws:iam::123456789012:role/dns-manager".
Applied as the `eks.amazonaws.com/role-arn` annotation. Accepts a literal ARN
or a reference to an AwsIamRole resource's output.

- references: AwsIamRole (`status.outputs.role_arn`)
- rule: {"required":true}
- rule: write as {value: <literal>} or {valueFrom: {kind: AwsIamRole, name: <that resource's name>, fieldPath: status.outputs.role_arn}} -- a bare string does not parse

### spec.workloadIdentity.aks

`KubernetesWorkloadIdentityAks`

Azure AD Workload Identity: annotate the ServiceAccount with an Entra application
(or user-assigned managed identity) client ID.

### spec.workloadIdentity.aks.clientId

`string | valueFrom` · required

Client ID (GUID) of the user-assigned managed identity or Entra application.
Applied as the `azure.workload.identity/client-id` annotation. Accepts a literal
GUID or a reference to an AzureUserAssignedIdentity resource's output.

- references: AzureUserAssignedIdentity (`status.outputs.client_id`)
- rule: {"required":true}
- rule: write as {value: <literal>} or {valueFrom: {kind: AzureUserAssignedIdentity, name: <that resource's name>, fieldPath: status.outputs.client_id}} -- a bare string does not parse

### spec.workloadIdentity.aks.tenantId

`string` · optional (explicit presence)

Entra tenant ID (GUID). Optional: only needed for cross-tenant scenarios; when
omitted the azure-workload-identity webhook uses its default tenant. Applied as
the `azure.workload.identity/tenant-id` annotation when set.

- rule: {"string":{"uuid":true}}

### spec.certificates

`KubernetesPostgresCertificates`

TLS certificates for client connections. By default the operator
self-signs a CA and server certificate per cluster; point
server_tls at a cert-manager-issued Certificate to serve a
organization-trusted chain.

- rule: server_tls_secret requires server_ca_secret — clients need the CA that signed the server certificate

### spec.certificates.serverTlsSecret

`string | valueFrom`

Name of a kubernetes.io/tls Secret with the SERVER certificate and
key. Accepts a literal Secret name or a reference to a
KubernetesCertificate resource (the cert-manager seam — issue the
certificate declaratively and wire it here). Empty = the operator
self-signs per cluster.

- references: KubernetesCertificate (`status.outputs.secret_name`)
- rule: write as {value: <literal>} or {valueFrom: {kind: KubernetesCertificate, name: <that resource's name>, fieldPath: status.outputs.secret_name}} -- a bare string does not parse

### spec.certificates.serverCaSecret

`string`

Name of the Secret with the CA that signed the server certificate
(`ca.crt` key) — clients verify against it. Required with
server_tls_secret.

### spec.certificates.serverAltDnsNames

`[]string`

Additional DNS names for the operator-generated server certificate
(external hostnames the database is reached at through composed
exposure).

### spec.monitoring

`KubernetesPostgresMonitoring`

The metrics exporter each instance ships (native Prometheus
format on port 9187).

### spec.monitoring.tlsEnabled

`bool`

Serve the per-instance metrics endpoint over TLS (uses the
cluster's server certificate). Forces a rollout when toggled.

### spec.monitoring.disableDefaultQueries

`bool`

Drop the operator's default metric queries (connections,
replication lag, WAL, storage) and expose only custom ones added
via upstream mechanisms. Leave false — the defaults are the
dashboard-grade signal set.

### spec.scheduling

`KubernetesPostgresScheduling`

Where instance pods run: spreading across nodes/zones, node
selection, tolerations, and scheduling priority.

### spec.scheduling.antiAffinityType

`string` · optional (explicit presence)

How strongly instances avoid sharing a node: "preferred" (the
upstream default — best effort, still schedules on a small
cluster) or "required" (hard rule — instances stay Pending unless
separate nodes exist; the production posture). On an autoscaled
cluster (GKE, EKS) "required" makes the autoscaler add a node per
instance — expect ~4 minutes per instance for the node to join and
pull the ~270 MB image before the cluster is Ready (live-measured);
on a fixed-size cluster with fewer nodes than instances it never
schedules.

- default: `preferred`
- rule: anti_affinity_type must be 'preferred' (best effort) or 'required' (hard rule)

### spec.scheduling.topologyKey

`string`

Topology key the anti-affinity spreads across. Upstream default:
kubernetes.io/hostname (nodes); use topology.kubernetes.io/zone to
spread across zones.

### spec.scheduling.nodeSelector

`map<string, string>`

Node selector for the instance pods.

### spec.scheduling.tolerations

`[]WorkloadToleration`

Tolerations for the instance pods.

### spec.scheduling.tolerations[].key

`string`

Taint key to tolerate. Empty key with operator "Exists" tolerates every taint.

### spec.scheduling.tolerations[].operator

`string`

How key/value match: "Equal" (default — value must match too) or "Exists"
(key presence alone matches).

- rule: Toleration operator must be either "Equal" or "Exists"

### spec.scheduling.tolerations[].value

`string`

Taint value to match when operator is "Equal".

### spec.scheduling.tolerations[].effect

`string`

Which taint effect is tolerated: "NoSchedule", "PreferNoSchedule", or
"NoExecute". Empty tolerates all effects for the key.

- rule: Toleration effect must be one of "NoSchedule", "PreferNoSchedule", or "NoExecute"

### spec.scheduling.tolerations[].tolerationSeconds

`int64` · optional (explicit presence)

For "NoExecute" taints only: how many seconds already-running pods stay bound
after the taint appears. Unset means tolerate forever.

### spec.scheduling.priorityClassName

`string`

PriorityClass for the instance pods — databases should outlive
stateless workloads under node pressure.

### spec.updateStrategy

`KubernetesPostgresUpdateStrategy`

How rolling updates treat the PRIMARY instance (replicas always
update first).

### spec.updateStrategy.primaryUpdateStrategy

`string` · optional (explicit presence)

When the primary's turn comes: "unsupervised" (the operator
proceeds automatically — the upstream default) or "supervised"
(the operator waits for a manual promotion — change-window
control).

- default: `unsupervised`
- rule: primary_update_strategy must be 'unsupervised' (automatic) or 'supervised' (waits for manual promotion)

### spec.updateStrategy.primaryUpdateMethod

`string` · optional (explicit presence)

How the primary is updated: "restart" in place (the upstream
default — brief write outage, no role change) or "switchover" (a
replica is promoted first — shorter write outage, the primary
moves).

- default: `restart`
- rule: primary_update_method must be 'restart' (in place) or 'switchover' (promote a replica first)

### spec.enablePdb

`bool` · optional (explicit presence)

Create a PodDisruptionBudget protecting the primary from voluntary
eviction. Chart of the upstream default: true. Disable only for
development clusters where node drains should never block.

- default: `true`

### spec.imagePullSecrets

`[]string`

Names of image-pull secrets (in the cluster's namespace) for pulling
the PostgreSQL image from a private registry.

## Outputs

Reference an output from another manifest as `valueFrom: {kind: KubernetesPostgres, name: <resource-name>, fieldPath: status.outputs.<output>}`.

| Output | Type | Description |
|---|---|---|
| `status.outputs.namespace` | `string` | Namespace the cluster runs in. |
| `status.outputs.cluster_name` | `string` | Name of the Cluster resource (equals metadata.name) — every derived object (pods, services, secrets) is prefixed with it. |
| `status.outputs.rw_service` | `string` | Name of the read-write Service (`<name>-rw`) — always points at the current primary. The Service applications write through. |
| `status.outputs.ro_service` | `string` | Name of the read-only Service (`<name>-ro`) — replicas only; empty routing until a replica exists. The read-scaling handle. |
| `status.outputs.r_service` | `string` | Name of the any-instance read Service (`<name>-r`) — every ready instance including the primary. |
| `status.outputs.kube_endpoint` | `string` | In-cluster endpoint of the read-write Service, `<name>-rw.<namespace>.svc.cluster.local:5432` — the connection host for applications in the same cluster. |
| `status.outputs.port_forward_command` | `string` | Port-forward command for reaching the primary from a workstation when no exposure is composed (`kubectl port-forward svc/<name>-rw -n <namespace> 5432:5432`). |
| `status.outputs.username_secret` | `KubernetesSecretKey` | Secret key holding the application user's name (the `<name>-app` Secret's `username` key). |
| `status.outputs.username_secret.name` | `string` | The name of the Kubernetes Secret. |
| `status.outputs.username_secret.key` | `string` | The key within the Kubernetes Secret. |
| `status.outputs.password_secret` | `KubernetesSecretKey` | Secret key holding the application user's password (the `<name>-app` Secret's `password` key). The same Secret also carries ready-made `uri` / `jdbc-uri` connection strings. |
| `status.outputs.password_secret.name` | `string` | The name of the Kubernetes Secret. |
| `status.outputs.password_secret.key` | `string` | The key within the Kubernetes Secret. |
| `status.outputs.superuser_secret_name` | `string` | Name of the superuser credential Secret (`<name>-superuser`) — populated only when superuser access is enabled, empty otherwise. |

## References

Fields that can point at another resource's outputs:

| Field | Kind | Output |
|---|---|---|
| `spec.namespace` | KubernetesNamespace | `spec.name` |
| `spec.storage.storageClass` | KubernetesStorageClass | `status.outputs.storage_class_name` |
| `spec.walStorage.storageClass` | KubernetesStorageClass | `status.outputs.storage_class_name` |
| `spec.bootstrap.recovery.objectStore.r2.accountId` | CloudflareR2Bucket | `status.outputs.account_id` |
| `spec.bootstrap.recovery.objectStore.r2.jurisdiction` | CloudflareR2Bucket | `status.outputs.jurisdiction` |
| `spec.bootstrap.recovery.objectStore.r2.credentials.accessKeyId` | CloudflareAccountApiToken | `status.outputs.r2_access_key_id` |
| `spec.bootstrap.recovery.objectStore.r2.credentials.secretAccessKey` | CloudflareAccountApiToken | `status.outputs.r2_secret_access_key` |
| `spec.backup.objectStore.r2.accountId` | CloudflareR2Bucket | `status.outputs.account_id` |
| `spec.backup.objectStore.r2.jurisdiction` | CloudflareR2Bucket | `status.outputs.jurisdiction` |
| `spec.backup.objectStore.r2.credentials.accessKeyId` | CloudflareAccountApiToken | `status.outputs.r2_access_key_id` |
| `spec.backup.objectStore.r2.credentials.secretAccessKey` | CloudflareAccountApiToken | `status.outputs.r2_secret_access_key` |
| `spec.workloadIdentity.gke.serviceAccountEmail` | GcpServiceAccount | `status.outputs.email` |
| `spec.workloadIdentity.eks.roleArn` | AwsIamRole | `status.outputs.role_arn` |
| `spec.workloadIdentity.aks.clientId` | AzureUserAssignedIdentity | `status.outputs.client_id` |
| `spec.certificates.serverTlsSecret` | KubernetesCertificate | `status.outputs.secret_name` |

## Referenced By

Fields on other kinds that can point at this resource:

| Kind | Field | Reads |
|---|---|---|
| KubernetesAirflow | `spec.database.postgres.host` | `status.outputs.rw_service` |
| KubernetesAirflow | `spec.database.postgres.passwordSecret.secretName` | `status.outputs.password_secret.name` |
| KubernetesArgoWorkflows | `spec.archive.host` | `status.outputs.rw_service` |
| KubernetesArgoWorkflows | `spec.archive.credentialsSecret.name` | `status.outputs.password_secret.name` |
| KubernetesGrafana | `spec.database.host` | `status.outputs.kube_endpoint` |
| KubernetesHarbor | `spec.database.external.host` | `status.outputs.rw_service` |
| KubernetesHarbor | `spec.database.external.passwordSecretName` | `status.outputs.password_secret.name` |
| KubernetesJupyterHub | `spec.hub.database.postgres.host` | `status.outputs.rw_service` |
| KubernetesJupyterHub | `spec.hub.database.postgres.passwordSecret.secretName` | `status.outputs.password_secret.name` |
| KubernetesKeycloak | `spec.db.host` | `status.outputs.rw_service` |
| KubernetesKeycloak | `spec.db.usernameSecret.name` | `status.outputs.password_secret.name` |
| KubernetesKeycloak | `spec.db.passwordSecret.name` | `status.outputs.password_secret.name` |
| KubernetesMlflow | `spec.backendStore.postgres.host` | `status.outputs.rw_service` |
| KubernetesMlflow | `spec.backendStore.postgres.passwordSecret.secretName` | `status.outputs.password_secret.name` |
| KubernetesOpenFga | `spec.datastore.postgres.host` | `status.outputs.rw_service` |
| KubernetesOpenFga | `spec.datastore.postgres.passwordSecret.secretName` | `status.outputs.password_secret.name` |
| KubernetesOpenFga | `spec.datastore.mysql.passwordSecret.secretName` | `status.outputs.password_secret.name` |
| KubernetesSuperset | `spec.metadataDatabase.host` | `status.outputs.rw_service` |
| KubernetesSuperset | `spec.metadataDatabase.passwordSecret.secretName` | `status.outputs.password_secret.name` |
| KubernetesTemporal | `spec.database.postgres.host` | `status.outputs.rw_service` |
| KubernetesTemporal | `spec.database.postgres.passwordSecret.secretName` | `status.outputs.password_secret.name` |
| KubernetesTemporal | `spec.database.cassandra.passwordSecret.secretName` | `status.outputs.password_secret.name` |
| KubernetesTemporal | `spec.database.visibility.postgres.host` | `status.outputs.rw_service` |
| KubernetesTemporal | `spec.database.visibility.postgres.passwordSecret.secretName` | `status.outputs.password_secret.name` |
| KubernetesTrino | `spec.catalogs.postgres[].host` | `status.outputs.rw_service` |
| KubernetesTrino | `spec.catalogs.postgres[].passwordSecret.secretName` | `status.outputs.password_secret.name` |

## See Also

- [Overview](../README.md)
