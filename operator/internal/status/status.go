package status

import (
	"strings"

	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	v1 "github.com/plantonhq/planton/operator/api/v1"
)

// Initialize sets the status to Pending with all component statuses and the
// Ready condition initialized. It returns true if any changes were made, false
// if the status was already initialized.
func Initialize(planton *v1.PlantonPlatform) bool {
	changed := false

	if planton.Status.Phase == "" {
		planton.Status.Phase = v1.PhasePending
		changed = true
	}

	if planton.Status.Version != planton.Spec.Version {
		planton.Status.Version = planton.Spec.Version
		changed = true
	}

	// Configuration echo like version: HOW the key is delivered, never
	// whether it verified (live license state is the control plane's own
	// entitlements advertisement -- the operator has no channel to it and
	// must not guess).
	if mode := licenseMode(planton); planton.Status.License != mode {
		planton.Status.License = mode
		changed = true
	}

	// Same discipline for email: WHICH provider arm is declared, never
	// whether the relay accepts mail (the control plane checks that on
	// demand and reports each verdict in words -- the operator has no
	// channel to the relay and must not probe it).
	if mode := emailMode(planton); planton.Status.Email != mode {
		planton.Status.Email = mode
		changed = true
	}

	if planton.Status.Components.PostgreSQL == nil {
		statuses := v1.ComponentStatuses{
			PostgreSQL:   &v1.ComponentStatus{Phase: v1.ComponentPhasePending},
			Redis:        &v1.ComponentStatus{Phase: v1.ComponentPhasePending},
			Temporal:     &v1.ComponentStatus{Phase: v1.ComponentPhasePending},
			ControlPlane: &v1.ComponentStatus{Phase: v1.ComponentPhasePending},
			Console:      &v1.ComponentStatus{Phase: v1.ComponentPhasePending},
		}

		if isAuthorizationEnabled(planton) {
			statuses.OpenFGA = &v1.ComponentStatus{Phase: v1.ComponentPhasePending}
		}
		if isOpenBAOEnabled(planton) {
			statuses.OpenBAO = &v1.ComponentStatus{Phase: v1.ComponentPhasePending}
		}
		if isNeo4jEnabled(planton) {
			statuses.Neo4j = &v1.ComponentStatus{Phase: v1.ComponentPhasePending}
		}

		planton.Status.Components = statuses
		changed = true
	}

	// The identity slot is unconditional: every install carries the bundled
	// identity server (sign-in through the gateway's port-forward front door
	// or the ingress hostname). An unauthenticated platform is
	// unrepresentable. Backfilled here (not only in the base block above) so
	// pre-identity installs pick the slot up on upgrade.
	if planton.Status.Components.Identity == nil {
		planton.Status.Components.Identity = &v1.ComponentStatus{Phase: v1.ComponentPhasePending}
		changed = true
	}

	// Exactly one front door: the ingress and gateway slots follow the
	// ingress toggle in OPPOSITE directions, in both directions each, so the
	// front door can be switched on an already-running platform. The
	// advertised URL is retired with whichever front door owned it -- its
	// replacement republishes the new URL in the same pass.
	if isIngressEnabled(planton) {
		if planton.Status.Components.Ingress == nil {
			planton.Status.Components.Ingress = &v1.ComponentStatus{Phase: v1.ComponentPhasePending}
			changed = true
		}
		if planton.Status.Components.Gateway != nil {
			planton.Status.Components.Gateway = nil
			planton.Status.ConsoleURL = ""
			planton.Status.Reachability = ""
			changed = true
		}
	} else {
		if planton.Status.Components.Gateway == nil {
			planton.Status.Components.Gateway = &v1.ComponentStatus{Phase: v1.ComponentPhasePending}
			changed = true
		}
		if planton.Status.Components.Ingress != nil {
			planton.Status.Components.Ingress = nil
			planton.Status.ConsoleURL = ""
			planton.Status.Reachability = ""
			changed = true
		}
	}

	// The runner slot follows its toggle in both directions (like the front
	// door), so an install can opt out -- or back in -- on a running
	// platform. Backfilled (not only in the base block) so pre-runner
	// installs pick the slot up on upgrade.
	changed = syncToggledSlot(&planton.Status.Components.Runner, isRunnerEnabled(planton)) || changed

	// Optional component slots follow their toggles in both directions so a
	// running platform can opt in (or back out) without a reinstall. Without
	// this backfill, a vault arm enabled after the first reconcile (the lab's
	// negative-then-vault flow, or a GitOps patch) would leave control plane
	// waiting forever for an openbao slot that was never allocated.
	changed = syncToggledSlot(&planton.Status.Components.OpenBAO, isOpenBAOEnabled(planton)) || changed
	changed = syncToggledSlot(&planton.Status.Components.Neo4j, isNeo4jEnabled(planton)) || changed
	changed = syncToggledSlot(&planton.Status.Components.OpenFGA, isAuthorizationEnabled(planton)) || changed

	// The tekton slot follows the build capability, like the runner slot it
	// depends on.
	changed = syncToggledSlot(&planton.Status.Components.Tekton, isTektonEnabled(planton)) || changed

	if changed {
		SetCondition(planton, v1.ConditionReady, metav1.ConditionFalse,
			"Pending", "Components have not been deployed yet")
	}

	return changed
}

// licenseMode mirrors component.effectiveLicense's blank-tolerance (an empty
// block or blank key IS Community) so the column and the rendered Deployment
// cannot disagree about whether a key was delivered.
func licenseMode(planton *v1.PlantonPlatform) string {
	l := planton.Spec.License
	switch {
	case l == nil:
		return v1.LicenseModeCommunity
	case l.SecretKeyRef != nil:
		return v1.LicenseModeSecretRef
	case strings.TrimSpace(l.Key) != "":
		return v1.LicenseModeInlineKey
	default:
		return v1.LicenseModeCommunity
	}
}

// emailMode mirrors component.effectiveEmail's arm resolution so the column
// and the rendered Deployment cannot disagree about which provider was
// declared. A block with neither arm (unreachable through the API server) is
// NotConfigured, exactly as the component renders it.
func emailMode(planton *v1.PlantonPlatform) string {
	e := planton.Spec.Email
	switch {
	case e == nil:
		return v1.EmailModeNotConfigured
	case e.SMTP != nil:
		return v1.EmailModeSMTP
	case e.Resend != nil:
		return v1.EmailModeResend
	default:
		return v1.EmailModeNotConfigured
	}
}

// syncToggledSlot makes a component's status slot follow its toggle in both
// directions: allocated (Pending) when enabled, retired when not. Reports
// whether it changed anything.
func syncToggledSlot(slot **v1.ComponentStatus, enabled bool) bool {
	if enabled {
		if *slot == nil {
			*slot = &v1.ComponentStatus{Phase: v1.ComponentPhasePending}
			return true
		}
		return false
	}
	if *slot != nil {
		*slot = nil
		return true
	}
	return false
}

// SetCondition sets a condition on the PlantonPlatform status.
func SetCondition(planton *v1.PlantonPlatform, condType string, condStatus metav1.ConditionStatus, reason, message string) {
	meta.SetStatusCondition(&planton.Status.Conditions, metav1.Condition{
		Type:               condType,
		Status:             condStatus,
		Reason:             reason,
		Message:            message,
		LastTransitionTime: metav1.Now(),
	})
}

// ReasonPlatformVersionUnsupported is the Ready condition's reason when the
// declared version is refused; the VersionSupported condition carries the
// finer reason and the same message.
const ReasonPlatformVersionUnsupported = "PlatformVersionUnsupported"

// RefuseVersion records that this operator will not run the declared platform
// version: phase Error, VersionSupported False with the given reason, and
// Ready False with the same message, so the explanation reaches the column a
// person reads first. Component statuses are left as they are -- on a fresh
// install they are Pending and untouched; on a running platform that an
// operator upgrade has outgrown they keep reporting what is actually running.
// Reports whether anything changed, so a refusal that is already recorded
// costs no status write.
func RefuseVersion(planton *v1.PlantonPlatform, reason, message string) bool {
	changed := false
	if planton.Status.Phase != v1.PhaseError {
		planton.Status.Phase = v1.PhaseError
		changed = true
	}
	changed = setConditionIfDifferent(planton, v1.ConditionVersionSupported, metav1.ConditionFalse, reason, message) || changed
	changed = setConditionIfDifferent(planton, v1.ConditionReady, metav1.ConditionFalse, ReasonPlatformVersionUnsupported, message) || changed
	return changed
}

// setConditionIfDifferent is SetCondition that also reports whether the
// condition's status, reason, or message moved.
func setConditionIfDifferent(planton *v1.PlantonPlatform, condType string, condStatus metav1.ConditionStatus, reason, message string) bool {
	if existing := meta.FindStatusCondition(planton.Status.Conditions, condType); existing != nil &&
		existing.Status == condStatus && existing.Reason == reason && existing.Message == message {
		return false
	}
	SetCondition(planton, condType, condStatus, reason, message)
	return true
}

// SetComponentPhase sets the phase and message for a component. The component
// pointer must be non-nil (call Initialize first).
func SetComponentPhase(cs *v1.ComponentStatus, phase v1.ComponentPhase, message string) {
	if cs == nil {
		return
	}
	cs.Phase = phase
	cs.Message = message
}

// UpdateReadyCondition sets the Ready condition based on the current component
// statuses. Ready is True only when all enabled components are Ready.
func UpdateReadyCondition(planton *v1.PlantonPlatform) {
	overall := ComputeOverallPhase(planton)
	switch overall {
	case v1.PhaseReady:
		SetCondition(planton, v1.ConditionReady, metav1.ConditionTrue,
			"AllComponentsReady", "All enabled components are healthy")
	case v1.PhaseError:
		SetCondition(planton, v1.ConditionReady, metav1.ConditionFalse,
			"ComponentError", "One or more components are in error state")
	default:
		SetCondition(planton, v1.ConditionReady, metav1.ConditionFalse,
			"Deploying", "Components are being deployed")
	}
}

// ComputeOverallPhase determines the overall deployment phase from component
// statuses. Rules:
//   - Any component Error -> Error
//   - All components Ready -> Ready
//   - Any component Deploying -> Deploying
//   - Otherwise -> Pending
func ComputeOverallPhase(planton *v1.PlantonPlatform) v1.PlantonPhase {
	components := allComponentPhases(planton)

	allReady := true
	anyError := false
	anyDeploying := false

	for _, phase := range components {
		switch phase {
		case v1.ComponentPhaseError:
			anyError = true
		case v1.ComponentPhaseDeploying:
			anyDeploying = true
			allReady = false
		case v1.ComponentPhaseReady:
			// keep going
		default:
			allReady = false
		}
	}

	switch {
	case anyError:
		return v1.PhaseError
	case allReady:
		return v1.PhaseReady
	case anyDeploying:
		return v1.PhaseDeploying
	default:
		return v1.PhasePending
	}
}

func allComponentPhases(planton *v1.PlantonPlatform) []v1.ComponentPhase {
	c := &planton.Status.Components
	var phases []v1.ComponentPhase
	for _, cs := range []*v1.ComponentStatus{
		c.PostgreSQL, c.Redis,
		c.OpenFGA, c.Temporal, c.Tekton,
		c.OpenBAO, c.Neo4j,
		c.ControlPlane, c.Runner, c.Ingress, c.Gateway, c.Identity, c.Console,
	} {
		if cs != nil {
			phases = append(phases, cs.Phase)
		}
	}
	return phases
}

func isAuthorizationEnabled(planton *v1.PlantonPlatform) bool {
	return planton.Spec.Components != nil &&
		planton.Spec.Components.Authorization != nil &&
		planton.Spec.Components.Authorization.Enabled
}

// isOpenBAOEnabled defaults to true: the bundled secrets manager is integral
// (credential store, envelope-encryption KEK, OIDC signing key), so absence of
// spec.vault means deploy it. Must agree with the component package's answer
// or the slot and the reconciler disagree about existence.
func isOpenBAOEnabled(planton *v1.PlantonPlatform) bool {
	return planton.Spec.Vault == nil || planton.Spec.Vault.Enabled == nil ||
		*planton.Spec.Vault.Enabled
}

func isNeo4jEnabled(planton *v1.PlantonPlatform) bool {
	return planton.Spec.Components != nil &&
		planton.Spec.Components.Graph != nil &&
		planton.Spec.Components.Graph.Enabled
}

func isIngressEnabled(planton *v1.PlantonPlatform) bool {
	return planton.Spec.Ingress != nil && planton.Spec.Ingress.Enabled
}

// isRunnerEnabled defaults to true: an install that cannot deploy
// infrastructure is a browsing UI. Must agree with the component package's
// answer or the slot and the reconciler disagree about existence.
func isRunnerEnabled(planton *v1.PlantonPlatform) bool {
	return planton.Spec.Runner == nil || planton.Spec.Runner.Enabled == nil ||
		*planton.Spec.Runner.Enabled
}

// isTektonEnabled mirrors the tekton component's IsEnabled: the build
// capability defaults to true (builds power Service Hub); with the runner
// disabled the slot exists only for an EXPLICIT spec.build.enabled=true,
// whose contradiction the component reports as an error. Must agree with the
// component package's answer or the slot and the reconciler disagree about
// existence.
func isTektonEnabled(planton *v1.PlantonPlatform) bool {
	buildEnabled := planton.Spec.Build == nil || planton.Spec.Build.Enabled == nil ||
		*planton.Spec.Build.Enabled
	buildExplicit := planton.Spec.Build != nil && planton.Spec.Build.Enabled != nil &&
		*planton.Spec.Build.Enabled
	return buildEnabled && (isRunnerEnabled(planton) || buildExplicit)
}
