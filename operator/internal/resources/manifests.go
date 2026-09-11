package resources

import (
	"bufio"
	"bytes"
	"embed"
	"fmt"
	"io"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/util/yaml"
)

//go:embed manifests/cloudnative-pg/release.yaml
var cloudNativePGFS embed.FS

//go:embed manifests/tekton-pipelines/release.yaml
var tektonPipelinesFS embed.FS

//go:embed manifests/openfga-chart/openfga-0.3.13.tgz
var openfgaChartData []byte

//go:embed manifests/temporal/temporal-0.62.0.tgz
var temporalChartData []byte

//go:embed manifests/valkey-chart/valkey-3.0.31.tgz
var valkeyChartData []byte

//go:embed manifests/openbao-chart/openbao-0.25.6.tgz
var openbaoChartData []byte

//go:embed manifests/neo4j-chart/neo4j-2026.1.4.tgz
var neo4jChartData []byte

// The Barman Cloud plugin chart is rendered by LoadBarmanCloudPluginManifests
// (barman_plugin_helm.go) into a sub-operator release, not a platform-owned
// component -- see that file for why it lives beside CloudNativePG.
//
//go:embed manifests/barman-plugin-chart/plugin-barman-cloud-0.7.0.tgz
var barmanPluginChartData []byte

// LoadCloudNativePGManifests parses the embedded CloudNativePG operator
// release manifest (namespace, CRDs, controller deployment, webhook
// configurations) into unstructured Kubernetes objects. The manifest is the
// upstream release YAML, vendored verbatim by `make generate-manifests-cnpg`
// (see Makefile for the pinned version) and embedded into the operator binary
// -- no chart rendering, because CloudNativePG ships plain YAML like Tekton.
// The stock install watches ALL namespaces, which is what lets several
// PlantonPlatform installs on one cluster share the one CloudNativePG.
func LoadCloudNativePGManifests() ([]*unstructured.Unstructured, error) {
	return loadEmbeddedManifests(cloudNativePGFS, []string{
		"manifests/cloudnative-pg/release.yaml",
	})
}

// LoadTektonPipelinesManifests parses the embedded Tekton Pipelines release
// manifest (namespaces, CRDs, controller, webhook, events controller,
// resolvers) into unstructured Kubernetes objects. The manifest is the
// upstream release YAML, vendored verbatim by `make generate-manifests-tekton`
// (see Makefile for the pinned version) and embedded into the operator binary
// -- no chart rendering, because Tekton ships plain YAML, not a Helm chart.
func LoadTektonPipelinesManifests() ([]*unstructured.Unstructured, error) {
	return loadEmbeddedManifests(tektonPipelinesFS, []string{
		"manifests/tekton-pipelines/release.yaml",
	})
}

// loadEmbeddedManifests reads and parses multi-document YAML files from an
// embedded filesystem into unstructured Kubernetes objects.
func loadEmbeddedManifests(fs embed.FS, files []string) ([]*unstructured.Unstructured, error) {
	var all []*unstructured.Unstructured

	for _, name := range files {
		data, err := fs.ReadFile(name)
		if err != nil {
			return nil, fmt.Errorf("reading embedded manifest %s: %w", name, err)
		}
		objs, err := parseMultiDocYAML(data)
		if err != nil {
			return nil, fmt.Errorf("parsing embedded manifest %s: %w", name, err)
		}
		all = append(all, objs...)
	}

	return all, nil
}

// LoadOpenFGAChart returns the raw bytes of the embedded OpenFGA Helm chart
// archive (.tgz). The chart is rendered at runtime using the Helm SDK with
// values computed from the PlantonPlatform CRD.
func LoadOpenFGAChart() []byte { return openfgaChartData }

// LoadTemporalChart returns the raw bytes of the embedded Temporal Helm chart
// archive (.tgz). The chart is rendered at runtime using the Helm SDK with
// values computed from the PlantonPlatform CRD.
func LoadTemporalChart() []byte { return temporalChartData }

// LoadValkeyChart returns the raw bytes of the embedded Bitnami Valkey Helm
// chart archive (.tgz). Valkey serves the platform's redis-protocol cache role
// (standalone architecture) -- see valkey_helm.go for why the engine is Valkey.
func LoadValkeyChart() []byte { return valkeyChartData }

// LoadOpenBAOChart returns the raw bytes of the embedded official OpenBAO Helm
// chart archive (.tgz). Used for secrets management deployments.
func LoadOpenBAOChart() []byte { return openbaoChartData }

// LoadNeo4jChart returns the raw bytes of the embedded official Neo4j Helm
// chart archive (.tgz). Used for graph database deployments.
func LoadNeo4jChart() []byte { return neo4jChartData }

// parseMultiDocYAML splits a multi-document YAML stream into individual
// unstructured Kubernetes objects, skipping empty documents.
func parseMultiDocYAML(data []byte) ([]*unstructured.Unstructured, error) {
	var result []*unstructured.Unstructured

	reader := yaml.NewYAMLReader(bufio.NewReader(bytes.NewReader(data)))
	for {
		doc, err := reader.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("reading YAML document: %w", err)
		}

		doc = bytes.TrimSpace(doc)
		if len(doc) == 0 || string(doc) == "---" {
			continue
		}

		obj := &unstructured.Unstructured{}
		if err := yaml.NewYAMLOrJSONDecoder(bytes.NewReader(doc), len(doc)).Decode(obj); err != nil {
			return nil, fmt.Errorf("decoding YAML document: %w", err)
		}

		if obj.GetKind() == "" {
			continue
		}

		result = append(result, obj)
	}

	return result, nil
}
