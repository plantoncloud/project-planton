package resources

import (
	"testing"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/types"
)

const (
	testStoreName      = "planton-postgres"
	testSettingsSecret = "planton-postgres-settings"
)

func nested(t *testing.T, obj *unstructured.Unstructured, fields ...string) any {
	t.Helper()
	v, found, err := unstructured.NestedFieldNoCopy(obj.Object, fields...)
	if err != nil {
		t.Fatalf("reading %v: %v", fields, err)
	}
	if !found {
		t.Fatalf("expected %v to be set; object: %v", fields, obj.Object)
	}
	return v
}

func absent(t *testing.T, obj *unstructured.Unstructured, fields ...string) {
	t.Helper()
	if _, found, _ := unstructured.NestedFieldNoCopy(obj.Object, fields...); found {
		t.Errorf("expected %v to be absent; object: %v", fields, obj.Object)
	}
}

func TestPostgreSQLBackupServerName_CarriesTheInstallUID(t *testing.T) {
	name := PostgreSQLBackupServerName("planton", types.UID("1a2b3c4d-0000-4000-8000-000000000000"))
	if name != "planton-postgres-1a2b3c4d" {
		t.Errorf("server name %q; want the cluster name plus the UID's first eight characters", name)
	}
	if got := PostgreSQLBackupServerName("planton", types.UID("abc")); got != "planton-postgres-abc" {
		t.Errorf("short UID rendered %q", got)
	}
}

func TestR2S3Endpoint(t *testing.T) {
	cases := map[string]string{
		"":        "https://acct.r2.cloudflarestorage.com",
		"default": "https://acct.r2.cloudflarestorage.com",
		"eu":      "https://acct.eu.r2.cloudflarestorage.com",
		"fedramp": "https://acct.fedramp.r2.cloudflarestorage.com",
	}
	for jurisdiction, want := range cases {
		if got := R2S3Endpoint("acct", jurisdiction); got != want {
			t.Errorf("jurisdiction %q: got %q want %q", jurisdiction, got, want)
		}
	}
}

func TestNewObjectStore_S3Keyless(t *testing.T) {
	obj := NewObjectStore(ObjectStoreOptions{
		Name: testStoreName, Namespace: "planton",
		DestinationPath: "s3://bucket/platform",
		S3:              &S3ObjectStoreOptions{},
		RetentionPolicy: "30d",
	})
	if obj.GetAPIVersion() != "barmancloud.cnpg.io/v1" || obj.GetKind() != "ObjectStore" {
		t.Fatalf("wrong GVK: %s %s", obj.GetAPIVersion(), obj.GetKind())
	}
	if got := nested(t, obj, "spec", "configuration", "destinationPath"); got != "s3://bucket/platform" {
		t.Errorf("destinationPath %v", got)
	}
	if got := nested(t, obj, "spec", "configuration", "s3Credentials", "inheritFromIAMRole"); got != true {
		t.Errorf("keyless S3 must inherit the IAM role, got %v", got)
	}
	absent(t, obj, "spec", "configuration", "s3Credentials", "accessKeyId")
	absent(t, obj, "spec", "configuration", "s3Credentials", "region")
	absent(t, obj, "spec", "configuration", "endpointURL")
	absent(t, obj, "spec", "configuration", "serverName")
	if got := nested(t, obj, "spec", "retentionPolicy"); got != "30d" {
		t.Errorf("retentionPolicy %v", got)
	}
	if NewObjectStoreSettingsSecret(ObjectStoreOptions{Name: "x", S3: &S3ObjectStoreOptions{}}) != nil {
		t.Error("a keyless S3 store without a region needs no settings Secret")
	}
}

func TestNewObjectStore_S3WithKeysRegionEndpointAndCA(t *testing.T) {
	opts := ObjectStoreOptions{
		Name: testStoreName, Namespace: "planton",
		DestinationPath: "s3://bucket/platform",
		S3: &S3ObjectStoreOptions{
			EndpointURL:           "https://minio.lab.svc:9000",
			Region:                "us-east-1",
			CredentialsSecretName: "backup-keys",
			EndpointCASecretName:  "lab-ca",
			EndpointCASecretKey:   "ca.crt",
		},
	}
	obj := NewObjectStore(opts)
	creds := nested(t, obj, "spec", "configuration", "s3Credentials").(map[string]any)
	absent(t, obj, "spec", "configuration", "s3Credentials", "inheritFromIAMRole")
	if ref := creds["accessKeyId"].(map[string]any); ref["name"] != "backup-keys" || ref["key"] != ObjectStoreKeyAccessKeyID {
		t.Errorf("accessKeyId ref %v", ref)
	}
	if ref := creds["secretAccessKey"].(map[string]any); ref["name"] != "backup-keys" || ref["key"] != ObjectStoreKeySecretAccessKey {
		t.Errorf("secretAccessKey ref %v", ref)
	}
	if ref := creds["region"].(map[string]any); ref["name"] != testSettingsSecret || ref["key"] != "AWS_REGION" {
		t.Errorf("region must be read from the operator's settings Secret, got %v", ref)
	}
	if got := nested(t, obj, "spec", "configuration", "endpointURL"); got != "https://minio.lab.svc:9000" {
		t.Errorf("endpointURL %v", got)
	}
	if ca := nested(t, obj, "spec", "configuration", "endpointCA").(map[string]any); ca["name"] != "lab-ca" || ca["key"] != "ca.crt" {
		t.Errorf("endpointCA %v", ca)
	}
	absent(t, obj, "spec", "retentionPolicy")

	settings := NewObjectStoreSettingsSecret(opts)
	if settings == nil {
		t.Fatal("a region needs the settings Secret")
	}
	if settings.GetName() != testSettingsSecret || settings.GetNamespace() != "planton" {
		t.Errorf("settings Secret %s/%s", settings.GetNamespace(), settings.GetName())
	}
	if got := nested(t, settings, "stringData", "AWS_REGION"); got != "us-east-1" {
		t.Errorf("AWS_REGION %v", got)
	}
	if keys := opts.RequiredCredentialKeys(); len(keys) != 2 || keys[0] != ObjectStoreKeyAccessKeyID {
		t.Errorf("required keys %v", keys)
	}
}

func TestNewObjectStore_R2IsS3DialectWithChecksumFallback(t *testing.T) {
	opts := ObjectStoreOptions{
		Name: testStoreName, Namespace: "planton",
		DestinationPath: "s3://bucket/platform",
		R2:              &R2ObjectStoreOptions{AccountID: "acct", Jurisdiction: "eu", CredentialsSecretName: "r2-keys"},
	}
	obj := NewObjectStore(opts)
	if got := nested(t, obj, "spec", "configuration", "endpointURL"); got != "https://acct.eu.r2.cloudflarestorage.com" {
		t.Errorf("R2 endpoint %v", got)
	}
	creds := nested(t, obj, "spec", "configuration", "s3Credentials").(map[string]any)
	if ref := creds["accessKeyId"].(map[string]any); ref["name"] != "r2-keys" {
		t.Errorf("accessKeyId ref %v", ref)
	}
	if ref := creds["region"].(map[string]any); ref["name"] != testSettingsSecret {
		t.Errorf("R2 region must come from the settings Secret, got %v", ref)
	}
	env := nested(t, obj, "spec", "instanceSidecarConfiguration", "env").([]any)
	if len(env) != 2 {
		t.Fatalf("expected the two checksum env vars on the sidecar, got %v", env)
	}
	if e := env[0].(map[string]any); e["name"] != "AWS_REQUEST_CHECKSUM_CALCULATION" || e["value"] != "when_required" {
		t.Errorf("first sidecar env %v", e)
	}
	settings := NewObjectStoreSettingsSecret(opts)
	if settings == nil {
		t.Fatal("R2 always needs the region setting")
	}
	if got := nested(t, settings, "stringData", "AWS_REGION"); got != "auto" {
		t.Errorf("R2 region must be %q, got %v", "auto", got)
	}
}

func TestNewObjectStore_GCSKeylessAndKeyed(t *testing.T) {
	keyless := NewObjectStore(ObjectStoreOptions{Name: "s", Namespace: "n", DestinationPath: "gs://b/p", GCS: &GCSObjectStoreOptions{}})
	if got := nested(t, keyless, "spec", "configuration", "googleCredentials", "gkeEnvironment"); got != true {
		t.Errorf("keyless GCS must set gkeEnvironment, got %v", got)
	}
	absent(t, keyless, "spec", "configuration", "googleCredentials", "applicationCredentials")

	keyedOpts := ObjectStoreOptions{Name: "s", Namespace: "n", DestinationPath: "gs://b/p", GCS: &GCSObjectStoreOptions{CredentialsSecretName: "gcs-key"}}
	keyed := NewObjectStore(keyedOpts)
	ref := nested(t, keyed, "spec", "configuration", "googleCredentials", "applicationCredentials").(map[string]any)
	if ref["name"] != "gcs-key" || ref["key"] != ObjectStoreKeyApplicationCredentials {
		t.Errorf("applicationCredentials ref %v", ref)
	}
	absent(t, keyed, "spec", "configuration", "googleCredentials", "gkeEnvironment")
	if keys := keyedOpts.RequiredCredentialKeys(); len(keys) != 1 || keys[0] != ObjectStoreKeyApplicationCredentials {
		t.Errorf("required keys %v", keys)
	}
	if NewObjectStoreSettingsSecret(keyedOpts) != nil {
		t.Error("GCS needs no settings Secret")
	}
}

func TestNewObjectStore_AzureKeylessAndKeyed(t *testing.T) {
	keylessOpts := ObjectStoreOptions{
		Name: "s", Namespace: "n",
		DestinationPath: "https://acct.blob.core.windows.net/c/p",
		AzureBlob:       &AzureBlobObjectStoreOptions{StorageAccount: "acct"},
	}
	keyless := NewObjectStore(keylessOpts)
	if got := nested(t, keyless, "spec", "configuration", "azureCredentials", "inheritFromAzureAD"); got != true {
		t.Errorf("keyless Azure must inherit from Azure AD, got %v", got)
	}
	if ref := nested(t, keyless, "spec", "configuration", "azureCredentials", "storageAccount").(map[string]any); ref["name"] != "s-settings" || ref["key"] != "AZURE_STORAGE_ACCOUNT" {
		t.Errorf("storageAccount ref %v", ref)
	}
	settings := NewObjectStoreSettingsSecret(keylessOpts)
	if settings == nil || nested(t, settings, "stringData", "AZURE_STORAGE_ACCOUNT") != "acct" {
		t.Errorf("keyless Azure needs the storage account in the settings Secret, got %v", settings)
	}

	keyedOpts := ObjectStoreOptions{
		Name: "s", Namespace: "n",
		DestinationPath: "https://acct.blob.core.windows.net/c/p",
		AzureBlob:       &AzureBlobObjectStoreOptions{StorageAccount: "acct", CredentialsSecretName: "az"},
	}
	keyed := NewObjectStore(keyedOpts)
	if ref := nested(t, keyed, "spec", "configuration", "azureCredentials", "connectionString").(map[string]any); ref["name"] != "az" || ref["key"] != ObjectStoreKeyAzureConnectionString {
		t.Errorf("connectionString ref %v", ref)
	}
	absent(t, keyed, "spec", "configuration", "azureCredentials", "inheritFromAzureAD")
	if NewObjectStoreSettingsSecret(keyedOpts) != nil {
		t.Error("a connection string carries the account; no settings Secret")
	}
}

func TestNewObjectStore_RecoveryStoreCarriesNoRetention(t *testing.T) {
	obj := NewObjectStore(ObjectStoreOptions{
		Name: PostgreSQLRecoveryObjectStoreName("planton"), Namespace: "planton",
		DestinationPath: "s3://bucket/platform", S3: &S3ObjectStoreOptions{},
	})
	if obj.GetName() != "planton-postgres-recovery-source" {
		t.Errorf("recovery store name %q", obj.GetName())
	}
	absent(t, obj, "spec", "retentionPolicy")
}

func TestNewScheduledBackup(t *testing.T) {
	obj := NewScheduledBackup(ScheduledBackupOptions{CRName: "planton", Namespace: "planton", ObjectStoreName: testStoreName})
	if obj.GetAPIVersion() != "postgresql.cnpg.io/v1" || obj.GetKind() != "ScheduledBackup" {
		t.Fatalf("wrong GVK: %s %s", obj.GetAPIVersion(), obj.GetKind())
	}
	if obj.GetName() != "planton-postgres-scheduled" {
		t.Errorf("name %q", obj.GetName())
	}
	if got := nested(t, obj, "spec", "schedule"); got != PostgreSQLBackupDefaultSchedule {
		t.Errorf("default schedule %v", got)
	}
	if got := nested(t, obj, "spec", "cluster", "name"); got != testStoreName {
		t.Errorf("cluster %v", got)
	}
	if got := nested(t, obj, "spec", "method"); got != "plugin" {
		t.Errorf("method %v", got)
	}
	if got := nested(t, obj, "spec", "pluginConfiguration", "name"); got != BarmanCloudPluginName {
		t.Errorf("plugin name %v", got)
	}
	if got := nested(t, obj, "spec", "pluginConfiguration", "parameters", "barmanObjectName"); got != testStoreName {
		t.Errorf("barmanObjectName %v", got)
	}
	if got := nested(t, obj, "spec", "immediate"); got != true {
		t.Error("the first base backup must be immediate: WAL alone restores nothing")
	}
	if got := nested(t, obj, "spec", "backupOwnerReference"); got != "self" {
		t.Errorf("backupOwnerReference %v", got)
	}

	custom := NewScheduledBackup(ScheduledBackupOptions{CRName: "planton", Namespace: "planton", ObjectStoreName: testStoreName, Schedule: "0 30 4 * * *"})
	if got := nested(t, custom, "spec", "schedule"); got != "0 30 4 * * *" {
		t.Errorf("custom schedule %v", got)
	}
}

func TestNewPostgreSQLCluster_NoBackupRendersNoPlugins(t *testing.T) {
	obj := NewPostgreSQLCluster(PostgreSQLClusterOptions{CRName: "planton", Namespace: "planton", Instances: 1, StorageSize: "10Gi"})
	absent(t, obj, "spec", "plugins")
	absent(t, obj, "spec", "externalClusters")
	absent(t, obj, "spec", "serviceAccountTemplate")
	nested(t, obj, "spec", "bootstrap", "initdb")
}

func TestNewPostgreSQLCluster_BackupWiresThePluginUnderTheServerName(t *testing.T) {
	obj := NewPostgreSQLCluster(PostgreSQLClusterOptions{
		CRName: "planton", Namespace: "planton", Instances: 1, StorageSize: "10Gi",
		Backup:                    &PostgreSQLClusterBackup{ObjectStoreName: testStoreName, ServerName: "planton-postgres-1a2b3c4d"},
		ServiceAccountAnnotations: map[string]string{"iam.gke.io/gcp-service-account": "backups@proj.iam.gserviceaccount.com"},
	})
	plugins := nested(t, obj, "spec", "plugins").([]any)
	if len(plugins) != 1 {
		t.Fatalf("expected one plugin entry, got %v", plugins)
	}
	p := plugins[0].(map[string]any)
	if p["name"] != BarmanCloudPluginName || p["isWALArchiver"] != true {
		t.Errorf("plugin entry %v", p)
	}
	params := p["parameters"].(map[string]any)
	if params["barmanObjectName"] != testStoreName || params["serverName"] != "planton-postgres-1a2b3c4d" {
		t.Errorf("plugin parameters %v", params)
	}
	if got := nested(t, obj, "spec", "serviceAccountTemplate", "metadata", "annotations", "iam.gke.io/gcp-service-account"); got != "backups@proj.iam.gserviceaccount.com" {
		t.Errorf("service account annotation %v", got)
	}
	// Still born from initdb: backups do not change how a fresh cluster's
	// data comes to exist.
	nested(t, obj, "spec", "bootstrap", "initdb")
	absent(t, obj, "spec", "externalClusters")
}

func TestNewPostgreSQLCluster_RecoveryReplacesInitdb(t *testing.T) {
	obj := NewPostgreSQLCluster(PostgreSQLClusterOptions{
		CRName: "planton", Namespace: "planton", Instances: 1, StorageSize: "10Gi",
		Recovery: &PostgreSQLClusterRecovery{
			ObjectStoreName: "planton-postgres-recovery-source",
			ServerName:      "planton-postgres-deadbeef",
			TargetTime:      "2026-09-10T12:00:00Z",
		},
		Backup: &PostgreSQLClusterBackup{ObjectStoreName: testStoreName, ServerName: "planton-postgres-1a2b3c4d"},
	})
	absent(t, obj, "spec", "bootstrap", "initdb")
	if got := nested(t, obj, "spec", "bootstrap", "recovery", "source"); got != "origin" {
		t.Errorf("recovery source %v", got)
	}
	if got := nested(t, obj, "spec", "bootstrap", "recovery", "recoveryTarget", "targetTime"); got != "2026-09-10T12:00:00Z" {
		t.Errorf("targetTime %v", got)
	}
	ext := nested(t, obj, "spec", "externalClusters").([]any)
	if len(ext) != 1 {
		t.Fatalf("expected one externalClusters entry, got %v", ext)
	}
	origin := ext[0].(map[string]any)
	if origin["name"] != "origin" {
		t.Errorf("externalClusters name %v", origin["name"])
	}
	plugin := origin["plugin"].(map[string]any)
	params := plugin["parameters"].(map[string]any)
	if plugin["name"] != BarmanCloudPluginName || params["barmanObjectName"] != "planton-postgres-recovery-source" || params["serverName"] != "planton-postgres-deadbeef" {
		t.Errorf("recovery plugin %v", plugin)
	}
	// The restored platform archives to its OWN store under its own name --
	// the two never point at the same server.
	archive := nested(t, obj, "spec", "plugins").([]any)[0].(map[string]any)["parameters"].(map[string]any)
	if archive["serverName"] == params["serverName"] {
		t.Error("a restored platform must not archive over its source")
	}
}

func TestNewPostgreSQLCluster_RecoveryWithoutTargetRecoversToLatest(t *testing.T) {
	obj := NewPostgreSQLCluster(PostgreSQLClusterOptions{
		CRName: "planton", Namespace: "planton", Instances: 1, StorageSize: "10Gi",
		Recovery: &PostgreSQLClusterRecovery{ObjectStoreName: "src", ServerName: "srv"},
	})
	absent(t, obj, "spec", "bootstrap", "recovery", "recoveryTarget")
}
