package module

import (
	kuberneteshttproutev1alpha1 "github.com/plantonhq/planton/catalog/kubernetes/kuberneteshttproute/v1alpha1"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

// Locals holds the resolved inputs the module operates on: the full target
// resource plus the scalar identifiers used for the resource name, namespace,
// labels, and stack outputs.
type Locals struct {
	KubernetesHttpRoute *kuberneteshttproutev1alpha1.KubernetesHttpRoute
	RouteName           string
	Namespace           string
	// The first declared hostname, or "" when the route matches every host.
	FirstHost string
	Labels    map[string]string
}

func initializeLocals(_ *pulumi.Context, stackInput *kuberneteshttproutev1alpha1.KubernetesHttpRouteStackInput) *Locals {
	target := stackInput.Target
	metadata := target.Metadata
	spec := target.Spec

	routeName := metadata.Name

	// namespace is a StringValueOrRef foreign key. The platform middleware
	// resolves valueFrom references to literal strings before the IaC module
	// runs, so GetValue() returns the resolved value.
	namespace := spec.GetNamespace().GetValue()

	firstHost := ""
	if hostnames := spec.GetHostnames(); len(hostnames) > 0 {
		firstHost = hostnames[0]
	}

	labels := map[string]string{
		"app.kubernetes.io/name":       "httproute",
		"app.kubernetes.io/instance":   metadata.Name,
		"app.kubernetes.io/managed-by": "planton",
		"app.kubernetes.io/component":  "httproute",
	}

	return &Locals{
		KubernetesHttpRoute: target,
		RouteName:           routeName,
		Namespace:           namespace,
		FirstHost:           firstHost,
		Labels:              labels,
	}
}
