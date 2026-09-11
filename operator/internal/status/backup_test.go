package status

import (
	"testing"

	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	v1 "github.com/plantonhq/planton/operator/api/v1"
)

func TestSyncBackupCondition(t *testing.T) {
	cases := []struct {
		state  v1.BackupState
		status metav1.ConditionStatus
	}{
		{v1.BackupStateNotConfigured, metav1.ConditionUnknown},
		{v1.BackupStateDeploying, metav1.ConditionUnknown},
		{v1.BackupStateHealthy, metav1.ConditionTrue},
		{v1.BackupStateFailing, metav1.ConditionFalse},
		{v1.BackupStateUnavailable, metav1.ConditionFalse},
	}
	for _, tc := range cases {
		t.Run(string(tc.state), func(t *testing.T) {
			planton := &v1.PlantonPlatform{}
			planton.Generation = 3
			planton.Status.Backup = &v1.BackupStatus{State: tc.state, Message: "because"}
			if !SyncBackupCondition(planton) {
				t.Fatal("first sync must report a change")
			}
			cond := meta.FindStatusCondition(planton.Status.Conditions, v1.ConditionBackupHealthy)
			if cond == nil {
				t.Fatal("condition not written")
			}
			if cond.Status != tc.status || cond.Reason != string(tc.state) || cond.Message != "because" || cond.ObservedGeneration != 3 {
				t.Errorf("condition %+v", cond)
			}
			if SyncBackupCondition(planton) {
				t.Error("an unchanged status must not report a change")
			}
		})
	}
}

func TestSyncBackupCondition_NoStatusWritesNothing(t *testing.T) {
	planton := &v1.PlantonPlatform{}
	if SyncBackupCondition(planton) || len(planton.Status.Conditions) != 0 {
		t.Error("without status.backup there is nothing to say")
	}
}

// The Ready condition never reads the backup: a failing backup is a column
// and a condition of its own, not a red platform.
func TestReadyConditionIgnoresBackup(t *testing.T) {
	planton := &v1.PlantonPlatform{}
	planton.Status.Components.PostgreSQL = &v1.ComponentStatus{Phase: v1.ComponentPhaseReady}
	planton.Status.Backup = &v1.BackupStatus{State: v1.BackupStateFailing, Message: "AccessDenied"}
	if ComputeOverallPhase(planton) != v1.PhaseReady {
		t.Fatalf("test premise: a platform whose only component is Ready computes Ready, got %s", ComputeOverallPhase(planton))
	}
	UpdateReadyCondition(planton)
	SyncBackupCondition(planton)
	ready := meta.FindStatusCondition(planton.Status.Conditions, v1.ConditionReady)
	if ready == nil || ready.Status != metav1.ConditionTrue {
		t.Errorf("Ready must stay True while the backup fails: %+v", ready)
	}
	backup := meta.FindStatusCondition(planton.Status.Conditions, v1.ConditionBackupHealthy)
	if backup == nil || backup.Status != metav1.ConditionFalse {
		t.Errorf("BackupHealthy must be False: %+v", backup)
	}
}
