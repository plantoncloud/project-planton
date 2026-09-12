package module

import (
	"strings"

	kubernetesprovider "github.com/plantonhq/planton/catalog/kubernetes"
)

// resourcesMap renders the shared ContainerResources message into the
// chart's resources shape (the standard Kubernetes limits/requests layout).
// Returns nil when nothing is set.
func resourcesMap(r *kubernetesprovider.ContainerResources) map[string]interface{} {
	if r == nil {
		return nil
	}
	out := map[string]interface{}{}
	if l := r.GetLimits(); l != nil && (l.GetCpu() != "" || l.GetMemory() != "") {
		limits := map[string]interface{}{}
		if l.GetCpu() != "" {
			limits["cpu"] = l.GetCpu()
		}
		if l.GetMemory() != "" {
			limits["memory"] = l.GetMemory()
		}
		out["limits"] = limits
	}
	if q := r.GetRequests(); q != nil && (q.GetCpu() != "" || q.GetMemory() != "") {
		requests := map[string]interface{}{}
		if q.GetCpu() != "" {
			requests["cpu"] = q.GetCpu()
		}
		if q.GetMemory() != "" {
			requests["memory"] = q.GetMemory()
		}
		out["requests"] = requests
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// tolerationsSlice renders the shared WorkloadToleration list into the
// chart's tolerations shape.
func tolerationsSlice(tolerations []*kubernetesprovider.WorkloadToleration) []interface{} {
	out := make([]interface{}, 0, len(tolerations))
	for _, t := range tolerations {
		tol := map[string]interface{}{}
		if t.GetKey() != "" {
			tol["key"] = t.GetKey()
		}
		if t.GetOperator() != "" {
			tol["operator"] = t.GetOperator()
		}
		if t.GetValue() != "" {
			tol["value"] = t.GetValue()
		}
		if t.GetEffect() != "" {
			tol["effect"] = t.GetEffect()
		}
		if t.TolerationSeconds != nil {
			tol["tolerationSeconds"] = t.GetTolerationSeconds()
		}
		out = append(out, tol)
	}
	return out
}

// splitImageRepository splits a combined image reference
// ("registry/path/name") into the chart's SPLIT shape (`image.registry` +
// `image.repository`) on the container-runtime rule: the first path segment
// is a registry only when it carries a dot or a colon or is "localhost";
// otherwise the whole value is the repository and the chart's default
// registry stays in force. Mapping a combined reference verbatim onto a
// split-shaped chart renders "<default-registry>/<mirror>/..." -- an
// ImagePullBackOff identical on both engines that no parity review can
// see; only the chart's pod template shows the shape.
func splitImageRepository(combined string) (registry, repository string) {
	first, rest, found := strings.Cut(combined, "/")
	if !found {
		return "", combined
	}
	if strings.ContainsAny(first, ".:") || first == "localhost" {
		return first, rest
	}
	return "", combined
}

// imageMap renders a typed image override into the chart's split image
// block. Only the halves that are set render -- an empty tag keeps the
// chart's appVersion default, an unset registry keeps the chart's default
// registry (ghcr.io for both plugin images at the pin).
func imageMap(repository, tag string) map[string]interface{} {
	image := map[string]interface{}{}
	if repository != "" {
		registry, repo := splitImageRepository(repository)
		if registry != "" {
			image["registry"] = registry
		}
		image["repository"] = repo
	}
	if tag != "" {
		image["tag"] = tag
	}
	if len(image) == 0 {
		return nil
	}
	return image
}

// mergeMaps deep-merges b over a with Helm's `-f` semantics: nested maps
// merge recursively with b winning per key; everything else (scalars,
// lists) is replaced by b's value.
func mergeMaps(a, b map[string]interface{}) map[string]interface{} {
	out := make(map[string]interface{}, len(a))
	for k, v := range a {
		out[k] = v
	}
	for k, v := range b {
		if bChild, ok := v.(map[string]interface{}); ok {
			if aChild, ok := out[k].(map[string]interface{}); ok {
				out[k] = mergeMaps(aChild, bChild)
				continue
			}
		}
		out[k] = v
	}
	return out
}

// stringMapToInterface converts a map[string]string into the
// map[string]interface{} YAML rendering expects.
func stringMapToInterface(in map[string]string) map[string]interface{} {
	out := make(map[string]interface{}, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}
