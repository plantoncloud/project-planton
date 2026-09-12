package component

import (
	"context"
	"errors"
	"testing"

	batchv1 "k8s.io/api/batch/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/validation/field"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"
)

// A chart upgrade that changes a migration Job's pod template (a new image)
// cannot be patched onto the completed run: the API server refuses the
// template as immutable. ApplyManifests answers the way Helm's
// before-hook-creation policy does -- delete the old run, apply the new one --
// and only for that refusal. This pins both halves with the API server's
// refusal injected at the client seam.

func immutableTemplateError(name string) error {
	return apierrors.NewInvalid(
		schema.GroupKind{Group: "batch", Kind: "Job"}, name,
		field.ErrorList{field.Invalid(field.NewPath("spec", "template"), "...", "field is immutable")},
	)
}

func renderedJob(name, image string) *unstructured.Unstructured {
	obj := &unstructured.Unstructured{}
	obj.SetAPIVersion("batch/v1")
	obj.SetKind("Job")
	obj.SetNamespace("planton")
	obj.SetName(name)
	_ = unstructured.SetNestedField(obj.Object, image, "spec", "template", "spec", "containers")
	return obj
}

func TestApplyManifestsReplacesAJobWhoseTemplateChanged(t *testing.T) {
	scheme := runtime.NewScheme()
	if err := batchv1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	var deleted []string
	patches := 0
	c := interceptor.NewClient(fake.NewClientBuilder().WithScheme(scheme).Build(), interceptor.Funcs{
		Patch: func(ctx context.Context, cl client.WithWatch, obj client.Object, patch client.Patch, opts ...client.PatchOption) error {
			patches++
			// The first apply meets the completed run with the old template;
			// after the replacement the apply succeeds (create).
			if patches == 1 {
				return immutableTemplateError(obj.GetName())
			}
			return nil
		},
		Delete: func(ctx context.Context, cl client.WithWatch, obj client.Object, opts ...client.DeleteOption) error {
			deleted = append(deleted, obj.GetName())
			return nil
		},
	})

	base := &Base{}
	job := renderedJob("planton-openfga-migrate", "openfga/openfga:v1.19.0")
	if err := base.ApplyManifests(context.Background(), c, ownershipPlatform(), []*unstructured.Unstructured{job}); err != nil {
		t.Fatalf("a changed Job must be replaced, not refused: %v", err)
	}
	if len(deleted) != 1 || deleted[0] != "planton-openfga-migrate" {
		t.Fatalf("expected the completed run to be deleted once, got deletions %v", deleted)
	}
	if patches != 2 {
		t.Fatalf("expected the apply to be retried after the replacement, got %d patches", patches)
	}
}

func TestApplyManifestsDoesNotDeleteOnOtherRefusals(t *testing.T) {
	scheme := runtime.NewScheme()
	if err := batchv1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	cases := map[string]struct {
		obj *unstructured.Unstructured
		err error
	}{
		"a Job refused for another reason": {
			obj: renderedJob("planton-openfga-migrate", "openfga/openfga:v1.19.0"),
			err: apierrors.NewInvalid(schema.GroupKind{Group: "batch", Kind: "Job"}, "planton-openfga-migrate",
				field.ErrorList{field.Required(field.NewPath("spec", "template", "spec", "containers"), "at least one container")}),
		},
		"a non-Job with an immutable field": {
			obj: rendered("Service", "planton", "planton-openfga"),
			err: apierrors.NewInvalid(schema.GroupKind{Kind: "Service"}, "planton-openfga",
				field.ErrorList{field.Invalid(field.NewPath("spec", "clusterIP"), "...", "field is immutable")}),
		},
		"a Job refused with a non-validation error": {
			obj: renderedJob("planton-openfga-migrate", "openfga/openfga:v1.19.0"),
			err: apierrors.NewForbidden(schema.GroupResource{Group: "batch", Resource: "jobs"}, "planton-openfga-migrate", errors.New("denied")),
		},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			deleted := 0
			c := interceptor.NewClient(fake.NewClientBuilder().WithScheme(scheme).Build(), interceptor.Funcs{
				Patch: func(ctx context.Context, cl client.WithWatch, obj client.Object, patch client.Patch, opts ...client.PatchOption) error {
					return tc.err
				},
				Delete: func(ctx context.Context, cl client.WithWatch, obj client.Object, opts ...client.DeleteOption) error {
					deleted++
					return nil
				},
			})
			base := &Base{}
			err := base.ApplyManifests(context.Background(), c, ownershipPlatform(), []*unstructured.Unstructured{tc.obj})
			if err == nil {
				t.Fatal("the refusal must surface as an error")
			}
			if deleted != 0 {
				t.Fatalf("nothing may be deleted over a refusal that is not a changed Job template; %d deletions", deleted)
			}
		})
	}
}
