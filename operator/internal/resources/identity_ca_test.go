package resources

import (
	"encoding/json"
	"testing"

	corev1 "k8s.io/api/core/v1"
)

const (
	testCAKey         = "ca.crt"
	testEmailCASecret = "corp-ca"
	testDirCASecret   = "corp-dir-ca"
	testDirCAHash     = "dirhash"
	testEmailCAHash   = "mailhash"
)

// truststorePaths returns KC_TRUSTSTORE_PATHS and whether it is set.
func truststorePaths(env []corev1.EnvVar) (string, bool) {
	for _, e := range env {
		if e.Name == "KC_TRUSTSTORE_PATHS" {
			return e.Value, true
		}
	}
	return "", false
}

func identityVolume(t *testing.T, volumes []corev1.Volume, name string) *corev1.Volume {
	t.Helper()
	for i := range volumes {
		if volumes[i].Name == name {
			return &volumes[i]
		}
	}
	return nil
}

// The two private CAs the identity server may need to trust -- the
// directory's and the mail relay's -- each ride their own mount with their
// own restart hash, and KC_TRUSTSTORE_PATHS lists exactly the ones declared.
func TestIdentityDeployment_PrivateCATruststore(t *testing.T) {
	t.Run("no CA: no truststore variable, no CA volume, no CA annotation", func(t *testing.T) {
		deploy := IdentityDeployment(testIdentityConfig())
		if _, set := truststorePaths(deploy.Spec.Template.Spec.Containers[0].Env); set {
			t.Error("KC_TRUSTSTORE_PATHS must not be set without a declared CA (the JVM's default roots suffice)")
		}
		for _, name := range []string{"ldap-ca", "email-ca"} {
			if identityVolume(t, deploy.Spec.Template.Spec.Volumes, name) != nil {
				t.Errorf("volume %s rendered with nothing to mount", name)
			}
		}
		for _, ann := range []string{IdentityCABundleHashAnnotation, IdentityEmailCAHashAnnotation} {
			if _, ok := deploy.Spec.Template.Annotations[ann]; ok {
				t.Errorf("annotation %s rendered with no CA", ann)
			}
		}
	})

	t.Run("directory CA only", func(t *testing.T) {
		cfg := testIdentityConfig()
		cfg.CABundleSecretName, cfg.CABundleSecretKey, cfg.CABundleHash = testDirCASecret, testCAKey, testDirCAHash
		deploy := IdentityDeployment(cfg)
		got, _ := truststorePaths(deploy.Spec.Template.Spec.Containers[0].Env)
		if got != IdentityCATruststorePath+"/"+IdentityCABundleFileName {
			t.Errorf("KC_TRUSTSTORE_PATHS = %q", got)
		}
		if deploy.Spec.Template.Annotations[IdentityCABundleHashAnnotation] != testDirCAHash {
			t.Error("the directory CA hash must ride the pod annotation")
		}
		if identityVolume(t, deploy.Spec.Template.Spec.Volumes, "email-ca") != nil {
			t.Error("no email CA declared, no email CA volume")
		}
	})

	t.Run("mail relay CA only", func(t *testing.T) {
		cfg := testIdentityConfig()
		cfg.EmailCABundleSecretName, cfg.EmailCABundleSecretKey, cfg.EmailCABundleHash = testEmailCASecret, testCAKey, testEmailCAHash
		deploy := IdentityDeployment(cfg)
		got, _ := truststorePaths(deploy.Spec.Template.Spec.Containers[0].Env)
		if got != IdentityEmailCATruststorePath+"/"+IdentityEmailCABundleFileName {
			t.Errorf("KC_TRUSTSTORE_PATHS = %q", got)
		}
		if deploy.Spec.Template.Annotations[IdentityEmailCAHashAnnotation] != testEmailCAHash {
			t.Error("the relay CA hash must ride the pod annotation (a changed bundle rolls the server)")
		}
		vol := identityVolume(t, deploy.Spec.Template.Spec.Volumes, "email-ca")
		if vol == nil || vol.Secret == nil || vol.Secret.SecretName != testEmailCASecret ||
			len(vol.Secret.Items) != 1 || vol.Secret.Items[0].Key != testCAKey || vol.Secret.Items[0].Path != IdentityEmailCABundleFileName {
			t.Errorf("email-ca volume = %+v", vol)
		}
		if identityVolume(t, deploy.Spec.Template.Spec.Volumes, "ldap-ca") != nil {
			t.Error("no directory CA declared, no directory CA volume")
		}
	})

	t.Run("both CAs join the one truststore list", func(t *testing.T) {
		cfg := testIdentityConfig()
		cfg.CABundleSecretName, cfg.CABundleSecretKey, cfg.CABundleHash = testDirCASecret, testCAKey, testDirCAHash
		cfg.EmailCABundleSecretName, cfg.EmailCABundleSecretKey, cfg.EmailCABundleHash = testEmailCASecret, testCAKey, testEmailCAHash
		deploy := IdentityDeployment(cfg)
		got, _ := truststorePaths(deploy.Spec.Template.Spec.Containers[0].Env)
		want := IdentityCATruststorePath + "/" + IdentityCABundleFileName + "," + IdentityEmailCATruststorePath + "/" + IdentityEmailCABundleFileName
		if got != want {
			t.Errorf("KC_TRUSTSTORE_PATHS = %q, want %q", got, want)
		}
		mounts := deploy.Spec.Template.Spec.Containers[0].VolumeMounts
		var paths []string
		for _, m := range mounts {
			if m.Name == "ldap-ca" || m.Name == "email-ca" {
				paths = append(paths, m.MountPath)
				if !m.ReadOnly {
					t.Errorf("CA mount %s must be read-only", m.Name)
				}
			}
		}
		if len(paths) != 2 || paths[0] != IdentityCATruststorePath || paths[1] != IdentityEmailCATruststorePath {
			t.Errorf("CA mount paths = %v", paths)
		}
	})
}

// The realm import bakes the email OFF-state and never a declaration: the
// switch off and an empty relay map (never null, so it compares equal to what
// the reconciler owns). The reconciler converges the platform's declaration
// onto the live realm after first boot.
func TestIdentityRealmImport_EmailOffState(t *testing.T) {
	data, err := IdentityRealmImport(IdentityRealmImportConfig{
		Realm: testIdentityRealm, PublicURL: "http://planton.example.com",
		ClientSecret: "s", UsersSecret: "u",
	})
	if err != nil {
		t.Fatal(err)
	}
	var realm map[string]any
	if err := json.Unmarshal(data, &realm); err != nil {
		t.Fatal(err)
	}
	if realm["resetPasswordAllowed"] != false {
		t.Errorf("resetPasswordAllowed = %v, want false until a relay is declared", realm["resetPasswordAllowed"])
	}
	smtp, ok := realm["smtpServer"].(map[string]any)
	if !ok || len(smtp) != 0 {
		t.Errorf("smtpServer = %v, want an empty map", realm["smtpServer"])
	}
}
