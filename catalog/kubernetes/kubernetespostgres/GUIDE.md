# KubernetesPostgres Guide

The judgment this guide carries: what this component assumes is already on
the cluster, and who should own its namespace — the two places
agent-composed architectures with a database go wrong before a single
PostgreSQL setting matters.

## The architecture must include the operator

This component does not run PostgreSQL by itself: it renders a CloudNativePG
`Cluster` custom resource, and
**[KubernetesCloudNativePgOperator](../kubernetescloudnativepgoperator/GUIDE.md)
must be on the cluster** to reconcile it — proposing a database without it
deploys a custom resource nothing acts on, with no error anywhere (the
[operator-prerequisite pattern](../../_patterns/operator-prerequisite.md)
is the general mechanism).

Two couplings to get right:

- **Backups couple to the plugin beside the operator.** Declaring
  `spec.backup` (or an object-store `bootstrap.recovery`) here renders the
  Barman Cloud plugin into the Cluster, so
  [KubernetesCnpgBarmanCloudPlugin](../kubernetescnpgbarmancloudplugin/GUIDE.md)
  must be on the cluster, in the operator's namespace. Without it the
  operator parks the Cluster in the phase "Cluster cannot proceed to
  reconciliation due to an unknown plugin being required" and never
  creates its instances — loud in `kubectl get cluster`, invisible to an
  apply waiting on Ready (the plugin's guide carries the install side).
- **One operator per cluster, many databases.** The operator is
  cluster-scoped; compose it once (typically in the shared-cluster chart),
  then any number of KubernetesPostgres resources in application
  environments.

## Namespace ownership

`spec.namespace` is a required foreign key targeting KubernetesNamespace.
`createNamespace: true` makes THIS database the namespace's owner in IaC
state: the namespace is created before the cluster and — per the field's
own contract — **deleted with the resource**. Safe for a namespace whose
only tenant is this database; wrong the moment a cache, an app, or a second
database shares it (the second `createNamespace` deploy fails on
already-exists, and destroying the database would delete the neighbors'
namespace). The judgment, the failure story, and the `valueFrom` wiring:
[namespace-ownership pattern](../../_patterns/namespace-ownership.md).

## Disaster recovery on GKE: the resource set

"Highly available, backed up, restorable" is six catalog resources on the
GCP side and the Kubernetes side together — every one of them a kind in this
catalog, wired by reference, and the whole set was proven live on GKE: a
three-instance cluster spread over three nodes, WAL and a base backup landing
in a GCS bucket keylessly, and a fresh cluster bootstrapped from that archive
carrying rows written both before and after the base backup.

| # | Resource | What it is for | Wiring |
|---|---|---|---|
| 1 | `GcpServiceAccount` (e.g. `pg-backup`) | The identity the instance pods assume — KEYLESS, no key created | — |
| 2 | `GcpGcsBucket` | The archive: WAL + base backups | `iam_members`: **two** roles for the identity — `roles/storage.objectAdmin` AND `roles/storage.legacyBucketReader` (Barman checks the bucket with `storage.buckets.get` before every archive; objectAdmin alone fails with "does not have storage.buckets.get access") — `member` by reference to #1's `status.outputs.member` |
| 3 | `GcpGkeWorkloadIdentityBinding` | Lets the cluster's KSA act as #1 | `ksa_namespace` = the database's namespace, `ksa_name` = the database's `metadata.name` (CloudNativePG names the ServiceAccount after the Cluster); `service_account_email` by reference to #1 |
| 4 | `KubernetesCnpgBarmanCloudPlugin` | The backup engine, beside the operator | `namespace` = the operator's (by reference to a `KubernetesCloudNativePgOperator` resource, or the literal namespace of a CloudNativePG that ALREADY runs on the cluster — a self-hosted platform installs one); cert-manager resident or declared |
| 5 | `KubernetesPostgres` (the production database) | HA + backups | `instances: 3`, `scheduling.anti_affinity_type: required`, `workload_identity.gke.service_account_email` by reference to #1, `backup.object_store` at `gs://<bucket>/<path>` with `gcs.keyless: true`, a schedule with `immediate: true`, `retention_policy` |
| 6 | `KubernetesPostgres` (the recovery target, on the bad day) | Restore | `bootstrap.recovery.object_store` = #5's store, `source_server_name` = #5's name, `database`/`owner` = #5's initdb values, `owner_secret_name` = #5's `<name>-app` Secret; its own `workload_identity` (and its own #3 binding — the KSA is named after IT); its own `backup` at a DIFFERENT path |

Two rules the set stands on:

- **Credential continuity.** The recovered data carries the source's roles
  and passwords. The recovery target must reference the source's `<name>-app`
  Secret (`owner_secret_name`), so that Secret must outlive the source —
  back it up with the archive (a `KubernetesSecret` / `ExternalSecret`
  declaration, or the secret backend). Without it the target hands out a
  freshly generated password the restored role does not have.
- **One archive path per cluster, forever.** A recovered cluster archives its
  own WAL to a new path; Barman refuses to archive into a path that already
  holds another cluster's WAL, and a second writer would corrupt the archive.

The validated manifests for this set are the `gcp-gke` lane's own:
`e2e/fixture-gke-source.yaml` (#5), `e2e/scenarios/gke-gcs-recovery.yaml`
(#6), the plugin kind's install profile (#4, declared on the scenario as a
prerequisite beside the resident operator), and the GCP side under
`../aa_e2e/realcluster/gcp-gke/manifests/` (#1–#3). The
`04-gke-ha-gcs-backups` preset is #5 as a starting point.

## On the diagram

Wiring `spec.namespace` via `valueFrom` draws the database's namespace edge
on the architecture diagram; the operator component appears as its own node
in the shared-cluster layer. An architecture whose diagram shows database,
namespace, and operator is one a user can actually reason about — a
`createNamespace` flag and an assumed operator show nothing.

## Pairs well with

- KubernetesCloudNativePgOperator — required, once per cluster.
- KubernetesNamespace — the namespace owner (pattern above).
- Application workloads connect through the `<name>-rw` / `<name>-ro`
  Services and `<name>-app` credential Secret the reference page's naming
  contract documents.
