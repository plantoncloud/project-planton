package resources

import (
	"strings"
	"testing"

	corev1 "k8s.io/api/core/v1"
)

func smtpPasswordEmailBinding() *EmailBinding {
	return &EmailBinding{
		Provider:    EmailProviderSMTP,
		FromAddress: "no-reply@planton.acme.com",
		FromName:    "Planton",
		ReplyTo:     "it-help@acme.com",
		SMTP: &EmailSMTPBinding{
			Host:                  "smtp.office365.com",
			Port:                  587,
			Security:              "starttls",
			CredentialsSecretName: "planton-email",
		},
	}
}

// No spec.email renders the provider as none OUT LOUD: the control plane's
// seam reads an unset provider as the hosted arm, so silence would mean
// "send like planton.ai". Nothing else about email is rendered, and no
// credentials volume exists for nothing to mount.
func TestControlPlaneDeployment_NoEmailRendersProviderNone(t *testing.T) {
	deploy := ControlPlaneDeployment(testControlPlaneConfig())
	envMap := envVarMap(deploy.Spec.Template.Spec.Containers[0].Env)

	if envMap["PLANTON_EMAIL_PROVIDER"] != EmailProviderNone {
		t.Errorf("PLANTON_EMAIL_PROVIDER = %q, want none on an install with no spec.email", envMap["PLANTON_EMAIL_PROVIDER"])
	}
	for name := range envMap {
		if strings.HasPrefix(name, "PLANTON_EMAIL_") && name != "PLANTON_EMAIL_PROVIDER" && !strings.HasPrefix(name, "PLANTON_EMAIL_SETUP_HINT_") {
			t.Errorf("%s must not be rendered when no email is declared", name)
		}
	}
	for _, v := range deploy.Spec.Template.Spec.Volumes {
		if v.Name == emailCredentialsVolumeName {
			t.Error("no credentials volume may be rendered when no Secret is referenced")
		}
	}
}

// A password-authenticated relay: the relay facts as env, the username and
// password as static FILE paths into the projected volume (never as env, so a
// rotated Secret is live without a pod roll), and the volume mapping the
// basic-auth Secret's conventional keys onto the operator's fixed file names.
func TestControlPlaneDeployment_EmailSMTPPassword(t *testing.T) {
	cfg := testControlPlaneConfig()
	cfg.Email = smtpPasswordEmailBinding()
	deploy := ControlPlaneDeployment(cfg)
	container := deploy.Spec.Template.Spec.Containers[0]
	envMap := envVarMap(container.Env)

	want := map[string]string{
		"PLANTON_EMAIL_PROVIDER":           "smtp",
		"PLANTON_EMAIL_FROM_ADDRESS":       "no-reply@planton.acme.com",
		"PLANTON_EMAIL_FROM_NAME":          "Planton",
		"PLANTON_EMAIL_REPLY_TO":           "it-help@acme.com",
		"PLANTON_EMAIL_SMTP_HOST":          "smtp.office365.com",
		"PLANTON_EMAIL_SMTP_PORT":          "587",
		"PLANTON_EMAIL_SMTP_SECURITY":      "starttls",
		"PLANTON_EMAIL_SMTP_USERNAME_FILE": "/etc/planton/email/smtp-username",
		"PLANTON_EMAIL_SMTP_PASSWORD_FILE": "/etc/planton/email/smtp-password",
	}
	for name, value := range want {
		if envMap[name] != value {
			t.Errorf("%s = %q, want %q", name, envMap[name], value)
		}
	}
	for _, absent := range []string{"PLANTON_EMAIL_SMTP_OAUTH2_USER", "PLANTON_EMAIL_SMTP_CA_BUNDLE_FILE", "PLANTON_EMAIL_RESEND_API_KEY_FILE"} {
		if _, ok := envMap[absent]; ok {
			t.Errorf("%s must not be rendered for a password-authenticated relay", absent)
		}
	}
	for _, e := range container.Env {
		if strings.HasPrefix(e.Name, "PLANTON_EMAIL_") && e.ValueFrom != nil {
			t.Errorf("%s must never be sourced from a Secret as env: credentials are files", e.Name)
		}
	}

	volume := findVolume(t, deploy.Spec.Template.Spec.Volumes, emailCredentialsVolumeName)
	if volume.Projected == nil || len(volume.Projected.Sources) != 1 {
		t.Fatalf("credentials volume must be one projected volume with one Secret source, got %+v", volume.VolumeSource)
	}
	source := volume.Projected.Sources[0].Secret
	if source == nil || source.Name != "planton-email" {
		t.Fatalf("source must be the basic-auth Secret, got %+v", volume.Projected.Sources[0])
	}
	assertProjectedItems(t, source.Items, map[string]string{"username": "smtp-username", "password": "smtp-password"})
	assertMount(t, container.VolumeMounts, emailCredentialsVolumeName, "/etc/planton/email")
}

// An OAuth2 relay behind a private CA: the grant facts as env, the client
// secret and the CA bundle as files from two different Secrets projected
// into the one volume, and no password files.
func TestControlPlaneDeployment_EmailSMTPOAuth2WithCABundle(t *testing.T) {
	cfg := testControlPlaneConfig()
	cfg.Email = &EmailBinding{
		Provider:    EmailProviderSMTP,
		FromAddress: "planton@acme.com",
		FromName:    "Planton",
		SMTP: &EmailSMTPBinding{
			Host:     "smtp.office365.com",
			Port:     587,
			Security: "starttls",
			OAuth2: &EmailSMTPOAuth2Binding{
				User:             "planton@acme.com",
				TokenURL:         "https://login.microsoftonline.com/tenant/oauth2/v2.0/token",
				Scope:            "https://outlook.office365.com/.default",
				ClientID:         "client-id",
				ClientSecretName: "planton-email-oauth",
				ClientSecretKey:  "client-secret",
			},
			CABundleSecretName: "corp-ca",
			CABundleSecretKey:  "ca.crt",
		},
	}
	deploy := ControlPlaneDeployment(cfg)
	container := deploy.Spec.Template.Spec.Containers[0]
	envMap := envVarMap(container.Env)

	want := map[string]string{
		"PLANTON_EMAIL_SMTP_OAUTH2_USER":               "planton@acme.com",
		"PLANTON_EMAIL_SMTP_OAUTH2_TOKEN_URL":          "https://login.microsoftonline.com/tenant/oauth2/v2.0/token",
		"PLANTON_EMAIL_SMTP_OAUTH2_SCOPE":              "https://outlook.office365.com/.default",
		"PLANTON_EMAIL_SMTP_OAUTH2_CLIENT_ID":          "client-id",
		"PLANTON_EMAIL_SMTP_OAUTH2_CLIENT_SECRET_FILE": "/etc/planton/email/smtp-oauth2-client-secret",
		"PLANTON_EMAIL_SMTP_CA_BUNDLE_FILE":            "/etc/planton/email/smtp-ca.crt",
	}
	for name, value := range want {
		if envMap[name] != value {
			t.Errorf("%s = %q, want %q", name, envMap[name], value)
		}
	}
	for _, absent := range []string{"PLANTON_EMAIL_SMTP_USERNAME_FILE", "PLANTON_EMAIL_SMTP_PASSWORD_FILE", "PLANTON_EMAIL_REPLY_TO"} {
		if _, ok := envMap[absent]; ok {
			t.Errorf("%s must not be rendered here", absent)
		}
	}

	volume := findVolume(t, deploy.Spec.Template.Spec.Volumes, emailCredentialsVolumeName)
	if len(volume.Projected.Sources) != 2 {
		t.Fatalf("two Secrets referenced must be two projected sources, got %d", len(volume.Projected.Sources))
	}
	assertProjectedItems(t, volume.Projected.Sources[0].Secret.Items, map[string]string{"client-secret": "smtp-oauth2-client-secret"})
	assertProjectedItems(t, volume.Projected.Sources[1].Secret.Items, map[string]string{"ca.crt": "smtp-ca.crt"})
}

// An unauthenticated internal relay references no Secret: relay facts only,
// no file paths, no volume.
func TestControlPlaneDeployment_EmailSMTPNoCredentials(t *testing.T) {
	cfg := testControlPlaneConfig()
	cfg.Email = &EmailBinding{
		Provider:    EmailProviderSMTP,
		FromAddress: "planton@acme.com",
		FromName:    "Planton",
		SMTP:        &EmailSMTPBinding{Host: "smtp-relay.corp.acme.com", Port: 25, Security: "none"},
	}
	deploy := ControlPlaneDeployment(cfg)
	envMap := envVarMap(deploy.Spec.Template.Spec.Containers[0].Env)

	if envMap["PLANTON_EMAIL_SMTP_SECURITY"] != "none" || envMap["PLANTON_EMAIL_SMTP_PORT"] != "25" {
		t.Errorf("relay facts must render verbatim, got security=%q port=%q", envMap["PLANTON_EMAIL_SMTP_SECURITY"], envMap["PLANTON_EMAIL_SMTP_PORT"])
	}
	for name := range envMap {
		if strings.HasSuffix(name, "_FILE") && strings.HasPrefix(name, "PLANTON_EMAIL_") {
			t.Errorf("%s must not be rendered when no Secret is referenced", name)
		}
	}
	for _, v := range deploy.Spec.Template.Spec.Volumes {
		if v.Name == emailCredentialsVolumeName {
			t.Error("no credentials volume may be rendered when no Secret is referenced")
		}
	}
}

// The Resend arm: provider and sender identity as env, the API key as a file.
func TestControlPlaneDeployment_EmailResend(t *testing.T) {
	cfg := testControlPlaneConfig()
	cfg.Email = &EmailBinding{
		Provider:    EmailProviderResend,
		FromAddress: "no-reply@planton.acme.com",
		FromName:    "Planton",
		Resend:      &EmailResendBinding{APIKeySecretName: "planton-email", APIKeySecretKey: "api-key"},
	}
	deploy := ControlPlaneDeployment(cfg)
	container := deploy.Spec.Template.Spec.Containers[0]
	envMap := envVarMap(container.Env)

	if envMap["PLANTON_EMAIL_PROVIDER"] != "resend" {
		t.Errorf("PLANTON_EMAIL_PROVIDER = %q, want resend", envMap["PLANTON_EMAIL_PROVIDER"])
	}
	if envMap["PLANTON_EMAIL_RESEND_API_KEY_FILE"] != "/etc/planton/email/resend-api-key" {
		t.Errorf("PLANTON_EMAIL_RESEND_API_KEY_FILE = %q", envMap["PLANTON_EMAIL_RESEND_API_KEY_FILE"])
	}
	if _, ok := envMap["PLANTON_EMAIL_SMTP_HOST"]; ok {
		t.Error("no SMTP fact may be rendered on the Resend arm")
	}
	volume := findVolume(t, deploy.Spec.Template.Spec.Volumes, emailCredentialsVolumeName)
	assertProjectedItems(t, volume.Projected.Sources[0].Secret.Items, map[string]string{"api-key": "resend-api-key"})
}

// The setup hints carry the platform's real name and namespace so the console
// never hardcodes deployment names, and they are rendered on every install
// (the page decides when to show them).
func TestControlPlaneDeployment_EmailSetupHints(t *testing.T) {
	const hintNamespace = "acme-platform"
	cfg := testControlPlaneConfig()
	cfg.CRName, cfg.Namespace = "prime", hintNamespace
	deploy := ControlPlaneDeployment(cfg)
	envMap := envVarMap(deploy.Spec.Template.Spec.Containers[0].Env)

	manifest := envMap["PLANTON_EMAIL_SETUP_HINT_MANIFEST"]
	for _, needle := range []string{"kubectl -n " + hintNamespace + " edit plantonplatform prime", "spec:\n  email:", "credentialsSecretName: planton-email", "security: starttls"} {
		if !strings.Contains(manifest, needle) {
			t.Errorf("manifest hint must contain %q, got:\n%s", needle, manifest)
		}
	}
	command := envMap["PLANTON_EMAIL_SETUP_HINT_SECRET_COMMAND"]
	for _, needle := range []string{"kubectl -n " + hintNamespace + " create secret generic planton-email", "--type=kubernetes.io/basic-auth", "--from-literal=username=", "--from-literal=password="} {
		if !strings.Contains(command, needle) {
			t.Errorf("secret command hint must contain %q, got: %s", needle, command)
		}
	}
	if strings.Contains(manifest, "planton-system") || strings.Contains(command, "planton-system") {
		t.Error("hints must carry the platform's own namespace, never a default one")
	}
}

func findVolume(t *testing.T, volumes []corev1.Volume, name string) corev1.Volume {
	t.Helper()
	for _, v := range volumes {
		if v.Name == name {
			return v
		}
	}
	t.Fatalf("volume %q not rendered", name)
	return corev1.Volume{}
}

func assertProjectedItems(t *testing.T, items []corev1.KeyToPath, want map[string]string) {
	t.Helper()
	if len(items) != len(want) {
		t.Fatalf("projected items = %+v, want %d mappings %v", items, len(want), want)
	}
	for _, item := range items {
		if want[item.Key] != item.Path {
			t.Errorf("Secret key %q projected to %q, want %q", item.Key, item.Path, want[item.Key])
		}
	}
}

func assertMount(t *testing.T, mounts []corev1.VolumeMount, name, mountPath string) {
	t.Helper()
	for _, m := range mounts {
		if m.Name != name {
			continue
		}
		if m.MountPath != mountPath || !m.ReadOnly || m.SubPath != "" {
			t.Errorf("mount %s = %+v, want read-only whole-directory mount at %s (a subPath mount freezes at pod start)", name, m, mountPath)
		}
		return
	}
	t.Errorf("mount %q not rendered", name)
}
