package ociregistry

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// A fake registry that answers the distribution API the way GHCR does: a
// bearer challenge on the first anonymous read, a token endpoint, an index of
// two platforms plus an attestation, a platform manifest, and a config blob
// carrying labels.
func fakeRegistry(t *testing.T) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	var server *httptest.Server
	tokenIssued := false
	mux.HandleFunc("/token", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("scope") != "repository:plantonhq/planton/control-plane:pull" {
			t.Errorf("unexpected scope %q", r.URL.Query().Get("scope"))
		}
		tokenIssued = true
		_ = json.NewEncoder(w).Encode(map[string]string{"token": "anon-token"})
	})
	requireToken := func(w http.ResponseWriter, r *http.Request) bool {
		if r.Header.Get("Authorization") == "Bearer anon-token" {
			return true
		}
		w.Header().Set("WWW-Authenticate", `Bearer realm="`+server.URL+`/token",service="registry",scope="repository:plantonhq/planton/control-plane:pull"`)
		w.WriteHeader(http.StatusUnauthorized)
		return false
	}
	mux.HandleFunc("/v2/plantonhq/planton/control-plane/manifests/v0.0.70", func(w http.ResponseWriter, r *http.Request) {
		if !requireToken(w, r) {
			return
		}
		if !strings.Contains(r.Header.Get("Accept"), "image.index") {
			t.Errorf("the index media type must be accepted, got %q", r.Header.Get("Accept"))
		}
		w.Header().Set("Content-Type", "application/vnd.oci.image.index.v1+json")
		_, _ = w.Write([]byte(`{"manifests":[
			{"digest":"sha256:att","mediaType":"application/vnd.oci.image.manifest.v1+json","platform":{"os":"unknown","architecture":"unknown"}},
			{"digest":"sha256:amd","mediaType":"application/vnd.oci.image.manifest.v1+json","platform":{"os":"linux","architecture":"amd64"}},
			{"digest":"sha256:arm","mediaType":"application/vnd.oci.image.manifest.v1+json","platform":{"os":"linux","architecture":"arm64"}}]}`))
	})
	mux.HandleFunc("/v2/plantonhq/planton/control-plane/manifests/sha256:amd", func(w http.ResponseWriter, r *http.Request) {
		if !requireToken(w, r) {
			return
		}
		w.Header().Set("Content-Type", "application/vnd.oci.image.manifest.v1+json")
		_, _ = w.Write([]byte(`{"config":{"digest":"sha256:cfg","mediaType":"application/vnd.oci.image.config.v1+json"},"layers":[]}`))
	})
	mux.HandleFunc("/v2/plantonhq/planton/control-plane/blobs/sha256:cfg", func(w http.ResponseWriter, r *http.Request) {
		if !requireToken(w, r) {
			return
		}
		_, _ = w.Write([]byte(`{"architecture":"amd64","os":"linux","config":{"Labels":{"planton.ai/minimum-operator-version":"v0.16.0","org.opencontainers.image.version":"v0.0.70"}}}`))
	})
	mux.HandleFunc("/v2/plantonhq/planton/control-plane/manifests/v0.0.1", func(w http.ResponseWriter, r *http.Request) {
		if !requireToken(w, r) {
			return
		}
		w.WriteHeader(http.StatusNotFound)
	})
	server = httptest.NewServer(mux)
	t.Cleanup(func() {
		server.Close()
		if !tokenIssued {
			t.Error("the anonymous read must have answered the bearer challenge")
		}
	})
	return server
}

func TestImageLabels(t *testing.T) {
	server := fakeRegistry(t)
	host := strings.TrimPrefix(server.URL, "http://")
	client := &Client{HTTP: server.Client()}
	// The client speaks https to real registries; the fake is plain http, so
	// the reference is rewritten through a transport that swaps the scheme.
	client.HTTP.Transport = schemeRewriter{inner: server.Client().Transport}

	labels, err := client.ImageLabels(context.Background(), host+"/plantonhq/planton/control-plane:v0.0.70")
	if err != nil {
		t.Fatal(err)
	}
	if labels["planton.ai/minimum-operator-version"] != "v0.16.0" {
		t.Errorf("labels = %v", labels)
	}

	if _, err := client.ImageLabels(context.Background(), host+"/plantonhq/planton/control-plane:v0.0.1"); err == nil || !strings.Contains(err.Error(), "not found") {
		t.Errorf("an unpublished tag is a not-found error, got %v", err)
	}
}

func TestParseReference(t *testing.T) {
	ref, err := ParseReference("ghcr.io/plantonhq/planton/control-plane:v0.0.70")
	if err != nil || ref.Host != "ghcr.io" || ref.Repository != "plantonhq/planton/control-plane" || ref.Tag != "v0.0.70" {
		t.Errorf("got %+v, %v", ref, err)
	}
	for _, bad := range []string{"control-plane:v1", "ghcr.io/plantonhq/control-plane", "ghcr.io/a:b/c"} {
		if _, err := ParseReference(bad); err == nil {
			t.Errorf("%q must be refused", bad)
		}
	}
}

// schemeRewriter lets the https-only client talk to the plain-http fake.
type schemeRewriter struct{ inner http.RoundTripper }

func (s schemeRewriter) RoundTrip(r *http.Request) (*http.Response, error) {
	r.URL.Scheme = "http"
	return s.inner.RoundTrip(r)
}
