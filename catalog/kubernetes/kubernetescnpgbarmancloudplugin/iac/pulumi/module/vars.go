package module

var vars = struct {
	// HelmChartRepo is the CloudNativePG project's chart repository, which
	// serves the plugin chart beside the operator chart.
	HelmChartRepo string
	// HelmChartName is the plugin chart ("plugin-barman-cloud").
	HelmChartName string
	// DefaultChartVersion is the fallback when spec.chart_version is unset
	// AND the platform's defaulting middleware did not run. Keep aligned
	// with the spec default and the chart-repo index (chart 0.7.0 ships
	// plugin v0.13.0 -- chart and app versions move SEPARATELY; the chart
	// pin governs).
	DefaultChartVersion string
	// ReleaseName is FIXED: the chart bakes the plugin's gRPC Service name
	// ("barman-cloud"), its two TLS Secrets, its Certificates and its
	// config ConfigMap into fixed names -- a second release in the same
	// namespace would fight the first over all of them -- and the
	// ObjectStore CRD the chart keeps on uninstall is adopted by a later
	// install ONLY when the release name and namespace match. One plugin
	// per operator namespace is a chart constraint; the release name never
	// derives from metadata.name.
	ReleaseName string
	// PluginName is the CNPG-I identifier the operator discovers the plugin
	// under (the `cnpg.io/pluginName` label the chart stamps on the
	// Service) and the name a Cluster's `plugins` list carries. Exported
	// so consumers compose against a fact, not a string they must know.
	PluginName string
	// ServiceName is the plugin's gRPC Service name -- fixed by the chart
	// (baked into the TLS certificate) and re-pinned after the helm_values
	// merge so the escape hatch can never break the operator handshake.
	ServiceName string
	// HelmTimeoutSeconds bounds the atomic install/upgrade. 600s covers
	// image pulls on cold clusters; atomic rolls back on expiry so a wedged
	// install never lingers half-deployed. This is also where a missing
	// cert-manager surfaces: the chart's Certificates never become ready
	// and the release rolls back with a clear timeout.
	HelmTimeoutSeconds int
}{
	// Chart identity -- MUST be identical in the Terraform module's locals
	// (cross-engine chart drift installs different software per engine).
	HelmChartRepo:       "https://cloudnative-pg.github.io/charts",
	HelmChartName:       "plugin-barman-cloud",
	DefaultChartVersion: "0.7.0",
	ReleaseName:         "plugin-barman-cloud",
	PluginName:          "barman-cloud.cloudnative-pg.io",
	ServiceName:         "barman-cloud",
	HelmTimeoutSeconds:  600,
}
