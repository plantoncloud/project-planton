package module

import (
	"github.com/pkg/errors"
	kubernetescloudnativepgoperatorv1alpha1 "github.com/plantonhq/planton/catalog/kubernetes/kubernetescloudnativepgoperator/v1alpha1"
	"github.com/plantonhq/planton/pkg/iac/pulumi/pulumimodule/provider/kubernetes/pulumikubernetesprovider"
	helmv3 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/helm/v3"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

// Resources installs CloudNativePG from the official Helm chart as ONE Helm
// release, "cnpg". The release name is FIXED: the operator registers
// cluster-scoped CRDs and webhooks whose service name is baked into the
// chart (and into the webhook certificate) — one installation per cluster
// is an upstream constraint.
//
// BACKUPS LIVE IN A SIBLING KIND: object-store backups run through the
// Barman Cloud CNPG-I plugin, which is a separate chart with its own pin
// and its own cert-manager dependency, installed into THIS release's
// namespace by KubernetesCnpgBarmanCloudPlugin. Upstream forbids folding
// the plugin into the operator's release (Helm ownership of shared
// resources would conflict), so this module never renders it — one kind
// per release.
//
// The typed spec renders into chart values (values.go); the helm_values
// escape hatch merges last with Helm -f semantics — the exact semantic
// twin of the Terraform module's helm_release with values = [typed,
// helm_values].
func Resources(ctx *pulumi.Context, stackInput *kubernetescloudnativepgoperatorv1alpha1.KubernetesCloudNativePgOperatorStackInput) error {
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

	var operatorDeps []pulumi.Resource
	if createdNamespace != nil {
		operatorDeps = append(operatorDeps, createdNamespace)
	}

	// ------------------------------ operator release ----------------------
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
		// Wait for the operator to become Available — an operator that
		// never becomes ready (a PodMonitor rendered without the
		// Prometheus operator CRDs is THE classic install failure) should
		// fail THIS deploy with a readiness timeout, not surface later as
		// Cluster resources that mysteriously never reconcile.
		Atomic:        pulumi.Bool(true),
		CleanupOnFail: pulumi.Bool(true),
		Timeout:       pulumi.Int(vars.HelmTimeoutSeconds),
	}, append([]pulumi.ResourceOption{
		pulumi.Provider(kubernetesProvider)},
		dependsOn(operatorDeps)...)...)
	if err != nil {
		return errors.Wrap(err, "failed to install cloudnative-pg helm release")
	}

	ctx.Export(OpNamespace, pulumi.String(locals.Namespace))
	ctx.Export(OpReleaseName, pulumi.String(vars.ReleaseName))

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
