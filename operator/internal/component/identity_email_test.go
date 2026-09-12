package component

import (
	"context"
	"testing"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	v1 "github.com/plantonhq/planton/operator/api/v1"
	"github.com/plantonhq/planton/operator/internal/keycloak"
	"github.com/plantonhq/planton/operator/internal/resources"
)

// relayCredentials is the kubernetes.io/basic-auth Secret a mail team hands
// the platform.
func relayCredentials(password string) *corev1.Secret {
	secret := federationTestSecret("planton-email", map[string]string{
		resources.BasicAuthUsernameKey: "planton@acme.com",
		resources.BasicAuthPasswordKey: password,
	})
	secret.Type = corev1.SecretTypeBasicAuth
	return secret
}

func buildEmail(c client.Client, platform *v1.PlantonPlatform) *emailRealmBuild {
	id := &Identity{}
	return id.buildEmailRealmState(context.Background(), c, platform, id.readRealmState(context.Background(), c, platform))
}

// No declaration owns the off-state: the realm carries no relay and password
// reset is off -- never hands off, so a relay typed into the identity
// server's console cannot contradict what the product advertises.
func TestBuildEmailRealmState_NoDeclarationOwnsOff(t *testing.T) {
	platform := testPlatform("prime")
	c := fake.NewClientBuilder().WithScheme(bindingScheme(t)).WithObjects(platform).Build()

	build := buildEmail(c, platform)
	if build.email == nil || build.email.Relay != nil {
		t.Fatalf("no declaration must own the off-state, got %+v", build.email)
	}
	if build.credentialSHA != "" || build.caBundleSecretName != "" {
		t.Error("no declaration carries no fingerprint and no CA")
	}
}

// A first-ever declaration: the Secret's values reach the relay, the
// credential is fingerprinted, and the (absent) record makes it a rotation
// write; the CA bundle rides to the Deployment with its content hash.
func TestBuildEmailRealmState_FirstDeclarationRotates(t *testing.T) {
	platform := testPlatform("prime")
	platform.Spec.Email = smtpEmailSpec()
	platform.Spec.Email.SMTP.CABundleSecretRef = &v1.SecretKeyRef{Name: "corp-ca", Key: "ca.crt"}
	c := fake.NewClientBuilder().WithScheme(bindingScheme(t)).
		WithObjects(platform, relayCredentials("relay-pw"), federationTestSecret("corp-ca", map[string]string{"ca.crt": "PEM"})).
		Build()

	build := buildEmail(c, platform)
	if build.email == nil || build.email.Relay == nil {
		t.Fatalf("a declared relay must be owned, got %+v", build.email)
	}
	relay := build.email.Relay
	if relay.Host != "smtp.office365.com" || relay.Port != 587 || relay.Security != "starttls" {
		t.Errorf("relay connection = %+v", relay)
	}
	if relay.From != "no-reply@planton.acme.com" || relay.FromDisplayName != "Planton" || relay.ReplyTo != "it-help@acme.com" {
		t.Errorf("relay identity = %+v", relay)
	}
	if relay.Auth != keycloak.OwnedSMTPAuthPassword || relay.User != "planton@acme.com" || relay.Password != "relay-pw" {
		t.Errorf("relay sign-in = %+v", relay)
	}
	if !build.email.RotateCredential {
		t.Error("a first-ever build (no recorded fingerprint) must write the credential")
	}
	if build.credentialSHA != sha256Hex("relay-pw") {
		t.Error("the fingerprint is the password's SHA-256")
	}
	if build.caBundleSecretName != "corp-ca" || build.caBundleSecretKey != "ca.crt" || build.caBundleHash != sha256Hex("PEM") {
		t.Errorf("CA bundle = %s/%s %s", build.caBundleSecretName, build.caBundleSecretKey, build.caBundleHash)
	}
}

// A recorded fingerprint matching the live Secret means no rotation write --
// what keeps the relay password from being re-written every 30 seconds.
func TestBuildEmailRealmState_SteadyStateNoRotation(t *testing.T) {
	platform := testPlatform("prime")
	platform.Spec.Email = smtpEmailSpec()
	state := federationTestSecret(resources.IdentityRealmStateSecretName("prime"), map[string]string{
		resources.IdentityRealmStateEmailCredentialKey: sha256Hex("relay-pw"),
	})
	c := fake.NewClientBuilder().WithScheme(bindingScheme(t)).
		WithObjects(platform, relayCredentials("relay-pw"), state).Build()

	build := buildEmail(c, platform)
	if build.email.RotateCredential {
		t.Error("an unchanged credential must not rotate")
	}
}

// A rotated Secret is detected against the record.
func TestBuildEmailRealmState_CredentialRotation(t *testing.T) {
	platform := testPlatform("prime")
	platform.Spec.Email = smtpEmailSpec()
	state := federationTestSecret(resources.IdentityRealmStateSecretName("prime"), map[string]string{
		resources.IdentityRealmStateEmailCredentialKey: sha256Hex("OLD-pw"),
	})
	c := fake.NewClientBuilder().WithScheme(bindingScheme(t)).
		WithObjects(platform, relayCredentials("NEW-pw"), state).Build()

	build := buildEmail(c, platform)
	if !build.email.RotateCredential {
		t.Error("a moved Secret must rotate the realm's credential")
	}
	if build.credentialSHA != sha256Hex("NEW-pw") {
		t.Error("the record must advance to the new fingerprint")
	}
}

// A declared relay whose Secret is missing is HANDS OFF: nil email, so the
// realm's settings stay as they are this pass and the sign-in page does not
// flip on a transient gap. The control plane carries the sentence.
func TestBuildEmailRealmState_MissingSecretIsHandsOff(t *testing.T) {
	platform := testPlatform("prime")
	platform.Spec.Email = smtpEmailSpec()
	c := fake.NewClientBuilder().WithScheme(bindingScheme(t)).WithObjects(platform).Build()

	build := buildEmail(c, platform)
	if build.email != nil {
		t.Errorf("a missing Secret must hand off, got %+v", build.email)
	}
}

// A relay that needs no credential: no sign-in, no fingerprint, never a
// rotation.
func TestBuildEmailRealmState_NoCredentialRelay(t *testing.T) {
	platform := testPlatform("prime")
	platform.Spec.Email = smtpEmailSpec()
	platform.Spec.Email.SMTP.CredentialsSecretName = ""
	platform.Spec.Email.SMTP.Security = v1.EmailSMTPSecurityNone
	platform.Spec.Email.SMTP.Port = 25
	c := fake.NewClientBuilder().WithScheme(bindingScheme(t)).WithObjects(platform).Build()

	build := buildEmail(c, platform)
	if build.email.Relay.Auth != keycloak.OwnedSMTPAuthNone || build.email.RotateCredential || build.credentialSHA != "" {
		t.Errorf("a no-credential relay = %+v (sha %q)", build.email, build.credentialSHA)
	}
}

// OAuth2: the client secret is read by reference and the four token facts
// reach the relay in Keycloak's token sign-in mode.
func TestBuildEmailRealmState_OAuth2(t *testing.T) {
	platform := testPlatform("prime")
	platform.Spec.Email = smtpEmailSpec()
	platform.Spec.Email.SMTP.CredentialsSecretName = ""
	platform.Spec.Email.SMTP.OAuth2 = &v1.EmailSMTPOAuth2Spec{
		User: "planton@acme.com", TokenURL: "https://login.microsoftonline.com/t/oauth2/v2.0/token",
		Scope: "https://outlook.office365.com/.default", ClientID: "app-id",
		ClientSecretRef: v1.SecretKeyRef{Name: "planton-email-oauth", Key: "client-secret"},
	}
	c := fake.NewClientBuilder().WithScheme(bindingScheme(t)).
		WithObjects(platform, federationTestSecret("planton-email-oauth", map[string]string{"client-secret": "app-secret"})).Build()

	build := buildEmail(c, platform)
	relay := build.email.Relay
	if relay.Auth != keycloak.OwnedSMTPAuthToken || relay.User != "planton@acme.com" || relay.TokenClientID != "app-id" ||
		relay.TokenClientSecret != "app-secret" || relay.TokenScope != "https://outlook.office365.com/.default" {
		t.Errorf("token relay = %+v", relay)
	}
	if build.credentialSHA != sha256Hex("app-secret") {
		t.Error("the fingerprint is the client secret's")
	}
}

// The Resend arm: the identity server has no Resend API, so it gets Resend's
// SMTP endpoint with the API key as the password -- one declaration, both
// senders.
func TestBuildEmailRealmState_ResendRidesResendSMTP(t *testing.T) {
	platform := testPlatform("prime")
	platform.Spec.Email = &v1.EmailSpec{
		From:   v1.EmailFromSpec{Address: "no-reply@planton.ai", Name: "Planton"},
		Resend: &v1.EmailResendSpec{APIKeySecretRef: v1.SecretKeyRef{Name: "resend", Key: "api-key"}},
	}
	c := fake.NewClientBuilder().WithScheme(bindingScheme(t)).
		WithObjects(platform, federationTestSecret("resend", map[string]string{"api-key": "re_123"})).Build()

	build := buildEmail(c, platform)
	relay := build.email.Relay
	if relay.Host != resendSMTPHost || relay.Port != resendSMTPPort || relay.Security != "tls" ||
		relay.Auth != keycloak.OwnedSMTPAuthPassword || relay.User != resendSMTPUser || relay.Password != "re_123" {
		t.Errorf("resend relay = %+v", relay)
	}
}

// The realm-state record: read once, written once, and a replayed pass keeps
// the recorded broker endpoints (a dropped record would strand a later
// recreate behind a discovery fetch).
func TestRealmStateRecord_ReadWriteAndEndpointsSurvive(t *testing.T) {
	platform := testPlatform("prime")
	c := fake.NewClientBuilder().WithScheme(bindingScheme(t)).WithObjects(platform).Build()
	id := &Identity{}
	ctx := context.Background()

	if got := id.readRealmState(ctx, c, platform); got != (realmStateRecord{}) {
		t.Fatalf("no record yet must read empty, got %+v", got)
	}

	first := realmStateRecord{FederationCredentialSHA: "fed", OIDCEndpointsJSON: `{"issuer":"x"}`, EmailCredentialSHA: "mail"}
	if err := id.writeRealmState(ctx, c, platform, first); err != nil {
		t.Fatal(err)
	}
	if got := id.readRealmState(ctx, c, platform); got != first {
		t.Errorf("round trip = %+v, want %+v", got, first)
	}

	// A pass that earned only the email field and replayed the endpoints
	// starts from the recorded value, so nothing is lost.
	record := id.readRealmState(ctx, c, platform)
	record.EmailCredentialSHA = "mail-2"
	if err := id.writeRealmState(ctx, c, platform, record); err != nil {
		t.Fatal(err)
	}
	var secret corev1.Secret
	if err := c.Get(ctx, types.NamespacedName{Name: resources.IdentityRealmStateSecretName("prime"), Namespace: bindingTestNamespace}, &secret); err != nil {
		t.Fatal(err)
	}
	if string(secret.Data[resources.IdentityRealmStateOIDCEndpointsKey]) != `{"issuer":"x"}` {
		t.Error("the recorded endpoints must survive a pass that discovered none")
	}
	if string(secret.Data[resources.IdentityRealmStateEmailCredentialKey]) != "mail-2" {
		t.Error("the email fingerprint must advance")
	}
}

func TestEmailThemeFacts(t *testing.T) {
	// Nothing declared: the product name, the front door, no reply-to.
	bare := emailThemeFacts(&v1.PlantonPlatform{}, "https://planton.acme.com")
	if bare.BrandName != "Planton" || bare.ConsoleURL != "https://planton.acme.com" || bare.ReplyTo != "" {
		t.Errorf("bare facts = %+v", bare)
	}

	declared := &v1.PlantonPlatform{Spec: v1.PlantonPlatformSpec{Email: &v1.EmailSpec{
		From:    v1.EmailFromSpec{Address: "no-reply@acme.com", Name: "Acme Platform"},
		ReplyTo: "it-help@acme.com",
	}}}
	facts := emailThemeFacts(declared, "https://planton.acme.com")
	if facts.BrandName != "Acme Platform" || facts.ReplyTo != "it-help@acme.com" || facts.ConsoleHost() != "planton.acme.com" {
		t.Errorf("declared facts = %+v", facts)
	}
}
