package component

import (
	"context"
	"fmt"
	"strings"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	v1 "github.com/plantonhq/planton/operator/api/v1"
	"github.com/plantonhq/planton/operator/internal/resources"
)

const SSAFieldManager = "planton-operator"

// PrerequisiteSkip is the spec.prerequisites.* value promising a sub-operator
// is already installed by other means, so the operator must not deploy it.
const PrerequisiteSkip = "skip"

// Component represents a single named infrastructure component managed by the
// operator. Each component declares its upstream dependencies, handles its own
// prerequisites (sub-operator deployment, credential creation), and reports
// readiness independently. The controller reconciles components whose
// dependencies are satisfied, allowing independent progress.
type Component interface {
	// Name returns a stable identifier used for logging, status lookup, and
	// dependency references. Must match the keys used in ComponentStatuses.
	Name() string

	// Dependencies returns the Names of components that must be Ready before
	// this component's Reconcile is called. An empty slice means no
	// dependencies (the component is always eligible for reconciliation).
	// It receives the CR (like IsEnabled) because a dependency can follow the
	// spec: e.g. the control plane waits for the identity server only when
	// ingress -- and therefore sign-in -- is enabled.
	Dependencies(planton *v1.PlantonPlatform) []string

	// IsEnabled reports whether this component should be deployed given the
	// current CRD spec. Disabled components are skipped entirely and do not
	// appear in status.components.
	IsEnabled(planton *v1.PlantonPlatform) bool

	// Reconcile drives the component toward its desired state. It creates or
	// updates Kubernetes resources, checks health, and returns a Result
	// indicating readiness. Implementations must be idempotent.
	Reconcile(ctx context.Context, c client.Client, scheme *runtime.Scheme, planton *v1.PlantonPlatform) (Result, error)
}

// Result describes the outcome of a single component reconciliation.
type Result struct {
	Ready   bool
	Message string
}

// All returns every component the operator knows about, in a stable order.
// The order does not imply execution order -- the controller uses dependency
// declarations to determine which components are eligible for reconciliation.
// One ordering IS load-bearing: Ingress precedes Console so a public URL
// resolved in a pass (written to status in memory) reaches the console's
// Deployment env in that same pass.
func All() []Component {
	return []Component{
		&PostgreSQL{},
		&Redis{},
		&OpenBAO{},
		&Neo4j{},
		&OpenFGA{},
		&Temporal{},
		&Tekton{},
		// The front door (Ingress or Gateway -- exactly one is enabled)
		// precedes Identity in the same pass ordering the Console relies on:
		// the resolved front-door URL feeds Keycloak's hostname and realm
		// config the pass it appears.
		&Ingress{},
		&Gateway{},
		&Identity{},
		&ControlPlane{},
		&Runner{},
		&Console{},
	}
}

// StatusFor returns the ComponentStatus pointer for a given component name,
// or nil if the name is unknown or the status slot was not initialized.
func StatusFor(components *v1.ComponentStatuses, name string) *v1.ComponentStatus {
	switch name {
	case "postgresql":
		return components.PostgreSQL
	case "redis":
		return components.Redis
	case "openbao":
		return components.OpenBAO
	case "neo4j":
		return components.Neo4j
	case "openfga":
		return components.OpenFGA
	case "temporal":
		return components.Temporal
	case "tekton":
		return components.Tekton
	case "controlplane":
		return components.ControlPlane
	case "runner":
		return components.Runner
	case "ingress":
		return components.Ingress
	case "gateway":
		return components.Gateway
	case "identity":
		return components.Identity
	case "console":
		return components.Console
	default:
		return nil
	}
}

// DependenciesReady returns true if every named dependency has a Ready status
// in the provided ComponentStatuses. Returns false and the name of the first
// unready dependency otherwise.
func DependenciesReady(components *v1.ComponentStatuses, deps []string) (bool, string) {
	for _, dep := range deps {
		cs := StatusFor(components, dep)
		if cs == nil || cs.Phase != v1.ComponentPhaseReady {
			return false, dep
		}
	}
	return true, ""
}

// Base provides shared reconciliation helpers that are embedded by every
// concrete component. This avoids duplicating SSA application, readiness
// checks, credential management, and owner reference construction across
// component implementations.
type Base struct{}

// ApplyManifests applies the platform's own objects -- rendered Helm chart
// output and hand-built unstructured resources alike -- with Server-Side
// Apply and force ownership, and makes every one of them the platform's:
// each object's ownerReferences carry the PlantonPlatform, so deleting the
// resource garbage-collects everything applied here. The operator has no
// finalizers; owner-reference GC IS its destroy contract, and this is the one
// seam that upholds it for chart-rendered objects (the typed builders set the
// reference themselves). Shared sub-operators the platform must never own go
// through ApplyOperatorManifests instead.
//
// Ownership has two preconditions Kubernetes enforces by deleting, not by
// refusing: a namespaced owner cannot own a cluster-scoped object, and a
// dependent whose owner lives in another namespace is treated as orphaned and
// collected. Both are refused here in words before anything is applied.
func (b *Base) ApplyManifests(ctx context.Context, c client.Client, planton *v1.PlantonPlatform, objs []*unstructured.Unstructured) error {
	log := logf.FromContext(ctx)

	if err := ownedByPlatform(planton, b.OwnerReferenceFor(planton), objs); err != nil {
		return err
	}

	for _, obj := range objs {
		opts := []client.PatchOption{
			client.ForceOwnership,
			client.FieldOwner(SSAFieldManager),
		}

		err := c.Patch(ctx, obj, client.Apply, opts...)
		if err != nil && isImmutableJobChange(obj, err) {
			// A Job's pod template is immutable: a chart upgrade that changes
			// a migration Job (a new image, new labels) cannot be patched onto
			// the completed run, only replaced -- which is exactly what Helm
			// does for a hook with the before-hook-creation policy. Delete the
			// old run (background propagation: its pods are done) and apply
			// the new one, so the new migration runs once. Patching the same
			// content is a no-op and never reaches here, so a Job that has not
			// changed is never re-run.
			log.Info("Replacing Job whose template changed",
				"name", obj.GetName(), "namespace", obj.GetNamespace())
			if derr := c.Delete(ctx, obj, client.PropagationPolicy(metav1.DeletePropagationBackground)); derr != nil && !apierrors.IsNotFound(derr) {
				return fmt.Errorf("replacing Job %s/%s whose template changed: deleting the completed run: %w",
					obj.GetNamespace(), obj.GetName(), derr)
			}
			err = c.Patch(ctx, obj, client.Apply, opts...)
		}
		if err != nil {
			return fmt.Errorf("applying %s %s/%s: %w",
				obj.GetKind(), obj.GetNamespace(), obj.GetName(), err)
		}
		log.V(1).Info("Applied manifest",
			"kind", obj.GetKind(),
			"name", obj.GetName(),
			"namespace", obj.GetNamespace(),
		)
	}
	return nil
}

// isImmutableJobChange reports whether a server-side apply was refused because
// a batch/v1 Job's immutable pod template differs from the rendered one -- the
// one refusal ApplyManifests answers by replacing the object. Any other
// invalid-object error stays an error: deleting a Job over a mistake in the
// manifest would hide the mistake.
func isImmutableJobChange(obj *unstructured.Unstructured, err error) bool {
	if obj.GetKind() != "Job" || obj.GroupVersionKind().Group != "batch" {
		return false
	}
	return apierrors.IsInvalid(err) && strings.Contains(err.Error(), "immutable")
}

// IsStatefulSetReady checks if a StatefulSet has at least one ready replica.
func (b *Base) IsStatefulSetReady(ctx context.Context, c client.Client, name, namespace string) (bool, error) {
	sts := &unstructured.Unstructured{}
	sts.SetGroupVersionKind(schema.GroupVersionKind{
		Group: "apps", Version: "v1", Kind: "StatefulSet",
	})

	if err := c.Get(ctx, types.NamespacedName{Name: name, Namespace: namespace}, sts); err != nil {
		if apierrors.IsNotFound(err) {
			return false, nil
		}
		return false, err
	}

	readyReplicas, found, err := unstructured.NestedInt64(sts.Object, "status", "readyReplicas")
	if err != nil || !found {
		return false, nil
	}
	return readyReplicas > 0, nil
}

// IsDeploymentReady reports whether a Deployment's rollout is complete: the
// controller has observed the current spec, every desired replica runs the
// current pod template, and every one of them is available.
//
// Availability alone is not readiness. While a Deployment rolls to a new
// image (a platform version change), the previous pod stays available until
// the new one is, so "at least one available replica" is true throughout
// the rollout and would let the platform report Ready at the new version
// while the old release is still the one serving. The rollout-complete
// definition is the one `kubectl rollout status` waits for.
func (b *Base) IsDeploymentReady(ctx context.Context, c client.Client, name, namespace string) (bool, error) {
	deploy := &unstructured.Unstructured{}
	deploy.SetGroupVersionKind(schema.GroupVersionKind{
		Group: "apps", Version: "v1", Kind: "Deployment",
	})

	if err := c.Get(ctx, types.NamespacedName{Name: name, Namespace: namespace}, deploy); err != nil {
		if apierrors.IsNotFound(err) {
			return false, nil
		}
		return false, err
	}
	return deploymentRolloutComplete(deploy), nil
}

// deploymentRolloutComplete is IsDeploymentReady's verdict on one Deployment
// object: the spec has been observed, every desired replica is on the current
// template, no replica from an older template remains (status.replicas counts
// old and new together, so it must equal the updated count), and every
// replica is available. spec.replicas defaults to 1 when unset, as the API
// server does.
func deploymentRolloutComplete(deploy *unstructured.Unstructured) bool {
	desired := int64(1)
	if replicas, found, err := unstructured.NestedInt64(deploy.Object, "spec", "replicas"); err == nil && found {
		desired = replicas
	}
	generation := deploy.GetGeneration()
	observed, _, _ := unstructured.NestedInt64(deploy.Object, "status", "observedGeneration")
	total, _, _ := unstructured.NestedInt64(deploy.Object, "status", "replicas")
	updated, _, _ := unstructured.NestedInt64(deploy.Object, "status", "updatedReplicas")
	available, _, _ := unstructured.NestedInt64(deploy.Object, "status", "availableReplicas")
	return desired > 0 && observed >= generation && updated == desired && total == desired && available == desired
}

// IsPodRunning checks if at least one Pod matching the given StatefulSet name
// is in Running phase, regardless of readiness. This is used for components
// like OpenBAO where the readiness probe fails until the operator performs
// initialization, but the pod must be running for the init API to respond.
func (b *Base) IsPodRunning(ctx context.Context, c client.Client, stsName, namespace string) (bool, error) {
	var pods corev1.PodList
	if err := c.List(ctx, &pods,
		client.InNamespace(namespace),
		client.MatchingLabels{"app.kubernetes.io/name": stsName},
	); err != nil {
		return false, err
	}

	for i := range pods.Items {
		if pods.Items[i].Status.Phase == corev1.PodRunning {
			return true, nil
		}
	}

	podName := fmt.Sprintf("%s-0", stsName)
	var pod corev1.Pod
	if err := c.Get(ctx, types.NamespacedName{Name: podName, Namespace: namespace}, &pod); err != nil {
		if apierrors.IsNotFound(err) {
			return false, nil
		}
		return false, err
	}
	return pod.Status.Phase == corev1.PodRunning, nil
}

// EnsureCredentialSecret creates a credential Secret with a generated password
// if it does not already exist. Existing secrets are left untouched.
func (b *Base) EnsureCredentialSecret(ctx context.Context, c client.Client, name, namespace, dataKey string, ownerRef *metav1.OwnerReference) error {
	log := logf.FromContext(ctx)

	var existing corev1.Secret
	err := c.Get(ctx, types.NamespacedName{Name: name, Namespace: namespace}, &existing)
	if err == nil {
		return nil
	}
	if !apierrors.IsNotFound(err) {
		return fmt.Errorf("getting secret %s: %w", name, err)
	}

	password, err := resources.GeneratePassword()
	if err != nil {
		return fmt.Errorf("generating password for %s: %w", name, err)
	}

	secret := resources.NewCredentialSecret(name, namespace, dataKey, password, ownerRef)
	if err := c.Create(ctx, secret); err != nil {
		return fmt.Errorf("creating secret %s: %w", name, err)
	}

	log.Info("Created credential secret", "name", name)
	return nil
}

// EnsureAndReadCredential creates a credential Secret if it does not exist,
// then returns the value of the specified data key. This is needed for
// components like Identity where the literal credential must appear in the
// rendered configuration (the chart does not support existingSecret there).
func (b *Base) EnsureAndReadCredential(ctx context.Context, c client.Client, name, namespace string, dataKeys map[string]string, ownerRef *metav1.OwnerReference) (map[string]string, error) {
	log := logf.FromContext(ctx)

	var existing corev1.Secret
	err := c.Get(ctx, types.NamespacedName{Name: name, Namespace: namespace}, &existing)
	if err == nil {
		result := make(map[string]string, len(dataKeys))
		for key := range dataKeys {
			result[key] = string(existing.Data[key])
		}
		return result, nil
	}
	if !apierrors.IsNotFound(err) {
		return nil, fmt.Errorf("getting secret %s: %w", name, err)
	}

	password, err := resources.GeneratePassword()
	if err != nil {
		return nil, fmt.Errorf("generating password for %s: %w", name, err)
	}

	secretData := make(map[string][]byte, len(dataKeys))
	result := make(map[string]string, len(dataKeys))
	for key, defaultVal := range dataKeys {
		val := defaultVal
		if val == "" {
			val = password
		}
		secretData[key] = []byte(val)
		result[key] = val
	}

	secret := &corev1.Secret{
		TypeMeta: metav1.TypeMeta{APIVersion: "v1", Kind: "Secret"},
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
			Labels: map[string]string{
				"app.kubernetes.io/managed-by": resources.ManagedByLabel,
			},
		},
		Type: corev1.SecretTypeOpaque,
		Data: secretData,
	}
	if ownerRef != nil {
		secret.OwnerReferences = []metav1.OwnerReference{*ownerRef}
	}

	if err := c.Create(ctx, secret); err != nil {
		return nil, fmt.Errorf("creating secret %s: %w", name, err)
	}

	log.Info("Created credential secret", "name", name)
	return result, nil
}

// EnsureSecretAnnotations SSA-applies ONLY the given annotations onto an
// existing Secret: the apply configuration carries no data, so field
// ownership keeps the credential content untouched (create-once Secrets stay
// create-once). Existing installs pick the annotations up on their next
// reconcile. Callers must ensure the Secret exists first -- applying against
// a missing Secret would create a data-less shell.
func (b *Base) EnsureSecretAnnotations(ctx context.Context, c client.Client, name, namespace string, annotations map[string]string) error {
	secret := &corev1.Secret{
		TypeMeta: metav1.TypeMeta{APIVersion: "v1", Kind: "Secret"},
		ObjectMeta: metav1.ObjectMeta{
			Name:        name,
			Namespace:   namespace,
			Annotations: annotations,
		},
	}
	if err := b.ApplyTypedObject(ctx, c, secret); err != nil {
		return fmt.Errorf("annotating secret %s: %w", name, err)
	}
	return nil
}

// OwnerReferenceFor builds a metav1.OwnerReference for the given CR, suitable
// for attaching to resources that should be garbage collected when the
// PlantonPlatform resource is deleted.
func (b *Base) OwnerReferenceFor(planton *v1.PlantonPlatform) *metav1.OwnerReference {
	apiVersion := planton.APIVersion
	kind := planton.Kind
	if apiVersion == "" {
		apiVersion = "planton.ai/v1"
	}
	if kind == "" {
		kind = "PlantonPlatform"
	}
	return &metav1.OwnerReference{
		APIVersion: apiVersion,
		Kind:       kind,
		Name:       planton.Name,
		UID:        planton.UID,
	}
}

// IsCRDInstalled checks if a CRD with the given name exists in the cluster.
func (b *Base) IsCRDInstalled(ctx context.Context, c client.Client, crdName string) (bool, error) {
	crd, err := b.getCRD(ctx, c, crdName)
	if err != nil {
		return false, err
	}
	return crd != nil, nil
}

// getCRD fetches a CRD by name, returning nil (not an error) when it does not
// exist so callers can branch on presence and still inspect the object.
func (b *Base) getCRD(ctx context.Context, c client.Client, crdName string) (*unstructured.Unstructured, error) {
	crd := &unstructured.Unstructured{}
	crd.SetGroupVersionKind(schema.GroupVersionKind{
		Group:   "apiextensions.k8s.io",
		Version: "v1",
		Kind:    "CustomResourceDefinition",
	})

	if err := c.Get(ctx, types.NamespacedName{Name: crdName}, crd); err != nil {
		if apierrors.IsNotFound(err) || meta.IsNoMatchError(err) {
			return nil, nil
		}
		return nil, err
	}
	return crd, nil
}

// ManifestLoaderFunc is the signature of functions that load embedded operator
// manifests (e.g., LoadCloudNativePGManifests).
type ManifestLoaderFunc func() ([]*unstructured.Unstructured, error)

// SubOperatorOptions describes one vendored sub-operator install for
// EnsureSubOperator: the CRD whose presence detects an install, the embedded
// manifest loader that performs it, and the controller Deployments that prove
// the install completed and is serving.
type SubOperatorOptions struct {
	// LogName identifies the sub-operator in logs.
	LogName string

	// SkipRequested carries the arm's spec.prerequisites.* == "skip" switch:
	// the adopter promises an install this operator does not manage.
	SkipRequested bool

	// CRDName is the CustomResourceDefinition whose presence means "installed".
	CRDName string

	// Loader loads the vendored release manifests.
	Loader ManifestLoaderFunc

	// Namespace is the default namespace for manifest objects that carry none.
	Namespace string

	// Deployments are the controller Deployments (in Namespace) that prove the
	// install completed AND is serving. All must be available for ready=true.
	Deployments []string
}

// EnsureSubOperator is the detect-or-install idiom shared by every vendored
// sub-operator (CloudNativePG, Tekton Pipelines). It
// returns true only when the sub-operator is usable:
//
//   - skip requested: trust the adopter's promise.
//   - CRD absent: apply the vendored release; not yet usable.
//   - CRD present and applied by THIS operator: usable once every named
//     controller Deployment is available. A MISSING Deployment means the
//     install died partway (applies land CRDs before Deployments), so the
//     release is RE-APPLIED to resume it -- SSA is idempotent. Without this
//     arm a partial apply strands the install forever: the CRD check says
//     "installed" while readiness waits on a controller that never deployed.
//   - CRD present but foreign (helm, GitOps, another operator instance's
//     manifests are indistinguishable only in ownership): its Deployment name
//     and namespace are unknown, so respect it exactly like an explicit skip.
//
// Ownership is read from the managed-by label ApplyOperatorManifests stamps
// on every vendored object (the same label EnsureNamespace and the install
// library's guard scan use -- and unlike managedFields it is visible to a
// human running kubectl). A COMPLETE install is deliberately never
// re-applied: some releases (Tekton) carry config content that later
// reconciles co-own under different field managers, and a gratuitous
// force-ownership re-apply would claw those fields back.
func (b *Base) EnsureSubOperator(ctx context.Context, c client.Client, opts SubOperatorOptions) (bool, error) {
	log := logf.FromContext(ctx).WithValues("subOperator", opts.LogName)

	if opts.SkipRequested {
		log.Info("Sub-operator deployment skipped by user")
		return true, nil
	}

	crd, err := b.getCRD(ctx, c, opts.CRDName)
	if err != nil {
		return false, fmt.Errorf("checking for %s CRD: %w", opts.CRDName, err)
	}

	if crd == nil {
		log.Info("Sub-operator CRD not found, deploying")
		if err := b.ApplyOperatorManifests(ctx, c, opts.Loader, opts.Namespace); err != nil {
			return false, fmt.Errorf("applying %s manifests: %w", opts.LogName, err)
		}
		return false, nil
	}

	if crd.GetLabels()[ManagedByLabel] != SSAFieldManager {
		log.V(1).Info("Sub-operator pre-installed by another owner, respecting it")
		return true, nil
	}

	allReady := true
	for _, name := range opts.Deployments {
		exists, ready, err := b.deploymentState(ctx, c, name, opts.Namespace)
		if err != nil {
			return false, fmt.Errorf("checking %s readiness: %w", name, err)
		}
		if !exists {
			log.Info("Sub-operator install incomplete, resuming apply", "missingDeployment", name)
			if err := b.ApplyOperatorManifests(ctx, c, opts.Loader, opts.Namespace); err != nil {
				return false, fmt.Errorf("resuming %s manifests: %w", opts.LogName, err)
			}
			return false, nil
		}
		if !ready {
			log.Info("Sub-operator not yet ready", "deployment", name)
			allReady = false
		}
	}
	return allReady, nil
}

// deploymentState reports whether a Deployment exists and whether it has at
// least one available replica -- distinct answers, because a MISSING
// controller Deployment under our own CRD means a partial install to resume,
// while an existing-but-unready one just needs time.
func (b *Base) deploymentState(ctx context.Context, c client.Client, name, namespace string) (exists, ready bool, err error) {
	deploy := &unstructured.Unstructured{}
	deploy.SetGroupVersionKind(schema.GroupVersionKind{
		Group: "apps", Version: "v1", Kind: "Deployment",
	})

	if err := c.Get(ctx, types.NamespacedName{Name: name, Namespace: namespace}, deploy); err != nil {
		if apierrors.IsNotFound(err) {
			return false, false, nil
		}
		return false, false, err
	}

	availableReplicas, found, err := unstructured.NestedInt64(deploy.Object, "status", "availableReplicas")
	if err != nil || !found {
		return true, false, nil
	}
	return true, availableReplicas > 0, nil
}

// ApplyOperatorManifests loads manifests via the provided loader and applies
// them using SSA. Namespaced resources without an explicit namespace are placed
// in defaultNamespace. The target namespace is created if it does not exist.
func (b *Base) ApplyOperatorManifests(ctx context.Context, c client.Client, loader ManifestLoaderFunc, defaultNamespace string) error {
	// Serialized with RemoveSubOperator: a platform arriving while the
	// janitor removes the previous install's definitions must apply after
	// the teardown has finished, never between its deletes.
	subOperatorMu.Lock()
	defer subOperatorMu.Unlock()

	log := logf.FromContext(ctx)

	objs, err := loader()
	if err != nil {
		return fmt.Errorf("loading embedded manifests: %w", err)
	}

	for _, obj := range objs {
		if obj.GetNamespace() == "" && isNamespaced(obj) {
			obj.SetNamespace(defaultNamespace)
		}

		// The managed-by label marks every vendored object as installed by
		// this operator -- EnsureSubOperator reads it off the detect CRD to
		// tell our (possibly partial) install apart from a foreign one it
		// must respect.
		labels := obj.GetLabels()
		if labels == nil {
			labels = map[string]string{}
		}
		labels[ManagedByLabel] = SSAFieldManager
		obj.SetLabels(labels)

		if err := b.EnsureNamespace(ctx, c, obj.GetNamespace()); err != nil {
			return err
		}

		opts := []client.PatchOption{
			client.ForceOwnership,
			client.FieldOwner(SSAFieldManager),
		}

		if err := c.Patch(ctx, obj, client.Apply, opts...); err != nil {
			return fmt.Errorf("applying %s %s/%s: %w",
				obj.GetKind(), obj.GetNamespace(), obj.GetName(), err)
		}
		log.Info("Applied manifest",
			"kind", obj.GetKind(),
			"name", obj.GetName(),
			"namespace", obj.GetNamespace(),
		)
	}

	return nil
}

// EnsureNamespace creates the target namespace if it does not already exist.
func (b *Base) EnsureNamespace(ctx context.Context, c client.Client, namespace string) error {
	if namespace == "" {
		return nil
	}

	ns := &unstructured.Unstructured{}
	ns.SetGroupVersionKind(schema.GroupVersionKind{Version: "v1", Kind: "Namespace"})

	if err := c.Get(ctx, types.NamespacedName{Name: namespace}, ns); err == nil {
		return nil
	} else if !apierrors.IsNotFound(err) {
		return err
	}

	newNS := &unstructured.Unstructured{}
	newNS.SetGroupVersionKind(schema.GroupVersionKind{Version: "v1", Kind: "Namespace"})
	newNS.SetName(namespace)
	newNS.SetLabels(map[string]string{
		ManagedByLabel: SSAFieldManager,
	})

	return c.Create(ctx, newNS)
}

// ApplyTypedObject applies a typed Kubernetes object using Server-Side Apply
// with force ownership. Used by components that build typed Go objects
// (Deployments, StatefulSets, Services) rather than rendering Helm charts.
func (b *Base) ApplyTypedObject(ctx context.Context, c client.Client, obj client.Object) error {
	opts := []client.PatchOption{
		client.ForceOwnership,
		client.FieldOwner(SSAFieldManager),
	}
	return c.Patch(ctx, obj, client.Apply, opts...)
}

func isNamespaced(obj *unstructured.Unstructured) bool {
	return resources.IsNamespacedKind(obj.GetKind())
}

// ownedByPlatform stamps the platform's owner reference on every object,
// merging with any reference the builder already set (matched by UID, so a
// controller reference on a CloudNativePG Cluster is kept as-is), and refuses
// the two shapes a namespaced owner cannot collect. It is pure so the
// contract is unit-tested; the apply path is proven by envtest and the lab.
func ownedByPlatform(planton *v1.PlantonPlatform, ownerRef *metav1.OwnerReference, objs []*unstructured.Unstructured) error {
	for _, obj := range objs {
		if !resources.IsNamespacedKind(obj.GetKind()) {
			return fmt.Errorf("%s %q is cluster-scoped, and a PlantonPlatform in namespace %q cannot own it: "+
				"a cluster-scoped object is never garbage-collected with the platform, so it must not be applied as the platform's own -- "+
				"render it into the namespace or manage it as shared cluster infrastructure",
				obj.GetKind(), obj.GetName(), planton.Namespace)
		}
		if obj.GetNamespace() != planton.Namespace {
			return fmt.Errorf("%s %s/%s lives outside the platform's namespace %q, and Kubernetes treats an owner in another namespace as absent: "+
				"the object would be garbage-collected immediately -- render it into the platform's namespace",
				obj.GetKind(), obj.GetNamespace(), obj.GetName(), planton.Namespace)
		}
		refs := obj.GetOwnerReferences()
		owned := false
		for _, existing := range refs {
			if existing.UID == ownerRef.UID {
				owned = true
				break
			}
		}
		if !owned {
			obj.SetOwnerReferences(append(refs, *ownerRef))
		}
	}
	return nil
}
