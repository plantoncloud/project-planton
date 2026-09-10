package module

import (
	"github.com/pkg/errors"
	kubernetespostgresv1alpha1 "github.com/plantonhq/planton/catalog/kubernetes/kubernetespostgres/v1alpha1"
	"github.com/plantonhq/planton/pkg/cloudflare/r2"
	barmancloudv1 "github.com/plantonhq/planton/pkg/kubernetes/kubernetestypes/cloudnativepg/kubernetes/barmancloud/v1"
	postgresqlv1 "github.com/plantonhq/planton/pkg/kubernetes/kubernetestypes/cloudnativepg/kubernetes/postgresql/v1"
	kubernetescorev1 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/core/v1"
	kubernetesmeta "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/meta/v1"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

// createObjectStores renders the Barman Cloud ObjectStore resources:
//
//   - the BACKUP store (named after the cluster) when spec.backup is set —
//     the Cluster's plugins entry points the WAL archiver at it;
//   - the RECOVERY-SOURCE store (`<name>-recovery-source`) when the
//     bootstrap restores from a backup — the synthetic externalClusters
//     entry points at it.
//
// Each store's declared credentials are first materialized as a Secret
// (`<name>-backup-creds` / `<name>-recovery-creds`, plus `-endpoint-ca`
// for self-signed S3-compatible endpoints); keyless arms render the
// backend's ambient-identity flag instead and need no Secret at all.
func createObjectStores(ctx *pulumi.Context, locals *Locals,
	kubernetesProvider pulumi.ProviderResource,
	dependencies []pulumi.ResourceOption,
) ([]pulumi.Resource, error) {
	var created []pulumi.Resource

	if backup := locals.Spec.GetBackup(); backup != nil {
		store, err := createObjectStore(ctx, locals, kubernetesProvider, dependencies,
			locals.BackupObjectStoreName, backup.GetObjectStore(), backup.GetRetentionPolicy(),
			locals.BackupCredsSecretName, locals.BackupEndpointCaName)
		if err != nil {
			return nil, errors.Wrap(err, "failed to create backup object store")
		}
		created = append(created, store)
	}

	if recovery := locals.Spec.GetBootstrap().GetRecovery(); recovery != nil {
		// Recovery reads an EXISTING archive: retention never applies to it
		// (the plugin must not prune the source cluster's backups).
		store, err := createObjectStore(ctx, locals, kubernetesProvider, dependencies,
			locals.RecoveryObjectStoreName, recovery.GetObjectStore(), "",
			locals.RecoveryCredsSecretName, locals.RecoveryEndpointCaName)
		if err != nil {
			return nil, errors.Wrap(err, "failed to create recovery-source object store")
		}
		created = append(created, store)
	}

	return created, nil
}

// createObjectStore renders one ObjectStore resource plus its credential
// satellites. The ObjectStore CRD forbids configuration.serverName (the
// plugin takes it as a per-Cluster parameter instead), so this function
// never sets it.
func createObjectStore(ctx *pulumi.Context, locals *Locals,
	kubernetesProvider pulumi.ProviderResource,
	dependencies []pulumi.ResourceOption,
	storeName string,
	objectStore *kubernetespostgresv1alpha1.KubernetesPostgresObjectStore,
	retentionPolicy string,
	credsSecretName string,
	endpointCaSecretName string,
) (pulumi.Resource, error) {
	configuration := barmancloudv1.ObjectStoreSpecConfigurationArgs{
		DestinationPath: pulumi.String(objectStore.GetDestinationPath()),
	}

	var storeDeps []pulumi.Resource
	// Set by an arm that needs the plugin's sidecar tuned for its store
	// (today: the r2 arm's checksum posture); nil renders no block.
	var instanceSidecarConfiguration *barmancloudv1.ObjectStoreSpecInstanceSidecarConfigurationArgs

	switch {
	case objectStore.GetS3() != nil:
		s3 := objectStore.GetS3()
		s3Credentials := barmancloudv1.ObjectStoreSpecConfigurationS3CredentialsArgs{}
		if s3.GetKeyless() {
			s3Credentials.InheritFromIAMRole = pulumi.Bool(true)
		} else if accessKeys := s3.GetAccessKeys(); accessKeys != nil {
			// Both keys live in one deterministic Secret; the barman config
			// addresses each by key name.
			credsSecret, err := createOpaqueSecret(ctx, locals, kubernetesProvider, dependencies,
				credsSecretName, map[string]string{
					"ACCESS_KEY_ID":     accessKeys.GetAccessKeyId(),
					"SECRET_ACCESS_KEY": accessKeys.GetSecretAccessKey(),
				})
			if err != nil {
				return nil, errors.Wrap(err, "failed to create s3 credentials secret")
			}
			storeDeps = append(storeDeps, credsSecret)
			s3Credentials.AccessKeyId = barmancloudv1.ObjectStoreSpecConfigurationS3CredentialsAccessKeyIdArgs{
				Name: pulumi.String(credsSecretName),
				Key:  pulumi.String("ACCESS_KEY_ID"),
			}
			s3Credentials.SecretAccessKey = barmancloudv1.ObjectStoreSpecConfigurationS3CredentialsSecretAccessKeyArgs{
				Name: pulumi.String(credsSecretName),
				Key:  pulumi.String("SECRET_ACCESS_KEY"),
			}
		}
		configuration.S3Credentials = s3Credentials
		if s3.GetRegion() != "" {
			// The CRD models the region as a SecretKeySelector (not a plain
			// string), so the literal region rides its own deterministic
			// single-key Secret. barman-cloud reads it ONLY on the declared-key
			// path (its credential resolver returns before the region on
			// inheritFromIAMRole, and the AWS SDK derives the region from the
			// pod's IRSA environment there), so on a keyless store this Secret
			// is rendered but inert -- kept for a uniform shape, not for effect.
			regionSecretName := storeName + "-region"
			regionSecret, err := createOpaqueSecret(ctx, locals, kubernetesProvider, dependencies,
				regionSecretName, map[string]string{"AWS_REGION": s3.GetRegion()})
			if err != nil {
				return nil, errors.Wrap(err, "failed to create s3 region secret")
			}
			storeDeps = append(storeDeps, regionSecret)
			s3Credentials.Region = barmancloudv1.ObjectStoreSpecConfigurationS3CredentialsRegionArgs{
				Name: pulumi.String(regionSecretName),
				Key:  pulumi.String("AWS_REGION"),
			}
			configuration.S3Credentials = s3Credentials
		}
		if s3.GetEndpointUrl() != "" {
			configuration.EndpointURL = pulumi.String(s3.GetEndpointUrl())
		}
		if s3.GetEndpointCaPem() != "" {
			caSecret, err := createOpaqueSecret(ctx, locals, kubernetesProvider, dependencies,
				endpointCaSecretName, map[string]string{"ca.crt": s3.GetEndpointCaPem()})
			if err != nil {
				return nil, errors.Wrap(err, "failed to create endpoint CA secret")
			}
			storeDeps = append(storeDeps, caSecret)
			configuration.EndpointCA = barmancloudv1.ObjectStoreSpecConfigurationEndpointCAArgs{
				Name: pulumi.String(endpointCaSecretName),
				Key:  pulumi.String("ca.crt"),
			}
		}

	case objectStore.GetR2() != nil:
		// Cloudflare R2 rides Barman Cloud's S3 code path, but the spec is in
		// R2's own vocabulary; this branch performs the translation:
		//   - endpoint: the jurisdiction's host, composed by the shared
		//     pkg/cloudflare/r2 helper (an eu/fedramp/us bucket is served ONLY
		//     through <account>.<jurisdiction>.r2.cloudflarestorage.com);
		//   - region: "auto", the only region R2 accepts (Cloudflare aliases
		//     "" and us-east-1 to it, but the plugin needs an explicit value
		//     to set AWS_DEFAULT_REGION, so it is always rendered);
		//   - credentials: the S3 key pair as declared -- by default the
		//     CloudflareAccountApiToken's r2_access_key_id/r2_secret_access_key
		//     outputs, which that kind derives (id, SHA-256 of the value); no
		//     hashing happens here. There is no keyless posture for R2.
		// The two AWS_*_CHECKSUM_* variables ride the sidecar for this arm:
		// barman-cloud's boto3 attaches data-integrity checksums by default,
		// which S3-compatible stores may reject; "when_required" is the
		// plugin's documented posture for them and costs nothing on a store
		// that accepts the checksums.
		r2Store := objectStore.GetR2()
		r2Creds := r2Store.GetCredentials()
		credsSecret, err := createOpaqueSecret(ctx, locals, kubernetesProvider, dependencies,
			credsSecretName, map[string]string{
				"ACCESS_KEY_ID":     r2Creds.GetAccessKeyId().GetValue(),
				"SECRET_ACCESS_KEY": r2Creds.GetSecretAccessKey().GetValue(),
			})
		if err != nil {
			return nil, errors.Wrap(err, "failed to create r2 credentials secret")
		}
		storeDeps = append(storeDeps, credsSecret)
		regionSecretName := storeName + "-region"
		regionSecret, err := createOpaqueSecret(ctx, locals, kubernetesProvider, dependencies,
			regionSecretName, map[string]string{"AWS_REGION": r2.Region})
		if err != nil {
			return nil, errors.Wrap(err, "failed to create r2 region secret")
		}
		storeDeps = append(storeDeps, regionSecret)
		configuration.S3Credentials = barmancloudv1.ObjectStoreSpecConfigurationS3CredentialsArgs{
			AccessKeyId: barmancloudv1.ObjectStoreSpecConfigurationS3CredentialsAccessKeyIdArgs{
				Name: pulumi.String(credsSecretName),
				Key:  pulumi.String("ACCESS_KEY_ID"),
			},
			SecretAccessKey: barmancloudv1.ObjectStoreSpecConfigurationS3CredentialsSecretAccessKeyArgs{
				Name: pulumi.String(credsSecretName),
				Key:  pulumi.String("SECRET_ACCESS_KEY"),
			},
			Region: barmancloudv1.ObjectStoreSpecConfigurationS3CredentialsRegionArgs{
				Name: pulumi.String(regionSecretName),
				Key:  pulumi.String("AWS_REGION"),
			},
		}
		configuration.EndpointURL = pulumi.String(r2.S3Endpoint(
			r2Store.GetAccountId().GetValue(), r2Store.GetJurisdiction().GetValue()))
		instanceSidecarConfiguration = &barmancloudv1.ObjectStoreSpecInstanceSidecarConfigurationArgs{
			Env: barmancloudv1.ObjectStoreSpecInstanceSidecarConfigurationEnvArray{
				barmancloudv1.ObjectStoreSpecInstanceSidecarConfigurationEnvArgs{
					Name:  pulumi.String("AWS_REQUEST_CHECKSUM_CALCULATION"),
					Value: pulumi.String("when_required"),
				},
				barmancloudv1.ObjectStoreSpecInstanceSidecarConfigurationEnvArgs{
					Name:  pulumi.String("AWS_RESPONSE_CHECKSUM_VALIDATION"),
					Value: pulumi.String("when_required"),
				},
			},
		}

	case objectStore.GetGcs() != nil:
		gcs := objectStore.GetGcs()
		googleCredentials := barmancloudv1.ObjectStoreSpecConfigurationGoogleCredentialsArgs{}
		if gcs.GetKeyless() {
			googleCredentials.GkeEnvironment = pulumi.Bool(true)
		} else {
			credsSecret, err := createOpaqueSecret(ctx, locals, kubernetesProvider, dependencies,
				credsSecretName, map[string]string{
					"APPLICATION_CREDENTIALS": gcs.GetServiceAccountKeyJson(),
				})
			if err != nil {
				return nil, errors.Wrap(err, "failed to create gcs credentials secret")
			}
			storeDeps = append(storeDeps, credsSecret)
			googleCredentials.ApplicationCredentials = barmancloudv1.ObjectStoreSpecConfigurationGoogleCredentialsApplicationCredentialsArgs{
				Name: pulumi.String(credsSecretName),
				Key:  pulumi.String("APPLICATION_CREDENTIALS"),
			}
		}
		configuration.GoogleCredentials = googleCredentials

	case objectStore.GetAzureBlob() != nil:
		azure := objectStore.GetAzureBlob()
		azureCredentials := barmancloudv1.ObjectStoreSpecConfigurationAzureCredentialsArgs{}
		secretData := map[string]string{}
		switch {
		case azure.GetKeyless():
			azureCredentials.InheritFromAzureAD = pulumi.Bool(true)
			// The storage account still identifies the endpoint; barman
			// reads it from a secret reference even in the keyless posture.
			secretData["AZURE_STORAGE_ACCOUNT"] = azure.GetStorageAccount()
		case azure.GetConnectionString() != "":
			secretData["AZURE_STORAGE_CONNECTION_STRING"] = azure.GetConnectionString()
		default:
			secretData["AZURE_STORAGE_ACCOUNT"] = azure.GetStorageAccount()
			secretData["AZURE_STORAGE_KEY"] = azure.GetStorageKey()
		}
		credsSecret, err := createOpaqueSecret(ctx, locals, kubernetesProvider, dependencies,
			credsSecretName, secretData)
		if err != nil {
			return nil, errors.Wrap(err, "failed to create azure credentials secret")
		}
		storeDeps = append(storeDeps, credsSecret)
		if _, ok := secretData["AZURE_STORAGE_CONNECTION_STRING"]; ok {
			azureCredentials.ConnectionString = barmancloudv1.ObjectStoreSpecConfigurationAzureCredentialsConnectionStringArgs{
				Name: pulumi.String(credsSecretName),
				Key:  pulumi.String("AZURE_STORAGE_CONNECTION_STRING"),
			}
		}
		if _, ok := secretData["AZURE_STORAGE_ACCOUNT"]; ok {
			azureCredentials.StorageAccount = barmancloudv1.ObjectStoreSpecConfigurationAzureCredentialsStorageAccountArgs{
				Name: pulumi.String(credsSecretName),
				Key:  pulumi.String("AZURE_STORAGE_ACCOUNT"),
			}
		}
		if _, ok := secretData["AZURE_STORAGE_KEY"]; ok {
			azureCredentials.StorageKey = barmancloudv1.ObjectStoreSpecConfigurationAzureCredentialsStorageKeyArgs{
				Name: pulumi.String(credsSecretName),
				Key:  pulumi.String("AZURE_STORAGE_KEY"),
			}
		}
		configuration.AzureCredentials = azureCredentials
	}

	if wal := objectStore.GetWal(); wal != nil {
		walArgs := barmancloudv1.ObjectStoreSpecConfigurationWalArgs{}
		if wal.GetCompression() != "" {
			walArgs.Compression = pulumi.String(wal.GetCompression())
		}
		if wal.MaxParallel != nil {
			walArgs.MaxParallel = pulumi.Int(int(wal.GetMaxParallel()))
		}
		configuration.Wal = walArgs
	}

	if data := objectStore.GetData(); data != nil {
		dataArgs := barmancloudv1.ObjectStoreSpecConfigurationDataArgs{}
		if data.GetCompression() != "" {
			dataArgs.Compression = pulumi.String(data.GetCompression())
		}
		if data.Jobs != nil {
			dataArgs.Jobs = pulumi.Int(int(data.GetJobs()))
		}
		if data.GetImmediateCheckpoint() {
			dataArgs.ImmediateCheckpoint = pulumi.Bool(true)
		}
		configuration.Data = dataArgs
	}

	storeSpec := barmancloudv1.ObjectStoreSpecArgs{
		Configuration: configuration,
	}
	if instanceSidecarConfiguration != nil {
		storeSpec.InstanceSidecarConfiguration = instanceSidecarConfiguration
	}
	if retentionPolicy != "" {
		storeSpec.RetentionPolicy = pulumi.String(retentionPolicy)
	}

	opts := append([]pulumi.ResourceOption{pulumi.Provider(kubernetesProvider)}, dependencies...)
	if len(storeDeps) > 0 {
		opts = append(opts, pulumi.DependsOn(storeDeps))
	}

	return barmancloudv1.NewObjectStore(ctx, storeName,
		&barmancloudv1.ObjectStoreArgs{
			Metadata: kubernetesmeta.ObjectMetaArgs{
				Name:      pulumi.String(storeName),
				Namespace: pulumi.String(locals.Namespace),
				Labels:    pulumi.ToStringMap(locals.Labels),
			},
			Spec: storeSpec,
		}, opts...)
}

// createScheduledBackups renders one ScheduledBackup per declared schedule,
// each explicitly method=plugin against the cluster's backup ObjectStore —
// never the deprecated in-tree barmanObjectStore method.
func createScheduledBackups(ctx *pulumi.Context, locals *Locals,
	kubernetesProvider pulumi.ProviderResource,
	dependencies []pulumi.ResourceOption,
) error {
	backup := locals.Spec.GetBackup()
	if backup == nil {
		return nil
	}

	for _, schedule := range backup.GetSchedules() {
		scheduledBackupName := locals.ClusterName + "-" + schedule.GetName()

		spec := postgresqlv1.ScheduledBackupSpecArgs{
			Schedule: pulumi.String(schedule.GetSchedule()),
			Cluster: postgresqlv1.ScheduledBackupSpecClusterArgs{
				Name: pulumi.String(locals.ClusterName),
			},
			Method: pulumi.String("plugin"),
			PluginConfiguration: postgresqlv1.ScheduledBackupSpecPluginConfigurationArgs{
				Name: pulumi.String(vars.BarmanCloudPluginName),
				Parameters: pulumi.StringMap{
					"barmanObjectName": pulumi.String(locals.BackupObjectStoreName),
				},
			},
			// Scheduled backups belong to their schedule: deleting the
			// schedule garbage-collects its Backup records while the stored
			// objects in the bucket survive either way.
			BackupOwnerReference: pulumi.String("self"),
		}
		if schedule.GetImmediate() {
			spec.Immediate = pulumi.Bool(true)
		}
		if schedule.GetSuspend() {
			spec.Suspend = pulumi.Bool(true)
		}
		if schedule.GetTarget() != "" {
			spec.Target = pulumi.String(schedule.GetTarget())
		}

		if _, err := postgresqlv1.NewScheduledBackup(ctx, scheduledBackupName,
			&postgresqlv1.ScheduledBackupArgs{
				Metadata: kubernetesmeta.ObjectMetaArgs{
					Name:      pulumi.String(scheduledBackupName),
					Namespace: pulumi.String(locals.Namespace),
					Labels:    pulumi.ToStringMap(locals.Labels),
				},
				Spec: spec,
			}, append([]pulumi.ResourceOption{pulumi.Provider(kubernetesProvider)}, dependencies...)...); err != nil {
			return errors.Wrapf(err, "failed to create scheduled backup %s", scheduledBackupName)
		}
	}

	return nil
}

// createOpaqueSecret creates a plain Opaque Secret with the given string
// data — the shape barman's SecretKeySelector references expect.
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
