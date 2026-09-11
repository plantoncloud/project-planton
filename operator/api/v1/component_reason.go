package v1

// IsFailure reports whether a reason means something is WRONG with the
// component, as opposed to something still happening (a boot, a volume being
// provisioned, a dependency not yet Ready). The distinction decides what
// becomes a Warning Event on the platform and which component the Ready
// condition speaks for first: a normal boot must leave `kubectl describe`
// quiet, and a crash loop must never be outranked by a component that is
// merely waiting on it.
//
// Kept beside the constants so a new reason is classified where it is
// declared; the troubleshooting reference groups its rows by this answer.
func (r ComponentReason) IsFailure() bool {
	switch r {
	case ComponentReasonVolumeUnprovisionable,
		ComponentReasonImagePullFailed,
		ComponentReasonContainerConfigInvalid,
		ComponentReasonOutOfMemory,
		ComponentReasonCrashLooping,
		ComponentReasonVolumeMountFailed,
		ComponentReasonUnschedulable,
		ComponentReasonRolloutStalled,
		ComponentReasonCreateRefused,
		ComponentReasonJobFailed,
		ComponentReasonConfigurationRefused,
		ComponentReasonReconcileFailed:
		return true
	}
	return false
}

// AllComponentReasons lists every reason the operator can write, in the
// order the troubleshooting reference presents them: the healthy and
// in-progress reasons first, then the failures. Tests pin that every entry
// has a sentence and a reference row.
func AllComponentReasons() []ComponentReason {
	return []ComponentReason{
		ComponentReasonHealthy,
		ComponentReasonWaitingForDependency,
		ComponentReasonDeploying,
		ComponentReasonStartingUp,
		ComponentReasonWaitingForSchema,
		ComponentReasonVolumeProvisioning,
		ComponentReasonVolumeUnprovisionable,
		ComponentReasonImagePullFailed,
		ComponentReasonContainerConfigInvalid,
		ComponentReasonOutOfMemory,
		ComponentReasonCrashLooping,
		ComponentReasonVolumeMountFailed,
		ComponentReasonUnschedulable,
		ComponentReasonRolloutStalled,
		ComponentReasonCreateRefused,
		ComponentReasonJobFailed,
		ComponentReasonConfigurationRefused,
		ComponentReasonReconcileFailed,
	}
}
