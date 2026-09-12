package resources

import (
	"bytes"
	"testing"

	"helm.sh/helm/v3/pkg/chart/loader"
)

// The vendored archive and the constant the operator reports must name the
// same chart version -- a bump that moves one without the other must fail
// here, not in a status message that lies about what is installed.
func TestBarmanCloudPluginChart_PinMatchesEmbeddedArchive(t *testing.T) {
	chrt, err := loader.LoadArchive(bytes.NewReader(barmanPluginChartData))
	if err != nil {
		t.Fatalf("loading embedded plugin chart: %v", err)
	}
	if chrt.Metadata.Name != BarmanCloudPluginReleaseName {
		t.Errorf("chart name %q; the release name %q relies on the two being equal so the chart's fullname collapses to it",
			chrt.Metadata.Name, BarmanCloudPluginReleaseName)
	}
	if chrt.Metadata.Version != BarmanCloudPluginChartVersion {
		t.Errorf("embedded chart version %q, constant %q: bump BARMAN_PLUGIN_HELM_CHART_VERSION, run make pull-chart-barman-plugin, and move the constant together",
			chrt.Metadata.Version, BarmanCloudPluginChartVersion)
	}
}

// The ObjectStore definition doubles as the detect-or-install probe, and the
// plugin Deployment must exist under the name the gate waits on, in
// CloudNativePG's namespace -- a chart bump that renames either must fail
// here, not in a forever-Deploying backup.
func TestLoadBarmanCloudPluginManifests_ContainsDetectAndReadinessTargets(t *testing.T) {
	objs, err := LoadBarmanCloudPluginManifests()
	if err != nil {
		t.Fatalf("rendering plugin chart: %v", err)
	}
	if len(objs) == 0 {
		t.Fatal("expected rendered objects, got none")
	}

	const kindCRD, kindDeployment = "CustomResourceDefinition", "Deployment"
	var foundCRD, foundDeployment, foundService bool
	for _, obj := range objs {
		switch {
		case obj.GetKind() == kindCRD && obj.GetName() == BarmanCloudObjectStoreCRDName:
			foundCRD = true
		case obj.GetKind() == kindDeployment && obj.GetName() == BarmanCloudPluginDeploymentName &&
			obj.GetNamespace() == CloudNativePGNamespace:
			foundDeployment = true
		case obj.GetKind() == "Service" && obj.GetName() == BarmanCloudPluginServiceName &&
			obj.GetNamespace() == CloudNativePGNamespace:
			// CloudNativePG discovers plugins by this label on Services in
			// its own namespace; without it the plugin is installed and
			// invisible.
			if obj.GetLabels()["cnpg.io/pluginName"] != BarmanCloudPluginName {
				t.Errorf("plugin Service lacks the cnpg.io/pluginName=%s label CloudNativePG discovers it by; labels: %v",
					BarmanCloudPluginName, obj.GetLabels())
			}
			foundService = true
		}
	}
	if !foundCRD {
		t.Errorf("missing detect CRD %s", BarmanCloudObjectStoreCRDName)
	}
	if !foundDeployment {
		t.Errorf("missing readiness Deployment %s/%s", CloudNativePGNamespace, BarmanCloudPluginDeploymentName)
	}
	if !foundService {
		t.Errorf("missing plugin Service %s/%s", CloudNativePGNamespace, BarmanCloudPluginServiceName)
	}
}

// The release is a sub-operator install: every object is either a
// cluster-scoped kind the operator may hold as shared infrastructure, or lives
// in CloudNativePG's namespace where the janitor sweeps it. Anything else
// would be an object the janitor's release-scoped removal never sees.
func TestLoadBarmanCloudPluginManifests_EveryObjectIsClusterScopedOrInCNPGNamespace(t *testing.T) {
	objs, err := LoadBarmanCloudPluginManifests()
	if err != nil {
		t.Fatalf("rendering plugin chart: %v", err)
	}
	for _, obj := range objs {
		if obj.GetKind() == "" || obj.GetName() == "" {
			t.Errorf("object with empty kind or name: %v", obj.Object)
			continue
		}
		if !IsNamespacedKind(obj.GetKind()) {
			continue
		}
		if obj.GetNamespace() != CloudNativePGNamespace {
			t.Errorf("%s %q rendered into namespace %q; the plugin lives only in %q",
				obj.GetKind(), obj.GetName(), obj.GetNamespace(), CloudNativePGNamespace)
		}
	}
}

// The chart mints the operator-to-plugin TLS pair through cert-manager and
// renders those objects unconditionally. The component preflights cert-manager
// before installing because of exactly this; if a chart bump ever drops the
// dependency, this test is the signal to drop the preflight too.
func TestLoadBarmanCloudPluginManifests_RendersCertManagerObjects(t *testing.T) {
	objs, err := LoadBarmanCloudPluginManifests()
	if err != nil {
		t.Fatalf("rendering plugin chart: %v", err)
	}
	counts := map[string]int{}
	for _, obj := range objs {
		if obj.GetAPIVersion() == "cert-manager.io/v1" {
			counts[obj.GetKind()]++
		}
	}
	if counts["Issuer"] != 1 || counts["Certificate"] != 2 {
		t.Errorf("expected one Issuer and two Certificates from cert-manager.io/v1, got %v", counts)
	}
}
