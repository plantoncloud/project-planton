# Cloudflare Account API Token

Deploys an account-owned Cloudflare API token: a scoped credential that belongs to the account rather than to any person, with its own permission policies, optional client-IP conditions, and an optional validity window. Because the token is not tied to a user it survives staff changes, which makes it the right shape for CI/CD and automation credentials. The token's secret value is returned by Cloudflare exactly once, at create time — capture it on the first apply, because no later read can recover it.

## What Gets Created

When you deploy this Cloud Resource, the IaC module provisions:

- **Account API Token** — one `cloudflare_account_token` carrying the name, permission policies, validity window, client-IP condition, and administrative status from the spec. Each policy's `resources` map travels to Cloudflare as a single raw JSON object; the module serializes the typed spec entries (whole-resource grant or nested sub-resource scoping) back to the API's shape.

Destroying the resource is a real delete: the credential stops working the same second, with no grace period. Retire consumers first, or park the token with `status: disabled` before deleting.

## Before You Deploy

### Planton Setup

- **Cloudflare Provider Connection** — an active connection in the Connect module whose API token carries **Account API Tokens → Write** (a token that can mint tokens). Pipeline tokens scoped to DNS or Workers alone fail with a flat authorization error at create time.
- **Planton Runner** — required when using Runner-based credential delivery. Not needed for inline credential authentication.

### Cloudflare Account

- **The account ID** (32-character hex) for `accountId` — the account that will own the token.
- **Permission-group UUIDs** for `policies[].permissionGroupIds` — Cloudflare identifies permission groups by UUID, not name. Fetch them with `GET /accounts/{account_id}/tokens/permission_groups` (filterable by name and scope) and pin the UUIDs you need.

## Deploy

### Console

Open the deployment store, find **Cloudflare Account API Token**, and click **Deploy**. The creation wizard walks you through environment and connection configuration, the owning account, the policy builder (effect, permission groups, and resource entries), and the optional IP condition and validity window. Start from the **CI DNS editor** preset in the [Presets](#presets) tab to pre-populate a working configuration.

### CLI

Create a manifest and apply it:

```yaml
apiVersion: cloudflare.planton.dev/v1alpha1
kind: CloudflareAccountApiToken
metadata:
  name: ci-dns-editor
  org: acme-corp
  env: prod
spec:
  accountId: "0a1b2c3d4e5f6a7b8c9d0e1f2a3b4c5d"
  name: ci-dns-editor
  policies:
    - effect: allow
      permissionGroupIds:
        - "4755a26eedb94da69e1066f98e79d058"
      resources:
        "com.cloudflare.api.account.0a1b2c3d4e5f6a7b8c9d0e1f2a3b4c5d":
          permission: "*"
```

```shell
planton apply -f account-api-token.yaml
```

This mints one active, never-expiring token whose single policy grants one permission group across the whole account. A Stack Job tracks the provisioning in real time.

## Key Configuration

These are the most important decisions when configuring an account API token. Explore the full field reference in the [API Explorer](#api-explorer) tab.

**Grant shape** — Each entry under `policies[].resources` is either a whole-resource grant (`permission`, normally `*`) or a nested scoping (`subresources`) — exactly one per entry. The difference is real: `permission: "*"` on `com.cloudflare.api.account.<id>` grants across the account, every zone present and future, while `subresources: {"com.cloudflare.api.account.zone.*": "*"}` scopes to the account's zones, and naming a specific zone ID instead of `*` narrows further. Picking the whole-account form when you meant the zone form silently over-grants.

**Deny beats allow** — A policy with `effect: deny` overrides allow policies covering the same resources. Use it to carve one zone out of an otherwise account-wide grant instead of enumerating every allowed zone.

**One chance at the value** — Cloudflare returns the secret in the create response and never again; an imported token arrives with an empty value. The recovery procedure for a lost value is rotation — delete and recreate the token, then update every consumer. Capture the `value` output into a managed secret on the first apply.

**Validity window** — `expiresOn` and `notBefore` (RFC 3339) time-box the credential so nobody has to remember to revoke it; the spec validates that the window runs forwards. An expired token is reported by Cloudflare with status `expired` and recreated — with a new secret — on the next apply.

**Staged revocation** — `status: disabled` suspends the credential without deleting it. Consumers start failing immediately, but you can re-enable in seconds if the revocation was wrong. Confirming with `disabled` before deleting is the safe habit; a delete is instant and irreversible.

**Client-IP conditions** — `condition.requestIp.inCidrs` limits the token to named CIDRs (a CI egress range, an office network); `notInCidrs` bars ranges outright and is evaluated first. A stolen token that only works from your CI network is a much smaller incident.

**Order is not yours** — Cloudflare canonically re-orders policies and permission groups server-side. Treat both lists as sets: reordering them in the manifest is a no-op.

## Outputs and Dependencies

### What This Component Consumes

This component has no foreign key dependencies. The owning account travels as a literal 32-hex `accountId` string, and permission groups are Cloudflare-defined UUIDs rather than catalog resources.

### What This Component Provides

After provisioning, `status.outputs` contains values that downstream Cloud Resources can consume via ValueFromRef:

| Output | Description | Common Downstream Use |
|--------|-------------|----------------------|
| `token_id` | The Cloudflare-assigned token ID — the token's identity for management calls, not the credential | Referencing the token in rotation or audit tooling |
| `value` | The token's secret value, returned exactly once at create and secret-marked | Storing in a Secrets Store Secret for Workers and pipelines to consume |
| `r2_access_key_id` | The token as an S3 access key id for R2's S3 API -- Cloudflare defines it as the token's id; meaningful only when the token carries an R2 permission group | A database's or backup tool's R2 store credential, by reference |
| `r2_secret_access_key` | The token as an S3 secret access key for R2's S3 API -- the SHA-256 of the value, exactly what the dashboard shows for the same token; secret-marked, same once-on-create lifecycle | Paired with `r2_access_key_id` on any S3-compatible client pointed at R2 |

## Common Patterns

Browse the [Presets](#presets) tab for ready-to-deploy configurations.

**Pipeline credential** — DNS write scoped to the account's zones via the nested `subresources` form, usable only from the CI egress range. The narrow-by-default shape for automation. Start from the **CI DNS editor** preset.

**Time-boxed auditor access** — read-only permission groups with a whole-account grant, alive for exactly one quarter via `notBefore` and `expiresOn`, then dead on its own. Start from the **Expiring read-only audit token** preset.

**Rotation without a gap** — deploy the replacement token as a second Cloud Resource, move consumers to it, then set the old token to `status: disabled` and watch for stragglers before destroying it.

## Works With

- [**Cloudflare Secrets Store Secret**](/cloud-catalog/cloudflare-secrets-store-secret) — where the minted `value` can live so Workers and pipelines consume it without re-handling the credential
- [**Cloudflare Zero Trust Access Service Token**](/cloud-catalog/cloudflare-zero-trust-access-service-token) — machine credentials for Access-protected applications, a different trust domain from API tokens
