package r2

import "testing"

// The account id and hosts below follow the examples in Cloudflare's R2
// documentation (the S3 API "Authentication" page).
const exampleAccount = "4793d734c0b8e484dfc37ec392b5fa8a"

func TestS3Endpoint(t *testing.T) {
	cases := []struct {
		jurisdiction string
		want         string
	}{
		{"", "https://4793d734c0b8e484dfc37ec392b5fa8a.r2.cloudflarestorage.com"},
		{"default", "https://4793d734c0b8e484dfc37ec392b5fa8a.r2.cloudflarestorage.com"},
		{"eu", "https://4793d734c0b8e484dfc37ec392b5fa8a.eu.r2.cloudflarestorage.com"},
		{"fedramp", "https://4793d734c0b8e484dfc37ec392b5fa8a.fedramp.r2.cloudflarestorage.com"},
		{"us", "https://4793d734c0b8e484dfc37ec392b5fa8a.us.r2.cloudflarestorage.com"},
	}
	for _, c := range cases {
		if got := S3Endpoint(exampleAccount, c.jurisdiction); got != c.want {
			t.Errorf("S3Endpoint(%q) = %q, want %q", c.jurisdiction, got, c.want)
		}
	}
}

func TestS3SecretAccessKey(t *testing.T) {
	// sha256("abc") -- a fixed vector so a change in the derivation rule is a
	// test failure, not a live authentication failure against R2.
	const want = "ba7816bf8f01cfea414140de5dae2223b00361a396177a9cb410ff61f20015ad"
	if got := S3SecretAccessKey("abc"); got != want {
		t.Errorf("S3SecretAccessKey(abc) = %q, want %q", got, want)
	}
	if len(S3SecretAccessKey("")) != 64 {
		t.Errorf("the derived secret is always 64 lowercase hex characters")
	}
}

func TestJurisdictions(t *testing.T) {
	for _, j := range []string{"", "default", "eu", "fedramp", "us"} {
		if !IsJurisdiction(j) {
			t.Errorf("IsJurisdiction(%q) = false, want true", j)
		}
	}
	for _, j := range []string{"EU", "europe", "fedramp-high", " "} {
		if IsJurisdiction(j) {
			t.Errorf("IsJurisdiction(%q) = true, want false", j)
		}
	}
	if NormalizeJurisdiction("") != JurisdictionDefault || NormalizeJurisdiction("eu") != "eu" {
		t.Errorf("NormalizeJurisdiction maps only the empty string")
	}
}
