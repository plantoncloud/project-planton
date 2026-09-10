package module

import (
	"github.com/pkg/errors"
	kubernetescnpgbarmancloudpluginv1alpha1 "github.com/plantonhq/planton/catalog/kubernetes/kubernetescnpgbarmancloudplugin/v1alpha1"
	kubernetescorev1 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/core/v1"
	kubernetesmeta "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/meta/v1"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

// namespace conditionally creates the installation namespace based on the
// create_namespace flag. Returns the created namespace resource (or nil when
// create_namespace is false -- the namespace must then already exist, which
// is the normal case here: it is the operator's namespace).
// Terraform equivalent: kubernetes_namespace_v1 with count.
func namespace(ctx *pulumi.Context,
	stackInput *kubernetescnpgbarmancloudpluginv1alpha1.KubernetesCnpgBarmanCloudPluginStackInput,
	locals *Locals,
	kubernetesProvider pulumi.ProviderResource,
) (*kubernetescorev1.Namespace, error) {
	if !stackInput.Target.Spec.CreateNamespace {
		return nil, nil
	}

	createdNamespace, err := kubernetescorev1.NewNamespace(ctx,
		locals.Namespace,
		&kubernetescorev1.NamespaceArgs{
			Metadata: kubernetesmeta.ObjectMetaPtrInput(
				&kubernetesmeta.ObjectMetaArgs{
					Name:   pulumi.String(locals.Namespace),
					Labels: pulumi.ToStringMap(locals.Labels),
				}),
		}, pulumi.Provider(kubernetesProvider))
	if err != nil {
		return nil, errors.Wrapf(err, "failed to create %s namespace", locals.Namespace)
	}

	return createdNamespace, nil
}
