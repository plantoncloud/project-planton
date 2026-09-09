package module

import (
	"strconv"

	kubernetescloudnativepgoperatorv1alpha1 "github.com/plantonhq/planton/catalog/kubernetes/kubernetescloudnativepgoperator/v1alpha1"
	"github.com/plantonhq/planton/pkg/iac/pulumi/pulumimodule/provider/kubernetes/kuberneteslabelkeys"
	"github.com/plantonhq/planton/shared/cloudresourcekind"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

// Locals holds computed values derived from the stack input for use across
// the module. Every resolution here has an exact twin in the Terraform
// module's locals.tf — keep them in lockstep.
type Locals struct {
	Spec *kubernetescloudnativepgoperatorv1alpha1.KubernetesCloudNativePgOperatorSpec

	// Resource-identity labels stamped on the module-created satellites
	// (the namespace — never injected into the charts' own resources;
	// Helm owns those).
	Labels map[string]string

	// Namespace the operator (and the plugin, when enabled) installs into
	// (resolved literal from the spec's value-or-ref; "cnpg-system" is
	// the upstream convention).
	Namespace string

	// Operator chart version resolved to the pinned default when unset,
	// so both engines install the same chart whether or not the
	// platform's defaulting middleware ran.
	ChartVersion string

	// Barman Cloud plugin arm resolved once here so the release wiring in
	// main.go and the outputs agree on whether the plugin release exists.
	// The plugin chart versions INDEPENDENTLY of the operator chart —
	// each release carries its own pin.
	BarmanPluginEnabled      bool
	BarmanPluginChartVersion string

	// InstallOperator is false in the plugin-only posture: a CloudNativePG
	// already runs on the cluster (the Planton operator's, a Helm or GitOps
	// install), so this resource renders no operator release and manages
	// only the plugin beside it. Resolved to the spec's default (true)
	// when unset, so both engines agree whether or not the platform's
	// defaulting middleware ran.
	InstallOperator bool
}

// initializeLocals extracts and transforms spec fields into module-local
// values.
func initializeLocals(_ *pulumi.Context, stackInput *kubernetescloudnativepgoperatorv1alpha1.KubernetesCloudNativePgOperatorStackInput) *Locals {
	target := stackInput.Target
	spec := target.Spec

	labels := map[string]string{
		kuberneteslabelkeys.Resource:     strconv.FormatBool(true),
		kuberneteslabelkeys.ResourceName: target.Metadata.Name,
		kuberneteslabelkeys.ResourceKind: cloudresourcekind.CloudResourceKind_KubernetesCloudNativePgOperator.String(),
	}
	if target.Metadata.Id != "" {
		labels[kuberneteslabelkeys.ResourceId] = target.Metadata.Id
	}
	if target.Metadata.Org != "" {
		labels[kuberneteslabelkeys.Organization] = target.Metadata.Org
	}
	if target.Metadata.Env != "" {
		labels[kuberneteslabelkeys.Environment] = target.Metadata.Env
	}

	chartVersion := spec.GetChartVersion()
	if chartVersion == "" {
		chartVersion = vars.DefaultChartVersion
	}

	pluginEnabled := spec.GetBarmanCloudPlugin().GetEnabled()
	pluginChartVersion := spec.GetBarmanCloudPlugin().GetChartVersion()
	if pluginChartVersion == "" {
		pluginChartVersion = vars.DefaultPluginChartVersion
	}

	installOperator := true
	if spec.InstallOperator != nil {
		installOperator = *spec.InstallOperator
	}

	return &Locals{
		Spec:                     spec,
		Labels:                   labels,
		Namespace:                spec.Namespace.GetValue(),
		ChartVersion:             chartVersion,
		BarmanPluginEnabled:      pluginEnabled,
		BarmanPluginChartVersion: pluginChartVersion,
		InstallOperator:          installOperator,
	}
}
