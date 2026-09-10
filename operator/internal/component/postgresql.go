package component

import (
	"context"
	"fmt"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	ctrlutil "sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	v1 "github.com/plantonhq/planton/operator/api/v1"
	"github.com/plantonhq/planton/operator/internal/resources"
)

const (
	defaultPostgresqlStorageSize = "10Gi"
	defaultPostgresqlInstances   = int32(1)

	cnpgClusterCRDName     = "clusters.postgresql.cnpg.io"
	cnpgOperatorNamespace  = "cnpg-system"
	cnpgDeploymentName     = "cnpg-controller-manager"
	cnpgReadyConditionType = "Ready"
)

// PostgreSQL deploys the platform's database as a CloudNativePG Cluster,
// handling the CloudNativePG operator itself as an internal prerequisite
// (detect-or-install; the stock install watches all namespaces, so several
// PlantonPlatform installs on one Kubernetes cluster share the one
// CloudNativePG). One Cluster serves the whole platform; spec.database.
// postgresql.replicas turns the same cluster into a streaming-replication
// HA setup with automated failover.
type PostgreSQL struct{ Base }

func (p *PostgreSQL) Name() string                                { return "postgresql" }
func (p *PostgreSQL) Dependencies(_ *v1.PlantonPlatform) []string { return nil }
func (p *PostgreSQL) IsEnabled(_ *v1.PlantonPlatform) bool        { return true }

// CloudNativePGSubOperator is the one definition of the CloudNativePG install
// this operator manages -- what detects it, what installs it, what proves it
// serving -- read by the install gate on every reconcile and by the janitor
// when the last platform leaves. One definition, so the two can never
// disagree about what "the operator's CloudNativePG" is.
func CloudNativePGSubOperator() SubOperatorOptions {
	return SubOperatorOptions{
		LogName:     "cloudnative-pg",
		CRDName:     cnpgClusterCRDName,
		Loader:      resources.LoadCloudNativePGManifests,
		Namespace:   cnpgOperatorNamespace,
		Deployments: []string{cnpgDeploymentName},
	}
}

func (p *PostgreSQL) Reconcile(ctx context.Context, c client.Client, scheme *runtime.Scheme, planton *v1.PlantonPlatform) (Result, error) {
	log := logf.FromContext(ctx).WithValues("component", p.Name())

	subOperator := CloudNativePGSubOperator()
	subOperator.SkipRequested = planton.Spec.Prerequisites != nil &&
		planton.Spec.Prerequisites.PostgresOperator == PrerequisiteSkip
	operatorReady, err := p.EnsureSubOperator(ctx, c, subOperator)
	if err != nil {
		return Result{}, err
	}
	if !operatorReady {
		return Result{Ready: false, Message: "Deploying CloudNativePG operator"}, nil
	}

	storageSize, storageClass := postgresqlStorage(planton)
	instances := defaultPostgresqlInstances
	if planton.Spec.Database != nil && planton.Spec.Database.PostgreSQL != nil &&
		planton.Spec.Database.PostgreSQL.Replicas != nil {
		instances = *planton.Spec.Database.PostgreSQL.Replicas
	}

	cluster := resources.NewPostgreSQLCluster(resources.PostgreSQLClusterOptions{
		CRName:           planton.Name,
		Namespace:        planton.Namespace,
		Instances:        instances,
		StorageSize:      storageSize,
		StorageClassName: storageClass,
		OwnerRef:         p.OwnerReferenceFor(planton),
	})
	if scheme != nil {
		if err := ctrlutil.SetControllerReference(planton, cluster, scheme); err != nil {
			log.Error(err, "Could not set owner reference, falling back to manual ownerRef",
				"cluster", cluster.GetName())
		}
	}

	// SSA-applied every reconcile (not create-once) so spec edits stay live:
	// raising replicas grows the HA topology, growing storage.size expands
	// the volumes. CloudNativePG never rewrites its Cluster spec (defaults
	// land at admission and re-land identically on every apply), so repeated
	// applies converge without ownership churn. Its webhook REJECTS invalid
	// mutations -- a storage shrink, a malformed quantity -- and that
	// rejection surfaces here as the component's own error message instead
	// of a silent no-op.
	if err := p.ApplyManifests(ctx, c, planton, []*unstructured.Unstructured{cluster}); err != nil {
		return Result{}, fmt.Errorf("applying PostgreSQL cluster: %w", err)
	}

	ready, statusMsg, err := p.clusterReady(ctx, c, cluster.GetName(), planton.Namespace, instances)
	if err != nil {
		return Result{}, err
	}
	if !ready {
		// CloudNativePG instance PVCs are named "{cluster}-N".
		if msg, ok := p.ExplainPendingStorage(ctx, c, planton.Namespace, cluster.GetName()); ok {
			return Result{Ready: false, Message: msg}, nil
		}
		return Result{Ready: false, Message: statusMsg}, nil
	}

	credentialsReady, err := p.superuserSecretExists(ctx, c, planton)
	if err != nil {
		return Result{}, err
	}
	if !credentialsReady {
		return Result{Ready: false, Message: "Waiting for PostgreSQL credentials"}, nil
	}

	log.Info("PostgreSQL ready")
	return Result{Ready: true, Message: statusMsg}, nil
}

// clusterReady reads the CloudNativePG Cluster's own readiness signals: the
// Ready condition (flips once bootstrap completed and the topology matches
// the spec) plus status.readyInstances against the declared instance count.
// status.phase is deliberately not consulted -- upstream treats phase-watching
// as deprecated; conditions are the scripted contract.
func (p *PostgreSQL) clusterReady(ctx context.Context, c client.Client, name, namespace string, instances int32) (bool, string, error) {
	existing := &unstructured.Unstructured{}
	existing.SetGroupVersionKind(resources.PostgreSQLClusterGVK)
	if err := c.Get(ctx, types.NamespacedName{Name: name, Namespace: namespace}, existing); err != nil {
		if apierrors.IsNotFound(err) {
			return false, "Waiting for PostgreSQL cluster", nil
		}
		return false, "", fmt.Errorf("getting PostgreSQL cluster %s: %w", name, err)
	}

	readyInstances, _, _ := unstructured.NestedInt64(existing.Object, "status", "readyInstances")

	conditions, _, _ := unstructured.NestedSlice(existing.Object, "status", "conditions")
	conditionReady := false
	for _, item := range conditions {
		cond, ok := item.(map[string]any)
		if !ok {
			continue
		}
		if cond["type"] == cnpgReadyConditionType && cond["status"] == "True" {
			conditionReady = true
			break
		}
	}

	if !conditionReady || readyInstances < int64(instances) {
		return false, fmt.Sprintf("Waiting for PostgreSQL cluster (%d/%d instances ready)",
			readyInstances, instances), nil
	}
	return true, fmt.Sprintf("PostgreSQL healthy (%d/%d instances ready)", readyInstances, instances), nil
}

// superuserSecretExists gates readiness on the CloudNativePG-generated
// superuser credential Secret every consumer's env references -- a Ready
// database whose credential has not materialized yet would boot consumers
// into CreateContainerConfigError.
func (p *PostgreSQL) superuserSecretExists(ctx context.Context, c client.Client, planton *v1.PlantonPlatform) (bool, error) {
	secretName := resources.PostgreSQLSuperuserSecretName(planton.Name)
	secret := &unstructured.Unstructured{}
	secret.SetGroupVersionKind(secretGVK())

	err := c.Get(ctx, types.NamespacedName{
		Name:      secretName,
		Namespace: planton.Namespace,
	}, secret)
	if apierrors.IsNotFound(err) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("checking credential secret %s: %w", secretName, err)
	}
	return true, nil
}

func postgresqlStorage(planton *v1.PlantonPlatform) (size, class string) {
	var componentSize resource.Quantity
	var componentClass string
	if planton.Spec.Database != nil && planton.Spec.Database.PostgreSQL != nil {
		componentSize = planton.Spec.Database.PostgreSQL.StorageSize
		componentClass = planton.Spec.Database.PostgreSQL.StorageClassName
	}
	return effectiveStorageSize(planton, componentSize, defaultPostgresqlStorageSize),
		effectiveStorageClass(planton, componentClass)
}

// RBAC markers for PostgreSQL resources.
// +kubebuilder:rbac:groups=postgresql.cnpg.io,resources=clusters,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=postgresql.cnpg.io,resources=clusters/status,verbs=get

// Sub-operator deployment RBAC.
// +kubebuilder:rbac:groups=apiextensions.k8s.io,resources=customresourcedefinitions,verbs=get;list;watch;create;update;patch
// +kubebuilder:rbac:groups="",resources=namespaces,verbs=get;list;watch;create
// +kubebuilder:rbac:groups=rbac.authorization.k8s.io,resources=clusterroles;clusterrolebindings,verbs=get;list;watch;create;update;patch
// +kubebuilder:rbac:groups=scheduling.k8s.io,resources=priorityclasses,verbs=get;list;watch;create;update;patch
