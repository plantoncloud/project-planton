package component

import (
	"context"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	v1 "github.com/plantonhq/planton/operator/api/v1"
	"github.com/plantonhq/planton/operator/internal/resources"
)

// The runner reads cluster nodes only to tell the adopter WHICH cloud-identity
// guidance applies to their cluster (see substrate.go) -- read-only, listed
// once per reconcile.
// +kubebuilder:rbac:groups="",resources=nodes,verbs=get;list;watch

// Runner deploys the in-cluster Planton runner: the worker pod that picks up
// IaC operations from Temporal and executes them (OpenTofu) against the
// adopter's cloud. Its registration (declaring its Kubernetes workload
// identity) and deploy defaults are seeded by the control plane at boot from
// env the controlplane component wires -- so a fresh install deploys real
// infrastructure with zero registration ceremony.
//
// The runner holds NO enrollment credential: its proof is the projected
// ServiceAccount badge on its pod, verified by the control plane with the
// cluster itself. The one operator-minted credential on this install is the
// CloudOps direct-dial token (a network perimeter for the control plane ->
// runner dial, not an identity), minted exactly once into its own Secret.
type Runner struct{ Base }

func (r *Runner) Name() string { return "runner" }

func (r *Runner) Dependencies(_ *v1.PlantonPlatform) []string {
	// controlplane (not just temporal): the runner's registration, credential
	// hash, and deploy defaults are boot seeds -- a runner started against an
	// unseeded control plane polls a queue nothing dispatches to and reports
	// an alarming not-ready. Waiting for controlplane makes first boot clean:
	// by the time the pod starts, its whole world exists.
	return []string{"temporal", "controlplane"}
}

func (r *Runner) IsEnabled(planton *v1.PlantonPlatform) bool {
	return isRunnerEnabled(planton)
}

func (r *Runner) Reconcile(ctx context.Context, c client.Client, _ *runtime.Scheme, planton *v1.PlantonPlatform) (Result, error) {
	log := logf.FromContext(ctx).WithValues("component", r.Name())
	ownerRef := r.OwnerReferenceFor(planton)
	cfg := runnerConfig(planton, ownerRef)

	// The CloudOps token normally exists already (the controlplane component
	// mints it before its own Deployment); minting here too keeps this
	// component correct in isolation.
	if _, err := EnsureCloudOpsToken(ctx, c, planton.Name, planton.Namespace, ownerRef); err != nil {
		return Result{}, fmt.Errorf("ensuring runner CloudOps token: %w", err)
	}

	if err := r.ApplyTypedObject(ctx, c, resources.RunnerServiceAccount(cfg)); err != nil {
		return Result{}, fmt.Errorf("applying runner ServiceAccount: %w", err)
	}

	// The build RBAC follows the capability in both directions: applied with
	// builds on, deleted on opt-out (a Role the runner no longer needs is a
	// standing grant nobody audits). The tekton component owns the grants in
	// Tekton's own namespace; these are the runner's build-namespace grants.
	if err := r.ensureBuildRBAC(ctx, c, cfg); err != nil {
		return Result{}, err
	}

	// The Service is the control plane's direct-dial door for live cloud
	// operations; the runner behind it requires the CloudOps auth token, so
	// exposing it is safe by construction.
	if err := r.ApplyTypedObject(ctx, c, resources.RunnerService(cfg)); err != nil {
		return Result{}, fmt.Errorf("applying runner Service: %w", err)
	}

	// PVC storage requests are immutable -- create once, leave alone after
	// (the same discipline as credential Secrets).
	if err := r.ensureStatePVC(ctx, c, cfg); err != nil {
		return Result{}, err
	}

	if err := r.ApplyTypedObject(ctx, c, resources.RunnerDeployment(cfg)); err != nil {
		return Result{}, fmt.Errorf("applying runner Deployment: %w", err)
	}

	ready, err := r.IsDeploymentReady(ctx, c, resources.RunnerDeploymentName(planton.Name), planton.Namespace)
	if err != nil {
		return Result{}, fmt.Errorf("checking runner readiness: %w", err)
	}
	if !ready {
		log.Info("Runner not ready")
		// Readiness is the worker-poll probe: "not ready" after boot means
		// the worker is not polling its Temporal queue yet.
		return r.NotReady(ctx, c, planton.Namespace, DeploymentRef(resources.RunnerDeploymentName(planton.Name)),
			"Waiting for the runner worker to start polling for deploys"), nil
	}

	log.Info("Runner ready")
	return Result{Ready: true, Message: runnerReadyMessage(ctx, c, planton)}, nil
}

// ensureBuildRBAC applies the build Role + RoleBinding when builds are
// enabled and removes them when not (missing objects are a no-op, so the
// disabled path costs two GETs).
func (r *Runner) ensureBuildRBAC(ctx context.Context, c client.Client, cfg resources.RunnerConfig) error {
	role := resources.RunnerBuildRole(cfg)
	binding := resources.RunnerBuildRoleBinding(cfg)

	if cfg.BuildEnabled {
		if err := r.ApplyTypedObject(ctx, c, role); err != nil {
			return fmt.Errorf("applying runner build Role: %w", err)
		}
		if err := r.ApplyTypedObject(ctx, c, binding); err != nil {
			return fmt.Errorf("applying runner build RoleBinding: %w", err)
		}
		return nil
	}

	if err := client.IgnoreNotFound(c.Delete(ctx, binding)); err != nil {
		return fmt.Errorf("removing runner build RoleBinding: %w", err)
	}
	if err := client.IgnoreNotFound(c.Delete(ctx, role)); err != nil {
		return fmt.Errorf("removing runner build Role: %w", err)
	}
	return nil
}

// ensureStatePVC creates the IaC state volume if absent.
func (r *Runner) ensureStatePVC(ctx context.Context, c client.Client, cfg resources.RunnerConfig) error {
	log := logf.FromContext(ctx)
	name := resources.RunnerStatePVCName(cfg.CRName)

	var existing corev1.PersistentVolumeClaim
	err := c.Get(ctx, types.NamespacedName{Name: name, Namespace: cfg.Namespace}, &existing)
	if err == nil {
		return nil
	}
	if !apierrors.IsNotFound(err) {
		return fmt.Errorf("getting runner state PVC: %w", err)
	}

	if err := c.Create(ctx, resources.RunnerStatePVC(cfg)); err != nil {
		return fmt.Errorf("creating runner state PVC: %w", err)
	}
	log.Info("Created runner state PVC", "name", name)
	return nil
}

// EnsureCloudOpsToken returns the CloudOps direct-dial token, minting it (and
// its Secret) on first call. Exported to the controlplane component, which
// must guarantee the Secret exists before its own Deployment references the
// key. An existing token is never re-minted: it has no server-side persisted
// state (both sides read it from this Secret), and rotating it is a
// deliberate act (delete the Secret), not a side effect of a reconcile.
func EnsureCloudOpsToken(ctx context.Context, c client.Client, crName, namespace string, ownerRef *metav1.OwnerReference) (string, error) {
	log := logf.FromContext(ctx)
	name := resources.RunnerCloudOpsSecretName(crName)

	var existing corev1.Secret
	err := c.Get(ctx, types.NamespacedName{Name: name, Namespace: namespace}, &existing)
	if err == nil {
		cloudOpsToken := string(existing.Data[resources.RunnerCloudOpsSecretKeyToken])
		if cloudOpsToken == "" {
			// A Secret without its one key is unusable half-state; heal it
			// in place (create-once semantics per credential -- both
			// components call this ensure, and each must be able to
			// reference the key immediately).
			if cloudOpsToken, err = resources.GenerateRunnerCloudOpsToken(); err != nil {
				return "", err
			}
			patch := client.MergeFrom(existing.DeepCopy())
			if existing.Data == nil {
				existing.Data = map[string][]byte{}
			}
			existing.Data[resources.RunnerCloudOpsSecretKeyToken] = []byte(cloudOpsToken)
			if err := c.Patch(ctx, &existing, patch); err != nil {
				return "", fmt.Errorf("adding CloudOps auth token to its Secret: %w", err)
			}
			log.Info("Minted runner CloudOps auth token into existing Secret", "name", name)
		}
		return cloudOpsToken, nil
	}
	if !apierrors.IsNotFound(err) {
		return "", fmt.Errorf("getting runner CloudOps token Secret: %w", err)
	}

	cloudOpsToken, err := resources.GenerateRunnerCloudOpsToken()
	if err != nil {
		return "", err
	}
	if err := c.Create(ctx, resources.RunnerCloudOpsSecret(crName, namespace, cloudOpsToken, ownerRef)); err != nil {
		return "", fmt.Errorf("creating runner CloudOps token Secret: %w", err)
	}
	log.Info("Created runner CloudOps token Secret", "name", name)
	return cloudOpsToken, nil
}

// runnerReadyMessage explains, in the component status, how the runner will
// authenticate to the adopter's cloud -- substrate-aware, because the right
// next step differs per cluster (see substrate.go). A runner with configured
// cloud identity gets a plain ready message.
func runnerReadyMessage(ctx context.Context, c client.Client, planton *v1.PlantonPlatform) string {
	spec := planton.Spec.Runner
	if spec != nil && (len(spec.ServiceAccountAnnotations) > 0 || spec.CloudCredentialsSecretName != "") {
		return "Runner ready to execute deploys"
	}

	var nodes corev1.NodeList
	sub := substrateUnknown
	if err := c.List(ctx, &nodes); err == nil {
		sub = detectSubstrate(nodes.Items)
	}

	base := "Runner ready. No cloud credentials configured yet -- deploys to a cloud provider need an identity: "
	switch sub {
	case substrateEKS:
		return base + "on EKS, set spec.runner.serviceAccountAnnotations[\"eks.amazonaws.com/role-arn\"] " +
			"to an IAM role (IRSA) -- no keys stored anywhere -- or reference a credentials Secret " +
			"via spec.runner.cloudCredentialsSecretName"
	case substrateAWS:
		return base + "on AWS (non-EKS), the node's EC2 instance profile is used automatically when it " +
			"has one; otherwise reference a credentials Secret via spec.runner.cloudCredentialsSecretName"
	case substrateGKE:
		return base + "on GKE, set spec.runner.serviceAccountAnnotations[\"iam.gke.io/gcp-service-account\"] " +
			"(Workload Identity), or reference a credentials Secret via spec.runner.cloudCredentialsSecretName"
	case substrateAKS:
		return base + "on AKS, set spec.runner.serviceAccountAnnotations[\"azure.workload.identity/client-id\"] " +
			"(Workload Identity), or reference a credentials Secret via spec.runner.cloudCredentialsSecretName"
	default:
		return base + "reference a Secret holding your cloud credentials via " +
			"spec.runner.cloudCredentialsSecretName (the platform itself stores nothing)"
	}
}

// runnerConfig resolves spec.runner (+ the bootstrap org) into the resource
// builders' config. Defaulting lives here for the same reason as
// effectiveBootstrap: CRD defaults only apply when the parent struct exists.
func runnerConfig(planton *v1.PlantonPlatform, ownerRef *metav1.OwnerReference) resources.RunnerConfig {
	cfg := resources.RunnerConfig{
		CRName:    planton.Name,
		Namespace: planton.Namespace,
		Version:   planton.Spec.Version,
		OwnerRef:  ownerRef,
		OrgSlug:   effectiveBootstrap(planton).OrgSlug,
		// Effective, not the raw toggle: builds are a capability OF the
		// runner, and inside this component the runner is enabled by
		// definition -- but the shared predicate keeps every reader honest.
		BuildEnabled: isBuildEffective(planton),
	}
	var componentSize resource.Quantity
	var componentClass string
	if spec := planton.Spec.Runner; spec != nil {
		if spec.Image != nil {
			cfg.ImageRepository = spec.Image.Repository
			cfg.ImageTag = spec.Image.Tag
		}
		componentSize = spec.StorageSize
		componentClass = spec.StorageClassName
		cfg.ServiceAccountAnnotations = spec.ServiceAccountAnnotations
		cfg.CloudCredentialsSecretName = spec.CloudCredentialsSecretName
	}
	cfg.StorageSize = resource.MustParse(
		effectiveStorageSize(planton, componentSize, resources.RunnerDefaultStorageSize))
	cfg.StorageClassName = effectiveStorageClass(planton, componentClass)
	return cfg
}

// isRunnerEnabled: deployed by default -- an install that cannot deploy
// infrastructure is a browsing UI. Opting out is the explicit act.
func isRunnerEnabled(p *v1.PlantonPlatform) bool {
	return p.Spec.Runner == nil || p.Spec.Runner.Enabled == nil || *p.Spec.Runner.Enabled
}
