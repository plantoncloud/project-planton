package resources

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

const (
	// RunnerDefaultImageRepo is the official runner image. It bundles the
	// planton-runner binary plus the IaC toolchain (OpenTofu, Pulumi, cloud
	// auth exec plugins), so the pod needs no init or sidecar containers.
	RunnerDefaultImageRepo = "ghcr.io/plantonhq/planton/runner"

	// RunnerBadgeAudience is the audience the runner's projected
	// ServiceAccount token is minted for, and the audience the control
	// plane's badge verification expects (KUBERNETES_WORKLOAD_AUTH_AUDIENCE)
	// -- the same value the hosted deployment uses, so one convention serves
	// every badge lane. The Kubernetes API server enforces the match during
	// TokenReview, which is what makes a badge captured at one door useless
	// at any other.
	RunnerBadgeAudience = "planton-control-plane"

	// runnerBadgeTokenDir is where the projected ServiceAccount token is
	// mounted in the runner pod; PLANTON_RUNNER_WORKLOAD_IDENTITY_TOKEN_FILE
	// points into it. The kubelet rotates the file and the runner binary
	// re-reads it per call, so no process ever holds a stale badge.
	runnerBadgeTokenDir = "/var/run/secrets/planton.ai"

	// runnerBadgeTokenFile is the projected token's file name under
	// runnerBadgeTokenDir.
	runnerBadgeTokenFile = "token"

	// runnerBadgeTokenExpirationSeconds is the projected token's lifetime.
	// One hour is the kubelet's conventional projection window: long enough
	// that rotation is cheap, short enough that a leaked file ages out fast.
	runnerBadgeTokenExpirationSeconds = int64(3600)

	// RunnerDefaultStorageSize sizes the IaC state volume. Tofu state files
	// are kilobytes each; 2Gi leaves headroom for hundreds of resources plus
	// provider lock metadata.
	RunnerDefaultStorageSize = "2Gi"

	// runnerGrpcPort hosts the runner's gRPC server: the CloudOps surface the
	// control plane direct-dials for live cloud operations, plus the health
	// keys the pod's probes dial (worker-poll readiness rides the same
	// server in dual mode).
	runnerGrpcPort = 50051

	// runnerWebhookPort hosts the runner's Tekton CloudEvents webhook -- the
	// in-cluster HTTP door Tekton posts pipeline events to when builds are
	// enabled. Matches the runner binary's default WEBHOOK_PORT; set
	// explicitly so the Deployment states its contract.
	runnerWebhookPort = 8086

	// RunnerWorkerHealthService is the gRPC health-check key under which the
	// runner reports "my IaC worker is polling its task queue". MUST match
	// the runner binary's temporal.WorkerHealthService constant -- two
	// processes agreeing on a name by convention, exactly like the desktop
	// daemon's runner supervision does.
	RunnerWorkerHealthService = "ai.planton.runner.iac.worker"

	// runnerIacStateDir is where the local state backend keeps each
	// resource's OpenTofu state, mounted from the PVC. The path is an
	// operator-internal contract with the Deployment's volumeMount.
	runnerIacStateDir = "/var/lib/planton/iac-state"

	// runnerIacCacheDir holds the module cache and per-job workspaces --
	// disposable, so it rides an emptyDir, not the state volume.
	runnerIacCacheDir = "/var/cache/planton"

	// runnerTemporalNamespace is the Temporal namespace the control plane
	// dispatches IaC-operation activities into (Java
	// PlatformTemporalNamespaces.pipelines). The worker MUST poll the same
	// namespace or deploys sit pending forever -- the same cross-process
	// contract the desktop daemon documents for its local runner.
	runnerTemporalNamespace = "platform.pipelines"

	// RunnerCloudOpsSecretKeyToken is the Secret data key holding the
	// CloudOps auth token -- the shared bearer both sides of the direct
	// dial hold: the control plane attaches it to every live cloud-operation
	// call (RUNNER_DIRECT_AUTH_TOKEN) and the runner requires it on its gRPC
	// surface (CLOUDOPS_AUTH_TOKEN). It is the perimeter that lets the
	// runner's ambient-credential CloudOps API be served across pods without
	// the mTLS tunnel (which exists to cross networks, not namespaces).
	RunnerCloudOpsSecretKeyToken = "cloudops-auth-token"

	// runnerTokenBytes is 32 bytes (256-bit) of entropy for operator-minted
	// bearer tokens -- the same entropy class the control plane's own
	// credential generators use.
	runnerTokenBytes = 32

	// runnerCloudOpsTokenPrefix distinguishes the CloudOps direct-dial token
	// from Planton API keys (pak_) at a glance -- the two are different trust
	// relationships (runner->control-plane vs control-plane->runner), and a
	// recognizable prefix stops one ever being pasted where the other
	// belongs. Not a control-plane-recognized credential shape by design.
	runnerCloudOpsTokenPrefix = "pcot_"
)

// RunnerConfig bundles all inputs needed to build the runner's resources.
type RunnerConfig struct {
	CRName    string
	Namespace string
	Version   string
	OwnerRef  *metav1.OwnerReference

	ImageRepository string
	ImageTag        string

	// OrgSlug is the bootstrap organization the runner belongs to; with the
	// slug it derives the registration and the Temporal task queue.
	OrgSlug string

	StorageSize resource.Quantity

	// StorageClassName pins the IaC-state PVC to a StorageClass. Empty
	// leaves the field nil so the cluster default provisions. Applied at
	// creation only -- the PVC is create-once and an existing volume keeps
	// its class.
	StorageClassName string

	// ServiceAccountAnnotations carry the workload-identity binding (IRSA /
	// GKE WI / Azure WI) onto the runner's dedicated ServiceAccount.
	ServiceAccountAnnotations map[string]string

	// CloudCredentialsSecretName, when set, is envFrom-injected into the
	// runner pod -- the static-credentials path for clusters without
	// workload identity. The Secret is customer-owned; the operator only
	// references it.
	CloudCredentialsSecretName string

	// BuildEnabled turns on the runner's build capability: the pipeline-build
	// Temporal worker, the Tekton CloudEvents webhook (container port +
	// Service exposure), the build namespace default, and the build RBAC.
	// Follows the effective build toggle (spec.build AND spec.runner) -- the
	// Tekton component installs the engine, this flag makes the runner use it.
	BuildEnabled bool
}

// RunnerDeploymentName returns the Deployment name: "{crName}-runner".
func RunnerDeploymentName(crName string) string {
	return fmt.Sprintf("%s-runner", crName)
}

// RunnerServiceAccountName returns the runner pod's dedicated ServiceAccount
// name: "{crName}-runner". This name IS the runner's registration slug (see
// RunnerSlug) -- the badge identity-binding grammar the control plane's write
// pipelines enforce (ServiceAccount name == slug), so the subject the cluster
// asserts and the registration the platform seeds can never drift apart.
func RunnerServiceAccountName(crName string) string {
	return fmt.Sprintf("%s-runner", crName)
}

// RunnerSlug returns the in-cluster runner's registration slug -- BY LAW the
// runner's Kubernetes ServiceAccount name (the badge grammar: the verified
// subject's ServiceAccount name is the runner's name; a namespace cannot
// name a ServiceAccount "default" for this because Kubernetes reserves it).
// One runner per install for now, so the slug carries no ceremony; it names
// the seeded registration, the Planton-side identity, and the task queue
// segment.
func RunnerSlug(crName string) string {
	return RunnerServiceAccountName(crName)
}

// RunnerStatePVCName returns the IaC state PVC name: "{crName}-runner-state".
func RunnerStatePVCName(crName string) string {
	return fmt.Sprintf("%s-runner-state", crName)
}

// RunnerCloudOpsSecretName returns the CloudOps token Secret name:
// "{crName}-runner-cloudops". The Secret carries exactly one credential --
// the direct-dial perimeter token -- and is named for it; the runner's
// platform identity is keyless (the badge) and rides no Secret at all.
func RunnerCloudOpsSecretName(crName string) string {
	return fmt.Sprintf("%s-runner-cloudops", crName)
}

// RunnerServiceName returns the runner Service name: "{crName}-runner".
func RunnerServiceName(crName string) string {
	return fmt.Sprintf("%s-runner", crName)
}

// RunnerServiceFQDN returns the in-cluster DNS name the control plane
// direct-dials for live cloud operations.
func RunnerServiceFQDN(crName, namespace string) string {
	return fmt.Sprintf("%s.%s.svc.cluster.local", RunnerServiceName(crName), namespace)
}

// RunnerChannelIdentifier returns the runner's channel identifier
// ("org.{org}.runner.{slug}"), from which both the runner binary and the
// control plane independently derive the Temporal task queue.
func RunnerChannelIdentifier(crName, orgSlug string) string {
	return fmt.Sprintf("org.%s.runner.%s", orgSlug, RunnerSlug(crName))
}

// RunnerTaskQueue returns the Temporal task queue the runner polls and the
// control plane dispatches to: "iac-operation." + channel identifier. The
// prefix mirrors the runner binary's temporal.TaskQueuePrefix and infra-hub's
// producer-side convention -- deterministic on both sides, no shared state.
func RunnerTaskQueue(crName, orgSlug string) string {
	return "iac-operation." + RunnerChannelIdentifier(crName, orgSlug)
}

// GenerateRunnerCloudOpsToken mints the CloudOps direct-dial bearer:
// "pcot_" + Base64URL(32 random bytes).
func GenerateRunnerCloudOpsToken() (string, error) {
	b := make([]byte, runnerTokenBytes)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("generating %s credential: %w", runnerCloudOpsTokenPrefix, err)
	}
	return runnerCloudOpsTokenPrefix + base64.RawURLEncoding.EncodeToString(b), nil
}

// RunnerIdentityDocumentJSON renders the SECRET-FREE identity document the
// runner binary boots from: identity coordinates + the in-cluster control
// plane endpoint, and deliberately NO credential of any kind -- the runner's
// proof is its projected ServiceAccount badge, presented per call from the
// file PLANTON_RUNNER_WORKLOAD_IDENTITY_TOKEN_FILE names. Because nothing in
// it is secret, it rides the Deployment env inline (the local instance's
// precedent for the same document), never a Secret. No tunnel endpoint and
// no certificates -- the mTLS tunnel is the remote runner's transport, not
// this one's.
func RunnerIdentityDocumentJSON(crName, namespace, orgSlug string) string {
	doc := map[string]string{
		"type":               "planton_runner",
		"org":                orgSlug,
		"runner":             RunnerSlug(crName),
		"channel_identifier": RunnerChannelIdentifier(crName, orgSlug),
		"planton_api_endpoint": fmt.Sprintf("%s:%d",
			ControlPlaneServiceFQDN(crName, namespace), controlPlaneServicePort),
	}
	// A map[string]string marshals totally; no error path exists.
	out, _ := json.Marshal(doc)
	return string(out)
}

// RunnerCloudOpsSecret builds the Secret carrying the CloudOps direct-dial
// token -- the one operator-minted credential that survives on this install,
// held by both sides of the dial. The caller preserves an existing token
// across reconciles: rotating it is a deliberate act (delete the Secret),
// not a side effect.
func RunnerCloudOpsSecret(crName, namespace, cloudOpsToken string, ownerRef *metav1.OwnerReference) *corev1.Secret {
	secret := &corev1.Secret{
		TypeMeta: metav1.TypeMeta{APIVersion: "v1", Kind: "Secret"},
		ObjectMeta: metav1.ObjectMeta{
			Name:      RunnerCloudOpsSecretName(crName),
			Namespace: namespace,
			Labels:    runnerLabels(crName),
		},
		Type: corev1.SecretTypeOpaque,
		Data: map[string][]byte{
			RunnerCloudOpsSecretKeyToken: []byte(cloudOpsToken),
		},
	}
	if ownerRef != nil {
		secret.OwnerReferences = []metav1.OwnerReference{*ownerRef}
	}
	return secret
}

// RunnerService builds the ClusterIP Service the control plane direct-dials
// for live cloud operations (CloudOps). The runner's gRPC surface behind it
// requires the CloudOps auth token on every non-health call, which is what
// makes serving it across pods safe -- the Service is deliberately created
// together with the token env on both sides, never alone.
//
// With builds enabled the Service additionally exposes the Tekton CloudEvents
// webhook on port 80 (so the sink URL needs no explicit port -- see
// TektonEventsSinkURL). The webhook is unauthenticated by the runner's design:
// its trust boundary is the cluster network, which is exactly the scope a
// ClusterIP Service grants.
func RunnerService(cfg RunnerConfig) *corev1.Service {
	ports := []corev1.ServicePort{{
		Name:       "grpc",
		Port:       runnerGrpcPort,
		TargetPort: intstr.FromInt32(runnerGrpcPort),
		Protocol:   corev1.ProtocolTCP,
	}}
	if cfg.BuildEnabled {
		ports = append(ports, corev1.ServicePort{
			Name:       "webhook",
			Port:       80,
			TargetPort: intstr.FromString("webhook"),
			Protocol:   corev1.ProtocolTCP,
		})
	}

	svc := &corev1.Service{
		TypeMeta: metav1.TypeMeta{APIVersion: "v1", Kind: "Service"},
		ObjectMeta: metav1.ObjectMeta{
			Name:      RunnerServiceName(cfg.CRName),
			Namespace: cfg.Namespace,
			Labels:    runnerLabels(cfg.CRName),
		},
		Spec: corev1.ServiceSpec{
			Type:     corev1.ServiceTypeClusterIP,
			Selector: runnerLabels(cfg.CRName),
			Ports:    ports,
		},
	}
	if cfg.OwnerRef != nil {
		svc.OwnerReferences = []metav1.OwnerReference{*cfg.OwnerRef}
	}
	return svc
}

// RunnerBuildRoleName returns the build Role name: "{crName}-runner-build".
func RunnerBuildRoleName(crName string) string {
	return fmt.Sprintf("%s-runner-build", crName)
}

// RunnerBuildRole grants the runner's ServiceAccount exactly the verbs the
// build capability uses in the build namespace (the CR namespace, where
// TEKTON_NAMESPACE points). The verb list mirrors the runner's own readiness
// probe (its rbac-build and rbac-logs checks assert these exact needs via
// SelfSubjectAccessReview) -- change one side only and verify reports the
// drift. The roles/rolebindings grants carry no privilege-escalation hazard:
// every permission the runner hands a build's ServiceAccount it must already
// hold itself, and Kubernetes enforces exactly that at grant time.
func RunnerBuildRole(cfg RunnerConfig) *rbacv1.Role {
	role := &rbacv1.Role{
		TypeMeta: metav1.TypeMeta{APIVersion: "rbac.authorization.k8s.io/v1", Kind: "Role"},
		ObjectMeta: metav1.ObjectMeta{
			Name:      RunnerBuildRoleName(cfg.CRName),
			Namespace: cfg.Namespace,
			Labels:    runnerLabels(cfg.CRName),
		},
		Rules: []rbacv1.PolicyRule{
			// Creating a build's PipelineRun, watching the run inventory the
			// reconcile safety net lists, and the labeled cleanup sweep.
			{
				APIGroups: []string{"tekton.dev"},
				Resources: []string{"pipelineruns"},
				Verbs:     []string{"create", "list", "deletecollection"},
			},
			{
				APIGroups: []string{"tekton.dev"},
				Resources: []string{"taskruns"},
				Verbs:     []string{"list"},
			},
			// Per-build workspace objects the create activity provisions and
			// the cleanup activity sweeps.
			{
				APIGroups: []string{""},
				Resources: []string{"secrets", "serviceaccounts"},
				Verbs:     []string{"create", "get", "deletecollection"},
			},
			{
				APIGroups: []string{""},
				Resources: []string{"configmaps"},
				Verbs:     []string{"get", "deletecollection"},
			},
			{
				APIGroups: []string{"rbac.authorization.k8s.io"},
				Resources: []string{"roles", "rolebindings"},
				Verbs:     []string{"create", "deletecollection"},
			},
			// The log streamer follows build task pods and ships their logs
			// to the control plane.
			{
				APIGroups: []string{""},
				Resources: []string{"pods"},
				Verbs:     []string{"get", "list", "watch"},
			},
			{
				APIGroups: []string{""},
				Resources: []string{"pods/log"},
				Verbs:     []string{"get"},
			},
		},
	}
	if cfg.OwnerRef != nil {
		role.OwnerReferences = []metav1.OwnerReference{*cfg.OwnerRef}
	}
	return role
}

// RunnerBuildRoleBinding binds the build Role to the runner's dedicated
// ServiceAccount.
func RunnerBuildRoleBinding(cfg RunnerConfig) *rbacv1.RoleBinding {
	binding := &rbacv1.RoleBinding{
		TypeMeta: metav1.TypeMeta{APIVersion: "rbac.authorization.k8s.io/v1", Kind: "RoleBinding"},
		ObjectMeta: metav1.ObjectMeta{
			Name:      RunnerBuildRoleName(cfg.CRName),
			Namespace: cfg.Namespace,
			Labels:    runnerLabels(cfg.CRName),
		},
		RoleRef: rbacv1.RoleRef{
			APIGroup: "rbac.authorization.k8s.io",
			Kind:     "Role",
			Name:     RunnerBuildRoleName(cfg.CRName),
		},
		Subjects: []rbacv1.Subject{{
			Kind:      "ServiceAccount",
			Name:      RunnerServiceAccountName(cfg.CRName),
			Namespace: cfg.Namespace,
		}},
	}
	if cfg.OwnerRef != nil {
		binding.OwnerReferences = []metav1.OwnerReference{*cfg.OwnerRef}
	}
	return binding
}

// RunnerServiceAccount builds the runner pod's dedicated ServiceAccount. It
// exists even without annotations so granting cloud access later is a pure
// annotation edit (no pod identity change), and so the runner never runs as
// the namespace default account.
func RunnerServiceAccount(cfg RunnerConfig) *corev1.ServiceAccount {
	sa := &corev1.ServiceAccount{
		TypeMeta: metav1.TypeMeta{APIVersion: "v1", Kind: "ServiceAccount"},
		ObjectMeta: metav1.ObjectMeta{
			Name:        RunnerServiceAccountName(cfg.CRName),
			Namespace:   cfg.Namespace,
			Labels:      runnerLabels(cfg.CRName),
			Annotations: cfg.ServiceAccountAnnotations,
		},
	}
	if cfg.OwnerRef != nil {
		sa.OwnerReferences = []metav1.OwnerReference{*cfg.OwnerRef}
	}
	return sa
}

// RunnerStatePVC builds the persistent volume claim holding IaC state.
// Created once and never mutated afterwards (PVC storage requests are
// immutable); like the credential Secrets, an existing claim is left alone.
func RunnerStatePVC(cfg RunnerConfig) *corev1.PersistentVolumeClaim {
	size := cfg.StorageSize
	if size.IsZero() {
		size = resource.MustParse(RunnerDefaultStorageSize)
	}
	pvc := &corev1.PersistentVolumeClaim{
		TypeMeta: metav1.TypeMeta{APIVersion: "v1", Kind: "PersistentVolumeClaim"},
		ObjectMeta: metav1.ObjectMeta{
			Name:      RunnerStatePVCName(cfg.CRName),
			Namespace: cfg.Namespace,
			Labels:    runnerLabels(cfg.CRName),
		},
		Spec: corev1.PersistentVolumeClaimSpec{
			AccessModes: []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOnce},
			Resources: corev1.VolumeResourceRequirements{
				Requests: corev1.ResourceList{corev1.ResourceStorage: size},
			},
		},
	}
	if cfg.StorageClassName != "" {
		// Nil (not "") when unpinned: an explicit empty string means "only
		// bind pre-provisioned volumes", never the cluster default.
		pvc.Spec.StorageClassName = &cfg.StorageClassName
	}
	if cfg.OwnerRef != nil {
		pvc.OwnerReferences = []metav1.OwnerReference{*cfg.OwnerRef}
	}
	return pvc
}

// RunnerDeployment builds the runner Deployment: one replica in dual mode
// (IaC worker + the CloudOps gRPC surface the control plane direct-dials).
// Exactly one replica, Recreate strategy -- the local state backend and
// per-resource state files live on a ReadWriteOnce volume, and two workers
// mutating the same state directory would corrupt it. Scaling runners is a
// registered-runner story, not a replica knob.
func RunnerDeployment(cfg RunnerConfig) *appsv1.Deployment {
	imageRepo := cfg.ImageRepository
	if imageRepo == "" {
		imageRepo = RunnerDefaultImageRepo
	}
	imageTag := cfg.ImageTag
	if imageTag == "" {
		imageTag = cfg.Version
	}
	replicas := int32(1)
	labels := runnerLabels(cfg.CRName)

	// The build capability's env renders explicitly in BOTH states so the
	// Deployment states its contract (the runner binary's defaults are ON;
	// relying on them would make "builds disabled" invisible in kubectl
	// describe). Worker and webhook flip together: the worker executes
	// builds, the webhook receives their live events -- one without the
	// other is a half-capability that only degrades.
	buildFlag := fmt.Sprintf("%t", cfg.BuildEnabled)

	env := []corev1.EnvVar{
		// Dual mode: the IaC worker plus the CloudOps gRPC surface for live
		// cloud operations (wizard verifications, K8s browsing, inventory).
		// The tunnel stays off -- it exists to cross networks, and this
		// runner shares the control plane's -- so the CloudOps surface's
		// perimeter is the auth token below: the runner binds beyond
		// loopback ONLY because every non-health call must present it.
		{Name: "EXECUTION_MODE", Value: "dual"},
		{Name: "TUNNEL_ENABLED", Value: "false"},
		{Name: "WEBHOOK_ENABLED", Value: buildFlag},
		{Name: "BUILD_WORKER_ENABLED", Value: buildFlag},
		{Name: "PORT", Value: fmt.Sprintf("%d", runnerGrpcPort)},
		// The token's plaintext never lands in the pod spec; the same Secret
		// key feeds the control plane's RUNNER_DIRECT_AUTH_TOKEN, so the two
		// sides cannot disagree.
		secretEnv("CLOUDOPS_AUTH_TOKEN",
			RunnerCloudOpsSecretName(cfg.CRName), RunnerCloudOpsSecretKeyToken),
		// The identity document is secret-free (no api_key), so it rides the
		// env inline -- the runner's auth ladder then falls through to the
		// badge below. Nothing about WHO this runner is lives in a Secret.
		{Name: "PLANTON_RUNNER_CREDENTIALS",
			Value: RunnerIdentityDocumentJSON(cfg.CRName, cfg.Namespace, cfg.OrgSlug)},
		// The badge: a projected, audience-bound ServiceAccount token the
		// control plane verifies with the cluster itself (TokenReview). The
		// runner re-reads the file per call, so kubelet rotation needs no
		// process cooperation.
		{Name: "PLANTON_RUNNER_WORKLOAD_IDENTITY_TOKEN_FILE",
			Value: fmt.Sprintf("%s/%s", runnerBadgeTokenDir, runnerBadgeTokenFile)},
		{Name: "TEMPORAL_SERVICE_ADDRESS", Value: TemporalFrontendEndpoint(cfg.CRName, cfg.Namespace)},
		// The dispatch namespace, NOT the runner's "default" fallback --
		// wrong namespace means the worker polls the right queue in the
		// wrong place and no deploy is ever picked up. The pipeline-build
		// worker rides the SAME shared client and namespace: the control
		// plane's build-stage workflows pin their Tekton activities to
		// queues in this namespace too, so one value serves both workers.
		{Name: "TEMPORAL_NAMESPACE", Value: runnerTemporalNamespace},
		{Name: "IAC_RUNNER_CACHE_DIR", Value: runnerIacCacheDir},
		{Name: "PLANTON_LOCAL_IAC_STATE_DIR", Value: runnerIacStateDir},
	}

	if cfg.BuildEnabled {
		env = append(env,
			// Build PipelineRuns land in the CR namespace (beside the runner)
			// when a build connection leaves placement empty, and the log
			// streamer watches exactly this namespace. Explicit rather than
			// the binary's own-namespace fallback: the Deployment states its
			// contract. Exactly ONE build-capable runner may watch a Tekton
			// namespace (the log streamer is a singleton per namespace) --
			// held by construction here, this Deployment is 1 replica.
			corev1.EnvVar{Name: "TEKTON_NAMESPACE", Value: cfg.Namespace},
			corev1.EnvVar{Name: "WEBHOOK_PORT", Value: fmt.Sprintf("%d", runnerWebhookPort)},
		)
	}

	var envFrom []corev1.EnvFromSource
	if cfg.CloudCredentialsSecretName != "" {
		envFrom = append(envFrom, corev1.EnvFromSource{
			SecretRef: &corev1.SecretEnvSource{
				LocalObjectReference: corev1.LocalObjectReference{
					Name: cfg.CloudCredentialsSecretName,
				},
			},
		})
	}

	ports := []corev1.ContainerPort{{
		Name:          "grpc",
		ContainerPort: runnerGrpcPort,
		Protocol:      corev1.ProtocolTCP,
	}}
	if cfg.BuildEnabled {
		ports = append(ports, corev1.ContainerPort{
			Name:          "webhook",
			ContainerPort: runnerWebhookPort,
			Protocol:      corev1.ProtocolTCP,
		})
	}

	deploy := &appsv1.Deployment{
		TypeMeta: metav1.TypeMeta{APIVersion: "apps/v1", Kind: "Deployment"},
		ObjectMeta: metav1.ObjectMeta{
			Name:      RunnerDeploymentName(cfg.CRName),
			Namespace: cfg.Namespace,
			Labels:    labels,
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: &replicas,
			Selector: &metav1.LabelSelector{MatchLabels: labels},
			// Recreate: the state PVC is ReadWriteOnce, so the old pod must
			// release it before the new one can bind.
			Strategy: appsv1.DeploymentStrategy{Type: appsv1.RecreateDeploymentStrategyType},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{Labels: labels},
				Spec: corev1.PodSpec{
					ServiceAccountName: RunnerServiceAccountName(cfg.CRName),
					Containers: []corev1.Container{{
						Name:  "runner",
						Image: fmt.Sprintf("%s:%s", imageRepo, imageTag),
						// The image's entrypoint is the BARE binary by
						// contract — every container consumer passes the
						// subcommand. Without this the pod prints help and
						// exits, which reads as a CrashLoop with no error.
						// The runner's name and identity come from the env
						// below (credentials + projected badge), so start
						// takes no further arguments here.
						Args:    []string{"start"},
						Env:     env,
						EnvFrom: envFrom,
						Ports:   ports,
						VolumeMounts: []corev1.VolumeMount{
							{Name: "badge-token", MountPath: runnerBadgeTokenDir, ReadOnly: true},
							{Name: "iac-state", MountPath: runnerIacStateDir},
							{Name: "iac-cache", MountPath: runnerIacCacheDir},
						},
						// Readiness = "the IaC worker is polling its task
						// queue", not merely "the process is up": SERVING on
						// this key proves the control plane's Temporal
						// accepted the worker, which is the moment deploys
						// can actually land.
						ReadinessProbe: &corev1.Probe{
							ProbeHandler: corev1.ProbeHandler{
								GRPC: &corev1.GRPCAction{
									Port:    runnerGrpcPort,
									Service: new(RunnerWorkerHealthService),
								},
							},
							InitialDelaySeconds: 5,
							PeriodSeconds:       10,
							TimeoutSeconds:      5,
							FailureThreshold:    3,
						},
						// Liveness deliberately probes only the overall
						// health server ("" service), not the worker key: a
						// Temporal outage must read as "not ready", never as
						// a kill-and-restart loop of a healthy process.
						LivenessProbe: &corev1.Probe{
							ProbeHandler: corev1.ProbeHandler{
								GRPC: &corev1.GRPCAction{Port: runnerGrpcPort},
							},
							InitialDelaySeconds: 15,
							PeriodSeconds:       30,
							TimeoutSeconds:      5,
							FailureThreshold:    3,
						},
					}},
					Volumes: []corev1.Volume{
						{
							// The projected badge: audience-bound to the
							// control plane's verifier, short-lived, rotated
							// by the kubelet. This volume is the ONLY
							// identity material the pod carries.
							Name: "badge-token",
							VolumeSource: corev1.VolumeSource{
								Projected: &corev1.ProjectedVolumeSource{
									Sources: []corev1.VolumeProjection{{
										ServiceAccountToken: &corev1.ServiceAccountTokenProjection{
											Audience:          RunnerBadgeAudience,
											ExpirationSeconds: new(runnerBadgeTokenExpirationSeconds),
											Path:              runnerBadgeTokenFile,
										},
									}},
								},
							},
						},
						{
							Name: "iac-state",
							VolumeSource: corev1.VolumeSource{
								PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
									ClaimName: RunnerStatePVCName(cfg.CRName),
								},
							},
						},
						{
							Name:         "iac-cache",
							VolumeSource: corev1.VolumeSource{EmptyDir: &corev1.EmptyDirVolumeSource{}},
						},
					},
				},
			},
		},
	}

	if cfg.OwnerRef != nil {
		deploy.OwnerReferences = []metav1.OwnerReference{*cfg.OwnerRef}
	}

	return deploy
}

func runnerLabels(crName string) map[string]string {
	return map[string]string{
		"app.kubernetes.io/name":       "runner",
		"app.kubernetes.io/instance":   crName,
		"app.kubernetes.io/managed-by": ManagedByLabel,
		"app.kubernetes.io/component":  "application",
	}
}
