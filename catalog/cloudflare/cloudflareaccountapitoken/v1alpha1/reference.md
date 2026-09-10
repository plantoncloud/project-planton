# CloudflareAccountApiToken

> Generated from the protobuf schema by `make generate-reference` -- do not
> edit by hand. To change a fact on this page, change the proto field comment
> or validation rule it is derived from, then regenerate.

**apiVersion**: `cloudflare.planton.dev/v1alpha1`

**Guide**: [GUIDE.md](../GUIDE.md) -- authored operational judgment for this component: conventions, trade-offs, and what pairs well with it.

CloudflareAccountApiTokenSpec creates an account-owned API token: a
scoped credential belonging to the ACCOUNT (not any user), with its own
policies, optional client-IP conditions, and optional validity window.
The token's secret value is returned exactly once, by the create call --
it can never be fetched again (see the outputs). A plain CRUD object --
real create, update, delete.

Each policy grants (or denies) a set of permission groups over a set of
resources. Permission groups are Cloudflare-defined and referenced by
UUID; list them with
GET /accounts/{account_id}/tokens/permission_groups (filterable by name
and scope) -- the catalog deliberately does not model that read-only
registry. Cloudflare canonically re-orders policies and permission groups
on its side, so treat their order as insignificant.

## Example

```yaml
# Complete example manifest for CloudflareAccountApiToken.
# A read-only DNS token limited to one office network, expiring at the end
# of 2027.
apiVersion: cloudflare.planton.dev/v1alpha1
kind: CloudflareAccountApiToken
metadata:
  name: dns-readonly
spec:
  account_id: "0a1b2c3d4e5f6a7b8c9d0e1f2a3b4c5d"
  name: dns-readonly
  policies:
    - effect: allow
      permission_group_ids:
        - "REPLACE_WITH_PERMISSION_GROUP_UUID"
      resources:
        "com.cloudflare.api.account.0a1b2c3d4e5f6a7b8c9d0e1f2a3b4c5d":
          subresources:
            "com.cloudflare.api.account.zone.*": "*"
  condition:
    request_ip:
      in_cidrs:
        - "198.51.100.0/24"
  expires_on: "2027-12-31T23:59:59Z"
  status: active
```

## Spec Fields

| Path | Type | Required | Default | References |
|---|---|---|---|---|
| `spec.accountId` | `string` | yes |  |  |
| `spec.name` | `string` | yes |  |  |
| `spec.policies` | `[]CloudflareAccountApiTokenPolicy` | yes |  |  |
| `spec.policies[].effect` | `string` | yes |  |  |
| `spec.policies[].permissionGroupIds` | `[]string` | yes |  |  |
| `spec.policies[].resources` | `map<string, CloudflareAccountApiTokenResourceScope>` | yes |  |  |
| `spec.policies[].resources.*.permission` | `string` |  |  |  |
| `spec.policies[].resources.*.subresources` | `map<string, string>` |  |  |  |
| `spec.expiresOn` | `string` |  |  |  |
| `spec.notBefore` | `string` |  |  |  |
| `spec.condition` | `CloudflareAccountApiTokenCondition` |  |  |  |
| `spec.condition.requestIp` | `CloudflareAccountApiTokenRequestIp` |  |  |  |
| `spec.condition.requestIp.inCidrs` | `[]string` |  |  |  |
| `spec.condition.requestIp.notInCidrs` | `[]string` |  |  |  |
| `spec.status` | `string` |  |  |  |

## Field Details

### spec.accountId

`string` · required

The Cloudflare account that owns the token.

- rule: account_id must be a 32-character hex string
- rule: {"required":true}

### spec.name

`string` · required

The token's name, shown in the account's API Tokens list.

- rule: {"required":true}

### spec.policies

`[]CloudflareAccountApiTokenPolicy` · required

What the token may (or may not) do. At least one policy is required.

- rule: {"repeated":{"minItems":"1"}}

### spec.policies[].effect

`string` · required

Whether the policy grants (allow) or withholds (deny) the permissions.
Deny policies override allow policies that cover the same resources.

- rule: {"required":true,"string":{"in":["allow","deny"]}}

### spec.policies[].permissionGroupIds

`[]string` · required

The Cloudflare permission groups this policy applies, by UUID (e.g.
"DNS Read", "Workers Scripts Write"). List available groups with
GET /accounts/{account_id}/tokens/permission_groups.

- rule: {"repeated":{"minItems":"1"}}

### spec.policies[].resources

`map<string, CloudflareAccountApiTokenResourceScope>` · required

The resources the policy covers, keyed by Cloudflare resource
identifier (e.g. "com.cloudflare.api.account.<account_id>" or
"com.cloudflare.api.account.zone.<zone_id>"). Each value either grants
the whole resource (permission, normally "*") or scopes into
sub-resources (subresources) -- exactly one per entry. Cloudflare's API
takes this map as one raw JSON object; the modules serialize it.

- rule: {"map":{"minPairs":"1"}}
- rule: set exactly one of permission (whole-resource grant, normally "*") or subresources (nested scoping)

### spec.policies[].resources.*.permission

`string`

Whole-resource grant, normally "*" (all operations the policy's
permission groups allow).

### spec.policies[].resources.*.subresources

`map<string, string>`

Nested scoping: sub-resource identifier (e.g.
"com.cloudflare.api.account.zone.*") to its grant (normally "*").

### spec.expiresOn

`string`

When the token expires, RFC 3339 (e.g. 2027-01-01T00:00:00Z). Empty
never expires. Cloudflare reports an expired token with status
"expired" and the provider recreates it on the next apply.

- rule: expires_on must be an RFC 3339 timestamp (e.g. 2027-01-01T00:00:00Z)

### spec.notBefore

`string`

The earliest time the token works, RFC 3339. Empty is immediately.

- rule: not_before must be an RFC 3339 timestamp (e.g. 2026-09-01T00:00:00Z)

### spec.condition

`CloudflareAccountApiTokenCondition`

Restricts which client IPs may use the token.

### spec.condition.requestIp

`CloudflareAccountApiTokenRequestIp`

Client IP restrictions checked on every API request made with the
token.

### spec.condition.requestIp.inCidrs

`[]string`

CIDRs the token may be used from (IPv4 or IPv6, e.g. 198.51.100.0/24).
Empty allows any origin not explicitly denied.

### spec.condition.requestIp.notInCidrs

`[]string`

CIDRs the token is barred from, evaluated before in_cidrs.

### spec.status

`string`

The token's administrative state: active (usable) or disabled
(suspended without deleting). Cloudflare additionally reports
server-side states -- "expired" and "revoked (exposed)" -- that are
never configured, only observed; the provider drops such tokens from
state and recreates them on the next apply.

- rule: status must be active or disabled

## Validation Rules

- `spec.validity_window`: not_before must be earlier than expires_on when both are set

## Outputs

Reference an output from another manifest as `valueFrom: {kind: CloudflareAccountApiToken, name: <resource-name>, fieldPath: status.outputs.<output>}`.

| Output | Type | Description |
|---|---|---|
| `status.outputs.token_id` | `string` | The Cloudflare-assigned token ID (the token's identity for management calls -- NOT the credential itself). |
| `status.outputs.value` | `string` | The token's secret value -- the credential. Returned by Cloudflare EXACTLY ONCE, on create: it can never be fetched again, and an imported token has no value at all. Sensitive both here (machine-readable) and in the modules' output registration. If the value is lost, rotate: roll the token (or delete and recreate) to mint a new one. |
| `status.outputs.r2_access_key_id` | `string` | The token as an S3 access key id for Cloudflare R2's S3 API: Cloudflare defines it as the token's id (the same value as token_id, exported under the name an S3 client configures). Meaningful only when the token carries an R2 permission group (Workers R2 Storage Write/Read at the account, or Workers R2 Storage Bucket Item Write/Read scoped to buckets); a token without one authorizes nothing on the S3 API. Pair with r2_secret_access_key, the endpoint of the bucket's jurisdiction, and region "auto". |
| `status.outputs.r2_secret_access_key` | `string` | The token as an S3 secret access key for Cloudflare R2's S3 API: Cloudflare defines it as the lowercase hex SHA-256 of the token's value -- exactly the "Secret Access Key" the dashboard shows when the same token is created there. Derived from `value`, so it shares its lifecycle: available from the create that minted the token, absent on an imported token, and rotated by rotating the token. Sensitive. |

## Referenced By

Fields on other kinds that can point at this resource:

| Kind | Field | Reads |
|---|---|---|
| KubernetesMongodb | `spec.backup.storages[].r2.credentials.accessKeyId` | `status.outputs.r2_access_key_id` |
| KubernetesMongodb | `spec.backup.storages[].r2.credentials.secretAccessKey` | `status.outputs.r2_secret_access_key` |
| KubernetesPostgres | `spec.bootstrap.recovery.objectStore.r2.credentials.accessKeyId` | `status.outputs.r2_access_key_id` |
| KubernetesPostgres | `spec.bootstrap.recovery.objectStore.r2.credentials.secretAccessKey` | `status.outputs.r2_secret_access_key` |
| KubernetesPostgres | `spec.backup.objectStore.r2.credentials.accessKeyId` | `status.outputs.r2_access_key_id` |
| KubernetesPostgres | `spec.backup.objectStore.r2.credentials.secretAccessKey` | `status.outputs.r2_secret_access_key` |

## See Also

- [Overview](../README.md)
