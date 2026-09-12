output "bucket_name" {
  description = "The name of the R2 bucket"
  value       = cloudflare_r2_bucket.main.name
}

output "bucket_url" {
  description = "The path-style S3 API URL of the bucket (its jurisdiction's endpoint plus the bucket name)"
  value       = local.bucket_url
}

output "account_id" {
  description = "The Cloudflare account that owns the bucket (same as spec.account_id)"
  value       = local.account_id
}

output "jurisdiction" {
  description = "The bucket's data-residency jurisdiction, normalized: default, eu, fedramp, or us"
  value       = local.jurisdiction_normalized
}

output "s3_endpoint" {
  description = "The S3 API endpoint that serves this bucket's jurisdiction -- the only host that does; configure S3 clients with it and region auto"
  value       = local.s3_endpoint
}

output "custom_domain_urls" {
  description = "URLs of the configured custom domains (one per enabled custom domain)"
  value       = [for domain, cd in cloudflare_r2_custom_domain.main : "https://${cd.domain}"]
}

output "public_url" {
  description = "The Cloudflare-managed r2.dev public URL, when public access is enabled"
  value       = local.public_access ? "https://${cloudflare_r2_managed_domain.main[0].domain}" : ""
}
