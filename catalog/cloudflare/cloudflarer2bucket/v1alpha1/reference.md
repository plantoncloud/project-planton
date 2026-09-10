# CloudflareR2Bucket

> Generated from the protobuf schema by `make generate-reference` -- do not
> edit by hand. To change a fact on this page, change the proto field comment
> or validation rule it is derived from, then regenerate.

**apiVersion**: `cloudflare.planton.dev/v1alpha1`

**Guide**: [GUIDE.md](../GUIDE.md) -- authored operational judgment for this component: conventions, trade-offs, and what pairs well with it.

CloudflareR2BucketSpec defines the user configuration for a Cloudflare R2 bucket
and its bucket-scoped configuration (custom domains, public access, CORS,
object lifecycle, and object lock). These sub-resources have no independent
lifecycle and exist only as configuration of this bucket, so they are modeled
as nested fields rather than separate resources.

## Example

```yaml
apiVersion: cloudflare.planton.dev/v1alpha1
kind: CloudflareR2Bucket
metadata:
  name: r2-hack-bucket
spec:
  bucketName: planton-r2-hack-bucket
  accountId: 0a1b2c3d4e5f6a7b8c9d0e1f2a3b4c5d
  location: weur
  jurisdiction: default
  storageClass: Standard
  publicAccess: true
  customDomains:
    - enabled: true
      zoneId:
        value: 0a1b2c3d4e5f6a7b8c9d0e1f2a3b4c5d
      domain: media.example.com
      minTls: "1.2"
  cors:
    rules:
      - allowed:
          methods: [GET, HEAD]
          origins:
            - https://app.example.com
        maxAgeSeconds: 3600
  lifecycle:
    rules:
      - id: expire-temp
        enabled: true
        conditions:
          prefix: tmp/
        abortMultipartUploadsTransition:
          maxAgeSeconds: 604800
        deleteObjectsTransition:
          condition:
            type: Age
            maxAgeSeconds: 2592000
        storageClassTransitions:
          - condition:
              type: Age
              maxAgeSeconds: 86400
  lock:
    rules:
      - id: retain-logs
        enabled: true
        prefix: logs/
        condition:
          type: Age
          maxAgeSeconds: 2592000
```

## Spec Fields

| Path | Type | Required | Default | References |
|---|---|---|---|---|
| `spec.bucketName` | `string` | yes |  |  |
| `spec.accountId` | `string` | yes |  |  |
| `spec.location` | `enum` |  | `auto` |  |
| `spec.publicAccess` | `bool` |  |  |  |
| `spec.customDomains` | `[]CloudflareR2BucketCustomDomainConfig` |  |  |  |
| `spec.customDomains[].enabled` | `bool` |  |  |  |
| `spec.customDomains[].zoneId` | `string \| valueFrom` |  |  | CloudflareDnsZone (`status.outputs.zone_id`) |
| `spec.customDomains[].domain` | `string` |  |  |  |
| `spec.customDomains[].minTls` | `string` |  |  |  |
| `spec.customDomains[].ciphers` | `[]string` |  |  |  |
| `spec.jurisdiction` | `string` |  | `default` |  |
| `spec.storageClass` | `enum` |  | `Standard` |  |
| `spec.cors` | `CloudflareR2BucketCorsConfig` |  |  |  |
| `spec.cors.rules` | `[]CloudflareR2BucketCorsRule` |  |  |  |
| `spec.cors.rules[].allowed` | `CloudflareR2BucketCorsAllowed` | yes |  |  |
| `spec.cors.rules[].allowed.methods` | `[]enum` | yes |  |  |
| `spec.cors.rules[].allowed.origins` | `[]string` | yes |  |  |
| `spec.cors.rules[].allowed.headers` | `[]string` |  |  |  |
| `spec.cors.rules[].id` | `string` |  |  |  |
| `spec.cors.rules[].exposeHeaders` | `[]string` |  |  |  |
| `spec.cors.rules[].maxAgeSeconds` | `int64` |  |  |  |
| `spec.lifecycle` | `CloudflareR2BucketLifecycleConfig` |  |  |  |
| `spec.lifecycle.rules` | `[]CloudflareR2BucketLifecycleRule` |  |  |  |
| `spec.lifecycle.rules[].id` | `string` | yes |  |  |
| `spec.lifecycle.rules[].conditions` | `CloudflareR2BucketLifecycleConditions` | yes |  |  |
| `spec.lifecycle.rules[].conditions.prefix` | `string` |  |  |  |
| `spec.lifecycle.rules[].enabled` | `bool` |  |  |  |
| `spec.lifecycle.rules[].abortMultipartUploadsTransition` | `CloudflareR2BucketLifecycleAbortMultipartUploadsTransition` |  |  |  |
| `spec.lifecycle.rules[].abortMultipartUploadsTransition.maxAgeSeconds` | `int64` | yes |  |  |
| `spec.lifecycle.rules[].deleteObjectsTransition` | `CloudflareR2BucketLifecycleDeleteObjectsTransition` |  |  |  |
| `spec.lifecycle.rules[].deleteObjectsTransition.condition` | `CloudflareR2BucketLifecycleTransitionCondition` | yes |  |  |
| `spec.lifecycle.rules[].deleteObjectsTransition.condition.type` | `enum` | yes |  |  |
| `spec.lifecycle.rules[].deleteObjectsTransition.condition.maxAgeSeconds` | `int64` |  |  |  |
| `spec.lifecycle.rules[].deleteObjectsTransition.condition.date` | `string` |  |  |  |
| `spec.lifecycle.rules[].storageClassTransitions` | `[]CloudflareR2BucketLifecycleStorageClassTransition` |  |  |  |
| `spec.lifecycle.rules[].storageClassTransitions[].condition` | `CloudflareR2BucketLifecycleTransitionCondition` | yes |  |  |
| `spec.lifecycle.rules[].storageClassTransitions[].condition.type` | `enum` | yes |  |  |
| `spec.lifecycle.rules[].storageClassTransitions[].condition.maxAgeSeconds` | `int64` |  |  |  |
| `spec.lifecycle.rules[].storageClassTransitions[].condition.date` | `string` |  |  |  |
| `spec.lock` | `CloudflareR2BucketLockConfig` |  |  |  |
| `spec.lock.rules` | `[]CloudflareR2BucketLockRule` |  |  |  |
| `spec.lock.rules[].id` | `string` | yes |  |  |
| `spec.lock.rules[].condition` | `CloudflareR2BucketLockRuleCondition` | yes |  |  |
| `spec.lock.rules[].condition.type` | `enum` | yes |  |  |
| `spec.lock.rules[].condition.maxAgeSeconds` | `int64` |  |  |  |
| `spec.lock.rules[].condition.date` | `string` |  |  |  |
| `spec.lock.rules[].enabled` | `bool` |  |  |  |
| `spec.lock.rules[].prefix` | `string` |  |  |  |
| `spec.eventNotifications` | `[]CloudflareR2BucketEventNotification` |  |  |  |
| `spec.eventNotifications[].queue` | `string \| valueFrom` | yes |  | CloudflareQueue (`status.outputs.queue_id`) |
| `spec.eventNotifications[].rules` | `[]CloudflareR2BucketEventNotificationRule` | yes |  |  |
| `spec.eventNotifications[].rules[].actions` | `[]string` | yes |  |  |
| `spec.eventNotifications[].rules[].description` | `string` |  |  |  |
| `spec.eventNotifications[].rules[].prefix` | `string` |  |  |  |
| `spec.eventNotifications[].rules[].suffix` | `string` |  |  |  |

## Field Details

### spec.bucketName

`string` · required

bucket name (DNS-compatible, 3-63 characters)

- rule: {"required":true,"string":{"minLen":"3","maxLen":"63","pattern":"^[a-z0-9]([-a-z0-9]*[a-z0-9])?$"}}

### spec.accountId

`string` · required

The Cloudflare account ID in which to create the bucket.

- rule: {"required":true,"string":{"len":"32","pattern":"^[0-9a-fA-F]{32}$"}}

### spec.location

`enum`

Primary region for the bucket (location hint). Leave as `auto` to let
Cloudflare choose the optimal region; the hint is only honored when the
bucket is first created and is best-effort, not a guarantee.

- default: `auto`

Allowed values (use exactly as shown):

- `auto`
- `wnam`
- `enam`
- `weur`
- `eeur`
- `apac`
- `oc`

### spec.publicAccess

`bool`

Expose the bucket publicly over Cloudflare's managed `r2.dev` domain. When
true, a managed public domain is enabled and its URL is published as the
`public_url` stack output. Custom domains (below) are the production-grade
path; the r2.dev domain is rate-limited and intended for development.

### spec.customDomains

`[]CloudflareR2BucketCustomDomainConfig`

Custom domains that serve the bucket's objects over your own hostnames
(e.g. "media.example.com"). A bucket may have multiple custom domains.

- rule: {"repeated":{"maxItems":"50"}}
- rule: zone_id is required when a custom domain is enabled
- rule: domain is required when a custom domain is enabled

### spec.customDomains[].enabled

`bool`

Whether to enable public access to the bucket at this custom domain.

### spec.customDomains[].zoneId

`string | valueFrom`

The Cloudflare Zone ID that hosts this custom domain. Can be a literal
value or referenced from a CloudflareDnsZone resource. Required when enabled.

- references: CloudflareDnsZone (`status.outputs.zone_id`)
- rule: write as {value: <literal>} or {valueFrom: {kind: CloudflareDnsZone, name: <that resource's name>, fieldPath: status.outputs.zone_id}} -- a bare string does not parse

### spec.customDomains[].domain

`string`

The full domain name used to access the bucket (e.g., "media.example.com").
Must be within the zone specified by zone_id. Required when enabled.

- rule: {"string":{"maxLen":"253"}}

### spec.customDomains[].minTls

`string`

Minimum TLS version this custom domain accepts for incoming connections.
One of "1.0", "1.1", "1.2", "1.3"; defaults to "1.0" when omitted.

- rule: min_tls must be one of "1.0", "1.1", "1.2", "1.3"

### spec.customDomains[].ciphers

`[]string`

Optional allowlist of TLS ciphers (BoringSSL format) for TLS termination
at this custom domain. Leave empty to use Cloudflare's default cipher suite.

### spec.jurisdiction

`string`

Data-residency jurisdiction for the bucket, fixed at creation and part of
the bucket's identity. One of "default" (standard global storage), "eu"
(European Union residency), "fedramp" (US FedRAMP), or "us" (United States
residency). Leave empty for "default". Every bucket-scoped sub-resource is
created in this jurisdiction. (Worker and Pages R2 BINDINGS accept a
different value set -- "eu", "fedramp", "fedramp-high" -- each surface
mirrors its own provider vocabulary deliberately.)

- default: `default`
- rule: jurisdiction must be one of "default", "eu", "fedramp", "us"

### spec.storageClass

`enum`

Default storage class for newly uploaded objects. `Standard` suits
frequently accessed data; `InfrequentAccess` lowers storage cost for
rarely accessed data at the expense of retrieval fees.

- default: `Standard`

Allowed values (use exactly as shown):

- `storage_class_unspecified`
- `Standard`
- `InfrequentAccess`

### spec.cors

`CloudflareR2BucketCorsConfig`

Cross-Origin Resource Sharing (CORS) configuration. Required when a web
application served from another origin needs browser access to the bucket.

### spec.cors.rules

`[]CloudflareR2BucketCorsRule`

Ordered list of CORS rules evaluated for each cross-origin request.

- rule: {"repeated":{"maxItems":"100"}}

### spec.cors.rules[].allowed

`CloudflareR2BucketCorsAllowed` · required

Allowed origins, methods, and request headers for this rule.

- rule: {"required":true}

### spec.cors.rules[].allowed.methods

`[]enum` · required

HTTP methods the rule allows (Access-Control-Allow-Methods).

- rule: {"repeated":{"minItems":"1","items":{"enum":{"definedOnly":true,"notIn":[0]}}}}

Allowed values (use exactly as shown):

- `cors_allowed_method_unspecified`
- `GET`
- `PUT`
- `POST`
- `DELETE`
- `HEAD`

### spec.cors.rules[].allowed.origins

`[]string` · required

Allowed request origins (Access-Control-Allow-Origin), e.g.
"https://app.example.com". Use ["*"] to allow any origin.

- rule: {"repeated":{"minItems":"1"}}

### spec.cors.rules[].allowed.headers

`[]string`

Allowed request headers (Access-Control-Allow-Headers) for preflighted
cross-origin requests that include custom headers (e.g., "x-user-id").

### spec.cors.rules[].id

`string`

Optional identifier for this rule.

### spec.cors.rules[].exposeHeaders

`[]string`

Response headers exposed to client-side JavaScript beyond the CORS-safelisted
set (e.g., "Content-Encoding", "cf-cache-status").

### spec.cors.rules[].maxAgeSeconds

`int64`

How long (in seconds) browsers may cache the CORS preflight response.
Browsers may cap this at 2 hours regardless; maximum is 86400 (24h).

- rule: max_age_seconds must be between 0 and 86400

### spec.lifecycle

`CloudflareR2BucketLifecycleConfig`

Object lifecycle configuration: automatic storage-class transitions,
object expiration, and cleanup of incomplete multipart uploads.

### spec.lifecycle.rules

`[]CloudflareR2BucketLifecycleRule`

Lifecycle rules applied to objects in the bucket.

- rule: {"repeated":{"maxItems":"1000"}}

### spec.lifecycle.rules[].id

`string` · required

Unique identifier for this rule.

- rule: {"required":true,"string":{"minLen":"1"}}

### spec.lifecycle.rules[].conditions

`CloudflareR2BucketLifecycleConditions` · required

Object selector for this rule. An empty prefix scopes the rule to all objects.

- rule: {"required":true}

### spec.lifecycle.rules[].conditions.prefix

`string`

Only objects whose key starts with this prefix are affected. Empty = all objects.

### spec.lifecycle.rules[].enabled

`bool`

Whether this rule is in effect.

### spec.lifecycle.rules[].abortMultipartUploadsTransition

`CloudflareR2BucketLifecycleAbortMultipartUploadsTransition`

Abort incomplete multipart uploads older than a given age.

### spec.lifecycle.rules[].abortMultipartUploadsTransition.maxAgeSeconds

`int64` · required

Age in seconds after which incomplete multipart uploads are aborted.

- rule: {"required":true,"int64":{"gt":"0"}}

### spec.lifecycle.rules[].deleteObjectsTransition

`CloudflareR2BucketLifecycleDeleteObjectsTransition`

Delete (expire) objects by age or on a date.

### spec.lifecycle.rules[].deleteObjectsTransition.condition

`CloudflareR2BucketLifecycleTransitionCondition` · required

Condition that triggers deletion (by age or on a date).

- rule: {"required":true}
- rule: lifecycle condition type must be Age or Date
- rule: max_age_seconds must be > 0 when type is Age
- rule: date (RFC3339) is required when type is Date

### spec.lifecycle.rules[].deleteObjectsTransition.condition.type

`enum` · required

Whether the transition is triggered by object age or by a calendar date.

- rule: {"required":true,"enum":{"definedOnly":true}}

Allowed values (use exactly as shown):

- `condition_type_unspecified`
- `Age`
- `Date`
- `Indefinite`

### spec.lifecycle.rules[].deleteObjectsTransition.condition.maxAgeSeconds

`int64`

Age in seconds since object creation (used when type is Age).

### spec.lifecycle.rules[].deleteObjectsTransition.condition.date

`string`

RFC3339 date on/after which the transition applies (used when type is Date),
e.g. "2027-01-01T00:00:00Z".

### spec.lifecycle.rules[].storageClassTransitions

`[]CloudflareR2BucketLifecycleStorageClassTransition`

Transition objects to a cheaper storage class (Infrequent Access) by age or date.

### spec.lifecycle.rules[].storageClassTransitions[].condition

`CloudflareR2BucketLifecycleTransitionCondition` · required

Condition that triggers the transition (by age or on a date).

- rule: {"required":true}
- rule: lifecycle condition type must be Age or Date
- rule: max_age_seconds must be > 0 when type is Age
- rule: date (RFC3339) is required when type is Date

### spec.lifecycle.rules[].storageClassTransitions[].condition.type

`enum` · required

Whether the transition is triggered by object age or by a calendar date.

- rule: {"required":true,"enum":{"definedOnly":true}}

Allowed values (use exactly as shown):

- `condition_type_unspecified`
- `Age`
- `Date`
- `Indefinite`

### spec.lifecycle.rules[].storageClassTransitions[].condition.maxAgeSeconds

`int64`

Age in seconds since object creation (used when type is Age).

### spec.lifecycle.rules[].storageClassTransitions[].condition.date

`string`

RFC3339 date on/after which the transition applies (used when type is Date),
e.g. "2027-01-01T00:00:00Z".

### spec.lock

`CloudflareR2BucketLockConfig`

Object lock configuration: retention rules that prevent objects from being
deleted or overwritten for a period (or indefinitely), for compliance.

### spec.lock.rules

`[]CloudflareR2BucketLockRule`

Object-lock retention rules applied to objects in the bucket.

- rule: {"repeated":{"maxItems":"1000"}}

### spec.lock.rules[].id

`string` · required

Unique identifier for this rule.

- rule: {"required":true,"string":{"minLen":"1"}}

### spec.lock.rules[].condition

`CloudflareR2BucketLockRuleCondition` · required

Retention condition: how long matching objects are locked.

- rule: {"required":true}
- rule: max_age_seconds must be > 0 when type is Age
- rule: date (RFC3339) is required when type is Date

### spec.lock.rules[].condition.type

`enum` · required

Whether objects are locked for an age, until a date, or indefinitely.

- rule: {"required":true,"enum":{"definedOnly":true}}

Allowed values (use exactly as shown):

- `condition_type_unspecified`
- `Age`
- `Date`
- `Indefinite`

### spec.lock.rules[].condition.maxAgeSeconds

`int64`

Retention period in seconds (used when type is Age).

### spec.lock.rules[].condition.date

`string`

RFC3339 date until which objects are retained (used when type is Date).

### spec.lock.rules[].enabled

`bool`

Whether this rule is in effect.

### spec.lock.rules[].prefix

`string`

Only objects whose key starts with this prefix are locked. Empty = all objects.

### spec.eventNotifications

`[]CloudflareR2BucketEventNotification`

Event notifications: push object lifecycle events (uploads, deletions) to
Cloudflare Queues for downstream processing by a Worker. A bucket may notify
multiple queues; each entry filters the events it forwards by action and by an
optional key prefix/suffix.

- rule: {"repeated":{"maxItems":"100"}}

### spec.eventNotifications[].queue

`string | valueFrom` · required

The destination Cloudflare Queue id, or a reference to a CloudflareQueue resource.

- references: CloudflareQueue (`status.outputs.queue_id`)
- rule: {"required":true}
- rule: write as {value: <literal>} or {valueFrom: {kind: CloudflareQueue, name: <that resource's name>, fieldPath: status.outputs.queue_id}} -- a bare string does not parse

### spec.eventNotifications[].rules

`[]CloudflareR2BucketEventNotificationRule` · required

The rules that select which object events are forwarded to the queue.

- rule: {"repeated":{"minItems":"1"}}

### spec.eventNotifications[].rules[].actions

`[]string` · required

Object actions that trigger a notification, e.g. "PutObject", "CopyObject",
"DeleteObject", "CompleteMultipartUpload", "AbortMultipartUpload",
"LifecycleDeletion".

- rule: {"repeated":{"minItems":"1"}}

### spec.eventNotifications[].rules[].description

`string`

Optional human-readable description of the rule.

### spec.eventNotifications[].rules[].prefix

`string`

Only notify for objects whose key starts with this prefix. Empty = all keys.

### spec.eventNotifications[].rules[].suffix

`string`

Only notify for objects whose key ends with this suffix. Empty = all keys.

## Outputs

Reference an output from another manifest as `valueFrom: {kind: CloudflareR2Bucket, name: <resource-name>, fieldPath: status.outputs.<output>}`.

| Output | Type | Description |
|---|---|---|
| `status.outputs.bucket_name` | `string` | The name of the bucket (same as spec.bucket_name) |
| `status.outputs.bucket_url` | `string` | The path-style S3 API URL of the bucket: the account's S3 endpoint for the bucket's jurisdiction followed by the bucket name (e.g., https://<account_id>.r2.cloudflarestorage.com/<bucket>, or https://<account_id>.eu.r2.cloudflarestorage.com/<bucket> for an EU bucket). |
| `status.outputs.custom_domain_urls` | `[]string` | The custom-domain URLs configured for the bucket (one per enabled custom domain), e.g., ["https://media.example.com"]. |
| `status.outputs.public_url` | `string` | The Cloudflare-managed public URL (r2.dev) when public_access is enabled, e.g., https://pub-<hash>.r2.dev. Empty when public access is disabled. |
| `status.outputs.account_id` | `string` | The Cloudflare account that owns the bucket (same as spec.account_id). Exported so a consumer that composes this bucket into an S3-compatible client can reference the account beside the bucket name instead of repeating it. |
| `status.outputs.jurisdiction` | `string` | The bucket's data-residency jurisdiction, normalized: "default" when the spec left it empty, otherwise "eu", "fedramp", or "us" as declared. Part of the bucket's identity and the selector for its S3 endpoint host. |
| `status.outputs.s3_endpoint` | `string` | The S3 API endpoint that serves this bucket -- the only host that does. https://<account_id>.r2.cloudflarestorage.com for the default jurisdiction, https://<account_id>.<jurisdiction>.r2.cloudflarestorage.com otherwise; requests for a jurisdictional bucket against the default host fail rather than redirect. An S3 client configures this as its endpoint URL with region "auto"; path-style and virtual-hosted addressing both work. |

## References

Fields that can point at another resource's outputs:

| Field | Kind | Output |
|---|---|---|
| `spec.customDomains[].zoneId` | CloudflareDnsZone | `status.outputs.zone_id` |
| `spec.eventNotifications[].queue` | CloudflareQueue | `status.outputs.queue_id` |

## Referenced By

Fields on other kinds that can point at this resource:

| Kind | Field | Reads |
|---|---|---|
| CloudflarePagesProject | `spec.deploymentConfigs.preview.r2Buckets[].bucketName` | `status.outputs.bucket_name` |
| CloudflarePagesProject | `spec.deploymentConfigs.production.r2Buckets[].bucketName` | `status.outputs.bucket_name` |
| CloudflareWorker | `spec.r2Bundle.bucket` | `status.outputs.bucket_name` |
| CloudflareWorker | `spec.r2Buckets[].bucketName` | `status.outputs.bucket_name` |
| KubernetesMongodb | `spec.backup.storages[].r2.bucket` | `status.outputs.bucket_name` |
| KubernetesMongodb | `spec.backup.storages[].r2.accountId` | `status.outputs.account_id` |
| KubernetesMongodb | `spec.backup.storages[].r2.jurisdiction` | `status.outputs.jurisdiction` |
| KubernetesPostgres | `spec.bootstrap.recovery.objectStore.r2.accountId` | `status.outputs.account_id` |
| KubernetesPostgres | `spec.bootstrap.recovery.objectStore.r2.jurisdiction` | `status.outputs.jurisdiction` |
| KubernetesPostgres | `spec.backup.objectStore.r2.accountId` | `status.outputs.account_id` |
| KubernetesPostgres | `spec.backup.objectStore.r2.jurisdiction` | `status.outputs.jurisdiction` |

## See Also

- [Overview](../README.md)
