# S3-Compatible Backups

This preset declares a highly available PostgreSQL cluster whose backups
land in an S3-COMPATIBLE object store — in-cluster MinIO, Cloudflare R2,
Ceph RGW, DigitalOcean Spaces, anything speaking the S3 API — via the
`endpoint_url` override with declared access keys. The self-contained
posture for on-prem clusters and stores outside AWS. Requires the Barman
Cloud plugin on the cluster, in the operator's namespace
(KubernetesCnpgBarmanCloudPlugin).

## When to Use

- On-prem and bare-metal clusters backing up to MinIO or Ceph RGW
- Clusters whose backup store is R2, Spaces, or another S3-compatible
  service — no AWS account involved

## Key Configuration Choices

- **`endpoint_url`** — what makes the arm S3-compatible: the store's
  endpoint instead of real AWS S3 (`http://minio.minio-system.svc:9000`
  for in-cluster MinIO, `https://<account>.r2.cloudflarestorage.com` for
  R2). The `destination_path` keeps the `s3://` scheme either way
- **`access_keys`, not `keyless`** — an S3-compatible endpoint
  authenticates with the store's key pair (for MinIO: access key =
  username, secret key = password); the keyless posture only mints AWS
  credentials and is spec-rejected with an endpoint URL. The keys
  materialize as the `app-db-backup-creds` Secret — never inline in the
  rendered resources
- **`region: minio`** — S3-compatible stores take a conventional value
  (MinIO accepts any; R2 expects `auto`)
- **`endpoint_ca_pem`** (commented) — for stores serving self-signed
  TLS, the PEM CA bundle materializes as a Secret the plugin verifies
  against
- **The rest is the production shape** — 3 instances, data checksums, a
  nightly immediate-on-creation schedule, 30-day retention

## Placeholders to Replace

| Placeholder | Description | Where to Find |
| --- | --- | --- |
| `<store-access-key>` | Access key ID (MinIO: username) | Your object store's admin console |
| `<store-secret-key>` | Secret access key (MinIO: password) — stored as a managed secret | Your object store's admin console |
| `http://minio.minio-system.svc:9000` | Endpoint URL of the store | In-cluster Service DNS or the provider's endpoint documentation |
| `s3://pg-backups/app-db` | Bucket + per-cluster path — one path per cluster | Your store's bucket layout |

## Related Presets

- **01-dev-single-instance** — the development shape: one instance, no
  backups
- **02-production-ha** — the same backup chain against real S3, keyless
  via IRSA
