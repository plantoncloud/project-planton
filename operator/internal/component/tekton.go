package component

import (
	"context"
	"fmt"

	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	v1 "github.com/plantonhq/planton/operator/api/v1"
	"github.com/plantonhq/planton/operator/internal/resources"
)

// Tekton installs and wires the build engine behind spec.build: Tekton
// Pipelines itself (vendored upstream release, or detected when the cluster
// already runs it), and Tekton's cluster-wide CloudEvents sink pointed at the
// runner's webhook so live build status flows without the reconciliation
// safety net. The component is named for the TOOL it deploys while the spec
// toggle is named for the CAPABILITY (spec.build) -- the same split as
// authorization/OpenFGA.
//
// The runner half of the capability (build worker env, webhook exposure,
// build RBAC) lives with the runner component; the seeded build routing lives
// with the control plane's env. All three read the same effective-build
// predicate, so the capability ships whole or not at all.
//
// Deliberately NOT undone on opt-out: disabling spec.build later stops the
// runner's build worker and retires the webhook Service port, but Tekton
// stays installed -- uninstalling a cluster-wide engine that may run
// workloads this operator never created is a human's call, never a reconcile
// side effect. It leaves only when the last platform leaves the cluster AND
// no Tekton object of any kind remains -- the janitor's rule, which turns
// that human's call into a fact the cluster states.
type Tekton struct{ Base }

// TektonPipelinesSubOperator is the one definition of the Tekton install this
// operator manages, read by the install gate and by the janitor alike (see
// CloudNativePGSubOperator for why there is exactly one).
func TektonPipelinesSubOperator() SubOperatorOptions {
	return SubOperatorOptions{
		LogName:   "tekton-pipelines",
		CRDName:   resources.TektonPipelineRunCRDName,
		Loader:    resources.LoadTektonPipelinesManifests,
		Namespace: resources.TektonPipelinesNamespace,
		Deployments: []string{
			resources.TektonControllerDeploymentName,
			resources.TektonWebhookDeploymentName,
		},
	}
}

// SharedSubOperators lists every vendored sub-operator the janitor may take
// back off the cluster once no platform remains -- the same definitions the
// install gate deploys, so nothing can be installed that cannot be removed.
func SharedSubOperators() []SubOperatorOptions {
	return []SubOperatorOptions{CloudNativePGSubOperator(), TektonPipelinesSubOperator()}
}

func (t *Tekton) Name() string { return "tekton" }

func (t *Tekton) Dependencies(_ *v1.PlantonPlatform) []string {
	// None: the install touches only Tekton's own namespaces, and the sink
	// URL is composed from deterministic names, so it can be written before
	// the runner Service exists. Tekton converging in parallel with the data
	// layer keeps first boot fast.
	return nil
}

// IsEnabled follows the effective build toggle, with one deliberate
// exception: an EXPLICIT spec.build.enabled=true with the runner disabled
// keeps the component enabled so Reconcile can report the contradiction as a
// component error -- silently skipping a stated intent would hide a broken
// promise. A DEFAULT build toggle with the runner disabled follows the runner
// off quietly: disabling the runner was the explicit act, and builds cannot
// exist without it.
func (t *Tekton) IsEnabled(planton *v1.PlantonPlatform) bool {
	return isBuildEnabled(planton) && (isRunnerEnabled(planton) || isBuildExplicitlyEnabled(planton))
}

func (t *Tekton) Reconcile(ctx context.Context, c client.Client, _ *runtime.Scheme, planton *v1.PlantonPlatform) (Result, error) {
	log := logf.FromContext(ctx).WithValues("component", t.Name())

	if !isRunnerEnabled(planton) {
		// Only reachable with an explicit spec.build.enabled=true (see
		// IsEnabled): name the contradiction and the two ways out.
		return Result{}, fmt.Errorf(
			"spec.build.enabled is true but spec.runner is disabled -- builds execute on the runner; " +
				"enable the runner or remove spec.build.enabled")
	}

	// EnsureSubOperator honors the skip switch, detects the PipelineRun CRD,
	// applies the vendored release only when absent, and resumes a partial
	// install whose controller Deployments never landed. Its
	// never-re-apply-a-COMPLETE-install guarantee is load-bearing here beyond
	// idempotency: the release apply and the sink write must never share
	// field ownership of config-defaults. In the one case where the release
	// IS re-applied (resuming a partial install claws config-defaults back),
	// the sink write below runs in the same pass and restores the key under
	// its own field manager.
	subOperator := TektonPipelinesSubOperator()
	subOperator.SkipRequested = planton.Spec.Prerequisites != nil &&
		planton.Spec.Prerequisites.TektonPipelines == PrerequisiteSkip
	ready, err := t.EnsureSubOperator(ctx, c, subOperator)
	if err != nil {
		return Result{}, err
	}

	// The sink write is unconditional and re-applied every reconcile (drift
	// heals), under its OWN field manager so the install apply can never claw
	// the key back (see TektonEventsSinkFieldManager). It happens even while
	// the install is still converging -- and in the skip case, where the
	// namespace belongs to the adopter's own Tekton -- because sink wiring is
	// the build capability's job regardless of who installed Tekton.
	if err := t.ensureEventsSink(ctx, c, planton); err != nil {
		return Result{}, err
	}

	if !ready {
		return Result{Ready: false, Message: "Deploying Tekton Pipelines"}, nil
	}

	log.Info("Tekton ready")
	return Result{Ready: true,
		Message: "Tekton Pipelines healthy; build events flow to the runner webhook"}, nil
}

// ensureEventsSink points Tekton's cluster-wide CloudEvents sink at the
// runner's webhook and grants the runner read access to that configuration
// (its readiness probe grades the sink; without the grant it can only report
// "could not determine").
func (t *Tekton) ensureEventsSink(ctx context.Context, c client.Client, planton *v1.PlantonPlatform) error {
	// The skip case is the one path where Tekton's namespace may not exist
	// yet (the adopter promised an install the operator does not manage);
	// creating it early is harmless and lets the sink land the moment Tekton
	// does.
	if err := t.EnsureNamespace(ctx, c, resources.TektonPipelinesNamespace); err != nil {
		return fmt.Errorf("ensuring Tekton namespace: %w", err)
	}

	sinkURL := resources.TektonEventsSinkURL(planton.Name, planton.Namespace)
	fragment := resources.TektonConfigDefaultsSinkFragment(sinkURL)
	opts := []client.PatchOption{
		client.ForceOwnership,
		client.FieldOwner(resources.TektonEventsSinkFieldManager),
	}
	if err := c.Patch(ctx, fragment, client.Apply, opts...); err != nil {
		return fmt.Errorf("writing Tekton CloudEvents sink: %w", err)
	}

	if err := t.ApplyTypedObject(ctx, c,
		resources.TektonEventsSinkReadRole(planton.Name, planton.Namespace)); err != nil {
		return fmt.Errorf("applying events-sink read Role: %w", err)
	}
	if err := t.ApplyTypedObject(ctx, c,
		resources.TektonEventsSinkReadRoleBinding(planton.Name, planton.Namespace)); err != nil {
		return fmt.Errorf("applying events-sink read RoleBinding: %w", err)
	}
	return nil
}

// isBuildEnabled defaults to true: builds power Service Hub -- an install
// without them can deploy infrastructure but never build a service from
// source, which is half the product. Opting out is the explicit act. Must
// agree with the status package's answer or the slot and the reconciler
// disagree about existence.
func isBuildEnabled(p *v1.PlantonPlatform) bool {
	return p.Spec.Build == nil || p.Spec.Build.Enabled == nil || *p.Spec.Build.Enabled
}

// isBuildExplicitlyEnabled is true only when a human wrote
// spec.build.enabled=true -- the pointer-bool is what lets a stated intent be
// told apart from the default, so a contradiction with a disabled runner can
// be an error instead of a silent skip.
func isBuildExplicitlyEnabled(p *v1.PlantonPlatform) bool {
	return p.Spec.Build != nil && p.Spec.Build.Enabled != nil && *p.Spec.Build.Enabled
}

// isBuildEffective is the one answer every consumer of "do builds run here?"
// reads: the runner env flip, the build RBAC, the control plane's seeded
// build routing, and the status slot. Builds require the runner -- the build
// worker is a capability OF the runner, not a sibling process.
func isBuildEffective(p *v1.PlantonPlatform) bool {
	return isBuildEnabled(p) && isRunnerEnabled(p)
}

// RBAC markers for the Tekton Pipelines install (the vendored release carries
// webhook configurations, an HPA, and its own Namespace objects on top of the
// kinds other components already apply).
// +kubebuilder:rbac:groups=admissionregistration.k8s.io,resources=mutatingwebhookconfigurations;validatingwebhookconfigurations,verbs=get;list;watch;create;update;patch
// +kubebuilder:rbac:groups=autoscaling,resources=horizontalpodautoscalers,verbs=get;list;watch;create;update;patch
//
// Namespaces need patch/update on top of the create other components hold:
// this component applies the vendored release via server-side apply (a
// PATCH), and Tekton's release.yaml carries the tekton-pipelines and
// tekton-pipelines-resolvers Namespace objects. Without patch, the install
// fails on its very first object in a real deployment -- envtest never
// catches it because it does not enforce RBAC.
// +kubebuilder:rbac:groups="",resources=namespaces,verbs=get;list;watch;create;update;patch
//
// The escalate/bind grants exist because this operator is an INSTALLER of
// sub-systems that carry their own RBAC (Tekton's controller ClusterRoles,
// the CloudNativePG operator's roles, the runner's build Role):
// without escalate, Kubernetes' escalation prevention rejects granting any
// permission the operator does not itself hold, which would force this role
// to mirror the union of every vendored manifest's grants -- brittle at every
// version bump and strictly broader than escalate in practice.
// +kubebuilder:rbac:groups=rbac.authorization.k8s.io,resources=clusterroles;roles,verbs=escalate;bind
