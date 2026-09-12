// Package keycloaklogintheme is the Planton design system translated to the
// self-hosted identity server: the sign-in pages every teammate on an
// adopting team sees daily, and the emails the identity server sends them
// (a password reset, a verification link, an administrator's required
// action).
//
// A Keycloak theme is static files, not code -- a manifest
// (theme.properties) per theme type declaring a parent theme, plus
// stylesheets, assets, and FreeMarker templates. This package carries those
// files as a real directory embedded at build time (theme/), so the package
// listing IS the manifest of what ships, each file is edited in its own
// language, and the generator that writes the email layout writes a file,
// not a Go constant. The operator materializes the directory into a
// ConfigMap mounted at /opt/keycloak/themes/planton/, so the OFFICIAL
// pinned Keycloak image runs unmodified -- no fork image on a
// security-critical surface, and a theme change is an operator release,
// never an identity-server rebuild. The realm import selects the theme by
// name (loginTheme); the server-wide default-theme flag selects it for
// every other type, email included.
//
// One file is rendered per install rather than shipped verbatim: the email
// type's manifest carries the install's own facts (its sending name, its
// console address, where a reply lands) as theme properties, which is how
// the identity server's emails name the install instead of Planton. See
// EmailFacts.
//
// See README.md for the full rationale: the theming model, the delivery
// choice, the generated email layout, and the token-translation contract
// with the console theme.
package keycloaklogintheme

import (
	"crypto/sha256"
	"embed"
	"encoding/hex"
	"fmt"
	"io/fs"
	"net/url"
	"sort"
	"strings"
)

// ThemeName is the directory name under /opt/keycloak/themes and the value
// the realm's loginTheme field selects.
const ThemeName = "planton"

// File paths inside the theme, relative to the theme's root directory
// (themes/planton/), for the files other packages or tests name directly.
// The nesting follows Keycloak's required layout:
// <type>/theme.properties + <type>/resources/** + <type>/{html,text}/*.ftl.
const (
	PathThemeProperties      = "login/theme.properties"
	PathStylesCSS            = "login/resources/css/planton.css"
	PathLogoSVG              = "login/resources/img/planton-logo.svg"
	PathInterFontWOFF2       = "login/resources/fonts/inter-latin.woff2"
	PathEmailThemeProperties = "email/theme.properties"
	PathEmailHTMLLayout      = "email/html/template.ftl"
	PathEmailTextLayout      = "email/text/template.ftl"
	PathEmailMessages        = "email/messages/messages_en.properties"
)

// The theme directory, embedded whole. Every file under it ships; there is
// no allowlist to keep in sync.
//
//go:embed all:theme
var themeFS embed.FS

// EmailFacts is what the identity server's emails say about the install
// that sent them. The operator fills it from the platform resource on every
// pass; the email templates read the values as FreeMarker theme properties
// (properties.brandName, properties.consoleUrl, properties.consoleHost,
// properties.replyTo). Zero values are honest: an install with no console
// URL names only itself, one with no reply-to offers no "write to" line.
type EmailFacts struct {
	// BrandName is the install's sending name (spec.email.from.name), the
	// product name when nothing is declared.
	BrandName string
	// ConsoleURL is the console's browser origin; empty when the install
	// serves no console.
	ConsoleURL string
	// ReplyTo is the declared reply-to address; empty when replies land on
	// the sending address.
	ReplyTo string
}

// ConsoleHost is the console URL's host (with a non-default port), the
// short way an email says which install sent it; empty with the URL.
func (f EmailFacts) ConsoleHost() string {
	if f.ConsoleURL == "" {
		return ""
	}
	u, err := url.Parse(f.ConsoleURL)
	if err != nil || u.Host == "" {
		return f.ConsoleURL
	}
	return u.Host
}

// Files returns every file the theme ships, keyed by its path under the
// theme root, with the email manifest rendered for the install described by
// facts. The map is rebuilt per call; callers own their copy.
func Files(facts EmailFacts) map[string][]byte {
	files := map[string][]byte{}
	err := fs.WalkDir(themeFS, "theme", func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		content, err := themeFS.ReadFile(p)
		if err != nil {
			return err
		}
		files[strings.TrimPrefix(p, "theme/")] = content
		return nil
	})
	if err != nil {
		// The directory is compiled into the binary; a walk that fails is a
		// build defect, never a runtime condition to handle.
		panic(fmt.Sprintf("walking the embedded identity theme: %v", err))
	}
	files[PathEmailThemeProperties] = emailThemeProperties(files[PathEmailThemeProperties], facts)
	return files
}

// emailThemeProperties appends the install's facts to the email type's
// shipped manifest as theme properties. Keycloak exposes a theme type's
// properties to its FreeMarker templates as `properties`, so this is the
// one channel through which a template learns anything about the install
// it runs on; the values are written verbatim (Java properties syntax
// treats the rest of the line after `=` as the value).
func emailThemeProperties(shipped []byte, facts EmailFacts) []byte {
	var b strings.Builder
	b.Write(shipped)
	if !strings.HasSuffix(b.String(), "\n") {
		b.WriteString("\n")
	}
	b.WriteString("\n# The install these emails come from, written by the operator on every pass.\n")
	fmt.Fprintf(&b, "brandName=%s\n", propertyValue(facts.BrandName))
	fmt.Fprintf(&b, "consoleUrl=%s\n", propertyValue(facts.ConsoleURL))
	fmt.Fprintf(&b, "consoleHost=%s\n", propertyValue(facts.ConsoleHost()))
	fmt.Fprintf(&b, "replyTo=%s\n", propertyValue(facts.ReplyTo))
	return []byte(b.String())
}

// propertyValue keeps a value on one line: a newline in a declared name
// would otherwise start a new property. Java properties need no other
// escaping for a value.
func propertyValue(v string) string {
	return strings.NewReplacer("\n", " ", "\r", " ").Replace(strings.TrimSpace(v))
}

// Hash fingerprints the complete theme content (paths and bytes, in
// deterministic order) for the install described by facts. The operator
// stamps it onto the identity pod so a theme change -- or a change to what
// the emails say about the install -- rolls the pod: Keycloak caches themes
// aggressively, and a restart is what makes a new version take effect.
func Hash(facts EmailFacts) string {
	files := Files(facts)
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
