package component

import (
	"context"
	"strings"
	"testing"
	"time"

	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	v1 "github.com/plantonhq/planton/operator/api/v1"
)

const (
	bindingTestNamespace = "planton"
	bindingTestVersion   = "v1.0.0"
	bindingTestIdpName   = "corp"
)

func bindingScheme(t *testing.T) *runtime.Scheme {
	t.Helper()
	scheme := runtime.NewScheme()
	if err := clientgoscheme.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	if err := v1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	return scheme
}

func testPlatform(name string) *v1.PlantonPlatform {
	p := &v1.PlantonPlatform{}
	p.Name = name
	p.Namespace = bindingTestNamespace
	p.Spec.Version = bindingTestVersion
	return p
}

func testIdentityProvider(name string, created time.Time, ref string) *v1.PlantonIdentityProvider {
	idp := &v1.PlantonIdentityProvider{}
	idp.Name = name
	idp.Namespace = bindingTestNamespace
	idp.CreationTimestamp = metav1.NewTime(created)
	if ref != "" {
		idp.Spec.PlatformRef = &v1.PlatformRef{Name: ref}
	}
	idp.Spec.OIDC = &v1.OIDCBrokerSpec{
		IssuerURL:       "https://login.microsoftonline.com/tenant/v2.0",
		ClientID:        "client",
		ClientSecretRef: v1.SecretKeyRef{Name: "secret", Key: "client-secret"},
	}
	return idp
}

func bindingOf(t *testing.T, c client.Client, name string) *v1.PlantonIdentityProvider {
	t.Helper()
	var idp v1.PlantonIdentityProvider
	if err := c.Get(context.Background(), types.NamespacedName{Name: name, Namespace: bindingTestNamespace}, &idp); err != nil {
		t.Fatal(err)
	}
	return &idp
}

// An empty platformRef resolves to the namespace's ONLY platform, and the
// resolution is recorded -- visible, never implicit.
func TestIdentityProviderBinding_EmptyRefSinglePlatform(t *testing.T) {
	scheme := bindingScheme(t)
	// The platform name deliberately differs from the namespace so a
	// name/namespace mixup in the resolution cannot pass silently.
	platform := testPlatform("prime")
	idp := testIdentityProvider(bindingTestIdpName, time.Now(), "")
	c := fake.NewClientBuilder().WithScheme(scheme).
		WithObjects(platform, idp).
		WithStatusSubresource(&v1.PlantonIdentityProvider{}).Build()

	winner, err := (&Identity{}).reconcileIdentityProviderBindings(context.Background(), c, platform)
	if err != nil {
		t.Fatal(err)
	}
	if winner == nil || winner.Name != bindingTestIdpName {
		t.Fatalf("winner = %v, want %s (federation provisioning consumes it)", winner, bindingTestIdpName)
	}

	got := bindingOf(t, c, bindingTestIdpName)
	if got.Status.BoundPlatform != "prime" {
		t.Errorf("boundPlatform = %q, want prime", got.Status.BoundPlatform)
	}
	cond := meta.FindStatusCondition(got.Status.Conditions, v1.ConditionBound)
	if cond == nil || cond.Status != metav1.ConditionTrue {
		t.Errorf("Bound condition = %v, want True", cond)
	}
}

// Two platforms and no ref is a NAMED status error -- never a guess, because
// guessing wrong silently federates a directory into the wrong Planton.
func TestIdentityProviderBinding_EmptyRefAmbiguous(t *testing.T) {
	scheme := bindingScheme(t)
	p1, p2 := testPlatform("alpha"), testPlatform("beta")
	idp := testIdentityProvider(bindingTestIdpName, time.Now(), "")
	c := fake.NewClientBuilder().WithScheme(scheme).
		WithObjects(p1, p2, idp).
		WithStatusSubresource(&v1.PlantonIdentityProvider{}).Build()

	winner, err := (&Identity{}).reconcileIdentityProviderBindings(context.Background(), c, p1)
	if err != nil {
		t.Fatal(err)
	}
	if winner != nil {
		t.Fatalf("winner = %s, want nil on ambiguity (never a guess)", winner.Name)
	}

	got := bindingOf(t, c, bindingTestIdpName)
	if got.Status.BoundPlatform != "" {
		t.Errorf("boundPlatform = %q, want empty on ambiguity", got.Status.BoundPlatform)
	}
	cond := meta.FindStatusCondition(got.Status.Conditions, v1.ConditionBound)
	if cond == nil || cond.Status != metav1.ConditionFalse || cond.Reason != "AmbiguousPlatform" {
		t.Fatalf("Bound condition = %+v, want False/AmbiguousPlatform", cond)
	}
	// The error must NAME both candidates so the fix is obvious.
	for _, name := range []string{"alpha", "beta"} {
		if !contains(cond.Message, name) {
			t.Errorf("ambiguity message must name candidate %s: %q", name, cond.Message)
		}
	}
}

// An explicit ref binds to exactly the named platform, even with several
// platforms in the namespace; a ref to ANOTHER platform is left untouched by
// this platform's pass.
func TestIdentityProviderBinding_ExplicitRef(t *testing.T) {
	scheme := bindingScheme(t)
	p1, p2 := testPlatform("alpha"), testPlatform("beta")
	mine := testIdentityProvider("corp-alpha", time.Now(), "alpha")
	other := testIdentityProvider("corp-beta", time.Now(), "beta")
	c := fake.NewClientBuilder().WithScheme(scheme).
		WithObjects(p1, p2, mine, other).
		WithStatusSubresource(&v1.PlantonIdentityProvider{}).Build()

	winner, err := (&Identity{}).reconcileIdentityProviderBindings(context.Background(), c, p1)
	if err != nil {
		t.Fatal(err)
	}
	if winner == nil || winner.Name != "corp-alpha" {
		t.Fatalf("winner = %v, want corp-alpha", winner)
	}

	if got := bindingOf(t, c, "corp-alpha"); got.Status.BoundPlatform != "alpha" {
		t.Errorf("corp-alpha boundPlatform = %q, want alpha", got.Status.BoundPlatform)
	}
	if got := bindingOf(t, c, "corp-beta"); got.Status.BoundPlatform != "" {
		t.Errorf("corp-beta boundPlatform = %q, want untouched by alpha's pass", got.Status.BoundPlatform)
	}
}

// At most ONE identity provider binds per platform (the v1 constraint):
// several candidates resolve deterministically to the oldest, and the losers'
// condition NAMES the winner so two passes can never flip-flop.
func TestIdentityProviderBinding_AtMostOnePerPlatform(t *testing.T) {
	scheme := bindingScheme(t)
	platform := testPlatform("prime")
	older := testIdentityProvider("older", time.Now().Add(-time.Hour), "prime")
	newer := testIdentityProvider("newer", time.Now(), "prime")
	c := fake.NewClientBuilder().WithScheme(scheme).
		WithObjects(platform, older, newer).
		WithStatusSubresource(&v1.PlantonIdentityProvider{}).Build()

	winner, err := (&Identity{}).reconcileIdentityProviderBindings(context.Background(), c, platform)
	if err != nil {
		t.Fatal(err)
	}
	if winner == nil || winner.Name != "older" {
		t.Fatalf("winner = %v, want older (oldest wins deterministically)", winner)
	}

	if got := bindingOf(t, c, "older"); got.Status.BoundPlatform != "prime" {
		t.Errorf("older boundPlatform = %q, want prime (oldest wins deterministically)", got.Status.BoundPlatform)
	}
	loser := bindingOf(t, c, "newer")
	cond := meta.FindStatusCondition(loser.Status.Conditions, v1.ConditionBound)
	if cond == nil || cond.Reason != "PlatformAlreadyBound" {
		t.Fatalf("newer Bound condition = %+v, want PlatformAlreadyBound", cond)
	}
	if !contains(cond.Message, "older") {
		t.Errorf("loser's condition must name the winner: %q", cond.Message)
	}
}

func contains(s, sub string) bool {
	return strings.Contains(s, sub)
}
