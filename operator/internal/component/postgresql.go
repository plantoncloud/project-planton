package component

import (
	"context"
	"fmt"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
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
	cnpgOperatorNamespace  = resources.CloudNativePGNamespace
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
		return p.SubOperatorNotReady(ctx, c, planton.Namespace, subOperator, "Deploying CloudNativePG operator"), nil
	}

	storageSize, storageClass := postgresqlStorage(planton)
	instances := defaultPostgresqlInstances
	if planton.Spec.Database != nil && planton.Spec.Database.PostgreSQL != nil &&
		planton.Spec.Database.PostgreSQL.Replicas != nil {
		instances = *planton.Spec.Database.PostgreSQL.Replicas
	}

	clusterName := resources.PostgreSQLClusterName(planton.Name)
	existing, err := p.liveCluster(ctx, c, clusterName, planton.Namespace)
	if err != nil {
		return Result{}, err
	}

	// The backup arm decides before the Cluster is built: whether the plugin
	// is wanted and serving, what the Cluster must carry, and what to apply
	// around it. Its provisional status is written now so a held or
	// plugin-less pass still explains itself; the final state is read off
	// the Cluster below.
	plan, err := p.planBackup(ctx, c, planton, existing)
	if err != nil {
		return Result{}, err
	}
	planton.Status.Backup = &plan.status
	if plan.holdCluster {
		return Result{Ready: false, Reason: v1.ComponentReasonDeploying, Message: plan.waiting}, nil
	}
	if len(plan.before) > 0 {
		if err := p.ApplyManifests(ctx, c, planton, plan.before); err != nil {
			return Result{}, fmt.Errorf("applying PostgreSQL backup store: %w", err)
		}
	}

	cluster := resources.NewPostgreSQLCluster(resources.PostgreSQLClusterOptions{
		CRName:                    planton.Name,
		Namespace:                 planton.Namespace,
		Instances:                 instances,
		StorageSize:               storageSize,
		StorageClassName:          storageClass,
		OwnerRef:                  p.OwnerReferenceFor(planton),
		Backup:                    plan.backup,
		Recovery:                  plan.recovery,
		ServiceAccountAnnotations: plan.serviceAccountAnnotations,
	})
	// How the data came to exist was decided once, at creation; a live
	// Cluster's bootstrap is kept as it is (recovery declared later is
	// explained in status.backup, never re-rendered).
	keepLiveBootstrap(cluster, existing)
	if scheme != nil {
		if err := ctrlutil.SetControllerReference(planton, cluster, scheme); err != nil {
			log.Error(err, "Could not set owner reference, falling back to manual ownerRef",
				"cluster", cluster.GetName())
		}
	}

	// SSA-applied every reconcile (not create-once) so spec edits stay live:
	// raising replicas grows the HA topology, growing storage.size expands
	// the volumes, declaring a backup attaches the plugin. CloudNativePG
	// never rewrites its Cluster spec (defaults land at admission and re-land
	// identically on every apply), so repeated applies converge without
	// ownership churn. Its webhook REJECTS invalid mutations -- a storage
	// shrink, a malformed quantity -- and that rejection surfaces here as
	// the component's own error message instead of a silent no-op.
	if err := p.ApplyManifests(ctx, c, planton, []*unstructured.Unstructured{cluster}); err != nil {
		return Result{}, fmt.Errorf("applying PostgreSQL cluster: %w", err)
	}
	if len(plan.after) > 0 {
		if err := p.ApplyManifests(ctx, c, planton, plan.after); err != nil {
			return Result{}, fmt.Errorf("applying PostgreSQL backup schedule: %w", err)
		}
	}

	live, err := p.liveCluster(ctx, c, clusterName, planton.Namespace)
	if err != nil {
		return Result{}, err
	}
	backupStatus := p.refreshBackupStatus(ctx, c, planton, live, plan.status)
	planton.Status.Backup = &backupStatus

	ready, statusMsg := clusterReadiness(live, instances)
	if !ready {
		// The Cluster's own readiness sentence is the generic answer; the
		// instance pods (labelled cnpg.io/cluster) supply anything sharper.
		return p.NotReady(ctx, c, planton.Namespace, PostgresClusterRef(clusterName), statusMsg), nil
	}

	credentialsReady, err := p.superuserSecretExists(ctx, c, planton)
	if err != nil {
		return Result{}, err
	}
	if !credentialsReady {
		return Result{Ready: false, Message: "Waiting for PostgreSQL credentials"}, nil
	}

	// A failing backup never takes a working database out of Ready; the
	// component's sentence carries it beside the health sentence, and the
	// Backup column and BackupHealthy condition carry it on their own.
	if backupStatus.State == v1.BackupStateFailing || backupStatus.State == v1.BackupStateUnavailable {
		statusMsg = fmt.Sprintf("%s; backups %s: %s", statusMsg, backupStatus.State, backupStatus.Message)
	}

	log.Info("PostgreSQL ready")
	return Result{Ready: true, Message: statusMsg}, nil
}

// liveCluster reads the platform's CloudNativePG Cluster as it is on the
// cluster, or nil when it does not exist yet.
func (p *PostgreSQL) liveCluster(ctx context.Context, c client.Client, name, namespace string) (*unstructured.Unstructured, error) {
	existing := &unstructured.Unstructured{}
	existing.SetGroupVersionKind(resources.PostgreSQLClusterGVK)
	if err := c.Get(ctx, types.NamespacedName{Name: name, Namespace: namespace}, existing); err != nil {
		if apierrors.IsNotFound(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("getting PostgreSQL cluster %s: %w", name, err)
	}
	return existing, nil
}

// clusterReadiness reads the CloudNativePG Cluster's own readiness signals:
// the Ready condition (flips once bootstrap completed and the topology
// matches the spec) plus status.readyInstances against the declared instance
// count. status.phase is never the verdict -- upstream treats phase-watching
// as deprecated; conditions are the scripted contract -- but when every
// instance is up and Ready is still False, the phase's own reason is the
// only sentence that says why, so it is relayed rather than hidden behind a
// "waiting for instances" that is not true.
func clusterReadiness(existing *unstructured.Unstructured, instances int32) (bool, string) {
	if existing == nil {
		return false, "Waiting for PostgreSQL cluster"
	}

	readyInstances, _, _ := unstructured.NestedInt64(existing.Object, "status", "readyInstances")
	readyStatus, _ := conditionStatus(existing, cnpgReadyConditionType)
	conditionReady := readyStatus == metav1.ConditionTrue

	if readyInstances < int64(instances) {
		return false, fmt.Sprintf("Waiting for PostgreSQL cluster (%d/%d instances ready)",
			readyInstances, instances)
	}
	if !conditionReady {
		phase, _, _ := unstructured.NestedString(existing.Object, "status", "phase")
		reason, _, _ := unstructured.NestedString(existing.Object, "status", "phaseReason")
		msg := fmt.Sprintf("PostgreSQL instances are up (%d/%d) but the cluster is not Ready", readyInstances, instances)
		switch {
		case reason != "":
			msg += ": " + reason
		case phase != "":
			msg += ": " + phase
		}
		return false, msg
	}
	return true, fmt.Sprintf("PostgreSQL healthy (%d/%d instances ready)", readyInstances, instances)
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

// RBAC markers for PostgreSQL resources. The backup arm applies the platform's
// ObjectStore and ScheduledBackup as platform-owned objects and reads the
// Backup objects the schedule creates for the state it reports.
// +kubebuilder:rbac:groups=postgresql.cnpg.io,resources=clusters,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=postgresql.cnpg.io,resources=clusters/status,verbs=get
// +kubebuilder:rbac:groups=postgresql.cnpg.io,resources=scheduledbackups,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=postgresql.cnpg.io,resources=backups,verbs=get;list;watch
// +kubebuilder:rbac:groups=barmancloud.cnpg.io,resources=objectstores,verbs=get;list;watch;create;update;patch;delete

// Sub-operator deployment RBAC.
// +kubebuilder:rbac:groups=apiextensions.k8s.io,resources=customresourcedefinitions,verbs=get;list;watch;create;update;patch
// +kubebuilder:rbac:groups="",resources=namespaces,verbs=get;list;watch;create
// +kubebuilder:rbac:groups=rbac.authorization.k8s.io,resources=clusterroles;clusterrolebindings,verbs=get;list;watch;create;update;patch
// +kubebuilder:rbac:groups=scheduling.k8s.io,resources=priorityclasses,verbs=get;list;watch;create;update;patch
