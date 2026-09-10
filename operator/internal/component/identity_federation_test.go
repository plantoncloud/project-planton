package component

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	v1 "github.com/plantonhq/planton/operator/api/v1"
	"github.com/plantonhq/planton/operator/internal/keycloak"
	"github.com/plantonhq/planton/operator/internal/resources"
)

func federationTestSecret(name string, data map[string]string) *corev1.Secret {
	secret := &corev1.Secret{}
	secret.Name = name
	secret.Namespace = bindingTestNamespace
	secret.Data = map[string][]byte{}
	for k, v := range data {
		secret.Data[k] = []byte(v)
	}
	return secret
}

func ldapIdentityProvider() *v1.PlantonIdentityProvider {
	idp := &v1.PlantonIdentityProvider{}
	idp.Name = bindingTestIdpName
	idp.Namespace = bindingTestNamespace
	idp.Generation = 1
	idp.Spec.ActiveDirectory = &v1.ActiveDirectorySpec{
		Servers:                 []string{"ldaps://dc1.lab.example.internal:636"},
		BindDN:                  "CN=svc,DC=lab,DC=example,DC=internal",
		BindCredentialSecretRef: v1.SecretKeyRef{Name: "corp-bind", Key: "password"},
		UsersDN:                 "OU=Staff,DC=lab,DC=example,DC=internal",
		GroupsDN:                "OU=Groups,DC=lab,DC=example,DC=internal",
	}
	return idp
}

// The LDAP build reads the bind credential by reference, applies the AD
// schema defaults (the CRD's admission defaulting owns them; the build's
// fallbacks guard fake-client paths so a zero never reaches the identity
// server), and flags rotation + verification on a first-ever build.
func TestBuildFederationState_LDAP(t *testing.T) {
	platform := testPlatform("prime")
	idp := ldapIdentityProvider()
	c := fake.NewClientBuilder().WithScheme(bindingScheme(t)).
		WithObjects(platform, idp, federationTestSecret("corp-bind", map[string]string{"password": "bind-pw"})).
		Build()

	build := (&Identity{}).buildFederationState(context.Background(), c, platform, idp)

	if build.buildErr != "" {
		t.Fatalf("buildErr = %q, want none", build.buildErr)
	}
	ldap := build.fed.LDAP
	if ldap == nil {
		t.Fatal("LDAP desired state missing")
	}
	if ldap.BindCredential != "bind-pw" {
		t.Errorf("bind credential not read from the referenced Secret")
	}
	if !ldap.RotateCredential {
		t.Error("a first-ever build (no recorded fingerprint) must write the credential")
	}
	if !build.verificationDue {
		t.Error("a never-verified manifest must verify")
	}
	if ldap.UsernameAttribute != "sAMAccountName" || ldap.SyncPeriodMinutes != 60 || !ldap.NestedGroups {
		t.Errorf("AD defaults not applied: %+v", ldap)
	}
}

// A recorded fingerprint matching the live Secret means NO rotation write --
// the property that keeps the credential from being re-written (and the
// reconcile from being write-noisy) every 30 seconds.
func TestBuildFederationState_SteadyStateNoRotation(t *testing.T) {
	platform := testPlatform("prime")
	idp := ldapIdentityProvider()
	idp.Status.Conditions = []metav1.Condition{{
		Type: v1.ConditionProvisioned, Status: metav1.ConditionTrue,
		Reason: "Provisioned", ObservedGeneration: 1, LastTransitionTime: metav1.Now(),
	}}
	idp.Status.Verification = &v1.IdentityProviderVerification{
		Checks: []v1.IdentityProviderVerificationCheck{{Name: "connection", Verdict: "Passed"}},
	}

	state := federationTestSecret(resources.IdentityFederationStateSecretName("prime"), map[string]string{
		resources.IdentityFederationStateCredentialKey: sha256Hex("bind-pw"),
	})
	c := fake.NewClientBuilder().WithScheme(bindingScheme(t)).
		WithObjects(platform, idp, state, federationTestSecret("corp-bind", map[string]string{"password": "bind-pw"})).
		Build()

	build := (&Identity{}).buildFederationState(context.Background(), c, platform, idp)

	if build.fed.LDAP.RotateCredential {
		t.Error("an unchanged credential must not rotate")
	}
	if build.verificationDue {
		t.Error("verified generation + unchanged credential = no probes (the cadence law)")
	}
}

// A rotated Secret is detected against the recorded fingerprint: the
// credential is re-written and verification re-runs.
func TestBuildFederationState_CredentialRotation(t *testing.T) {
	platform := testPlatform("prime")
	idp := ldapIdentityProvider()
	idp.Status.Conditions = []metav1.Condition{{
		Type: v1.ConditionProvisioned, Status: metav1.ConditionTrue,
		Reason: "Provisioned", ObservedGeneration: 1, LastTransitionTime: metav1.Now(),
	}}
	idp.Status.Verification = &v1.IdentityProviderVerification{
		Checks: []v1.IdentityProviderVerificationCheck{{Name: "connection", Verdict: "Passed"}},
	}

	state := federationTestSecret(resources.IdentityFederationStateSecretName("prime"), map[string]string{
		resources.IdentityFederationStateCredentialKey: sha256Hex("OLD-pw"),
	})
	c := fake.NewClientBuilder().WithScheme(bindingScheme(t)).
		WithObjects(platform, idp, state, federationTestSecret("corp-bind", map[string]string{"password": "NEW-pw"})).
		Build()

	build := (&Identity{}).buildFederationState(context.Background(), c, platform, idp)

	if !build.fed.LDAP.RotateCredential {
		t.Error("a rotated credential must be re-written")
	}
	if !build.verificationDue {
		t.Error("a rotated credential must re-verify")
	}
}

// An unreadable referenced Secret becomes a plain-language buildErr and NO
// desired state -- the hands-off contract that keeps a transient read
// failure from tearing down the live federation a company signs in through.
func TestBuildFederationState_MissingSecretIsHandsOff(t *testing.T) {
	platform := testPlatform("prime")
	idp := ldapIdentityProvider()
	c := fake.NewClientBuilder().WithScheme(bindingScheme(t)).WithObjects(platform, idp).Build()

	build := (&Identity{}).buildFederationState(context.Background(), c, platform, idp)

	if build.buildErr == "" || build.fed != nil {
		t.Fatalf("missing Secret must yield buildErr and nil desired state, got fed=%+v err=%q", build.fed, build.buildErr)
	}
	if federationForConverge(idp, build) != nil {
		t.Error("hands off: converge must receive nil (leave federation untouched), never an empty sweep")
	}
}

// The nil-vs-empty mapping: nothing bound sweeps; unbuildable state is hands
// off; built state passes through.
func TestFederationForConverge(t *testing.T) {
	if fed := federationForConverge(nil, nil); fed == nil || fed.LDAP != nil || fed.Broker != nil {
		t.Errorf("nothing bound must map to the EMPTY federation (sweep), got %+v", fed)
	}
	built := &federationBuild{fed: &keycloak.OwnedFederation{LDAP: &keycloak.OwnedLDAPFederation{}}}
	if fed := federationForConverge(ldapIdentityProvider(), built); fed != built.fed {
		t.Error("built state must pass through")
	}
}

// The broker build discovers endpoints on verification passes (proven
// against a local discovery server) and records them; steady-state passes
// replay the record without any fetch.
func TestBuildFederationState_BrokerDiscoveryAndReplay(t *testing.T) {
	var discoveryCalls int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		discoveryCalls++
		_ = json.NewEncoder(w).Encode(map[string]string{
			"issuer":                 serverURLHolder,
			"authorization_endpoint": serverURLHolder + "/authorize",
			"token_endpoint":         serverURLHolder + "/token",
			"jwks_uri":               serverURLHolder + "/keys",
		})
	}))
	defer server.Close()
	serverURLHolder = server.URL

	platform := testPlatform("prime")
	idp := &v1.PlantonIdentityProvider{}
	idp.Name = bindingTestIdpName
	idp.Namespace = bindingTestNamespace
	idp.Generation = 1
	idp.Spec.OIDC = &v1.OIDCBrokerSpec{
		IssuerURL:       server.URL,
		ClientID:        "client-id",
		ClientSecretRef: v1.SecretKeyRef{Name: "corp-oidc", Key: "client-secret"},
	}

	c := fake.NewClientBuilder().WithScheme(bindingScheme(t)).
		WithObjects(platform, idp, federationTestSecret("corp-oidc", map[string]string{"client-secret": "s3cret"})).
		Build()

	build := (&Identity{}).buildFederationState(context.Background(), c, platform, idp)
	if build.buildErr != "" {
		t.Fatalf("buildErr = %q, want none", build.buildErr)
	}
	if discoveryCalls != 1 {
		t.Fatalf("discovery calls = %d, want exactly 1 on a verification pass", discoveryCalls)
	}
	broker := build.fed.Broker
	if broker.Endpoints == nil || broker.Endpoints.AuthorizationURL != server.URL+"/authorize" {
		t.Fatalf("endpoints not discovered: %+v", broker.Endpoints)
	}
	if build.endpointsJSON == "" {
		t.Error("fresh discovery must be recorded for replay")
	}
	// The claim defaults: groups arrive in "groups", the stable subject in
	// "sub" (present on every provider; Entra deployments declare oid).
	if broker.GroupsClaim != "groups" || broker.SubjectClaim != "sub" {
		t.Errorf("claim defaults = groups:%q subject:%q, want groups/sub", broker.GroupsClaim, broker.SubjectClaim)
	}

	// Steady state: verified generation + recorded endpoints -> zero
	// fetches (the cadence law for the upstream's discovery document).
	idp.Status.Conditions = []metav1.Condition{{
		Type: v1.ConditionProvisioned, Status: metav1.ConditionTrue,
		Reason: "Provisioned", ObservedGeneration: 1, LastTransitionTime: metav1.Now(),
	}}
	idp.Status.Verification = &v1.IdentityProviderVerification{
		Checks: []v1.IdentityProviderVerificationCheck{{Name: "issuer", Verdict: "Passed"}},
	}
	state := federationTestSecret(resources.IdentityFederationStateSecretName("prime"), map[string]string{
		resources.IdentityFederationStateCredentialKey: sha256Hex("s3cret"),
		resources.IdentityFederationStateEndpointsKey:  build.endpointsJSON,
	})
	c = fake.NewClientBuilder().WithScheme(bindingScheme(t)).
		WithObjects(platform, idp, state, federationTestSecret("corp-oidc", map[string]string{"client-secret": "s3cret"})).
		Build()

	discoveryCalls = 0
	steady := (&Identity{}).buildFederationState(context.Background(), c, platform, idp)
	if steady.buildErr != "" {
		t.Fatalf("steady buildErr = %q, want none", steady.buildErr)
	}
	if discoveryCalls != 0 {
		t.Errorf("steady-state discovery calls = %d, want 0 (replayed from the record)", discoveryCalls)
	}
	if steady.fed.Broker.Endpoints == nil || steady.fed.Broker.Endpoints.AuthorizationURL != server.URL+"/authorize" {
		t.Errorf("recorded endpoints not replayed: %+v", steady.fed.Broker.Endpoints)
	}
}

// serverURLHolder lets the discovery handler advertise its own dynamic URL
// as the issuer (strict issuer comparison needs the exact string).
var serverURLHolder string

// ---- the federation facts projection --------------------------------------
//
// The facts ConfigMap is the operator→product advertisement the control
// plane mounts: it must exist on EVERY install (configured=false until a
// manifest binds), carry the manifest's arm + verdicts verbatim when one
// does, flip back on the sweep, and stamp observed-at only when a
// verification pass actually recorded fresh verdicts.

func factsOf(t *testing.T, c client.Client, crName string) resources.IdentityFederationFacts {
	t.Helper()
	var configMap corev1.ConfigMap
	if err := c.Get(context.Background(), types.NamespacedName{
		Name: resources.IdentityFederationFactsConfigMapName(crName), Namespace: bindingTestNamespace,
	}, &configMap); err != nil {
		t.Fatalf("facts ConfigMap missing: %v", err)
	}
	var facts resources.IdentityFederationFacts
	if err := json.Unmarshal([]byte(configMap.Data[resources.IdentityFederationFactsKey]), &facts); err != nil {
		t.Fatalf("facts payload unparseable: %v", err)
	}
	return facts
}

// No manifest bound: the projection still publishes, saying so -- the
// unconditional-existence law the control plane's mount depends on.
func TestProjectFederationFacts_NoManifestBound(t *testing.T) {
	platform := testPlatform("prime")
	c := fake.NewClientBuilder().WithScheme(bindingScheme(t)).WithObjects(platform).Build()

	(&Identity{}).projectFederationFacts(context.Background(), c, platform, nil, false)

	facts := factsOf(t, c, "prime")
	if facts.Configured || facts.Arm != "" || len(facts.Checks) != 0 {
		t.Errorf("unbound facts must say configured=false with no arm/checks, got %+v", facts)
	}
}

// A bound manifest projects its declared arm, the defaulted provider label,
// the Provisioned judgment, and every verdict VERBATIM -- and a fresh
// verification stamps observed-at.
func TestProjectFederationFacts_BoundManifestProjectsVerdicts(t *testing.T) {
	platform := testPlatform("prime")
	idp := ldapIdentityProvider()
	idp.Status.Conditions = []metav1.Condition{{
		Type: v1.ConditionProvisioned, Status: metav1.ConditionTrue,
		Reason: "Provisioned", ObservedGeneration: 1, LastTransitionTime: metav1.Now(),
	}}
	idp.Status.Verification = &v1.IdentityProviderVerification{
		Checks: []v1.IdentityProviderVerificationCheck{
			{Name: "connection", Verdict: "Passed", Message: "the directory answered"},
			{Name: "usersSearch", Verdict: "Failed", Message: "the users base is not readable"},
		},
	}
	c := fake.NewClientBuilder().WithScheme(bindingScheme(t)).WithObjects(platform, idp).Build()

	(&Identity{}).projectFederationFacts(context.Background(), c, platform, idp, true)

	facts := factsOf(t, c, "prime")
	if !facts.Configured || facts.Arm != "ldap" {
		t.Fatalf("facts = %+v, want configured ldap arm", facts)
	}
	if facts.ProviderLabel != "Active Directory" {
		t.Errorf("providerLabel = %q, want the LDAP-arm default", facts.ProviderLabel)
	}
	if !facts.Provisioned {
		t.Error("Provisioned=True condition must project as provisioned")
	}
	if len(facts.Checks) != 2 || facts.Checks[1].Verdict != "Failed" ||
		facts.Checks[1].Message != "the users base is not readable" {
		t.Errorf("verdicts must project verbatim, got %+v", facts.Checks)
	}
	if facts.ObservedAt == "" {
		t.Error("a fresh verification must stamp observedAt")
	}
}

// The stamp means "when the verdicts were recorded": a projection without a
// fresh verification preserves the recorded stamp, never re-stamps it.
func TestProjectFederationFacts_PreservesObservedAtWhenNotFresh(t *testing.T) {
	platform := testPlatform("prime")
	idp := ldapIdentityProvider()
	idp.Status.Verification = &v1.IdentityProviderVerification{
		Checks: []v1.IdentityProviderVerificationCheck{{Name: "connection", Verdict: "Passed"}},
	}
	c := fake.NewClientBuilder().WithScheme(bindingScheme(t)).WithObjects(platform, idp).Build()

	identity := &Identity{}
	identity.projectFederationFacts(context.Background(), c, platform, idp, true)
	recorded := factsOf(t, c, "prime").ObservedAt
	if recorded == "" {
		t.Fatal("fresh projection must stamp observedAt")
	}

	identity.projectFederationFacts(context.Background(), c, platform, idp, false)
	if got := factsOf(t, c, "prime").ObservedAt; got != recorded {
		t.Errorf("a non-fresh projection must preserve the stamp: got %q, recorded %q", got, recorded)
	}
}

// The manifest-deletion sweep: the next pass projects configured=false with
// nothing stale left behind -- the console flips back to the connect journey.
func TestProjectFederationFacts_SweepFlipsBackToUnconfigured(t *testing.T) {
	platform := testPlatform("prime")
	idp := ldapIdentityProvider()
	idp.Status.Verification = &v1.IdentityProviderVerification{
		Checks: []v1.IdentityProviderVerificationCheck{{Name: "connection", Verdict: "Passed"}},
	}
	c := fake.NewClientBuilder().WithScheme(bindingScheme(t)).WithObjects(platform, idp).Build()

	identity := &Identity{}
	identity.projectFederationFacts(context.Background(), c, platform, idp, true)
	identity.projectFederationFacts(context.Background(), c, platform, nil, false)

	facts := factsOf(t, c, "prime")
	if facts.Configured || facts.Arm != "" || len(facts.Checks) != 0 || facts.ObservedAt != "" {
		t.Errorf("post-sweep facts must be fully unconfigured, got %+v", facts)
	}
}
