// Package ociregistry reads one thing from an OCI registry without a
// registry client dependency: the labels on an image's config. The operator
// uses it to learn what a platform release declares about the operator it
// needs (a label the platform's release stamps on its control-plane image),
// before it renders anything for that release.
//
// Standard library only, on purpose: the operator's module must stay small
// and its Docker build context is the operator directory alone. This is a
// deliberate twin of the installer library's registry reader in the platform
// repository -- the two read the same registry the same way and must keep
// agreeing; unification rides the shared boot-contract extraction.
//
// Every read is anonymous (the platform's images are public) and follows the
// distribution API's bearer challenge: a 401 with WWW-Authenticate names the
// token endpoint, the token is fetched with the scope the challenge states,
// and the request is retried once with it.
package ociregistry

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// Reference is a parsed image reference: registry host, repository path, and
// tag. Digests are not needed here and are not parsed.
type Reference struct {
	Host       string
	Repository string
	Tag        string
}

// ParseReference splits "ghcr.io/org/name:tag" into its parts. A reference
// without a registry host or without a tag is refused: the operator always
// reads a fully named release.
func ParseReference(ref string) (Reference, error) {
	host, rest, ok := strings.Cut(ref, "/")
	if !ok {
		return Reference{}, fmt.Errorf("image reference %q has no registry host", ref)
	}
	colon := strings.LastIndex(rest, ":")
	if colon < 0 || strings.Contains(rest[colon:], "/") {
		return Reference{}, fmt.Errorf("image reference %q has no tag", ref)
	}
	return Reference{Host: host, Repository: rest[:colon], Tag: rest[colon+1:]}, nil
}

// Client reads from registries anonymously.
type Client struct {
	HTTP *http.Client
}

// NewClient returns a client with a bounded timeout: a registry read is a
// best-effort fact for the operator, never something a reconcile waits on
// for long.
func NewClient() *Client {
	return &Client{HTTP: &http.Client{Timeout: 15 * time.Second}}
}

const acceptManifests = "application/vnd.oci.image.index.v1+json, " +
	"application/vnd.docker.distribution.manifest.list.v2+json, " +
	"application/vnd.oci.image.manifest.v1+json, " +
	"application/vnd.docker.distribution.manifest.v2+json"

// ImageLabels returns the labels on the image's config. For a multi-platform
// image (an index) the first platform manifest is read: labels are set at
// build time and are the same on every platform of one release.
func (c *Client) ImageLabels(ctx context.Context, ref string) (map[string]string, error) {
	parsed, err := ParseReference(ref)
	if err != nil {
		return nil, err
	}
	base := fmt.Sprintf("https://%s/v2/%s", parsed.Host, parsed.Repository)
	token := ""

	manifest, mediaType, err := c.get(ctx, base+"/manifests/"+parsed.Tag, acceptManifests, &token)
	if err != nil {
		return nil, fmt.Errorf("reading manifest of %s: %w", ref, err)
	}
	if strings.Contains(mediaType, "index") || strings.Contains(mediaType, "manifest.list") {
		var index struct {
			Manifests []struct {
				Digest    string `json:"digest"`
				MediaType string `json:"mediaType"`
				Platform  *struct {
					OS string `json:"os"`
				} `json:"platform"`
			} `json:"manifests"`
		}
		if err := json.Unmarshal(manifest, &index); err != nil {
			return nil, fmt.Errorf("decoding index of %s: %w", ref, err)
		}
		digest := ""
		for _, m := range index.Manifests {
			// Attestation manifests ride indexes as platform "unknown"; skip
			// them for a real platform's manifest.
			if m.Platform != nil && m.Platform.OS != "" && m.Platform.OS != "unknown" {
				digest = m.Digest
				break
			}
		}
		if digest == "" {
			return nil, fmt.Errorf("index of %s lists no platform manifest", ref)
		}
		manifest, _, err = c.get(ctx, base+"/manifests/"+digest, acceptManifests, &token)
		if err != nil {
			return nil, fmt.Errorf("reading platform manifest of %s: %w", ref, err)
		}
	}

	var m struct {
		Config struct {
			Digest string `json:"digest"`
		} `json:"config"`
	}
	if err := json.Unmarshal(manifest, &m); err != nil {
		return nil, fmt.Errorf("decoding manifest of %s: %w", ref, err)
	}
	if m.Config.Digest == "" {
		return nil, fmt.Errorf("manifest of %s names no config", ref)
	}
	configBlob, _, err := c.get(ctx, base+"/blobs/"+m.Config.Digest, "application/octet-stream, */*", &token)
	if err != nil {
		return nil, fmt.Errorf("reading config of %s: %w", ref, err)
	}
	var config struct {
		Config struct {
			Labels map[string]string `json:"Labels"`
		} `json:"config"`
	}
	if err := json.Unmarshal(configBlob, &config); err != nil {
		return nil, fmt.Errorf("decoding config of %s: %w", ref, err)
	}
	return config.Config.Labels, nil
}

// get performs one registry GET, answering a bearer challenge once. The token
// is kept across calls on the same repository so the challenge is answered
// one time per read.
func (c *Client) get(ctx context.Context, url, accept string, token *string) ([]byte, string, error) {
	for attempt := range 2 {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
		if err != nil {
			return nil, "", err
		}
		req.Header.Set("Accept", accept)
		if *token != "" {
			req.Header.Set("Authorization", "Bearer "+*token)
		}
		resp, err := c.HTTP.Do(req)
		if err != nil {
			return nil, "", err
		}
		body, readErr := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
		_ = resp.Body.Close()
		if readErr != nil {
			return nil, "", readErr
		}
		switch {
		case resp.StatusCode == http.StatusOK:
			// A redirect to blob storage has already been followed by the
			// HTTP client; the content type then is the storage's, which is
			// why callers pass what they need and we return what we saw.
			return body, resp.Header.Get("Content-Type"), nil
		case resp.StatusCode == http.StatusUnauthorized && attempt == 0 && *token == "":
			t, err := c.token(ctx, resp.Header.Get("WWW-Authenticate"))
			if err != nil {
				return nil, "", err
			}
			*token = t
			continue
		case resp.StatusCode == http.StatusNotFound:
			return nil, "", fmt.Errorf("%s: not found", url)
		default:
			return nil, "", fmt.Errorf("%s: HTTP %d", url, resp.StatusCode)
		}
	}
	return nil, "", errors.New("registry refused an anonymous read twice")
}

// token answers a bearer challenge: WWW-Authenticate: Bearer
// realm="https://ghcr.io/token",service="ghcr.io",scope="repository:x:pull".
func (c *Client) token(ctx context.Context, challenge string) (string, error) {
	params := parseChallenge(challenge)
	realm := params["realm"]
	if realm == "" {
		return "", fmt.Errorf("registry challenge names no token realm: %q", challenge)
	}
	url := realm + "?service=" + params["service"] + "&scope=" + params["scope"]
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", err
	}
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return "", err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("token endpoint answered HTTP %d", resp.StatusCode)
	}
	var body struct {
		Token       string `json:"token"`
		AccessToken string `json:"access_token"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return "", fmt.Errorf("decoding token: %w", err)
	}
	if body.Token != "" {
		return body.Token, nil
	}
	return body.AccessToken, nil
}

// parseChallenge reads the key="value" pairs of a Bearer challenge.
func parseChallenge(header string) map[string]string {
	params := map[string]string{}
	header = strings.TrimPrefix(strings.TrimSpace(header), "Bearer ")
	for part := range strings.SplitSeq(header, ",") {
		kv := strings.SplitN(strings.TrimSpace(part), "=", 2)
		if len(kv) != 2 {
			continue
		}
		params[kv[0]] = strings.Trim(kv[1], `"`)
	}
	return params
}
