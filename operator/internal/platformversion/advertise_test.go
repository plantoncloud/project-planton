package platformversion

import (
	"context"
	"testing"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

var crdGVK = schema.GroupVersionKind{Group: "apiextensions.k8s.io", Version: "v1", Kind: "CustomResourceDefinition"}

func definition(annotations map[string]string) *unstructured.Unstructured {
	crd := &unstructured.Unstructured{}
	crd.SetGroupVersionKind(crdGVK)
	crd.SetName(PlatformDefinitionName)
	if annotations != nil {
		crd.SetAnnotations(annotations)
	}
	return crd
}

func TestAdvertise_StampsTheFloorAndKeepsOtherAnnotations(t *testing.T) {
	scheme := runtime.NewScheme()
	scheme.AddKnownTypeWithName(crdGVK, &unstructured.Unstructured{})
	c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(definition(map[string]string{
		"helm.sh/resource-policy": "keep",
		Annotation:                "v0.0.1", // an older operator's advertisement, moved by this one
	})).Build()

	if err := Advertise(context.Background(), c); err != nil {
		t.Fatal(err)
	}
	got := definition(nil)
	if err := c.Get(context.Background(), types.NamespacedName{Name: PlatformDefinitionName}, got); err != nil {
		t.Fatal(err)
	}
	if got.GetAnnotations()[Annotation] != MinimumSupported {
		t.Fatalf("floor not advertised: %v", got.GetAnnotations())
	}
	if got.GetAnnotations()["helm.sh/resource-policy"] != "keep" {
		t.Fatalf("the chart's own annotation must survive: %v", got.GetAnnotations())
	}
	// A second call is a no-op read.
	if err := Advertise(context.Background(), c); err != nil {
		t.Fatalf("re-advertising must be idempotent: %v", err)
	}
}

func TestAdvertise_MissingDefinitionIsAnError(t *testing.T) {
	scheme := runtime.NewScheme()
	scheme.AddKnownTypeWithName(crdGVK, &unstructured.Unstructured{})
	c := fake.NewClientBuilder().WithScheme(scheme).Build()
	if err := Advertise(context.Background(), c); err == nil {
		t.Fatal("no definition to advertise on must be reported, so the caller can log it")
	}
}
