package component

import (
	"context"
	"fmt"
	"slices"
	"sort"
	"strings"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	v1 "github.com/plantonhq/planton/operator/api/v1"
)

// A component that is not Ready owes the person one sentence that names what
// is wrong and what to do -- never "Waiting for X" for an hour while a pod
// sits in ImagePullBackOff. This file is where every component's not-ready
// branch gets that sentence. Base.NotReady reads the workload's own pods,
// the volume claims those pods reference, the workload's conditions, and the
// Events the kubelet and scheduler wrote about the pods, and hands them to a
// pure classifier so every arm's wording is pinned offline.
//
// The order of the arms is the order of certainty. Storage first: a claim
// that will never provision pins its pod in Pending, and the pod's own
// "unschedulable" would name the symptom instead of the cause. Then the
// container-level facts the kubelet states outright (a pull failure, a
// missing Secret key, an out-of-memory kill, a crash loop). Then the pod- and
// workload-level verdicts (unschedulable, a mount that will not attach, a
// rollout the controller gave up on, a creation the API server refused).
// Last, the calm arm: a pod that runs and has simply not answered its health
// check yet, which on a first boot is the normal state for minutes.
//
// Everything here only READS. Explaining is best-effort and never fails a
// reconcile: a read error yields the component's own generic sentence.

// WorkloadRef names the workload whose pods explain a component's readiness.
type WorkloadRef struct {
	// Kind is Deployment, StatefulSet, or the CloudNativePG Cluster kind
	// (whose pods carry the cnpg.io/cluster label rather than a selector the
	// operator can read).
	Kind string
	Name string
	// Namespace is set only for a workload outside the platform's own
	// namespace (a shared sub-operator's controller); the objects an
	// explanation names then carry it too, so `kubectl describe` lands.
	Namespace string
}

// DeploymentRef, StatefulSetRef, and PostgresClusterRef build the reference
// for the three workload shapes the platform runs.
func DeploymentRef(name string) WorkloadRef      { return WorkloadRef{Kind: "Deployment", Name: name} }
func StatefulSetRef(name string) WorkloadRef     { return WorkloadRef{Kind: "StatefulSet", Name: name} }
func PostgresClusterRef(name string) WorkloadRef { return WorkloadRef{Kind: "Cluster", Name: name} }

// In returns the reference for the same workload in another namespace.
func (w WorkloadRef) In(namespace string) WorkloadRef {
	w.Namespace = namespace
	return w
}

// cnpgClusterLabel is the label CloudNativePG stamps on every instance pod
// and claim of a Cluster.
const cnpgClusterLabel = "cnpg.io/cluster"

// Explanation is one classified cause: the reason, the object it is about,
// and the sentence a person acts on.
type Explanation struct {
	Reason  v1.ComponentReason
	Object  *v1.ComponentObjectReference
	Message string
}

// startingUpPatience is how long a Running, restart-free pod may fail its
// health check before the sentence tells the person to look at the log. The
// control plane's first boot is around two minutes on a small cluster; ten
// gives a slow node room without hiding a real hang.
const startingUpPatience = 10 * time.Minute

// NotReady is the one not-ready answer every component returns: the most
// specific explanation the cluster supports for why workload is not Ready,
// or the component's own generic sentence when nothing more specific can be
// said (the workload has no pods yet, or a read failed).
func (b *Base) NotReady(ctx context.Context, c client.Client, namespace string, workload WorkloadRef, waiting string) Result {
	generic := Result{Ready: false, Reason: v1.ComponentReasonDeploying, Message: waiting}
	if expl := b.explainWorkload(ctx, c, namespace, workload); expl != nil {
		if expl.Object != nil && workload.Namespace != "" && workload.Namespace != namespace {
			expl.Object.Namespace = workload.Namespace
		}
		return Result{Ready: false, Reason: expl.Reason, Object: expl.Object, Message: expl.Message}
	}
	return generic
}

// explainWorkload gathers the facts and classifies them. Every read is
// best-effort: a failure to read one source drops that source's arms, never
// the whole explanation.
func (b *Base) explainWorkload(ctx context.Context, c client.Client, namespace string, workload WorkloadRef) *Explanation {
	if workload.Namespace != "" {
		namespace = workload.Namespace
	}
	selector, conditions, found := workloadSelector(ctx, c, namespace, workload)
	if !found {
		return nil
	}

	var pods corev1.PodList
	if err := c.List(ctx, &pods, client.InNamespace(namespace), client.MatchingLabels(selector)); err != nil {
		return nil
	}

	var claims corev1.PersistentVolumeClaimList
	_ = c.List(ctx, &claims, client.InNamespace(namespace))
	var events corev1.EventList
	_ = c.List(ctx, &events, client.InNamespace(namespace))

	if expl := explainPendingClaims(pods.Items, selector, claims.Items, storageFactsReader(ctx, c), events.Items); expl != nil {
		return expl
	}
	return classifyUnreadyWorkload(pods.Items, conditions, events.Items, time.Now())
}

// workloadSelector returns the label selector the workload's pods carry and
// the workload's own conditions (Deployments only; StatefulSets and Clusters
// have none the classifier reads). found is false when the workload does not
// exist yet -- there is nothing to explain before the first apply landed.
func workloadSelector(ctx context.Context, c client.Client, namespace string, workload WorkloadRef) (map[string]string, []appsv1.DeploymentCondition, bool) {
	if workload.Kind == "Cluster" {
		return map[string]string{cnpgClusterLabel: workload.Name}, nil, true
	}

	obj := &unstructured.Unstructured{}
	obj.SetGroupVersionKind(schema.GroupVersionKind{Group: "apps", Version: "v1", Kind: workload.Kind})
	if err := c.Get(ctx, types.NamespacedName{Name: workload.Name, Namespace: namespace}, obj); err != nil {
		return nil, nil, false
	}
	labels, found, err := unstructured.NestedStringMap(obj.Object, "spec", "selector", "matchLabels")
	if err != nil || !found || len(labels) == 0 {
		return nil, nil, false
	}

	var conditions []appsv1.DeploymentCondition
	if workload.Kind == "Deployment" {
		raw, _, _ := unstructured.NestedSlice(obj.Object, "status", "conditions")
		for _, item := range raw {
			cond, ok := item.(map[string]any)
			if !ok {
				continue
			}
			conditions = append(conditions, appsv1.DeploymentCondition{
				Type:    appsv1.DeploymentConditionType(stringField(cond, "type")),
				Status:  corev1.ConditionStatus(stringField(cond, "status")),
				Reason:  stringField(cond, "reason"),
				Message: stringField(cond, "message"),
			})
		}
	}
	return labels, conditions, true
}

func stringField(m map[string]any, key string) string {
	s, _ := m[key].(string)
	return s
}

// classifyUnreadyWorkload is the pure classifier over a workload's pods, its
// Deployment conditions, and the namespace's Events. It returns nil when the
// facts support nothing more specific than "still deploying". Pods are read
// newest first: during a rollout the newest pod's trouble is the one that
// matters, and an old pod that is fine says nothing about the new template.
func classifyUnreadyWorkload(pods []corev1.Pod, conditions []appsv1.DeploymentCondition, events []corev1.Event, now time.Time) *Explanation {
	sorted := make([]corev1.Pod, len(pods))
	copy(sorted, pods)
	sort.SliceStable(sorted, func(i, j int) bool {
		return sorted[i].CreationTimestamp.After(sorted[j].CreationTimestamp.Time)
	})

	// Container-level facts the kubelet states outright, across every pod,
	// before any pod-level verdict: a pull failure on the new pod beats the
	// old pod's healthy-but-terminating state.
	for i := range sorted {
		if expl := explainContainers(&sorted[i]); expl != nil {
			return expl
		}
	}

	for i := range sorted {
		if expl := explainScheduling(&sorted[i]); expl != nil {
			return expl
		}
		if expl := explainMount(&sorted[i], events); expl != nil {
			return expl
		}
	}

	if expl := explainDeploymentConditions(conditions); expl != nil {
		return expl
	}

	for i := range sorted {
		if expl := explainStartingUp(&sorted[i], events, now); expl != nil {
			return expl
		}
	}
	return nil
}

// explainContainers reads the container statuses of one pod, init containers
// first (a failing init container blocks everything after it).
func explainContainers(pod *corev1.Pod) *Explanation {
	all := append(append([]corev1.ContainerStatus{}, pod.Status.InitContainerStatuses...), pod.Status.ContainerStatuses...)
	object := podObject(pod)

	for _, cs := range all {
		if w := cs.State.Waiting; w != nil {
			switch w.Reason {
			case "ErrImagePull", "ImagePullBackOff", "InvalidImageName":
				return &Explanation{
					Reason: v1.ComponentReasonImagePullFailed,
					Object: object,
					Message: fmt.Sprintf(
						"image %s for container %q of pod %s cannot be pulled (%s: %s) -- "+
							"check that the tag exists and this cluster can reach the registry, "+
							"or point the component's image override at a registry it can reach",
						cs.Image, cs.Name, pod.Name, w.Reason, oneLine(w.Message)),
				}
			case "CreateContainerConfigError", "CreateContainerError":
				return &Explanation{
					Reason: v1.ComponentReasonContainerConfigInvalid,
					Object: object,
					Message: fmt.Sprintf(
						"container %q of pod %s cannot be created: %s -- "+
							"the named Secret or ConfigMap key must exist in this namespace before the pod can start",
						cs.Name, pod.Name, oneLine(w.Message)),
				}
			}
		}
	}

	// A kill or a crash reads from the last termination, whichever state the
	// container is in now (Waiting in CrashLoopBackOff, or already Running
	// again on its next attempt).
	for _, cs := range all {
		last := cs.LastTerminationState.Terminated
		if last == nil && cs.State.Terminated != nil {
			last = cs.State.Terminated
		}
		if last == nil {
			continue
		}
		if last.Reason == "OOMKilled" {
			return &Explanation{
				Reason: v1.ComponentReasonOutOfMemory,
				Object: object,
				Message: fmt.Sprintf(
					"container %q of pod %s was killed for exceeding its memory limit%s (%d restarts) -- "+
						"raise the component's memory limit in its resources, or give the node more memory",
					cs.Name, pod.Name, memoryLimitClause(pod, cs.Name), cs.RestartCount),
			}
		}
		crashLooping := cs.State.Waiting != nil && cs.State.Waiting.Reason == "CrashLoopBackOff"
		if crashLooping || cs.RestartCount > 0 && last.ExitCode != 0 {
			return &Explanation{
				Reason: v1.ComponentReasonCrashLooping,
				Object: object,
				Message: fmt.Sprintf(
					"container %q of pod %s keeps exiting (%d restarts, last exit code %d%s) -- "+
						"read its last log with kubectl logs -n %s %s -c %s --previous",
					cs.Name, pod.Name, cs.RestartCount, last.ExitCode, terminationClause(last), pod.Namespace, pod.Name, cs.Name),
			}
		}
	}
	return nil
}

// explainScheduling relays the scheduler's own verdict, which the pod carries
// on its PodScheduled condition -- no Event read needed.
func explainScheduling(pod *corev1.Pod) *Explanation {
	if pod.Status.Phase != corev1.PodPending {
		return nil
	}
	for _, cond := range pod.Status.Conditions {
		if cond.Type == corev1.PodScheduled && cond.Status == corev1.ConditionFalse && cond.Reason == corev1.PodReasonUnschedulable {
			return &Explanation{
				Reason: v1.ComponentReasonUnschedulable,
				Object: podObject(pod),
				Message: fmt.Sprintf(
					"no node can take pod %s: %s -- add capacity, lower the component's resource requests, "+
						"or remove the taint or affinity that excludes every node",
					pod.Name, oneLine(cond.Message)),
			}
		}
	}
	return nil
}

// explainMount relays the kubelet's FailedMount / FailedAttachVolume Event
// for a pod: the claim exists and is Bound, but the volume will not reach
// the node.
func explainMount(pod *corev1.Pod, events []corev1.Event) *Explanation {
	ev := newestEvent(events, "Pod", pod.Name, "FailedMount", "FailedAttachVolume")
	if ev == nil {
		return nil
	}
	return &Explanation{
		Reason: v1.ComponentReasonVolumeMountFailed,
		Object: podObject(pod),
		Message: fmt.Sprintf(
			"a volume of pod %s cannot be attached or mounted: %s -- "+
				"check the storage driver's own pods and that the volume is not still attached to another node",
			pod.Name, oneLine(ev.Message)),
	}
}

// explainDeploymentConditions reads the two verdicts the Deployment
// controller writes for its ReplicaSets: a creation the API server refused
// (a quota, an admission policy) and a rollout it gave up on.
func explainDeploymentConditions(conditions []appsv1.DeploymentCondition) *Explanation {
	for _, cond := range conditions {
		if cond.Type == appsv1.DeploymentReplicaFailure && cond.Status == corev1.ConditionTrue {
			return &Explanation{
				Reason: v1.ComponentReasonCreateRefused,
				Message: fmt.Sprintf(
					"the cluster refused to create this component's pods: %s -- "+
						"raise the namespace quota or adjust the admission policy that refused them",
					oneLine(cond.Message)),
			}
		}
	}
	for _, cond := range conditions {
		if cond.Type == appsv1.DeploymentProgressing && cond.Status == corev1.ConditionFalse && cond.Reason == "ProgressDeadlineExceeded" {
			return &Explanation{
				Reason: v1.ComponentReasonRolloutStalled,
				Message: fmt.Sprintf(
					"the rollout stopped making progress and the Deployment controller gave up (%s) -- "+
						"the pods' own events name the cause; kubectl describe the Deployment and its newest pod",
					oneLine(cond.Message)),
			}
		}
	}
	return nil
}

// explainStartingUp is the calm arm: a pod that runs with no restarts and has
// not answered its health check yet. The sentence carries how long since the
// container started and, once patience has run out, where to look.
func explainStartingUp(pod *corev1.Pod, events []corev1.Event, now time.Time) *Explanation {
	if pod.Status.Phase != corev1.PodRunning || podReady(pod) {
		return nil
	}
	var started time.Time
	for _, cs := range pod.Status.ContainerStatuses {
		if cs.RestartCount > 0 {
			return nil
		}
		if cs.State.Running != nil && (started.IsZero() || cs.State.Running.StartedAt.Time.Before(started)) {
			started = cs.State.Running.StartedAt.Time
		}
	}
	if started.IsZero() {
		return nil
	}
	since := now.Sub(started).Truncate(time.Second)
	probe := ""
	if ev := newestEvent(events, "Pod", pod.Name, "Unhealthy"); ev != nil {
		probe = " (" + oneLine(ev.Message) + ")"
	}
	guidance := "normal in the first minutes of a boot"
	if since > startingUpPatience {
		guidance = fmt.Sprintf("longer than expected; read its log with kubectl logs -n %s %s", pod.Namespace, pod.Name)
	}
	return &Explanation{
		Reason: v1.ComponentReasonStartingUp,
		Object: podObject(pod),
		Message: fmt.Sprintf("pod %s is running and not yet answering its health check%s, %s since start -- %s",
			pod.Name, probe, since, guidance),
	}
}

// explainPendingClaims classifies the Pending volume claims that belong to
// the workload: the ones its pods reference, and -- for the moment before the
// first pod exists -- the ones carrying the workload's own selector labels
// (a StatefulSet stamps its template labels on the claims it creates;
// CloudNativePG stamps its cluster label). Never a name fragment. A claim the
// classifier can condemn is a failure; a claim that is merely still
// provisioning is the calm VolumeProvisioning arm, so the person knows the
// wait is the storage backend's. facts is read lazily: the StorageClass and
// CSIDriver lists are only fetched when a claim is actually Pending.
func explainPendingClaims(pods []corev1.Pod, selector map[string]string, claims []corev1.PersistentVolumeClaim, facts func() storageFacts, events []corev1.Event) *Explanation {
	referenced := map[string]bool{}
	for i := range pods {
		for _, vol := range pods[i].Spec.Volumes {
			if vol.PersistentVolumeClaim != nil {
				referenced[vol.PersistentVolumeClaim.ClaimName] = true
			}
		}
	}
	belongs := func(pvc *corev1.PersistentVolumeClaim) bool {
		if referenced[pvc.Name] {
			return true
		}
		if len(selector) == 0 {
			return false
		}
		for k, v := range selector {
			if pvc.Labels[k] != v {
				return false
			}
		}
		return true
	}
	for i := range claims {
		pvc := &claims[i]
		if pvc.Status.Phase != corev1.ClaimPending || !belongs(pvc) {
			continue
		}
		object := &v1.ComponentObjectReference{Kind: "PersistentVolumeClaim", Name: pvc.Name}
		f := facts()
		if msg, ok := classifyPendingPVC(pvc, f.classes, f.drivers, events); ok {
			return &Explanation{Reason: v1.ComponentReasonVolumeUnprovisionable, Object: object, Message: msg}
		}
		return &Explanation{
			Reason: v1.ComponentReasonVolumeProvisioning,
			Object: object,
			Message: fmt.Sprintf("volume %s is waiting for the storage backend to provision it%s -- "+
				"usually under a minute; if it stays Pending, kubectl describe the claim for the provisioner's own events",
				pvc.Name, claimClassClause(pvc)),
		}
	}
	return nil
}

// SubOperatorNotReady is NotReady for a shared sub-operator (CloudNativePG,
// Tekton) whose controller Deployments live in their own namespace: the first
// Deployment that is not serving is the one explained, with the namespace
// stamped on the object so the next command lands. A missing Deployment is a
// partial install EnsureSubOperator is already resuming, and gets the
// generic sentence.
func (b *Base) SubOperatorNotReady(ctx context.Context, c client.Client, platformNamespace string, opts SubOperatorOptions, waiting string) Result {
	for _, name := range opts.Deployments {
		exists, ready, err := b.deploymentState(ctx, c, name, opts.Namespace)
		if err != nil || !exists || ready {
			continue
		}
		return b.NotReady(ctx, c, platformNamespace, DeploymentRef(name).In(opts.Namespace), waiting)
	}
	return Result{Ready: false, Reason: v1.ComponentReasonDeploying, Message: waiting}
}

// explainJobs reads the one-time Jobs a component owns (selected by label)
// and explains the one that matters: a Job still running is the calm
// WaitingForSchema arm -- the workload's restarts meanwhile are expected --
// and a Job that failed is the cause the workload's pods can only echo.
// Returns nil when no Job is running or failed (finished Jobs and absent
// Jobs say nothing about readiness). purpose names the Job in the sentence
// ("Temporal schema setup").
func (b *Base) explainJobs(ctx context.Context, c client.Client, namespace string, selector map[string]string, purpose string) *Explanation {
	var jobs batchv1.JobList
	if err := c.List(ctx, &jobs, client.InNamespace(namespace), client.MatchingLabels(selector)); err != nil {
		return nil
	}
	return classifyJobs(jobs.Items, purpose)
}

// classifyJobs is the pure half of explainJobs: a failed Job first (it is
// the cause), then a running one (it is the wait).
func classifyJobs(jobs []batchv1.Job, purpose string) *Explanation {
	for i := range jobs {
		job := &jobs[i]
		for _, cond := range job.Status.Conditions {
			if cond.Type == batchv1.JobFailed && cond.Status == corev1.ConditionTrue {
				return &Explanation{
					Reason: v1.ComponentReasonJobFailed,
					Object: &v1.ComponentObjectReference{Kind: "Job", Name: job.Name},
					Message: fmt.Sprintf(
						"the %s job %s failed (%s: %s) -- read its log with kubectl logs -n %s job/%s; "+
							"the workload depending on it cannot start until the job succeeds",
						purpose, job.Name, cond.Reason, oneLine(cond.Message), job.Namespace, job.Name),
				}
			}
		}
	}
	for i := range jobs {
		job := &jobs[i]
		if job.Status.Succeeded > 0 {
			continue
		}
		if job.Status.Active > 0 || (job.Status.Failed == 0 && job.Status.CompletionTime == nil) {
			return &Explanation{
				Reason: v1.ComponentReasonWaitingForSchema,
				Object: &v1.ComponentObjectReference{Kind: "Job", Name: job.Name},
				Message: fmt.Sprintf(
					"waiting for the %s job %s to finish -- server pods restart until it does, which is expected on a first boot",
					purpose, job.Name),
			}
		}
	}
	return nil
}

// --- small helpers -------------------------------------------------------

func podObject(pod *corev1.Pod) *v1.ComponentObjectReference {
	return &v1.ComponentObjectReference{Kind: "Pod", Name: pod.Name}
}

func podReady(pod *corev1.Pod) bool {
	for _, cond := range pod.Status.Conditions {
		if cond.Type == corev1.PodReady {
			return cond.Status == corev1.ConditionTrue
		}
	}
	return false
}

// memoryLimitClause names the container's memory limit when the pod declares
// one; a kill with no limit means the node itself ran out.
func memoryLimitClause(pod *corev1.Pod, container string) string {
	for _, c := range pod.Spec.Containers {
		if c.Name != container {
			continue
		}
		if limit, ok := c.Resources.Limits[corev1.ResourceMemory]; ok {
			return " of " + limit.String()
		}
	}
	return " (no limit set: the node itself ran out of memory)"
}

// terminationClause adds the kubelet's own termination reason and message
// when they say more than the exit code.
func terminationClause(t *corev1.ContainerStateTerminated) string {
	parts := []string{}
	if t.Reason != "" && t.Reason != "Error" {
		parts = append(parts, t.Reason)
	}
	if msg := oneLine(t.Message); msg != "" {
		parts = append(parts, msg)
	}
	if len(parts) == 0 {
		return ""
	}
	return ", " + strings.Join(parts, ": ")
}

func claimClassClause(pvc *corev1.PersistentVolumeClaim) string {
	if pvc.Spec.StorageClassName != nil && *pvc.Spec.StorageClassName != "" {
		return " through StorageClass " + *pvc.Spec.StorageClassName
	}
	return " through the cluster's default StorageClass"
}

// newestEvent returns the newest Event about one object carrying any of the
// given reasons, or nil.
func newestEvent(events []corev1.Event, kind, name string, reasons ...string) *corev1.Event {
	var newest *corev1.Event
	for i := range events {
		ev := &events[i]
		if ev.InvolvedObject.Kind != kind || ev.InvolvedObject.Name != name {
			continue
		}
		if !slices.Contains(reasons, ev.Reason) {
			continue
		}
		if newest == nil || eventTime(ev).After(eventTime(newest)) {
			newest = ev
		}
	}
	return newest
}

// eventTime is the Event's most recent timestamp whichever field the writer
// filled (the legacy lastTimestamp, or the series/eventTime pair newer
// writers use).
func eventTime(ev *corev1.Event) time.Time {
	if ev.Series != nil && !ev.Series.LastObservedTime.IsZero() {
		return ev.Series.LastObservedTime.Time
	}
	if !ev.LastTimestamp.IsZero() {
		return ev.LastTimestamp.Time
	}
	if !ev.EventTime.IsZero() {
		return ev.EventTime.Time
	}
	return ev.CreationTimestamp.Time
}

// oneLine collapses a multi-line upstream message so the status column stays
// a sentence.
func oneLine(s string) string {
	return strings.Join(strings.Fields(s), " ")
}
