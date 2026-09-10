// Package platformversion decides whether this operator can run the platform
// version a PlantonPlatform declares.
//
// The operator and the platform release on independent lines. What binds
// them is the boot contract: the environment the operator renders for the
// control plane, the runner, and the console, and the set of images it
// expects to exist for one version. When that contract changes, platform
// images released before the change cannot boot under the new operator, and
// without this package the only symptom is a crash-looping control plane
// with nothing said at the resource. The floor makes the pairing explicit:
// the operator names the oldest platform release it runs, refuses anything
// older before creating a single object, and says why in the resource's
// status.
//
// A declared version is judged strictly. plantond's definitions floor waves
// an unstamped ("dev", "local") build of ITSELF through, because a developer
// build is newer than any release by definition. That reasoning does not
// transfer here: spec.version is typed by a person for the platform, not
// reported by the running binary about itself, and the operator cannot tell
// "local" from a typo. Custom builds run at a release version with the
// per-component image.tag override, which the API defines as a mirror of
// the same version.
package platformversion

import (
	"fmt"
	"regexp"

	"golang.org/x/mod/semver"
)

// MinimumSupported is the oldest platform release this operator runs: the
// oldest one whose control plane reads the settings this operator passes AND
// that was published whole (every platform image present for the tag).
//
// Raise it when the environment rendered for a platform component changes
// in a way an older platform image cannot tolerate -- a variable it requires,
// a variable it needs removed, a value whose meaning changed -- to the first
// release that reads the new shape. The boot-contract fixture test in
// internal/resources asks this question whenever the rendered variable names
// change; a variable older platforms simply ignore does not move the floor.
//
// v0.0.50: the control plane serves its browser-facing gRPC-Web API under the
// front-door path namespace (resources.APIPathPrefix) instead of at the
// origin root, and every front door this operator renders routes that
// namespace. An older control plane answers only at the root, so under this
// operator every console API call would 404.
//
// v0.0.56: the front door is the keyless identity issuer and declares its
// reachability, and the operator renders every internet-facing posture from
// it. Three reasons, any one of which moves the floor: the meaning of the
// keyless availability value changed (it used to be a literal "unavailable"
// with the catalog's generic sentence; it is now the door's verdict with the
// door's own reason, and an older catalog would show a reason it cannot
// explain); the control plane reads OIDC_ISSUER_URL and the per-cloud OAuth
// enabled flags only from this release, so an older one would keep minting
// the hosted issuer and offering sign-ins it has no app for; and the webhook
// receiver this operator advertises lives under the control plane's webhook
// namespace (resources.WebhooksPathPrefix), which an older control plane does
// not serve, so GitHub's deliveries would 404 at the door.
//
// v0.0.60: email is a declared capability, and invitations are links. The
// operator stops handing the control plane placeholder email credentials
// (RESEND_API_KEY and the two SENDGRID names) and the invitation URL base
// path, and renders PLANTON_EMAIL_PROVIDER=none instead; the control plane
// composes every invitation link from PLANTON_CONSOLE_URL. An older control
// plane requires the placeholder key and the base path to boot at all, so
// under this operator it would never come up.
const MinimumSupported = "v0.0.60"

// releaseForm is the only shape spec.version may take: a full semantic
// version with the "v" prefix, optionally with a pre-release suffix and build
// metadata. The CRD enforces the same expression at admission, so this check
// is reached only through a definition older than this operator. Kept as
// strict as the CRD on purpose: the semver library accepts shorthand such as
// "v1.2", and two gates that disagree about what a version is would be a
// defect of their own.
var releaseForm = regexp.MustCompile(`^v[0-9]+\.[0-9]+\.[0-9]+(-[0-9A-Za-z.-]+)?(\+[0-9A-Za-z.-]+)?$`)

// Verdict is the outcome of judging a declared platform version.
type Verdict struct {
	// Supported is true when this operator runs the declared version.
	Supported bool
	// Reason is the stable, CamelCase condition reason for the outcome.
	Reason string
	// Message explains the outcome in plain language: what was observed,
	// what it means, and the next step. Empty when Supported.
	Message string
}

// Condition reasons, stable so an agent can act on them.
const (
	// ReasonSupported: the declared version is at or above the floor.
	ReasonSupported = "Supported"
	// ReasonBelowOperatorMinimum: the declared version is a release older
	// than the oldest this operator runs.
	ReasonBelowOperatorMinimum = "BelowOperatorMinimum"
	// ReasonUnreadable: the declared version is not a release version, so it
	// cannot be placed on the release line at all.
	ReasonUnreadable = "Unreadable"
)

// Check judges a declared platform version against MinimumSupported.
// Comparison follows semantic-versioning precedence: a pre-release of the
// floor version (v0.0.42-rc.1 against v0.0.42) sorts below it and is refused,
// because a build that names itself a pre-release of a release was cut before
// that release; build metadata never affects the outcome.
func Check(version string) Verdict {
	if !releaseForm.MatchString(version) {
		return Verdict{
			Reason: ReasonUnreadable,
			Message: fmt.Sprintf(
				"spec.version %q is not a Planton release version, so this operator cannot tell which platform to run. "+
					"Set spec.version to a release as vMAJOR.MINOR.PATCH (%s or newer); "+
					"to run a custom build, keep spec.version at a release and set image.tag on the component",
				version, MinimumSupported),
		}
	}
	if semver.Compare(version, MinimumSupported) < 0 {
		return Verdict{
			Reason: ReasonBelowOperatorMinimum,
			Message: fmt.Sprintf(
				"spec.version %s is older than the oldest platform release this operator runs (%s); "+
					"the settings and images this operator expects belong to %s and newer. "+
					"Nothing running was changed. "+
					"Set spec.version to %s or newer, or install an operator release built for platform %s",
				version, MinimumSupported, MinimumSupported, MinimumSupported, version),
		}
	}
	return Verdict{Supported: true, Reason: ReasonSupported}
}
