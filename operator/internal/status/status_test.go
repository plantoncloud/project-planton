package status

import (
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	v1 "github.com/plantonhq/planton/operator/api/v1"
)

func newMinimalPlanton() *v1.PlantonPlatform {
	return &v1.PlantonPlatform{
		Spec: v1.PlantonPlatformSpec{
			Version: "v1.0.0",
		},
	}
}

func TestInitialize_SetsPhaseAndVersion(t *testing.T) {
	p := newMinimalPlanton()
	changed := Initialize(p)

	if !changed {
		t.Fatal("expected Initialize to report changes on a fresh resource")
	}
	if p.Status.Phase != v1.PhasePending {
		t.Errorf("expected phase Pending, got %s", p.Status.Phase)
	}
	if p.Status.Version != "v1.0.0" {
		t.Errorf("expected version v1.0.0, got %s", p.Status.Version)
	}
}

func TestInitialize_SetsAllComponents(t *testing.T) {
	p := newMinimalPlanton()
	Initialize(p)

	components := []*v1.ComponentStatus{
		p.Status.Components.PostgreSQL,
		p.Status.Components.Redis,
		p.Status.Components.Temporal,
		p.Status.Components.ControlPlane,
		p.Status.Components.Console,
	}

	for _, cs := range components {
		if cs == nil {
			t.Fatal("expected all core component statuses to be initialized")
		}
		if cs.Phase != v1.ComponentPhasePending {
			t.Errorf("expected component phase Pending, got %s", cs.Phase)
		}
	}
}

func TestInitialize_SetsReadyCondition(t *testing.T) {
	p := newMinimalPlanton()
	Initialize(p)

	if len(p.Status.Conditions) != 1 {
		t.Fatalf("expected 1 condition, got %d", len(p.Status.Conditions))
	}

	c := p.Status.Conditions[0]
	if c.Type != v1.ConditionReady {
		t.Errorf("expected condition type Ready, got %s", c.Type)
	}
	if c.Status != metav1.ConditionFalse {
		t.Errorf("expected condition status False, got %s", c.Status)
	}
}

func TestInitialize_Idempotent(t *testing.T) {
	p := newMinimalPlanton()
	Initialize(p)

	changed := Initialize(p)
	if changed {
		t.Error("expected Initialize to be a no-op on an already-initialized resource")
	}
}

func TestInitialize_UpdatesVersion(t *testing.T) {
	p := newMinimalPlanton()
	Initialize(p)

	p.Spec.Version = "v2.0.0"
	changed := Initialize(p)
	if !changed {
		t.Fatal("expected Initialize to detect version change")
	}
	if p.Status.Version != "v2.0.0" {
		t.Errorf("expected version v2.0.0, got %s", p.Status.Version)
	}
}

// The email column follows the spec in both directions and speaks the
// license column's grammar: which arm is declared, never whether the relay
// accepts mail.
func TestInitialize_EmailColumnFollowsSpec(t *testing.T) {
	p := newMinimalPlanton()
	Initialize(p)
	if p.Status.Email != v1.EmailModeNotConfigured {
		t.Errorf("email = %q, want NotConfigured on an install with no spec.email", p.Status.Email)
	}

	p.Spec.Email = &v1.EmailSpec{
		From: v1.EmailFromSpec{Address: "no-reply@planton.acme.com"},
		SMTP: &v1.EmailSMTPSpec{Host: "smtp.office365.com", Port: 587},
	}
	if changed := Initialize(p); !changed {
		t.Fatal("expected Initialize to detect the declared email")
	}
	if p.Status.Email != v1.EmailModeSMTP {
		t.Errorf("email = %q, want SMTP", p.Status.Email)
	}

	p.Spec.Email = &v1.EmailSpec{
		From:   v1.EmailFromSpec{Address: "no-reply@planton.acme.com"},
		Resend: &v1.EmailResendSpec{APIKeySecretRef: v1.SecretKeyRef{Name: "planton-email", Key: "api-key"}},
	}
	Initialize(p)
	if p.Status.Email != v1.EmailModeResend {
		t.Errorf("email = %q, want Resend", p.Status.Email)
	}

	p.Spec.Email = nil
	if changed := Initialize(p); !changed {
		t.Fatal("expected Initialize to detect the removed email")
	}
	if p.Status.Email != v1.EmailModeNotConfigured {
		t.Errorf("email = %q, want NotConfigured after removal", p.Status.Email)
	}
}

// The license column follows the spec in both directions (a key can be added
// to or removed from a running install), and blank-tolerance matches
// effectiveLicense so the column never claims a key the Deployment does not
// carry.
func TestInitialize_LicenseColumnFollowsSpec(t *testing.T) {
	p := newMinimalPlanton()
	Initialize(p)
	if p.Status.License != v1.LicenseModeCommunity {
		t.Errorf("license = %q, want Community on a licenseless install", p.Status.License)
	}

	p.Spec.License = &v1.LicenseSpec{Key: "plk1.1.c.s"}
	if changed := Initialize(p); !changed {
		t.Fatal("expected Initialize to detect the added license key")
	}
	if p.Status.License != v1.LicenseModeInlineKey {
		t.Errorf("license = %q, want InlineKey", p.Status.License)
	}

	p.Spec.License = &v1.LicenseSpec{
		SecretKeyRef: &v1.SecretKeyRef{Name: "acme-license", Key: "license-key"},
	}
	Initialize(p)
	if p.Status.License != v1.LicenseModeSecretRef {
		t.Errorf("license = %q, want SecretRef", p.Status.License)
	}

	// A declared-but-blank key is Community -- the same answer
	// effectiveLicense renders (no env var).
	p.Spec.License = &v1.LicenseSpec{Key: "  "}
	Initialize(p)
	if p.Status.License != v1.LicenseModeCommunity {
		t.Errorf("license = %q, want Community for a blank key", p.Status.License)
	}
}

func TestInitialize_OptionalComponents(t *testing.T) {
	p := &v1.PlantonPlatform{
		Spec: v1.PlantonPlatformSpec{
			Version: "v1.0.0",
			Components: &v1.ComponentsSpec{
				Authorization: &v1.ComponentToggle{Enabled: true},
				Graph:         &v1.Neo4jSpec{Enabled: true},
			},
		},
	}
	Initialize(p)

	if p.Status.Components.OpenFGA == nil {
		t.Error("expected OpenFGA status to be initialized when authorization is enabled")
	}
	if p.Status.Components.Neo4j == nil {
		t.Error("expected Neo4j status to be initialized when enabled")
	}
}

func TestInitialize_OptionalComponentsDisabled(t *testing.T) {
	p := newMinimalPlanton()
	Initialize(p)

	if p.Status.Components.OpenFGA != nil {
		t.Error("expected OpenFGA status to be nil when authorization is disabled")
	}
	if p.Status.Components.Neo4j != nil {
		t.Error("expected Neo4j status to be nil when disabled")
	}
}

func TestComputeOverallPhase_AllPending(t *testing.T) {
	p := newMinimalPlanton()
	Initialize(p)

	phase := ComputeOverallPhase(p)
	if phase != v1.PhasePending {
		t.Errorf("expected Pending, got %s", phase)
	}
}

func TestComputeOverallPhase_AllReady(t *testing.T) {
	p := newMinimalPlanton()
	Initialize(p)

	for _, cs := range []*v1.ComponentStatus{
		p.Status.Components.PostgreSQL,
		p.Status.Components.Redis, p.Status.Components.Temporal,
		p.Status.Components.ControlPlane, p.Status.Components.Console,
		// The minimal footprint now includes the front-door gateway and the
		// identity server -- sign-in is unconditional -- plus the in-cluster
		// runner, the bundled secrets manager, and the build engine
		// (Tekton), all on by default.
		p.Status.Components.Gateway, p.Status.Components.Identity,
		p.Status.Components.Runner, p.Status.Components.OpenBAO,
		p.Status.Components.Tekton,
	} {
		cs.Phase = v1.ComponentPhaseReady
	}

	phase := ComputeOverallPhase(p)
	if phase != v1.PhaseReady {
		t.Errorf("expected Ready, got %s", phase)
	}
}

func TestComputeOverallPhase_AnyDeploying(t *testing.T) {
	p := newMinimalPlanton()
	Initialize(p)
	p.Status.Components.PostgreSQL.Phase = v1.ComponentPhaseReady
	p.Status.Components.Redis.Phase = v1.ComponentPhaseDeploying

	phase := ComputeOverallPhase(p)
	if phase != v1.PhaseDeploying {
		t.Errorf("expected Deploying, got %s", phase)
	}
}

func TestComputeOverallPhase_AnyError(t *testing.T) {
	p := newMinimalPlanton()
	Initialize(p)
	p.Status.Components.PostgreSQL.Phase = v1.ComponentPhaseReady
	p.Status.Components.Redis.Phase = v1.ComponentPhaseError

	phase := ComputeOverallPhase(p)
	if phase != v1.PhaseError {
		t.Errorf("expected Error, got %s", phase)
	}
}

func TestComputeOverallPhase_ErrorTakesPrecedence(t *testing.T) {
	p := newMinimalPlanton()
	Initialize(p)
	p.Status.Components.PostgreSQL.Phase = v1.ComponentPhaseDeploying
	p.Status.Components.Redis.Phase = v1.ComponentPhaseError

	phase := ComputeOverallPhase(p)
	if phase != v1.PhaseError {
		t.Errorf("expected Error to take precedence over Deploying, got %s", phase)
	}
}

func TestSetComponentPhase(t *testing.T) {
	cs := &v1.ComponentStatus{Phase: v1.ComponentPhasePending}
	SetComponentPhase(cs, v1.ComponentPhaseDeploying, "Creating resources")

	if cs.Phase != v1.ComponentPhaseDeploying {
		t.Errorf("expected Deploying, got %s", cs.Phase)
	}
	if cs.Message != "Creating resources" {
		t.Errorf("expected message 'Creating resources', got %s", cs.Message)
	}
}

func TestSetComponentPhase_NilSafe(t *testing.T) {
	SetComponentPhase(nil, v1.ComponentPhaseDeploying, "should not panic")
}

func TestSetCondition(t *testing.T) {
	p := newMinimalPlanton()
	SetCondition(p, v1.ConditionReady, metav1.ConditionTrue, "AllReady", "All components are ready")

	if len(p.Status.Conditions) != 1 {
		t.Fatalf("expected 1 condition, got %d", len(p.Status.Conditions))
	}
	c := p.Status.Conditions[0]
	if c.Type != v1.ConditionReady {
		t.Errorf("expected type Ready, got %s", c.Type)
	}
	if c.Status != metav1.ConditionTrue {
		t.Errorf("expected status True, got %s", c.Status)
	}
	if c.Reason != "AllReady" {
		t.Errorf("expected reason AllReady, got %s", c.Reason)
	}
}

func TestUpdateReadyCondition_AllReady(t *testing.T) {
	p := newMinimalPlanton()
	Initialize(p)

	for _, cs := range []*v1.ComponentStatus{
		p.Status.Components.PostgreSQL,
		p.Status.Components.Redis, p.Status.Components.Temporal,
		p.Status.Components.ControlPlane, p.Status.Components.Console,
		p.Status.Components.Gateway, p.Status.Components.Identity,
		p.Status.Components.Runner, p.Status.Components.OpenBAO,
		p.Status.Components.Tekton,
	} {
		cs.Phase = v1.ComponentPhaseReady
	}

	UpdateReadyCondition(p)

	found := false
	for _, c := range p.Status.Conditions {
		if c.Type == v1.ConditionReady {
			found = true
			if c.Status != metav1.ConditionTrue {
				t.Errorf("expected Ready=True, got %s", c.Status)
			}
		}
	}
	if !found {
		t.Error("expected Ready condition to exist")
	}
}

func TestUpdateReadyCondition_NotReady(t *testing.T) {
	p := newMinimalPlanton()
	Initialize(p)

	UpdateReadyCondition(p)

	for _, c := range p.Status.Conditions {
		if c.Type == v1.ConditionReady {
			if c.Status != metav1.ConditionFalse {
				t.Errorf("expected Ready=False, got %s", c.Status)
			}
			return
		}
	}
	t.Error("expected Ready condition to exist")
}

// Exactly one front door: the ingress and gateway slots follow the ingress
// toggle in opposite directions, in both directions each, so the front door
// can be switched on an already-running platform. The advertised URL retires
// with whichever front door owned it.
func TestInitialize_FrontDoorSlotsFollowToggle(t *testing.T) {
	p := newMinimalPlanton()
	Initialize(p)
	if p.Status.Components.Ingress != nil {
		t.Fatal("ingress slot must be nil while spec.ingress is unset")
	}
	if p.Status.Components.Gateway == nil || p.Status.Components.Gateway.Phase != v1.ComponentPhasePending {
		t.Fatal("gateway slot must exist while ingress is off -- it IS the front door")
	}

	// Switch the front door to ingress: the gateway retires with its
	// localhost URL; the ingress slot backfills.
	p.Status.ConsoleURL = "http://localhost:8080"
	p.Spec.Ingress = &v1.IngressSpec{Enabled: true}
	if !Initialize(p) {
		t.Fatal("expected Initialize to switch the front-door slots when ingress is enabled")
	}
	if p.Status.Components.Ingress == nil || p.Status.Components.Ingress.Phase != v1.ComponentPhasePending {
		t.Fatal("expected a Pending ingress slot after enabling")
	}
	if p.Status.Components.Gateway != nil {
		t.Error("expected the gateway slot to retire when ingress takes over")
	}
	if p.Status.ConsoleURL != "" {
		t.Error("expected the gateway's localhost URL to be cleared on the switch")
	}

	// Switch back: ingress retires with its public URL; the gateway returns.
	p.Status.ConsoleURL = "http://planton.203-0-113-7.sslip.io"
	p.Spec.Ingress.Enabled = false
	if !Initialize(p) {
		t.Fatal("expected Initialize to switch the front-door slots when ingress is disabled")
	}
	if p.Status.Components.Ingress != nil {
		t.Error("expected the ingress slot to be removed after disabling")
	}
	if p.Status.Components.Gateway == nil {
		t.Error("expected the gateway slot to return after disabling ingress")
	}
	if p.Status.ConsoleURL != "" {
		t.Error("expected the advertised console URL to be cleared after disabling")
	}
}

// The identity slot is unconditional: every install carries the bundled
// identity server, in both front-door modes. It never retires.
func TestInitialize_IdentitySlotUnconditional(t *testing.T) {
	p := newMinimalPlanton()
	Initialize(p)
	if p.Status.Components.Identity == nil || p.Status.Components.Identity.Phase != v1.ComponentPhasePending {
		t.Fatal("identity slot must exist without ingress -- sign-in is unconditional")
	}

	p.Spec.Ingress = &v1.IngressSpec{Enabled: true}
	Initialize(p)
	if p.Status.Components.Identity == nil {
		t.Fatal("identity slot must survive the front-door switch to ingress")
	}

	p.Spec.Ingress.Enabled = false
	Initialize(p)
	if p.Status.Components.Identity == nil {
		t.Error("identity slot must survive the front-door switch back to the gateway")
	}
}

// The runner slot follows its toggle in both directions: on by default (an
// install that cannot deploy infrastructure is a browsing UI), retired on an
// explicit opt-out, back on re-enable -- all on a running platform.
func TestInitialize_RunnerSlotFollowsToggle(t *testing.T) {
	p := newMinimalPlanton()
	Initialize(p)
	if p.Status.Components.Runner == nil || p.Status.Components.Runner.Phase != v1.ComponentPhasePending {
		t.Fatal("runner slot must exist by default")
	}

	off := false
	p.Spec.Runner = &v1.RunnerSpec{Enabled: &off}
	if !Initialize(p) {
		t.Fatal("expected Initialize to report the runner slot retiring")
	}
	if p.Status.Components.Runner != nil {
		t.Error("expected the runner slot to retire when disabled")
	}

	on := true
	p.Spec.Runner.Enabled = &on
	if !Initialize(p) {
		t.Fatal("expected Initialize to report the runner slot returning")
	}
	if p.Status.Components.Runner == nil || p.Status.Components.Runner.Phase != v1.ComponentPhasePending {
		t.Error("expected a Pending runner slot after re-enabling")
	}
}

// The vault slot follows its toggle in both directions: on by default (the
// bundled secrets manager is integral -- credential store, KEK, signing key),
// retired on an explicit opt-out, back on re-enable -- all on a running
// platform (the secrets lab's arms, or a GitOps patch).
func TestInitialize_OpenBAOSlotFollowsToggle(t *testing.T) {
	p := newMinimalPlanton()
	Initialize(p)
	if p.Status.Components.OpenBAO == nil || p.Status.Components.OpenBAO.Phase != v1.ComponentPhasePending {
		t.Fatal("openbao slot must exist by default")
	}

	off := false
	p.Spec.Vault = &v1.OpenBAOSpec{Enabled: &off}
	if !Initialize(p) {
		t.Fatal("expected Initialize to report the openbao slot retiring")
	}
	if p.Status.Components.OpenBAO != nil {
		t.Error("expected the openbao slot to retire when vault is disabled")
	}

	on := true
	p.Spec.Vault.Enabled = &on
	if !Initialize(p) {
		t.Fatal("expected Initialize to report the openbao slot returning")
	}
	if p.Status.Components.OpenBAO == nil || p.Status.Components.OpenBAO.Phase != v1.ComponentPhasePending {
		t.Error("expected a Pending openbao slot after re-enabling")
	}
}

// The tekton slot follows the build capability: on by default (builds power
// Service Hub -- the DEFAULT install renders builds ON, the load-bearing
// product claim), retired on an explicit spec.build opt-out, and following
// the runner off when the runner is disabled with builds left at default --
// EXCEPT when builds are explicitly true, where the slot stays so the
// component can report the contradiction instead of silently skipping a
// stated intent.
func TestInitialize_TektonSlotFollowsBuildCapability(t *testing.T) {
	p := newMinimalPlanton()
	Initialize(p)
	if p.Status.Components.Tekton == nil || p.Status.Components.Tekton.Phase != v1.ComponentPhasePending {
		t.Fatal("tekton slot must exist by default -- builds are on unless opted out")
	}

	off := false
	p.Spec.Build = &v1.BuildSpec{Enabled: &off}
	if !Initialize(p) {
		t.Fatal("expected Initialize to report the tekton slot retiring on build opt-out")
	}
	if p.Status.Components.Tekton != nil {
		t.Error("expected the tekton slot to retire when builds are disabled")
	}

	// Runner off with builds left at DEFAULT: builds follow the runner off
	// quietly -- disabling the runner was the explicit act.
	p.Spec.Build = nil
	p.Spec.Runner = &v1.RunnerSpec{Enabled: &off}
	Initialize(p)
	if p.Status.Components.Tekton != nil {
		t.Error("expected the tekton slot to follow a disabled runner off when builds are default")
	}

	// Runner off with builds EXPLICITLY on: the slot stays so the component
	// can surface the contradiction as an error.
	on := true
	p.Spec.Build = &v1.BuildSpec{Enabled: &on}
	if !Initialize(p) {
		t.Fatal("expected Initialize to allocate the tekton slot for an explicit build enable")
	}
	if p.Status.Components.Tekton == nil {
		t.Error("expected the tekton slot to exist for explicit build enable even with the runner off")
	}
}

func TestRefuseVersion_RecordsTheRefusalOnceAndLeavesComponentsAlone(t *testing.T) {
	p := newMinimalPlanton()
	Initialize(p)
	p.Status.Components.PostgreSQL.Phase = v1.ComponentPhaseReady // a running platform an operator upgrade has outgrown

	if !RefuseVersion(p, "BelowOperatorMinimum", "too old") {
		t.Fatal("expected the first refusal to report a change")
	}
	if p.Status.Phase != v1.PhaseError {
		t.Errorf("expected phase Error, got %s", p.Status.Phase)
	}
	if p.Status.Components.PostgreSQL.Phase != v1.ComponentPhaseReady {
		t.Error("a refusal must not rewrite what components report is running")
	}
	for _, want := range []struct {
		condType, reason string
	}{
		{v1.ConditionVersionSupported, "BelowOperatorMinimum"},
		{v1.ConditionReady, ReasonPlatformVersionUnsupported},
	} {
		c := findCondition(p, want.condType)
		if c == nil || c.Status != metav1.ConditionFalse || c.Reason != want.reason || c.Message != "too old" {
			t.Errorf("condition %s = %+v, want False/%s/too old", want.condType, c, want.reason)
		}
	}

	if RefuseVersion(p, "BelowOperatorMinimum", "too old") {
		t.Error("an already-recorded refusal must not report a change (it would cost a status write per reconcile)")
	}
	if !RefuseVersion(p, "BelowOperatorMinimum", "different words") {
		t.Error("a changed message must be recorded")
	}
}

func findCondition(p *v1.PlantonPlatform, condType string) *metav1.Condition {
	for i := range p.Status.Conditions {
		if p.Status.Conditions[i].Type == condType {
			return &p.Status.Conditions[i]
		}
	}
	return nil
}
