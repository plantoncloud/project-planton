package resources

import (
	"strings"
	"testing"

	keycloaklogintheme "github.com/plantonhq/planton/operator/internal/keycloaklogintheme"
)

func TestIdentityThemeConfigMap(t *testing.T) {
	cm, err := IdentityThemeConfigMap("planton", "default", nil)
	if err != nil {
		t.Fatal(err)
	}
	if cm.Name != "planton-identity-theme" {
		t.Errorf("name = %s, want planton-identity-theme", cm.Name)
	}

	// Every library file must land in the ConfigMap: text in Data (legible
	// in kubectl describe), fonts in BinaryData.
	files := keycloaklogintheme.Files()
	if len(cm.Data)+len(cm.BinaryData) != len(files) {
		t.Fatalf("ConfigMap carries %d entries, theme has %d files",
			len(cm.Data)+len(cm.BinaryData), len(files))
	}
	// Keys are the flattened theme paths, so both theme types can carry a
	// theme.properties of their own.
	for _, key := range []string{"login__theme.properties", "email__theme.properties", "login__resources__css__planton.css", "login__resources__img__planton-logo.svg"} {
		if _, ok := cm.Data[key]; !ok {
			t.Errorf("%s missing from Data", key)
		}
	}
	if _, ok := cm.BinaryData["login__resources__fonts__inter-latin.woff2"]; !ok {
		t.Error("the font must ride BinaryData (it is not valid UTF-8)")
	}
}

func TestIdentityDeployment_ThemeMountsAndRestartHash(t *testing.T) {
	cfg := testIdentityConfig()
	cfg.ThemeHash = "theme-hash-value"
	deploy := IdentityDeployment(cfg)

	// The theme volume rides beside the realm import.
	var themeVolumeFound bool
	for _, v := range deploy.Spec.Template.Spec.Volumes {
		if v.ConfigMap != nil && v.ConfigMap.Name == "planton-identity-theme" {
			themeVolumeFound = true
		}
	}
	if !themeVolumeFound {
		t.Fatal("theme ConfigMap volume missing from the Deployment")
	}

	// One subPath mount per theme file, laying the flat ConfigMap keys back
	// out as Keycloak's nested theme directory. Each type's theme.properties
	// path is the discovery contract: Keycloak only sees a theme type with
	// this file at exactly this location.
	mounts := deploy.Spec.Template.Spec.Containers[0].VolumeMounts
	wantMounts := map[string]string{
		"/opt/keycloak/themes/planton/login/theme.properties":                  "login__theme.properties",
		"/opt/keycloak/themes/planton/email/theme.properties":                  "email__theme.properties",
		"/opt/keycloak/themes/planton/login/resources/css/planton.css":         "login__resources__css__planton.css",
		"/opt/keycloak/themes/planton/login/resources/img/planton-logo.svg":    "login__resources__img__planton-logo.svg",
		"/opt/keycloak/themes/planton/login/resources/fonts/inter-latin.woff2": "login__resources__fonts__inter-latin.woff2",
	}
	found := 0
	for _, m := range mounts {
		if sub, ok := wantMounts[m.MountPath]; ok {
			found++
			if m.SubPath != sub {
				t.Errorf("mount %s has subPath %q, want %q", m.MountPath, m.SubPath, sub)
			}
			if !m.ReadOnly {
				t.Errorf("mount %s must be read-only", m.MountPath)
			}
		}
	}
	if found != len(wantMounts) {
		t.Errorf("found %d theme mounts, want %d", found, len(wantMounts))
	}

	// A theme change must roll the pod (Keycloak caches themes).
	if ann := deploy.Spec.Template.Annotations[IdentityThemeHashAnnotation]; ann != "theme-hash-value" {
		t.Errorf("theme hash annotation = %q, want theme-hash-value", ann)
	}
}

func TestIdentityRealmImport_LoginThemeAndDisplayName(t *testing.T) {
	realm := parseTestRealmImport(t)

	// Fresh realms pin the theme explicitly (the server-wide default flag
	// covers realms that predate it -- the import is create-only).
	if realm["loginTheme"] != keycloaklogintheme.ThemeName {
		t.Errorf("loginTheme = %v, want %s", realm["loginTheme"], keycloaklogintheme.ThemeName)
	}
	// The page <title> greets with the product name, never the realm slug.
	if realm["displayName"] != "Planton" {
		t.Errorf("displayName = %v, want Planton", realm["displayName"])
	}
	if realm["displayNameHtml"] != "Planton" {
		t.Errorf("displayNameHtml = %v, want Planton", realm["displayNameHtml"])
	}
}

func TestIdentityThemeHashIsStable(t *testing.T) {
	first := IdentityThemeHash()
	second := IdentityThemeHash()
	if first != second {
		t.Errorf("theme hash must be deterministic: %q vs %q", first, second)
	}
	if strings.TrimSpace(first) == "" {
		t.Error("theme hash must not be empty")
	}
}
