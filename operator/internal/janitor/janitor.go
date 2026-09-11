// Package janitor removes what the operator installed cluster-wide once nothing
// on the cluster needs it any more.
//
// The operator has no finalizers: deleting a PlantonPlatform garbage-collects
// everything the platform owns, and a platform deleted after the operator is
// uninstalled never hangs on a finalizer nobody will run. Two classes of object
// escape that contract because Kubernetes lets no namespaced owner collect a
// cluster-scoped dependent:
//
//   - a platform's own cluster-scoped satellites (today: the control plane's
//     token-reviewer ClusterRole and ClusterRoleBinding), which belong to one
//     platform and should leave when it leaves;
//   - the shared sub-operators (CloudNativePG, Tekton Pipelines) the first
//     platform on a cluster installs and every later platform reuses, which
//     should leave when the LAST platform leaves -- unless something else on
//     the cluster still runs on them.
//
// The janitor is the one place both are taken back. It runs when a platform
// vanishes (the reconcile of a deleted resource) and on a slow tick (so an
// operator restarted mid-sweep, or objects still draining through garbage
// collection, converge without anyone noticing), and it decides from the
// live cluster every time: which platforms exist, which satellites name a
// platform that does not, whether any object still uses a sub-operator. It
// deletes only what carries the operator's own mark, and when it keeps
// something it says why on the definition a person will look at.
package janitor

import (
	"context"
	"fmt"
	"sort"
	"time"

	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	v1 "github.com/plantonhq/planton/operator/api/v1"
	"github.com/plantonhq/planton/operator/internal/component"
	"github.com/plantonhq/planton/operator/internal/resources"
)

// DrainRequeue is how soon a sweep that found objects still draining through
// garbage collection asks to run again.
const DrainRequeue = 15 * time.Second

// Janitor sweeps the cluster-scoped objects the operator installed.
type Janitor struct {
	client.Client
	// Recorder writes the verdict on a kept sub-operator to its detect
	// definition, where a person running kubectl describe will read it.
	Recorder record.EventRecorder
	// SubOperators are the shared installs the janitor may remove; defaults
	// to component.SharedSubOperators().
	SubOperators []component.SubOperatorOptions

	base component.Base
}

// Outcome summarizes one sweep for the caller that decides whether to run
// again soon.
type Outcome struct {
	// PlatformsRemaining is how many PlantonPlatforms the cluster still has.
	PlatformsRemaining int
	// SatellitesRemoved counts stranded per-platform cluster objects deleted.
	SatellitesRemoved int
	// Verdicts holds every sub-operator's verdict, by LogName, when the sweep
	// reached them (no platform remained).
	Verdicts map[string]component.TeardownVerdict
	// Draining is true when at least one sub-operator is waiting on garbage
	// collection; the caller should sweep again after DrainRequeue.
	Draining bool
}

// The grants the sweep needs beyond what reconciling a platform already holds:
// deleting the definitions, admission webhooks, and cluster RBAC a vendored
// release installs; reading the instances of every kind a release defines (to
// know whether anything still uses it); and deleting exactly the namespaces
// the releases create -- pinned by name, because deleting an arbitrary
// namespace is a different blast radius from patching one. The backup plugin's
// release adds two kinds the others do not carry: its own ObjectStore
// definition (read for in-use stores) and the cert-manager Issuer and
// Certificates that mint its TLS pair (deleted with the release).
//
// +kubebuilder:rbac:groups=apiextensions.k8s.io,resources=customresourcedefinitions,verbs=delete
// +kubebuilder:rbac:groups=admissionregistration.k8s.io,resources=mutatingwebhookconfigurations;validatingwebhookconfigurations,verbs=delete
// +kubebuilder:rbac:groups=rbac.authorization.k8s.io,resources=clusterroles;clusterrolebindings,verbs=delete
// +kubebuilder:rbac:groups="",resources=namespaces,verbs=delete,resourceNames=cnpg-system;tekton-pipelines;tekton-pipelines-resolvers
// +kubebuilder:rbac:groups="",resources=serviceaccounts;configmaps;secrets;services,verbs=delete
// +kubebuilder:rbac:groups=apps,resources=deployments,verbs=delete
// +kubebuilder:rbac:groups=autoscaling,resources=horizontalpodautoscalers,verbs=delete
// +kubebuilder:rbac:groups=postgresql.cnpg.io,resources=*,verbs=get;list
// +kubebuilder:rbac:groups=barmancloud.cnpg.io,resources=*,verbs=get;list
// +kubebuilder:rbac:groups=cert-manager.io,resources=issuers;certificates,verbs=get;list;delete
// +kubebuilder:rbac:groups=tekton.dev,resources=*,verbs=get;list
// +kubebuilder:rbac:groups=resolution.tekton.dev,resources=*,verbs=get;list
// +kubebuilder:rbac:groups="",resources=events,verbs=create;patch
// +kubebuilder:rbac:groups=apps,resources=daemonsets,verbs=get;list
// +kubebuilder:rbac:groups=batch,resources=cronjobs,verbs=get;list
// +kubebuilder:rbac:groups=autoscaling,resources=horizontalpodautoscalers,verbs=get;list

// Sweep is the whole pass: list the platforms once; delete every satellite
// whose UID label names no live platform; then, only when no platform
// remains, take back each shared sub-operator that is ours and unused.
// Idempotent and safe to call from the reconcile and the tick at once -- the
// sub-operator seam holds its own mutex.
func (j *Janitor) Sweep(ctx context.Context) (Outcome, error) {
	log := logf.FromContext(ctx).WithName("janitor")

	var platforms v1.PlantonPlatformList
	if err := j.List(ctx, &platforms); err != nil {
		return Outcome{}, fmt.Errorf("listing PlantonPlatforms: %w", err)
	}
	live := make(map[types.UID]struct{}, len(platforms.Items))
	for i := range platforms.Items {
		live[platforms.Items[i].UID] = struct{}{}
	}
	outcome := Outcome{PlatformsRemaining: len(platforms.Items), Verdicts: map[string]component.TeardownVerdict{}}

	removed, err := j.removeStrandedSatellites(ctx, live)
	if err != nil {
		return outcome, err
	}
	outcome.SatellitesRemoved = removed

	if len(platforms.Items) > 0 {
		return outcome, nil
	}

	subOperators := j.SubOperators
	if subOperators == nil {
		subOperators = component.SharedSubOperators()
	}
	for _, opts := range subOperators {
		verdict, err := j.base.RemoveSubOperator(ctx, j.Client, opts, j.platformExists)
		if err != nil {
			return outcome, fmt.Errorf("removing %s: %w", opts.LogName, err)
		}
		outcome.Verdicts[opts.LogName] = verdict
		switch verdict.Outcome {
		case component.TeardownDraining:
			outcome.Draining = true
			log.Info(verdict.Explain(opts.LogName))
		case component.TeardownKeptInUse:
			log.Info(verdict.Explain(opts.LogName))
			j.recordOnDefinition(ctx, opts, "KeptInUse", verdict.Explain(opts.LogName))
		case component.TeardownRemoved:
			log.Info(verdict.Explain(opts.LogName))
		case component.TeardownKeptForeign:
			log.V(1).Info(verdict.Explain(opts.LogName))
		}
	}
	return outcome, nil
}

// removeStrandedSatellites deletes every token-reviewer ClusterRole and
// ClusterRoleBinding whose platform-uid label names no live platform. Objects
// without the label predate it and are left alone: the janitor never guesses
// ownership from a name.
func (j *Janitor) removeStrandedSatellites(ctx context.Context, live map[types.UID]struct{}) (int, error) {
	log := logf.FromContext(ctx).WithName("janitor")
	selector := client.HasLabels{resources.PlatformUIDLabel}
	removed := 0

	var bindings rbacv1.ClusterRoleBindingList
	if err := j.List(ctx, &bindings, selector); err != nil {
		return 0, fmt.Errorf("listing ClusterRoleBindings: %w", err)
	}
	for i := range bindings.Items {
		b := &bindings.Items[i]
		if _, ok := live[types.UID(b.Labels[resources.PlatformUIDLabel])]; ok {
			continue
		}
		if err := j.Delete(ctx, b); err != nil && !apierrors.IsNotFound(err) {
			return removed, fmt.Errorf("deleting ClusterRoleBinding %s: %w", b.Name, err)
		}
		log.Info("Deleted stranded ClusterRoleBinding", "name", b.Name, "platformUID", b.Labels[resources.PlatformUIDLabel])
		removed++
	}

	var roles rbacv1.ClusterRoleList
	if err := j.List(ctx, &roles, selector); err != nil {
		return removed, fmt.Errorf("listing ClusterRoles: %w", err)
	}
	for i := range roles.Items {
		r := &roles.Items[i]
		if _, ok := live[types.UID(r.Labels[resources.PlatformUIDLabel])]; ok {
			continue
		}
		if err := j.Delete(ctx, r); err != nil && !apierrors.IsNotFound(err) {
			return removed, fmt.Errorf("deleting ClusterRole %s: %w", r.Name, err)
		}
		log.Info("Deleted stranded ClusterRole", "name", r.Name, "platformUID", r.Labels[resources.PlatformUIDLabel])
		removed++
	}
	return removed, nil
}

// platformExists answers RemoveSubOperator's question about an instance's
// owner: does a PlantonPlatform with this UID still exist?
func (j *Janitor) platformExists(ctx context.Context, namespace, name string, uid types.UID) (bool, error) {
	var platform v1.PlantonPlatform
	if err := j.Get(ctx, types.NamespacedName{Namespace: namespace, Name: name}, &platform); err != nil {
		if apierrors.IsNotFound(err) {
			return false, nil
		}
		return false, fmt.Errorf("reading PlantonPlatform %s/%s: %w", namespace, name, err)
	}
	return platform.UID == uid, nil
}

// recordOnDefinition leaves the verdict as a Warning Event on the
// sub-operator's detect definition -- the object a person inspects when
// wondering why the operator left CloudNativePG or Tekton behind.
func (j *Janitor) recordOnDefinition(ctx context.Context, opts component.SubOperatorOptions, reason, message string) {
	if j.Recorder == nil {
		return
	}
	crd := &unstructured.Unstructured{}
	crd.SetGroupVersionKind(schema.GroupVersionKind{Group: "apiextensions.k8s.io", Version: "v1", Kind: "CustomResourceDefinition"})
	if err := j.Get(ctx, types.NamespacedName{Name: opts.CRDName}, crd); err != nil {
		return
	}
	j.Recorder.Event(crd, corev1.EventTypeWarning, reason, message)
}

// SortedVerdictNames returns the sub-operator names of an outcome in a
// stable order, for logs and tests.
func (o Outcome) SortedVerdictNames() []string {
	names := make([]string, 0, len(o.Verdicts))
	for name := range o.Verdicts {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}
