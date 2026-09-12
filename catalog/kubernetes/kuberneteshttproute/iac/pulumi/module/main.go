package module

import (
	"github.com/pkg/errors"
	kuberneteshttproutev1alpha1 "github.com/plantonhq/planton/catalog/kubernetes/kuberneteshttproute/v1alpha1"
	"github.com/plantonhq/planton/pkg/iac/pulumi/pulumimodule/provider/kubernetes/pulumikubernetesprovider"
	gatewayv1 "github.com/plantonhq/planton/pkg/kubernetes/kubernetestypes/gatewayapis/kubernetes/gateway/v1"
	"github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes"
	metav1 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/meta/v1"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

func Resources(ctx *pulumi.Context, stackInput *kuberneteshttproutev1alpha1.KubernetesHttpRouteStackInput) error {
	locals := initializeLocals(ctx, stackInput)

	kubeProvider, err := pulumikubernetesprovider.GetWithKubernetesProviderConfig(
		ctx, stackInput.ProviderConfig, "kubernetes")
	if err != nil {
		return errors.Wrap(err, "failed to set up kubernetes provider")
	}

	if err := createHttpRoute(ctx, kubeProvider, locals); err != nil {
		return errors.Wrap(err, "failed to create http route")
	}

	ctx.Export(OpRouteName, pulumi.String(locals.RouteName))
	ctx.Export(OpNamespace, pulumi.String(locals.Namespace))
	ctx.Export(OpFirstHost, pulumi.String(locals.FirstHost))

	return nil
}

// createHttpRoute creates the namespaced Gateway API HTTPRoute using the typed
// crd2pulumi SDK (gatewayv1.NewHTTPRoute), consistent with every other Planton
// ingress component. The typed approach catches field-name and structure errors
// at compile time rather than at deployment time. The upstream HTTPRouteSpec is
// the largest in the family, so its mapping is split across parent_refs.go,
// rules.go, matches.go, filters.go, and backend_refs.go.
func createHttpRoute(
	ctx *pulumi.Context,
	kubeProvider *kubernetes.Provider,
	locals *Locals,
) error {
	spec := locals.KubernetesHttpRoute.Spec

	httpRouteSpec := gatewayv1.HTTPRouteSpecArgs{
		Rules: buildRules(spec.GetRules()),
	}

	if parentRefs := spec.GetParentRefs(); len(parentRefs) > 0 {
		httpRouteSpec.ParentRefs = buildParentRefs(parentRefs)
	}
	if hostnames := spec.GetHostnames(); len(hostnames) > 0 {
		httpRouteSpec.Hostnames = pulumi.ToStringArray(hostnames)
	}

	_, err := gatewayv1.NewHTTPRoute(ctx, locals.RouteName,
		&gatewayv1.HTTPRouteArgs{
			Metadata: metav1.ObjectMetaArgs{
				Name:      pulumi.String(locals.RouteName),
				Namespace: pulumi.String(locals.Namespace),
				Labels:    pulumi.ToStringMap(locals.Labels),
			},
			Spec: httpRouteSpec,
		},
		pulumi.Provider(kubeProvider))

	return err
}
