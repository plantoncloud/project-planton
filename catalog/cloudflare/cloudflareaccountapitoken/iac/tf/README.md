# CloudflareAccountApiToken Terraform Module

Terraform IaC module for account-owned API tokens.

## Architecture

```
provider.tf   — Cloudflare provider configuration (~> 5.23)
variables.tf  — Input variables mirroring CloudflareAccountApiTokenSpec (generated)
locals.tf     — Naming/labels
main.tf       — cloudflare_account_token (policies serializer + condition mapping)
outputs.tf    — token_id, value, r2_access_key_id, r2_secret_access_key
```

## Behavior

A plain CRUD resource: real create/update/delete; only the account forces replacement.

Each policy's `resources` travels to Cloudflare as ONE raw JSON object. The spec types it -- per entry, either a whole-resource `permission` string or a nested `subresources` map -- and this module serializes it with `jsonencode`. Because a single conditional cannot return both a string and a map, the two shapes are built as separate type-homogeneous comprehensions and merged into one object.

The token's secret `value` is returned by Cloudflare only on create and is marked sensitive here. Cloudflare canonically re-orders policies and permission groups server-side, so treat both lists as sets.

Import as `{account_id}/{token_id}` -- configuration only; the value is never re-fetchable.

## Outputs

| Name | Description |
|------|-------------|
| `token_id` | The token's management id (not the credential) |
| `value` | The secret token value, returned once at create (sensitive) |
| `r2_access_key_id` | The token as an S3 access key id for R2's S3 API (the token's id) |
| `r2_secret_access_key` | The token as an S3 secret access key for R2's S3 API (`sha256(value)`, sensitive) |

## Provider Version

Uses `cloudflare/cloudflare ~> 5.23`.
