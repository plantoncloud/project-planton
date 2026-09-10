package module

import (
	"strconv"

	kubernetescnpgbarmancloudpluginv1alpha1 "github.com/plantonhq/planton/catalog/kubernetes/kubernetescnpgbarmancloudplugin/v1alpha1"
	"github.com/plantonhq/planton/pkg/iac/pulumi/pulumimodule/provider/kubernetes/kuberneteslabelkeys"
	"github.com/plantonhq/planton/shared/cloudresourcekind"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

// Locals holds computed values derived from the stack input for use across
// the module. Every resolution here has an exact twin in the Terraform
// module's locals.tf -- keep them in lockstep.
type Locals struct {
	Spec *kubernetescnpgbarmancloudpluginv1alpha1.KubernetesCnpgBarmanCloudPluginSpec

	// Resource-identity labels stamped on the module-created satellites
	// (the namespace -- never injected into the chart's own resources;
	// Helm owns those).
	Labels map[string]string

	// Namespace the plugin installs into -- the CloudNativePG operator's
	// namespace, by requirement (resolved literal from the spec's
	// value-or-ref; the platform resolves a reference to the operator
	// resource's namespace output before the module runs).
	Namespace string

	// Chart version resolved to the pinned default when unset, so both
	// engines install the same chart whether or not the platform's
	// defaulting middleware ran.
	ChartVersion string
}

// initializeLocals extracts and transforms spec fields into module-local
// values.
func initializeLocals(_ *pulumi.Context, stackInput *kubernetescnpgbarmancloudpluginv1alpha1.KubernetesCnpgBarmanCloudPluginStackInput) *Locals {
	target := stackInput.Target
	spec := target.Spec

	labels := map[string]string{
		kuberneteslabelkeys.Resource:     strconv.FormatBool(true),
		kuberneteslabelkeys.ResourceName: target.Metadata.Name,
		kuberneteslabelkeys.ResourceKind: cloudresourcekind.CloudResourceKind_KubernetesCnpgBarmanCloudPlugin.String(),
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

	return &Locals{
		Spec:         spec,
		Labels:       labels,
		Namespace:    spec.Namespace.GetValue(),
		ChartVersion: chartVersion,
	}
}
