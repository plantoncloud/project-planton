package status

import (
	"fmt"
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
	// And for GitHub: WHICH hosts are declared and which carry an install
	// App, never whether GitHub accepts the App (the control plane learns
	// that at connection time and says so in the wizard).
	if echo := githubEcho(planton); planton.Status.Github != echo {
		planton.Status.Github = echo
		changed = true
	}

	if planton.Status.Components.PostgreSQL == nil {
		statuses := v1.ComponentStatuses{
			PostgreSQL:   &v1.ComponentStatus{Phase: v1.ComponentPhasePending},
			Redis:        &v1.ComponentStatus{Phase: v1.ComponentPhasePending},
			OpenFGA:      &v1.ComponentStatus{Phase: v1.ComponentPhasePending},
			Temporal:     &v1.ComponentStatus{Phase: v1.ComponentPhasePending},
			ControlPlane: &v1.ComponentStatus{Phase: v1.ComponentPhasePending},
			Console:      &v1.ComponentStatus{Phase: v1.ComponentPhasePending},
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

	// The policy-engine slot is unconditional for the same reason: every
	// request the control plane serves is authorized by OpenFGA, so a platform
	// without it is unrepresentable. Backfilled (not only in the base block) so
	// a platform whose status was allocated without the slot picks it up on the
	// next reconcile instead of leaving the control plane waiting on a
	// dependency that never reports.
	if planton.Status.Components.OpenFGA == nil {
		planton.Status.Components.OpenFGA = &v1.ComponentStatus{Phase: v1.ComponentPhasePending}
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

	// The tekton slot follows the build capability, like the runner slot it
	// depends on.
	changed = syncToggledSlot(&planton.Status.Components.Tekton, isTektonEnabled(planton)) || changed

	if changed {
		SetCondition(planton, v1.ConditionReady, metav1.ConditionFalse,
			ReasonComponentsPending, "Components have not been deployed yet")
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

// githubEcho renders the declared GitHub hosts in declaration order, marking
// the ones that carry an install App: "github.example.com (App), github.com".
// NotConfigured when nothing is declared (the github.com defaults apply).
func githubEcho(planton *v1.PlantonPlatform) string {
	g := planton.Spec.Github
	if g == nil || len(g.Hosts) == 0 {
		return v1.GithubModeNotConfigured
	}
	parts := make([]string, 0, len(g.Hosts))
	for _, h := range g.Hosts {
		if h.App != nil {
			parts = append(parts, h.Host+" (App)")
		} else {
			parts = append(parts, h.Host)
		}
	}
	return strings.Join(parts, ", ")
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

// SetCondition sets a condition on the PlantonPlatform status. The condition
// records the generation it speaks about, so a reader can tell a verdict on
// the current spec from one left over from before an edit.
func SetCondition(planton *v1.PlantonPlatform, condType string, condStatus metav1.ConditionStatus, reason, message string) {
	meta.SetStatusCondition(&planton.Status.Conditions, metav1.Condition{
		Type:               condType,
		Status:             condStatus,
		ObservedGeneration: planton.Generation,
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

// ComponentState is what a reconcile learned about one component: the reason
// behind its phase, the object the reason is about (nil when it is about the
// component as a whole), and the sentence a person reads.
type ComponentState struct {
	Reason  v1.ComponentReason
	Object  *v1.ComponentObjectReference
	Message string
}

// SetComponentPhase records a component's phase and state. The component
// pointer must be non-nil (call Initialize first). An empty reason is
// normalized from the phase (Ready is Healthy, anything else is Deploying),
// so a component that never learned to name a reason still reports one.
// lastTransitionTime moves only when the phase, reason, or object changes --
// a message that ticks ("90s since start") is the same condition persisting,
// and a reconcile every thirty seconds must not make every component look
// freshly changed. Reports whether the phase, reason, or object moved, which
// is what the controller's Event emission keys on.
func SetComponentPhase(cs *v1.ComponentStatus, phase v1.ComponentPhase, state ComponentState) bool {
	if cs == nil {
		return false
	}
	reason := state.Reason
	if reason == "" {
		if phase == v1.ComponentPhaseReady {
			reason = v1.ComponentReasonHealthy
		} else {
			reason = v1.ComponentReasonDeploying
		}
	}
	transitioned := cs.Phase != phase || cs.Reason != reason || !sameObject(cs.Object, state.Object)
	cs.Phase = phase
	cs.Reason = reason
	cs.Object = state.Object
	cs.Message = state.Message
	if transitioned || cs.LastTransitionTime.IsZero() {
		cs.LastTransitionTime = metav1.Now()
	}
	return transitioned
}

func sameObject(a, b *v1.ComponentObjectReference) bool {
	if a == nil || b == nil {
		return a == b
	}
	return *a == *b
}

// Ready-condition reasons for the platform as a whole. When a component is
// the reason the platform is not Ready, the condition carries THAT
// component's reason instead, so the one-line answer names the cause.
const (
	ReasonAllComponentsReady = "AllComponentsReady"
	ReasonComponentsPending  = "ComponentsPending"
)

// UpdateReadyCondition sets the Ready condition from the component statuses.
// Ready is True only when every enabled component is Ready. Otherwise the
// condition speaks for the worst-off component -- the first in error, else
// the first with a failure reason, else the first still deploying -- with
// its reason and its own sentence prefixed by the component's key in
// status.components, so the MESSAGE column a person reads first names the
// component and says what is wrong, and the count of others still waiting.
func UpdateReadyCondition(planton *v1.PlantonPlatform) {
	switch ComputeOverallPhase(planton) {
	case v1.PhaseReady:
		SetCondition(planton, v1.ConditionReady, metav1.ConditionTrue,
			ReasonAllComponentsReady, "All enabled components are healthy")
		return
	}

	key, cs, others := worstOffComponent(planton)
	if cs == nil {
		SetCondition(planton, v1.ConditionReady, metav1.ConditionFalse,
			ReasonComponentsPending, "Components have not been deployed yet")
		return
	}
	message := key + ": " + cs.Message
	if cs.Message == "" {
		message = key + " is " + strings.ToLower(string(cs.Phase))
	}
	if others > 0 {
		message += fmt.Sprintf(" (%d more %s not ready yet)", others, plural(others, "component is", "components are"))
	}
	SetCondition(planton, v1.ConditionReady, metav1.ConditionFalse, string(cs.Reason), message)
}

// worstOffComponent picks the component the Ready condition speaks for and
// counts the other not-ready ones. Ties inside a tier resolve in the stable
// slot order, which follows the dependency graph from the data layer up, so
// the root of a cascade is named before the components waiting on it.
func worstOffComponent(planton *v1.PlantonPlatform) (key string, cs *v1.ComponentStatus, others int) {
	slots := componentSlots(planton)
	notReady := 0
	pick := func(match func(*v1.ComponentStatus) bool) {
		for _, slot := range slots {
			if slot.cs != nil && cs == nil && match(slot.cs) {
				key, cs = slot.key, slot.cs
			}
		}
	}
	for _, slot := range slots {
		if slot.cs != nil && slot.cs.Phase != v1.ComponentPhaseReady {
			notReady++
		}
	}
	pick(func(c *v1.ComponentStatus) bool { return c.Phase == v1.ComponentPhaseError })
	pick(func(c *v1.ComponentStatus) bool {
		return c.Phase != v1.ComponentPhaseReady && c.Reason.IsFailure()
	})
	pick(func(c *v1.ComponentStatus) bool { return c.Phase == v1.ComponentPhaseDeploying })
	pick(func(c *v1.ComponentStatus) bool { return c.Phase != v1.ComponentPhaseReady })
	if cs == nil {
		return "", nil, 0
	}
	return key, cs, notReady - 1
}

func plural(n int, one, many string) string {
	if n == 1 {
		return one
	}
	return many
}

// componentSlot pairs a status slot with its key in status.components -- the
// name a person finds in `kubectl get -o yaml`.
type componentSlot struct {
	key string
	cs  *v1.ComponentStatus
}

// componentSlots lists every slot in the stable order the phase computation
// and the Ready condition share: data layer first, then the platform
// services, then the front door and the clients that depend on them.
func componentSlots(planton *v1.PlantonPlatform) []componentSlot {
	c := &planton.Status.Components
	return []componentSlot{
		{"postgresql", c.PostgreSQL}, {"redis", c.Redis},
		{"openFGA", c.OpenFGA}, {"temporal", c.Temporal}, {"tekton", c.Tekton},
		{"openBao", c.OpenBAO}, {"neo4j", c.Neo4j},
		{"controlPlane", c.ControlPlane}, {"runner", c.Runner}, {"ingress", c.Ingress},
		{"gateway", c.Gateway}, {"identity", c.Identity}, {"console", c.Console},
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
	var phases []v1.ComponentPhase
	for _, slot := range componentSlots(planton) {
		if slot.cs != nil {
			phases = append(phases, slot.cs.Phase)
		}
	}
	return phases
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
