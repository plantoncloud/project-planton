package component

import (
	"context"
	"maps"
	"strings"
	"testing"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	v1 "github.com/plantonhq/planton/operator/api/v1"
	"github.com/plantonhq/planton/operator/internal/resources"
)

// The backup arm's decisions -- install, refuse, hold, attach, explain -- are
// what these tests pin, on a fake cluster whose facts are the objects handed
// in. The gate's real apply path (rendering the plugin chart onto a cluster)
// is exercised by the lab lane; here the plugin is always already present or
// deliberately absent, so nothing has to be applied.

func backupScheme(t *testing.T) *runtime.Scheme {
	t.Helper()
	s := subOperatorScheme(t)
	for _, gvk := range []schema.GroupVersionKind{resources.PostgreSQLClusterGVK, resources.BackupGVK, resources.ObjectStoreGVK} {
		s.AddKnownTypeWithName(gvk, &unstructured.Unstructured{})
		s.AddKnownTypeWithName(gvk.GroupVersion().WithKind(gvk.Kind+"List"), &unstructured.UnstructuredList{})
	}
	return s
}

func crd(name, managedBy string) *unstructured.Unstructured {
	obj := &unstructured.Unstructured{}
	obj.SetGroupVersionKind(schema.GroupVersionKind{Group: "apiextensions.k8s.io", Version: "v1", Kind: "CustomResourceDefinition"})
	obj.SetName(name)
	if managedBy != "" {
		obj.SetLabels(map[string]string{ManagedByLabel: managedBy})
	}
	return obj
}

func pluginDeployment(available bool) *appsv1.Deployment {
	deploy := &appsv1.Deployment{ObjectMeta: metav1.ObjectMeta{
		Name: resources.BarmanCloudPluginDeploymentName, Namespace: resources.CloudNativePGNamespace,
	}}
	if available {
		deploy.Status.ObservedGeneration = deploy.Generation
		deploy.Status.Replicas, deploy.Status.UpdatedReplicas, deploy.Status.AvailableReplicas = 1, 1, 1
	}
	return deploy
}

// The cluster shapes the arm reasons about.
var (
	cnpgOurs      = crd(cnpgClusterCRDName, SSAFieldManager)
	cnpgForeign   = crd(cnpgClusterCRDName, "Helm")
	certManager   = crd(certManagerCertificateCRD, "")
	certIssuers   = crd(certManagerIssuerCRD, "")
	pluginOursCRD = crd(resources.BarmanCloudObjectStoreCRDName, SSAFieldManager)
)

func backupPlatform(declared bool, prerequisite string) *v1.PlantonPlatform {
	planton := &v1.PlantonPlatform{ObjectMeta: metav1.ObjectMeta{
		Name: "planton", Namespace: "planton", UID: types.UID("1a2b3c4d-0000-4000-8000-000000000000"),
	}}
	if prerequisite != "" {
		planton.Spec.Prerequisites = &v1.PrerequisitesSpec{PostgresBackupPlugin: prerequisite}
	}
	if declared {
		planton.Spec.Database = &v1.DatabaseSpec{PostgreSQL: &v1.PostgreSQLSpec{Backup: &v1.PostgreSQLBackupSpec{
			ObjectStore: v1.ObjectStoreSpec{
				DestinationPath: "s3://bucket/platform",
				S3:              &v1.S3ObjectStoreSpec{Region: "us-east-1", CredentialsSecretName: "backup-keys"},
			},
		}}}
	}
	return planton
}

func keysSecret(keys ...string) *corev1.Secret {
	data := map[string][]byte{}
	for _, k := range keys {
		data[k] = []byte("x")
	}
	return &corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: "backup-keys", Namespace: "planton"}, Data: data}
}

func fakeCluster(t *testing.T, objs ...client.Object) client.Client {
	t.Helper()
	return fake.NewClientBuilder().WithScheme(backupScheme(t)).WithObjects(objs...).Build()
}

func TestDecideBackupPlugin(t *testing.T) {
	cases := []struct {
		name     string
		planton  *v1.PlantonPlatform
		cluster  []client.Object
		decision pluginDecision
	}{
		{"auto, ours, cert-manager, no backup: installs for catalog databases", backupPlatform(false, ""), []client.Object{cnpgOurs, certManager, certIssuers}, pluginOurs},
		{"auto, ours, no cert-manager, no backup: nothing, nothing missing", backupPlatform(false, ""), []client.Object{cnpgOurs}, pluginNotWanted},
		{"auto, foreign CloudNativePG, cert-manager, no backup: theirs to extend", backupPlatform(false, ""), []client.Object{cnpgForeign, certManager, certIssuers}, pluginNotWanted},
		{"auto, declared, cert-manager: installs", backupPlatform(true, ""), []client.Object{cnpgForeign, certManager, certIssuers}, pluginOurs},
		{"auto, declared, no cert-manager: unavailable", backupPlatform(true, ""), []client.Object{cnpgOurs}, pluginUnavailableNoCertManager},
		{"auto, declared, cert-manager without the Issuer kind: unavailable", backupPlatform(true, ""), []client.Object{cnpgOurs, certManager}, pluginUnavailableNoCertManager},
		{"auto, ours, cert-manager without the Issuer kind, no backup: nothing", backupPlatform(false, ""), []client.Object{cnpgOurs, certManager}, pluginNotWanted},
		{"skip, no backup: nothing", backupPlatform(false, PrerequisiteSkip), []client.Object{cnpgOurs, certManager, certIssuers}, pluginNotWanted},
		{"skip, declared, plugin present: theirs", backupPlatform(true, PrerequisiteSkip), []client.Object{cnpgOurs, certManager, certIssuers, crd(resources.BarmanCloudObjectStoreCRDName, "Helm")}, pluginTheirs},
		{"skip, declared, no plugin: unavailable in words", backupPlatform(true, PrerequisiteSkip), []client.Object{cnpgOurs, certManager, certIssuers}, pluginSkippedButMissing},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			p := &PostgreSQL{}
			got, err := p.decideBackupPlugin(context.Background(), fakeCluster(t, tc.cluster...), tc.planton)
			if err != nil {
				t.Fatalf("decide: %v", err)
			}
			if got != tc.decision {
				t.Errorf("decision %v, want %v", got, tc.decision)
			}
		})
	}
}

func TestPlanBackup_NothingDeclared(t *testing.T) {
	p := &PostgreSQL{}
	plan, err := p.planBackup(context.Background(), fakeCluster(t, cnpgOurs, certManager, certIssuers, pluginOursCRD, pluginDeployment(true)), backupPlatform(false, ""), nil)
	if err != nil {
		t.Fatal(err)
	}
	if plan.status.State != v1.BackupStateNotConfigured || plan.backup != nil || len(plan.before)+len(plan.after) != 0 || plan.holdCluster {
		t.Errorf("no declaration must render nothing: %+v", plan)
	}
	if plan.status.ServerName != "" {
		t.Errorf("no server name without a declaration, got %q", plan.status.ServerName)
	}
}

func TestPlanBackup_NoCertManagerIsUnavailableAndTheClusterStillRenders(t *testing.T) {
	p := &PostgreSQL{}
	plan, err := p.planBackup(context.Background(), fakeCluster(t, cnpgOurs), backupPlatform(true, ""), nil)
	if err != nil {
		t.Fatal(err)
	}
	if plan.status.State != v1.BackupStateUnavailable || !strings.Contains(plan.status.Message, "cert-manager") {
		t.Errorf("want Unavailable naming cert-manager, got %+v", plan.status)
	}
	if plan.holdCluster || plan.backup != nil {
		t.Error("a missing prerequisite must never hold the database or wire a plugin the operator cannot reach")
	}
	if plan.status.ServerName == "" {
		t.Error("the server name is known from the declaration even before the plugin serves")
	}
}

func TestPlanBackup_FreshInstallWaitsForThePlugin(t *testing.T) {
	p := &PostgreSQL{}
	// Plugin installed (CRD ours) but its Deployment not yet available.
	c := fakeCluster(t, cnpgOurs, certManager, certIssuers, pluginOursCRD, pluginDeployment(false), keysSecret(resources.ObjectStoreKeyAccessKeyID, resources.ObjectStoreKeySecretAccessKey))
	plan, err := p.planBackup(context.Background(), c, backupPlatform(true, ""), nil)
	if err != nil {
		t.Fatal(err)
	}
	if !plan.holdCluster {
		t.Fatal("a fresh install with a backup must wait for the plugin so the database is born archiving")
	}
	if plan.status.State != v1.BackupStateDeploying || !strings.Contains(plan.waiting, "before the database") {
		t.Errorf("hold must be explained: %+v", plan)
	}
	if plan.backup != nil {
		t.Error("no plugin entry before the plugin serves")
	}
}

func TestPlanBackup_RunningDatabaseAttachesLiveAndNamesTheRestart(t *testing.T) {
	p := &PostgreSQL{}
	existing := resources.NewPostgreSQLCluster(resources.PostgreSQLClusterOptions{CRName: "planton", Namespace: "planton", Instances: 1, StorageSize: "1Gi"})

	// Plugin not serving yet: the Cluster keeps running without it.
	c := fakeCluster(t, cnpgOurs, certManager, certIssuers, pluginOursCRD, pluginDeployment(false), keysSecret(resources.ObjectStoreKeyAccessKeyID, resources.ObjectStoreKeySecretAccessKey))
	plan, err := p.planBackup(context.Background(), c, backupPlatform(true, ""), existing)
	if err != nil {
		t.Fatal(err)
	}
	if plan.holdCluster || plan.backup != nil {
		t.Errorf("a running database is never held and never wired to a plugin that is not serving: %+v", plan)
	}
	if !strings.Contains(plan.status.Message, "restarts the database once") {
		t.Errorf("the restart must be named before it happens: %q", plan.status.Message)
	}

	// Plugin serving: wire it, and still say the restart is coming.
	c = fakeCluster(t, cnpgOurs, certManager, certIssuers, pluginOursCRD, pluginDeployment(true), keysSecret(resources.ObjectStoreKeyAccessKeyID, resources.ObjectStoreKeySecretAccessKey))
	plan, err = p.planBackup(context.Background(), c, backupPlatform(true, ""), existing)
	if err != nil {
		t.Fatal(err)
	}
	if plan.backup == nil || plan.backup.ServerName != "planton-postgres-1a2b3c4d" || plan.backup.ObjectStoreName != "planton-postgres" {
		t.Errorf("plugin wiring %+v", plan.backup)
	}
	if !strings.Contains(plan.status.Message, "restarts the database once") {
		t.Errorf("attaching to a running database names the restart: %q", plan.status.Message)
	}
	kinds := map[string]int{}
	for _, o := range plan.before {
		kinds[o.GetKind()]++
	}
	if kinds["ObjectStore"] != 1 || kinds["Secret"] != 1 {
		t.Errorf("before the Cluster: one store and its settings Secret, got %v", kinds)
	}
	if len(plan.after) != 1 || plan.after[0].GetKind() != "ScheduledBackup" {
		t.Errorf("after the Cluster: the schedule, got %v", plan.after)
	}
}

func TestPlanBackup_MissingCredentialsIsFailingInWordsAndRendersNoStore(t *testing.T) {
	p := &PostgreSQL{}
	c := fakeCluster(t, cnpgOurs, certManager, certIssuers, pluginOursCRD, pluginDeployment(true), keysSecret(resources.ObjectStoreKeyAccessKeyID))
	plan, err := p.planBackup(context.Background(), c, backupPlatform(true, ""), nil)
	if err != nil {
		t.Fatal(err)
	}
	if plan.status.State != v1.BackupStateFailing || !strings.Contains(plan.status.Message, resources.ObjectStoreKeySecretAccessKey) {
		t.Errorf("want Failing naming the missing key, got %+v", plan.status)
	}
	if plan.backup != nil || len(plan.before) != 0 || plan.holdCluster {
		t.Error("nothing renders around a credential the plugin could not read; the database still runs")
	}

	c = fakeCluster(t, cnpgOurs, certManager, certIssuers, pluginOursCRD, pluginDeployment(true))
	plan, err = p.planBackup(context.Background(), c, backupPlatform(true, ""), nil)
	if err != nil {
		t.Fatal(err)
	}
	if plan.status.State != v1.BackupStateFailing || !strings.Contains(plan.status.Message, `"backup-keys"`) || !strings.Contains(plan.status.Message, "not found") {
		t.Errorf("a missing Secret is named: %+v", plan.status)
	}
}

func TestPlanBackup_RecoveryOnAFreshInstall(t *testing.T) {
	p := &PostgreSQL{}
	planton := backupPlatform(true, "")
	planton.Spec.Database.PostgreSQL.RecoverFrom = &v1.PostgreSQLRecoverFromSpec{
		ObjectStore: v1.ObjectStoreSpec{DestinationPath: "s3://bucket/platform", S3: &v1.S3ObjectStoreSpec{}},
		ServerName:  "planton-postgres-deadbeef",
		TargetTime:  "2026-09-10T12:00:00Z",
	}
	c := fakeCluster(t, cnpgOurs, certManager, certIssuers, pluginOursCRD, pluginDeployment(true), keysSecret(resources.ObjectStoreKeyAccessKeyID, resources.ObjectStoreKeySecretAccessKey))
	plan, err := p.planBackup(context.Background(), c, planton, nil)
	if err != nil {
		t.Fatal(err)
	}
	if plan.recovery == nil || plan.recovery.ServerName != "planton-postgres-deadbeef" || plan.recovery.ObjectStoreName != "planton-postgres-recovery-source" || plan.recovery.TargetTime != "2026-09-10T12:00:00Z" {
		t.Errorf("recovery wiring %+v", plan.recovery)
	}
	stores := 0
	for _, o := range plan.before {
		if o.GetKind() == "ObjectStore" {
			stores++
		}
	}
	if stores != 2 {
		t.Errorf("a recovering platform renders its own store AND the recovery source, got %d stores", stores)
	}
	if !strings.Contains(plan.status.Message, "Restoring") {
		t.Errorf("status says it is restoring: %q", plan.status.Message)
	}
}

func TestPlanBackup_RecoveryOnARunningDatabaseIsExplainedNotApplied(t *testing.T) {
	p := &PostgreSQL{}
	planton := backupPlatform(true, "")
	planton.Spec.Database.PostgreSQL.RecoverFrom = &v1.PostgreSQLRecoverFromSpec{
		ObjectStore: v1.ObjectStoreSpec{DestinationPath: "s3://bucket/platform", S3: &v1.S3ObjectStoreSpec{}},
		ServerName:  "planton-postgres-deadbeef",
	}
	existing := resources.NewPostgreSQLCluster(resources.PostgreSQLClusterOptions{CRName: "planton", Namespace: "planton", Instances: 1, StorageSize: "1Gi"})
	c := fakeCluster(t, cnpgOurs, certManager, certIssuers, pluginOursCRD, pluginDeployment(true), keysSecret(resources.ObjectStoreKeyAccessKeyID, resources.ObjectStoreKeySecretAccessKey))
	plan, err := p.planBackup(context.Background(), c, planton, existing)
	if err != nil {
		t.Fatal(err)
	}
	if plan.recovery != nil {
		t.Error("recovery is honored only at creation; a running database keeps its bootstrap")
	}
	if !strings.Contains(plan.status.Message, "delete its database volume claim") {
		t.Errorf("the procedure is named: %q", plan.status.Message)
	}
	if plan.holdCluster {
		t.Error("a running database is never held")
	}
}

func TestKeepLiveBootstrap(t *testing.T) {
	desired := resources.NewPostgreSQLCluster(resources.PostgreSQLClusterOptions{
		CRName: "planton", Namespace: "planton", Instances: 1, StorageSize: "1Gi",
		Recovery: &resources.PostgreSQLClusterRecovery{ObjectStoreName: "src", ServerName: "srv"},
	})
	live := resources.NewPostgreSQLCluster(resources.PostgreSQLClusterOptions{CRName: "planton", Namespace: "planton", Instances: 1, StorageSize: "1Gi"})
	keepLiveBootstrap(desired, live)
	if _, found, _ := unstructured.NestedMap(desired.Object, "spec", "bootstrap", "initdb"); !found {
		t.Error("the live initdb bootstrap must be kept")
	}
	if _, found, _ := unstructured.NestedSlice(desired.Object, "spec", "externalClusters"); found {
		t.Error("externalClusters the live cluster does not have must not be rendered")
	}
	keepLiveBootstrap(desired, nil)
}

func clusterWithStatus(conditions []any, status map[string]any) *unstructured.Unstructured {
	c := resources.NewPostgreSQLCluster(resources.PostgreSQLClusterOptions{
		CRName: "planton", Namespace: "planton", Instances: 1, StorageSize: "1Gi",
		Backup: &resources.PostgreSQLClusterBackup{ObjectStoreName: "planton-postgres", ServerName: "planton-postgres-1a2b3c4d"},
	})
	st := map[string]any{"conditions": conditions}
	maps.Copy(st, status)
	c.Object["status"] = st
	return c
}

func TestRefreshBackupStatus(t *testing.T) {
	p := &PostgreSQL{}
	planton := backupPlatform(true, "")
	provisional := v1.BackupStatus{State: v1.BackupStateDeploying, ServerName: "planton-postgres-1a2b3c4d", Message: "Waiting for the first base backup"}
	archivingTrue := []any{map[string]any{"type": "ContinuousArchiving", "status": "True", "message": "Continuous archiving is working"}}
	archivingFalse := []any{map[string]any{"type": "ContinuousArchiving", "status": "False", "message": "AccessDenied: the bucket refused the key"}}

	t.Run("no plugin on the cluster yet: provisional stands", func(t *testing.T) {
		plain := resources.NewPostgreSQLCluster(resources.PostgreSQLClusterOptions{CRName: "planton", Namespace: "planton", Instances: 1, StorageSize: "1Gi"})
		got := p.refreshBackupStatus(context.Background(), fakeCluster(t), planton, plain, provisional)
		if got != provisional {
			t.Errorf("got %+v", got)
		}
	})
	t.Run("archiving but no base backup yet: Deploying", func(t *testing.T) {
		got := p.refreshBackupStatus(context.Background(), fakeCluster(t), planton, clusterWithStatus(archivingTrue, nil), provisional)
		if got.State != v1.BackupStateDeploying || !strings.Contains(got.Message, "first base backup") {
			t.Errorf("got %+v", got)
		}
	})
	t.Run("archiving and a base backup: Healthy with the timestamp", func(t *testing.T) {
		got := p.refreshBackupStatus(context.Background(), fakeCluster(t), planton,
			clusterWithStatus(archivingTrue, map[string]any{"lastSuccessfulBackup": "2026-09-10T02:00:00Z", "firstRecoverabilityPoint": "2026-09-01T02:00:00Z"}), provisional)
		if got.State != v1.BackupStateHealthy || got.LastSuccessfulBackup == nil || got.FirstRecoverabilityPoint == nil || !strings.Contains(got.Message, "2026-09-10T02:00:00Z") {
			t.Errorf("got %+v", got)
		}
	})
	t.Run("archiving failing: Failing in the plugin's words, whatever the backups say", func(t *testing.T) {
		got := p.refreshBackupStatus(context.Background(), fakeCluster(t), planton,
			clusterWithStatus(archivingFalse, map[string]any{"lastSuccessfulBackup": "2026-09-10T02:00:00Z"}), provisional)
		if got.State != v1.BackupStateFailing || !strings.Contains(got.Message, "AccessDenied") {
			t.Errorf("got %+v", got)
		}
	})
	t.Run("a base backup failed after the last success: Failing with the Backup's own error", func(t *testing.T) {
		failed := &unstructured.Unstructured{}
		failed.SetGroupVersionKind(resources.BackupGVK)
		failed.SetName("planton-postgres-scheduled-1")
		failed.SetNamespace("planton")
		failed.Object["spec"] = map[string]any{"cluster": map[string]any{"name": "planton-postgres"}}
		failed.Object["status"] = map[string]any{"phase": "failed", "error": "NoSuchBucket: the bucket does not exist", "stoppedAt": "2026-09-11T02:01:00Z"}
		got := p.refreshBackupStatus(context.Background(), fakeCluster(t, failed), planton,
			clusterWithStatus(archivingTrue, map[string]any{"lastSuccessfulBackup": "2026-09-10T02:00:00Z", "lastFailedBackup": "2026-09-11T02:01:00Z"}), provisional)
		if got.State != v1.BackupStateFailing || !strings.Contains(got.Message, "NoSuchBucket") {
			t.Errorf("got %+v", got)
		}
	})
}
