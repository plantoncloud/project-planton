// Package r2 holds the facts an S3 client needs to talk to Cloudflare R2, in
// one place: which host serves a bucket (the account plus the bucket's
// jurisdiction) and how a Cloudflare API token becomes an S3 key pair.
//
// Every catalog module that renders R2 into an S3-compatible client (the
// bucket kind's own endpoint output, a database's object-store arm, a backup
// tool's destination) derives these values here rather than carrying its own
// copy of the host table or the hash rule. The Terraform twins of those
// modules cannot import Go; each carries the same table in HCL with a comment
// naming this package as the source of truth to keep in step.
//
// Both rules are Cloudflare's, published in the R2 documentation:
//
//   - S3 API endpoint: https://<account_id>.r2.cloudflarestorage.com for a
//     bucket in the default jurisdiction; jurisdictional buckets are served
//     ONLY through their own host (<account_id>.eu.r2.cloudflarestorage.com,
//     .fedramp., .us.) -- a request for an EU bucket against the default host
//     fails, it is not redirected.
//   - S3 credentials from an account API token: the access key id is the
//     token's id; the secret access key is the lowercase hex SHA-256 of the
//     token's value. The token must carry an R2 permission group for the pair
//     to authorize anything.
package r2

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
)

// Region is the only region R2 accepts on the S3 API. Cloudflare aliases an
// empty region and "us-east-1" to it for clients that insist on a region.
const Region = "auto"

// JurisdictionDefault is the jurisdiction of a bucket created without one:
// standard global storage, served from the account's default host. The R2
// bucket kind reports an empty jurisdiction as this value on its outputs so
// consumers never have to treat "" and "default" as two cases.
const JurisdictionDefault = "default"

// Jurisdictions lists the values Cloudflare accepts for a bucket's data
// residency, each of which selects its own S3 host.
var Jurisdictions = []string{JurisdictionDefault, "eu", "fedramp", "us"}

// IsJurisdiction reports whether s names a jurisdiction Cloudflare accepts.
// The empty string is accepted as the default jurisdiction.
func IsJurisdiction(s string) bool {
	if s == "" {
		return true
	}
	for _, j := range Jurisdictions {
		if j == s {
			return true
		}
	}
	return false
}

// NormalizeJurisdiction maps the empty jurisdiction to JurisdictionDefault and
// returns any other value unchanged.
func NormalizeJurisdiction(s string) string {
	if s == "" {
		return JurisdictionDefault
	}
	return s
}

// S3Endpoint returns the S3 API endpoint that serves buckets of the given
// jurisdiction in the given account: https://<account>.r2.cloudflarestorage.com
// for the default jurisdiction, https://<account>.<jurisdiction>.r2.cloudflarestorage.com
// otherwise. The caller validates the jurisdiction (IsJurisdiction); an
// unknown value is rendered as written so a typo surfaces as a DNS failure
// naming the bad host rather than silently landing on the default host.
func S3Endpoint(accountID, jurisdiction string) string {
	if NormalizeJurisdiction(jurisdiction) == JurisdictionDefault {
		return fmt.Sprintf("https://%s.r2.cloudflarestorage.com", accountID)
	}
	return fmt.Sprintf("https://%s.%s.r2.cloudflarestorage.com", accountID, jurisdiction)
}

// S3SecretAccessKey derives the S3 secret access key from an account API
// token's value: the lowercase hex SHA-256 of the value, exactly what the
// Cloudflare dashboard shows as "Secret Access Key" when the same token is
// created there. The matching access key id is the token's id, unchanged.
func S3SecretAccessKey(tokenValue string) string {
	sum := sha256.Sum256([]byte(tokenValue))
	return hex.EncodeToString(sum[:])
}
