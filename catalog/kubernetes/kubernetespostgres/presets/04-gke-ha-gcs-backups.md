# GKE Production HA with GCS Backups

This preset declares the production PostgreSQL posture on GKE with disaster
recovery built in: three instances with quorum synchronous replication and
hard anti-affinity, a dedicated WAL volume, and continuous backups — WAL
archiving plus a nightly base backup — landing KEYLESSLY in a GCS bucket
through GKE Workload Identity. This is the production half of the DR
resource set in the component guide; the restore half is a second
`KubernetesPostgres` with a `bootstrap.recovery` block pointing at this
archive and referencing this cluster's `-app` Secret.

## When to Use

- Any production database on GKE whose loss or downtime matters
- Together with the GCP-side trio: a keyless `GcpServiceAccount`, a
  `GcpGkeWorkloadIdentityBinding` for the cluster's ServiceAccount (named
  after the cluster, in its namespace), and a `GcpGcsBucket` granting the
  account `roles/storage.objectAdmin` and `roles/storage.legacyBucketReader`
- Requires the Barman Cloud plugin (`KubernetesCnpgBarmanCloudPlugin`)
  in the operator's namespace — on a cluster that already runs
  CloudNativePG (a self-hosted platform installs one), declare it with
  the resident operator's namespace as a literal

## Key Configuration Choices

- **`instances: 3` + `anti_affinity_type: required`** — a hard rule the
  GKE autoscaler satisfies by adding nodes; the instances really are on
  three nodes
- **Quorum synchronous replication** — a failover loses nothing;
  `required` blocks writes when no replica is available (durability over
  availability)
- **`gcs.keyless` + `workload_identity.gke`** — no key anywhere; the
  identity's email flows in by reference from the `GcpServiceAccount`
- **Two bucket roles** — `objectAdmin` alone fails every archive with
  "does not have storage.buckets.get access"; add `legacyBucketReader`
- **One destination path per cluster** — a recovered cluster archives to a
  new path; Barman refuses to write into another cluster's archive
- **What to back up alongside the data** — this cluster's `prod-db-app`
  Secret: the recovered roles keep these passwords, and the recovery target
  references the Secret (`bootstrap.recovery.owner_secret_name`)
