// Package chart holds the render-time contract of helm/planton-operator, the
// chart that packages this operator: the chart owns its CRDs as release
// resources, and what it renders is exactly what controller-gen derived from
// the Go types. Rendering goes through the Helm SDK client-side, so this runs
// wherever `go test` runs and needs no cluster and no helm binary.
package chart

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"helm.sh/helm/v3/pkg/action"
	"helm.sh/helm/v3/pkg/chart"
	"helm.sh/helm/v3/pkg/chart/loader"
	"helm.sh/helm/v3/pkg/chartutil"
	"sigs.k8s.io/yaml"
)

const (
	chartDir   = "../../../helm/planton-operator"
	crdBaseDir = "../../config/crd/bases"

	keepAnnotationLine = "    helm.sh/resource-policy: keep\n"

	// devPlaceholder is what a checkout's Chart.yaml carries for version and
	// appVersion; the release lane replaces both from the operator's tag.
	devPlaceholder = "0.0.0-dev"
)

var crdNames = []string{"plantonplatforms.planton.ai", "plantonidentityproviders.planton.ai"}

func loadChart(t *testing.T) *chart.Chart {
	t.Helper()
	chrt, err := loader.Load(chartDir)
	if err != nil {
		t.Fatalf("loading chart: %v", err)
	}
	return chrt
}

// render returns the chart's manifest as Helm would install it, client-side.
func render(t *testing.T, chrt *chart.Chart, values map[string]any) string {
	t.Helper()
	install := action.NewInstall(new(action.Configuration))
	install.DryRun = true
	install.ClientOnly = true
	install.ReleaseName = "planton-operator"
	install.Namespace = "planton"
	// Client-side renders default to an old Kubernetes version that the chart's
	// kubeVersion constraint rejects; render as the oldest version it supports.
	kubeVersion, err := chartutil.ParseKubeVersion("v1.24.0")
	if err != nil {
		t.Fatal(err)
	}
	install.KubeVersion = kubeVersion
	rel, err := install.Run(chrt, values)
	if err != nil {
		t.Fatalf("rendering chart: %v", err)
	}
	return rel.Manifest
}

type document struct {
	source string
	body   string
	object map[string]any
}

// documents splits a rendered manifest into its objects, keeping each
// document's source template and its exact text.
func documents(t *testing.T, manifest string) []document {
	t.Helper()
	var docs []document
	for raw := range strings.SplitSeq(manifest, "\n---\n") {
		raw = strings.TrimSpace(raw)
		raw = strings.TrimPrefix(raw, "---\n")
		if raw == "" {
			continue
		}
		var source string
		var body []string
		for line := range strings.SplitSeq(raw, "\n") {
			if after, ok := strings.CutPrefix(line, "# Source: "); ok {
				source = after
				continue
			}
			body = append(body, line)
		}
		text := strings.Join(body, "\n") + "\n"
		var obj map[string]any
		if err := yaml.Unmarshal([]byte(text), &obj); err != nil {
			t.Fatalf("parsing rendered document from %s: %v", source, err)
		}
		if len(obj) == 0 {
			continue
		}
		docs = append(docs, document{source: source, body: text, object: obj})
	}
	return docs
}

func crds(docs []document) map[string]document {
	out := map[string]document{}
	for _, d := range docs {
		if d.object["kind"] == "CustomResourceDefinition" {
			meta := d.object["metadata"].(map[string]any)
			out[meta["name"].(string)] = d
		}
	}
	return out
}

func annotation(d document, key string) (string, bool) {
	meta := d.object["metadata"].(map[string]any)
	annotations, _ := meta["annotations"].(map[string]any)
	v, ok := annotations[key].(string)
	return v, ok
}

// The chart's version and appVersion are stamped from the operator's release
// tag when it is packaged; a checkout carries a development placeholder. A
// hand-typed version here would silently re-create the old "edit the line to
// release" path and let the chart and the operator drift apart.
func TestChartVersionsAreStampedAtRelease(t *testing.T) {
	meta := loadChart(t).Metadata
	if meta.Version != devPlaceholder {
		t.Fatalf("Chart.yaml version must be the development placeholder %q (the release tag stamps the real one), got %q", devPlaceholder, meta.Version)
	}
	if meta.AppVersion != devPlaceholder {
		t.Fatalf("Chart.yaml appVersion must be the development placeholder %q (the release tag stamps the real one), got %q", devPlaceholder, meta.AppVersion)
	}
}

// Helm's crds/ directory installs once, outside the release, and never
// upgrades. If it ever came back, its copies would be installed first and the
// templated copies would then fail on ownership, so the chart must have none.
func TestChartHasNoInstallOnceCRDDirectory(t *testing.T) {
	if got := loadChart(t).CRDObjects(); len(got) != 0 {
		t.Fatalf("the chart carries %d file(s) under crds/; its CRDs must live under templates/crds/ so the release owns them", len(got))
	}
}

func TestDefaultRenderOwnsBothCRDsAndKeepsThem(t *testing.T) {
	got := crds(documents(t, render(t, loadChart(t), nil)))
	if len(got) != len(crdNames) {
		t.Fatalf("expected %d CRDs, rendered %d: %v", len(crdNames), len(got), keys(got))
	}
	for _, name := range crdNames {
		d, ok := got[name]
		if !ok {
			t.Fatalf("CRD %s not rendered", name)
		}
		if v, ok := annotation(d, "helm.sh/resource-policy"); !ok || v != "keep" {
			t.Errorf("CRD %s must carry helm.sh/resource-policy: keep by default, got %q", name, v)
		}
	}
}

func TestKeepFalseRendersCRDsWithoutTheKeepPolicy(t *testing.T) {
	got := crds(documents(t, render(t, loadChart(t), map[string]any{"crds": map[string]any{"keep": false}})))
	if len(got) != len(crdNames) {
		t.Fatalf("expected %d CRDs, rendered %d", len(crdNames), len(got))
	}
	for name, d := range got {
		if _, ok := annotation(d, "helm.sh/resource-policy"); ok {
			t.Errorf("CRD %s must render without a resource policy when crds.keep=false", name)
		}
	}
}

func TestEnabledFalseRendersNoCRDsAndNoPreflight(t *testing.T) {
	manifest := render(t, loadChart(t), map[string]any{"crds": map[string]any{"enabled": false}})
	if got := crds(documents(t, manifest)); len(got) != 0 {
		t.Fatalf("crds.enabled=false must render no CRDs, rendered %v", keys(got))
	}
	if strings.Contains(manifest, "CustomResourceDefinition") {
		t.Fatal("no CustomResourceDefinition text may remain when crds.enabled=false")
	}
}

// The generator's promise: a rendered definition with the keep line removed is
// controller-gen's file, byte for byte.
func TestRenderedCRDsAreControllerGenOutputVerbatim(t *testing.T) {
	got := crds(documents(t, render(t, loadChart(t), nil)))
	for name, d := range got {
		file := filepath.Join(crdBaseDir, filepath.Base(d.source))
		want, err := os.ReadFile(file)
		if err != nil {
			t.Fatalf("CRD %s rendered from %s has no controller-gen source at %s: %v", name, d.source, file, err)
		}
		rendered := strings.Replace(d.body, keepAnnotationLine, "", 1)
		expected := strings.TrimPrefix(string(want), "---\n")
		if strings.TrimSpace(rendered) != strings.TrimSpace(expected) {
			t.Errorf("CRD %s differs from controller-gen's %s; run `make -C operator manifests`", name, file)
		}
	}
}

func keys(m map[string]document) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}
