package module

const (
	// OpBucketName is the exported stack output containing the bucket name.
	OpBucketName = "bucket_name"
	// OpBucketUrl is the exported stack output containing the path-style S3 API URL
	// of the bucket (its jurisdiction's endpoint plus the bucket name).
	OpBucketUrl = "bucket_url"
	// OpCustomDomainUrls is the exported stack output containing the custom-domain URLs.
	OpCustomDomainUrls = "custom_domain_urls"
	// OpPublicUrl is the exported stack output containing the managed r2.dev public URL.
	OpPublicUrl = "public_url"
	// OpAccountId is the exported stack output containing the owning account id.
	OpAccountId = "account_id"
	// OpJurisdiction is the exported stack output containing the normalized
	// jurisdiction ("default" when the spec left it empty).
	OpJurisdiction = "jurisdiction"
	// OpS3Endpoint is the exported stack output containing the S3 API endpoint
	// that serves the bucket's jurisdiction.
	OpS3Endpoint = "s3_endpoint"
)
