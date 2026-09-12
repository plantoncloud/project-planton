package component

import (
	"slices"
	"testing"

	v1 "github.com/plantonhq/planton/operator/api/v1"
)

// effectiveBootstrap is where the seed defaulting actually lives: kubebuilder
// defaults only apply when spec.bootstrap is present, and the admins default is
// cross-field ([spec.identity.adminEmail]) which CRD markers cannot express.
func TestEffectiveBootstrap_Defaults(t *testing.T) {
	p := ingressPlatform(true)

	binding := effectiveBootstrap(p)
	if binding.OrgSlug != "default" || binding.OrgName != "default" {
		t.Errorf("org = %s/%s, want default/default", binding.OrgSlug, binding.OrgName)
	}
	if binding.EnvSlug != "default" || binding.EnvName != "default" {
		t.Errorf("env = %s/%s, want default/default", binding.EnvSlug, binding.EnvName)
	}
	if len(binding.Admins) != 0 {
		t.Errorf("admins = %v, want none (no adminEmail declared)", binding.Admins)
	}
}

func TestEffectiveBootstrap_AdminEmailIsTheDefaultAdmin(t *testing.T) {
	p := ingressPlatform(true)
	p.Spec.Identity = &v1.IdentitySpec{AdminEmail: "boss@corp.example"}

	binding := effectiveBootstrap(p)
	if len(binding.Admins) != 1 || binding.Admins[0] != "boss@corp.example" {
		t.Errorf("admins = %v, want [boss@corp.example] (the declared identity admin)", binding.Admins)
	}
}

func TestEffectiveBootstrap_ExplicitSpecWins(t *testing.T) {
	p := ingressPlatform(true)
	p.Spec.Identity = &v1.IdentitySpec{AdminEmail: "boss@corp.example"}
	p.Spec.Bootstrap = &v1.BootstrapSpec{
		Organization: &v1.BootstrapOrganizationSpec{Slug: "acme", Name: "Acme Corp"},
		Environment:  &v1.BootstrapEnvironmentSpec{Slug: "prod"},
		Admins:       []string{"first@acme.example", "second@acme.example"},
	}

	binding := effectiveBootstrap(p)
	if binding.OrgSlug != "acme" || binding.OrgName != "Acme Corp" {
		t.Errorf("org = %s/%s, want acme/Acme Corp", binding.OrgSlug, binding.OrgName)
	}
	// Display name defaults to the slug when unset.
	if binding.EnvSlug != "prod" || binding.EnvName != "prod" {
		t.Errorf("env = %s/%s, want prod/prod", binding.EnvSlug, binding.EnvName)
	}
	// An explicit admins list REPLACES the adminEmail default, never merges:
	// the manifest is the whole truth.
	if len(binding.Admins) != 2 || binding.Admins[0] != "first@acme.example" {
		t.Errorf("admins = %v, want the explicit list", binding.Admins)
	}
}

// effectiveLicense resolves the delivery form; blank-tolerance matters
// because a declared-but-empty block must read as Community, never as an
// empty PLANTON_LICENSING_KEY.
func TestEffectiveLicense(t *testing.T) {
	p := ingressPlatform(true)

	if got := effectiveLicense(p); got != nil {
		t.Errorf("no spec.license must resolve nil (Community), got %+v", got)
	}

	p.Spec.License = &v1.LicenseSpec{}
	if got := effectiveLicense(p); got != nil {
		t.Errorf("an empty license block must resolve nil (Community), got %+v", got)
	}

	p.Spec.License = &v1.LicenseSpec{Key: "   "}
	if got := effectiveLicense(p); got != nil {
		t.Errorf("a blank key must resolve nil (Community), got %+v", got)
	}

	p.Spec.License = &v1.LicenseSpec{Key: " plk1.1.c.s "}
	got := effectiveLicense(p)
	if got == nil || got.Key != "plk1.1.c.s" || got.SecretName != "" {
		t.Errorf("inline key must resolve trimmed as the literal binding, got %+v", got)
	}

	p.Spec.License = &v1.LicenseSpec{
		SecretKeyRef: &v1.SecretKeyRef{Name: "acme-license", Key: "license-key"},
	}
	got = effectiveLicense(p)
	if got == nil || got.SecretName != "acme-license" || got.SecretKey != "license-key" || got.Key != "" {
		t.Errorf("secretKeyRef must resolve as the Secret-backed binding, got %+v", got)
	}
}

// The policy engine is wired on every platform: the control plane's FGA
// connection is always real.
func TestBuildConfig_OpenFGAAlwaysWired(t *testing.T) {
	cp := &ControlPlane{}

	cfg := cp.buildConfig(ingressPlatform(true), nil)
	if cfg.OpenFGA.HTTPURL == "" || cfg.OpenFGA.BootstrapConfigMapName == "" {
		t.Error("the OpenFGA connection must be wired on every platform")
	}
}

// The control plane's FGA store env comes from the openfga bootstrap
// ConfigMap, so openfga gates its startup on every platform -- an explained
// wait instead of CreateContainerConfigError.
func TestControlPlaneDependencies_OpenFGAAlwaysGates(t *testing.T) {
	cp := &ControlPlane{}

	deps := cp.Dependencies(ingressPlatform(true))
	if !slices.Contains(deps, "openfga") {
		t.Error("openfga must gate the control plane on every platform")
	}
}
