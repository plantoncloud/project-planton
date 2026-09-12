package component

import (
	"context"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	v1 "github.com/plantonhq/planton/operator/api/v1"
	"github.com/plantonhq/planton/operator/internal/resources"
)

// effectiveEmail resolves spec.email into the delivery binding. Pure: the
// CRD's rules already guarantee exactly one arm and one way in, so this is a
// translation, never a judgment. nil when no email is declared -- the renderer
// then says PLANTON_EMAIL_PROVIDER=none out loud, because the control plane's
// seam reads an unset provider as the hosted arm. Whether the relay accepts
// mail is deliberately NOT decided here: the control plane checks on demand
// and answers in the relay's own words.
func effectiveEmail(planton *v1.PlantonPlatform) *resources.EmailBinding {
	e := planton.Spec.Email
	if e == nil {
		return nil
	}
	binding := &resources.EmailBinding{
		FromAddress: e.From.Address,
		FromName:    e.From.Name,
		ReplyTo:     e.ReplyTo,
	}
	switch {
	case e.SMTP != nil:
		binding.Provider = resources.EmailProviderSMTP
		binding.SMTP = &resources.EmailSMTPBinding{
			Host:                  e.SMTP.Host,
			Port:                  e.SMTP.Port,
			Security:              string(e.SMTP.Security),
			CredentialsSecretName: e.SMTP.CredentialsSecretName,
		}
		if o := e.SMTP.OAuth2; o != nil {
			binding.SMTP.OAuth2 = &resources.EmailSMTPOAuth2Binding{
				User:             o.User,
				TokenURL:         o.TokenURL,
				Scope:            o.Scope,
				ClientID:         o.ClientID,
				ClientSecretName: o.ClientSecretRef.Name,
				ClientSecretKey:  o.ClientSecretRef.Key,
			}
		}
		if ca := e.SMTP.CABundleSecretRef; ca != nil {
			binding.SMTP.CABundleSecretName = ca.Name
			binding.SMTP.CABundleSecretKey = ca.Key
		}
	case e.Resend != nil:
		binding.Provider = resources.EmailProviderResend
		binding.Resend = &resources.EmailResendBinding{
			APIKeySecretName: e.Resend.APIKeySecretRef.Name,
			APIKeySecretKey:  e.Resend.APIKeySecretRef.Key,
		}
	default:
		// Unreachable through the API server (the CRD requires one arm);
		// reachable only through a definition older than this operator.
		// Declared with no arm is declared nothing.
		return nil
	}
	return binding
}

// preflightEmailSecrets verifies that every Secret spec.email references
// exists and carries the keys the credentials volume will project, BEFORE the
// Deployment is rendered: a projected item whose Secret or key is missing
// leaves the pod in FailedMount, a state nobody can read a reason from. On a
// finding it returns the sentence for status -- what was declared, what is
// missing, the two ways out -- and the caller renders the pod as if no email
// were declared, so the platform keeps running honestly while the person
// fixes the manifest or the Secret. Existence and keys only: the values are
// never read here (the identity-federation reader is the one place the
// operator reads a user Secret's bytes, and it has a reason to).
func preflightEmailSecrets(ctx context.Context, c client.Client, planton *v1.PlantonPlatform) (string, error) {
	e := planton.Spec.Email
	if e == nil {
		return "", nil
	}
	ns := planton.Namespace

	if smtp := e.SMTP; smtp != nil {
		if name := smtp.CredentialsSecretName; name != "" {
			msg, err := preflightSecretKeys(ctx, c, ns, name, emailSecretPreflight("spec.email.smtp.credentialsSecretName", "type kubernetes.io/basic-auth", resources.BasicAuthUsernameKey, resources.BasicAuthPasswordKey))
			if msg != "" || err != nil {
				return msg, err
			}
		}
		if o := smtp.OAuth2; o != nil {
			msg, err := preflightSecretKeys(ctx, c, ns, o.ClientSecretRef.Name, emailSecretPreflight("spec.email.smtp.oauth2", "the OAuth2 client secret", o.ClientSecretRef.Key))
			if msg != "" || err != nil {
				return msg, err
			}
		}
		if ca := smtp.CABundleSecretRef; ca != nil {
			msg, err := preflightSecretKeys(ctx, c, ns, ca.Name, emailSecretPreflight("spec.email.smtp.caBundleSecretRef", "the relay's PEM CA bundle", ca.Key))
			if msg != "" || err != nil {
				return msg, err
			}
		}
	}
	if r := e.Resend; r != nil {
		msg, err := preflightSecretKeys(ctx, c, ns, r.APIKeySecretRef.Name, emailSecretPreflight("spec.email.resend", "the Resend API key", r.APIKeySecretRef.Key))
		if msg != "" || err != nil {
			return msg, err
		}
	}
	return "", nil
}

// secretPreflight names one referenced Secret in the words a refusal uses:
// what the Secret is (the noun a person recognises), the spec field that
// references it, what it must hold, and what the platform does until it
// exists. One shape for every credential-by-reference on the resource --
// email relays and GitHub Apps today -- so a missing Secret reads the same
// wherever it is missing.
type secretPreflight struct {
	// Noun is what the Secret is called in the sentence ("email credentials
	// Secret", "GitHub App Secret").
	Noun string
	// Field is the spec path that references the Secret.
	Field string
	// Contents describes what the Secret must hold, in the words the fix uses.
	Contents string
	// Consequence is what the platform does until the Secret exists.
	Consequence string
	// Keys are the data keys the volume will project.
	Keys []string
}

// emailSecretPreflight is the email relay's shape of the sentence.
func emailSecretPreflight(field, contents string, keys ...string) secretPreflight {
	return secretPreflight{
		Noun: "email credentials Secret", Field: field, Contents: contents,
		Consequence: "the platform runs as if no email were declared", Keys: keys,
	}
}

// preflightSecretKeys reports, in one sentence, a referenced Secret that is
// absent or that lacks a key the volume will project.
func preflightSecretKeys(ctx context.Context, c client.Client, namespace, name string, want secretPreflight) (string, error) {
	var secret corev1.Secret
	err := c.Get(ctx, types.NamespacedName{Name: name, Namespace: namespace}, &secret)
	if apierrors.IsNotFound(err) {
		return fmt.Sprintf(
			"%s %q not found in namespace %q; create it (%s) or remove %s -- until then %s",
			want.Noun, name, namespace, want.Contents, want.Field, want.Consequence), nil
	}
	if err != nil {
		return "", fmt.Errorf("checking %s %s: %w", want.Noun, name, err)
	}
	for _, key := range want.Keys {
		if _, ok := secret.Data[key]; !ok {
			return fmt.Sprintf(
				"%s %q in namespace %q has no %q key; add it (%s) or remove %s -- until then %s",
				want.Noun, name, namespace, key, want.Contents, want.Field, want.Consequence), nil
		}
	}
	return "", nil
}
