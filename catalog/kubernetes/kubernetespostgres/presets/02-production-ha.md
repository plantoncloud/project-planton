# Production HA

This preset declares the production PostgreSQL posture on EKS: three
instances with quorum synchronous replication (zero data loss on
failover), a dedicated WAL volume, hard anti-affinity, and continuous
backups — WAL archiving plus a nightly base backup — landing keylessly
in S3 via IRSA, pruned after 30 days. Requires the Barman Cloud plugin
on the cluster, in the operator's namespace
(KubernetesCnpgBarmanCloudPlugin).

## When to Use

- Any production database whose loss or downtime matters
- The 30-second choice for AWS: this is the standard production shape;
  swap the backup arm for GCS/Azure-Blob on other clouds

## Key Configuration Choices

- **`instances: 3`** — one primary + two replicas: automated failover
  with capacity to spare during maintenance
- **Quorum synchronous replication (`synchronous: any/1/required`)** —
  transactions wait for one replica before committing; a failover loses
  nothing. `required` means writes BLOCK if no replica is available —
  durability over availability (soften to `preferred` if the trade goes
  the other way)
- **`wal_storage`** — sequential WAL writes on their own volume, the
  standard I/O-isolation move; must be set at creation
- **`data_checksums: true`** — detects silent storage corruption; cannot
  be enabled after creation
- **Keyless S3 backups (`s3.keyless` + `workload_identity.eks`)** — the
  instance pods' ServiceAccount carries the
  `eks.amazonaws.com/role-arn` annotation; no credential Secret exists
  at all. The IAM role's trust policy names the cluster's own
  ServiceAccount (named after the cluster, in its namespace)
- **`schedules: daily` with `immediate: true`** — WAL archiving starts
  when the cluster is healthy, but PITR needs a base backup to start
  from; the immediate flag takes one on creation instead of waiting for
  the first cron tick. Note the SIX-field cron — seconds first
- **`retention_policy: 30d`** — the plugin prunes backups and WAL after
  each backup
- **`anti_affinity_type: required`** — instances stay Pending rather
  than sharing a node; pair with `topology_key:
  topology.kubernetes.io/zone` to spread across zones instead
- **`primary_update_method: switchover`** — rolling updates promote a
  replica before touching the primary: shorter write outage, the
  primary moves

## Placeholders to Replace

| Placeholder | Description | Where to Find |
| --- | --- | --- |
| `<pg-backups-bucket>` | S3 bucket for backups (path suffix `prod-db` keeps one path per cluster) | S3 console or `AwsS3Bucket` outputs |
| `<aws-region>` | Region of the bucket | Your AWS account |
| `arn:aws:iam::123456789012:role/prod-db-backups` | IRSA role ARN — replace account id and name | IAM console or `AwsIamRole` outputs |

## Related Presets

- **01-dev-single-instance** — the development shape: one instance, no
  backups
- **03-s3-compatible-backups** — the same backup chain against
  MinIO/R2/any S3-compatible endpoint with declared keys
