// Package verify provides manifest-driven resource verification for
// Kubernetes E2E tests. Each verifier type checks that a specific class
// of Kubernetes resource (namespace, workload, Helm chart, CRD workload,
// operator) is present and healthy after deployment, or absent after destroy.
package verify

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/pkg/errors"
	"gopkg.in/yaml.v3"
)

// ManifestInfo holds the parsed fields from a manifest needed for verification.
type ManifestInfo struct {
	Kind      string
	Name      string
	Namespace string
}

// ParseManifestInfo extracts kind, name, and namespace from a manifest YAML file
// to drive dynamic verification without hardcoded values.
func ParseManifestInfo(manifestPath string) (*ManifestInfo, error) {
	data, err := os.ReadFile(manifestPath)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to read manifest %s", manifestPath)
	}

	var raw map[string]interface{}
	if err := yaml.Unmarshal(data, &raw); err != nil {
		return nil, errors.Wrapf(err, "failed to parse manifest YAML %s", manifestPath)
	}

	info := &ManifestInfo{}

	if kind, ok := raw["kind"].(string); ok {
		info.Kind = kind
	}

	if metadata, ok := raw["metadata"].(map[string]interface{}); ok {
		if name, ok := metadata["name"].(string); ok {
			info.Name = name
		}
	}

	if spec, ok := raw["spec"].(map[string]interface{}); ok {
		if name, ok := spec["name"].(string); ok {
			info.Name = name
		}

		switch ns := spec["namespace"].(type) {
		case string:
			info.Namespace = ns
		case map[string]interface{}:
			if val, ok := ns["value"].(string); ok {
				info.Namespace = val
			}
		}
	}

	if info.Namespace == "" {
		info.Namespace = "default"
	}

	return info, nil
}

// manifestSpecString reads one scalar spec field (protojson camelCase key)
// from a manifest — for verifiers that need a spec detail beyond the
// kind/name/namespace triple (e.g. a Certificate's target Secret name).
func manifestSpecString(manifestPath, key string) (string, error) {
	data, err := os.ReadFile(manifestPath)
	if err != nil {
		return "", errors.Wrapf(err, "failed to read manifest %s", manifestPath)
	}

	var raw map[string]interface{}
	if err := yaml.Unmarshal(data, &raw); err != nil {
		return "", errors.Wrapf(err, "failed to parse manifest YAML %s", manifestPath)
	}

	spec, ok := raw["spec"].(map[string]interface{})
	if !ok {
		return "", errors.Errorf("manifest %s has no spec block", manifestPath)
	}
	value, ok := spec[key].(string)
	if !ok || value == "" {
		return "", errors.Errorf("manifest %s spec.%s is missing or not a string", manifestPath, key)
	}
	return value, nil
}

// manifestSpecMap reads the manifest's spec block, tolerating every error
// with a nil return — for verifier construction that has usable defaults
// when a detail is absent.
func manifestSpecMap(manifestPath string) map[string]interface{} {
	data, err := os.ReadFile(manifestPath)
	if err != nil {
		return nil
	}
	var raw map[string]interface{}
	if err := yaml.Unmarshal(data, &raw); err != nil {
		return nil
	}
	spec, _ := raw["spec"].(map[string]interface{})
	return spec
}

// manifestNestedSpecString reads spec.<parent>.<key> (protojson camelCase
// keys), returning "" when absent — for optional nested details with
// defaults (e.g. an ingress-nginx instance's class name).
func manifestNestedSpecString(manifestPath, parent, key string) string {
	spec := manifestSpecMap(manifestPath)
	if spec == nil {
		return ""
	}
	nested, ok := spec[parent].(map[string]interface{})
	if !ok {
		return ""
	}
	value, _ := nested[key].(string)
	return value
}

// manifestSpecFirstString reads the first element of a repeated string spec
// field (protojson camelCase key), returning "" when absent or empty — for
// scenarios that key a behavioral probe off a list value (e.g. an
// HTTPRoute's first hostname).
func manifestSpecFirstString(manifestPath, key string) (string, error) {
	spec := manifestSpecMap(manifestPath)
	if spec == nil {
		return "", nil
	}
	list, ok := spec[key].([]interface{})
	if !ok || len(list) == 0 {
		return "", nil
	}
	value, _ := list[0].(string)
	return value, nil
}

// manifestSpecInt reads one integer spec field (protojson camelCase key),
// returning the fallback when absent or not numeric.
func manifestSpecInt(manifestPath, key string, fallback int64) int64 {
	spec := manifestSpecMap(manifestPath)
	if spec == nil {
		return fallback
	}
	switch v := spec[key].(type) {
	case int:
		return int64(v)
	case int64:
		return v
	case float64:
		return int64(v)
	default:
		return fallback
	}
}

// manifestNestedRecoveryOwnerSecret reads spec.bootstrap.recovery.owner_secret_name
// (either field-name convention) from a KubernetesPostgres manifest — "" when
// the manifest declares no recovery or brings no Secret.
func manifestNestedRecoveryOwnerSecret(manifestPath string) string {
	spec := manifestSpecMap(manifestPath)
	if spec == nil {
		return ""
	}
	bootstrap, _ := spec["bootstrap"].(map[string]interface{})
	recovery, _ := bootstrap["recovery"].(map[string]interface{})
	for _, key := range []string{"ownerSecretName", "owner_secret_name"} {
		if value, ok := recovery[key].(string); ok {
			return value
		}
	}
	return ""
}

// manifestDeclaresBarmanPlugin reports whether a KubernetesPostgres manifest
// renders the Barman Cloud plugin into its Cluster: a backup block or an
// object-store recovery (either field-name convention). Both need the
// plugin on the cluster before the Cluster can reconcile.
func manifestDeclaresBarmanPlugin(manifestPath string) bool {
	spec := manifestSpecMap(manifestPath)
	if spec == nil {
		return false
	}
	if _, ok := spec["backup"].(map[string]interface{}); ok {
		return true
	}
	bootstrap, _ := spec["bootstrap"].(map[string]interface{})
	if _, ok := bootstrap["recovery"].(map[string]interface{}); ok {
		return true
	}
	return false
}

// scenarioMatches reports whether the scenario file's name (its basename
// without the extension) starts with prefix and ends with suffix -- the
// store-neutral form of the name-keyed dispatch, so `gke-gcs-recovery` and
// `gke-r2-recovery` switch on the same proof. The runner keeps a scenario's
// basename on the expanded copies it hands to verification, so the match
// holds for those too.
func scenarioMatches(manifestPath, prefix, suffix string) bool {
	name := strings.TrimSuffix(filepath.Base(manifestPath), filepath.Ext(manifestPath))
	return strings.HasPrefix(name, prefix) && strings.HasSuffix(name, suffix)
}

// manifestAnnotation reads one metadata.annotations value, "" when absent or
// unreadable -- for verifiers whose destroy assertions depend on what the
// scenario declared (a deploy that was designed to be refused never created
// what a keep assertion would look for).
func manifestAnnotation(manifestPath, key string) string {
	data, err := os.ReadFile(manifestPath)
	if err != nil {
		return ""
	}
	var raw map[string]interface{}
	if err := yaml.Unmarshal(data, &raw); err != nil {
		return ""
	}
	metadata, _ := raw["metadata"].(map[string]interface{})
	annotations, _ := metadata["annotations"].(map[string]interface{})
	value, _ := annotations[key].(string)
	return value
}

// manifestHasPrerequisite reports whether the manifest's e2e-prerequisites
// annotation names the given kind — how a scenario signals it runs with a
// fixture (e.g. an HPA scenario that installs metrics-server and can
// therefore be verified behaviorally).
func manifestHasPrerequisite(manifestPath, kind string) bool {
	data, err := os.ReadFile(manifestPath)
	if err != nil {
		return false
	}
	var raw map[string]interface{}
	if err := yaml.Unmarshal(data, &raw); err != nil {
		return false
	}
	metadata, ok := raw["metadata"].(map[string]interface{})
	if !ok {
		return false
	}
	annotations, ok := metadata["annotations"].(map[string]interface{})
	if !ok {
		return false
	}
	value, _ := annotations["planton.dev/e2e-prerequisites"].(string)
	for _, entry := range strings.Split(value, ",") {
		if strings.TrimSpace(entry) == kind {
			return true
		}
	}
	return false
}

// manifestHasPrerequisiteSuffix reports whether any e2e-prerequisites entry
// ends with the given suffix — how a verifier recognizes a MANIFEST-PATH
// fixture (e.g. a behavioral scenario's scale-target Deployment) without
// coupling to the repo-relative prefix of the path.
func manifestHasPrerequisiteSuffix(manifestPath, suffix string) bool {
	data, err := os.ReadFile(manifestPath)
	if err != nil {
		return false
	}
	var raw map[string]interface{}
	if err := yaml.Unmarshal(data, &raw); err != nil {
		return false
	}
	metadata, ok := raw["metadata"].(map[string]interface{})
	if !ok {
		return false
	}
	annotations, ok := metadata["annotations"].(map[string]interface{})
	if !ok {
		return false
	}
	value, _ := annotations["planton.dev/e2e-prerequisites"].(string)
	for _, entry := range strings.Split(value, ",") {
		if strings.HasSuffix(strings.TrimSpace(entry), suffix) {
			return true
		}
	}
	return false
}

// specStringList reads a repeated-string spec field, tolerating both the
// snake_case and camelCase manifest key forms (manifests are authored in
// either; protojson emits camelCase).
func specStringList(spec map[string]interface{}, keys ...string) []string {
	for _, key := range keys {
		entries, ok := spec[key].([]interface{})
		if !ok {
			continue
		}
		out := make([]string, 0, len(entries))
		for _, entry := range entries {
			if s, ok := entry.(string); ok {
				out = append(out, s)
			}
		}
		return out
	}
	return nil
}
