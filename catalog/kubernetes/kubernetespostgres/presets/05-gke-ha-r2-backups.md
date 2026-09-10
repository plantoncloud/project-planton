# GKE Production HA with Cloudflare R2 Backups

This preset declares the production PostgreSQL posture on GKE with disaster
recovery built in: three instances with quorum synchronous replication and
hard anti-affinity, a dedicated WAL volume, and continuous backups — WAL
archiving plus a nightly base backup — landing in a Cloudflare R2 bucket
declared in R2's own terms: the bucket, its account and jurisdiction, and a
Cloudflare API token, all by reference to the catalog's Cloudflare kinds. The
module does the S3 translation R2 needs (the jurisdiction's endpoint, region
`auto`, the token as an S3 key pair); nothing S3-shaped is typed. This is the
production half of the DR resource set in the component guide; the restore
half is a second `KubernetesPostgres` with a `bootstrap.recovery` block
pointing at this archive and referencing this cluster's `-app` Secret.

## When to Use

- Any production database on GKE (or any cluster — R2 is reachable from
  anywhere) whose backups should live outside the cloud that runs it, or in
  an object store already paid for
- Together with the Cloudflare-side pair: a `CloudflareR2Bucket` and a
  `CloudflareAccountApiToken` whose policy grants `Workers R2 Storage Bucket
  Item Write` on that bucket (resource
  `com.cloudflare.edge.r2.bucket.<account>_<jurisdiction>_<bucket>`)
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
- **`r2.account_id` and `r2.jurisdiction` by reference to the bucket** — the
  S3 host follows the bucket; an EU bucket is served only through its own
  host, and a reference cannot drift from it
- **`r2.credentials` by reference to the token** — the token kind exports
  itself as the S3 key pair (`r2_access_key_id`, `r2_secret_access_key`);
  rotating the token rotates the pair, and the next apply follows. There is
  no keyless posture for R2 from any cluster
- **The bucket is in `destination_path`** — `s3://<bucket>/<path>`, exactly
  as the gcs and azure_blob arms name theirs; one path per cluster, forever
- **What to back up alongside the data** — this cluster's `prod-db-app`
  Secret: the recovered roles keep these passwords, and the recovery target
  references the Secret (`bootstrap.recovery.owner_secret_name`)
