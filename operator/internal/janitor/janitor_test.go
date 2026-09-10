package janitor

import (
	"context"
	"testing"

	rbacv1 "k8s.io/api/rbac/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	v1 "github.com/plantonhq/planton/operator/api/v1"
	"github.com/plantonhq/planton/operator/internal/component"
	"github.com/plantonhq/planton/operator/internal/resources"
)

// The sweep's decisions are pinned here: satellites leave when their platform
// is gone (by UID, never by name), sub-operators are not even considered while
// a platform remains, and both are taken back when the last one leaves.

const testCRD = "widgets.example.io"

func janitorScheme(t *testing.T) *runtime.Scheme {
	t.Helper()
	s := runtime.NewScheme()
	if err := clientgoscheme.AddToScheme(s); err != nil {
		t.Fatalf("scheme: %v", err)
	}
	if err := v1.AddToScheme(s); err != nil {
		t.Fatalf("scheme: %v", err)
	}
	crdGVK := schema.GroupVersionKind{Group: "apiextensions.k8s.io", Version: "v1", Kind: "CustomResourceDefinition"}
	s.AddKnownTypeWithName(crdGVK, &unstructured.Unstructured{})
	s.AddKnownTypeWithName(crdGVK.GroupVersion().WithKind("CustomResourceDefinitionList"), &unstructured.UnstructuredList{})
	widget := schema.GroupVersionKind{Group: "example.io", Version: "v1", Kind: "Widget"}
	s.AddKnownTypeWithName(widget, &unstructured.Unstructured{})
	s.AddKnownTypeWithName(widget.GroupVersion().WithKind("WidgetList"), &unstructured.UnstructuredList{})
	return s
}

func platform(name string, uid types.UID) *v1.PlantonPlatform {
	return &v1.PlantonPlatform{ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: name, UID: uid}}
}

func satellitePair(name string, uid types.UID) []client.Object {
	labels := map[string]string{resources.PlatformUIDLabel: string(uid)}
	return []client.Object{
		&rbacv1.ClusterRole{ObjectMeta: metav1.ObjectMeta{Name: name, Labels: labels}},
		&rbacv1.ClusterRoleBinding{ObjectMeta: metav1.ObjectMeta{Name: name, Labels: labels}},
	}
}

func ourDefinition() *unstructured.Unstructured {
	crd := &unstructured.Unstructured{}
	crd.SetGroupVersionKind(schema.GroupVersionKind{Group: "apiextensions.k8s.io", Version: "v1", Kind: "CustomResourceDefinition"})
	crd.SetName(testCRD)
	crd.SetLabels(map[string]string{component.ManagedByLabel: component.SSAFieldManager})
	_ = unstructured.SetNestedField(crd.Object, "example.io", "spec", "group")
	_ = unstructured.SetNestedField(crd.Object, "Widget", "spec", "names", "kind")
	_ = unstructured.SetNestedField(crd.Object, "WidgetList", "spec", "names", "listKind")
	_ = unstructured.SetNestedSlice(crd.Object, []any{map[string]any{"name": "v1", "served": true, "storage": true}}, "spec", "versions")
	return crd
}

func newJanitor(t *testing.T, objs ...client.Object) (*Janitor, client.Client) {
	t.Helper()
	c := fake.NewClientBuilder().WithScheme(janitorScheme(t)).WithObjects(objs...).Build()
	j := &Janitor{
		Client: c,
		SubOperators: []component.SubOperatorOptions{{
			LogName: "test-sub-operator",
			CRDName: testCRD,
			Loader: func() ([]*unstructured.Unstructured, error) {
				crd := ourDefinition()
				crd.SetLabels(nil)
				return []*unstructured.Unstructured{crd}, nil
			},
			Namespace: "sub-op-system",
		}},
	}
	return j, c
}

func clusterRoleExists(t *testing.T, c client.Client, name string) bool {
	t.Helper()
	err := c.Get(context.Background(), types.NamespacedName{Name: name}, &rbacv1.ClusterRole{})
	if err == nil {
		return true
	}
	if apierrors.IsNotFound(err) {
		return false
	}
	t.Fatalf("reading ClusterRole %s: %v", name, err)
	return false
}

func definitionExists(t *testing.T, c client.Client) bool {
	t.Helper()
	crd := &unstructured.Unstructured{}
	crd.SetGroupVersionKind(schema.GroupVersionKind{Group: "apiextensions.k8s.io", Version: "v1", Kind: "CustomResourceDefinition"})
	err := c.Get(context.Background(), types.NamespacedName{Name: testCRD}, crd)
	if err == nil {
		return true
	}
	if apierrors.IsNotFound(err) {
		return false
	}
	t.Fatalf("reading definition: %v", err)
	return false
}

// A platform remains: its own pair stays, the departed platform's pair goes,
// and the shared sub-operator is not even considered.
func TestSweep_WithAPlatformRemaining_RemovesOnlyStrandedSatellites(t *testing.T) {
	objs := append([]client.Object{platform("keep", "uid-keep"), ourDefinition()},
		append(satellitePair("keep-token-reviewer", "uid-keep"), satellitePair("gone-token-reviewer", "uid-gone")...)...)
	j, c := newJanitor(t, objs...)

	outcome, err := j.Sweep(context.Background())
	if err != nil {
		t.Fatalf("Sweep: %v", err)
	}
	if outcome.PlatformsRemaining != 1 || outcome.SatellitesRemoved != 2 {
		t.Errorf("outcome = %+v", outcome)
	}
	if !clusterRoleExists(t, c, "keep-token-reviewer") {
		t.Error("the live platform's ClusterRole was deleted")
	}
	if clusterRoleExists(t, c, "gone-token-reviewer") {
		t.Error("the departed platform's ClusterRole survived")
	}
	if len(outcome.Verdicts) != 0 || !definitionExists(t, c) {
		t.Error("a shared sub-operator must not be touched while a platform remains")
	}
}

// Same name, new UID: a platform recreated under its old name keeps the pair
// its new reconcile applied. This is why the label carries the UID.
func TestSweep_RecreatedPlatformUnderTheSameName_KeepsItsPair(t *testing.T) {
	objs := append([]client.Object{platform("planton", "uid-new"), ourDefinition()},
		satellitePair("planton-planton-control-plane-token-reviewer", "uid-new")...)
	j, c := newJanitor(t, objs...)

	if _, err := j.Sweep(context.Background()); err != nil {
		t.Fatalf("Sweep: %v", err)
	}
	if !clusterRoleExists(t, c, "planton-planton-control-plane-token-reviewer") {
		t.Error("the recreated platform's grant was deleted")
	}
}

// A pair without the UID label predates it; the janitor never guesses
// ownership from a name.
func TestSweep_UnlabelledClusterRole_IsLeftAlone(t *testing.T) {
	objs := []client.Object{&rbacv1.ClusterRole{ObjectMeta: metav1.ObjectMeta{Name: "somebody-elses"}}}
	j, c := newJanitor(t, objs...)

	outcome, err := j.Sweep(context.Background())
	if err != nil {
		t.Fatalf("Sweep: %v", err)
	}
	if outcome.SatellitesRemoved != 0 || !clusterRoleExists(t, c, "somebody-elses") {
		t.Error("an unlabelled ClusterRole was touched")
	}
}

// No platform remains: the stranded pair goes and the shared sub-operator is
// taken back.
func TestSweep_LastPlatformGone_RemovesTheSubOperator(t *testing.T) {
	objs := append([]client.Object{ourDefinition()}, satellitePair("gone-token-reviewer", "uid-gone")...)
	j, c := newJanitor(t, objs...)

	outcome, err := j.Sweep(context.Background())
	if err != nil {
		t.Fatalf("Sweep: %v", err)
	}
	if outcome.PlatformsRemaining != 0 || outcome.SatellitesRemoved != 2 || outcome.Draining {
		t.Errorf("outcome = %+v", outcome)
	}
	if v := outcome.Verdicts["test-sub-operator"]; v.Outcome != component.TeardownRemoved {
		t.Errorf("verdict = %+v", v)
	}
	if definitionExists(t, c) {
		t.Error("the shared sub-operator's definition survived the last platform")
	}
}

// The platform's own instance is still draining through garbage collection:
// the sweep reports Draining so the caller asks again, and deletes nothing of
// the sub-operator yet.
func TestSweep_DrainingInstance_WaitsForGarbageCollection(t *testing.T) {
	instance := &unstructured.Unstructured{}
	instance.SetGroupVersionKind(schema.GroupVersionKind{Group: "example.io", Version: "v1", Kind: "Widget"})
	instance.SetNamespace("planton")
	instance.SetName("planton-postgres")
	instance.SetOwnerReferences([]metav1.OwnerReference{{APIVersion: "planton.ai/v1", Kind: "PlantonPlatform", Name: "planton", UID: "uid-gone"}})
	j, c := newJanitor(t, ourDefinition(), instance)

	outcome, err := j.Sweep(context.Background())
	if err != nil {
		t.Fatalf("Sweep: %v", err)
	}
	if !outcome.Draining {
		t.Errorf("expected Draining, got %+v", outcome.Verdicts)
	}
	if !definitionExists(t, c) {
		t.Error("the definition was deleted under a draining instance")
	}
}
