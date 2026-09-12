locals {
  # Resource naming
  resource_name = coalesce(try(var.metadata.name, null), "cloudflare-r2-bucket")

  # Labels/tags
  labels = merge({
    "name" = local.resource_name
  }, try(var.metadata.labels, {}))

  # Core bucket configuration
  bucket_name = var.spec.bucket_name
  account_id  = var.spec.account_id

  # Location hint. The enum value already matches Cloudflare's expected string;
  # "auto" means "no hint", which the provider expresses by omitting the field.
  location      = coalesce(try(var.spec.location, null), "auto")
  location_hint = local.location == "auto" ? null : local.location

  # Jurisdiction is part of the bucket identity and is applied to the bucket and
  # every bucket-scoped sub-resource. Omitted -> null -> provider default "default".
  jurisdiction = try(var.spec.jurisdiction, null) != null && try(var.spec.jurisdiction, "") != "" ? var.spec.jurisdiction : null

  # Default storage class for new objects. Omitted -> null -> provider default "Standard".
  storage_class = try(var.spec.storage_class, null) != null && try(var.spec.storage_class, "") != "" ? var.spec.storage_class : null

  # Public access via the managed r2.dev domain.
  public_access = coalesce(try(var.spec.public_access, null), false)

  # The bucket's jurisdiction as Cloudflare reports it: "default" when unset.
  jurisdiction_normalized = coalesce(local.jurisdiction, "default")

  # The S3 API endpoint that serves this bucket -- the ONLY host that does. A
  # bucket created in "eu" (or "fedramp", "us") is served through
  # <account>.<jurisdiction>.r2.cloudflarestorage.com; a request against the
  # default host fails rather than redirects. The host table mirrors the Go
  # helper package pkg/cloudflare/r2 (the source of truth both engines follow).
  s3_endpoint = local.jurisdiction_normalized == "default" ? "https://${local.account_id}.r2.cloudflarestorage.com" : "https://${local.account_id}.${local.jurisdiction_normalized}.r2.cloudflarestorage.com"

  # Path-style S3 API URL for the bucket: the jurisdiction's endpoint plus the name.
  bucket_url = "${local.s3_endpoint}/${local.bucket_name}"

  # Enabled custom domains, keyed by domain name for for_each.
  custom_domains_enabled = {
    for cd in coalesce(try(var.spec.custom_domains, []), []) :
    cd.domain => cd if try(cd.enabled, false)
  }

  # CORS rules (empty list when no cors block).
  cors_rules = try(var.spec.cors.rules, [])

  # Lifecycle rules (empty list when no lifecycle block).
  lifecycle_rules = try(var.spec.lifecycle.rules, [])

  # Lock rules (empty list when no lock block).
  lock_rules = try(var.spec.lock.rules, [])

  # Event notifications, keyed by destination queue for a stable for_each.
  event_notifications = { for en in try(var.spec.event_notifications, []) : en.queue => en }
}
