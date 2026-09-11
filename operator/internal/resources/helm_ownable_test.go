package resources

import "testing"

// Every chart the operator renders on the platform's behalf must be OWNABLE by
// the platform: only namespaced objects, all in the release namespace. The
// platform-owned apply refuses anything else, so a chart that started rendering
// a ClusterRoleBinding (an upstream default flipping, a values key drifting)
// would stop the platform from booting -- and this test is where that shows up
// first, on the operator's own values, before any cluster sees it.
//
// The Barman Cloud plugin chart is deliberately NOT in this list: it is a
// shared sub-operator release (cluster RBAC, CloudNativePG's namespace), applied
// by the sub-operator gate and removed by the janitor, never owned by a
// platform. Its own contract is held by TestLoadBarmanCloudPluginManifests_*.
func TestEveryShippedChartRendersOnlyOwnableObjects(t *testing.T) {
	const crName, namespace = "planton", "planton"
	charts := []struct {
		name   string
		data   []byte
		values map[string]any
	}{
		{"valkey", LoadValkeyChart(), ValkeyHelmValues(crName, "1Gi", "")},
		{"temporal", LoadTemporalChart(), TemporalHelmValues(crName, namespace)},
		{"openbao", LoadOpenBAOChart(), OpenBAOHelmValues(crName, "2Gi", "")},
		{"neo4j", LoadNeo4jChart(), Neo4jHelmValues(crName, "10Gi", "")},
		{"openfga", LoadOpenFGAChart(), OpenFGAHelmValues(crName, namespace)},
	}
	for _, chart := range charts {
		t.Run(chart.name, func(t *testing.T) {
			objs, err := RenderHelmChart(chart.data, crName+"-"+chart.name, namespace, chart.values)
			if err != nil {
				t.Fatalf("rendering %s: %v", chart.name, err)
			}
			if len(objs) == 0 {
				t.Fatalf("%s rendered nothing -- the values disabled the whole chart", chart.name)
			}
			for _, obj := range objs {
				if !IsNamespacedKind(obj.GetKind()) {
					t.Errorf("%s renders cluster-scoped %s %q: the platform cannot own it and would refuse to apply the chart", chart.name, obj.GetKind(), obj.GetName())
				}
				if obj.GetNamespace() != namespace {
					t.Errorf("%s renders %s %q into namespace %q, outside the platform's %q", chart.name, obj.GetKind(), obj.GetName(), obj.GetNamespace(), namespace)
				}
			}
		})
	}
}
