# KubernetesPostgres Terraform Module

Deploys one CloudNativePG-managed PostgreSQL cluster: the optional
namespace, the declared-credential Secrets, the Barman Cloud
`ObjectStore` resource(s), the `postgresql.cnpg.io/v1` Cluster resource,
and one `ScheduledBackup` per declared schedule. The custom resources
apply through `kubectl_manifest` (alekc/kubectl provider, server-side
apply), which needs no cluster connection at plan time — the database
can be planned before CloudNativePG's CRDs exist.

Prerequisites at apply time: the CloudNativePG operator
(`KubernetesCloudNativePgOperator`) on the cluster, and the Barman Cloud
plugin (`KubernetesCnpgBarmanCloudPlugin`, in the operator's namespace)
when backups or an object-store recovery are declared.

## Module Behavior

- **The naming contract flows from `metadata.name`** — the module
  computes the derived names (`-rw`/`-ro`/`-r` services, `-app` /
  `-superuser` secrets) in `locals.tf` and exports them; deterministic
  names (never engine-generated suffixes) keep both engines
  byte-identical and let the import map derive every address blind.
- **Backups are PLUGIN-BASED, deliberately** — CloudNativePG's in-tree
  `barmanObjectStore` support is deprecated upstream and not modeled.
  The backup block renders an `ObjectStore` named after the cluster
  (carrying the retention policy) plus the Cluster's plugin wiring (WAL
  archiving starts immediately); each schedule renders a
  `ScheduledBackup` (`<cluster>-<schedule>`) explicitly
  `method: plugin`.
- **Recovery reads through a synthetic external cluster** — a recovery
  bootstrap renders a second `ObjectStore` (`<name>-recovery-source`,
  never with retention — the plugin must not prune the source archive)
  read through an `externalClusters` entry named `origin` that carries
  the source's `serverName` as a plugin parameter (the ObjectStore CRD
  forbids it inline).
- **Secrets never appear inline** — every declared credential (owner
  password, role passwords, superuser password, external-cluster
  passwords, object-store keys, endpoint CA, the S3 region — the
  ObjectStore CRD models it as a secret reference) materializes as its
  own `kubernetes_secret_v1` resource; the rendered custom resources
  carry only secretKeyRef pointers. Keyless backup arms render the
  backend's ambient-identity flag and create no Secret at all.
- **A declared initdb owner password changes the effective app secret**
  — the module materializes `<name>-app-provided` and the operator
  adopts it instead of generating `<name>-app`; the
  `username_secret`/`password_secret` outputs point at the EFFECTIVE
  secret either way.
- **Unset optionals are omitted** — the rendered CR bodies are
  null-pruned so the apiserver applies the CRD's own defaults;
  default-matching values (anti-affinity `preferred`, `enable_pdb` true,
  role ensure `present`, connection limit −1) render only on divergence.
- **The module (not the operator) owns namespace creation** —
  `create_namespace` drives a `kubernetes_namespace_v1` resource
  carrying the standard governance labels.

## Resources

| Resource | Condition |
|---|---|
| `kubernetes_namespace_v1.namespace` | `spec.create_namespace` |
| `kubernetes_secret_v1.provided_app_secret` | initdb declares `owner_password` |
| `kubernetes_secret_v1.provided_superuser_secret` | superuser enabled with a declared password |
| `kubernetes_secret_v1.role_password_secret[<role>]` | role declares a password |
| `kubernetes_secret_v1.external_cluster_password_secret[<ext>]` | external cluster declares a password |
| `kubernetes_secret_v1.backup_credentials_secret` / `recovery_credentials_secret` | declared-key backend arm |
| `kubernetes_secret_v1.backup_region_secret` / `recovery_region_secret` | S3 arm with a region; always for the R2 arm (`auto`) |
| `kubernetes_secret_v1.backup_endpoint_ca_secret` / `recovery_endpoint_ca_secret` | S3-compatible arm with `endpoint_ca_pem` |
| `kubectl_manifest.backup_object_store` | `spec.backup` |
| `kubectl_manifest.recovery_object_store` | recovery bootstrap |
| `kubectl_manifest.cluster` | always |
| `kubectl_manifest.scheduled_backup[<schedule>]` | per declared schedule |

## Usage

```bash
planton tofu apply --manifest kubernetes-postgres.yaml
```

## Local Development

```bash
terraform init
terraform validate
terraform plan -var-file=terraform.tfvars.json
terraform apply -var-file=terraform.tfvars.json
```

Offline plan fixtures (hand-converted tfvars for the hack manifests)
live in `../hack/`.

## Inputs

See `variables.tf` for the full variable specification (generated from
the spec proto). The spec arrives from the proto→tfvars converter in
snake_case with every `StringValueOrRef` foreign key — `namespace`
(KubernetesNamespace), `storage.storage_class`
(KubernetesStorageClass), `certificates.server_tls_secret`
(KubernetesCertificate), and the workload-identity references —
resolved to a literal string before Terraform runs.

## State Import

Existing deployments can be adopted into state. `kubectl_manifest` uses
the composed import ID `apiVersion//kind//name//namespace`; every module
resource name is deterministic (derived from `metadata.name`), so the
component's `iac/import-map.yaml` can derive each address blind.

## Outputs

| Output | Description |
|--------|-------------|
| `namespace` | Namespace the cluster runs in |
| `cluster_name` | Name of the Cluster resource (equals `metadata.name`) |
| `rw_service` | Read-write Service (`<name>-rw`) — the current primary |
| `ro_service` | Read-only Service (`<name>-ro`) — replicas only |
| `r_service` | Any-instance read Service (`<name>-r`) |
| `kube_endpoint` | In-cluster endpoint of the read-write Service |
| `port_forward_command` | Port-forward command for workstation access |
| `username_secret` | `{name, key}` of the application user's name (the effective app Secret) |
| `password_secret` | `{name, key}` of the application user's password |
| `superuser_secret_name` | Superuser credential Secret — empty unless superuser access is enabled |

## Parity

This module is the behavioral twin of the Pulumi module
(`../pulumi/module/`): same resource names, same rendered CR bodies
(null-pruned, defaults-on-divergence), same Secret names/keys/types,
same plugin identifier (`barman-cloud.cloudnative-pg.io`) and synthetic
recovery-source name (`origin`), same outputs — keep them in lockstep.
