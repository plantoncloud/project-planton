# GKE Replica Set with GCS Backups

This preset declares the production MongoDB posture on GKE with disaster
recovery built in: a three-member replica set spread across nodes, required
TLS, a declarative application user, nightly logical backups into a GCS
bucket with continuous oplog archiving (point-in-time recovery), and the
backup credential wired by reference from a `GcpServiceAccount`. This is
the production half of the DR resource set in the component guide; the
restore half is a second `KubernetesMongodb` declaring the same storage, a
`restore` block, and the source's system-users Secret.

## When to Use

- Any production MongoDB on GKE whose loss matters
- Together with the GCP-side trio: a `GcpServiceAccount` with
  `user_managed_key: {}`, and a `GcpGcsBucket` granting it
  `roles/storage.objectAdmin` and `roles/storage.legacyBucketReader`

## Key Configuration Choices

- **`size: 3` on the default anti-affinity** — one member per node; on GKE
  the cluster autoscaler adds nodes to honor it, so the set really is
  spread
- **`credentials.service_account_key` by reference** — MongoDB backups on
  GCS are KEYED (Percona Backup for MongoDB has no Workload Identity
  path); the `GcpServiceAccount`'s `key_base64` output flows in as a
  reference, so the key lives in state and in the operator's Secret only —
  never in a manifest
- **Two bucket roles** — the storage client reads the bucket's attributes
  before writing; `objectAdmin` alone is refused
- **`prefix: prod-mongo`** — one prefix per cluster; the restore target
  declares this exact storage to read the backups back
- **`pitr.enabled`** — oplog chunks archive continuously, so a restore can
  land between the nightly backups
- **What to back up alongside the data** — the operator's system-users
  Secret (`prod-mongo-secrets`): a restored database carries this
  cluster's users and passwords, and the restore target must reference
  that Secret (`system_users_secret_name`)
