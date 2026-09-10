# GKE Replica Set with Cloudflare R2 Backups

This preset declares the production MongoDB posture on GKE with disaster
recovery built in: a three-member replica set spread across nodes, required
TLS, a declarative application user, nightly logical backups with continuous
oplog archiving (point-in-time recovery) landing in a Cloudflare R2 bucket
declared in R2's own terms — the bucket, its account and jurisdiction, and a
Cloudflare API token, all by reference to the catalog's Cloudflare kinds. The
module does the S3 translation R2 needs (the jurisdiction's endpoint, region
`auto`, path-style addressing, the token as an S3 key pair); nothing S3-shaped
is typed. This is the production half of the DR resource set in the component
guide; the restore half is a second `KubernetesMongodb` declaring the same
storage, a `restore` block, and the source's system-users Secret.

## When to Use

- Any production MongoDB on GKE (or any cluster — R2 is reachable from
  anywhere) whose backups should live outside the cloud that runs it, or in
  an object store already paid for
- Together with the Cloudflare-side pair: a `CloudflareR2Bucket` and a
  `CloudflareAccountApiToken` whose policy grants `Workers R2 Storage Bucket
  Item Write` on that bucket (resource
  `com.cloudflare.edge.r2.bucket.<account>_<jurisdiction>_<bucket>`)

## Key Configuration Choices

- **`size: 3` on the default anti-affinity** — one member per node; on GKE
  the cluster autoscaler adds nodes to honor it, so the set really is
  spread
- **`r2.bucket`, `r2.account_id`, `r2.jurisdiction` by reference to the
  bucket** — the S3 host follows the bucket; an EU bucket is served only
  through its own host, and a reference cannot drift from it
- **`r2.credentials` by reference to the token** — the token kind exports
  itself as the S3 key pair (`r2_access_key_id`, `r2_secret_access_key`);
  rotating the token rotates the pair, and the next apply follows. There is
  no keyless posture for R2 from any cluster
- **`prefix: prod-mongo`** — one prefix per cluster; the restore target
  declares this exact storage to read the backups back. A backup here
  reports its destination as `s3://<bucket>/prod-mongo/<PBM timestamp>`
- **`pitr.enabled`** — oplog chunks archive continuously, so a restore can
  land between the nightly backups
- **What to back up alongside the data** — the operator's system-users
  Secret (`prod-mongo-secrets`): a restored database carries this
  cluster's users and passwords, and the restore target must reference
  that Secret (`system_users_secret_name`)
