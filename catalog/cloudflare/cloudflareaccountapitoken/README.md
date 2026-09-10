# Cloudflare Account API Token

## Overview

`CloudflareAccountApiToken` creates an account-owned API token: a scoped Cloudflare credential that belongs to the ACCOUNT rather than to any person, with its own permission policies, optional client-IP restrictions, and an optional validity window. Because it is not tied to a user, it survives staff changes -- the right shape for automation. The secret value is returned exactly once, by the create call. A plain CRUD object -- real create, update, delete.

## Key Features

- **Least-privilege policies** -- each policy binds permission groups (allow or deny) to explicitly named account and zone resources; deny overrides allow
- **Typed resource scoping** -- grant a whole resource, or scope into sub-resources (all zones in an account, for instance), with the two shapes distinguished in the spec rather than hidden in a JSON blob
- **Client-IP conditions** -- restrict the token to named CIDRs, and bar others outright
- **Validity window** -- `not_before` and `expires_on`, validated as RFC 3339 and checked against each other
- **Suspend without deleting** -- `status: disabled` parks a token so a revocation can be tested before it is made permanent

## Use Cases

**Ideal for:**

- CI/CD credentials scoped to exactly the zones and permissions a pipeline needs
- Automation tokens that outlive the engineer who created them
- Short-lived tokens for a migration or an audit, bounded by `expires_on`

**Not ideal for:**

- A person's own credential -- that is Cloudflare's user-scoped API token, a different resource with a different ownership model
- Cloudflare's legacy global API key, which this component deliberately never models

## API Specification

### Required Fields

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `account_id` | string | Yes | The account that owns the token (32-hex). |
| `name` | string | Yes | Shown in the account's API Tokens list. |
| `policies` | list | Yes | At least one policy: effect, permission groups, and resources. |

### Key Optional Fields

| Field | Type | Description |
|-------|------|-------------|
| `expires_on` | string | RFC 3339 expiry; empty never expires. |
| `not_before` | string | RFC 3339 start; empty is immediately. |
| `condition.request_ip.in_cidrs` | list | Client CIDRs the token may be used from. |
| `condition.request_ip.not_in_cidrs` | list | Client CIDRs barred, evaluated first. |
| `status` | string | `active` or `disabled`. |

### Policy Shape

| Field | Type | Description |
|-------|------|-------------|
| `effect` | string | `allow` or `deny`. |
| `permission_group_ids` | list | Cloudflare permission-group UUIDs (at least one). |
| `resources` | map | Resource identifier to grant: either `permission` (whole resource, normally `*`) or `subresources` (nested scoping) -- exactly one per entry. |

### Stack Outputs

| Field | Description |
|-------|-------------|
| `token_id` | The token's management id (not the credential) |
| `value` | The secret token value -- returned once, at create; secret-marked |
| `r2_access_key_id` | The token as an S3 access key id for R2's S3 API (the token's id); meaningful only with an R2 permission group |
| `r2_secret_access_key` | The token as an S3 secret access key for R2's S3 API (SHA-256 of the value); secret-marked, same once-on-create lifecycle |

## Example Manifest

```yaml
apiVersion: cloudflare.planton.dev/v1alpha1
kind: CloudflareAccountApiToken
metadata:
  name: ci-dns-editor
spec:
  account_id: "0a1b2c3d4e5f6a7b8c9d0e1f2a3b4c5d"
  name: ci-dns-editor
  policies:
    - effect: allow
      permission_group_ids:
        - "4755a26eedb94da69e1066f98e79d058"
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

## Prerequisites

- **A Cloudflare API token** carrying the Account API Tokens Write permission (a token that can mint tokens)
- **Permission-group UUIDs** -- list them with `GET /accounts/{account_id}/tokens/permission_groups`, filterable by name and scope

## Destroy Semantics

Real delete. The credential stops working immediately -- anything still using it starts failing authentication, so retire consumers first.

## Related Components

- [Cloudflare Zero Trust Access Service Token](/docs/catalog/cloudflare/cloudflarezerotrustaccessservicetoken) -- machine credentials for Access-protected applications, a different trust domain
- [Cloudflare Secrets Store Secret](/docs/catalog/cloudflare/cloudflaresecretsstoresecret) -- where a minted token value can be stored for Workers to consume

---

© Planton. Licensed under [Apache-2.0](https://github.com/plantonhq/planton/blob/main/LICENSE).
