package resources

import (
	"fmt"
	"path"
	"sort"
	"strings"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	keycloaklogintheme "github.com/plantonhq/planton/operator/internal/keycloaklogintheme"
)

// The identity server's sign-in pages ship in the Planton design system via
// the keycloak-login-theme library. The operator materializes the library's
// files into a ConfigMap mounted under Keycloak's themes directory, so the
// OFFICIAL pinned Keycloak image runs unmodified -- a theme change is an
// operator release, never an identity-server image rebuild.
const (
	// identityThemeMountRoot is where Keycloak discovers custom themes.
	identityThemeMountRoot = "/opt/keycloak/themes"

	// IdentityThemeHashAnnotation rolls the Keycloak pod when the theme
	// content changes (same pattern as the realm-import hash): Keycloak
	// caches themes, so a restart is what makes a new version take effect.
	IdentityThemeHashAnnotation = "planton.ai/identity-theme-hash"
)

// IdentityThemeConfigMapName returns "{crName}-identity-theme".
func IdentityThemeConfigMapName(crName string) string {
	return fmt.Sprintf("%s-identity-theme", crName)
}

// IdentityThemeHash fingerprints the theme content for the pod-restart
// annotation.
func IdentityThemeHash(facts keycloaklogintheme.EmailFacts) string {
	return keycloaklogintheme.Hash(facts)
}

// identityThemeFilePaths returns the theme's file paths (relative to the
// theme root) in deterministic order -- the shared iteration order for the
// ConfigMap keys and the volume mounts, which must agree.
func identityThemeFilePaths() []string {
	// The set of paths does not depend on the install's facts (they render
	// INTO a file that always ships), so the zero facts list them.
	files := keycloaklogintheme.Files(keycloaklogintheme.EmailFacts{})
	paths := make([]string, 0, len(files))
	for p := range files {
		paths = append(paths, p)
	}
	sort.Strings(paths)
	return paths
}

// identityThemeConfigMapKey flattens a theme path into a ConfigMap key:
// keys cannot contain path separators, so "login/theme.properties" becomes
// "login__theme.properties". Keying by the WHOLE path is what lets two theme
// types (login, email) each carry their own theme.properties -- a base-name
// key would collide -- and the Deployment mounts each key back at its full
// theme path via subPath.
func identityThemeConfigMapKey(themePath string) string {
	return strings.ReplaceAll(themePath, "/", "__")
}

// IdentityThemeConfigMap builds the ConfigMap carrying every theme file,
// keyed by its flattened path (identityThemeConfigMapKey), with the email
// manifest rendered for this install (facts). Text files go in Data
// (legible in kubectl describe); the font goes in BinaryData.
func IdentityThemeConfigMap(crName, namespace string, facts keycloaklogintheme.EmailFacts, ownerRef *metav1.OwnerReference) (*corev1.ConfigMap, error) {
	files := keycloaklogintheme.Files(facts)

	cm := &corev1.ConfigMap{
		TypeMeta: metav1.TypeMeta{APIVersion: "v1", Kind: "ConfigMap"},
		ObjectMeta: metav1.ObjectMeta{
			Name:      IdentityThemeConfigMapName(crName),
			Namespace: namespace,
			Labels: map[string]string{
				"app.kubernetes.io/name":       "identity",
				"app.kubernetes.io/instance":   crName,
				"app.kubernetes.io/managed-by": ManagedByLabel,
				"app.kubernetes.io/component":  "application",
			},
		},
		Data:       map[string]string{},
		BinaryData: map[string][]byte{},
	}

	for _, p := range identityThemeFilePaths() {
		key := identityThemeConfigMapKey(p)
		if strings.HasSuffix(p, ".woff2") {
			cm.BinaryData[key] = files[p]
		} else {
			cm.Data[key] = string(files[p])
		}
	}

	if ownerRef != nil {
		cm.OwnerReferences = []metav1.OwnerReference{*ownerRef}
	}
	return cm, nil
}

// identityThemeVolume returns the theme volume plus one subPath mount per
// theme file, laying the flat ConfigMap keys back out as Keycloak's nested
// theme directory. subPath mounts do not see live ConfigMap updates -- by
// design a non-issue here, because any theme change arrives with a new
// operator whose theme hash rolls the pod anyway.
func identityThemeVolume(crName string) (corev1.Volume, []corev1.VolumeMount) {
	volume := corev1.Volume{
		Name: "login-theme",
		VolumeSource: corev1.VolumeSource{
			ConfigMap: &corev1.ConfigMapVolumeSource{
				LocalObjectReference: corev1.LocalObjectReference{
					Name: IdentityThemeConfigMapName(crName),
				},
			},
		},
	}

	mounts := make([]corev1.VolumeMount, 0, len(identityThemeFilePaths()))
	for _, p := range identityThemeFilePaths() {
		mounts = append(mounts, corev1.VolumeMount{
			Name:      volume.Name,
			MountPath: path.Join(identityThemeMountRoot, keycloaklogintheme.ThemeName, p),
			SubPath:   identityThemeConfigMapKey(p),
			ReadOnly:  true,
		})
	}
	return volume, mounts
}
