// Package keycloaklogintheme is the Planton design system translated to the
// self-hosted identity server's pages: the sign-in form, forced password
// update, profile completion, error, and logout screens that every teammate
// on an adopting team sees daily.
//
// A Keycloak theme is static files, not code -- a manifest
// (theme.properties) declaring a parent theme plus stylesheets and assets.
// This package carries those files as named Go constants (one file per
// artifact; the package listing IS the manifest of what ships) and exposes
// them as a path-keyed map. The operator materializes the map into a
// ConfigMap mounted at /opt/keycloak/themes/planton/, so the OFFICIAL
// pinned Keycloak image runs unmodified -- no fork image on a
// security-critical surface, and a theme change is an operator release,
// never an identity-server rebuild. The realm import selects the theme by
// name (loginTheme).
//
// See README.md for the full rationale: the theming model, the delivery
// choice, and the token-translation contract with the console theme.
package keycloaklogintheme

import (
	"crypto/sha256"
	"encoding/hex"
	"sort"
)

// ThemeName is the directory name under /opt/keycloak/themes and the value
// the realm's loginTheme field selects.
const ThemeName = "planton"

// File paths inside the theme, relative to the theme's root directory
// (themes/planton/). The nesting follows Keycloak's required layout:
// <type>/theme.properties + <type>/resources/**. Two types ship: login (the
// styled pages) and email (the manifest that keeps the server-wide default
// theme resolvable for the emails the identity server sends; see
// email_theme_properties.go).
const (
	PathThemeProperties      = "login/theme.properties"
	PathStylesCSS            = "login/resources/css/planton.css"
	PathLogoSVG              = "login/resources/img/planton-logo.svg"
	PathInterFontWOFF2       = "login/resources/fonts/inter-latin.woff2"
	PathEmailThemeProperties = "email/theme.properties"
)

// Files returns every file the theme ships, keyed by its path under the
// theme root. The map is rebuilt per call; callers own their copy.
func Files() map[string][]byte {
	return map[string][]byte{
		PathThemeProperties:      []byte(themeProperties),
		PathStylesCSS:            []byte(stylesCSS),
		PathLogoSVG:              []byte(logoSVG),
		PathInterFontWOFF2:       interLatinWOFF2,
		PathEmailThemeProperties: []byte(emailThemeProperties),
	}
}

// Hash fingerprints the complete theme content (paths and bytes, in
// deterministic order). The operator stamps it onto the identity pod so a
// theme change rolls the pod -- Keycloak caches themes aggressively, and a
// restart is what makes a new theme version take effect.
func Hash() string {
	files := Files()
	paths := make([]string, 0, len(files))
	for p := range files {
		paths = append(paths, p)
	}
	sort.Strings(paths)

	h := sha256.New()
	for _, p := range paths {
		h.Write([]byte(p))
		h.Write([]byte{0})
		h.Write(files[p])
		h.Write([]byte{0})
	}
	return hex.EncodeToString(h.Sum(nil))
}
