package module

import (
	"github.com/pkg/errors"
	kubernetescnpgbarmancloudpluginv1alpha1 "github.com/plantonhq/planton/catalog/kubernetes/kubernetescnpgbarmancloudplugin/v1alpha1"
	"github.com/plantonhq/planton/pkg/iac/pulumi/pulumimodule/provider/kubernetes/pulumikubernetesprovider"
	helmv3 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/helm/v3"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

// Resources installs the Barman Cloud plugin for CloudNativePG from the
// official Helm chart as ONE Helm release, "plugin-barman-cloud", in the
// operator's namespace.
//
// WHY ITS OWN RELEASE, ITS OWN KIND: the plugin is a different chart with a
// different pin and a different dependency set than the operator (it needs
// cert-manager; the operator does not), and on many clusters a different
// owner -- a CloudNativePG installed by Helm, GitOps, or a platform
// operator has no plugin, and this kind is how that cluster gets one.
// Upstream forbids folding the plugin into the operator's release (Helm
// ownership of shared resources would conflict), so one kind per release
// is also the honest grain.
//
// WHY THE NAMESPACE IS THE OPERATOR'S: CloudNativePG discovers plugins
// through Services labeled `cnpg.io/pluginName` in its OWN namespace only
// (the operator's plugin predicate rejects any other namespace), and the
// chart fixes that Service's name to "barman-cloud" because it is baked
// into the plugin's TLS certificate. A plugin anywhere else is invisible to
// the operator, and every backup-declaring database stays parked in the
// "unknown plugin being required" phase.
//
// CERT-MANAGER DEPENDENCY (deliberate, documented): the chart renders a
// self-signed Issuer and two Certificates for the operator <-> plugin gRPC
// TLS UNCONDITIONALLY. Without cert-manager on the cluster
// (KubernetesCertManager) the Certificates never become ready and the
// release times out; atomic rolls it back cleanly with a clear message.
//
// ORDERING: the operator (and its CRDs) must exist before the plugin
// registers over CNPG-I. When the operator is declared in the same
// composition, the `namespace` reference onto the operator resource IS the
// ordering edge; against a resident operator the namespace is a literal
// and the operator is already running. The plugin's own registration is
// idempotent -- it re-registers whenever its Service endpoints change.
//
// The typed spec renders into chart values (values.go); the helm_values
// escape hatch merges over them with Helm -f semantics, and the chart's
// fixed identities are re-pinned last -- the exact semantic twin of the
// Terraform module's helm_release with values = [typed, helm_values, pins].
func Resources(ctx *pulumi.Context, stackInput *kubernetescnpgbarmancloudpluginv1alpha1.KubernetesCnpgBarmanCloudPluginStackInput) error {
	locals := initializeLocals(ctx, stackInput)

	kubernetesProvider, err := pulumikubernetesprovider.GetWithKubernetesProviderConfig(ctx,
		stackInput.ProviderConfig, "kubernetes")
	if err != nil {
		return errors.Wrap(err, "failed to create kubernetes provider")
	}

	// ------------------------------ namespace ----------------------------
	createdNamespace, err := namespace(ctx, stackInput, locals, kubernetesProvider)
	if err != nil {
		return errors.Wrap(err, "failed to create namespace")
	}

	var releaseDeps []pulumi.Resource
	if createdNamespace != nil {
		releaseDeps = append(releaseDeps, createdNamespace)
	}

	// ------------------------------ plugin release ------------------------
	mergedValues, err := buildHelmValues(locals)
	if err != nil {
		return errors.Wrap(err, "failed to build helm values")
	}

	_, err = helmv3.NewRelease(ctx, vars.ReleaseName, &helmv3.ReleaseArgs{
		Name:      pulumi.String(vars.ReleaseName),
		Namespace: pulumi.String(locals.Namespace),
		Chart:     pulumi.String(vars.HelmChartName),
		Version:   pulumi.String(locals.ChartVersion),
		RepositoryOpts: &helmv3.RepositoryOptsArgs{
			Repo: pulumi.String(vars.HelmChartRepo),
		},
		Values: pulumi.ToMap(mergedValues),
		// The module owns namespace creation (create_namespace flag).
		CreateNamespace: pulumi.Bool(false),
		// Wait for the plugin to become Available -- a plugin that never
		// becomes ready (cert-manager absent, so its Certificates never
		// issue) should fail THIS deploy with a readiness timeout, not
		// surface later as databases that mysteriously never reconcile.
		Atomic:        pulumi.Bool(true),
		CleanupOnFail: pulumi.Bool(true),
		Timeout:       pulumi.Int(vars.HelmTimeoutSeconds),
	}, append([]pulumi.ResourceOption{
		pulumi.Provider(kubernetesProvider)},
		dependsOn(releaseDeps)...)...)
	if err != nil {
		return errors.Wrap(err, "failed to install plugin-barman-cloud helm release")
	}

	ctx.Export(OpNamespace, pulumi.String(locals.Namespace))
	ctx.Export(OpReleaseName, pulumi.String(vars.ReleaseName))
	// The identifier a Cluster's `plugins` list names -- a chart fact
	// exported so consumers compose against it rather than a string they
	// must already know.
	ctx.Export(OpPluginName, pulumi.String(vars.PluginName))

	return nil
}

// dependsOn wraps a possibly-empty dependency list into resource options
// (an empty DependsOn is a valid no-op).
func dependsOn(deps []pulumi.Resource) []pulumi.ResourceOption {
	if len(deps) == 0 {
		return nil
	}
	return []pulumi.ResourceOption{pulumi.DependsOn(deps)}
}
