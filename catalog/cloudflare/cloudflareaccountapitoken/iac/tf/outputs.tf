output "token_id" {
  description = "The Cloudflare-assigned token ID (the token's identity for management calls, not the credential)"
  value       = cloudflare_account_token.main.id
}

output "value" {
  description = "The token's secret value -- returned by Cloudflare exactly once, on create; if lost, rotate"
  value       = cloudflare_account_token.main.value
  sensitive   = true
}

# The same token in the shape R2's S3 API authenticates. Cloudflare's rule:
# access key id = the token's id; secret access key = the lowercase hex SHA-256
# of the token's value. The Go twin of this derivation is the shared helper
# package pkg/cloudflare/r2 (the source of truth both engines follow); HCL's
# sha256() yields the same lowercase hex. Meaningful only when the token carries
# an R2 permission group.
output "r2_access_key_id" {
  description = "The token as an S3 access key id for R2's S3 API (the token's id)"
  value       = cloudflare_account_token.main.id
}

output "r2_secret_access_key" {
  description = "The token as an S3 secret access key for R2's S3 API (SHA-256 of the token's value); shares the value's once-on-create lifecycle"
  value       = sha256(cloudflare_account_token.main.value)
  sensitive   = true
}
