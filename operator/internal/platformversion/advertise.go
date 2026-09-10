package platformversion

import (
	"context"
	"fmt"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// Annotation is the key under which the operator publishes MinimumSupported
// on the PlantonPlatform definition. The definition is the one object every
// installer already reads before declaring a platform (it waits for it to be
// established), so the floor rides there: a desktop or a script can refuse a
// too-old version in words before a single object exists, instead of
// discovering the refusal on the resource afterwards. Installers that find
// the annotation absent (an operator older than this) fall back to the
// VersionSupported condition, which stays the authority.
const Annotation = "planton.ai/minimum-platform-version"

// PlatformDefinitionName is the PlantonPlatform CustomResourceDefinition's
// metadata.name, the object the floor is advertised on.
const PlatformDefinitionName = "plantonplatforms.planton.ai"

// +kubebuilder:rbac:groups=apiextensions.k8s.io,resources=customresourcedefinitions,verbs=get;patch

// Advertise stamps MinimumSupported onto the PlantonPlatform definition's
// annotations. Idempotent: a merge patch of one annotation, so the operator's
// own definition (kept by the chart) and any other annotation on it are left
// as they are, and every restart re-asserts the floor this build carries --
// an operator upgrade moves the advertised value with it. Advertising is a
// courtesy to installers, not the operator's job: the caller logs a failure
// and keeps serving.
func Advertise(ctx context.Context, c client.Client) error {
	crd := &unstructured.Unstructured{}
	crd.SetGroupVersionKind(schema.GroupVersionKind{Group: "apiextensions.k8s.io", Version: "v1", Kind: "CustomResourceDefinition"})
	if err := c.Get(ctx, types.NamespacedName{Name: PlatformDefinitionName}, crd); err != nil {
		return fmt.Errorf("reading the %s definition: %w", PlatformDefinitionName, err)
	}
	if crd.GetAnnotations()[Annotation] == MinimumSupported {
		return nil
	}
	patch := client.RawPatch(types.MergePatchType, []byte(fmt.Sprintf(
		`{"metadata":{"annotations":{%q:%q}}}`, Annotation, MinimumSupported)))
	if err := c.Patch(ctx, crd, patch); err != nil {
		return fmt.Errorf("advertising the platform version floor on %s: %w", PlatformDefinitionName, err)
	}
	return nil
}
