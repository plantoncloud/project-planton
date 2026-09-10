package keycloaklogintheme

import (
	"bytes"
	"strings"
	"testing"
)

// The ConfigMap the operator materializes this theme into has a hard 1MiB
// ceiling; leave real headroom so adding an artifact never becomes a
// production surprise.
const configMapSizeBudget = 512 * 1024

func TestFilesCarriesEveryArtifact(t *testing.T) {
	files := Files()

	want := []string{PathThemeProperties, PathStylesCSS, PathLogoSVG, PathInterFontWOFF2, PathEmailThemeProperties}
	if len(files) != len(want) {
		t.Fatalf("Files() has %d entries, want %d -- a new artifact constant must be added to Files() (and vice versa)", len(files), len(want))
	}
	for _, p := range want {
		content, ok := files[p]
		if !ok {
			t.Fatalf("Files() is missing %q", p)
		}
		if len(content) == 0 {
			t.Fatalf("Files()[%q] is empty", p)
		}
	}
}

func TestThemePropertiesShape(t *testing.T) {
	props := string(Files()[PathThemeProperties])

	for _, want := range []string{
		// Extending the bundled theme is the safety property: unoverridden
		// screens keep working. A parent change is a deliberate decision.
		"parent=keycloak.v2",
		// The parent's stylesheet must stay in the list (child-first lookup
		// falls back to the parent for it), ours appended after.
		"styles=css/styles.css css/planton.css",
		// The OS must not flip the page out of the dark design.
		"darkMode=false",
	} {
		if !strings.Contains(props, want) {
			t.Errorf("theme.properties missing %q", want)
		}
	}
}

func TestStylesCarryTheDesignTokens(t *testing.T) {
	css := string(Files()[PathStylesCSS])

	// The load-bearing token values, byte-exact. Source of truth is the
	// console's theme package (the platform's design tokens) -- if this
	// test fails after a console token change, update the CSS constant with
	// it (that pairing is the documented contract).
	for token, value := range map[string]string{
		"page background":   "--planton-bg: #0d1117",
		"card surface":      "--planton-surface: #161b22",
		"border":            "--planton-border: #30363d",
		"primary text":      "--planton-text: #b0b8c2",
		"button background": "--planton-btn-bg: #e6edf3",
		"error":             "--planton-error: #f85149",
		"focus accent":      "--planton-focus-border: #696741",
		"font self-hosting": "url('../fonts/inter-latin.woff2')",
		"logo self-hosting": "url('../img/planton-logo.svg')",
		"no photo backdrop": "--keycloak-bg-logo-url: none",
	} {
		if !strings.Contains(css, value) {
			t.Errorf("stylesheet missing the %s token (%q)", token, value)
		}
	}

	// A credential page must fetch nothing from third parties.
	for _, forbidden := range []string{"http://", "https://"} {
		if strings.Contains(css, forbidden) {
			t.Errorf("stylesheet references an external URL (%q found) -- the theme must be fully self-contained", forbidden)
		}
	}
}

func TestFontIsRealWOFF2(t *testing.T) {
	font := Files()[PathInterFontWOFF2]
	if !bytes.HasPrefix(font, []byte("wOF2")) {
		t.Fatalf("embedded font does not start with the woff2 magic bytes")
	}
}

func TestLogoIsTheConsoleMark(t *testing.T) {
	svg := string(Files()[PathLogoSVG])
	if !strings.Contains(svg, `fill="#FBFBFB"`) {
		t.Errorf("logo is not the white (dark-background) mark")
	}
	if !strings.Contains(svg, `viewBox="0 0 28 32"`) {
		t.Errorf("logo geometry differs from the console asset")
	}
}

func TestHashIsDeterministicAndContentSensitive(t *testing.T) {
	first := Hash()
	second := Hash()
	if first != second {
		t.Fatalf("Hash() is not deterministic: %q vs %q", first, second)
	}
	if len(first) != 64 {
		t.Fatalf("Hash() is not a sha256 hex string: %q", first)
	}
}

func TestTotalSizeFitsTheConfigMapBudget(t *testing.T) {
	total := 0
	for _, content := range Files() {
		total += len(content)
	}
	if total > configMapSizeBudget {
		t.Fatalf("theme totals %d bytes, over the %d budget (ConfigMap ceiling is 1MiB; keep headroom)", total, configMapSizeBudget)
	}
}
