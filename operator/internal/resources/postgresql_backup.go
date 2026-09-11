package resources

import (
	"fmt"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
)

// The platform's PostgreSQL backs up through the Barman Cloud plugin
// (barman_plugin_helm.go): an ObjectStore names where the backups go and how
// to authenticate, the Cluster's plugins entry archives WAL there
// continuously under a server name, and a ScheduledBackup takes the base
// backups WAL is replayed onto. Recovery is the same three objects read the
// other way: a second ObjectStore pointing at the source's store, and the
// Cluster bootstrapped from it instead of from initdb.
//
// Everything here is a pure builder over plain option structs so the render
// maps are pinned by tests without a Kubernetes API in the room; the
// component maps the platform's declaration onto these options.
const (
	// PostgreSQLBackupDefaultSchedule is the base-backup cadence when the
	// declaration names none: daily at 02:00 UTC. CloudNativePG's schedule is
	// a SIX-field cron (seconds first), not the five-field crontab shape.
	PostgreSQLBackupDefaultSchedule = "0 0 2 * * *"

	// PostgreSQLBackupDefaultRetention keeps thirty days of recoverability
	// when the declaration names no policy. Barman's grammar: an integer and a
	// unit, d/w/m.
	PostgreSQLBackupDefaultRetention = "30d"

	// ObjectStoreKeyAccessKeyID and ObjectStoreKeySecretAccessKey are the two
	// keys an S3-compatible credentials Secret carries (S3 with access keys,
	// and Cloudflare R2, whose S3 key pair is derived from an API token).
	ObjectStoreKeyAccessKeyID     = "ACCESS_KEY_ID"
	ObjectStoreKeySecretAccessKey = "SECRET_ACCESS_KEY"

	// ObjectStoreKeyApplicationCredentials is the one key a GCS credentials
	// Secret carries: a service-account JSON key. Keyless installs (GKE
	// Workload Identity) name no Secret at all.
	ObjectStoreKeyApplicationCredentials = "APPLICATION_CREDENTIALS"

	// ObjectStoreKeyAzureConnectionString is the one key an Azure Blob
	// credentials Secret carries. Keyless installs (Azure AD workload
	// identity) name no Secret and identify the account by name instead.
	ObjectStoreKeyAzureConnectionString = "AZURE_STORAGE_CONNECTION_STRING"

	// The plugin's ObjectStore takes every setting as a Secret key
	// reference, including two that are not secret at all: the S3 region and
	// the Azure storage account. The operator renders them into a small
	// settings Secret beside the store rather than asking the adopter to put
	// non-secrets into their credentials Secret.
	objectStoreSettingsKeyRegion         = "AWS_REGION"
	objectStoreSettingsKeyStorageAccount = "AZURE_STORAGE_ACCOUNT"

	// r2Region is the only region Cloudflare R2 accepts on the S3 API, and
	// r2JurisdictionDefault the jurisdiction of a bucket created without one.
	// Source of truth: pkg/cloudflare/r2 in this repository, which the
	// operator (a standalone module) mirrors rather than imports.
	r2Region              = "auto"
	r2JurisdictionDefault = "default"

	// recoverySourceName is the externalClusters entry a recovering Cluster
	// bootstraps from. A constant because nothing else ever refers to it.
	recoverySourceName = "origin"

	scheduledBackupKind = "ScheduledBackup"
	backupKind          = "Backup"
	objectStoreKind     = "ObjectStore"
)

// ObjectStoreGVK is the Barman Cloud plugin's object-store definition.
var ObjectStoreGVK = schema.GroupVersionKind{
	Group:   barmanCloudAPIGroup,
	Version: barmanCloudAPIVersion,
	Kind:    objectStoreKind,
}

// ScheduledBackupGVK is CloudNativePG's recurring base-backup schedule.
var ScheduledBackupGVK = schema.GroupVersionKind{
	Group:   postgresqlAPIGroup,
	Version: postgresqlAPIVersion,
	Kind:    scheduledBackupKind,
}

// BackupGVK is one CloudNativePG base backup, created by a ScheduledBackup
// (or by hand). The component reads them for the backup state it reports.
var BackupGVK = schema.GroupVersionKind{
	Group:   postgresqlAPIGroup,
	Version: postgresqlAPIVersion,
	Kind:    backupKind,
}

// PostgreSQLBackupObjectStoreName is the platform's own backup store, named
// after the cluster it protects.
func PostgreSQLBackupObjectStoreName(crName string) string {
	return PostgreSQLClusterName(crName)
}

// PostgreSQLRecoveryObjectStoreName is the store a recovering platform reads
// its source from. Distinct from the backup store so a restored platform
// keeps archiving to its own destination while reading from the source's.
func PostgreSQLRecoveryObjectStoreName(crName string) string {
	return fmt.Sprintf("%s-recovery-source", PostgreSQLClusterName(crName))
}

// PostgreSQLScheduledBackupName is the platform's one base-backup schedule.
func PostgreSQLScheduledBackupName(crName string) string {
	return fmt.Sprintf("%s-scheduled", PostgreSQLClusterName(crName))
}

// ObjectStoreSettingsSecretName is the operator-rendered Secret holding the
// non-secret settings the plugin insists on reading from a Secret.
func ObjectStoreSettingsSecretName(storeName string) string {
	return fmt.Sprintf("%s-settings", storeName)
}

// PostgreSQLBackupServerName is the name this platform's archive is filed
// under in the object store: the cluster name plus the first eight characters
// of the platform's UID.
//
// Barman refuses to archive into a server whose WAL history belongs to
// another PostgreSQL system, and the refusal is quiet (archiving fails,
// instances stay healthy). A reinstall under the same platform name into the
// same bucket would hit exactly that; a restored platform archiving beside its
// source would too. The UID makes every install's archive its own, and the
// status prints the name so the recovery declaration can copy it.
func PostgreSQLBackupServerName(crName string, uid types.UID) string {
	short := string(uid)
	if len(short) > 8 {
		short = short[:8]
	}
	return fmt.Sprintf("%s-%s", PostgreSQLClusterName(crName), short)
}

// R2S3Endpoint is the S3 API host for a Cloudflare R2 account and
// jurisdiction: https://<account>.r2.cloudflarestorage.com for the default
// jurisdiction, https://<account>.<jurisdiction>.r2.cloudflarestorage.com
// otherwise. A jurisdictional bucket is served ONLY through its own host (the
// default host fails rather than redirects). Source of truth:
// pkg/cloudflare/r2.S3Endpoint in this repository; kept in step by hand
// because the operator module does not import the root module.
func R2S3Endpoint(accountID, jurisdiction string) string {
	if jurisdiction == "" || jurisdiction == r2JurisdictionDefault {
		return fmt.Sprintf("https://%s.r2.cloudflarestorage.com", accountID)
	}
	return fmt.Sprintf("https://%s.%s.r2.cloudflarestorage.com", accountID, jurisdiction)
}

// ObjectStoreOptions describes one Barman Cloud object store: where the
// backups live and how the database's pods authenticate to it. Exactly one
// backend is set; the caller (CEL on the definition, then the component)
// guarantees it.
type ObjectStoreOptions struct {
	Name      string
	Namespace string

	// DestinationPath is the store URL the backend expects: s3://bucket/path
	// for S3 and R2, gs://bucket/path for GCS, https://<account>.blob.core.windows.net/<container>/<path>
	// for Azure Blob.
	DestinationPath string

	S3        *S3ObjectStoreOptions
	GCS       *GCSObjectStoreOptions
	AzureBlob *AzureBlobObjectStoreOptions
	R2        *R2ObjectStoreOptions

	// RetentionPolicy (e.g. "30d") is rendered on a BACKUP store only; a
	// recovery store never carries one, so nothing this platform does can
	// expire the source's history.
	RetentionPolicy string

	OwnerRef *metav1.OwnerReference
}

// S3ObjectStoreOptions: Amazon S3 or any S3-compatible store. No credentials
// Secret means the database pods inherit an IAM role (IRSA, Pod Identity, an
// instance profile).
type S3ObjectStoreOptions struct {
	// EndpointURL is set for S3-compatible stores; empty for Amazon S3.
	EndpointURL string
	// Region is rendered into the settings Secret when set.
	Region string
	// CredentialsSecretName names an adopter-owned Secret carrying
	// ObjectStoreKeyAccessKeyID and ObjectStoreKeySecretAccessKey.
	CredentialsSecretName string
	// EndpointCASecretName/Key reference the CA bundle a private endpoint's
	// TLS chains to; both empty means the system trust store.
	EndpointCASecretName string
	EndpointCASecretKey  string
}

// GCSObjectStoreOptions: Google Cloud Storage. No credentials Secret means
// GKE Workload Identity on the database's ServiceAccount.
type GCSObjectStoreOptions struct {
	// CredentialsSecretName names an adopter-owned Secret carrying
	// ObjectStoreKeyApplicationCredentials.
	CredentialsSecretName string
}

// AzureBlobObjectStoreOptions: Azure Blob Storage. No credentials Secret
// means Azure AD workload identity, in which case StorageAccount identifies
// the endpoint.
type AzureBlobObjectStoreOptions struct {
	StorageAccount string
	// CredentialsSecretName names an adopter-owned Secret carrying
	// ObjectStoreKeyAzureConnectionString.
	CredentialsSecretName string
}

// R2ObjectStoreOptions: Cloudflare R2 through its S3 API. R2 has no keyless
// posture; the credentials Secret is required and carries the S3 key pair
// derived from a Cloudflare API token (ObjectStoreKeyAccessKeyID and
// ObjectStoreKeySecretAccessKey).
type R2ObjectStoreOptions struct {
	AccountID             string
	Jurisdiction          string
	CredentialsSecretName string
}

// CredentialsSecretName returns the adopter-owned credentials Secret the
// store references, or "" for a keyless posture.
func (o ObjectStoreOptions) CredentialsSecretName() string {
	switch {
	case o.S3 != nil:
		return o.S3.CredentialsSecretName
	case o.GCS != nil:
		return o.GCS.CredentialsSecretName
	case o.AzureBlob != nil:
		return o.AzureBlob.CredentialsSecretName
	case o.R2 != nil:
		return o.R2.CredentialsSecretName
	}
	return ""
}

// RequiredCredentialKeys lists the keys the credentials Secret must carry for
// the store's backend -- what the component preflights before rendering, so a
// missing key is reported in words instead of as a sidecar that cannot start.
// Empty for a keyless posture.
func (o ObjectStoreOptions) RequiredCredentialKeys() []string {
	if o.CredentialsSecretName() == "" {
		return nil
	}
	switch {
	case o.S3 != nil, o.R2 != nil:
		return []string{ObjectStoreKeyAccessKeyID, ObjectStoreKeySecretAccessKey}
	case o.GCS != nil:
		return []string{ObjectStoreKeyApplicationCredentials}
	case o.AzureBlob != nil:
		return []string{ObjectStoreKeyAzureConnectionString}
	}
	return nil
}

// NewObjectStore renders the plugin's barmancloud.cnpg.io/v1 ObjectStore.
//
// configuration.serverName is deliberately never set here: the plugin takes
// the server name as a per-Cluster parameter (see NewPostgreSQLCluster), so
// one store can hold several clusters' archives under distinct names.
func NewObjectStore(opts ObjectStoreOptions) *unstructured.Unstructured {
	obj := &unstructured.Unstructured{}
	obj.SetGroupVersionKind(ObjectStoreGVK)
	obj.SetName(opts.Name)
	obj.SetNamespace(opts.Namespace)
	if opts.OwnerRef != nil {
		obj.SetOwnerReferences([]metav1.OwnerReference{*opts.OwnerRef})
	}

	configuration := map[string]any{
		"destinationPath": opts.DestinationPath,
		// zstd for WAL (small, fast, the plugin's recommended default) and
		// gzip for base backups (widest tool compatibility for an archive a
		// person may have to read by hand in a disaster).
		"wal":  map[string]any{"compression": "zstd", "maxParallel": int64(2)},
		"data": map[string]any{"compression": "gzip", "jobs": int64(2), "immediateCheckpoint": true},
	}

	settings := ObjectStoreSettingsSecretName(opts.Name)
	spec := map[string]any{"configuration": configuration}

	switch {
	case opts.S3 != nil:
		s3 := opts.S3
		creds := map[string]any{}
		if s3.CredentialsSecretName == "" {
			creds["inheritFromIAMRole"] = true
		} else {
			creds["accessKeyId"] = secretKeyRef(s3.CredentialsSecretName, ObjectStoreKeyAccessKeyID)
			creds["secretAccessKey"] = secretKeyRef(s3.CredentialsSecretName, ObjectStoreKeySecretAccessKey)
		}
		if s3.Region != "" {
			creds["region"] = secretKeyRef(settings, objectStoreSettingsKeyRegion)
		}
		configuration["s3Credentials"] = creds
		if s3.EndpointURL != "" {
			configuration["endpointURL"] = s3.EndpointURL
		}
		if s3.EndpointCASecretName != "" {
			configuration["endpointCA"] = secretKeyRef(s3.EndpointCASecretName, s3.EndpointCASecretKey)
		}

	case opts.R2 != nil:
		r2 := opts.R2
		configuration["endpointURL"] = R2S3Endpoint(r2.AccountID, r2.Jurisdiction)
		configuration["s3Credentials"] = map[string]any{
			"accessKeyId":     secretKeyRef(r2.CredentialsSecretName, ObjectStoreKeyAccessKeyID),
			"secretAccessKey": secretKeyRef(r2.CredentialsSecretName, ObjectStoreKeySecretAccessKey),
			"region":          secretKeyRef(settings, objectStoreSettingsKeyRegion),
		}
		// R2 does not implement the request/response checksum trailers newer
		// AWS SDKs send by default; the sidecar's S3 client must fall back to
		// checksums only when the operation requires them.
		spec["instanceSidecarConfiguration"] = map[string]any{
			"env": []any{
				map[string]any{"name": "AWS_REQUEST_CHECKSUM_CALCULATION", "value": "when_required"},
				map[string]any{"name": "AWS_RESPONSE_CHECKSUM_VALIDATION", "value": "when_required"},
			},
		}

	case opts.GCS != nil:
		gcs := opts.GCS
		creds := map[string]any{}
		if gcs.CredentialsSecretName == "" {
			creds["gkeEnvironment"] = true
		} else {
			creds["applicationCredentials"] = secretKeyRef(gcs.CredentialsSecretName, ObjectStoreKeyApplicationCredentials)
		}
		configuration["googleCredentials"] = creds

	case opts.AzureBlob != nil:
		az := opts.AzureBlob
		creds := map[string]any{}
		if az.CredentialsSecretName == "" {
			creds["inheritFromAzureAD"] = true
			creds["storageAccount"] = secretKeyRef(settings, objectStoreSettingsKeyStorageAccount)
		} else {
			creds["connectionString"] = secretKeyRef(az.CredentialsSecretName, ObjectStoreKeyAzureConnectionString)
		}
		configuration["azureCredentials"] = creds
	}

	if opts.RetentionPolicy != "" {
		spec["retentionPolicy"] = opts.RetentionPolicy
	}

	obj.Object["spec"] = spec
	return obj
}

// NewObjectStoreSettingsSecret renders the settings Secret an ObjectStore's
// non-secret Secret-key references point at (the S3/R2 region, the Azure
// storage account). Returns nil when the store needs none, so the caller
// applies exactly what the store references.
func NewObjectStoreSettingsSecret(opts ObjectStoreOptions) *unstructured.Unstructured {
	data := map[string]any{}
	switch {
	case opts.S3 != nil && opts.S3.Region != "":
		data[objectStoreSettingsKeyRegion] = opts.S3.Region
	case opts.R2 != nil:
		data[objectStoreSettingsKeyRegion] = r2Region
	case opts.AzureBlob != nil && opts.AzureBlob.CredentialsSecretName == "":
		data[objectStoreSettingsKeyStorageAccount] = opts.AzureBlob.StorageAccount
	}
	if len(data) == 0 {
		return nil
	}

	obj := &unstructured.Unstructured{}
	obj.SetAPIVersion("v1")
	obj.SetKind("Secret")
	obj.SetName(ObjectStoreSettingsSecretName(opts.Name))
	obj.SetNamespace(opts.Namespace)
	if opts.OwnerRef != nil {
		obj.SetOwnerReferences([]metav1.OwnerReference{*opts.OwnerRef})
	}
	obj.Object["type"] = "Opaque"
	obj.Object["stringData"] = data
	return obj
}

// ScheduledBackupOptions configures the platform's one base-backup schedule.
type ScheduledBackupOptions struct {
	CRName    string
	Namespace string
	// Schedule is a six-field cron (seconds first); empty means
	// PostgreSQLBackupDefaultSchedule.
	Schedule string
	// ObjectStoreName is the backup store the base backups go to.
	ObjectStoreName string
	OwnerRef        *metav1.OwnerReference
}

// NewScheduledBackup renders the postgresql.cnpg.io/v1 ScheduledBackup that
// takes the platform's base backups through the plugin.
//
// immediate is always true: WAL alone restores nothing without a base backup
// to replay onto, so the first backup runs the moment the schedule exists
// rather than at the first cron tick (up to a day away). backupOwnerReference
// "self" makes every Backup object the schedule's child, so they leave with
// the schedule and the schedule leaves with the platform.
func NewScheduledBackup(opts ScheduledBackupOptions) *unstructured.Unstructured {
	obj := &unstructured.Unstructured{}
	obj.SetGroupVersionKind(ScheduledBackupGVK)
	obj.SetName(PostgreSQLScheduledBackupName(opts.CRName))
	obj.SetNamespace(opts.Namespace)
	if opts.OwnerRef != nil {
		obj.SetOwnerReferences([]metav1.OwnerReference{*opts.OwnerRef})
	}

	schedule := opts.Schedule
	if schedule == "" {
		schedule = PostgreSQLBackupDefaultSchedule
	}

	obj.Object["spec"] = map[string]any{
		"schedule":             schedule,
		"cluster":              map[string]any{"name": PostgreSQLClusterName(opts.CRName)},
		"method":               "plugin",
		"pluginConfiguration":  pluginConfiguration(opts.ObjectStoreName, ""),
		"backupOwnerReference": "self",
		"immediate":            true,
	}
	return obj
}

// pluginConfiguration is the {name, parameters} block a Cluster's plugins
// entry, a ScheduledBackup, and an externalClusters entry all address the
// plugin with. serverName is omitted when empty so the plugin's default (the
// cluster name) applies where the caller wants it.
func pluginConfiguration(objectStoreName, serverName string) map[string]any {
	params := map[string]any{"barmanObjectName": objectStoreName}
	if serverName != "" {
		params["serverName"] = serverName
	}
	return map[string]any{
		"name":       BarmanCloudPluginName,
		"parameters": params,
	}
}

func secretKeyRef(name, key string) map[string]any {
	return map[string]any{"name": name, "key": key}
}
