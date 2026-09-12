package kubernetespostgresv1alpha1

import (
	"testing"

	"buf.build/go/protovalidate"
	"github.com/onsi/ginkgo/v2"
	"github.com/onsi/gomega"
	kubernetes "github.com/plantonhq/planton/catalog/kubernetes"
	"github.com/plantonhq/planton/shared"
	"github.com/plantonhq/planton/shared/cloudresourcekind"
	foreignkeyv1 "github.com/plantonhq/planton/shared/foreignkey/v1"
)

func TestKubernetesPostgres(t *testing.T) {
	gomega.RegisterFailHandler(ginkgo.Fail)
	ginkgo.RunSpecs(t, "KubernetesPostgres Suite")
}

func int32Ptr(i int32) *int32    { return &i }
func int64Ptr(i int64) *int64    { return &i }
func boolPtr(b bool) *bool       { return &b }
func stringPtr(s string) *string { return &s }

func literal(value string) *foreignkeyv1.StringValueOrRef {
	return &foreignkeyv1.StringValueOrRef{
		LiteralOrRef: &foreignkeyv1.StringValueOrRef_Value{Value: value},
	}
}

func valueFrom(kind cloudresourcekind.CloudResourceKind, name, fieldPath string) *foreignkeyv1.StringValueOrRef {
	return &foreignkeyv1.StringValueOrRef{
		LiteralOrRef: &foreignkeyv1.StringValueOrRef_ValueFrom{
			ValueFrom: &foreignkeyv1.ValueFromRef{
				Kind:      kind,
				Name:      name,
				FieldPath: fieldPath,
			},
		},
	}
}

// s3KeylessStore returns a minimal valid object store (s3 arm, keyless
// posture) — the base every backup/recovery test mutates from.
func s3KeylessStore(path string) *KubernetesPostgresObjectStore {
	return &KubernetesPostgresObjectStore{
		DestinationPath: path,
		Backend: &KubernetesPostgresObjectStore_S3{
			S3: &KubernetesPostgresS3ObjectStore{
				Region:  "us-east-1",
				Keyless: true,
			},
		},
	}
}

func gcsStore(path string, gcs *KubernetesPostgresGcsObjectStore) *KubernetesPostgresObjectStore {
	return &KubernetesPostgresObjectStore{
		DestinationPath: path,
		Backend:         &KubernetesPostgresObjectStore_Gcs{Gcs: gcs},
	}
}

func azureStore(path string, azure *KubernetesPostgresAzureBlobObjectStore) *KubernetesPostgresObjectStore {
	return &KubernetesPostgresObjectStore{
		DestinationPath: path,
		Backend:         &KubernetesPostgresObjectStore_AzureBlob{AzureBlob: azure},
	}
}

// r2Store returns an object store on the Cloudflare R2 arm with every
// reference in its default-kind form (the bucket for account and
// jurisdiction, the account API token for the key pair) — the composed
// shape a chart or the console produces.
func r2Store(path string) *KubernetesPostgresObjectStore {
	return &KubernetesPostgresObjectStore{
		DestinationPath: path,
		Backend: &KubernetesPostgresObjectStore_R2{R2: &KubernetesPostgresR2ObjectStore{
			AccountId:    valueFrom(cloudresourcekind.CloudResourceKind_CloudflareR2Bucket, "pg-archive", "status.outputs.account_id"),
			Jurisdiction: valueFrom(cloudresourcekind.CloudResourceKind_CloudflareR2Bucket, "pg-archive", "status.outputs.jurisdiction"),
			Credentials: &KubernetesPostgresR2Credentials{
				AccessKeyId:     valueFrom(cloudresourcekind.CloudResourceKind_CloudflareAccountApiToken, "pg-archive-writer", "status.outputs.r2_access_key_id"),
				SecretAccessKey: valueFrom(cloudresourcekind.CloudResourceKind_CloudflareAccountApiToken, "pg-archive-writer", "status.outputs.r2_secret_access_key"),
			},
		}},
	}
}

// validBackup returns a minimal valid backup block (s3 keyless, no
// schedules) for tests that mutate one backup rule at a time.
func validBackup() *KubernetesPostgresBackup {
	return &KubernetesPostgresBackup{
		ObjectStore: s3KeylessStore("s3://pg-backups/main"),
	}
}

var _ = ginkgo.Describe("KubernetesPostgres Validation Tests", func() {
	var input *KubernetesPostgres

	ginkgo.BeforeEach(func() {
		input = &KubernetesPostgres{
			ApiVersion: "kubernetes.planton.dev/v1alpha1",
			Kind:       "KubernetesPostgres",
			Metadata: &shared.CloudResourceMetadata{
				Name: "test-postgres",
			},
			Spec: &KubernetesPostgresSpec{
				Namespace: literal("databases"),
				Storage:   &KubernetesPostgresStorage{Size: "10Gi"},
			},
		}
	})

	ginkgo.Describe("When valid input is passed", func() {
		ginkgo.It("minimal spec should not return a validation error", func() {
			gomega.Expect(protovalidate.Validate(input)).To(gomega.BeNil())
		})

		ginkgo.It("namespace as a reference should be valid", func() {
			input.Spec.Namespace = valueFrom(cloudresourcekind.CloudResourceKind_KubernetesNamespace, "databases", "spec.name")
			gomega.Expect(protovalidate.Validate(input)).To(gomega.BeNil())
		})

		ginkgo.It("instances of 1 should be valid (gte 1 boundary)", func() {
			input.Spec.Instances = int32Ptr(1)
			gomega.Expect(protovalidate.Validate(input)).To(gomega.BeNil())
		})

		ginkgo.It("production instance count should be valid", func() {
			input.Spec.Instances = int32Ptr(3)
			gomega.Expect(protovalidate.Validate(input)).To(gomega.BeNil())
		})

		ginkgo.It("storage size '500Mi' should be valid (size_quantity)", func() {
			input.Spec.Storage.Size = "500Mi"
			gomega.Expect(protovalidate.Validate(input)).To(gomega.BeNil())
		})

		ginkgo.It("fractional storage size '1.5Ti' should be valid (size_quantity)", func() {
			input.Spec.Storage.Size = "1.5Ti"
			gomega.Expect(protovalidate.Validate(input)).To(gomega.BeNil())
		})

		ginkgo.It("storage class as a reference with resize disabled should be valid", func() {
			input.Spec.Storage.StorageClass = valueFrom(cloudresourcekind.CloudResourceKind_KubernetesStorageClass, "fast-ssd", "status.outputs.storage_class_name")
			input.Spec.Storage.ResizeInUseVolumes = boolPtr(false)
			gomega.Expect(protovalidate.Validate(input)).To(gomega.BeNil())
		})

		ginkgo.It("dedicated WAL storage should be valid", func() {
			input.Spec.WalStorage = &KubernetesPostgresStorage{Size: "5Gi"}
			gomega.Expect(protovalidate.Validate(input)).To(gomega.BeNil())
		})

		ginkgo.It("quorum synchronous replication should be valid", func() {
			input.Spec.Instances = int32Ptr(3)
			input.Spec.Postgresql = &KubernetesPostgresServerConfig{
				Synchronous: &KubernetesPostgresSynchronousReplication{
					Method:         stringPtr("any"),
					Number:         1,
					DataDurability: stringPtr("required"),
				},
			}
			gomega.Expect(protovalidate.Validate(input)).To(gomega.BeNil())
		})

		ginkgo.It("priority synchronous replication with preferred durability should be valid", func() {
			input.Spec.Instances = int32Ptr(3)
			input.Spec.Postgresql = &KubernetesPostgresServerConfig{
				Synchronous: &KubernetesPostgresSynchronousReplication{
					Method:         stringPtr("first"),
					Number:         2,
					DataDurability: stringPtr("preferred"),
				},
			}
			gomega.Expect(protovalidate.Validate(input)).To(gomega.BeNil())
		})

		ginkgo.It("initdb bootstrap should be valid", func() {
			input.Spec.Bootstrap = &KubernetesPostgresBootstrap{
				Method: &KubernetesPostgresBootstrap_Initdb{
					Initdb: &KubernetesPostgresBootstrapInitDb{
						Database:      stringPtr("orders"),
						Owner:         "orders-owner",
						DataChecksums: true,
					},
				},
			}
			gomega.Expect(protovalidate.Validate(input)).To(gomega.BeNil())
		})

		ginkgo.It("microservice import with exactly one database should be valid", func() {
			input.Spec.ExternalClusters = []*KubernetesPostgresExternalCluster{
				{Name: "legacy-rds", ConnectionParameters: map[string]string{"host": "legacy.example.com"}},
			}
			input.Spec.Bootstrap = &KubernetesPostgresBootstrap{
				Method: &KubernetesPostgresBootstrap_Initdb{
					Initdb: &KubernetesPostgresBootstrapInitDb{
						Import: &KubernetesPostgresImport{
							Type:                  "microservice",
							SourceExternalCluster: "legacy-rds",
							Databases:             []string{"orders"},
						},
					},
				},
			}
			gomega.Expect(protovalidate.Validate(input)).To(gomega.BeNil())
		})

		ginkgo.It("monolith import with several databases and roles should be valid", func() {
			input.Spec.ExternalClusters = []*KubernetesPostgresExternalCluster{
				{Name: "legacy-rds"},
			}
			input.Spec.Bootstrap = &KubernetesPostgresBootstrap{
				Method: &KubernetesPostgresBootstrap_Initdb{
					Initdb: &KubernetesPostgresBootstrapInitDb{
						Import: &KubernetesPostgresImport{
							Type:                  "monolith",
							SourceExternalCluster: "legacy-rds",
							Databases:             []string{"orders", "billing"},
							Roles:                 []string{"app-reader", "app-writer"},
						},
					},
				},
			}
			gomega.Expect(protovalidate.Validate(input)).To(gomega.BeNil())
		})

		ginkgo.It("recovery bootstrap with a single PITR selector should be valid", func() {
			input.Spec.Bootstrap = &KubernetesPostgresBootstrap{
				Method: &KubernetesPostgresBootstrap_Recovery{
					Recovery: &KubernetesPostgresBootstrapRecovery{
						ObjectStore:      s3KeylessStore("s3://pg-backups/source"),
						SourceServerName: "orders-db",
						RecoveryTarget: &KubernetesPostgresRecoveryTarget{
							TargetTime: "2026-07-20T06:00:00Z",
						},
					},
				},
			}
			gomega.Expect(protovalidate.Validate(input)).To(gomega.BeNil())
		})

		ginkgo.It("recovery naming the source's database, owner, and app Secret should be valid (credential continuity — the DR shape)", func() {
			input.Spec.Bootstrap = &KubernetesPostgresBootstrap{
				Method: &KubernetesPostgresBootstrap_Recovery{
					Recovery: &KubernetesPostgresBootstrapRecovery{
						ObjectStore:      s3KeylessStore("s3://pg-backups/source"),
						SourceServerName: "orders-db",
						Database:         "orders",
						Owner:            "orders",
						OwnerSecretName:  "orders-db-app",
					},
				},
			}
			gomega.Expect(protovalidate.Validate(input)).To(gomega.BeNil())
		})

		ginkgo.It("recovery target with only backup_id should be valid (backup_id is not a selector)", func() {
			input.Spec.Bootstrap = &KubernetesPostgresBootstrap{
				Method: &KubernetesPostgresBootstrap_Recovery{
					Recovery: &KubernetesPostgresBootstrapRecovery{
						ObjectStore:      s3KeylessStore("s3://pg-backups/source"),
						SourceServerName: "orders-db",
						RecoveryTarget: &KubernetesPostgresRecoveryTarget{
							BackupId: "20260720T060000",
						},
					},
				},
			}
			gomega.Expect(protovalidate.Validate(input)).To(gomega.BeNil())
		})

		ginkgo.It("pg_basebackup bootstrap with a declared source should be valid", func() {
			input.Spec.ExternalClusters = []*KubernetesPostgresExternalCluster{
				{Name: "live-primary", Password: "replica-pass"},
			}
			input.Spec.Bootstrap = &KubernetesPostgresBootstrap{
				Method: &KubernetesPostgresBootstrap_PgBasebackup{
					PgBasebackup: &KubernetesPostgresBootstrapPgBaseBackup{
						Source: "live-primary",
					},
				},
			}
			gomega.Expect(protovalidate.Validate(input)).To(gomega.BeNil())
		})

		ginkgo.It("external clusters with distinct names should be valid (unique_names)", func() {
			input.Spec.ExternalClusters = []*KubernetesPostgresExternalCluster{
				{Name: "source-a"},
				{Name: "source-b"},
			}
			gomega.Expect(protovalidate.Validate(input)).To(gomega.BeNil())
		})

		ginkgo.It("superuser enabled with a password should be valid (password_requires_enabled)", func() {
			input.Spec.Superuser = &KubernetesPostgresSuperuser{
				Enabled:  true,
				Password: "s3cr3t",
			}
			gomega.Expect(protovalidate.Validate(input)).To(gomega.BeNil())
		})

		ginkgo.It("superuser enabled without a password should be valid (operator generates one)", func() {
			input.Spec.Superuser = &KubernetesPostgresSuperuser{Enabled: true}
			gomega.Expect(protovalidate.Validate(input)).To(gomega.BeNil())
		})

		ginkgo.It("roles with distinct names and each password posture should be valid", func() {
			input.Spec.Roles = []*KubernetesPostgresRole{
				{Name: "app-writer", Login: true, Password: "writer-pass", ConnectionLimit: int64Ptr(50)},
				{Name: "cert-user", Login: true, DisablePassword: true},
				{Name: "analytics", Ensure: stringPtr("absent"), ConnectionLimit: int64Ptr(-1)},
			}
			gomega.Expect(protovalidate.Validate(input)).To(gomega.BeNil())
		})

		ginkgo.It("backup to s3 with the keyless posture should be valid", func() {
			input.Spec.Backup = validBackup()
			gomega.Expect(protovalidate.Validate(input)).To(gomega.BeNil())
		})

		ginkgo.It("backup to s3 with access keys should be valid (keyless_xor_keys)", func() {
			backup := validBackup()
			backup.ObjectStore.GetS3().Keyless = false
			backup.ObjectStore.GetS3().AccessKeys = &KubernetesPostgresS3AccessKeys{
				AccessKeyId:     "AKIAEXAMPLE",
				SecretAccessKey: "secret",
			}
			input.Spec.Backup = backup
			gomega.Expect(protovalidate.Validate(input)).To(gomega.BeNil())
		})

		ginkgo.It("backup to an S3-compatible endpoint with access keys should be valid (MinIO posture)", func() {
			backup := validBackup()
			s3 := backup.ObjectStore.GetS3()
			s3.Keyless = false
			s3.EndpointUrl = "http://minio.minio-system.svc:9000"
			s3.EndpointCaPem = "-----BEGIN CERTIFICATE-----\nMIIB...\n-----END CERTIFICATE-----"
			s3.AccessKeys = &KubernetesPostgresS3AccessKeys{
				AccessKeyId:     "minio",
				SecretAccessKey: "minio123",
			}
			input.Spec.Backup = backup
			gomega.Expect(protovalidate.Validate(input)).To(gomega.BeNil())
		})

		ginkgo.It("backup to r2 composed from the Cloudflare kinds should be valid", func() {
			input.Spec.Backup = &KubernetesPostgresBackup{ObjectStore: r2Store("s3://pg-archive/main")}
			gomega.Expect(protovalidate.Validate(input)).To(gomega.BeNil())
		})

		ginkgo.It("backup to r2 with a literal account, jurisdiction, and dashboard-minted key pair should be valid", func() {
			store := r2Store("s3://pg-archive/main")
			store.GetR2().AccountId = literal("4793d734c0b8e484dfc37ec392b5fa8a")
			store.GetR2().Jurisdiction = literal("eu")
			store.GetR2().Credentials.AccessKeyId = literal("f267e341f3dd4697bd3b9f71dd96247f")
			store.GetR2().Credentials.SecretAccessKey = literal("ba7816bf8f01cfea414140de5dae2223b00361a396177a9cb410ff61f20015ad")
			input.Spec.Backup = &KubernetesPostgresBackup{ObjectStore: store}
			gomega.Expect(protovalidate.Validate(input)).To(gomega.BeNil())
		})

		ginkgo.It("backup to r2 without a jurisdiction should be valid (empty is the default jurisdiction)", func() {
			store := r2Store("s3://pg-archive/main")
			store.GetR2().Jurisdiction = nil
			input.Spec.Backup = &KubernetesPostgresBackup{ObjectStore: store}
			gomega.Expect(protovalidate.Validate(input)).To(gomega.BeNil())
		})

		ginkgo.It("recovery from an r2 archive should be valid (the arm serves both the backup and the recovery store)", func() {
			input.Spec.Bootstrap = &KubernetesPostgresBootstrap{
				Method: &KubernetesPostgresBootstrap_Recovery{
					Recovery: &KubernetesPostgresBootstrapRecovery{
						ObjectStore:      r2Store("s3://pg-archive/source"),
						SourceServerName: "orders-db",
						Database:         "orders",
						Owner:            "orders",
						OwnerSecretName:  "orders-db-app",
					},
				},
			}
			gomega.Expect(protovalidate.Validate(input)).To(gomega.BeNil())
		})

		ginkgo.It("backup to gcs with the keyless posture should be valid (keyless_xor_key)", func() {
			input.Spec.Backup = &KubernetesPostgresBackup{
				ObjectStore: gcsStore("gs://pg-backups/main", &KubernetesPostgresGcsObjectStore{Keyless: true}),
			}
			gomega.Expect(protovalidate.Validate(input)).To(gomega.BeNil())
		})

		ginkgo.It("backup to gcs with a service-account key should be valid (keyless_xor_key)", func() {
			input.Spec.Backup = &KubernetesPostgresBackup{
				ObjectStore: gcsStore("gs://pg-backups/main", &KubernetesPostgresGcsObjectStore{
					ServiceAccountKeyJson: `{"type":"service_account","project_id":"my-project"}`,
				}),
			}
			gomega.Expect(protovalidate.Validate(input)).To(gomega.BeNil())
		})

		ginkgo.It("backup to azure with the keyless posture and an account should be valid (single_posture)", func() {
			input.Spec.Backup = &KubernetesPostgresBackup{
				ObjectStore: azureStore("https://myaccount.blob.core.windows.net/backups/pg", &KubernetesPostgresAzureBlobObjectStore{
					Keyless:        true,
					StorageAccount: "myaccount",
				}),
			}
			gomega.Expect(protovalidate.Validate(input)).To(gomega.BeNil())
		})

		ginkgo.It("backup to azure with a connection string should be valid (single_posture)", func() {
			input.Spec.Backup = &KubernetesPostgresBackup{
				ObjectStore: azureStore("https://myaccount.blob.core.windows.net/backups/pg", &KubernetesPostgresAzureBlobObjectStore{
					ConnectionString: "DefaultEndpointsProtocol=https;AccountName=myaccount;AccountKey=key==",
				}),
			}
			gomega.Expect(protovalidate.Validate(input)).To(gomega.BeNil())
		})

		ginkgo.It("backup to azure with storage_account + storage_key should be valid (single_posture)", func() {
			input.Spec.Backup = &KubernetesPostgresBackup{
				ObjectStore: azureStore("https://myaccount.blob.core.windows.net/backups/pg", &KubernetesPostgresAzureBlobObjectStore{
					StorageAccount: "myaccount",
					StorageKey:     "base64key==",
				}),
			}
			gomega.Expect(protovalidate.Validate(input)).To(gomega.BeNil())
		})

		ginkgo.It("retention policies in days, weeks, and months should be valid (retention_format)", func() {
			for _, retention := range []string{"30d", "8w", "6m"} {
				backup := validBackup()
				backup.RetentionPolicy = retention
				input.Spec.Backup = backup
				gomega.Expect(protovalidate.Validate(input)).To(gomega.BeNil())
			}
		})

		ginkgo.It("backup schedules with distinct names and six-field crons should be valid", func() {
			backup := validBackup()
			backup.Schedules = []*KubernetesPostgresBackupSchedule{
				{Name: "daily", Schedule: "0 0 2 * * *", Immediate: true},
				{Name: "weekly", Schedule: "0 30 4 * * 0", Suspend: true, Target: stringPtr("primary")},
			}
			input.Spec.Backup = backup
			gomega.Expect(protovalidate.Validate(input)).To(gomega.BeNil())
		})

		ginkgo.It("prefer-standby backup target should be valid (target_enum)", func() {
			backup := validBackup()
			backup.Schedules = []*KubernetesPostgresBackupSchedule{
				{Name: "daily", Schedule: "0 0 2 * * *", Target: stringPtr("prefer-standby")},
			}
			input.Spec.Backup = backup
			gomega.Expect(protovalidate.Validate(input)).To(gomega.BeNil())
		})

		ginkgo.It("WAL and data tuning with valid compression and parallelism should be valid", func() {
			backup := validBackup()
			backup.ObjectStore.Wal = &KubernetesPostgresWalTuning{
				Compression: "zstd",
				MaxParallel: int32Ptr(4),
			}
			backup.ObjectStore.Data = &KubernetesPostgresDataTuning{
				Compression:         "snappy",
				Jobs:                int32Ptr(2),
				ImmediateCheckpoint: true,
			}
			input.Spec.Backup = backup
			gomega.Expect(protovalidate.Validate(input)).To(gomega.BeNil())
		})

		ginkgo.It("wal max_parallel and data jobs of 1 should be valid (gte 1 boundary)", func() {
			backup := validBackup()
			backup.ObjectStore.Wal = &KubernetesPostgresWalTuning{MaxParallel: int32Ptr(1)}
			backup.ObjectStore.Data = &KubernetesPostgresDataTuning{Jobs: int32Ptr(1)}
			input.Spec.Backup = backup
			gomega.Expect(protovalidate.Validate(input)).To(gomega.BeNil())
		})

		ginkgo.It("server TLS secret paired with its CA secret should be valid (tls_needs_ca)", func() {
			input.Spec.Certificates = &KubernetesPostgresCertificates{
				ServerTlsSecret: valueFrom(cloudresourcekind.CloudResourceKind_KubernetesCertificate, "pg-server-cert", "status.outputs.secret_name"),
				ServerCaSecret:  "pg-server-ca",
			}
			gomega.Expect(protovalidate.Validate(input)).To(gomega.BeNil())
		})

		ginkgo.It("alt DNS names without a custom server certificate should be valid", func() {
			input.Spec.Certificates = &KubernetesPostgresCertificates{
				ServerAltDnsNames: []string{"pg.example.com"},
			}
			gomega.Expect(protovalidate.Validate(input)).To(gomega.BeNil())
		})

		ginkgo.It("required anti-affinity scheduling should be valid (anti_affinity_enum)", func() {
			input.Spec.Scheduling = &KubernetesPostgresScheduling{
				AntiAffinityType: stringPtr("required"),
				TopologyKey:      "topology.kubernetes.io/zone",
			}
			gomega.Expect(protovalidate.Validate(input)).To(gomega.BeNil())
		})

		ginkgo.It("supervised switchover update strategy should be valid (strategy/method enums)", func() {
			input.Spec.UpdateStrategy = &KubernetesPostgresUpdateStrategy{
				PrimaryUpdateStrategy: stringPtr("supervised"),
				PrimaryUpdateMethod:   stringPtr("switchover"),
			}
			gomega.Expect(protovalidate.Validate(input)).To(gomega.BeNil())
		})

		ginkgo.It("enable_pdb explicitly disabled should be valid", func() {
			input.Spec.EnablePdb = boolPtr(false)
			gomega.Expect(protovalidate.Validate(input)).To(gomega.BeNil())
		})

		ginkgo.It("full-surface spec with every block populated should be valid", func() {
			input.Spec = &KubernetesPostgresSpec{
				Namespace:       literal("databases"),
				CreateNamespace: true,
				Instances:       int32Ptr(3),
				ImageName:       "ghcr.io/cloudnative-pg/postgresql:17.5",
				Storage: &KubernetesPostgresStorage{
					Size:               "100Gi",
					StorageClass:       literal("fast-ssd"),
					ResizeInUseVolumes: boolPtr(true),
				},
				WalStorage: &KubernetesPostgresStorage{
					Size:         "20Gi",
					StorageClass: literal("fast-ssd"),
				},
				Resources: &kubernetes.ContainerResources{
					Requests: &kubernetes.CpuMemory{Cpu: "1", Memory: "2Gi"},
					Limits:   &kubernetes.CpuMemory{Cpu: "2", Memory: "4Gi"},
				},
				Postgresql: &KubernetesPostgresServerConfig{
					Parameters: map[string]string{
						"max_connections": "200",
						"shared_buffers":  "1GB",
					},
					PgHba:                  []string{"hostssl all all 10.0.0.0/8 scram-sha-256"},
					PgIdent:                []string{"appmap /^(.*)@example\\.com$ \\1"},
					SharedPreloadLibraries: []string{"pg_stat_statements"},
					Synchronous: &KubernetesPostgresSynchronousReplication{
						Method:         stringPtr("any"),
						Number:         1,
						DataDurability: stringPtr("required"),
					},
					EnableAlterSystem: true,
				},
				Bootstrap: &KubernetesPostgresBootstrap{
					Method: &KubernetesPostgresBootstrap_Initdb{
						Initdb: &KubernetesPostgresBootstrapInitDb{
							Database:               stringPtr("orders"),
							Owner:                  "orders-owner",
							OwnerPassword:          "owner-pass",
							DataChecksums:          true,
							Encoding:               stringPtr("UTF8"),
							LocaleCollate:          "C",
							LocaleCtype:            "C",
							PostInitSql:            []string{"CREATE EXTENSION IF NOT EXISTS pg_stat_statements"},
							PostInitApplicationSql: []string{"CREATE EXTENSION IF NOT EXISTS postgis"},
							Import: &KubernetesPostgresImport{
								Type:                  "microservice",
								SourceExternalCluster: "legacy-rds",
								Databases:             []string{"orders"},
								SchemaOnly:            true,
							},
						},
					},
				},
				ExternalClusters: []*KubernetesPostgresExternalCluster{
					{
						Name: "legacy-rds",
						ConnectionParameters: map[string]string{
							"host":    "legacy.abcdef.us-east-1.rds.amazonaws.com",
							"user":    "postgres",
							"dbname":  "orders",
							"sslmode": "require",
						},
						Password: "legacy-pass",
					},
				},
				Superuser: &KubernetesPostgresSuperuser{
					Enabled:  true,
					Password: "super-pass",
				},
				Roles: []*KubernetesPostgresRole{
					{
						Name:            "app-writer",
						Comment:         "application write path",
						Ensure:          stringPtr("present"),
						Password:        "writer-pass",
						Login:           true,
						Createdb:        true,
						InRoles:         []string{"pg_monitor"},
						ConnectionLimit: int64Ptr(100),
					},
					{
						Name:            "replicator",
						DisablePassword: true,
						Replication:     true,
						Bypassrls:       true,
						ConnectionLimit: int64Ptr(-1),
					},
				},
				Backup: &KubernetesPostgresBackup{
					ObjectStore: &KubernetesPostgresObjectStore{
						DestinationPath: "s3://pg-backups/orders",
						Backend: &KubernetesPostgresObjectStore_S3{
							S3: &KubernetesPostgresS3ObjectStore{
								Region:  "us-east-1",
								Keyless: true,
							},
						},
						Wal: &KubernetesPostgresWalTuning{
							Compression: "zstd",
							MaxParallel: int32Ptr(4),
						},
						Data: &KubernetesPostgresDataTuning{
							Compression:         "gzip",
							Jobs:                int32Ptr(2),
							ImmediateCheckpoint: true,
						},
					},
					RetentionPolicy: "30d",
					Schedules: []*KubernetesPostgresBackupSchedule{
						{Name: "daily", Schedule: "0 0 2 * * *", Immediate: true, Target: stringPtr("prefer-standby")},
						{Name: "weekly-full", Schedule: "0 0 5 * * 0", Target: stringPtr("primary")},
					},
				},
				WorkloadIdentity: &kubernetes.KubernetesWorkloadIdentity{
					Provider: &kubernetes.KubernetesWorkloadIdentity_Eks{
						Eks: &kubernetes.KubernetesWorkloadIdentityEksIrsa{
							RoleArn: literal("arn:aws:iam::123456789012:role/pg-backups"),
						},
					},
				},
				Certificates: &KubernetesPostgresCertificates{
					ServerTlsSecret:   literal("pg-server-cert"),
					ServerCaSecret:    "pg-server-ca",
					ServerAltDnsNames: []string{"pg.example.com"},
				},
				Monitoring: &KubernetesPostgresMonitoring{
					TlsEnabled:            true,
					DisableDefaultQueries: false,
				},
				Scheduling: &KubernetesPostgresScheduling{
					AntiAffinityType: stringPtr("required"),
					TopologyKey:      "topology.kubernetes.io/zone",
					NodeSelector:     map[string]string{"workload": "databases"},
					Tolerations: []*kubernetes.WorkloadToleration{
						{Key: "dedicated", Operator: "Equal", Value: "databases", Effect: "NoSchedule"},
					},
					PriorityClassName: "database-critical",
				},
				UpdateStrategy: &KubernetesPostgresUpdateStrategy{
					PrimaryUpdateStrategy: stringPtr("supervised"),
					PrimaryUpdateMethod:   stringPtr("switchover"),
				},
				EnablePdb:        boolPtr(true),
				ImagePullSecrets: []string{"registry-pull"},
			}
			gomega.Expect(protovalidate.Validate(input)).To(gomega.BeNil())
		})
	})

	ginkgo.Describe("When invalid input is passed", func() {
		ginkgo.It("missing namespace should fail (required)", func() {
			input.Spec.Namespace = nil
			gomega.Expect(protovalidate.Validate(input)).ToNot(gomega.BeNil())
		})

		ginkgo.It("zero instances should fail (gte 1)", func() {
			input.Spec.Instances = int32Ptr(0)
			gomega.Expect(protovalidate.Validate(input)).ToNot(gomega.BeNil())
		})

		ginkgo.It("missing storage should fail (required)", func() {
			input.Spec.Storage = nil
			gomega.Expect(protovalidate.Validate(input)).ToNot(gomega.BeNil())
		})

		ginkgo.It("empty storage size should fail (required)", func() {
			input.Spec.Storage.Size = ""
			gomega.Expect(protovalidate.Validate(input)).ToNot(gomega.BeNil())
		})

		ginkgo.It("storage size '10GB' should fail (size_quantity — GB is not a Kubernetes suffix)", func() {
			input.Spec.Storage.Size = "10GB"
			gomega.Expect(protovalidate.Validate(input)).ToNot(gomega.BeNil())
		})

		ginkgo.It("storage size 'abc' should fail (size_quantity)", func() {
			input.Spec.Storage.Size = "abc"
			gomega.Expect(protovalidate.Validate(input)).ToNot(gomega.BeNil())
		})

		ginkgo.It("malformed WAL storage size should fail (size_quantity)", func() {
			input.Spec.WalStorage = &KubernetesPostgresStorage{Size: "10GB"}
			gomega.Expect(protovalidate.Validate(input)).ToNot(gomega.BeNil())
		})

		ginkgo.It("duplicate external cluster names should fail (unique_names)", func() {
			input.Spec.ExternalClusters = []*KubernetesPostgresExternalCluster{
				{Name: "source"},
				{Name: "source"},
			}
			gomega.Expect(protovalidate.Validate(input)).ToNot(gomega.BeNil())
		})

		ginkgo.It("external cluster without a name should fail (required)", func() {
			input.Spec.ExternalClusters = []*KubernetesPostgresExternalCluster{
				{ConnectionParameters: map[string]string{"host": "example.com"}},
			}
			gomega.Expect(protovalidate.Validate(input)).ToNot(gomega.BeNil())
		})

		ginkgo.It("unknown synchronous method should fail (method_enum)", func() {
			input.Spec.Postgresql = &KubernetesPostgresServerConfig{
				Synchronous: &KubernetesPostgresSynchronousReplication{
					Method: stringPtr("quorum"),
					Number: 1,
				},
			}
			gomega.Expect(protovalidate.Validate(input)).ToNot(gomega.BeNil())
		})

		ginkgo.It("zero synchronous number should fail (gte 1)", func() {
			input.Spec.Postgresql = &KubernetesPostgresServerConfig{
				Synchronous: &KubernetesPostgresSynchronousReplication{Number: 0},
			}
			gomega.Expect(protovalidate.Validate(input)).ToNot(gomega.BeNil())
		})

		ginkgo.It("unknown data_durability should fail (durability_enum)", func() {
			input.Spec.Postgresql = &KubernetesPostgresServerConfig{
				Synchronous: &KubernetesPostgresSynchronousReplication{
					Number:         1,
					DataDurability: stringPtr("relaxed"),
				},
			}
			gomega.Expect(protovalidate.Validate(input)).ToNot(gomega.BeNil())
		})

		ginkgo.It("bootstrap without a method arm should fail (oneof required)", func() {
			input.Spec.Bootstrap = &KubernetesPostgresBootstrap{}
			gomega.Expect(protovalidate.Validate(input)).ToNot(gomega.BeNil())
		})

		ginkgo.It("unknown import type should fail (type_enum)", func() {
			input.Spec.Bootstrap = &KubernetesPostgresBootstrap{
				Method: &KubernetesPostgresBootstrap_Initdb{
					Initdb: &KubernetesPostgresBootstrapInitDb{
						Import: &KubernetesPostgresImport{
							Type:                  "logical",
							SourceExternalCluster: "legacy-rds",
							Databases:             []string{"orders"},
						},
					},
				},
			}
			gomega.Expect(protovalidate.Validate(input)).ToNot(gomega.BeNil())
		})

		ginkgo.It("microservice import with two databases should fail (microservice_single_db)", func() {
			input.Spec.Bootstrap = &KubernetesPostgresBootstrap{
				Method: &KubernetesPostgresBootstrap_Initdb{
					Initdb: &KubernetesPostgresBootstrapInitDb{
						Import: &KubernetesPostgresImport{
							Type:                  "microservice",
							SourceExternalCluster: "legacy-rds",
							Databases:             []string{"orders", "billing"},
						},
					},
				},
			}
			gomega.Expect(protovalidate.Validate(input)).ToNot(gomega.BeNil())
		})

		ginkgo.It("microservice import with roles should fail (roles_monolith_only)", func() {
			input.Spec.Bootstrap = &KubernetesPostgresBootstrap{
				Method: &KubernetesPostgresBootstrap_Initdb{
					Initdb: &KubernetesPostgresBootstrapInitDb{
						Import: &KubernetesPostgresImport{
							Type:                  "microservice",
							SourceExternalCluster: "legacy-rds",
							Databases:             []string{"orders"},
							Roles:                 []string{"app-reader"},
						},
					},
				},
			}
			gomega.Expect(protovalidate.Validate(input)).ToNot(gomega.BeNil())
		})

		ginkgo.It("import without databases should fail (min_items 1)", func() {
			input.Spec.Bootstrap = &KubernetesPostgresBootstrap{
				Method: &KubernetesPostgresBootstrap_Initdb{
					Initdb: &KubernetesPostgresBootstrapInitDb{
						Import: &KubernetesPostgresImport{
							Type:                  "monolith",
							SourceExternalCluster: "legacy-rds",
						},
					},
				},
			}
			gomega.Expect(protovalidate.Validate(input)).ToNot(gomega.BeNil())
		})

		ginkgo.It("import without source_external_cluster should fail (required)", func() {
			input.Spec.Bootstrap = &KubernetesPostgresBootstrap{
				Method: &KubernetesPostgresBootstrap_Initdb{
					Initdb: &KubernetesPostgresBootstrapInitDb{
						Import: &KubernetesPostgresImport{
							Type:      "microservice",
							Databases: []string{"orders"},
						},
					},
				},
			}
			gomega.Expect(protovalidate.Validate(input)).ToNot(gomega.BeNil())
		})

		ginkgo.It("recovery without object_store should fail (required)", func() {
			input.Spec.Bootstrap = &KubernetesPostgresBootstrap{
				Method: &KubernetesPostgresBootstrap_Recovery{
					Recovery: &KubernetesPostgresBootstrapRecovery{
						SourceServerName: "orders-db",
					},
				},
			}
			gomega.Expect(protovalidate.Validate(input)).ToNot(gomega.BeNil())
		})

		ginkgo.It("recovery without source_server_name should fail (required)", func() {
			input.Spec.Bootstrap = &KubernetesPostgresBootstrap{
				Method: &KubernetesPostgresBootstrap_Recovery{
					Recovery: &KubernetesPostgresBootstrapRecovery{
						ObjectStore: s3KeylessStore("s3://pg-backups/source"),
					},
				},
			}
			gomega.Expect(protovalidate.Validate(input)).ToNot(gomega.BeNil())
		})

		ginkgo.It("two recovery target selectors should fail (single_selector)", func() {
			input.Spec.Bootstrap = &KubernetesPostgresBootstrap{
				Method: &KubernetesPostgresBootstrap_Recovery{
					Recovery: &KubernetesPostgresBootstrapRecovery{
						ObjectStore:      s3KeylessStore("s3://pg-backups/source"),
						SourceServerName: "orders-db",
						RecoveryTarget: &KubernetesPostgresRecoveryTarget{
							TargetTime: "2026-07-20T06:00:00Z",
							TargetLsn:  "0/3000000",
						},
					},
				},
			}
			gomega.Expect(protovalidate.Validate(input)).ToNot(gomega.BeNil())
		})

		ginkgo.It("target_name combined with target_immediate should fail (single_selector)", func() {
			input.Spec.Bootstrap = &KubernetesPostgresBootstrap{
				Method: &KubernetesPostgresBootstrap_Recovery{
					Recovery: &KubernetesPostgresBootstrapRecovery{
						ObjectStore:      s3KeylessStore("s3://pg-backups/source"),
						SourceServerName: "orders-db",
						RecoveryTarget: &KubernetesPostgresRecoveryTarget{
							TargetName:      "before-migration",
							TargetImmediate: true,
						},
					},
				},
			}
			gomega.Expect(protovalidate.Validate(input)).ToNot(gomega.BeNil())
		})

		ginkgo.It("pg_basebackup without a source should fail (required)", func() {
			input.Spec.Bootstrap = &KubernetesPostgresBootstrap{
				Method: &KubernetesPostgresBootstrap_PgBasebackup{
					PgBasebackup: &KubernetesPostgresBootstrapPgBaseBackup{},
				},
			}
			gomega.Expect(protovalidate.Validate(input)).ToNot(gomega.BeNil())
		})

		ginkgo.It("superuser password without enabled should fail (password_requires_enabled)", func() {
			input.Spec.Superuser = &KubernetesPostgresSuperuser{Password: "s3cr3t"}
			gomega.Expect(protovalidate.Validate(input)).ToNot(gomega.BeNil())
		})

		ginkgo.It("duplicate role names should fail (unique_names)", func() {
			input.Spec.Roles = []*KubernetesPostgresRole{
				{Name: "app-writer"},
				{Name: "app-writer"},
			}
			gomega.Expect(protovalidate.Validate(input)).ToNot(gomega.BeNil())
		})

		ginkgo.It("role without a name should fail (required)", func() {
			input.Spec.Roles = []*KubernetesPostgresRole{{Login: true}}
			gomega.Expect(protovalidate.Validate(input)).ToNot(gomega.BeNil())
		})

		ginkgo.It("role with both password and disable_password should fail (password_xor_disable)", func() {
			input.Spec.Roles = []*KubernetesPostgresRole{
				{Name: "app-writer", Password: "writer-pass", DisablePassword: true},
			}
			gomega.Expect(protovalidate.Validate(input)).ToNot(gomega.BeNil())
		})

		ginkgo.It("unknown role ensure should fail (ensure_enum)", func() {
			input.Spec.Roles = []*KubernetesPostgresRole{
				{Name: "app-writer", Ensure: stringPtr("maybe")},
			}
			gomega.Expect(protovalidate.Validate(input)).ToNot(gomega.BeNil())
		})

		ginkgo.It("connection_limit below -1 should fail (gte -1)", func() {
			input.Spec.Roles = []*KubernetesPostgresRole{
				{Name: "app-writer", ConnectionLimit: int64Ptr(-2)},
			}
			gomega.Expect(protovalidate.Validate(input)).ToNot(gomega.BeNil())
		})

		ginkgo.It("backup without object_store should fail (required)", func() {
			input.Spec.Backup = &KubernetesPostgresBackup{RetentionPolicy: "30d"}
			gomega.Expect(protovalidate.Validate(input)).ToNot(gomega.BeNil())
		})

		ginkgo.It("retention '0d' should fail (retention_format — zero is not a retention)", func() {
			backup := validBackup()
			backup.RetentionPolicy = "0d"
			input.Spec.Backup = backup
			gomega.Expect(protovalidate.Validate(input)).ToNot(gomega.BeNil())
		})

		ginkgo.It("retention '30x' should fail (retention_format — unknown unit)", func() {
			backup := validBackup()
			backup.RetentionPolicy = "30x"
			input.Spec.Backup = backup
			gomega.Expect(protovalidate.Validate(input)).ToNot(gomega.BeNil())
		})

		ginkgo.It("retention 'd30' should fail (retention_format — unit before number)", func() {
			backup := validBackup()
			backup.RetentionPolicy = "d30"
			input.Spec.Backup = backup
			gomega.Expect(protovalidate.Validate(input)).ToNot(gomega.BeNil())
		})

		ginkgo.It("duplicate backup schedule names should fail (unique_names)", func() {
			backup := validBackup()
			backup.Schedules = []*KubernetesPostgresBackupSchedule{
				{Name: "daily", Schedule: "0 0 2 * * *"},
				{Name: "daily", Schedule: "0 0 4 * * *"},
			}
			input.Spec.Backup = backup
			gomega.Expect(protovalidate.Validate(input)).ToNot(gomega.BeNil())
		})

		ginkgo.It("schedule without a name should fail (required)", func() {
			backup := validBackup()
			backup.Schedules = []*KubernetesPostgresBackupSchedule{
				{Schedule: "0 0 2 * * *"},
			}
			input.Spec.Backup = backup
			gomega.Expect(protovalidate.Validate(input)).ToNot(gomega.BeNil())
		})

		ginkgo.It("uppercase schedule name should fail (name_format — DNS label)", func() {
			backup := validBackup()
			backup.Schedules = []*KubernetesPostgresBackupSchedule{
				{Name: "Daily", Schedule: "0 0 2 * * *"},
			}
			input.Spec.Backup = backup
			gomega.Expect(protovalidate.Validate(input)).ToNot(gomega.BeNil())
		})

		ginkgo.It("schedule without a cron expression should fail (required)", func() {
			backup := validBackup()
			backup.Schedules = []*KubernetesPostgresBackupSchedule{
				{Name: "daily"},
			}
			input.Spec.Backup = backup
			gomega.Expect(protovalidate.Validate(input)).ToNot(gomega.BeNil())
		})

		ginkgo.It("five-field Kubernetes cron should fail (cron_six_fields)", func() {
			backup := validBackup()
			backup.Schedules = []*KubernetesPostgresBackupSchedule{
				{Name: "daily", Schedule: "0 2 * * *"},
			}
			input.Spec.Backup = backup
			gomega.Expect(protovalidate.Validate(input)).ToNot(gomega.BeNil())
		})

		ginkgo.It("unknown backup target should fail (target_enum)", func() {
			backup := validBackup()
			backup.Schedules = []*KubernetesPostgresBackupSchedule{
				{Name: "daily", Schedule: "0 0 2 * * *", Target: stringPtr("standby")},
			}
			input.Spec.Backup = backup
			gomega.Expect(protovalidate.Validate(input)).ToNot(gomega.BeNil())
		})

		ginkgo.It("object store without destination_path should fail (required)", func() {
			backup := validBackup()
			backup.ObjectStore.DestinationPath = ""
			input.Spec.Backup = backup
			gomega.Expect(protovalidate.Validate(input)).ToNot(gomega.BeNil())
		})

		ginkgo.It("object store without a backend arm should fail (oneof required)", func() {
			input.Spec.Backup = &KubernetesPostgresBackup{
				ObjectStore: &KubernetesPostgresObjectStore{
					DestinationPath: "s3://pg-backups/main",
				},
			}
			gomega.Expect(protovalidate.Validate(input)).ToNot(gomega.BeNil())
		})

		ginkgo.It("s3 backend with a gs:// path should fail (s3_path_scheme)", func() {
			backup := validBackup()
			backup.ObjectStore.DestinationPath = "gs://pg-backups/main"
			input.Spec.Backup = backup
			gomega.Expect(protovalidate.Validate(input)).ToNot(gomega.BeNil())
		})

		ginkgo.It("gcs backend with an s3:// path should fail (gcs_path_scheme)", func() {
			input.Spec.Backup = &KubernetesPostgresBackup{
				ObjectStore: gcsStore("s3://pg-backups/main", &KubernetesPostgresGcsObjectStore{Keyless: true}),
			}
			gomega.Expect(protovalidate.Validate(input)).ToNot(gomega.BeNil())
		})

		ginkgo.It("azure backend with an s3:// path should fail (azure_path_scheme)", func() {
			input.Spec.Backup = &KubernetesPostgresBackup{
				ObjectStore: azureStore("s3://pg-backups/main", &KubernetesPostgresAzureBlobObjectStore{
					Keyless:        true,
					StorageAccount: "myaccount",
				}),
			}
			gomega.Expect(protovalidate.Validate(input)).ToNot(gomega.BeNil())
		})

		ginkgo.It("r2 backend with a gs:// path should fail (r2_path_scheme)", func() {
			input.Spec.Backup = &KubernetesPostgresBackup{ObjectStore: r2Store("gs://pg-archive/main")}
			gomega.Expect(protovalidate.Validate(input)).ToNot(gomega.BeNil())
		})

		ginkgo.It("r2 without an account_id should fail (required)", func() {
			store := r2Store("s3://pg-archive/main")
			store.GetR2().AccountId = nil
			input.Spec.Backup = &KubernetesPostgresBackup{ObjectStore: store}
			gomega.Expect(protovalidate.Validate(input)).ToNot(gomega.BeNil())
		})

		ginkgo.It("r2 with a malformed literal account_id should fail (account_id_format)", func() {
			store := r2Store("s3://pg-archive/main")
			store.GetR2().AccountId = literal("not-an-account-id")
			input.Spec.Backup = &KubernetesPostgresBackup{ObjectStore: store}
			gomega.Expect(protovalidate.Validate(input)).ToNot(gomega.BeNil())
		})

		ginkgo.It("r2 with an unknown literal jurisdiction should fail (jurisdiction_valid)", func() {
			store := r2Store("s3://pg-archive/main")
			store.GetR2().Jurisdiction = literal("europe")
			input.Spec.Backup = &KubernetesPostgresBackup{ObjectStore: store}
			gomega.Expect(protovalidate.Validate(input)).ToNot(gomega.BeNil())
		})

		ginkgo.It("r2 without credentials should fail (required — R2 has no keyless posture)", func() {
			store := r2Store("s3://pg-archive/main")
			store.GetR2().Credentials = nil
			input.Spec.Backup = &KubernetesPostgresBackup{ObjectStore: store}
			gomega.Expect(protovalidate.Validate(input)).ToNot(gomega.BeNil())
		})

		ginkgo.It("r2 credentials missing the secret access key should fail (required)", func() {
			store := r2Store("s3://pg-archive/main")
			store.GetR2().Credentials.SecretAccessKey = nil
			input.Spec.Backup = &KubernetesPostgresBackup{ObjectStore: store}
			gomega.Expect(protovalidate.Validate(input)).ToNot(gomega.BeNil())
		})

		ginkgo.It("recovery from r2 with a malformed literal account_id should fail (account_id_format on the recovery store)", func() {
			store := r2Store("s3://pg-archive/source")
			store.GetR2().AccountId = literal("short")
			input.Spec.Bootstrap = &KubernetesPostgresBootstrap{
				Method: &KubernetesPostgresBootstrap_Recovery{
					Recovery: &KubernetesPostgresBootstrapRecovery{ObjectStore: store, SourceServerName: "orders-db"},
				},
			}
			gomega.Expect(protovalidate.Validate(input)).ToNot(gomega.BeNil())
		})

		ginkgo.It("s3 with both keyless and access keys should fail (keyless_xor_keys)", func() {
			backup := validBackup()
			backup.ObjectStore.GetS3().AccessKeys = &KubernetesPostgresS3AccessKeys{
				AccessKeyId:     "AKIAEXAMPLE",
				SecretAccessKey: "secret",
			}
			input.Spec.Backup = backup
			gomega.Expect(protovalidate.Validate(input)).ToNot(gomega.BeNil())
		})

		ginkgo.It("s3 with neither keyless nor access keys should fail (keyless_xor_keys)", func() {
			backup := validBackup()
			backup.ObjectStore.GetS3().Keyless = false
			input.Spec.Backup = backup
			gomega.Expect(protovalidate.Validate(input)).ToNot(gomega.BeNil())
		})

		ginkgo.It("non-http endpoint_url should fail (endpoint_format)", func() {
			backup := validBackup()
			s3 := backup.ObjectStore.GetS3()
			s3.Keyless = false
			s3.EndpointUrl = "minio.minio-system.svc:9000"
			s3.AccessKeys = &KubernetesPostgresS3AccessKeys{
				AccessKeyId:     "minio",
				SecretAccessKey: "minio123",
			}
			input.Spec.Backup = backup
			gomega.Expect(protovalidate.Validate(input)).ToNot(gomega.BeNil())
		})

		ginkgo.It("S3-compatible endpoint with the keyless posture should fail (compatible_needs_keys)", func() {
			backup := validBackup()
			backup.ObjectStore.GetS3().EndpointUrl = "http://minio.minio-system.svc:9000"
			input.Spec.Backup = backup
			gomega.Expect(protovalidate.Validate(input)).ToNot(gomega.BeNil())
		})

		ginkgo.It("access keys without access_key_id should fail (required)", func() {
			backup := validBackup()
			s3 := backup.ObjectStore.GetS3()
			s3.Keyless = false
			s3.AccessKeys = &KubernetesPostgresS3AccessKeys{SecretAccessKey: "secret"}
			input.Spec.Backup = backup
			gomega.Expect(protovalidate.Validate(input)).ToNot(gomega.BeNil())
		})

		ginkgo.It("access keys without secret_access_key should fail (required)", func() {
			backup := validBackup()
			s3 := backup.ObjectStore.GetS3()
			s3.Keyless = false
			s3.AccessKeys = &KubernetesPostgresS3AccessKeys{AccessKeyId: "AKIAEXAMPLE"}
			input.Spec.Backup = backup
			gomega.Expect(protovalidate.Validate(input)).ToNot(gomega.BeNil())
		})

		ginkgo.It("gcs with both keyless and a key should fail (keyless_xor_key)", func() {
			input.Spec.Backup = &KubernetesPostgresBackup{
				ObjectStore: gcsStore("gs://pg-backups/main", &KubernetesPostgresGcsObjectStore{
					Keyless:               true,
					ServiceAccountKeyJson: `{"type":"service_account"}`,
				}),
			}
			gomega.Expect(protovalidate.Validate(input)).ToNot(gomega.BeNil())
		})

		ginkgo.It("gcs with neither keyless nor a key should fail (keyless_xor_key)", func() {
			input.Spec.Backup = &KubernetesPostgresBackup{
				ObjectStore: gcsStore("gs://pg-backups/main", &KubernetesPostgresGcsObjectStore{}),
			}
			gomega.Expect(protovalidate.Validate(input)).ToNot(gomega.BeNil())
		})

		ginkgo.It("azure with no credential posture should fail (single_posture)", func() {
			input.Spec.Backup = &KubernetesPostgresBackup{
				ObjectStore: azureStore("https://myaccount.blob.core.windows.net/backups/pg", &KubernetesPostgresAzureBlobObjectStore{
					StorageAccount: "myaccount",
				}),
			}
			gomega.Expect(protovalidate.Validate(input)).ToNot(gomega.BeNil())
		})

		ginkgo.It("azure with two credential postures should fail (single_posture)", func() {
			input.Spec.Backup = &KubernetesPostgresBackup{
				ObjectStore: azureStore("https://myaccount.blob.core.windows.net/backups/pg", &KubernetesPostgresAzureBlobObjectStore{
					Keyless:          true,
					StorageAccount:   "myaccount",
					ConnectionString: "DefaultEndpointsProtocol=https;AccountName=myaccount;AccountKey=key==",
				}),
			}
			gomega.Expect(protovalidate.Validate(input)).ToNot(gomega.BeNil())
		})

		ginkgo.It("azure storage_key without storage_account should fail (key_needs_account)", func() {
			input.Spec.Backup = &KubernetesPostgresBackup{
				ObjectStore: azureStore("https://myaccount.blob.core.windows.net/backups/pg", &KubernetesPostgresAzureBlobObjectStore{
					StorageKey: "base64key==",
				}),
			}
			gomega.Expect(protovalidate.Validate(input)).ToNot(gomega.BeNil())
		})

		ginkgo.It("azure keyless without storage_account should fail (keyless_needs_account)", func() {
			input.Spec.Backup = &KubernetesPostgresBackup{
				ObjectStore: azureStore("https://myaccount.blob.core.windows.net/backups/pg", &KubernetesPostgresAzureBlobObjectStore{
					Keyless: true,
				}),
			}
			gomega.Expect(protovalidate.Validate(input)).ToNot(gomega.BeNil())
		})

		ginkgo.It("unknown WAL compression should fail (compression_enum)", func() {
			backup := validBackup()
			backup.ObjectStore.Wal = &KubernetesPostgresWalTuning{Compression: "brotli"}
			input.Spec.Backup = backup
			gomega.Expect(protovalidate.Validate(input)).ToNot(gomega.BeNil())
		})

		ginkgo.It("zero WAL max_parallel should fail (gte 1)", func() {
			backup := validBackup()
			backup.ObjectStore.Wal = &KubernetesPostgresWalTuning{MaxParallel: int32Ptr(0)}
			input.Spec.Backup = backup
			gomega.Expect(protovalidate.Validate(input)).ToNot(gomega.BeNil())
		})

		ginkgo.It("zstd data compression should fail (compression_enum — data supports gzip/bzip2/lz4/snappy only)", func() {
			backup := validBackup()
			backup.ObjectStore.Data = &KubernetesPostgresDataTuning{Compression: "zstd"}
			input.Spec.Backup = backup
			gomega.Expect(protovalidate.Validate(input)).ToNot(gomega.BeNil())
		})

		ginkgo.It("zero data jobs should fail (gte 1)", func() {
			backup := validBackup()
			backup.ObjectStore.Data = &KubernetesPostgresDataTuning{Jobs: int32Ptr(0)}
			input.Spec.Backup = backup
			gomega.Expect(protovalidate.Validate(input)).ToNot(gomega.BeNil())
		})

		ginkgo.It("server TLS secret without its CA secret should fail (tls_needs_ca)", func() {
			input.Spec.Certificates = &KubernetesPostgresCertificates{
				ServerTlsSecret: literal("pg-server-cert"),
			}
			gomega.Expect(protovalidate.Validate(input)).ToNot(gomega.BeNil())
		})

		ginkgo.It("unknown anti_affinity_type should fail (anti_affinity_enum)", func() {
			input.Spec.Scheduling = &KubernetesPostgresScheduling{
				AntiAffinityType: stringPtr("hard"),
			}
			gomega.Expect(protovalidate.Validate(input)).ToNot(gomega.BeNil())
		})

		ginkgo.It("unknown primary_update_strategy should fail (strategy_enum)", func() {
			input.Spec.UpdateStrategy = &KubernetesPostgresUpdateStrategy{
				PrimaryUpdateStrategy: stringPtr("automatic"),
			}
			gomega.Expect(protovalidate.Validate(input)).ToNot(gomega.BeNil())
		})

		ginkgo.It("unknown primary_update_method should fail (method_enum)", func() {
			input.Spec.UpdateStrategy = &KubernetesPostgresUpdateStrategy{
				PrimaryUpdateMethod: stringPtr("recreate"),
			}
			gomega.Expect(protovalidate.Validate(input)).ToNot(gomega.BeNil())
		})
	})
})
