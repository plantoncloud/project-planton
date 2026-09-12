package module

import (
	"encoding/base64"
	"encoding/json"
	"strings"

	"github.com/pkg/errors"
	kubernetesmongodbv1alpha1 "github.com/plantonhq/planton/catalog/kubernetes/kubernetesmongodb/v1alpha1"
	kubernetescorev1 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/core/v1"
	kubernetesmeta "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/meta/v1"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

// createCredentialSecrets materializes every DECLARED credential in the spec
// as a deterministic Kubernetes Secret, so nothing sensitive ever appears
// inline in the rendered custom resource — the operator and PBM only ever
// see Secret references:
//
//   - `<name>-user-<username>` (key `password`): a declared user password.
//     The CR's passwordSecretRef points at it; rotating the value rotates
//     the database password. Users WITHOUT a declared password get no Secret
//     here — the operator generates one.
//   - `<name>-backup-<storage>`: object-store keys for a backup storage.
//     Key names are exactly what the operator's PBM integration reads per
//     backend arm. The keyless S3 arm creates NO Secret — the PBM agents
//     use the pods' ambient AWS identity. GCS has no keyless arm (PBM's
//     Google client requires a key); a GCS storage that names an
//     `existing_secret_name` creates no Secret either — the operator reads
//     the one the user brought.
//
// Names are deterministic (never engine-generated suffixes) so both engines
// agree byte-for-byte and the import recipes derive them blind.
func createCredentialSecrets(ctx *pulumi.Context, locals *Locals,
	kubernetesProvider pulumi.ProviderResource,
	dependencies []pulumi.ResourceOption,
) ([]pulumi.Resource, error) {
	var created []pulumi.Resource

	for _, user := range locals.Spec.GetUsers() {
		if user.GetPassword() == "" {
			continue
		}
		secretName := locals.ClusterName + "-user-" + user.GetName()
		// The operator CO-OWNS this Secret's annotations: after applying
		// a password it stamps a `percona.com/<cluster>-<user>-hash`
		// marker (its rotation detector) onto the object. The module
		// owns the data; the annotations are the operator's — never
		// fight them (Terraform twin: lifecycle ignore_changes).
		secret, err := createOpaqueSecret(ctx, locals, kubernetesProvider,
			append(dependencies, pulumi.IgnoreChanges([]string{"metadata.annotations"})),
			secretName, map[string]string{"password": user.GetPassword()})
		if err != nil {
			return nil, errors.Wrapf(err, "failed to create password secret for user %s", user.GetName())
		}
		created = append(created, secret)
	}

	if backup := locals.Spec.GetBackup(); backup != nil {
		for _, storage := range backup.GetStorages() {
			data, ok, err := backupCredentialData(storage)
			if err != nil {
				return nil, errors.Wrapf(err, "backup storage %s credentials", storage.GetName())
			}
			if !ok {
				continue
			}
			secretName := locals.ClusterName + "-backup-" + storage.GetName()
			secret, err := createOpaqueSecret(ctx, locals, kubernetesProvider, dependencies,
				secretName, data)
			if err != nil {
				return nil, errors.Wrapf(err, "failed to create credentials secret for backup storage %s", storage.GetName())
			}
			created = append(created, secret)
		}
	}

	return created, nil
}

// backupCredentialData returns the Secret payload for a backup storage when
// declared credentials exist — mirrors local.backup_credential_secrets in
// the Terraform module's locals.tf.
func backupCredentialData(storage *kubernetesmongodbv1alpha1.KubernetesMongodbBackupStorage) (map[string]string, bool, error) {
	if s3 := storage.GetS3(); s3 != nil && s3.GetAccessKeys() != nil {
		return map[string]string{
			"AWS_ACCESS_KEY_ID":     s3.GetAccessKeys().GetAccessKeyId(),
			"AWS_SECRET_ACCESS_KEY": s3.GetAccessKeys().GetSecretAccessKey(),
		}, true, nil
	}
	if r2Storage := storage.GetR2(); r2Storage != nil {
		// The same two keys the operator reads for any S3-API store; the
		// values are the token's S3 pair as declared (the token kind derives
		// them -- nothing is hashed here).
		return map[string]string{
			"AWS_ACCESS_KEY_ID":     r2Storage.GetCredentials().GetAccessKeyId().GetValue(),
			"AWS_SECRET_ACCESS_KEY": r2Storage.GetCredentials().GetSecretAccessKey().GetValue(),
		}, true, nil
	}
	if gcs := storage.GetGcs(); gcs != nil && gcs.GetCredentials().GetServiceAccountKey() != nil {
		keyJSON, err := decodeServiceAccountKey(gcs.GetCredentials().GetServiceAccountKey().GetValue())
		if err != nil {
			return nil, false, err
		}
		var key struct {
			ClientEmail string `json:"client_email"`
			PrivateKey  string `json:"private_key"`
		}
		if err := json.Unmarshal(keyJSON, &key); err != nil {
			return nil, false, errors.Wrap(err, "malformed GCS service account key JSON")
		}
		if key.ClientEmail == "" || key.PrivateKey == "" {
			return nil, false, errors.New("the GCS service account key carries no client_email/private_key: PBM authenticates with exactly those two fields — export the key with GcpServiceAccount.user_managed_key (the generate flow), or paste the JSON key file's content")
		}
		return map[string]string{
			"GCS_CLIENT_EMAIL": key.ClientEmail,
			"GCS_PRIVATE_KEY":  key.PrivateKey,
		}, true, nil
	}
	if azure := storage.GetAzure(); azure != nil {
		return map[string]string{
			"AZURE_STORAGE_ACCOUNT_NAME": azure.GetStorageAccount(),
			"AZURE_STORAGE_ACCOUNT_KEY":  azure.GetAccessKey(),
		}, true, nil
	}
	return nil, false, nil
}

// decodeServiceAccountKey accepts the key file two ways — raw JSON, or the
// base64 encoding a GcpServiceAccount resource exports as `key_base64` — and
// returns the JSON bytes. Raw JSON is recognized by its opening brace; anything
// else must be valid standard base64. Twin: local.gcs_key_json in locals.tf.
func decodeServiceAccountKey(value string) ([]byte, error) {
	trimmed := strings.TrimSpace(value)
	if strings.HasPrefix(trimmed, "{") {
		return []byte(trimmed), nil
	}
	decoded, err := base64.StdEncoding.DecodeString(trimmed)
	if err != nil {
		return nil, errors.Wrap(err, "the GCS service account key is neither a JSON key file nor its base64 encoding (the GcpServiceAccount key_base64 output)")
	}
	return decoded, nil
}

func createOpaqueSecret(ctx *pulumi.Context, locals *Locals,
	kubernetesProvider pulumi.ProviderResource,
	dependencies []pulumi.ResourceOption,
	secretName string, data map[string]string,
) (pulumi.Resource, error) {
	return kubernetescorev1.NewSecret(ctx, secretName,
		&kubernetescorev1.SecretArgs{
			Metadata: kubernetesmeta.ObjectMetaArgs{
				Name:      pulumi.String(secretName),
				Namespace: pulumi.String(locals.Namespace),
				Labels:    pulumi.ToStringMap(locals.Labels),
			},
			StringData: pulumi.ToStringMap(data),
		}, append([]pulumi.ResourceOption{pulumi.Provider(kubernetesProvider)}, dependencies...)...)
}
