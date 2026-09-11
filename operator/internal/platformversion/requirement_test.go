package platformversion

import (
	"strings"
	"testing"
)

func withOperatorRelease(t *testing.T, release string) {
	t.Helper()
	previous := OperatorRelease
	OperatorRelease = release
	t.Cleanup(func() { OperatorRelease = previous })
}

// A development build cannot place itself on the release line and judges
// nothing, whatever the platform asks for.
func TestCheckOperatorRequirement_DevBuildJudgesNothing(t *testing.T) {
	withOperatorRelease(t, DevRelease)
	if IsReleaseBuild() {
		t.Fatal("the development value is not a release")
	}
	if v := CheckOperatorRequirement("v0.0.70", "v9.9.9"); !v.Supported {
		t.Errorf("a dev build never refuses, got %+v", v)
	}
}

// The platform's requirement, in either spelling, against this release.
func TestCheckOperatorRequirement(t *testing.T) {
	withOperatorRelease(t, "v0.14.0")
	if !IsReleaseBuild() {
		t.Fatal("v0.14.0 is a release")
	}
	cases := []struct {
		required  string
		supported bool
	}{
		{"", true},              // the release predates the label
		{"v0.14.0", true},       // exactly this operator
		{"0.13.0", true},        // older, chart spelling
		{"v0.16.0", false},      // newer
		{"0.16.0", false},       // newer, chart spelling
		{"not-a-version", true}, // an unreadable label asks for nothing
	}
	for _, tc := range cases {
		v := CheckOperatorRequirement("v0.0.70", tc.required)
		if v.Supported != tc.supported {
			t.Errorf("required %q: supported=%v, want %v (%s)", tc.required, v.Supported, tc.supported, v.Message)
		}
	}
}

// The refusal names the platform version, the operator it needs, the one
// running, that nothing changed, and the exact upgrade command.
func TestCheckOperatorRequirement_RefusalSentence(t *testing.T) {
	withOperatorRelease(t, "v0.14.0")
	v := CheckOperatorRequirement("v0.0.70", "v0.16.0")
	if v.Reason != ReasonRequiresNewerOperator {
		t.Fatalf("reason %q", v.Reason)
	}
	for _, want := range []string{"spec.version v0.0.70 needs operator v0.16.0 or newer", "this operator is v0.14.0",
		"Nothing running was changed", "helm upgrade planton-operator", "--version 0.16.0", "then declare this version"} {
		if !strings.Contains(v.Message, want) {
			t.Errorf("message must contain %q, got: %s", want, v.Message)
		}
	}
}
