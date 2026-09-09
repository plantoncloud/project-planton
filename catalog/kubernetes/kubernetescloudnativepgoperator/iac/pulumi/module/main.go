package module

import (
	"github.com/pkg/errors"
	kubernetescloudnativepgoperatorv1alpha1 "github.com/plantonhq/planton/catalog/kubernetes/kubernetescloudnativepgoperator/v1alpha1"
	"github.com/plantonhq/planton/pkg/iac/pulumi/pulumimodule/provider/kubernetes/pulumikubernetesprovider"
	helmv3 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/helm/v3"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

// Resources installs CloudNativePG from the official Helm charts as up to
// TWO real Helm releases in the same namespace:
//
//  1. "cnpg" — the operator chart (cloudnative-pg). The release name is
//     FIXED: the operator registers cluster-scoped CRDs and webhooks whose
//     service name is baked into the chart (and into the webhook
//     certificate) — one installation per cluster is an upstream
//     constraint.
//  2. "plugin-barman-cloud" (when spec.barman_cloud_plugin.enabled) — the
//     Barman Cloud CNPG-I plugin chart, the object-store backup path for
//     every KubernetesPostgres on the cluster. A SEPARATE release in the
//     SAME namespace: upstream forbids folding the plugin into the
//     operator's release (Helm ownership of shared resources would
//     conflict). Installed AFTER the operator so the plugin's CNPG-I
//     registration always lands on a running operator.
//
// PLUGIN-ONLY POSTURE (spec.install_operator false): a CloudNativePG
// already runs on the cluster — the Planton operator installs one for the
// platform's own database, and Helm/GitOps installs are common — and the
// operator's CRDs and webhooks are cluster singletons, so no operator
// release renders at all. Only the plugin release is managed, into the
// declared namespace beside the resident operator, and it registers with
// that operator over CNPG-I exactly as it would with one this module
// installed. The operator-release outputs are then honestly empty.
//
// CERT-MANAGER DEPENDENCY (deliberate, documented): the plugin chart
// renders cert-manager Issuer/Certificate resources UNCONDITIONALLY — its
// operator↔sidecar TLS is issued by cert-manager. Without cert-manager on
// the cluster (KubernetesCertManager) the plugin release fails to install;
// atomic rolls it back cleanly.
//
// The typed spec renders into operator-chart values (values.go); the
// helm_values escape hatch merges last with Helm -f semantics — the exact
// semantic twin of the Terraform module's helm_release with
// values = [typed, helm_values]. helm_values scopes to the OPERATOR chart
// only.
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
	// pluginDeps is what the plugin release waits on: the operator release
	// when this module installs it (the plugin registers with the operator
	// over CNPG-I, so the operator and its CRDs must exist first — uninstall
	// unwinds in reverse for free); only the namespace in the plugin-only
	// posture, where the resident operator is already running.
	pluginDeps := operatorDeps
	operatorReleaseName := ""
	if locals.InstallOperator {
		mergedValues, err := buildHelmValues(locals)
		if err != nil {
			return errors.Wrap(err, "failed to build helm values")
		}

		operatorRelease, err := helmv3.NewRelease(ctx, vars.ReleaseName, &helmv3.ReleaseArgs{
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
		pluginDeps = []pulumi.Resource{operatorRelease}
		operatorReleaseName = vars.ReleaseName
	}

	// ------------------------------ plugin release ------------------------
	barmanPluginReleaseName := ""
	if locals.BarmanPluginEnabled {
		_, err := helmv3.NewRelease(ctx, vars.PluginReleaseName, &helmv3.ReleaseArgs{
			Name:      pulumi.String(vars.PluginReleaseName),
			Namespace: pulumi.String(locals.Namespace),
			Chart:     pulumi.String(vars.PluginChartName),
			Version:   pulumi.String(locals.BarmanPluginChartVersion),
			RepositoryOpts: &helmv3.RepositoryOptsArgs{
				Repo: pulumi.String(vars.HelmChartRepo),
			},
			Values: pulumi.ToMap(buildPluginHelmValues(locals)),
			// The module owns namespace creation (create_namespace flag).
			CreateNamespace: pulumi.Bool(false),
			// Same atomic/wait posture as the operator release. This is
			// also where the cert-manager dependency surfaces: without
			// cert-manager the plugin's Certificate resources never become
			// ready and the release rolls back with a clear timeout.
			Atomic:        pulumi.Bool(true),
			CleanupOnFail: pulumi.Bool(true),
			Timeout:       pulumi.Int(vars.HelmTimeoutSeconds),
		}, append([]pulumi.ResourceOption{
			pulumi.Provider(kubernetesProvider)},
			dependsOn(pluginDeps)...)...)
		if err != nil {
			return errors.Wrap(err, "failed to install plugin-barman-cloud helm release")
		}
		barmanPluginReleaseName = vars.PluginReleaseName
	}

	ctx.Export(OpNamespace, pulumi.String(locals.Namespace))
	// Empty in the plugin-only posture — the operator release is someone
	// else's; this resource never claims a handle it does not own.
	ctx.Export(OpReleaseName, pulumi.String(operatorReleaseName))
	// Empty when the plugin arm is off — KubernetesPostgres backup blocks
	// key off this handle to know whether object-store backups can work.
	ctx.Export(OpBarmanPluginReleaseName, pulumi.String(barmanPluginReleaseName))

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
