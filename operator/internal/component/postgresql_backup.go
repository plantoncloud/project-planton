package component

import (
	"context"
	"fmt"
	"time"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	v1 "github.com/plantonhq/planton/operator/api/v1"
	"github.com/plantonhq/planton/operator/internal/resources"
)

// The platform's database backs up through the Barman Cloud plugin, a shared
// sub-operator beside CloudNativePG (resources/barman_plugin_helm.go). This
// file is the PostgreSQL component's backup arm: deciding whether the plugin
// is wanted and installable, installing it through the same gate every shared
// sub-operator uses, preflighting the adopter's credentials, rendering the
// store and the schedule around the Cluster, and reading the database
// operator's own conditions into status.backup. The Cluster itself is built
// and applied by the component's Reconcile; this file hands it what to
// render and when.
//
// Two orderings are load-bearing and both are for the person's sake. A
// Cluster never carries a plugins entry before the plugin serves -- a Cluster
// naming a plugin the operator cannot reach is parked with no instances at
// all, and the whole platform would stall on its database. And a platform
// whose backup fails stays Ready: it is doing its job; its safety net is not,
// and the Backup column says so where a person reads first.

const (
	// cnpgContinuousArchivingConditionType is CloudNativePG's condition for
	// WAL archiving; False is the quiet failure this arm exists to surface.
	cnpgContinuousArchivingConditionType = "ContinuousArchiving"

	// certManagerIssuerCRD is the second cert-manager kind the plugin chart
	// renders (beside certificates.cert-manager.io, which the ingress
	// preflight already names).
	certManagerIssuerCRD = "issuers.cert-manager.io"

	// backupFieldPath is how status messages name the declaration.
	backupFieldPath = "spec.database.postgresql.backup"
	// recoverFromFieldPath is how status messages name the recovery declaration.
	recoverFromFieldPath = "spec.database.postgresql.recoverFrom"
)

// BarmanCloudPluginSubOperator is the one definition of the Barman Cloud
// plugin install this operator manages -- what detects it, what installs it,
// what proves it serving -- read by the install gate and by the janitor, the
// same way CloudNativePGSubOperator is.
func BarmanCloudPluginSubOperator() SubOperatorOptions {
	return SubOperatorOptions{
		LogName:     "barman-cloud-plugin",
		CRDName:     resources.BarmanCloudObjectStoreCRDName,
		Loader:      resources.LoadBarmanCloudPluginManifests,
		Namespace:   resources.CloudNativePGNamespace,
		Deployments: []string{resources.BarmanCloudPluginDeploymentName},
	}
}

// postgresqlBackupSpec returns the platform's backup declaration, or nil.
func postgresqlBackupSpec(planton *v1.PlantonPlatform) *v1.PostgreSQLBackupSpec {
	if planton.Spec.Database == nil || planton.Spec.Database.PostgreSQL == nil {
		return nil
	}
	return planton.Spec.Database.PostgreSQL.Backup
}

// postgresqlRecoverFromSpec returns the platform's recovery declaration, or nil.
func postgresqlRecoverFromSpec(planton *v1.PlantonPlatform) *v1.PostgreSQLRecoverFromSpec {
	if planton.Spec.Database == nil || planton.Spec.Database.PostgreSQL == nil {
		return nil
	}
	return planton.Spec.Database.PostgreSQL.RecoverFrom
}

// backupPluginSkipped is the adopter's one override: the plugin is theirs (or
// unwanted), never the operator's to install.
func backupPluginSkipped(planton *v1.PlantonPlatform) bool {
	return planton.Spec.Prerequisites != nil &&
		planton.Spec.Prerequisites.PostgresBackupPlugin == PrerequisiteSkip
}

// backupPlan is what the backup arm decided for this reconcile: the objects to
// apply before and after the Cluster, what the Cluster itself must carry, and
// the provisional backup status. The Cluster's final backup status is read
// after it is applied (refreshBackupStatus).
type backupPlan struct {
	// holdCluster asks Reconcile not to create the Cluster yet: a fresh
	// install with a backup declared waits for the plugin so the database is
	// born archiving instead of restarting to attach it. Never set once a
	// Cluster exists.
	holdCluster bool
	// waiting is the component's not-ready sentence while holdCluster is set.
	waiting string

	// before are applied ahead of the Cluster (the stores the Cluster
	// references and their settings Secrets); after follows it (the
	// schedule, which needs the Cluster to exist).
	before []*unstructured.Unstructured
	after  []*unstructured.Unstructured

	backup                    *resources.PostgreSQLClusterBackup
	recovery                  *resources.PostgreSQLClusterRecovery
	serviceAccountAnnotations map[string]string

	// status is the provisional backup status; refreshBackupStatus replaces
	// it with what the database operator reports once the Cluster carries the
	// plugin.
	status v1.BackupStatus
}

// pluginDecision is the backup arm's answer to "does this cluster get the
// plugin, and if not, why not?" -- decided from reads alone so the answer is
// pinned offline, then acted on by planBackup.
type pluginDecision int

const (
	// pluginNotWanted: nothing installs and nothing is missing -- no backup
	// declared, and either the adopter said skip, cert-manager is absent, or
	// the CloudNativePG on the cluster is someone else's.
	pluginNotWanted pluginDecision = iota
	// pluginUnavailableNoCertManager: a backup is declared but the plugin's
	// prerequisite is absent.
	pluginUnavailableNoCertManager
	// pluginSkippedButMissing: the adopter said skip, declared a backup, and
	// no plugin definition is on the cluster.
	pluginSkippedButMissing
	// pluginTheirs: the adopter said skip and a plugin is present; use it,
	// never install or remove it.
	pluginTheirs
	// pluginOurs: the operator installs (or already installed) the plugin.
	pluginOurs
)

// decideBackupPlugin reads the cluster's facts and decides. "auto" means: the
// operator installs the plugin whenever it installed CloudNativePG itself and
// cert-manager is present, or a backup is declared. With the plugin present,
// every PostgreSQL an adopter deploys through Planton can back itself up
// without anyone finding a toggle; a cluster that cannot run the plugin pays
// nothing for it.
func (p *PostgreSQL) decideBackupPlugin(ctx context.Context, c client.Client, planton *v1.PlantonPlatform) (pluginDecision, error) {
	declared := postgresqlBackupSpec(planton) != nil

	if backupPluginSkipped(planton) {
		if !declared {
			return pluginNotWanted, nil
		}
		installed, err := p.IsCRDInstalled(ctx, c, resources.BarmanCloudObjectStoreCRDName)
		if err != nil {
			return pluginNotWanted, fmt.Errorf("checking for the backup plugin: %w", err)
		}
		if !installed {
			return pluginSkippedButMissing, nil
		}
		return pluginTheirs, nil
	}

	certManager, err := p.certManagerServesThePlugin(ctx, c)
	if err != nil {
		return pluginNotWanted, err
	}
	if !certManager {
		if declared {
			return pluginUnavailableNoCertManager, nil
		}
		return pluginNotWanted, nil
	}
	if declared {
		return pluginOurs, nil
	}
	ours, err := p.cloudNativePGIsOurs(ctx, c)
	if err != nil {
		return pluginNotWanted, err
	}
	if ours {
		return pluginOurs, nil
	}
	return pluginNotWanted, nil
}

// planBackup runs the backup arm's decisions for one reconcile. existing is
// the live Cluster, or nil when none exists yet.
func (p *PostgreSQL) planBackup(ctx context.Context, c client.Client, planton *v1.PlantonPlatform, existing *unstructured.Unstructured) (backupPlan, error) {
	spec := postgresqlBackupSpec(planton)
	plan := backupPlan{status: v1.BackupStatus{State: v1.BackupStateNotConfigured}}
	if spec != nil {
		plan.status.ServerName = resources.PostgreSQLBackupServerName(planton.Name, planton.UID)
	}

	decision, err := p.decideBackupPlugin(ctx, c, planton)
	if err != nil {
		return plan, err
	}
	switch decision {
	case pluginNotWanted:
		return plan, nil
	case pluginUnavailableNoCertManager:
		plan.status.State = v1.BackupStateUnavailable
		plan.status.Message = "cert-manager is not installed (Certificate definition missing); the backup plugin needs it to mint the TLS pair between the database operator and the plugin. Install cert-manager and the operator installs the plugin on its next pass -- until then the platform runs, and nothing is being saved"
		return plan, nil
	case pluginSkippedButMissing:
		plan.status.State = v1.BackupStateUnavailable
		plan.status.Message = fmt.Sprintf(
			"spec.prerequisites.postgresBackupPlugin is %q but no Barman Cloud plugin is installed on the cluster (definition %s missing); install the plugin, or set the prerequisite to auto so the operator installs it -- until then nothing is being saved",
			PrerequisiteSkip, resources.BarmanCloudObjectStoreCRDName)
		return plan, nil
	case pluginTheirs:
		return p.planDeclaredBackup(ctx, c, planton, spec, existing, plan)
	}

	pluginReady, err := p.EnsureSubOperator(ctx, c, BarmanCloudPluginSubOperator())
	if err != nil {
		// A plugin that cannot be installed must never take the database
		// with it: the Cluster is applied without it, and the Backup column
		// carries the reason. The gate retries on the next pass.
		if spec != nil {
			plan.status.State = v1.BackupStateUnavailable
			plan.status.Message = fmt.Sprintf("the backup plugin could not be installed: %v; the database runs without backups until it can", err)
		}
		return plan, nil
	}
	if spec == nil {
		return plan, nil
	}
	if !pluginReady {
		plan.status.State = v1.BackupStateDeploying
		explained := p.SubOperatorNotReady(ctx, c, planton.Namespace, BarmanCloudPluginSubOperator(), "Deploying the backup plugin")
		if existing == nil {
			plan.holdCluster = true
			plan.waiting = "Deploying the backup plugin before the database, so it is born archiving"
			if explained.Reason != v1.ComponentReasonDeploying {
				plan.waiting = explained.Message
			}
			plan.status.Message = plan.waiting
			return plan, nil
		}
		plan.status.Message = explained.Message + "; once it serves, enabling backups restarts the database once to attach the archiving sidecar"
		return plan, nil
	}

	return p.planDeclaredBackup(ctx, c, planton, spec, existing, plan)
}

// planDeclaredBackup renders a declared backup once the plugin serves: the
// credentials preflight, the store and its settings, the Cluster's plugin
// wiring, the schedule, and -- on a Cluster not yet created -- the recovery
// source.
func (p *PostgreSQL) planDeclaredBackup(ctx context.Context, c client.Client, planton *v1.PlantonPlatform, spec *v1.PostgreSQLBackupSpec, existing *unstructured.Unstructured, plan backupPlan) (backupPlan, error) {
	owner := p.OwnerReferenceFor(planton)
	storeName := resources.PostgreSQLBackupObjectStoreName(planton.Name)
	store := objectStoreOptions(spec.ObjectStore, storeName, planton.Namespace, owner)
	store.RetentionPolicy = spec.RetentionPolicy
	if store.RetentionPolicy == "" {
		store.RetentionPolicy = resources.PostgreSQLBackupDefaultRetention
	}

	if msg, err := p.preflightObjectStoreSecret(ctx, c, planton.Namespace, store, backupFieldPath); err != nil {
		return plan, err
	} else if msg != "" {
		plan.status.State = v1.BackupStateFailing
		plan.status.Message = msg
		return plan, nil
	}

	plan.before = append(plan.before, resources.NewObjectStore(store))
	if settings := resources.NewObjectStoreSettingsSecret(store); settings != nil {
		plan.before = append(plan.before, settings)
	}
	plan.backup = &resources.PostgreSQLClusterBackup{
		ObjectStoreName: storeName,
		ServerName:      plan.status.ServerName,
	}
	plan.serviceAccountAnnotations = spec.ServiceAccountAnnotations
	plan.after = append(plan.after, resources.NewScheduledBackup(resources.ScheduledBackupOptions{
		CRName:          planton.Name,
		Namespace:       planton.Namespace,
		Schedule:        spec.Schedule,
		ObjectStoreName: storeName,
		OwnerRef:        owner,
	}))

	plan.status.State = v1.BackupStateDeploying
	switch {
	case existing == nil:
		plan.status.Message = "Creating the database with archiving on; waiting for the first base backup"
	case !clusterCarriesPlugin(existing):
		plan.status.Message = "Enabling backups restarts the database once to attach the archiving sidecar; then waiting for the first base backup"
	default:
		plan.status.Message = "Waiting for the first base backup"
	}

	if rec := postgresqlRecoverFromSpec(planton); rec != nil {
		if existing != nil {
			// Honored at creation only; the message says how to actually
			// restore. The live bootstrap is kept by keepLiveBootstrap.
			if !clusterBootstrappedFromRecovery(existing) {
				plan.status.Message += fmt.Sprintf(
					". %s is honored only when the database is first created and this database already exists: to restore, delete the platform, delete its database volume claim, and declare it again with %s",
					recoverFromFieldPath, recoverFromFieldPath)
			}
			return plan, nil
		}
		recoveryStoreName := resources.PostgreSQLRecoveryObjectStoreName(planton.Name)
		recoveryStore := objectStoreOptions(rec.ObjectStore, recoveryStoreName, planton.Namespace, owner)
		if msg, err := p.preflightObjectStoreSecret(ctx, c, planton.Namespace, recoveryStore, recoverFromFieldPath); err != nil {
			return plan, err
		} else if msg != "" {
			// A recovery that cannot read its source must not create an empty
			// database in its place: hold the Cluster and say why.
			plan.holdCluster = true
			plan.waiting = msg
			plan.status.State = v1.BackupStateFailing
			plan.status.Message = msg
			return plan, nil
		}
		plan.before = append(plan.before, resources.NewObjectStore(recoveryStore))
		if settings := resources.NewObjectStoreSettingsSecret(recoveryStore); settings != nil {
			plan.before = append(plan.before, settings)
		}
		plan.recovery = &resources.PostgreSQLClusterRecovery{
			ObjectStoreName: recoveryStoreName,
			ServerName:      rec.ServerName,
			TargetTime:      rec.TargetTime,
		}
		plan.status.Message = fmt.Sprintf("Restoring the database from server %q; then waiting for the first base backup of the restored platform", rec.ServerName)
	}
	return plan, nil
}

// objectStoreOptions maps the declaration onto the render options.
func objectStoreOptions(spec v1.ObjectStoreSpec, name, namespace string, owner *metav1.OwnerReference) resources.ObjectStoreOptions {
	opts := resources.ObjectStoreOptions{
		Name:            name,
		Namespace:       namespace,
		DestinationPath: spec.DestinationPath,
		OwnerRef:        owner,
	}
	switch {
	case spec.S3 != nil:
		opts.S3 = &resources.S3ObjectStoreOptions{
			EndpointURL:           spec.S3.EndpointURL,
			Region:                spec.S3.Region,
			CredentialsSecretName: spec.S3.CredentialsSecretName,
		}
		if spec.S3.EndpointCASecretRef != nil {
			opts.S3.EndpointCASecretName = spec.S3.EndpointCASecretRef.Name
			opts.S3.EndpointCASecretKey = spec.S3.EndpointCASecretRef.Key
		}
	case spec.GCS != nil:
		opts.GCS = &resources.GCSObjectStoreOptions{CredentialsSecretName: spec.GCS.CredentialsSecretName}
	case spec.AzureBlob != nil:
		opts.AzureBlob = &resources.AzureBlobObjectStoreOptions{
			StorageAccount:        spec.AzureBlob.StorageAccount,
			CredentialsSecretName: spec.AzureBlob.CredentialsSecretName,
		}
	case spec.R2 != nil:
		opts.R2 = &resources.R2ObjectStoreOptions{
			AccountID:             spec.R2.AccountID,
			Jurisdiction:          spec.R2.Jurisdiction,
			CredentialsSecretName: spec.R2.CredentialsSecretName,
		}
	}
	return opts
}

// preflightObjectStoreSecret checks the adopter-owned credentials Secret a
// store references exists and carries the keys its backend needs. A missing
// Secret or key is a sentence, not an error: the store is not rendered and
// the Backup column says exactly what to add. Keyless stores need nothing.
func (p *PostgreSQL) preflightObjectStoreSecret(ctx context.Context, c client.Client, namespace string, store resources.ObjectStoreOptions, field string) (string, error) {
	name := store.CredentialsSecretName()
	if name == "" {
		return "", nil
	}
	keys := store.RequiredCredentialKeys()
	var secret corev1.Secret
	err := c.Get(ctx, types.NamespacedName{Name: name, Namespace: namespace}, &secret)
	if apierrors.IsNotFound(err) {
		return fmt.Sprintf(
			"credentials Secret %q named by %s not found in namespace %q; create it with keys %v, or leave credentialsSecretName empty to use the database's own cloud identity -- until then nothing is being saved",
			name, field, namespace, keys), nil
	}
	if err != nil {
		return "", fmt.Errorf("checking backup credentials Secret %s: %w", name, err)
	}
	for _, key := range keys {
		if _, ok := secret.Data[key]; !ok {
			return fmt.Sprintf(
				"credentials Secret %q named by %s has no %q key; add it (the store needs %v) -- until then nothing is being saved",
				name, field, key, keys), nil
		}
	}
	return "", nil
}

// certManagerServesThePlugin reports whether cert-manager is installed with
// both kinds the plugin chart renders -- an Issuer and Certificates. The
// ingress preflight checks the Certificate definition alone because that is
// all it renders; the plugin needs both, so a cert-manager missing either is
// a cert-manager that cannot serve it.
func (p *PostgreSQL) certManagerServesThePlugin(ctx context.Context, c client.Client) (bool, error) {
	for _, crd := range []string{certManagerCertificateCRD, certManagerIssuerCRD} {
		installed, err := p.IsCRDInstalled(ctx, c, crd)
		if err != nil {
			return false, fmt.Errorf("checking for cert-manager: %w", err)
		}
		if !installed {
			return false, nil
		}
	}
	return true, nil
}

// cloudNativePGIsOurs reports whether this operator installed the
// CloudNativePG on the cluster (its detect definition carries our mark). A
// foreign CloudNativePG is someone else's to extend with a plugin.
func (p *PostgreSQL) cloudNativePGIsOurs(ctx context.Context, c client.Client) (bool, error) {
	crd, err := p.getCRD(ctx, c, cnpgClusterCRDName)
	if err != nil {
		return false, fmt.Errorf("checking who installed CloudNativePG: %w", err)
	}
	return crd != nil && crd.GetLabels()[ManagedByLabel] == SSAFieldManager, nil
}

// clusterCarriesPlugin reports whether a live Cluster already names the
// backup plugin -- the difference between "attaching, one restart" and
// "already archiving".
func clusterCarriesPlugin(cluster *unstructured.Unstructured) bool {
	plugins, _, _ := unstructured.NestedSlice(cluster.Object, "spec", "plugins")
	for _, p := range plugins {
		if m, ok := p.(map[string]any); ok && m["name"] == resources.BarmanCloudPluginName {
			return true
		}
	}
	return false
}

// clusterBootstrappedFromRecovery reports whether a live Cluster was created
// from an archive.
func clusterBootstrappedFromRecovery(cluster *unstructured.Unstructured) bool {
	_, found, _ := unstructured.NestedMap(cluster.Object, "spec", "bootstrap", "recovery")
	return found
}

// keepLiveBootstrap copies a live Cluster's bootstrap and externalClusters
// onto the desired object. How a cluster's data came to exist is decided
// once, by CloudNativePG, at creation; re-rendering a different answer every
// thirty seconds would be a fight the operator cannot win and must not try.
func keepLiveBootstrap(desired, existing *unstructured.Unstructured) {
	if existing == nil {
		return
	}
	if bootstrap, found, _ := unstructured.NestedMap(existing.Object, "spec", "bootstrap"); found {
		_ = unstructured.SetNestedMap(desired.Object, bootstrap, "spec", "bootstrap")
	}
	if ext, found, _ := unstructured.NestedSlice(existing.Object, "spec", "externalClusters"); found {
		_ = unstructured.SetNestedSlice(desired.Object, ext, "spec", "externalClusters")
	} else {
		unstructured.RemoveNestedField(desired.Object, "spec", "externalClusters")
	}
}

// refreshBackupStatus reads what the database operator reports about a
// Cluster that carries the plugin -- the archiving condition, the last
// successful and failed base backups, the first recoverability point -- into
// status.backup. It never consults status.phase. When the Cluster does not
// carry the plugin yet, the provisional status stands.
func (p *PostgreSQL) refreshBackupStatus(ctx context.Context, c client.Client, planton *v1.PlantonPlatform, cluster *unstructured.Unstructured, provisional v1.BackupStatus) v1.BackupStatus {
	status := provisional
	if cluster == nil || !clusterCarriesPlugin(cluster) {
		return status
	}
	status.FirstRecoverabilityPoint = timeField(cluster, "status", "firstRecoverabilityPoint")
	status.LastSuccessfulBackup = timeField(cluster, "status", "lastSuccessfulBackup")
	status.LastFailedBackup = timeField(cluster, "status", "lastFailedBackup")

	archiving, archivingMessage := conditionStatus(cluster, cnpgContinuousArchivingConditionType)
	switch {
	case archiving == metav1.ConditionFalse:
		status.State = v1.BackupStateFailing
		status.Message = "WAL archiving is failing: " + archivingMessage
	case status.LastFailedBackup != nil &&
		(status.LastSuccessfulBackup == nil || status.LastFailedBackup.After(status.LastSuccessfulBackup.Time)):
		status.State = v1.BackupStateFailing
		status.Message = "the last base backup failed"
		if reason := p.lastBackupError(ctx, c, planton.Namespace, cluster.GetName()); reason != "" {
			status.Message += ": " + reason
		}
	case status.LastSuccessfulBackup == nil:
		status.State = v1.BackupStateDeploying
		if archiving == metav1.ConditionTrue {
			status.Message = "WAL archiving is continuous; waiting for the first base backup"
		} else {
			status.Message = "Waiting for WAL archiving to start and the first base backup"
		}
	default:
		status.State = v1.BackupStateHealthy
		status.Message = fmt.Sprintf("WAL archiving is continuous; last base backup %s", status.LastSuccessfulBackup.UTC().Format(time.RFC3339))
	}
	return status
}

// lastBackupError reads the failed Backup object's own error for the
// cluster, so a failed base backup is reported in the plugin's words.
func (p *PostgreSQL) lastBackupError(ctx context.Context, c client.Client, namespace, clusterName string) string {
	list := &unstructured.UnstructuredList{}
	list.SetGroupVersionKind(resources.BackupGVK.GroupVersion().WithKind(resources.BackupGVK.Kind + "List"))
	if err := c.List(ctx, list, client.InNamespace(namespace)); err != nil {
		return ""
	}
	var latest *unstructured.Unstructured
	var latestTime time.Time
	for i := range list.Items {
		item := &list.Items[i]
		if name, _, _ := unstructured.NestedString(item.Object, "spec", "cluster", "name"); name != clusterName {
			continue
		}
		if phase, _, _ := unstructured.NestedString(item.Object, "status", "phase"); phase != "failed" {
			continue
		}
		stopped := timeField(item, "status", "stoppedAt")
		at := item.GetCreationTimestamp().Time
		if stopped != nil {
			at = stopped.Time
		}
		if latest == nil || at.After(latestTime) {
			latest, latestTime = item, at
		}
	}
	if latest == nil {
		return ""
	}
	reason, _, _ := unstructured.NestedString(latest.Object, "status", "error")
	return reason
}

// conditionStatus reads one condition off an unstructured object's status.
func conditionStatus(obj *unstructured.Unstructured, condType string) (metav1.ConditionStatus, string) {
	conditions, _, _ := unstructured.NestedSlice(obj.Object, "status", "conditions")
	for _, raw := range conditions {
		cond, ok := raw.(map[string]any)
		if !ok || cond["type"] != condType {
			continue
		}
		status, _ := cond["status"].(string)
		message, _ := cond["message"].(string)
		return metav1.ConditionStatus(status), message
	}
	return "", ""
}

// timeField parses an RFC 3339 timestamp field off an unstructured object,
// or returns nil when absent or unparseable.
func timeField(obj *unstructured.Unstructured, fields ...string) *metav1.Time {
	raw, found, _ := unstructured.NestedString(obj.Object, fields...)
	if !found || raw == "" {
		return nil
	}
	t, err := time.Parse(time.RFC3339, raw)
	if err != nil {
		return nil
	}
	return &metav1.Time{Time: t}
}
