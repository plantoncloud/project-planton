package module

import (
	"github.com/pkg/errors"
	"sigs.k8s.io/yaml"
)

// buildHelmValues renders the typed spec into the plugin chart's values
// map, merges the spec's helm_values escape hatch over it with Helm `-f`
// semantics (maps deep-merge with the later document winning, lists
// replace), then RE-PINS the chart's fixed identities so the escape hatch
// can never rename what the operator handshake and the CRD adoption depend
// on.
//
// PARITY: the Terraform module reaches the same result natively -- its
// helm_release passes values = [yamlencode(typed values), helm_values,
// yamlencode(pins)] and the provider merges the documents in exactly this
// order. Keep every typed mapping below in lockstep with the Terraform
// module's locals.
func buildHelmValues(locals *Locals) (map[string]interface{}, error) {
	spec := locals.Spec

	values := map[string]interface{}{}

	// ---- CRD lifecycle ------------------------------------------------------
	// crds.create matches the chart's own default (true) -- rendered only on
	// explicit opt-out (something else manages the ObjectStore CRD). No keep
	// knob is needed: the chart stamps `helm.sh/resource-policy: keep` on the
	// CRD, so uninstalling never deletes the ObjectStore resources databases
	// point at -- the upstream safety posture, kept as-is.
	if spec.GetCrds() != nil && spec.GetCrds().Install != nil && !spec.GetCrds().GetInstall() {
		values["crds"] = map[string]interface{}{
			"create": false,
		}
	}

	// ---- sizing ---------------------------------------------------------------
	if spec.Replicas != nil {
		values["replicaCount"] = int(spec.GetReplicas())
	}
	if r := resourcesMap(spec.GetResources()); r != nil {
		values["resources"] = r
	}

	// ---- images ---------------------------------------------------------------
	// The chart takes BOTH images in the split shape (registry + repository,
	// registry defaulting to ghcr.io); imageMap splits a combined reference
	// on the container-runtime rule. The sidecar image is what the DATABASE
	// pods pull -- the plugin publishes it through its config ConfigMap and
	// injects the container into every instance pod.
	if image := imageMap(spec.GetImage().GetRepository(), spec.GetImage().GetTag()); image != nil {
		values["image"] = image
	}
	if sidecar := imageMap(spec.GetSidecarImage().GetRepository(), spec.GetSidecarImage().GetTag()); sidecar != nil {
		values["sidecarImage"] = sidecar
	}
	// Pull secrets are name references in the chart's values
	// ([{name: ...}]); they cover the plugin pod only -- the sidecar is
	// pulled by the database pods in their own namespaces.
	if len(spec.GetImagePullSecrets()) > 0 {
		pullSecrets := make([]interface{}, 0, len(spec.GetImagePullSecrets()))
		for _, name := range spec.GetImagePullSecrets() {
			pullSecrets = append(pullSecrets, map[string]interface{}{"name": name})
		}
		values["imagePullSecrets"] = pullSecrets
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

	// ---- escape hatch (merged LAST, helm -f semantics) --------------------------
	if spec.GetHelmValues() != "" {
		overrides := map[string]interface{}{}
		if err := yaml.Unmarshal([]byte(spec.GetHelmValues()), &overrides); err != nil {
			return nil, errors.Wrap(err, "failed to parse helm_values as a YAML document")
		}
		values = mergeMaps(values, overrides)
	}

	// ---- fixed identities, re-pinned AFTER the escape hatch ----------------------
	// The Service name is baked into the plugin's TLS certificate (the chart
	// says so itself: "DO NOT CHANGE THE SERVICE NAME"), and the release's
	// derived names are what a later install adopts the kept CRD through
	// and what the E2E verifier and import recipes key on. None of them is
	// configuration; a helm_values document that set them would break the
	// operator handshake silently, so the pins win over the escape hatch.
	values = mergeMaps(values, fixedIdentityPins())

	return values, nil
}

// fixedIdentityPins is the values document that closes the chart's
// name-shaped knobs. Rendered identically by the Terraform module as its
// third values document.
func fixedIdentityPins() map[string]interface{} {
	return map[string]interface{}{
		"nameOverride":      "",
		"fullnameOverride":  "",
		"namespaceOverride": "",
		"service": map[string]interface{}{
			"name": vars.ServiceName,
		},
	}
}
