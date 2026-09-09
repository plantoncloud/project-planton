# KubernetesMongodb Terraform Module

Deploys one Percona-operator-managed MongoDB cluster: the optional
namespace, the declared-credential Secrets, and the
`psmdb.percona.com/v1` PerconaServerMongoDB resource. The custom
resource applies through `kubectl_manifest` (alekc/kubectl provider,
server-side apply), which needs no cluster connection at plan time —
the database can be planned before the Percona operator's CRDs exist.

Prerequisites at apply time: the Percona Operator for MongoDB
(`KubernetesPerconaMongoOperator`, pinned v1.22.0) watching the target
namespace.

## Module Behavior

- **The naming contract flows from `metadata.name`** — pods
  (`<name>-<rs>-N`), per-set headless Services (`<name>-<rs>`), the
  mongos Service (`<name>-mongos`), the system-users Secret
  (`<name>-secrets`). The module's own satellites are equally
  deterministic (`<name>-user-<username>`, `<name>-backup-<storage>`)
  so both engines agree byte-for-byte and the import map can derive
  every address blind.
- **`spec.secrets.users` is pinned to `<name>-secrets`** — the
  operator's own fallback for an unset name is the STATIC
  `percona-server-mongodb-users` (shared by every cluster in the
  namespace), so per-cluster naming requires the module to render it.
- **Secrets never appear inline** — declared user passwords
  (`<name>-user-<username>`, key `password`) and backup-storage
  credentials (`<name>-backup-<storage>`, keys per backend arm:
  `AWS_ACCESS_KEY_ID`/`AWS_SECRET_ACCESS_KEY`,
  `GCS_CLIENT_EMAIL`/`GCS_PRIVATE_KEY`,
  `AZURE_STORAGE_ACCOUNT_NAME`/`AZURE_STORAGE_ACCOUNT_KEY`)
  materialize as `kubernetes_secret_v1` resources; the CR carries only
  references. The keyless S3 arm creates no Secret — the PBM agents use
  the pods' ambient AWS identity. GCS has no keyless arm: the module
  extracts `client_email`/`private_key` from the declared
  service-account key (raw JSON or the base64 form a GcpServiceAccount
  exports; the operator reads the two fields, not the file — a
  malformed key fails the plan loudly), or renders the user's
  `existing_secret_name` and creates nothing.
- **A restore is a run with a stable name** — `spec.restore` renders a
  `PerconaServerMongoDBRestore` named `<name>-restore-<8 hex>`, the
  suffix hashing the declaration; an unchanged declaration is a no-op
  on every apply and a changed one is a new object and a new run. The
  module never waits on it (no `wait_for`) — the operator gates the run
  on the members being up.
- **Unset optionals are omitted** — the rendered CR body is null-pruned
  so the operator applies its own defaults; `sharding` is omitted
  entirely unless enabled, `logcollector` is omitted when the spec
  block is absent (operator v1.22.0 runs the sidecar only when the
  block is present AND enabled), `unsafeFlags` renders only flags that
  are true.
- **Disabling TLS is a two-key turn** — `tls.mode: disabled` requires
  `unsafe.tls`; the module deliberately does NOT auto-set the unsafe
  flag, so the operator rejects the CR until the user opts in
  explicitly.
- **Anti-affinity "none" passes through verbatim** — it is the
  operator's own OFF switch (`AffinityOff` in upstream defaults).
- **Version changes ride `image_name`** — `upgradeOptions` is pinned to
  `apply: disabled`; automated version application is deliberately not
  modeled.
- **The module (not the operator) owns namespace creation** —
  `create_namespace` drives a `kubernetes_namespace_v1` resource
  carrying the standard governance labels.

## Resources

| Resource | Condition |
|---|---|
| `kubernetes_namespace_v1.namespace` | `spec.create_namespace` |
| `kubernetes_secret_v1.user_password_secret[<name>-user-<user>]` | user declares a password |
| `kubernetes_secret_v1.backup_credentials_secret[<name>-backup-<storage>]` | declared-key backend arm |
| `kubectl_manifest.mongodb` | always |

## Usage

```bash
planton tofu apply --manifest kubernetes-mongodb.yaml
```

## Local Development

```bash
terraform init
terraform validate
terraform plan -var-file=terraform.tfvars.json
terraform apply -var-file=terraform.tfvars.json
```

The full-surface hack manifest lives in `../../e2e/manifest.yaml`.

## Inputs

See `variables.tf` for the full variable specification (generated from
the spec proto). The spec arrives from the proto→tfvars converter in
snake_case with every `StringValueOrRef` foreign key — `namespace`
(KubernetesNamespace), `replica_sets[].storage.storage_class`
(KubernetesStorageClass), `tls.issuer` (KubernetesClusterIssuer) —
resolved to a literal string before Terraform runs.

## State Import

Existing deployments can be adopted into state. `kubectl_manifest` uses
the composed import ID `apiVersion//kind//name//namespace`; every module
resource name is deterministic (derived from `metadata.name`) and every
`for_each` resource is keyed by the FULL Secret object name, so import
recipes can derive each address blind.

## Outputs

| Output | Description |
|--------|-------------|
| `namespace` | Namespace the cluster runs in |
| `cluster_name` | Name of the PerconaServerMongoDB resource (equals `metadata.name`) |
| `service` | `<name>-mongos` when sharding, else the first set's headless Service `<name>-<rs>` |
| `kube_endpoint` | In-cluster endpoint (`<service>.<namespace>.svc.cluster.local:27017`) |
| `replica_set` | First replica set's name (driver `replicaSet` parameter); empty when sharded |
| `port_forward_command` | Port-forward command for workstation access |
| `admin_password_secret` | `{name, key}` of the database-admin password in the `<name>-secrets` system-users Secret |

## Parity

This module is the behavioral twin of the Pulumi module
(`../pulumi/module/`): same resource names, same rendered CR body
(null-pruned, presence-sensitive keys), same Secret names/keys/types,
same module constants (crVersion 1.22.0, the pinned backup/fluentbit
images, upgradeOptions disabled), same outputs — keep them in lockstep.
