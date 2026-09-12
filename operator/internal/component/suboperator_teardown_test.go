package component

import (
	"context"
	"strings"
	"testing"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/plantonhq/planton/operator/internal/resources"
)

// The teardown's decisions are what these tests pin -- foreign is untouched,
// in-use is kept and named, draining waits, and a clean removal deletes
// exactly the marked objects in the release, namespace included only when
// nothing foreign lives in it. The real API server path is exercised by
// envtest and the live lab.

var widgetGVK = schema.GroupVersionKind{Group: "example.io", Version: "v1", Kind: "Widget"}

func teardownScheme(t *testing.T) *runtime.Scheme {
	t.Helper()
	s := subOperatorScheme(t)
	s.AddKnownTypeWithName(widgetGVK, &unstructured.Unstructured{})
	s.AddKnownTypeWithName(widgetGVK.GroupVersion().WithKind("WidgetList"), &unstructured.UnstructuredList{})
	return s
}

func unstructuredObject(gvk schema.GroupVersionKind, namespace, name string, marked bool) *unstructured.Unstructured {
	obj := &unstructured.Unstructured{}
	obj.SetGroupVersionKind(gvk)
	obj.SetName(name)
	if namespace != "" {
		obj.SetNamespace(namespace)
	}
	if marked {
		obj.SetLabels(map[string]string{ManagedByLabel: SSAFieldManager})
	}
	return obj
}

// testRelease is the vendored release under test: a namespace, the detect
// definition, an admission webhook, cluster RBAC, and one controller
// Deployment -- the shape both CloudNativePG and Tekton share.
func testRelease(marked bool) []*unstructured.Unstructured {
	crd := unstructuredObject(schema.GroupVersionKind{Group: "apiextensions.k8s.io", Version: "v1", Kind: "CustomResourceDefinition"}, "", testCRDName, marked)
	_ = unstructured.SetNestedField(crd.Object, "example.io", "spec", "group")
	_ = unstructured.SetNestedField(crd.Object, "Widget", "spec", "names", "kind")
	_ = unstructured.SetNestedField(crd.Object, "WidgetList", "spec", "names", "listKind")
	_ = unstructured.SetNestedSlice(crd.Object, []any{
		map[string]any{"name": "v1", "served": true, "storage": true},
	}, "spec", "versions")
	return []*unstructured.Unstructured{
		unstructuredObject(schema.GroupVersionKind{Version: "v1", Kind: "Namespace"}, "", testNamespace, marked),
		crd,
		unstructuredObject(schema.GroupVersionKind{Group: "admissionregistration.k8s.io", Version: "v1", Kind: "ValidatingWebhookConfiguration"}, "", "sub-op-validating", marked),
		unstructuredObject(schema.GroupVersionKind{Group: "rbac.authorization.k8s.io", Version: "v1", Kind: "ClusterRole"}, "", "sub-op-manager", marked),
		unstructuredObject(schema.GroupVersionKind{Group: "rbac.authorization.k8s.io", Version: "v1", Kind: "ClusterRoleBinding"}, "", "sub-op-manager", marked),
		unstructuredObject(schema.GroupVersionKind{Group: "apps", Version: "v1", Kind: "Deployment"}, testNamespace, testDeployment, marked),
	}
}

func widget(name string, owner *metav1.OwnerReference) *unstructured.Unstructured {
	w := unstructuredObject(widgetGVK, "tenant", name, false)
	if owner != nil {
		w.SetOwnerReferences([]metav1.OwnerReference{*owner})
	}
	return w
}

func departedPlatformOwner() *metav1.OwnerReference {
	return &metav1.OwnerReference{APIVersion: "planton.ai/v1", Kind: "PlantonPlatform", Name: "planton", UID: types.UID("gone")}
}

// runTeardown builds a fake cluster from the release (as applied, marked) plus
// extras, and runs the teardown with the given platform-existence answer.
func runTeardown(t *testing.T, platformExists bool, cluster []*unstructured.Unstructured, extras ...client.Object) (TeardownVerdict, client.Client) {
	t.Helper()
	objs := make([]client.Object, 0, len(cluster)+len(extras))
	for _, o := range cluster {
		objs = append(objs, o)
	}
	objs = append(objs, extras...)
	c := fake.NewClientBuilder().WithScheme(teardownScheme(t)).WithObjects(objs...).Build()

	base := &Base{}
	verdict, err := base.RemoveSubOperator(context.Background(), c, SubOperatorOptions{
		LogName:   "test-sub-operator",
		CRDName:   testCRDName,
		Loader:    func() ([]*unstructured.Unstructured, error) { return testRelease(false), nil },
		Namespace: testNamespace,
	}, func(context.Context, string, string, types.UID) (bool, error) { return platformExists, nil })
	if err != nil {
		t.Fatalf("RemoveSubOperator: %v", err)
	}
	return verdict, c
}

func exists(t *testing.T, c client.Client, gvk schema.GroupVersionKind, namespace, name string) bool {
	t.Helper()
	obj := &unstructured.Unstructured{}
	obj.SetGroupVersionKind(gvk)
	err := c.Get(context.Background(), types.NamespacedName{Namespace: namespace, Name: name}, obj)
	if err == nil {
		return true
	}
	if apierrors.IsNotFound(err) {
		return false
	}
	t.Fatalf("reading %s %s/%s: %v", gvk.Kind, namespace, name, err)
	return false
}

func TestRemoveSubOperator_AbsentDefinition_IsAlreadyRemoved(t *testing.T) {
	verdict, _ := runTeardown(t, false, nil)
	if verdict.Outcome != TeardownRemoved {
		t.Errorf("no definition means nothing to remove, got %s", verdict.Outcome)
	}
}

// An earlier pass died after the definitions went and before the namespaced
// objects did (the live lab found this: a missing delete grant stopped the
// pass halfway). The next pass must finish the job, not declare it done
// because the detect definition is gone.
func TestRemoveSubOperator_InterruptedRemoval_ResumesFromTheNamespace(t *testing.T) {
	leftovers := testRelease(true)
	// Definitions, webhooks, and cluster RBAC already gone; the namespace and
	// the controller Deployment remain.
	var remaining []*unstructured.Unstructured
	for _, obj := range leftovers {
		if obj.GetKind() == "Namespace" || obj.GetKind() == "Deployment" {
			remaining = append(remaining, obj)
		}
	}
	verdict, c := runTeardown(t, false, remaining)
	if verdict.Outcome != TeardownRemoved {
		t.Fatalf("expected Removed, got %s", verdict.Outcome)
	}
	if exists(t, c, schema.GroupVersionKind{Group: "apps", Version: "v1", Kind: "Deployment"}, testNamespace, testDeployment) {
		t.Error("the leftover controller Deployment survived the resumed pass")
	}
	if exists(t, c, schema.GroupVersionKind{Version: "v1", Kind: "Namespace"}, "", testNamespace) {
		t.Error("the leftover namespace survived the resumed pass")
	}
}

// A release installed by anything but this operator is never ours to remove,
// however many platforms came and went.
func TestRemoveSubOperator_ForeignInstall_IsKeptUntouched(t *testing.T) {
	foreign := testRelease(false)
	foreign[1].SetLabels(map[string]string{ManagedByLabel: "Helm"})
	verdict, c := runTeardown(t, false, foreign)
	if verdict.Outcome != TeardownKeptForeign {
		t.Fatalf("expected KeptForeign, got %s", verdict.Outcome)
	}
	for _, obj := range foreign {
		if !exists(t, c, obj.GroupVersionKind(), obj.GetNamespace(), obj.GetName()) {
			t.Errorf("%s %s was deleted from a foreign install", obj.GetKind(), obj.GetName())
		}
	}
	if !strings.Contains(verdict.Explain("cloudnative-pg"), "not this operator's to remove") {
		t.Errorf("the explanation must say why it stayed: %s", verdict.Explain("cloudnative-pg"))
	}
}

// Instances nobody departed owns are somebody's data: the release stays and
// every one of them is named, so the person knows exactly what to move.
func TestRemoveSubOperator_InstancesInUse_KeepTheReleaseAndNameThem(t *testing.T) {
	verdict, c := runTeardown(t, true, testRelease(true),
		widget("customer-db", nil),
		widget("still-owned", &metav1.OwnerReference{APIVersion: "planton.ai/v1", Kind: "PlantonPlatform", Name: "other", UID: "alive"}))
	if verdict.Outcome != TeardownKeptInUse {
		t.Fatalf("expected KeptInUse, got %s", verdict.Outcome)
	}
	want := []string{"Widget tenant/customer-db", "Widget tenant/still-owned"}
	if strings.Join(verdict.Objects, ",") != strings.Join(want, ",") {
		t.Errorf("objects = %v, want %v", verdict.Objects, want)
	}
	if !exists(t, c, testRelease(false)[1].GroupVersionKind(), "", testCRDName) {
		t.Error("the definition was deleted while instances used it -- their data would have gone with it")
	}
	explanation := verdict.Explain("cloudnative-pg")
	for _, must := range []string{"still in use by", "Widget tenant/customer-db", "delete those", "keep cloudnative-pg as your own"} {
		if !strings.Contains(explanation, must) {
			t.Errorf("explanation lacks %q: %s", must, explanation)
		}
	}
}

// An instance whose owner is a platform that no longer exists is garbage
// collection's to take; the teardown waits instead of racing it.
func TestRemoveSubOperator_InstancesOfADepartedPlatform_Drain(t *testing.T) {
	verdict, c := runTeardown(t, false, testRelease(true), widget("platform-db", departedPlatformOwner()))
	if verdict.Outcome != TeardownDraining {
		t.Fatalf("expected Draining, got %s", verdict.Outcome)
	}
	if len(verdict.Objects) != 1 || verdict.Objects[0] != "Widget tenant/platform-db" {
		t.Errorf("draining objects = %v", verdict.Objects)
	}
	if !exists(t, c, testRelease(false)[1].GroupVersionKind(), "", testCRDName) {
		t.Error("the definition must stay until the draining instance is gone")
	}
}

// Nothing uses it: every marked object in the release leaves, the namespace
// included, and a same-named object without the mark is not ours to touch.
func TestRemoveSubOperator_Unused_RemovesExactlyTheMarkedRelease(t *testing.T) {
	cluster := testRelease(true)
	// The ClusterRoleBinding on this cluster is somebody else's, same name.
	cluster[4].SetLabels(nil)
	verdict, c := runTeardown(t, false, cluster)
	if verdict.Outcome != TeardownRemoved {
		t.Fatalf("expected Removed, got %s", verdict.Outcome)
	}
	for _, obj := range testRelease(false) {
		if obj.GetKind() == "ClusterRoleBinding" {
			if !exists(t, c, obj.GroupVersionKind(), "", obj.GetName()) {
				t.Error("an unmarked same-named ClusterRoleBinding was deleted -- never ours to touch")
			}
			continue
		}
		if exists(t, c, obj.GroupVersionKind(), obj.GetNamespace(), obj.GetName()) {
			t.Errorf("%s %s survived a clean removal", obj.GetKind(), obj.GetName())
		}
	}
	if len(verdict.ForeignInNamespace) != 0 {
		t.Errorf("nothing foreign lived in the namespace: %v", verdict.ForeignInNamespace)
	}
}

// A release namespace holding something the operator did not install keeps
// the namespace: our objects leave, theirs stay, and the verdict names them.
func TestRemoveSubOperator_ForeignObjectInReleaseNamespace_KeepsTheNamespace(t *testing.T) {
	stranger := unstructuredObject(schema.GroupVersionKind{Group: "apps", Version: "v1", Kind: "Deployment"}, testNamespace, "their-tool", false)
	verdict, c := runTeardown(t, false, testRelease(true), stranger)
	if verdict.Outcome != TeardownRemoved {
		t.Fatalf("expected Removed, got %s", verdict.Outcome)
	}
	if len(verdict.ForeignInNamespace) != 1 || verdict.ForeignInNamespace[0] != "Deployment sub-op-system/their-tool" {
		t.Errorf("foreign objects = %v", verdict.ForeignInNamespace)
	}
	if !exists(t, c, schema.GroupVersionKind{Version: "v1", Kind: "Namespace"}, "", testNamespace) {
		t.Error("the namespace was deleted with a stranger's Deployment inside it")
	}
	if !exists(t, c, stranger.GroupVersionKind(), testNamespace, "their-tool") {
		t.Error("the stranger's Deployment was deleted")
	}
	if exists(t, c, schema.GroupVersionKind{Group: "apps", Version: "v1", Kind: "Deployment"}, testNamespace, testDeployment) {
		t.Error("our controller Deployment survived")
	}
	if !strings.Contains(verdict.Explain("tekton-pipelines"), "kept its namespace") {
		t.Errorf("explanation: %s", verdict.Explain("tekton-pipelines"))
	}
}

func TestSharedSubOperators_AreTheInstallGatesDefinitions(t *testing.T) {
	shared := SharedSubOperators()
	if len(shared) != 3 {
		t.Fatalf("expected the three vendored sub-operators, got %d", len(shared))
	}
	// The plugin precedes CloudNativePG on purpose: the two share a namespace
	// and the sweep would otherwise delete it from under the plugin.
	if shared[0].CRDName != resources.BarmanCloudObjectStoreCRDName || shared[0].Namespace != cnpgOperatorNamespace || shared[0].Loader == nil {
		t.Errorf("Barman Cloud plugin definition drifted or is not first: %+v", shared[0])
	}
	if shared[1].CRDName != cnpgClusterCRDName || shared[1].Namespace != cnpgOperatorNamespace {
		t.Errorf("CloudNativePG definition drifted: %+v", shared[1])
	}
	if shared[2].LogName != "tekton-pipelines" || shared[2].Loader == nil {
		t.Errorf("Tekton definition drifted: %+v", shared[2])
	}
}
