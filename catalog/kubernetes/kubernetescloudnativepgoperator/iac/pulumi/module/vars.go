package module

var vars = struct {
	// HelmChartRepo is the CloudNativePG project's chart repository (it
	// also serves the Barman Cloud plugin chart, which its own kind
	// installs).
	HelmChartRepo string
	// HelmChartName is the operator chart ("cloudnative-pg").
	HelmChartName string
	// DefaultChartVersion is the fallback when spec.chart_version is unset
	// AND the platform's defaulting middleware did not run. Keep aligned
	// with the spec default and the chart-repo index (chart 0.29.0 ships
	// operator 1.30.0 — chart and app versions move SEPARATELY; the chart
	// pin governs).
	DefaultChartVersion string
	// ReleaseName is FIXED: the operator registers cluster-scoped CRDs and
	// mutating/validating webhooks whose service name is baked into the
	// chart ("cnpg-webhook-service" — it is embedded in the webhook
	// certificate and cannot be configured), so a second installation would
	// fight over both. One operator per cluster is an upstream constraint —
	// the release name never derives from metadata.name.
	ReleaseName string

	// HelmTimeoutSeconds bounds the atomic install/upgrade. 600s covers
	// image pulls on cold clusters; atomic rolls back on expiry so a
	// wedged install never lingers half-deployed.
	HelmTimeoutSeconds int
}{
	// Chart identity — MUST be identical in the Terraform module's locals
	// (cross-engine chart drift installs different software per engine).
	HelmChartRepo:       "https://cloudnative-pg.github.io/charts",
	HelmChartName:       "cloudnative-pg",
	DefaultChartVersion: "0.29.0",
	ReleaseName:         "cnpg",
	HelmTimeoutSeconds:  600,
}
