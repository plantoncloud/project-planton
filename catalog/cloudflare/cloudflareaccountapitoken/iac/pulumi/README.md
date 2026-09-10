# CloudflareAccountApiToken Pulumi Module

Pulumi (Go) IaC module for account-owned API tokens.

## Architecture

```
main.go                        — stack-input loading + module entry
module/main.go                 — provider setup + resource orchestration
module/locals.go               — metadata/credential references
module/account_api_token.go    — AccountToken (policies serializer + condition mapping)
module/outputs.go              — token_id, value, r2_access_key_id, r2_secret_access_key
```

## Behavior

A plain CRUD resource: real create/update/delete; only the account forces replacement.

Each policy's `resources` travels to Cloudflare as ONE raw JSON string. The spec types it -- per entry, either a whole-resource `permission` string or a nested `subresources` map -- and `serializeResources` marshals it back to the API's object shape. The SDK names the condition's CIDR lists `Ins`/`NotIns`; the spec names them `in_cidrs`/`not_in_cidrs` for clarity.

The token's secret `value` is returned by Cloudflare only on create; the SDK registers it as a secret output. Cloudflare canonically re-orders policies and permission groups server-side, so treat both lists as sets.

Import as `{account_id}/{token_id}` -- configuration only; the value is never re-fetchable.

## Outputs

| Name | Description |
|------|-------------|
| `token_id` | The token's management id (not the credential) |
| `value` | The secret token value, returned once at create (secret) |
| `r2_access_key_id` | The token as an S3 access key id for R2's S3 API (the token's id) |
| `r2_secret_access_key` | The token as an S3 secret access key for R2's S3 API (SHA-256 of the value, secret) |

## SDK Version

Uses `pulumi-cloudflare` v6.19.0.
