package module

const (
	// OpTokenId is the exported stack output containing the
	// Cloudflare-assigned token ID (the token's identity for management
	// calls, not the credential).
	OpTokenId = "token_id"

	// OpValue is the exported stack output containing the token's secret
	// value -- returned by Cloudflare exactly once, on create (secret-marked).
	OpValue = "value"

	// OpR2AccessKeyId is the exported stack output containing the token as an
	// S3 access key id for R2's S3 API (Cloudflare's rule: the token's id).
	OpR2AccessKeyId = "r2_access_key_id"

	// OpR2SecretAccessKey is the exported stack output containing the token as
	// an S3 secret access key for R2's S3 API (Cloudflare's rule: the SHA-256
	// of the token's value; secret-marked like the value it derives from).
	OpR2SecretAccessKey = "r2_secret_access_key"
)
