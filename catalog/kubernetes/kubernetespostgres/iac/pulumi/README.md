# KubernetesPostgres Pulumi Module

Deploys one CloudNativePG-managed PostgreSQL cluster: the optional
namespace, the declared-credential Secrets, the Barman Cloud
`ObjectStore` resource(s), the `postgresql.cnpg.io/v1` Cluster resource,
and one `ScheduledBackup` per declared schedule. The custom resources
render through typed crd2pulumi SDK bindings pinned to the CloudNativePG
CRDs — field or structure drift against the pinned CRD fails at COMPILE
time, not at apply time.

Prerequisites at deploy time: the CloudNativePG operator
(`KubernetesCloudNativePgOperator`) on the cluster, and the Barman Cloud
plugin (`KubernetesCnpgBarmanCloudPlugin`, in the operator's namespace)
when backups or an object-store recovery are declared.

## What the Module Creates

1. **Namespace** (optional) — created with the standard governance
   labels when `create_namespace` is true
2. **Credential Secrets** — every DECLARED credential materializes as a
   deterministic Secret so nothing sensitive appears inline in a
   rendered custom resource (the operator and plugin only ever see
   secretKeyRef pointers):
   - `<name>-app-provided` / `<name>-superuser-provided` /
     `<name>-role-<role>` — `kubernetes.io/basic-auth` pairs
     CloudNativePG WATCHES: rotating the value rotates the database
     password
   - `<name>-ext-<external-cluster>` — external-server passwords (the
     operator builds a passfile from them)
   - `<name>-backup-creds` / `<name>-recovery-creds` (+ `-endpoint-ca`
     for self-signed S3-compatible endpoints, + `-region` because the
     ObjectStore CRD models the S3 region as a secret reference) —
     object-store credentials; keyless arms render the backend's
     ambient-identity flag and need no Secret at all
3. **ObjectStore(s)** (`barmancloud.cnpg.io/v1`) — the backup store
   (named after the cluster, carrying the retention policy) when
   `spec.backup` is set; the recovery-source store
   (`<name>-recovery-source`, never with retention — the plugin must not
   prune the source archive) when the bootstrap restores from a backup
4. **Cluster** — the PostgreSQL cluster itself; unset optionals are
   omitted entirely so the apiserver applies the CRD's own defaults, and
   chart-default-matching values (anti-affinity `preferred`,
   `enable_pdb` true, role ensure `present`, connection limit −1) render
   only on divergence
5. **ScheduledBackup(s)** (`<cluster>-<schedule>`) — each explicitly
   `method: plugin` against the cluster's backup ObjectStore, never the
   deprecated in-tree method

Ordering: Secrets and ObjectStores land before the Cluster (the operator
reads the Secrets and the plugin resolves the ObjectStore at reconcile
time); ScheduledBackups follow the Cluster.

## Rendering Notes

- **The naming contract flows from `metadata.name`** — the module
  computes the derived names (`-rw`/`-ro`/`-r` services, `-app` /
  `-superuser` secrets) in `locals.go` and exports them; deterministic
  names (never engine-generated suffixes) keep both engines
  byte-identical and let import recipes derive them blind.
- **A declared initdb owner password changes the effective app secret**
  — the module materializes `<name>-app-provided` and the operator
  adopts it instead of generating `<name>-app`; the
  `username_secret`/`password_secret` outputs point at the EFFECTIVE
  secret either way.
- **Recovery reads through a synthetic external cluster** — the module
  renders an `externalClusters` entry named `origin` whose plugin block
  points at the recovery-source ObjectStore with the SOURCE cluster's
  `serverName` as a plugin parameter (the ObjectStore CRD forbids
  `serverName` inline).
- **Workload identity rides the ServiceAccount template** — the
  `workload_identity` arm renders the exact per-cloud annotation
  (`eks.amazonaws.com/role-arn`, `iam.gke.io/gcp-service-account`,
  `azure.workload.identity/client-id` [+ tenant]) on the cluster's own
  ServiceAccount — the identity keyless backup arms authenticate with.

## Usage

```shell
planton pulumi up --manifest e2e/manifest.yaml --module-dir <path-to-this-module>
```

## Outputs

| Output | Description |
|---|---|
| `namespace` | Namespace the cluster runs in |
| `cluster_name` | Name of the Cluster resource (equals `metadata.name`) |
| `rw_service` | Read-write Service (`<name>-rw`) — the current primary |
| `ro_service` | Read-only Service (`<name>-ro`) — replicas only |
| `r_service` | Any-instance read Service (`<name>-r`) |
| `kube_endpoint` | In-cluster endpoint of the read-write Service (`<name>-rw.<namespace>.svc.cluster.local:5432`) |
| `port_forward_command` | Port-forward command for workstation access |
| `username_secret` | `{name, key}` of the application user's name (the effective app Secret) |
| `password_secret` | `{name, key}` of the application user's password |
| `superuser_secret_name` | Superuser credential Secret — empty unless superuser access is enabled |

## Module Structure

- `main.go`: entrypoint that calls the module
- `module/main.go`: namespace → credential Secrets → ObjectStores →
  Cluster → ScheduledBackups → output exports
- `module/locals.go`: the naming contract (service/secret names, the
  effective app secret) — kept in lockstep with the Terraform module's
  `locals.tf`
- `module/cluster.go`: the Cluster resource (storage, PostgreSQL config,
  synchronous replication, roles, plugin wiring, workload-identity
  annotations, certificates, scheduling, update strategy)
- `module/bootstrap.go`: the bootstrap oneof (initdb with import,
  recovery with PITR targets, pg_basebackup) and the externalClusters
  list including the synthetic recovery entry
- `module/backup.go`: ObjectStore rendering per backend arm (S3 /
  GCS / Azure Blob, keyless vs declared keys), ScheduledBackups
- `module/secrets.go`: deterministic credential-Secret materialization
- `module/vars.go`: the CNPG-I plugin identifier
  (`barman-cloud.cloudnative-pg.io`) and the synthetic recovery-source
  name (`origin`)
