package v1

import metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

// PostgreSQLBackupSpec declares where the platform's own database is backed
// up and how. Declaring it turns on continuous WAL archiving to the store, a
// base backup on the schedule (the first one immediately -- WAL alone restores
// nothing), and a retention policy the store enforces; it also installs the
// Barman Cloud plugin, the database operator's backup engine, if the cluster
// does not have it yet (see PrerequisitesSpec.PostgresBackupPlugin).
//
// The platform never stores the cloud secret. Keyless identity through the
// database's own ServiceAccount is the preferred posture on every cloud;
// where keys must be used they live in a Secret the adopter owns and this
// declaration only names.
type PostgreSQLBackupSpec struct {
	// objectStore is the bucket the backups go to and how the database's pods
	// authenticate to it.
	ObjectStore ObjectStoreSpec `json:"objectStore"`

	// retentionPolicy is how long the store keeps backups and the WAL that
	// goes with them: an integer and a unit, d (days), w (weeks), or m
	// (months). Default 30d.
	// +kubebuilder:default="30d"
	// +kubebuilder:validation:Pattern=`^[1-9][0-9]*[dwm]$`
	// +optional
	RetentionPolicy string `json:"retentionPolicy,omitempty"`

	// schedule is when base backups run, as a SIX-field cron with seconds
	// first (the database operator's grammar): "0 0 2 * * *" is daily at
	// 02:00 UTC, the default. The first base backup always runs the moment
	// backups are declared, whatever this says.
	// +kubebuilder:default="0 0 2 * * *"
	// +optional
	Schedule string `json:"schedule,omitempty"`

	// serviceAccountAnnotations go on the ServiceAccount the database operator
	// creates for the database's own pods (it names that account after the
	// PostgreSQL cluster: "{platform}-postgres"). This is the keyless backup
	// identity: EKS "eks.amazonaws.com/role-arn", GKE
	// "iam.gke.io/gcp-service-account", AKS "azure.workload.identity/client-id"
	// -- the cloud-side binding is written against that account, and no
	// credential exists in the cluster. On GCS the bound identity needs BOTH
	// roles/storage.objectAdmin and roles/storage.legacyBucketReader: the
	// plugin verifies the destination with a bucket-level read before every
	// archive, and object administration alone does not carry it.
	// +optional
	ServiceAccountAnnotations map[string]string `json:"serviceAccountAnnotations,omitempty"`
}

// PostgreSQLRecoverFromSpec declares that this platform's database is to be
// restored from another platform's archive instead of created empty.
//
// Honored only when the database is first created: the database operator
// decides how a cluster's data comes to exist exactly once. On a running
// platform the operator keeps the database as it is and the status names the
// procedure -- delete the platform, delete its database volume claim, declare
// it again with recoverFrom. Nothing here ever destroys data to honor a
// declaration.
type PostgreSQLRecoverFromSpec struct {
	// objectStore is the store the source platform archived to. Recovery only
	// reads from it; this platform's own backups go to spec.database.postgresql.backup.
	ObjectStore ObjectStoreSpec `json:"objectStore"`

	// serverName is the name the source's archive is filed under -- printed
	// on the source platform's status (status.backup.serverName), or read from
	// the bucket's own listing when the source is gone. Every platform archives
	// under its own name, so a restored platform never writes over its source.
	// +kubebuilder:validation:MinLength=1
	ServerName string `json:"serverName"`

	// targetTime recovers to a point in time (RFC 3339). Empty recovers to
	// the end of the archive.
	// +optional
	TargetTime string `json:"targetTime,omitempty"`
}

// ObjectStoreSpec is a bucket and the way to authenticate to it. Exactly one
// backend is set, and the destination path carries that backend's scheme.
// +kubebuilder:validation:XValidation:rule="[has(self.s3), has(self.gcs), has(self.azureBlob), has(self.r2)].filter(x, x).size() == 1",message="an object store names exactly one backend: s3, gcs, azureBlob, or r2"
// +kubebuilder:validation:XValidation:rule="!(has(self.s3) || has(self.r2)) || self.destinationPath.startsWith('s3://')",message="an s3 or r2 store's destinationPath starts with s3://"
// +kubebuilder:validation:XValidation:rule="!has(self.gcs) || self.destinationPath.startsWith('gs://')",message="a gcs store's destinationPath starts with gs://"
// +kubebuilder:validation:XValidation:rule="!has(self.azureBlob) || self.destinationPath.startsWith('https://')",message="an azureBlob store's destinationPath is the container URL, https://<account>.blob.core.windows.net/<container>/<path>"
type ObjectStoreSpec struct {
	// destinationPath is the bucket and prefix: s3://bucket/path (s3 and
	// r2), gs://bucket/path (gcs), or the container URL (azureBlob).
	// +kubebuilder:validation:MinLength=1
	DestinationPath string `json:"destinationPath"`

	// +optional
	S3 *S3ObjectStoreSpec `json:"s3,omitempty"`
	// +optional
	GCS *GCSObjectStoreSpec `json:"gcs,omitempty"`
	// +optional
	AzureBlob *AzureBlobObjectStoreSpec `json:"azureBlob,omitempty"`
	// +optional
	R2 *R2ObjectStoreSpec `json:"r2,omitempty"`
}

// S3ObjectStoreSpec: Amazon S3, or any S3-compatible store by endpoint.
type S3ObjectStoreSpec struct {
	// endpointURL points at an S3-compatible store (MinIO, Ceph, a NAS).
	// Empty means Amazon S3.
	// +optional
	EndpointURL string `json:"endpointURL,omitempty"`

	// region of the bucket. Required by Amazon S3; some compatible stores
	// ignore it.
	// +optional
	Region string `json:"region,omitempty"`

	// credentialsSecretName names a Secret in the platform's namespace with
	// keys ACCESS_KEY_ID and SECRET_ACCESS_KEY. Empty means the database's
	// pods inherit an IAM role (IRSA, EKS Pod Identity, an instance profile)
	// -- the preferred posture; see serviceAccountAnnotations.
	// +optional
	CredentialsSecretName string `json:"credentialsSecretName,omitempty"`

	// endpointCASecretRef names the CA bundle a private endpoint's TLS chains
	// to, when the system trust store does not carry it.
	// +optional
	EndpointCASecretRef *SecretKeyRef `json:"endpointCASecretRef,omitempty"`
}

// GCSObjectStoreSpec: Google Cloud Storage.
type GCSObjectStoreSpec struct {
	// credentialsSecretName names a Secret in the platform's namespace with
	// key APPLICATION_CREDENTIALS holding a service-account JSON key. Empty
	// means GKE Workload Identity on the database's ServiceAccount -- the
	// preferred posture; see serviceAccountAnnotations.
	// +optional
	CredentialsSecretName string `json:"credentialsSecretName,omitempty"`
}

// AzureBlobObjectStoreSpec: Azure Blob Storage.
type AzureBlobObjectStoreSpec struct {
	// storageAccount is the account the container belongs to. Identifies the
	// endpoint for the keyless posture.
	// +kubebuilder:validation:MinLength=1
	StorageAccount string `json:"storageAccount"`

	// credentialsSecretName names a Secret in the platform's namespace with
	// key AZURE_STORAGE_CONNECTION_STRING. Empty means Azure AD workload
	// identity on the database's ServiceAccount -- the preferred posture; see
	// serviceAccountAnnotations.
	// +optional
	CredentialsSecretName string `json:"credentialsSecretName,omitempty"`
}

// R2ObjectStoreSpec: Cloudflare R2 through its S3 API. R2 has no keyless
// posture; the key pair is derived from a Cloudflare API token carrying an
// R2 permission group (access key id = the token's id; secret access key =
// the lowercase hex SHA-256 of the token's value) and always arrives as a
// Secret. The endpoint follows from the account and the bucket's
// jurisdiction, and the only region R2 accepts is set for you.
type R2ObjectStoreSpec struct {
	// accountId is the Cloudflare account that owns the bucket.
	// +kubebuilder:validation:MinLength=1
	AccountID string `json:"accountId"`

	// jurisdiction is the bucket's data residency: empty or "default" for
	// standard storage, else "eu", "fedramp", or "us". A jurisdictional
	// bucket is served ONLY through its own host, so this must match how the
	// bucket was created.
	// +kubebuilder:validation:Enum="";default;eu;fedramp;us
	// +optional
	Jurisdiction string `json:"jurisdiction,omitempty"`

	// credentialsSecretName names a Secret in the platform's namespace with
	// keys ACCESS_KEY_ID and SECRET_ACCESS_KEY.
	// +kubebuilder:validation:MinLength=1
	CredentialsSecretName string `json:"credentialsSecretName"`
}

// BackupState is the one-word answer to "is the platform's database being
// saved?", printed in the Backup column of `kubectl get plantonplatform`.
//
// Deliberately NOT an enum on the definition, for the same reason as
// ComponentReason: the adopter upgrades the operator before the platform, and
// an operator writing a state the installed definition's enum lacked would
// have its status write refused.
type BackupState string

const (
	// BackupStateNotConfigured: no backup is declared. Nothing is missing.
	BackupStateNotConfigured BackupState = "NotConfigured"

	// BackupStateUnavailable: a backup is declared but the plugin cannot be
	// installed on this cluster -- cert-manager is absent. The message names
	// the remedy. The platform runs; nothing is being saved yet.
	BackupStateUnavailable BackupState = "Unavailable"

	// BackupStateDeploying: the plugin is installing, the database is being
	// re-rolled to attach it, or the first base backup has not completed.
	BackupStateDeploying BackupState = "Deploying"

	// BackupStateHealthy: WAL archiving is continuous and a base backup
	// exists to replay it onto.
	BackupStateHealthy BackupState = "Healthy"

	// BackupStateFailing: archiving or a base backup failed; the message is
	// the plugin's own sentence. The platform stays Ready -- it is doing its
	// job; its safety net is not.
	BackupStateFailing BackupState = "Failing"
)

// BackupStatus is what the operator knows about the platform database's
// backup, read from the database operator's own conditions and backup
// objects -- never guessed here.
//
// A struct where status.license and status.email are single words, on
// purpose: for those the control plane owns the detail and the operator only
// names the mode; here the operator IS the source of truth, and a restore
// needs the server name and the recoverability point machine-readable.
type BackupStatus struct {
	// state is the one-word answer; see BackupState.
	State BackupState `json:"state"`

	// serverName is the name this platform's archive is filed under in the
	// store -- what a recovery declaration copies. Set once backups are
	// declared.
	// +optional
	ServerName string `json:"serverName,omitempty"`

	// firstRecoverabilityPoint is the earliest point in time the archive can
	// restore to (the start of the oldest base backup still retained).
	// +optional
	FirstRecoverabilityPoint *metav1.Time `json:"firstRecoverabilityPoint,omitempty"`

	// lastSuccessfulBackup is when the most recent base backup completed.
	// +optional
	LastSuccessfulBackup *metav1.Time `json:"lastSuccessfulBackup,omitempty"`

	// lastFailedBackup is when a base backup last failed, if one has.
	// +optional
	LastFailedBackup *metav1.Time `json:"lastFailedBackup,omitempty"`

	// message says why the state is what it is, in plain language -- the
	// plugin's own sentence when it is the one that knows.
	// +optional
	Message string `json:"message,omitempty"`
}
