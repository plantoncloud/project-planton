package component

import (
	"context"
	"strings"
	"testing"

	v1 "github.com/plantonhq/planton/operator/api/v1"
)

//go:fix inline
func boolPtr(b bool) *bool { return new(b) }

// THE load-bearing product claim: a CR with no build configuration at all
// runs builds. Builds power Service Hub -- an install without them is half a
// product -- so opting OUT is the explicit act, exactly like the runner.
func TestIsBuildEffective_DefaultOn(t *testing.T) {
	if !isBuildEffective(ingressPlatform(false)) {
		t.Fatal("a minimal CR must run builds -- default ON is the product contract")
	}
}

func TestIsBuildEffective_FollowsExplicitActs(t *testing.T) {
	optedOut := ingressPlatform(false)
	optedOut.Spec.Build = &v1.BuildSpec{Enabled: new(false)}
	if isBuildEffective(optedOut) {
		t.Error("an explicit spec.build opt-out must disable builds")
	}

	runnerOff := ingressPlatform(false)
	runnerOff.Spec.Runner = &v1.RunnerSpec{Enabled: new(false)}
	if isBuildEffective(runnerOff) {
		t.Error("builds must follow a disabled runner off -- the build worker is a capability OF the runner")
	}
}

// The component's enablement mirrors the status slot (see the status
// package's isTektonEnabled): on by default; quietly following the runner
// off when builds are left at default; staying enabled for an EXPLICIT
// build enable so the contradiction surfaces as an error, never a silent
// skip.
func TestTekton_IsEnabled(t *testing.T) {
	tekton := &Tekton{}

	if !tekton.IsEnabled(ingressPlatform(false)) {
		t.Error("tekton must be enabled on a minimal CR (builds default on)")
	}

	optedOut := ingressPlatform(false)
	optedOut.Spec.Build = &v1.BuildSpec{Enabled: new(false)}
	if tekton.IsEnabled(optedOut) {
		t.Error("tekton must be disabled on build opt-out")
	}

	runnerOffDefaultBuild := ingressPlatform(false)
	runnerOffDefaultBuild.Spec.Runner = &v1.RunnerSpec{Enabled: new(false)}
	if tekton.IsEnabled(runnerOffDefaultBuild) {
		t.Error("with builds at default, tekton must quietly follow a disabled runner off")
	}

	runnerOffExplicitBuild := ingressPlatform(false)
	runnerOffExplicitBuild.Spec.Runner = &v1.RunnerSpec{Enabled: new(false)}
	runnerOffExplicitBuild.Spec.Build = &v1.BuildSpec{Enabled: new(true)}
	if !tekton.IsEnabled(runnerOffExplicitBuild) {
		t.Error("an explicit build enable must keep tekton enabled so the runner contradiction can error")
	}
}

// An explicit spec.build.enabled=true with the runner disabled is a stated
// intent the install cannot honor: the component errors with both ways out
// named. The check precedes every cluster call, so no client is needed.
func TestTekton_ReconcileRejectsExplicitBuildWithoutRunner(t *testing.T) {
	p := ingressPlatform(false)
	p.Spec.Runner = &v1.RunnerSpec{Enabled: new(false)}
	p.Spec.Build = &v1.BuildSpec{Enabled: new(true)}

	tekton := &Tekton{}
	_, err := tekton.Reconcile(context.Background(), nil, nil, p)
	if err == nil {
		t.Fatal("expected the runner contradiction to be a component error")
	}
	if !strings.Contains(err.Error(), "spec.runner") || !strings.Contains(err.Error(), "spec.build") {
		t.Errorf("the error must name both fields so the fix is obvious, got: %v", err)
	}
}

// The runner Deployment's build flip follows the spec through runnerConfig:
// the default CR renders a build-capable runner, the opt-out does not.
func TestRunnerConfig_BuildEnabledFollowsSpec(t *testing.T) {
	if cfg := runnerConfig(ingressPlatform(false), nil); !cfg.BuildEnabled {
		t.Error("the default CR must render a build-capable runner")
	}

	optedOut := ingressPlatform(false)
	optedOut.Spec.Build = &v1.BuildSpec{Enabled: new(false)}
	if cfg := runnerConfig(optedOut, nil); cfg.BuildEnabled {
		t.Error("build opt-out must render the runner without the build capability")
	}
}

// The control plane's build-routing seed follows the same predicate: seeded
// by default, absent on opt-out -- one effective-build answer for the whole
// capability.
func TestBuildConfig_BuildRoutingSeedFollowsSpec(t *testing.T) {
	cp := &ControlPlane{}

	cfg := cp.buildConfig(ingressPlatform(true), nil)
	if cfg.Runner == nil || !cfg.Runner.BuildEnabled {
		t.Error("the default CR must seed build routing (runner binding with builds enabled)")
	}

	optedOut := ingressPlatform(true)
	optedOut.Spec.Build = &v1.BuildSpec{Enabled: new(false)}
	cfg = cp.buildConfig(optedOut, nil)
	if cfg.Runner == nil {
		t.Fatal("the runner binding must survive a build opt-out -- IaC deploys are unaffected")
	}
	if cfg.Runner.BuildEnabled {
		t.Error("build opt-out must not seed build routing")
	}
}
