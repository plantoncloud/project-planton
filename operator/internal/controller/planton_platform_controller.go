/*
Copyright 2026.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package controller

import (
	"context"
	"time"

	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	plantonaiv1 "github.com/plantonhq/planton/operator/api/v1"
	"github.com/plantonhq/planton/operator/internal/component"
	"github.com/plantonhq/planton/operator/internal/janitor"
	"github.com/plantonhq/planton/operator/internal/platformversion"
	"github.com/plantonhq/planton/operator/internal/status"
)

const requeueInterval = 30 * time.Second

// PlantonPlatformReconciler reconciles a PlantonPlatform object.
type PlantonPlatformReconciler struct {
	client.Client
	Scheme *runtime.Scheme
	// Janitor takes back what the operator installed cluster-wide once a
	// platform is gone (its own satellites) or once no platform remains (the
	// shared sub-operators). Optional: nil skips the sweep, which only tests
	// that build a reconciler by hand rely on.
	Janitor *janitor.Janitor
}

// +kubebuilder:rbac:groups=planton.ai,resources=plantonplatforms,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=planton.ai,resources=plantonplatforms/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=planton.ai,resources=plantonplatforms/finalizers,verbs=update
// +kubebuilder:rbac:groups=planton.ai,resources=plantonidentityproviders,verbs=get;list;watch
// +kubebuilder:rbac:groups=planton.ai,resources=plantonidentityproviders/status,verbs=get;update;patch

// Reconcile moves the cluster state toward the desired state declared in the
// PlantonPlatform spec. It iterates all registered components, reconciling
// those whose dependencies are satisfied and skipping the rest.
func (r *PlantonPlatformReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := logf.FromContext(ctx)

	var planton plantonaiv1.PlantonPlatform
	if err := r.Get(ctx, req.NamespacedName, &planton); err != nil {
		if errors.IsNotFound(err) {
			// The platform's own objects are already on their way out through
			// owner-reference garbage collection (the operator has no
			// finalizers, so this branch is the whole deletion path). What GC
			// cannot reach -- the platform's cluster-scoped satellites and,
			// when this was the last platform, the shared sub-operators -- the
			// janitor takes back now, and asks to run again while anything is
			// still draining.
			return r.sweepAfterDeletion(ctx, req)
		}
		return ctrl.Result{}, err
	}

	log.Info("Reconciling PlantonPlatform",
		"name", planton.Name,
		"namespace", planton.Namespace,
		"version", planton.Spec.Version,
	)

	if status.Initialize(&planton) {
		if err := r.Status().Update(ctx, &planton); err != nil {
			return ctrl.Result{}, err
		}
		log.Info("Initialized status")
		return ctrl.Result{Requeue: true}, nil
	}

	// The declared version is judged before any component runs. A platform
	// this operator cannot run is refused whole -- nothing created, nothing
	// deleted, a running platform left exactly as it is -- and the reason is
	// written where the person will look. No requeue: there is nothing to
	// watch until the spec changes, and a spec change re-enqueues on its own.
	if verdict := platformversion.Check(planton.Spec.Version); !verdict.Supported {
		log.Info("Refusing to reconcile: platform version unsupported",
			"version", planton.Spec.Version,
			"minimumSupported", platformversion.MinimumSupported,
			"reason", verdict.Reason,
		)
		if status.RefuseVersion(&planton, verdict.Reason, verdict.Message) {
			if err := r.Status().Update(ctx, &planton); err != nil {
				return ctrl.Result{}, err
			}
		}
		return ctrl.Result{}, nil
	}
	status.SetCondition(&planton, plantonaiv1.ConditionVersionSupported, metav1.ConditionTrue,
		platformversion.ReasonSupported, "spec.version names a platform release this operator runs")

	components := component.All()

	for _, comp := range components {
		if !comp.IsEnabled(&planton) {
			continue
		}

		cs := component.StatusFor(&planton.Status.Components, comp.Name())
		if cs == nil {
			continue
		}

		ready, unreadyDep := component.DependenciesReady(&planton.Status.Components, comp.Dependencies(&planton))
		if !ready {
			status.SetComponentPhase(cs,
				plantonaiv1.ComponentPhasePending,
				"Waiting for dependency: "+unreadyDep)
			continue
		}

		result, err := comp.Reconcile(ctx, r.Client, r.Scheme, &planton)
		if err != nil {
			log.Error(err, "Component reconciliation failed", "component", comp.Name())
			status.SetComponentPhase(cs, plantonaiv1.ComponentPhaseError, err.Error())
			continue
		}

		if result.Ready {
			status.SetComponentPhase(cs, plantonaiv1.ComponentPhaseReady, result.Message)
		} else {
			status.SetComponentPhase(cs, plantonaiv1.ComponentPhaseDeploying, result.Message)
		}
	}

	overallPhase := status.ComputeOverallPhase(&planton)
	planton.Status.Phase = overallPhase
	status.UpdateReadyCondition(&planton)

	if err := r.Status().Update(ctx, &planton); err != nil {
		return ctrl.Result{}, err
	}

	log.Info("Reconciliation complete, requeuing",
		"phase", overallPhase,
		"interval", requeueInterval,
	)
	return ctrl.Result{RequeueAfter: requeueInterval}, nil
}

// sweepAfterDeletion runs the janitor for a platform that no longer exists
// and turns its outcome into the reconcile's answer: requeue while garbage
// collection is still draining an instance the teardown must wait for,
// otherwise done.
func (r *PlantonPlatformReconciler) sweepAfterDeletion(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := logf.FromContext(ctx)
	if r.Janitor == nil {
		log.Info("PlantonPlatform resource deleted, nothing to reconcile")
		return ctrl.Result{}, nil
	}
	outcome, err := r.Janitor.Sweep(ctx)
	if err != nil {
		return ctrl.Result{}, err
	}
	log.Info("PlantonPlatform resource deleted; swept what garbage collection cannot reach",
		"platform", req.String(),
		"platformsRemaining", outcome.PlatformsRemaining,
		"satellitesRemoved", outcome.SatellitesRemoved,
		"subOperators", outcome.SortedVerdictNames(),
	)
	if outcome.Draining {
		return ctrl.Result{RequeueAfter: janitor.DrainRequeue}, nil
	}
	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
//
// PlantonIdentityProvider deliberately has NO controller of its own: identity
// config is realm state the identity component owns, so a change to one
// simply re-enqueues the platform(s) it may bind to and rides the same
// component reconcile -- one loop, one cadence, no second writer.
func (r *PlantonPlatformReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&plantonaiv1.PlantonPlatform{}).
		Watches(&plantonaiv1.PlantonIdentityProvider{},
			handler.EnqueueRequestsFromMapFunc(r.platformsForIdentityProvider)).
		Named("plantonplatform").
		Complete(r)
}

// platformsForIdentityProvider maps an identity-provider event to the
// platform reconciles it may affect: every platform in the resource's
// namespace. Binding resolution (which platform it actually binds to, or an
// ambiguity verdict) is the identity component's job -- the mapping stays
// deliberately dumb so the resolution logic lives in exactly one place.
func (r *PlantonPlatformReconciler) platformsForIdentityProvider(ctx context.Context, obj client.Object) []reconcile.Request {
	var platforms plantonaiv1.PlantonPlatformList
	if err := r.List(ctx, &platforms, client.InNamespace(obj.GetNamespace())); err != nil {
		logf.FromContext(ctx).Error(err, "Failed to list PlantonPlatforms for identity-provider event",
			"namespace", obj.GetNamespace())
		return nil
	}
	requests := make([]reconcile.Request, 0, len(platforms.Items))
	for i := range platforms.Items {
		requests = append(requests, reconcile.Request{
			NamespacedName: client.ObjectKeyFromObject(&platforms.Items[i]),
		})
	}
	return requests
}
