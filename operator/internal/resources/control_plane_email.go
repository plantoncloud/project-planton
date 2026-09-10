package resources

import (
	"fmt"
	"path"
	"strconv"

	corev1 "k8s.io/api/core/v1"
)

// Email provider values rendered as PLANTON_EMAIL_PROVIDER. They are the
// control plane's own vocabulary for its email-delivery seam (the arm that
// sends), so the operator spells them exactly as the seam reads them. "none"
// is rendered explicitly on an install with no spec.email: the seam resolves
// an UNSET provider to the hosted arm, so silence would mean "send like
// planton.ai", never "send nothing".
const (
	EmailProviderNone   = "none"
	EmailProviderSMTP   = "smtp"
	EmailProviderResend = "resend"
)

// The credentials volume. Every secret value spec.email references reaches
// the control plane as a FILE under one directory, never as an environment
// variable: env is read at container start and rolls the pod when it
// changes, while projected content updates in place -- so a rotated relay
// password or API key is live on the next send with no restart and no
// operator watch (the identity-federation facts file precedent). The file
// names are fixed by the operator, whatever keys the adopter's Secret uses,
// so the control plane's readers never learn a Secret's layout.
const (
	// EmailCredentialsMountPath is the directory the credentials volume is
	// mounted at inside the control-plane container.
	EmailCredentialsMountPath = "/etc/planton/email"

	emailCredentialsVolumeName = "email-credentials"

	EmailSMTPUsernameFileName           = "smtp-username"
	EmailSMTPPasswordFileName           = "smtp-password"
	EmailSMTPOAuth2ClientSecretFileName = "smtp-oauth2-client-secret"
	EmailSMTPCABundleFileName           = "smtp-ca.crt"
	EmailResendAPIKeyFileName           = "resend-api-key"

	// The keys a kubernetes.io/basic-auth Secret carries by convention.
	basicAuthUsernameKey = "username"
	basicAuthPasswordKey = "password"
)

// EmailBinding is the resolved spec.email: which arm sends, the sender
// identity every email carries, and the arm's own settings. nil means no
// email is declared, and the renderer says so out loud (PLANTON_EMAIL_PROVIDER
// = none). The operator is deliberately dumb about the relay itself: no
// probe, no send, no verdict -- the control plane checks the connection on
// demand from its settings page and reports each failure in the relay's own
// words, so a bad password fails with a precise message there instead of a
// vague one here.
type EmailBinding struct {
	// Provider is one of the EmailProvider* values.
	Provider string

	// FromAddress and FromName are the sender identity; ReplyTo is where a
	// reply lands (empty = the sending address).
	FromAddress string
	FromName    string
	ReplyTo     string

	// SMTP is set when Provider is smtp.
	SMTP *EmailSMTPBinding

	// Resend is set when Provider is resend.
	Resend *EmailResendBinding
}

// EmailSMTPBinding is the relay and the one way in (password, OAuth2 token,
// or none) -- exclusivity is the CRD's rule and the component's job; the
// renderer emits what is present.
type EmailSMTPBinding struct {
	Host     string
	Port     int32
	Security string

	// CredentialsSecretName names the kubernetes.io/basic-auth Secret; empty
	// when the relay needs no password.
	CredentialsSecretName string

	// OAuth2 is set when the relay is entered with a client-credentials
	// token instead of a password.
	OAuth2 *EmailSMTPOAuth2Binding

	// CABundleSecretName/CABundleSecretKey reference the PEM bundle for a
	// relay behind a private CA; empty when the certificate chains to a
	// public root.
	CABundleSecretName string
	CABundleSecretKey  string
}

// EmailSMTPOAuth2Binding is the client-credentials grant behind an XOAUTH2
// sign-in; the client secret rides the credentials volume.
type EmailSMTPOAuth2Binding struct {
	User     string
	TokenURL string
	Scope    string
	ClientID string

	ClientSecretName string
	ClientSecretKey  string
}

// EmailResendBinding references the Resend API key.
type EmailResendBinding struct {
	APIKeySecretName string
	APIKeySecretKey  string
}

// emailEnvVars renders the control plane's email arm. The provider is ALWAYS
// rendered (none included); the sender identity and the arm's settings only
// when declared; every *_FILE variable is a static path into the credentials
// volume and appears only when its Secret is referenced, so the control
// plane's readers stay inert for anything the manifest did not declare.
func emailEnvVars(binding *EmailBinding) []corev1.EnvVar {
	if binding == nil {
		return []corev1.EnvVar{{Name: "PLANTON_EMAIL_PROVIDER", Value: EmailProviderNone}}
	}

	envs := []corev1.EnvVar{
		{Name: "PLANTON_EMAIL_PROVIDER", Value: binding.Provider},
		{Name: "PLANTON_EMAIL_FROM_ADDRESS", Value: binding.FromAddress},
		{Name: "PLANTON_EMAIL_FROM_NAME", Value: binding.FromName},
	}
	if binding.ReplyTo != "" {
		envs = append(envs, corev1.EnvVar{Name: "PLANTON_EMAIL_REPLY_TO", Value: binding.ReplyTo})
	}

	if smtp := binding.SMTP; smtp != nil {
		envs = append(envs,
			corev1.EnvVar{Name: "PLANTON_EMAIL_SMTP_HOST", Value: smtp.Host},
			corev1.EnvVar{Name: "PLANTON_EMAIL_SMTP_PORT", Value: strconv.Itoa(int(smtp.Port))},
			corev1.EnvVar{Name: "PLANTON_EMAIL_SMTP_SECURITY", Value: smtp.Security},
		)
		if smtp.CredentialsSecretName != "" {
			envs = append(envs,
				corev1.EnvVar{Name: "PLANTON_EMAIL_SMTP_USERNAME_FILE", Value: EmailCredentialFilePath(EmailSMTPUsernameFileName)},
				corev1.EnvVar{Name: "PLANTON_EMAIL_SMTP_PASSWORD_FILE", Value: EmailCredentialFilePath(EmailSMTPPasswordFileName)},
			)
		}
		if o := smtp.OAuth2; o != nil {
			envs = append(envs,
				corev1.EnvVar{Name: "PLANTON_EMAIL_SMTP_OAUTH2_USER", Value: o.User},
				corev1.EnvVar{Name: "PLANTON_EMAIL_SMTP_OAUTH2_TOKEN_URL", Value: o.TokenURL},
				corev1.EnvVar{Name: "PLANTON_EMAIL_SMTP_OAUTH2_SCOPE", Value: o.Scope},
				corev1.EnvVar{Name: "PLANTON_EMAIL_SMTP_OAUTH2_CLIENT_ID", Value: o.ClientID},
				corev1.EnvVar{Name: "PLANTON_EMAIL_SMTP_OAUTH2_CLIENT_SECRET_FILE", Value: EmailCredentialFilePath(EmailSMTPOAuth2ClientSecretFileName)},
			)
		}
		if smtp.CABundleSecretName != "" {
			envs = append(envs, corev1.EnvVar{Name: "PLANTON_EMAIL_SMTP_CA_BUNDLE_FILE", Value: EmailCredentialFilePath(EmailSMTPCABundleFileName)})
		}
	}

	if binding.Resend != nil {
		envs = append(envs, corev1.EnvVar{Name: "PLANTON_EMAIL_RESEND_API_KEY_FILE", Value: EmailCredentialFilePath(EmailResendAPIKeyFileName)})
	}

	return envs
}

// EmailCredentialFilePath is where a named credential file lives inside the
// control-plane container.
func EmailCredentialFilePath(fileName string) string {
	return path.Join(EmailCredentialsMountPath, fileName)
}

// emailCredentialsVolume renders the one projected volume that carries every
// secret value spec.email references, each Secret a source and each key
// mapped to its fixed file name, plus its read-only mount. nil when the
// declaration references no Secret (an unauthenticated relay, or no email at
// all): no volume is rendered for nothing to mount. Whole-directory, never
// subPath -- a subPath mount freezes at pod start and would defeat live
// rotation.
func emailCredentialsVolume(binding *EmailBinding) (*corev1.Volume, *corev1.VolumeMount) {
	sources := emailCredentialSources(binding)
	if len(sources) == 0 {
		return nil, nil
	}
	volume := &corev1.Volume{
		Name: emailCredentialsVolumeName,
		VolumeSource: corev1.VolumeSource{
			Projected: &corev1.ProjectedVolumeSource{Sources: sources},
		},
	}
	mount := &corev1.VolumeMount{
		Name:      emailCredentialsVolumeName,
		MountPath: EmailCredentialsMountPath,
		ReadOnly:  true,
	}
	return volume, mount
}

func emailCredentialSources(binding *EmailBinding) []corev1.VolumeProjection {
	if binding == nil {
		return nil
	}
	var sources []corev1.VolumeProjection
	if smtp := binding.SMTP; smtp != nil {
		if smtp.CredentialsSecretName != "" {
			sources = append(sources, secretProjection(smtp.CredentialsSecretName,
				corev1.KeyToPath{Key: basicAuthUsernameKey, Path: EmailSMTPUsernameFileName},
				corev1.KeyToPath{Key: basicAuthPasswordKey, Path: EmailSMTPPasswordFileName},
			))
		}
		if o := smtp.OAuth2; o != nil {
			sources = append(sources, secretProjection(o.ClientSecretName,
				corev1.KeyToPath{Key: o.ClientSecretKey, Path: EmailSMTPOAuth2ClientSecretFileName}))
		}
		if smtp.CABundleSecretName != "" {
			sources = append(sources, secretProjection(smtp.CABundleSecretName,
				corev1.KeyToPath{Key: smtp.CABundleSecretKey, Path: EmailSMTPCABundleFileName}))
		}
	}
	if r := binding.Resend; r != nil {
		sources = append(sources, secretProjection(r.APIKeySecretName,
			corev1.KeyToPath{Key: r.APIKeySecretKey, Path: EmailResendAPIKeyFileName}))
	}
	return sources
}

func secretProjection(secretName string, items ...corev1.KeyToPath) corev1.VolumeProjection {
	return corev1.VolumeProjection{
		Secret: &corev1.SecretProjection{
			LocalObjectReference: corev1.LocalObjectReference{Name: secretName},
			Items:                items,
		},
	}
}

// emailSetupHintEnvVars carries the two copyable artifacts the console's
// Email settings page shows while nothing is configured: the manifest
// snippet and the Secret command, each its own variable because each is its
// own copy button. Static per platform (name and namespace never change), so
// they never roll the pod.
func emailSetupHintEnvVars(crName, namespace string) []corev1.EnvVar {
	return []corev1.EnvVar{
		{Name: "PLANTON_EMAIL_SETUP_HINT_MANIFEST", Value: EmailSetupHintManifest(crName, namespace)},
		{Name: "PLANTON_EMAIL_SETUP_HINT_SECRET_COMMAND", Value: EmailSetupHintSecretCommand(namespace)},
	}
}

// EmailSetupHintExampleSecretName is the Secret name the setup hint proposes;
// the person configuring email may pick any other.
const EmailSetupHintExampleSecretName = "planton-email"

// EmailSetupHintManifest renders the spec.email snippet an administrator
// adds to THIS platform resource -- with its real name and namespace, so the
// console's Email settings page shows a copyable declaration and never
// hardcodes deployment names (the setup-code hint precedent). Rendered on
// every install; the page shows it only while email is not configured.
func EmailSetupHintManifest(crName, namespace string) string {
	return fmt.Sprintf(`# kubectl -n %s edit plantonplatform %s
spec:
  email:
    from:
      address: no-reply@example.com      # the address this install sends as
      name: Planton
    smtp:
      host: smtp.example.com
      port: 587
      security: starttls                 # starttls | tls | none
      credentialsSecretName: %s
`, namespace, crName, EmailSetupHintExampleSecretName)
}

// EmailSetupHintSecretCommand renders the one command that creates the
// relay credentials Secret the snippet references, in the platform's
// namespace and in the Secret type the CRD expects.
func EmailSetupHintSecretCommand(namespace string) string {
	return fmt.Sprintf(
		"kubectl -n %s create secret generic %s --type=kubernetes.io/basic-auth --from-literal=username=<relay-username> --from-literal=password=<relay-password>",
		namespace, EmailSetupHintExampleSecretName)
}
