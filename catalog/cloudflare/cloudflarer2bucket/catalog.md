# Cloudflare R2 Bucket

Deploys an R2 object storage bucket on Cloudflare with configurable location hints, public access controls, and optional custom domain serving. R2 provides S3-compatible storage with zero egress fees. The bucket's full configuration surface travels with it: custom domains, CORS, object lifecycle, object lock retention, and event notifications that push object events to Cloudflare Queues.

## What Gets Created

When you deploy this Cloud Resource, the IaC module provisions:

- **R2 Bucket** -- an object storage bucket in the specified Cloudflare account with the configured location hint, jurisdiction, and default storage class
- **Managed Public Domain (r2.dev)** -- created only when `publicAccess` is `true`; its URL is published as the `public_url` output
- **R2 Custom Domains** -- one per enabled entry in `customDomains`; each serves the bucket at a hostname in the referenced Cloudflare zone
- **CORS Configuration** -- created only when `cors` rules are declared
- **Lifecycle Rules** -- created only when `lifecycle` rules are declared (storage-class transitions, expiration, multipart-upload cleanup)
- **Object Lock Rules** -- created only when `lock` rules are declared (compliance retention)
- **Event Notifications** -- one per `eventNotifications` entry, pushing matching object events to a Cloudflare Queue

## Before You Deploy

### Planton Setup

- **Cloudflare Provider Connection** -- an active connection in the Connect module with a Cloudflare API token that has R2 permissions. Map it as the default for your environment, or specify it explicitly when creating the Cloud Resource.
- **Planton Runner** -- required when using Runner-based credential delivery. Not needed for inline API token authentication.

### Cloudflare Account

- **A Cloudflare account** with R2 enabled. The `accountId` field identifies which account owns the bucket.
- **A Cloudflare DNS zone** (optional) -- required only when using a custom domain. Provide the zone ID directly or reference a CloudflareDnsZone Cloud Resource via ValueFromRef.

## Deploy

### Console

Open the deployment store, find **Cloudflare R2 Bucket**, and click **Deploy**. The creation wizard walks you through preset selection, environment and connection configuration, and spec fields. Start from the **Private R2 Bucket** preset in the [Presets](#presets) tab to pre-populate a secure default configuration.

### CLI

Create a manifest and apply it:

```yaml
apiVersion: cloudflare.planton.dev/v1alpha1
kind: CloudflareR2Bucket
metadata:
  name: app-assets
  org: acme-corp
  env: prod
spec:
  bucketName: app-assets-prod
  accountId: "a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4"
  location: auto
  publicAccess: false
```

```shell
planton apply -f r2-bucket.yaml
```

This creates a private R2 bucket with automatic location selection. No public URL or custom domain is configured. Access is limited to Workers, API tokens, or the Cloudflare dashboard. A Stack Job tracks the provisioning in real time.

### InfraChart

When using a custom domain, use ValueFromRef to wire the bucket to a DNS zone deployed in the same InfraPipeline:

```yaml
spec:
  customDomains:
    - enabled: true
      zoneId:
        valueFrom:
          kind: CloudflareDnsZone
          name: example-zone
          fieldPath: status.outputs.zone_id
      domain: media.example.com
```

The InfraPipeline resolves the dependency graph, deploys the DNS zone first, then provisions the R2 bucket with the custom domain configured on the resolved zone.

## Key Configuration

These are the most important decisions when configuring an R2 bucket. Explore the full field reference in the [API Explorer](#api-explorer) tab.

**Location hint** -- The `location` field influences where Cloudflare initially places bucket data. Use `auto` (default) to let Cloudflare choose, or hint a region (`wnam`, `enam`, `weur`, `eeur`, `apac`, `oc`) to keep data near your users. The hint is honored only at creation and is best-effort, not a residency guarantee -- for a guarantee, use `jurisdiction`.

**Jurisdiction** -- `default`, `eu`, `fedramp`, or `us`. This is a one-way door: the jurisdiction is fixed at creation and is part of the bucket's identity, and every bucket-scoped sub-resource is created inside it. Changing it means a new bucket and a data migration.

**Public access** -- Leave `publicAccess: false` (default) to restrict access to Workers, API tokens, and the Cloudflare dashboard. Setting it `true` enables the managed `r2.dev` URL, which is rate-limited and intended for development -- custom domains are the production-grade public path.

**Custom domains** -- Add entries to `customDomains` to serve bucket content from branded hostnames (e.g., `media.example.com`). Each requires a zone on the same account and inherits Cloudflare's CDN and caching; `minTls` defaults to 1.0, so raise it if your compliance baseline demands modern TLS.

**Storage class and lifecycle** -- `storageClass: InfrequentAccess` lowers storage cost but adds retrieval fees, so it only pays off for data you rarely read. Lifecycle rules automate the same transition by age or date and can expire objects outright -- an expiration rule with the wrong prefix deletes data you meant to keep.

**Object lock** -- Lock rules prevent matching objects from being deleted or overwritten for a period or indefinitely. An `Indefinite` rule retains objects forever -- there is no undo, so scope lock rules to precise prefixes.

## Outputs and Dependencies

### What This Component Consumes

| Dependency | Field | ValueFromRef Path |
|------------|-------|-------------------|
| **CloudflareDnsZone** (optional) | `customDomains[].zoneId` | `status.outputs.zone_id` |
| **CloudflareQueue** (optional) | `eventNotifications[].queue` | `status.outputs.queue_id` |

### What This Component Provides

After provisioning, `status.outputs` contains values that downstream Cloud Resources can consume via ValueFromRef:

| Output | Description | Common Downstream Use |
|--------|-------------|----------------------|
| `bucket_name` | The name of the R2 bucket | Referenced by a Worker's or Pages project's `r2Buckets` binding |
| `bucket_url` | The path-style S3 API URL of the bucket (its jurisdiction's endpoint plus the bucket name) | SDK endpoint configuration for S3-compatible clients |
| `custom_domain_urls` | One URL per enabled custom domain (e.g., `https://media.example.com`) | Frontend asset references, public download URLs |
| `public_url` | The managed `r2.dev` URL when `publicAccess` is enabled; empty otherwise | Development and testing access before a custom domain exists |
| `account_id` | The owning Cloudflare account (same as `accountId`) | Composing the bucket into an S3-compatible client beside its name without repeating the account |
| `jurisdiction` | The bucket's data-residency jurisdiction, normalized (`default`, `eu`, `fedramp`, `us`) | Selecting the S3 host; scoping an account API token to this bucket |
| `s3_endpoint` | The S3 API endpoint that serves this bucket's jurisdiction -- the only host that does (`https://<account>.r2.cloudflarestorage.com`, or `https://<account>.<jurisdiction>.r2.cloudflarestorage.com`) | Backup tools and databases that archive to R2 over the S3 API (region `auto`) |

## Common Patterns

Browse the [Presets](#presets) tab for ready-to-deploy configurations.

**Private bucket** -- No public access, automatic location. Access via Workers or API tokens only. Suitable for backups, logs, CI/CD artifacts, and internal data. Start from the **Private R2 Bucket** preset.

**Public CDN bucket** -- A custom domain for branded content delivery. Combines R2's zero-egress pricing with Cloudflare's CDN for static assets, images, and media files. Start from the **Public R2 Bucket with Custom Domain** preset.

**Lifecycle-managed archive** -- A private bucket that transitions aging objects to Infrequent Access, expires them on schedule, and locks compliance-critical prefixes against deletion. Start from the **Private R2 Bucket with Lifecycle and Retention** preset.

## Works With

- [**Cloudflare DNS Zone**](/cloud-catalog/cloudflare-dns-zone) -- provides the zone ID for custom domain configuration
- [**Cloudflare Queue**](/cloud-catalog/cloudflare-queue) -- receives this bucket's event notifications for downstream processing
- [**Cloudflare Worker**](/cloud-catalog/cloudflare-worker) -- binds the bucket through an `r2Buckets` binding for object reads and writes
- [**Cloudflare Pages Project**](/cloud-catalog/cloudflare-pages-project) -- binds the bucket to Pages Functions the same way