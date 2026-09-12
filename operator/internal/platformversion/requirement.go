package platformversion

import (
	"context"
	"fmt"
	"strings"

	"golang.org/x/mod/semver"

	"github.com/plantonhq/planton/operator/internal/ociregistry"
)

// The contract between the operator and the platform is guarded in both
// directions. The floor (MinimumSupported) is the operator's side: the oldest
// platform release it renders for. The other side is the PLATFORM's to state:
// each platform release declares the oldest operator that serves it, as a
// label on its control-plane image, moved only when the platform starts
// depending on something a newer operator renders. The operator never carries
// a ceiling of its own -- "the newest platform I was built for" would refuse
// every platform release that changed nothing until an operator was
// re-released, an operator release per platform release, forever.
//
// Reading the requirement needs the registry, which an air-gapped install may
// not reach. The read is therefore best-effort: when it fails, the operator
// proceeds exactly as it did before this guard existed and says in its log
// that it could not check. The installer (desktop, CLI) enforces the same
// label before declaring, where the registry is always reachable.

// OperatorRelease is this operator's own release (vX.Y.Z), stamped at build
// time by the release workflow (-ldflags -X). A development build carries
// DevRelease and never refuses on the requirement: it cannot place itself on
// the release line.
var OperatorRelease = DevRelease

// DevRelease is the version a build carries when nothing stamped it.
const DevRelease = "0.0.0-dev"

// MinimumOperatorLabel is the OCI label a platform release stamps on its
// control-plane image: the oldest operator release that serves it, as
// vX.Y.Z (the operator's release form; the chart version is the same
// number without the v).
const MinimumOperatorLabel = "planton.ai/minimum-operator-version"

// ReasonRequiresNewerOperator: the declared platform release needs an
// operator newer than this one.
const ReasonRequiresNewerOperator = "RequiresNewerOperator"

// IsReleaseBuild reports whether this operator knows its own release, and so
// can judge a platform's requirement.
func IsReleaseBuild() bool {
	return semver.IsValid(normalize(OperatorRelease)) && OperatorRelease != DevRelease
}

// CheckOperatorRequirement judges this operator against the oldest operator
// a platform release declares it needs. An empty requirement (the platform
// release predates the label, or the read found none) is supported: the
// platform asked for nothing.
func CheckOperatorRequirement(platformVersion, requiredOperator string) Verdict {
	required := normalize(requiredOperator)
	if required == "" || !semver.IsValid(required) || !IsReleaseBuild() {
		return Verdict{Supported: true, Reason: ReasonSupported}
	}
	if semver.Compare(normalize(OperatorRelease), required) < 0 {
		chart := strings.TrimPrefix(required, "v")
		return Verdict{
			Reason: ReasonRequiresNewerOperator,
			Message: fmt.Sprintf(
				"spec.version %s needs operator %s or newer, and this operator is %s; "+
					"the platform depends on something this operator does not render yet. "+
					"Nothing running was changed. "+
					"Upgrade the operator first (helm upgrade planton-operator oci://ghcr.io/plantonhq/charts/planton-operator --version %s -n planton-operator), then declare this version",
				platformVersion, required, normalize(OperatorRelease), chart),
		}
	}
	return Verdict{Supported: true, Reason: ReasonSupported}
}

// normalize gives every release form the v prefix semver expects, so a label
// written as 0.16.0 and one written as v0.16.0 compare the same.
func normalize(version string) string {
	version = strings.TrimSpace(version)
	if version == "" {
		return ""
	}
	if !strings.HasPrefix(version, "v") {
		version = "v" + version
	}
	return version
}

// RequirementReader reads the operator requirement a platform release
// declares. The controller holds one and caches its answers per version.
type RequirementReader interface {
	RequiredOperator(ctx context.Context, platformVersion string) (string, error)
}

// RegistryRequirementReader reads the label off the control-plane image of a
// release in the registry.
type RegistryRequirementReader struct {
	// ImageRepository is the control-plane image without a tag
	// (ghcr.io/plantonhq/planton/control-plane).
	ImageRepository string
	Client          *ociregistry.Client
}

// RequiredOperator returns the label's value, or "" when the release carries
// none (a platform older than the label).
func (r *RegistryRequirementReader) RequiredOperator(ctx context.Context, platformVersion string) (string, error) {
	labels, err := r.Client.ImageLabels(ctx, r.ImageRepository+":"+platformVersion)
	if err != nil {
		return "", err
	}
	return labels[MinimumOperatorLabel], nil
}
