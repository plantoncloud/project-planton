package module

var vars = struct {
	// HelmOciRepo is the OCI registry path holding the planton-operator
	// chart. Pulumi's helm.v3.Release does not resolve oci:// through
	// RepositoryOpts the way the Terraform provider does — the chart
	// reference must be the JOINED "<repo>/<chart>" string (see main.go).
	HelmOciRepo string
	// HelmChartName is the operator chart ("planton-operator").
	HelmChartName string
	// DefaultChartVersion is the chart this catalog release was validated
	// against — the version installed when spec.chart_version is unset.
	// Mirrors the proto field's default; the two move together.
	DefaultChartVersion string
	// MinChartVersion is the oldest chart whose definitions are release
	// resources behind the crds.enabled / crds.keep values this module
	// renders. Older charts install their definitions once from Helm's
	// crds/ directory and have no such values, so the spec's two crds dials
	// would be silently dropped — the one outcome a module must never
	// produce. Refused at plan time on both engines instead; the Terraform
	// module's lifecycle precondition is the twin.
	MinChartVersion string
	// ReleaseName is FIXED: the operator enforces one installation per
	// cluster itself at startup (a label-matched Deployment scan that
	// refuses to start beside a sibling), so the release name never
	// derives from metadata.name — a second differently-named release
	// would only produce a crash-looping manager, never a second
	// operator. Kept definitions carry this release identity in their
	// Helm ownership metadata, so every later install of the operator on
	// the cluster adopts them.
	ReleaseName string
	// HelmTimeoutSeconds bounds the atomic install/upgrade. 600s covers
	// image pulls on cold clusters; atomic rolls back on expiry so a
	// wedged install never lingers half-deployed.
	HelmTimeoutSeconds int
}{
	// Chart identity — MUST be identical in the Terraform module's locals
	// (cross-engine chart drift installs different software per engine).
	HelmOciRepo:         "oci://ghcr.io/plantonhq/charts",
	HelmChartName:       "planton-operator",
	DefaultChartVersion: "0.13.0",
	MinChartVersion:     "0.8.0",
	ReleaseName:         "planton-operator",
	HelmTimeoutSeconds:  600,
}
