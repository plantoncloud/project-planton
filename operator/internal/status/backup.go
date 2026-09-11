package status

import (
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	v1 "github.com/plantonhq/planton/operator/api/v1"
)

// SyncBackupCondition derives the BackupHealthy condition from status.backup,
// which the PostgreSQL component writes from the database operator's own
// signals. The condition is deliberately NOT an input to Ready: a platform
// whose backup fails is doing its job and its safety net is not, and the
// Backup print column plus this condition carry that where a person reads
// first. Returns true when the condition changed.
func SyncBackupCondition(planton *v1.PlantonPlatform) bool {
	backup := planton.Status.Backup
	if backup == nil {
		return false
	}
	var before *metav1.Condition
	if existing := meta.FindStatusCondition(planton.Status.Conditions, v1.ConditionBackupHealthy); existing != nil {
		copied := *existing
		before = &copied
	}

	condStatus := metav1.ConditionUnknown
	switch backup.State {
	case v1.BackupStateHealthy:
		condStatus = metav1.ConditionTrue
	case v1.BackupStateFailing, v1.BackupStateUnavailable:
		condStatus = metav1.ConditionFalse
	case v1.BackupStateNotConfigured, v1.BackupStateDeploying:
		condStatus = metav1.ConditionUnknown
	}
	message := backup.Message
	if message == "" {
		message = string(backup.State)
	}
	SetCondition(planton, v1.ConditionBackupHealthy, condStatus, string(backup.State), message)

	after := meta.FindStatusCondition(planton.Status.Conditions, v1.ConditionBackupHealthy)
	return before == nil || before.Status != after.Status || before.Reason != after.Reason || before.Message != after.Message
}
