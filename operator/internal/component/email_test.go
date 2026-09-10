package component

import (
	"context"
	"strings"
	"testing"

	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	v1 "github.com/plantonhq/planton/operator/api/v1"
	"github.com/plantonhq/planton/operator/internal/resources"
)

func smtpEmailSpec() *v1.EmailSpec {
	return &v1.EmailSpec{
		From:    v1.EmailFromSpec{Address: "no-reply@planton.acme.com", Name: "Planton"},
		ReplyTo: "it-help@acme.com",
		SMTP: &v1.EmailSMTPSpec{
			Host:                  "smtp.office365.com",
			Port:                  587,
			Security:              v1.EmailSMTPSecurityStartTLS,
			CredentialsSecretName: "planton-email",
		},
	}
}

func basicAuthSecret(name string, keys ...string) *corev1.Secret {
	secret := federationTestSecret(name, nil)
	secret.Type = corev1.SecretTypeBasicAuth
	for _, k := range keys {
		secret.Data[k] = []byte("x")
	}
	return secret
}

// effectiveEmail is a pure translation of the declared arm; no spec.email is
// nil (rendered as provider none), and every declared fact lands verbatim.
func TestEffectiveEmail(t *testing.T) {
	p := testPlatform("prime")

	if got := effectiveEmail(p); got != nil {
		t.Errorf("no spec.email must resolve nil, got %+v", got)
	}

	p.Spec.Email = smtpEmailSpec()
	got := effectiveEmail(p)
	if got == nil || got.Provider != resources.EmailProviderSMTP {
		t.Fatalf("smtp arm must resolve as the smtp provider, got %+v", got)
	}
	if got.FromAddress != "no-reply@planton.acme.com" || got.FromName != "Planton" || got.ReplyTo != "it-help@acme.com" {
		t.Errorf("sender identity must resolve verbatim, got %+v", got)
	}
	if got.SMTP == nil || got.SMTP.Host != "smtp.office365.com" || got.SMTP.Port != 587 || got.SMTP.Security != "starttls" || got.SMTP.CredentialsSecretName != "planton-email" {
		t.Errorf("relay facts must resolve verbatim, got %+v", got.SMTP)
	}
	if got.SMTP.OAuth2 != nil || got.SMTP.CABundleSecretName != "" || got.Resend != nil {
		t.Errorf("nothing undeclared may resolve, got %+v", got)
	}

	p.Spec.Email.SMTP.CredentialsSecretName = ""
	p.Spec.Email.SMTP.OAuth2 = &v1.EmailSMTPOAuth2Spec{
		User: "planton@acme.com", TokenURL: "https://login.microsoftonline.com/t/oauth2/v2.0/token",
		Scope: "https://outlook.office365.com/.default", ClientID: "cid",
		ClientSecretRef: v1.SecretKeyRef{Name: "planton-email-oauth", Key: "client-secret"},
	}
	p.Spec.Email.SMTP.CABundleSecretRef = &v1.SecretKeyRef{Name: "corp-ca", Key: "ca.crt"}
	got = effectiveEmail(p)
	if got.SMTP.OAuth2 == nil || got.SMTP.OAuth2.ClientSecretName != "planton-email-oauth" || got.SMTP.OAuth2.ClientSecretKey != "client-secret" || got.SMTP.OAuth2.User != "planton@acme.com" {
		t.Errorf("oauth2 grant must resolve with its Secret reference, got %+v", got.SMTP.OAuth2)
	}
	if got.SMTP.CABundleSecretName != "corp-ca" || got.SMTP.CABundleSecretKey != "ca.crt" {
		t.Errorf("CA bundle reference must resolve, got %+v", got.SMTP)
	}

	p.Spec.Email = &v1.EmailSpec{
		From:   v1.EmailFromSpec{Address: "no-reply@planton.acme.com"},
		Resend: &v1.EmailResendSpec{APIKeySecretRef: v1.SecretKeyRef{Name: "planton-email", Key: "api-key"}},
	}
	got = effectiveEmail(p)
	if got == nil || got.Provider != resources.EmailProviderResend || got.Resend == nil || got.Resend.APIKeySecretName != "planton-email" || got.Resend.APIKeySecretKey != "api-key" || got.SMTP != nil {
		t.Errorf("resend arm must resolve as the resend provider with its key reference, got %+v", got)
	}

	p.Spec.Email = &v1.EmailSpec{From: v1.EmailFromSpec{Address: "x@y"}}
	if got := effectiveEmail(p); got != nil {
		t.Errorf("a block with no arm (unreachable through the API server) must resolve nil, got %+v", got)
	}
}

// The preflight finds a missing Secret or a missing key BEFORE the volume
// would project it, and says which Secret, which key, what to create, and
// what to remove -- the whole way out in one sentence.
func TestPreflightEmailSecrets(t *testing.T) {
	ctx := context.Background()

	t.Run("no email declared passes", func(t *testing.T) {
		p := testPlatform("prime")
		c := fake.NewClientBuilder().WithScheme(bindingScheme(t)).WithObjects(p).Build()
		msg, err := preflightEmailSecrets(ctx, c, p)
		if err != nil || msg != "" {
			t.Errorf("msg=%q err=%v, want nothing", msg, err)
		}
	})

	t.Run("present basic-auth Secret passes", func(t *testing.T) {
		p := testPlatform("prime")
		p.Spec.Email = smtpEmailSpec()
		c := fake.NewClientBuilder().WithScheme(bindingScheme(t)).
			WithObjects(p, basicAuthSecret("planton-email", "username", "password")).Build()
		msg, err := preflightEmailSecrets(ctx, c, p)
		if err != nil || msg != "" {
			t.Errorf("msg=%q err=%v, want nothing", msg, err)
		}
	})

	t.Run("missing credentials Secret is named with both ways out", func(t *testing.T) {
		p := testPlatform("prime")
		p.Spec.Email = smtpEmailSpec()
		c := fake.NewClientBuilder().WithScheme(bindingScheme(t)).WithObjects(p).Build()
		msg, err := preflightEmailSecrets(ctx, c, p)
		if err != nil {
			t.Fatal(err)
		}
		want := `email credentials Secret "planton-email" not found in namespace "` + bindingTestNamespace + `"; create it (type kubernetes.io/basic-auth) or remove spec.email.smtp.credentialsSecretName -- until then the platform runs as if no email were declared`
		if msg != want {
			t.Errorf("message =\n%s\nwant\n%s", msg, want)
		}
	})

	t.Run("basic-auth Secret without a password key is named", func(t *testing.T) {
		p := testPlatform("prime")
		p.Spec.Email = smtpEmailSpec()
		c := fake.NewClientBuilder().WithScheme(bindingScheme(t)).
			WithObjects(p, basicAuthSecret("planton-email", "username")).Build()
		msg, _ := preflightEmailSecrets(ctx, c, p)
		if !strings.Contains(msg, `has no "password" key`) || !strings.Contains(msg, "spec.email.smtp.credentialsSecretName") {
			t.Errorf("message must name the missing key and the field, got: %s", msg)
		}
	})

	t.Run("unauthenticated relay references nothing and passes", func(t *testing.T) {
		p := testPlatform("prime")
		p.Spec.Email = smtpEmailSpec()
		p.Spec.Email.SMTP.CredentialsSecretName = ""
		p.Spec.Email.SMTP.Security = v1.EmailSMTPSecurityNone
		c := fake.NewClientBuilder().WithScheme(bindingScheme(t)).WithObjects(p).Build()
		msg, err := preflightEmailSecrets(ctx, c, p)
		if err != nil || msg != "" {
			t.Errorf("msg=%q err=%v, want nothing", msg, err)
		}
	})

	t.Run("oauth2 client secret and CA bundle are each checked by key", func(t *testing.T) {
		p := testPlatform("prime")
		p.Spec.Email = smtpEmailSpec()
		p.Spec.Email.SMTP.CredentialsSecretName = ""
		p.Spec.Email.SMTP.OAuth2 = &v1.EmailSMTPOAuth2Spec{
			User: "u", TokenURL: "https://t", Scope: "s", ClientID: "c",
			ClientSecretRef: v1.SecretKeyRef{Name: "planton-email-oauth", Key: "client-secret"},
		}
		p.Spec.Email.SMTP.CABundleSecretRef = &v1.SecretKeyRef{Name: "corp-ca", Key: "ca.crt"}

		c := fake.NewClientBuilder().WithScheme(bindingScheme(t)).
			WithObjects(p, federationTestSecret("planton-email-oauth", map[string]string{"client-secret": "x"})).Build()
		msg, _ := preflightEmailSecrets(ctx, c, p)
		if !strings.Contains(msg, `Secret "corp-ca" not found`) || !strings.Contains(msg, "the relay's PEM CA bundle") || !strings.Contains(msg, "spec.email.smtp.caBundleSecretRef") {
			t.Errorf("the CA bundle Secret must be named with its purpose and field, got: %s", msg)
		}

		c = fake.NewClientBuilder().WithScheme(bindingScheme(t)).
			WithObjects(p,
				federationTestSecret("planton-email-oauth", map[string]string{"wrong-key": "x"}),
				federationTestSecret("corp-ca", map[string]string{"ca.crt": "x"})).Build()
		msg, _ = preflightEmailSecrets(ctx, c, p)
		if !strings.Contains(msg, `Secret "planton-email-oauth"`) || !strings.Contains(msg, `has no "client-secret" key`) || !strings.Contains(msg, "spec.email.smtp.oauth2") {
			t.Errorf("the OAuth2 Secret's missing key must be named with its field, got: %s", msg)
		}
	})

	t.Run("resend key is checked", func(t *testing.T) {
		p := testPlatform("prime")
		p.Spec.Email = &v1.EmailSpec{
			From:   v1.EmailFromSpec{Address: "x@y"},
			Resend: &v1.EmailResendSpec{APIKeySecretRef: v1.SecretKeyRef{Name: "planton-email", Key: "api-key"}},
		}
		c := fake.NewClientBuilder().WithScheme(bindingScheme(t)).WithObjects(p).Build()
		msg, _ := preflightEmailSecrets(ctx, c, p)
		if !strings.Contains(msg, "the Resend API key") || !strings.Contains(msg, "spec.email.resend") {
			t.Errorf("the Resend Secret must be named with its purpose and field, got: %s", msg)
		}
	})
}

// The component's config carries the email binding, and a preflight finding
// renders the pod as if no email were declared while the message names the
// Secret: the platform keeps running honestly, never a pod in FailedMount.
func TestBuildConfig_EmailBinding(t *testing.T) {
	cp := &ControlPlane{}
	p := ingressPlatform(true)

	if cfg := cp.buildConfig(p, nil); cfg.Email != nil {
		t.Errorf("no spec.email must leave the binding nil, got %+v", cfg.Email)
	}

	p.Spec.Email = smtpEmailSpec()
	cfg := cp.buildConfig(p, nil)
	if cfg.Email == nil || cfg.Email.Provider != resources.EmailProviderSMTP {
		t.Errorf("a declared smtp arm must reach the config, got %+v", cfg.Email)
	}
}
