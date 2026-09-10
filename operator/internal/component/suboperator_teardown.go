package component

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"sync"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
)

// subOperatorMu serializes the install and teardown of vendored sub-operators.
// The install gate runs inside a platform's reconcile; the teardown runs from
// the janitor when no platform remains. A platform created while a teardown
// is deleting the previous install's definitions must never interleave with
// it -- the two would leave a half-applied release neither side recognizes.
var subOperatorMu sync.Mutex

const (
	kindNamespace     = "Namespace"
	kindPlatformOwner = "PlantonPlatform"
)

// ManagedByLabel is the label every vendored object carries once this
// operator has applied it -- the one ownership mark the install gate reads on
// the detect CRD and the teardown re-reads on every object before deleting.
const ManagedByLabel = "app.kubernetes.io/managed-by"

// TeardownOutcome names what RemoveSubOperator decided.
type TeardownOutcome string

const (
	// TeardownRemoved: the release was ours, nothing used it, and every object
	// it installed is gone (or already was).
	TeardownRemoved TeardownOutcome = "Removed"
	// TeardownKeptForeign: the detect CRD exists but is not ours -- a Helm,
	// GitOps, or hand-applied install this operator only ever borrowed.
	TeardownKeptForeign TeardownOutcome = "KeptForeign"
	// TeardownKeptInUse: objects of the release's own kinds still exist that no
	// departed platform owns -- a customer's database, a build somebody ran.
	// Removing the release would destroy them; the objects are named.
	TeardownKeptInUse TeardownOutcome = "KeptInUse"
	// TeardownDraining: every remaining instance is owned by a platform that
	// no longer exists and is on its way out through garbage collection.
	// Ask again shortly.
	TeardownDraining TeardownOutcome = "Draining"
)

// TeardownVerdict is RemoveSubOperator's answer: the outcome, the objects
// that decided it (in use or draining), and -- when the release was ours and
// its namespace held something foreign -- the objects that kept the namespace
// alive. Every list is sorted so a verdict reads the same twice.
type TeardownVerdict struct {
	Outcome TeardownOutcome
	// Objects that kept the release (KeptInUse) or delay it (Draining), as
	// "kind namespace/name" or "kind name" for cluster-scoped ones.
	Objects []string
	// ForeignInNamespace lists objects that were not ours inside a namespace
	// the release installed, so the namespace stayed while our objects left.
	ForeignInNamespace []string
}

// Explain renders the verdict for a person -- an Event message, a log line --
// with the objects it turned on and the way forward.
func (v TeardownVerdict) Explain(logName string) string {
	switch v.Outcome {
	case TeardownRemoved:
		if len(v.ForeignInNamespace) == 0 {
			return fmt.Sprintf("Removed %s: no platform remains on this cluster and nothing else used it", logName)
		}
		return fmt.Sprintf("Removed %s, but kept its namespace: it holds objects this operator did not install (%s) -- delete them and the namespace when they are no longer needed",
			logName, strings.Join(v.ForeignInNamespace, ", "))
	case TeardownKeptForeign:
		return fmt.Sprintf("Kept %s: it was installed by something other than this operator, so it is not this operator's to remove", logName)
	case TeardownKeptInUse:
		return fmt.Sprintf("Kept %s: no platform remains on this cluster, but it is still in use by %s -- delete those to let the operator remove it, or keep %s as your own",
			logName, strings.Join(v.Objects, ", "), logName)
	case TeardownDraining:
		return fmt.Sprintf("Waiting to remove %s: %s belonged to a deleted platform and is still being garbage-collected",
			logName, strings.Join(v.Objects, ", "))
	}
	return fmt.Sprintf("%s: %s", logName, v.Outcome)
}

// PlatformExists answers whether a PlantonPlatform with the given UID is
// still on the cluster; RemoveSubOperator uses it to tell a draining
// instance (its owner is gone) from one in use (its owner is not a platform,
// or is a platform that still exists).
type PlatformExists func(ctx context.Context, namespace, name string, uid types.UID) (bool, error)

// RemoveSubOperator is EnsureSubOperator's mirror: the one place a vendored
// sub-operator install is taken back off the cluster. It is called only when
// no platform remains, and it removes only what carries this operator's mark.
//
//   - detect CRD absent: nothing to do -- Removed.
//   - detect CRD present but foreign: KeptForeign, nothing touched.
//   - instances of any of the release's kinds exist: if every one is owned by
//     a PlantonPlatform that no longer exists it is Draining (garbage
//     collection is mid-flight; call again); otherwise KeptInUse, the
//     objects named. Removing the definitions would delete those instances --
//     a customer's database on the shared operator -- so the release stays,
//     and the caller writes the verdict where a person will read it.
//   - nothing uses it: the release's objects are deleted in the order that
//     never strands a caller -- admission webhooks first (so nothing is left
//     refusing writes for a controller that is gone), then the definitions
//     (instance-free, so instant), then cluster RBAC, then the release's
//     namespaced objects, then the namespaces the release created. A
//     namespace goes only when it carries the mark AND holds no workload or
//     volume that is not the release's own; otherwise it stays, its foreign
//     contents named in the verdict. Configuration a mesh or the departed
//     controller left behind (an injected root-CA ConfigMap, a webhook
//     certificate, leader-election leases) is nobody's data and never keeps
//     a namespace alive.
//
// Every deletion re-reads the live object and requires the mark: a same-named
// object someone else created is never ours to delete, whatever the manifest
// says. The whole pass holds the sub-operator mutex so a platform being
// created cannot interleave its install with this teardown.
func (b *Base) RemoveSubOperator(ctx context.Context, c client.Client, opts SubOperatorOptions, platformExists PlatformExists) (TeardownVerdict, error) {
	subOperatorMu.Lock()
	defer subOperatorMu.Unlock()

	log := logf.FromContext(ctx).WithValues("subOperator", opts.LogName)

	crd, err := b.getCRD(ctx, c, opts.CRDName)
	if err != nil {
		return TeardownVerdict{}, fmt.Errorf("checking for %s CRD: %w", opts.CRDName, err)
	}
	if crd != nil && crd.GetLabels()[ManagedByLabel] != SSAFieldManager {
		return TeardownVerdict{Outcome: TeardownKeptForeign}, nil
	}

	objs, err := opts.Loader()
	if err != nil {
		return TeardownVerdict{}, fmt.Errorf("loading %s manifests: %w", opts.LogName, err)
	}
	for _, obj := range objs {
		if obj.GetNamespace() == "" && isNamespaced(obj) {
			obj.SetNamespace(opts.Namespace)
		}
	}

	// No detect definition: either the release was never installed, or an
	// earlier pass was interrupted after the definitions went and before the
	// namespaced objects did. The release's namespaces tell the two apart --
	// and every delete below re-reads the mark, so resuming over a foreign
	// namespace of the same name deletes nothing of theirs.
	if crd == nil {
		remaining, err := b.anyNamespaceExists(ctx, c, releaseNamespaces(objs, opts.Namespace))
		if err != nil {
			return TeardownVerdict{}, err
		}
		if !remaining {
			return TeardownVerdict{Outcome: TeardownRemoved}, nil
		}
		log.Info("Resuming an interrupted removal: definitions gone, namespaced objects remain")
	}

	inUse, draining, err := b.subOperatorInstances(ctx, c, objs, platformExists)
	if err != nil {
		return TeardownVerdict{}, err
	}
	if len(inUse) > 0 {
		return TeardownVerdict{Outcome: TeardownKeptInUse, Objects: inUse}, nil
	}
	if len(draining) > 0 {
		return TeardownVerdict{Outcome: TeardownDraining, Objects: draining}, nil
	}

	// Cluster-scoped objects, in the order that never strands a caller.
	for _, kind := range []string{"MutatingWebhookConfiguration", "ValidatingWebhookConfiguration", "CustomResourceDefinition", "ClusterRoleBinding", "ClusterRole"} {
		for _, obj := range objs {
			if obj.GetKind() != kind {
				continue
			}
			if err := b.deleteIfMarked(ctx, c, obj); err != nil {
				return TeardownVerdict{}, err
			}
		}
	}
	// Any other cluster-scoped kind the release may carry (a PriorityClass,
	// a StorageClass) goes the same way.
	for _, obj := range objs {
		if isNamespaced(obj) || obj.GetKind() == kindNamespace {
			continue
		}
		switch obj.GetKind() {
		case "MutatingWebhookConfiguration", "ValidatingWebhookConfiguration", "CustomResourceDefinition", "ClusterRoleBinding", "ClusterRole":
			continue
		}
		if err := b.deleteIfMarked(ctx, c, obj); err != nil {
			return TeardownVerdict{}, err
		}
	}

	// The release's namespaced objects leave explicitly -- never left to a
	// namespace deletion that may be slow, or that may not happen at all.
	for _, obj := range objs {
		if !isNamespaced(obj) {
			continue
		}
		if err := b.deleteIfMarked(ctx, c, obj); err != nil {
			return TeardownVerdict{}, err
		}
	}

	// Then the release's namespaces: deleted when nothing foreign lives
	// there, kept (and the foreign objects named) otherwise.
	var foreign []string
	for _, ns := range releaseNamespaces(objs, opts.Namespace) {
		strangers, err := b.foreignObjectsIn(ctx, c, ns)
		if err != nil {
			return TeardownVerdict{}, err
		}
		if len(strangers) > 0 {
			foreign = append(foreign, strangers...)
			continue
		}
		nsObj := &unstructured.Unstructured{}
		nsObj.SetGroupVersionKind(schema.GroupVersionKind{Version: "v1", Kind: kindNamespace})
		nsObj.SetName(ns)
		if err := b.deleteIfMarked(ctx, c, nsObj); err != nil {
			return TeardownVerdict{}, err
		}
	}
	sort.Strings(foreign)

	log.Info("Removed sub-operator", "foreignInNamespace", len(foreign))
	return TeardownVerdict{Outcome: TeardownRemoved, ForeignInNamespace: foreign}, nil
}

// subOperatorInstances lists every object of every kind the release defines
// and sorts them into in-use (kept) and draining (owned by a platform that no
// longer exists). Kinds whose definition is already gone contribute nothing.
func (b *Base) subOperatorInstances(ctx context.Context, c client.Client, objs []*unstructured.Unstructured, platformExists PlatformExists) (inUse, draining []string, err error) {
	for _, obj := range objs {
		if obj.GetKind() != "CustomResourceDefinition" {
			continue
		}
		group, _, _ := unstructured.NestedString(obj.Object, "spec", "group")
		listKind, _, _ := unstructured.NestedString(obj.Object, "spec", "names", "listKind")
		kind, _, _ := unstructured.NestedString(obj.Object, "spec", "names", "kind")
		versions, _, _ := unstructured.NestedSlice(obj.Object, "spec", "versions")
		if group == "" || listKind == "" || len(versions) == 0 {
			continue
		}
		version := servedVersion(versions)
		if version == "" {
			continue
		}
		list := &unstructured.UnstructuredList{}
		list.SetGroupVersionKind(schema.GroupVersionKind{Group: group, Version: version, Kind: listKind})
		if err := c.List(ctx, list); err != nil {
			if meta.IsNoMatchError(err) || apierrors.IsNotFound(err) {
				continue
			}
			return nil, nil, fmt.Errorf("listing %s.%s instances: %w", kind, group, err)
		}
		for i := range list.Items {
			item := &list.Items[i]
			gone, err := ownedByDepartedPlatform(ctx, item, platformExists)
			if err != nil {
				return nil, nil, err
			}
			name := objectName(kind, item.GetNamespace(), item.GetName())
			if gone {
				draining = append(draining, name)
			} else {
				inUse = append(inUse, name)
			}
		}
	}
	sort.Strings(inUse)
	sort.Strings(draining)
	return inUse, draining, nil
}

// servedVersion picks the storage version of a definition, else the first
// served one -- the version a list must ask for.
func servedVersion(versions []any) string {
	first := ""
	for _, v := range versions {
		m, ok := v.(map[string]any)
		if !ok {
			continue
		}
		name, _ := m["name"].(string)
		served, _ := m["served"].(bool)
		storage, _ := m["storage"].(bool)
		if storage {
			return name
		}
		if served && first == "" {
			first = name
		}
	}
	return first
}

// ownedByDepartedPlatform is true when the object's owner is a PlantonPlatform
// that no longer exists: garbage collection will take the object, the
// teardown only has to wait. Any other owner -- or none -- means in use.
func ownedByDepartedPlatform(ctx context.Context, obj *unstructured.Unstructured, platformExists PlatformExists) (bool, error) {
	for _, ref := range obj.GetOwnerReferences() {
		if ref.Kind != kindPlatformOwner || !strings.HasPrefix(ref.APIVersion, "planton.ai/") {
			continue
		}
		exists, err := platformExists(ctx, obj.GetNamespace(), ref.Name, ref.UID)
		if err != nil {
			return false, err
		}
		return !exists, nil
	}
	return false, nil
}

// anyNamespaceExists reports whether any of the named namespaces is on the
// cluster (Terminating counts: its contents are still going).
func (b *Base) anyNamespaceExists(ctx context.Context, c client.Client, namespaces []string) (bool, error) {
	for _, name := range namespaces {
		ns := &unstructured.Unstructured{}
		ns.SetGroupVersionKind(schema.GroupVersionKind{Version: "v1", Kind: kindNamespace})
		if err := c.Get(ctx, types.NamespacedName{Name: name}, ns); err != nil {
			if apierrors.IsNotFound(err) {
				continue
			}
			return false, fmt.Errorf("reading namespace %s: %w", name, err)
		}
		return true, nil
	}
	return false, nil
}

// releaseNamespaces returns every namespace the release's objects live in,
// the default included, in a stable order.
func releaseNamespaces(objs []*unstructured.Unstructured, defaultNamespace string) []string {
	seen := map[string]bool{}
	for _, obj := range objs {
		if obj.GetKind() == kindNamespace {
			seen[obj.GetName()] = true
		} else if isNamespaced(obj) && obj.GetNamespace() != "" {
			seen[obj.GetNamespace()] = true
		}
	}
	if defaultNamespace != "" {
		seen[defaultNamespace] = true
	}
	out := make([]string, 0, len(seen))
	for ns := range seen {
		out = append(out, ns)
	}
	sort.Strings(out)
	return out
}

// foreignObjectsIn lists the workloads and volumes in a release namespace
// that are not the release's own -- anything of those kinds without the mark.
func (b *Base) foreignObjectsIn(ctx context.Context, c client.Client, namespace string) ([]string, error) {
	var strangers []string
	for _, gvk := range namespacedInventoryKinds {
		list := &unstructured.UnstructuredList{}
		list.SetGroupVersionKind(gvk)
		if err := c.List(ctx, list, client.InNamespace(namespace)); err != nil {
			if meta.IsNoMatchError(err) {
				continue
			}
			return nil, fmt.Errorf("listing %s in %s: %w", gvk.Kind, namespace, err)
		}
		for i := range list.Items {
			item := &list.Items[i]
			if item.GetLabels()[ManagedByLabel] == SSAFieldManager {
				continue
			}
			strangers = append(strangers, objectName(item.GetKind(), namespace, item.GetName()))
		}
	}
	sort.Strings(strangers)
	return strangers, nil
}

// namespacedInventoryKinds are the kinds that keep a release namespace alive
// when something unmarked of that kind lives there: workloads and volumes --
// what a person would lose. Deliberately NOT configuration objects: a mesh
// injects a root-CA ConfigMap into every namespace, the controller that just
// left minted its own webhook certificate Secret and leader-election Leases,
// and none of those is anyone's data. A namespace holding only such
// leftovers goes with the release; one holding a stranger's Deployment or
// claim stays, and the verdict names it.
var namespacedInventoryKinds = []schema.GroupVersionKind{
	{Group: "apps", Version: "v1", Kind: "DeploymentList"},
	{Group: "apps", Version: "v1", Kind: "StatefulSetList"},
	{Group: "apps", Version: "v1", Kind: "DaemonSetList"},
	{Group: "batch", Version: "v1", Kind: "JobList"},
	{Group: "batch", Version: "v1", Kind: "CronJobList"},
	{Group: "", Version: "v1", Kind: "PersistentVolumeClaimList"},
}

// deleteIfMarked deletes the live object the manifest names only when it
// carries this operator's mark. Absent is success; unmarked is left alone.
func (b *Base) deleteIfMarked(ctx context.Context, c client.Client, obj *unstructured.Unstructured) error {
	live := &unstructured.Unstructured{}
	live.SetGroupVersionKind(obj.GroupVersionKind())
	key := types.NamespacedName{Name: obj.GetName()}
	if isNamespaced(obj) {
		key.Namespace = obj.GetNamespace()
	}
	if err := c.Get(ctx, key, live); err != nil {
		if apierrors.IsNotFound(err) || meta.IsNoMatchError(err) {
			return nil
		}
		return fmt.Errorf("reading %s %s: %w", obj.GetKind(), key, err)
	}
	if live.GetLabels()[ManagedByLabel] != SSAFieldManager {
		logf.FromContext(ctx).Info("Left object without this operator's mark in place", "kind", obj.GetKind(), "name", key.String())
		return nil
	}
	if err := c.Delete(ctx, live); err != nil && !apierrors.IsNotFound(err) {
		return fmt.Errorf("deleting %s %s: %w", obj.GetKind(), key, err)
	}
	logf.FromContext(ctx).Info("Deleted sub-operator object", "kind", obj.GetKind(), "name", key.String())
	return nil
}

func objectName(kind, namespace, name string) string {
	if namespace == "" {
		return kind + " " + name
	}
	return kind + " " + namespace + "/" + name
}
