package component

import (
	"context"
	"fmt"

	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	v1 "github.com/plantonhq/planton/operator/api/v1"
	"github.com/plantonhq/planton/operator/internal/keycloak"
	"github.com/plantonhq/planton/operator/internal/resources"
)

// The email half of the identity component: the platform's one email
// declaration (spec.email) becomes the identity server's own mail settings,
// so a "Forgot password?" on the sign-in page sends through the same relay
// the control plane sends invitations through. The control plane reads the
// declaration as environment and mounted files; the identity server has no
// such seam, so the operator reads the credential's VALUE here (the
// directory bind credential's precedent) and hands it to the realm through
// the reconciler, fingerprinting it so a rotated Secret is written once and
// a steady one never.

// Resend's SMTP endpoint: a platform on the Resend arm gives the identity
// server Resend's relay with the API key as the password, so one declaration
// powers both senders. Implicit TLS on 465 deliberately -- Keycloak's
// STARTTLS is an opportunistic upgrade, and the relay offers the stricter
// door.
const (
	resendSMTPHost = "smtp.resend.com"
	resendSMTPPort = 465
	resendSMTPUser = "resend"
)

// emailRealmBuild is one pass's translation of spec.email for the realm plus
// the bookkeeping the rest of the pass needs.
type emailRealmBuild struct {
	// email is the reconciler's desired state on its nil-vs-empty contract:
	// nil hands the realm's email settings off this pass, a value with no
	// relay owns them off, a relay owns them as declared.
	email *keycloak.OwnedRealmEmail

	// credentialSHA fingerprints the secret handed to the realm (empty for a
	// relay that needs none), recorded after a clean convergence.
	credentialSHA string

	// The relay's private CA, carried into the identity Deployment's
	// truststore (the directory CA's shape).
	caBundleSecretName string
	caBundleSecretKey  string
	caBundleHash       string
}

// applyTruststore carries the relay's private CA into the identity
// Deployment's truststore configuration (a changed bundle rolls the server).
func (b *emailRealmBuild) applyTruststore(cfg *resources.IdentityConfig) {
	if b.caBundleSecretName == "" {
		return
	}
	cfg.EmailCABundleSecretName = b.caBundleSecretName
	cfg.EmailCABundleSecretKey = b.caBundleSecretKey
	cfg.EmailCABundleHash = b.caBundleHash
}

// advanceRecord moves the record's email fingerprint to what this pass put
// on the realm. A hands-off pass (nil email) leaves the recorded value alone,
// so the next pass still knows what the realm holds.
func (b *emailRealmBuild) advanceRecord(record *realmStateRecord) {
	if b.email == nil {
		return
	}
	record.EmailCredentialSHA = b.credentialSHA
}

// buildEmailRealmState translates spec.email into the realm's desired email
// state. It never fails the pass: a referenced Secret that cannot be read is
// logged and the realm's email settings are left untouched this pass -- a
// transient gap must never flip the sign-in page, and the control plane
// already reports the same finding as its own not-Ready message, so this
// component does not add a second voice (and must not go not-Ready itself:
// the control plane waits on this component).
func (id *Identity) buildEmailRealmState(ctx context.Context, c client.Client, planton *v1.PlantonPlatform, recorded realmStateRecord) *emailRealmBuild {
	log := logf.FromContext(ctx).WithValues("component", id.Name())

	binding := effectiveEmail(planton)
	if binding == nil {
		// Nothing declared: the realm carries no relay and password reset
		// is off, owned that way so a relay typed into the identity server's
		// admin console cannot contradict what the product advertises.
		return &emailRealmBuild{email: &keycloak.OwnedRealmEmail{}}
	}

	finding, err := preflightEmailSecrets(ctx, c, planton)
	if err != nil {
		log.Error(err, "Email Secrets could not be checked; the identity server's email settings are left as they are this pass")
		return &emailRealmBuild{}
	}
	if finding != "" {
		log.Info("Email is declared but a referenced Secret is not ready; the identity server's email settings are left as they are this pass", "finding", finding)
		return &emailRealmBuild{}
	}

	relay := &keycloak.OwnedSMTPRelay{
		From:            binding.FromAddress,
		FromDisplayName: binding.FromName,
		ReplyTo:         binding.ReplyTo,
	}
	build := &emailRealmBuild{}
	var credential string

	switch binding.Provider {
	case resources.EmailProviderSMTP:
		smtp := binding.SMTP
		relay.Host, relay.Port, relay.Security = smtp.Host, smtp.Port, smtp.Security
		switch {
		case smtp.CredentialsSecretName != "":
			username, err := readSecretKey(ctx, c, planton.Namespace, v1.SecretKeyRef{Name: smtp.CredentialsSecretName, Key: resources.BasicAuthUsernameKey})
			if err != nil {
				return handsOffEmail(log, smtp.CredentialsSecretName, resources.BasicAuthUsernameKey, err)
			}
			password, err := readSecretKey(ctx, c, planton.Namespace, v1.SecretKeyRef{Name: smtp.CredentialsSecretName, Key: resources.BasicAuthPasswordKey})
			if err != nil {
				return handsOffEmail(log, smtp.CredentialsSecretName, resources.BasicAuthPasswordKey, err)
			}
			relay.Auth, relay.User, relay.Password = keycloak.OwnedSMTPAuthPassword, username, password
			credential = password
		case smtp.OAuth2 != nil:
			o := smtp.OAuth2
			clientSecret, err := readSecretKey(ctx, c, planton.Namespace, v1.SecretKeyRef{Name: o.ClientSecretName, Key: o.ClientSecretKey})
			if err != nil {
				return handsOffEmail(log, o.ClientSecretName, o.ClientSecretKey, err)
			}
			relay.Auth, relay.User = keycloak.OwnedSMTPAuthToken, o.User
			relay.TokenURL, relay.TokenScope, relay.TokenClientID, relay.TokenClientSecret = o.TokenURL, o.Scope, o.ClientID, clientSecret
			credential = clientSecret
		default:
			relay.Auth = keycloak.OwnedSMTPAuthNone
		}
		if smtp.CABundleSecretName != "" {
			caBundle, err := readSecretKey(ctx, c, planton.Namespace, v1.SecretKeyRef{Name: smtp.CABundleSecretName, Key: smtp.CABundleSecretKey})
			if err != nil {
				return handsOffEmail(log, smtp.CABundleSecretName, smtp.CABundleSecretKey, err)
			}
			build.caBundleSecretName, build.caBundleSecretKey, build.caBundleHash = smtp.CABundleSecretName, smtp.CABundleSecretKey, sha256Hex(caBundle)
		}
	case resources.EmailProviderResend:
		apiKey, err := readSecretKey(ctx, c, planton.Namespace, v1.SecretKeyRef{Name: binding.Resend.APIKeySecretName, Key: binding.Resend.APIKeySecretKey})
		if err != nil {
			return handsOffEmail(log, binding.Resend.APIKeySecretName, binding.Resend.APIKeySecretKey, err)
		}
		relay.Host, relay.Port, relay.Security = resendSMTPHost, resendSMTPPort, string(v1.EmailSMTPSecurityTLS)
		relay.Auth, relay.User, relay.Password = keycloak.OwnedSMTPAuthPassword, resendSMTPUser, apiKey
		credential = apiKey
	default:
		// Unreachable through the API server (the CRD requires one arm).
		return &emailRealmBuild{email: &keycloak.OwnedRealmEmail{}}
	}

	if credential != "" {
		build.credentialSHA = sha256Hex(credential)
	}
	build.email = &keycloak.OwnedRealmEmail{
		Relay: relay,
		// The realm holds the credential masked; the record is the only way
		// to know it moved. A relay with no credential never rotates.
		RotateCredential: credential != "" && recorded.EmailCredentialSHA != build.credentialSHA,
	}
	return build
}

// handsOffEmail is the one sentence for a referenced Secret that passed the
// preflight but could not be read (a race with its deletion, a permission
// gap): the realm's email settings stay as they are this pass. The value is
// never in the message.
func handsOffEmail(log interface{ Info(string, ...any) }, secretName, key string, err error) *emailRealmBuild {
	log.Info(fmt.Sprintf("the email credential could not be read from Secret %s (key %s); the identity server's email settings are left as they are this pass", secretName, key), "error", err.Error())
	return &emailRealmBuild{}
}
