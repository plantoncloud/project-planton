package component

import (
	"strings"
	"testing"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	storagev1 "k8s.io/api/storage/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	v1 "github.com/plantonhq/planton/operator/api/v1"
)

// Every arm of the classifier is pinned here: the reason it yields, the object
// it names, and the words a person reads. A sentence that names only a
// mechanism ("CrashLoopBackOff") without the object and the next step is a
// defect these tests exist to catch.

var now = time.Date(2026, 9, 10, 12, 0, 0, 0, time.UTC)

func runningPod(name string, startedAgo time.Duration, restarts int32) corev1.Pod {
	return corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: "planton", CreationTimestamp: metav1.NewTime(now.Add(-startedAgo - time.Second))},
		Status: corev1.PodStatus{
			Phase:      corev1.PodRunning,
			Conditions: []corev1.PodCondition{{Type: corev1.PodReady, Status: corev1.ConditionFalse}},
			ContainerStatuses: []corev1.ContainerStatus{{
				Name: "main", Image: "ghcr.io/plantonhq/console:v0.0.60", RestartCount: restarts,
				State: corev1.ContainerState{Running: &corev1.ContainerStateRunning{StartedAt: metav1.NewTime(now.Add(-startedAgo))}},
			}},
		},
	}
}

func waitingPod(name, reason, message string) corev1.Pod {
	return corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: "planton", CreationTimestamp: metav1.NewTime(now)},
		Status: corev1.PodStatus{
			Phase: corev1.PodPending,
			ContainerStatuses: []corev1.ContainerStatus{{
				Name: "main", Image: "ghcr.io/plantonhq/console:v0.0.61",
				State: corev1.ContainerState{Waiting: &corev1.ContainerStateWaiting{Reason: reason, Message: message}},
			}},
		},
	}
}

func mustContain(t *testing.T, msg string, wants ...string) {
	t.Helper()
	for _, want := range wants {
		if !strings.Contains(msg, want) {
			t.Errorf("message must contain %q, got: %s", want, msg)
		}
	}
}

func TestClassifyUnreadyWorkload_ImagePull(t *testing.T) {
	pod := waitingPod("planton-console-7d9f-x1", "ImagePullBackOff", "Back-off pulling image \"ghcr.io/plantonhq/console:v0.0.61\": manifest unknown")
	expl := classifyUnreadyWorkload([]corev1.Pod{pod}, nil, nil, now)
	if expl == nil || expl.Reason != v1.ComponentReasonImagePullFailed {
		t.Fatalf("expected ImagePullFailed, got %+v", expl)
	}
	if expl.Object == nil || expl.Object.Kind != "Pod" || expl.Object.Name != pod.Name {
		t.Errorf("the pod is the object, got %+v", expl.Object)
	}
	mustContain(t, expl.Message, "ghcr.io/plantonhq/console:v0.0.61", pod.Name, "manifest unknown", "cannot be pulled", "image override")
}

func TestClassifyUnreadyWorkload_ContainerConfig(t *testing.T) {
	pod := waitingPod("planton-control-plane-abc", "CreateContainerConfigError", "secret \"planton-email-relay\" not found")
	expl := classifyUnreadyWorkload([]corev1.Pod{pod}, nil, nil, now)
	if expl == nil || expl.Reason != v1.ComponentReasonContainerConfigInvalid {
		t.Fatalf("expected ContainerConfigInvalid, got %+v", expl)
	}
	mustContain(t, expl.Message, "planton-email-relay", "cannot be created", "must exist in this namespace")
}

func TestClassifyUnreadyWorkload_OutOfMemory(t *testing.T) {
	pod := runningPod("planton-gateway-1", 2*time.Minute, 3)
	pod.Spec.Containers = []corev1.Container{{Name: "main", Resources: corev1.ResourceRequirements{
		Limits: corev1.ResourceList{corev1.ResourceMemory: resource.MustParse("32Mi")}}}}
	pod.Status.ContainerStatuses[0].LastTerminationState = corev1.ContainerState{
		Terminated: &corev1.ContainerStateTerminated{Reason: "OOMKilled", ExitCode: 137}}
	expl := classifyUnreadyWorkload([]corev1.Pod{pod}, nil, nil, now)
	if expl == nil || expl.Reason != v1.ComponentReasonOutOfMemory {
		t.Fatalf("expected OutOfMemory, got %+v", expl)
	}
	mustContain(t, expl.Message, "memory limit of 32Mi", "3 restarts", "raise the component's memory limit")
}

func TestClassifyUnreadyWorkload_OutOfMemoryWithoutLimitBlamesTheNode(t *testing.T) {
	pod := runningPod("planton-gateway-1", 2*time.Minute, 1)
	pod.Status.ContainerStatuses[0].LastTerminationState = corev1.ContainerState{
		Terminated: &corev1.ContainerStateTerminated{Reason: "OOMKilled", ExitCode: 137}}
	expl := classifyUnreadyWorkload([]corev1.Pod{pod}, nil, nil, now)
	mustContain(t, expl.Message, "no limit set: the node itself ran out of memory")
}

func TestClassifyUnreadyWorkload_CrashLooping(t *testing.T) {
	pod := waitingPod("planton-control-plane-abc", "CrashLoopBackOff", "back-off 5m0s restarting failed container")
	pod.Status.ContainerStatuses[0].RestartCount = 4
	pod.Status.ContainerStatuses[0].LastTerminationState = corev1.ContainerState{
		Terminated: &corev1.ContainerStateTerminated{Reason: "Error", ExitCode: 1}}
	expl := classifyUnreadyWorkload([]corev1.Pod{pod}, nil, nil, now)
	if expl == nil || expl.Reason != v1.ComponentReasonCrashLooping {
		t.Fatalf("expected CrashLooping, got %+v", expl)
	}
	mustContain(t, expl.Message, "keeps exiting", "4 restarts", "last exit code 1",
		"kubectl logs -n planton planton-control-plane-abc -c main --previous")
	if strings.Contains(expl.Message, "Error:") {
		t.Errorf("the bare kubelet reason \"Error\" adds nothing and must not be relayed: %s", expl.Message)
	}
}

// A container that exited once with a message worth relaying (a Java stack's
// last line, an exit reason) carries it; a restart while Running again is
// still a crash loop in the making.
func TestClassifyUnreadyWorkload_RestartedRunningContainerIsCrashLooping(t *testing.T) {
	pod := runningPod("planton-runner-1", 20*time.Second, 2)
	pod.Status.ContainerStatuses[0].LastTerminationState = corev1.ContainerState{
		Terminated: &corev1.ContainerStateTerminated{Reason: "Error", ExitCode: 2, Message: "panic: config missing"}}
	expl := classifyUnreadyWorkload([]corev1.Pod{pod}, nil, nil, now)
	if expl == nil || expl.Reason != v1.ComponentReasonCrashLooping {
		t.Fatalf("expected CrashLooping, got %+v", expl)
	}
	mustContain(t, expl.Message, "2 restarts", "last exit code 2, panic: config missing")
}

func TestClassifyUnreadyWorkload_Unschedulable(t *testing.T) {
	pod := corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: "planton-postgres-1", Namespace: "planton"},
		Status: corev1.PodStatus{Phase: corev1.PodPending, Conditions: []corev1.PodCondition{{
			Type: corev1.PodScheduled, Status: corev1.ConditionFalse, Reason: corev1.PodReasonUnschedulable,
			Message: "0/3 nodes are available: 3 Insufficient memory."}}},
	}
	expl := classifyUnreadyWorkload([]corev1.Pod{pod}, nil, nil, now)
	if expl == nil || expl.Reason != v1.ComponentReasonUnschedulable {
		t.Fatalf("expected Unschedulable, got %+v", expl)
	}
	mustContain(t, expl.Message, "no node can take pod planton-postgres-1", "3 Insufficient memory", "add capacity")
}

func TestClassifyUnreadyWorkload_VolumeMount(t *testing.T) {
	pod := corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: "planton-redis-0", Namespace: "planton"},
		Status:     corev1.PodStatus{Phase: corev1.PodPending},
	}
	events := []corev1.Event{{
		InvolvedObject: corev1.ObjectReference{Kind: "Pod", Name: "planton-redis-0"},
		Reason:         "FailedMount",
		Message:        "MountVolume.MountDevice failed for volume \"pvc-1\": rpc error: volume is attached to node-2",
		LastTimestamp:  metav1.NewTime(now),
	}}
	expl := classifyUnreadyWorkload([]corev1.Pod{pod}, nil, events, now)
	if expl == nil || expl.Reason != v1.ComponentReasonVolumeMountFailed {
		t.Fatalf("expected VolumeMountFailed, got %+v", expl)
	}
	mustContain(t, expl.Message, "cannot be attached or mounted", "attached to node-2", "storage driver's own pods")
}

func TestClassifyUnreadyWorkload_CreateRefused(t *testing.T) {
	conditions := []appsv1.DeploymentCondition{{
		Type: appsv1.DeploymentReplicaFailure, Status: corev1.ConditionTrue, Reason: "FailedCreate",
		Message: "pods \"planton-console-7d9f-\" is forbidden: exceeded quota: compute, requested: pods=1, used: pods=10, limited: pods=10"}}
	expl := classifyUnreadyWorkload(nil, conditions, nil, now)
	if expl == nil || expl.Reason != v1.ComponentReasonCreateRefused {
		t.Fatalf("expected CreateRefused, got %+v", expl)
	}
	if expl.Object != nil {
		t.Error("a refusal to create pods is about the component, not a pod")
	}
	mustContain(t, expl.Message, "refused to create", "exceeded quota", "raise the namespace quota")
}

func TestClassifyUnreadyWorkload_RolloutStalled(t *testing.T) {
	conditions := []appsv1.DeploymentCondition{{
		Type: appsv1.DeploymentProgressing, Status: corev1.ConditionFalse, Reason: "ProgressDeadlineExceeded",
		Message: "ReplicaSet \"planton-console-7d9f\" has timed out progressing."}}
	expl := classifyUnreadyWorkload(nil, conditions, nil, now)
	if expl == nil || expl.Reason != v1.ComponentReasonRolloutStalled {
		t.Fatalf("expected RolloutStalled, got %+v", expl)
	}
	mustContain(t, expl.Message, "gave up", "timed out progressing", "kubectl describe the Deployment")
}

// The calm arm: running, no restarts, health check not yet green. The
// sentence says how long and that this is normal -- and only after patience
// runs out does it send the person to the log.
func TestClassifyUnreadyWorkload_StartingUp(t *testing.T) {
	pod := runningPod("planton-control-plane-abc", 95*time.Second, 0)
	events := []corev1.Event{{
		InvolvedObject: corev1.ObjectReference{Kind: "Pod", Name: pod.Name},
		Reason:         "Unhealthy",
		Message:        "Readiness probe failed: Get \"http://10.0.0.5:8080/actuator/health\": dial tcp: connection refused",
		LastTimestamp:  metav1.NewTime(now),
	}}
	expl := classifyUnreadyWorkload([]corev1.Pod{pod}, nil, events, now)
	if expl == nil || expl.Reason != v1.ComponentReasonStartingUp {
		t.Fatalf("expected StartingUp, got %+v", expl)
	}
	if expl.Reason.IsFailure() {
		t.Error("StartingUp is never a failure")
	}
	mustContain(t, expl.Message, "running and not yet answering its health check", "Readiness probe failed", "1m35s since start", "normal in the first minutes")
	if strings.Contains(expl.Message, "kubectl logs") {
		t.Error("within patience, the sentence does not send the person to the log")
	}

	late := classifyUnreadyWorkload([]corev1.Pod{runningPod("planton-control-plane-abc", 12*time.Minute, 0)}, nil, nil, now)
	mustContain(t, late.Message, "longer than expected", "kubectl logs -n planton planton-control-plane-abc")
}

// Nothing to say: no pods yet (the Deployment was just applied), or a pod that
// is Pending with no verdict on it. The component's own sentence stands.
func TestClassifyUnreadyWorkload_NothingSpecific(t *testing.T) {
	if expl := classifyUnreadyWorkload(nil, nil, nil, now); expl != nil {
		t.Errorf("no pods, no conditions: nothing to explain, got %+v", expl)
	}
	pending := corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "p"}, Status: corev1.PodStatus{Phase: corev1.PodPending}}
	if expl := classifyUnreadyWorkload([]corev1.Pod{pending}, nil, nil, now); expl != nil {
		t.Errorf("a Pending pod with no verdict is not explained, got %+v", expl)
	}
}

// During a rollout the newest pod's trouble is the one that matters; the old
// pod, healthy and about to be replaced, must not mask it.
func TestClassifyUnreadyWorkload_NewestPodWins(t *testing.T) {
	old := runningPod("planton-console-old", 3*time.Hour, 0)
	old.Status.Conditions = []corev1.PodCondition{{Type: corev1.PodReady, Status: corev1.ConditionTrue}}
	fresh := waitingPod("planton-console-new", "ErrImagePull", "not found")
	expl := classifyUnreadyWorkload([]corev1.Pod{old, fresh}, nil, nil, now)
	if expl == nil || expl.Reason != v1.ComponentReasonImagePullFailed || expl.Object.Name != "planton-console-new" {
		t.Fatalf("the new pod's pull failure is the explanation, got %+v", expl)
	}
}

// A failure on any pod outranks a calm StartingUp on another.
func TestClassifyUnreadyWorkload_FailureOutranksStartingUp(t *testing.T) {
	starting := runningPod("planton-temporal-frontend-a", 30*time.Second, 0)
	crashing := waitingPod("planton-temporal-frontend-b", "CrashLoopBackOff", "")
	crashing.CreationTimestamp = metav1.NewTime(now.Add(-time.Hour)) // older, still wins
	crashing.Status.ContainerStatuses[0].RestartCount = 6
	crashing.Status.ContainerStatuses[0].LastTerminationState = corev1.ContainerState{Terminated: &corev1.ContainerStateTerminated{ExitCode: 1}}
	expl := classifyUnreadyWorkload([]corev1.Pod{starting, crashing}, nil, nil, now)
	if expl == nil || expl.Reason != v1.ComponentReasonCrashLooping {
		t.Fatalf("the crash outranks the boot, got %+v", expl)
	}
}

// Init containers are read first: a failing init container blocks everything
// after it, and its image is the one to name.
func TestClassifyUnreadyWorkload_InitContainerFirst(t *testing.T) {
	pod := corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: "planton-identity-x", Namespace: "planton"},
		Status: corev1.PodStatus{
			Phase: corev1.PodPending,
			InitContainerStatuses: []corev1.ContainerStatus{{Name: "theme", Image: "ghcr.io/plantonhq/theme:v1",
				State: corev1.ContainerState{Waiting: &corev1.ContainerStateWaiting{Reason: "ErrImagePull", Message: "denied"}}}},
			ContainerStatuses: []corev1.ContainerStatus{{Name: "keycloak", Image: "quay.io/keycloak/keycloak:26.3",
				State: corev1.ContainerState{Waiting: &corev1.ContainerStateWaiting{Reason: "PodInitializing"}}}},
		},
	}
	expl := classifyUnreadyWorkload([]corev1.Pod{pod}, nil, nil, now)
	if expl == nil || expl.Reason != v1.ComponentReasonImagePullFailed {
		t.Fatalf("expected ImagePullFailed on the init container, got %+v", expl)
	}
	mustContain(t, expl.Message, "ghcr.io/plantonhq/theme:v1", "container \"theme\"")
}

// --- storage through the pods ------------------------------------------

func podWithClaim(name, claim string) corev1.Pod {
	return corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: "planton"},
		Spec: corev1.PodSpec{Volumes: []corev1.Volume{{Name: "data", VolumeSource: corev1.VolumeSource{
			PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{ClaimName: claim}}}}},
		Status: corev1.PodStatus{Phase: corev1.PodPending},
	}
}

func storageFactsOf(classes []storagev1.StorageClass, drivers []storagev1.CSIDriver) func() storageFacts {
	return func() storageFacts { return storageFacts{classes: classes, drivers: drivers} }
}

// A claim the classifier can condemn is a failure that names the claim.
func TestExplainPendingClaims_Unprovisionable(t *testing.T) {
	pod := podWithClaim("planton-redis-0", "data-planton-redis-0")
	claim := pendingPVC("data-planton-redis-0", nil)
	expl := explainPendingClaims([]corev1.Pod{pod}, nil, []corev1.PersistentVolumeClaim{*claim},
		storageFactsOf([]storagev1.StorageClass{storageClass("trident", "csi.trident.netapp.io", false)}, nil), nil)
	if expl == nil || expl.Reason != v1.ComponentReasonVolumeUnprovisionable {
		t.Fatalf("expected VolumeUnprovisionable, got %+v", expl)
	}
	if expl.Object == nil || expl.Object.Kind != "PersistentVolumeClaim" || expl.Object.Name != "data-planton-redis-0" {
		t.Errorf("the claim is the object, got %+v", expl.Object)
	}
	mustContain(t, expl.Message, "no default StorageClass", "spec.storage.storageClassName")
}

// A claim that is merely still provisioning is the calm arm, naming the class
// doing the work.
func TestExplainPendingClaims_Provisioning(t *testing.T) {
	pod := podWithClaim("planton-redis-0", "data-planton-redis-0")
	className := "gp3"
	claim := pendingPVC("data-planton-redis-0", &className)
	expl := explainPendingClaims([]corev1.Pod{pod}, nil, []corev1.PersistentVolumeClaim{*claim},
		storageFactsOf([]storagev1.StorageClass{storageClass("gp3", "ebs.csi.aws.com", true)}, []storagev1.CSIDriver{csiDriver("ebs.csi.aws.com")}), nil)
	if expl == nil || expl.Reason != v1.ComponentReasonVolumeProvisioning {
		t.Fatalf("expected VolumeProvisioning, got %+v", expl)
	}
	if expl.Reason.IsFailure() {
		t.Error("a provisioning volume is not a failure")
	}
	mustContain(t, expl.Message, "waiting for the storage backend to provision it through StorageClass gp3", "usually under a minute")
}

// Only claims THIS workload's pods reference are considered: another
// component's stuck volume is its own component's news.
func TestExplainPendingClaims_OnlyReferencedClaims(t *testing.T) {
	pod := podWithClaim("planton-redis-0", "data-planton-redis-0")
	other := pendingPVC("data-planton-neo4j-0", nil)
	facts := storageFactsOf(nil, nil)
	if expl := explainPendingClaims([]corev1.Pod{pod}, nil, []corev1.PersistentVolumeClaim{*other}, facts, nil); expl != nil {
		t.Errorf("an unreferenced claim must not be explained here, got %+v", expl)
	}
	if expl := explainPendingClaims(nil, nil, []corev1.PersistentVolumeClaim{*other}, facts, nil); expl != nil {
		t.Errorf("no pods, no claims to judge, got %+v", expl)
	}
}

// Before the first pod exists, a claim carrying the workload's own selector
// labels is the workload's (a StatefulSet's volumeClaimTemplates, a
// CloudNativePG instance claim) -- never a claim that merely shares a name.
func TestExplainPendingClaims_LabelledClaimBeforeAnyPod(t *testing.T) {
	claim := pendingPVC("storage-explain-postgres-1", nil)
	claim.Labels = map[string]string{"cnpg.io/cluster": "storage-explain-postgres"}
	lookalike := pendingPVC("storage-explain-postgres-backup", nil)
	facts := storageFactsOf(nil, nil)
	expl := explainPendingClaims(nil, map[string]string{"cnpg.io/cluster": "storage-explain-postgres"},
		[]corev1.PersistentVolumeClaim{*lookalike, *claim}, facts, nil)
	if expl == nil || expl.Object == nil || expl.Object.Name != "storage-explain-postgres-1" {
		t.Fatalf("the labelled claim is the workload's, got %+v", expl)
	}
	if expl := explainPendingClaims(nil, map[string]string{"cnpg.io/cluster": "other"},
		[]corev1.PersistentVolumeClaim{*lookalike}, facts, nil); expl != nil {
		t.Errorf("a claim that only shares a name prefix is not the workload's, got %+v", expl)
	}
}

// --- jobs ---------------------------------------------------------------

func TestClassifyJobs(t *testing.T) {
	running := batchv1.Job{ObjectMeta: metav1.ObjectMeta{Name: "planton-temporal-schema-1", Namespace: "planton"},
		Status: batchv1.JobStatus{Active: 1}}
	expl := classifyJobs([]batchv1.Job{running}, "Temporal schema setup")
	if expl == nil || expl.Reason != v1.ComponentReasonWaitingForSchema {
		t.Fatalf("a running job is the wait, got %+v", expl)
	}
	mustContain(t, expl.Message, "Temporal schema setup job planton-temporal-schema-1", "restart until it does, which is expected")

	failed := batchv1.Job{ObjectMeta: metav1.ObjectMeta{Name: "planton-temporal-schema-1", Namespace: "planton"},
		Status: batchv1.JobStatus{Failed: 4, Conditions: []batchv1.JobCondition{{
			Type: batchv1.JobFailed, Status: corev1.ConditionTrue, Reason: "BackoffLimitExceeded", Message: "Job has reached the specified backoff limit"}}}}
	expl = classifyJobs([]batchv1.Job{running, failed}, "Temporal schema setup")
	if expl == nil || expl.Reason != v1.ComponentReasonJobFailed {
		t.Fatalf("a failed job is the cause and outranks a running one, got %+v", expl)
	}
	mustContain(t, expl.Message, "failed (BackoffLimitExceeded: Job has reached the specified backoff limit)",
		"kubectl logs -n planton job/planton-temporal-schema-1")

	done := batchv1.Job{ObjectMeta: metav1.ObjectMeta{Name: "s"}, Status: batchv1.JobStatus{Succeeded: 1, CompletionTime: &metav1.Time{Time: now}}}
	if expl := classifyJobs([]batchv1.Job{done}, "x"); expl != nil {
		t.Errorf("a finished job says nothing, got %+v", expl)
	}
	if expl := classifyJobs(nil, "x"); expl != nil {
		t.Errorf("no jobs, nothing to say, got %+v", expl)
	}
}

// Every reason the classifier can yield is a documented one.
func TestClassifier_YieldsOnlyDocumentedReasons(t *testing.T) {
	documented := map[v1.ComponentReason]bool{}
	for _, r := range v1.AllComponentReasons() {
		documented[r] = true
	}
	yielded := []*Explanation{
		classifyUnreadyWorkload([]corev1.Pod{waitingPod("a", "ImagePullBackOff", "")}, nil, nil, now),
		classifyUnreadyWorkload([]corev1.Pod{waitingPod("a", "CreateContainerConfigError", "")}, nil, nil, now),
		classifyUnreadyWorkload([]corev1.Pod{runningPod("a", time.Minute, 0)}, nil, nil, now),
		classifyJobs([]batchv1.Job{{Status: batchv1.JobStatus{Active: 1}}}, "x"),
	}
	for _, e := range yielded {
		if e == nil || !documented[e.Reason] {
			t.Errorf("classifier yielded an undocumented reason: %+v", e)
		}
	}
}
