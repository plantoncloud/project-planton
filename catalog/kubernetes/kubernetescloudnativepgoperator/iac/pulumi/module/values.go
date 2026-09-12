package module

import (
	"strings"

	"github.com/pkg/errors"
	"sigs.k8s.io/yaml"
)

// buildHelmValues renders the typed spec into the chart's values map, then
// merges the spec's helm_values escape hatch over it with Helm
// `-f` semantics (maps deep-merge with the later document winning, lists
// replace).
//
// PARITY: the Terraform module reaches the same result natively — its
// helm_release passes values = [yamlencode(typed values), helm_values] and
// the provider merges the documents in exactly this order. Keep every typed
// mapping below in lockstep with the Terraform module's locals.
//
// No fullnameOverride: the chart hard-codes the names that matter (the
// webhook service is "cnpg-webhook-service" regardless of release name —
// baked into the webhook certificate) — there is nothing for an override
// to pin.
func buildHelmValues(locals *Locals) (map[string]interface{}, error) {
	spec := locals.Spec

	values := map[string]interface{}{}

	// ---- CRD lifecycle ------------------------------------------------------
	// crds.create matches the chart's own default (true) — rendered only on
	// explicit opt-out (something else manages the CRDs). No keep knob is
	// needed here: the chart stamps `helm.sh/resource-policy: keep` on every
	// CRD UNCONDITIONALLY, so uninstalling the release never cascade-deletes
	// the Cluster resources (and the databases behind them) — the upstream
	// safety posture, kept as-is.
	if spec.GetCrds() != nil && spec.GetCrds().Install != nil && !spec.GetCrds().GetInstall() {
		values["crds"] = map[string]interface{}{
			"create": false,
		}
	}

	// ---- operator sizing ------------------------------------------------------
	if spec.Replicas != nil {
		values["replicaCount"] = int(spec.GetReplicas())
	}
	if r := resourcesMap(spec.GetResources()); r != nil {
		values["resources"] = r
	}

	// ---- operator configuration (watch scope + config.data) --------------------
	// The chart folds three typed concerns into ONE `config` block:
	// clusterWide (RBAC scope), data (the operator's configmap entries),
	// and maxConcurrentReconciles. Rendered together here so the block
	// appears at most once.
	//
	// WATCH_NAMESPACE PRECEDENCE: the typed watch field OWNS the
	// WATCH_NAMESPACE key. A user entry under that key in operator_config
	// is always stripped; the key is rendered ONLY from watch.namespaces
	// (comma-joined) when cluster_wide is false. Spec CEL rules guarantee
	// namespaces are present exactly when cluster_wide is false.
	clusterWide := true
	if spec.GetWatch() != nil && spec.GetWatch().ClusterWide != nil {
		clusterWide = spec.GetWatch().GetClusterWide()
	}

	config := map[string]interface{}{}
	if !clusterWide {
		// config.clusterWide matches the chart's own default (true) —
		// rendered only when fencing the operator into namespaces.
		config["clusterWide"] = false
	}

	configData := map[string]interface{}{}
	for k, v := range spec.GetOperatorConfig() {
		if k == "WATCH_NAMESPACE" {
			// Owned by the typed watch field — see the precedence note.
			continue
		}
		configData[k] = v
	}
	if !clusterWide {
		configData["WATCH_NAMESPACE"] = strings.Join(spec.GetWatch().GetNamespaces(), ",")
	}
	if len(configData) > 0 {
		config["data"] = configData
	}

	if spec.MaxConcurrentReconciles != nil {
		config["maxConcurrentReconciles"] = int(spec.GetMaxConcurrentReconciles())
	}

	if len(config) > 0 {
		values["config"] = config
	}

	// ---- own telemetry ---------------------------------------------------------
	// Both flags default false upstream — rendered only when on. The
	// PodMonitor requires the Prometheus operator CRDs on the cluster; the
	// release FAILS to install without them (atomic rolls it back).
	monitoring := map[string]interface{}{}
	if spec.GetMonitoring().GetPodMonitorEnabled() {
		monitoring["podMonitorEnabled"] = true
	}
	if spec.GetMonitoring().GetGrafanaDashboard() {
		monitoring["grafanaDashboard"] = map[string]interface{}{
			"create": true,
		}
	}
	if len(monitoring) > 0 {
		values["monitoring"] = monitoring
	}

	// ---- scheduling -----------------------------------------------------------
	if spec.GetPriorityClassName() != "" {
		values["priorityClassName"] = spec.GetPriorityClassName()
	}
	if len(spec.GetNodeSelector()) > 0 {
		values["nodeSelector"] = stringMapToInterface(spec.GetNodeSelector())
	}
	if len(spec.GetTolerations()) > 0 {
		values["tolerations"] = tolerationsSlice(spec.GetTolerations())
	}

	// ---- image ----------------------------------------------------------------
	// Pull secrets are name references in the chart's values
	// ([{name: ...}]); the image override renders only the halves that are
	// set — an empty tag keeps the chart's appVersion default.
	if len(spec.GetImagePullSecrets()) > 0 {
		pullSecrets := make([]interface{}, 0, len(spec.GetImagePullSecrets()))
		for _, name := range spec.GetImagePullSecrets() {
			pullSecrets = append(pullSecrets, map[string]interface{}{"name": name})
		}
		values["imagePullSecrets"] = pullSecrets
	}
	image := map[string]interface{}{}
	if spec.GetImage().GetRepository() != "" {
		image["repository"] = spec.GetImage().GetRepository()
	}
	if spec.GetImage().GetTag() != "" {
		image["tag"] = spec.GetImage().GetTag()
	}
	if len(image) > 0 {
		values["image"] = image
	}

	// ---- escape hatch (merged LAST, helm -f semantics) --------------------------
	if spec.GetHelmValues() != "" {
		overrides := map[string]interface{}{}
		if err := yaml.Unmarshal([]byte(spec.GetHelmValues()), &overrides); err != nil {
			return nil, errors.Wrap(err, "failed to parse helm_values as a YAML document")
		}
		values = mergeMaps(values, overrides)
	}

	return values, nil
}
