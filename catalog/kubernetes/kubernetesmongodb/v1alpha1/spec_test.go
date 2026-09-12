package kubernetesmongodbv1alpha1

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

func TestKubernetesMongodb(t *testing.T) {
	gomega.RegisterFailHandler(ginkgo.Fail)
	ginkgo.RunSpecs(t, "KubernetesMongodb Suite")
}

func int32Ptr(i int32) *int32    { return &i }
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

// replicaSet returns a minimal valid replica set — the base every
// topology test mutates from.
func replicaSet(name string) *KubernetesMongodbReplicaSet {
	return &KubernetesMongodbReplicaSet{
		Name:    name,
		Storage: &KubernetesMongodbStorage{Size: "10Gi"},
	}
}

// s3Storage returns a minimal valid S3 backup storage (keyless posture —
// real AWS S3 without an endpoint override).
func s3Storage(name string) *KubernetesMongodbBackupStorage {
	return &KubernetesMongodbBackupStorage{
		Name: name,
		Backend: &KubernetesMongodbBackupStorage_S3{
			S3: &KubernetesMongodbS3Storage{
				Bucket: "mongo-backups",
				Region: "us-east-1",
			},
		},
	}
}

// gcsStorage returns a minimal valid GCS backup storage: a literal
// service-account key (GCS has no keyless arm — the credentials message is
// required).
func gcsStorage(name string) *KubernetesMongodbBackupStorage {
	return &KubernetesMongodbBackupStorage{
		Name: name,
		Backend: &KubernetesMongodbBackupStorage_Gcs{
			Gcs: &KubernetesMongodbGcsStorage{
				Bucket: "mongo-backups",
				Credentials: &KubernetesMongodbGcsCredentials{
					Source: &KubernetesMongodbGcsCredentials_ServiceAccountKey{
						ServiceAccountKey: literal(`{"type":"service_account","client_email":"mongo-backup@my-project.iam.gserviceaccount.com","private_key":"-----BEGIN PRIVATE KEY-----\nabc\n-----END PRIVATE KEY-----\n"}`),
					},
				},
			},
		},
	}
}

// r2Storage returns a Cloudflare R2 backup storage with every reference in
// its default-kind form (the bucket for name, account, and jurisdiction; the
// account API token for the key pair) — the composed shape a chart or the
// console produces.
func r2Storage(name string) *KubernetesMongodbBackupStorage {
	return &KubernetesMongodbBackupStorage{
		Name: name,
		Backend: &KubernetesMongodbBackupStorage_R2{
			R2: &KubernetesMongodbR2Storage{
				Bucket:       valueFrom(cloudresourcekind.CloudResourceKind_CloudflareR2Bucket, "mongo-archive", "status.outputs.bucket_name"),
				Prefix:       "prod-mongo",
				AccountId:    valueFrom(cloudresourcekind.CloudResourceKind_CloudflareR2Bucket, "mongo-archive", "status.outputs.account_id"),
				Jurisdiction: valueFrom(cloudresourcekind.CloudResourceKind_CloudflareR2Bucket, "mongo-archive", "status.outputs.jurisdiction"),
				Credentials: &KubernetesMongodbR2Credentials{
					AccessKeyId:     valueFrom(cloudresourcekind.CloudResourceKind_CloudflareAccountApiToken, "mongo-archive-writer", "status.outputs.r2_access_key_id"),
					SecretAccessKey: valueFrom(cloudresourcekind.CloudResourceKind_CloudflareAccountApiToken, "mongo-archive-writer", "status.outputs.r2_secret_access_key"),
				},
			},
		},
	}
}

// validBackup returns a minimal valid backup block (one S3 storage, no
// tasks) for tests that mutate one backup rule at a time.
func validBackup() *KubernetesMongodbBackup {
	return &KubernetesMongodbBackup{
		Storages: []*KubernetesMongodbBackupStorage{s3Storage("primary")},
	}
}

// validSharding returns a minimal valid sharding block with the required
// config-server and mongos declarations.
func validSharding() *KubernetesMongodbSharding {
	return &KubernetesMongodbSharding{
		Enabled: true,
		ConfigServer: &KubernetesMongodbConfigServer{
			Storage: &KubernetesMongodbStorage{Size: "2Gi"},
		},
		Mongos: &KubernetesMongodbMongos{},
	}
}

var _ = ginkgo.Describe("KubernetesMongodb Validation Tests", func() {
	var input *KubernetesMongodb

	ginkgo.BeforeEach(func() {
		input = &KubernetesMongodb{
			ApiVersion: "kubernetes.planton.dev/v1alpha1",
			Kind:       "KubernetesMongodb",
			Metadata: &shared.CloudResourceMetadata{
				Name: "test-mongodb",
			},
			Spec: &KubernetesMongodbSpec{
				Namespace:   literal("databases"),
				ReplicaSets: []*KubernetesMongodbReplicaSet{replicaSet("rs0")},
			},
		}
	})

	ginkgo.Describe("When valid input is passed", func() {
		ginkgo.It("minimal spec should not return a validation error (every optional block omitted)", func() {
			gomega.Expect(protovalidate.Validate(input)).To(gomega.BeNil())
		})

		ginkgo.It("namespace as a reference should be valid", func() {
			input.Spec.Namespace = valueFrom(cloudresourcekind.CloudResourceKind_KubernetesNamespace, "databases", "spec.name")
			gomega.Expect(protovalidate.Validate(input)).To(gomega.BeNil())
		})

		ginkgo.It("replica set of three members with an arbiter should be valid", func() {
			rs := replicaSet("rs0")
			rs.Size = int32Ptr(3)
			rs.Arbiter = &KubernetesMongodbArbiter{Enabled: true, Size: int32Ptr(1)}
			input.Spec.ReplicaSets = []*KubernetesMongodbReplicaSet{rs}
			gomega.Expect(protovalidate.Validate(input)).To(gomega.BeNil())
		})

		ginkgo.It("storage size '500Mi' should be valid (size_quantity)", func() {
			input.Spec.ReplicaSets[0].Storage.Size = "500Mi"
			gomega.Expect(protovalidate.Validate(input)).To(gomega.BeNil())
		})

		ginkgo.It("fractional storage size '1.5Ti' should be valid (size_quantity)", func() {
			input.Spec.ReplicaSets[0].Storage.Size = "1.5Ti"
			gomega.Expect(protovalidate.Validate(input)).To(gomega.BeNil())
		})

		ginkgo.It("per-member exposure of each allowed type should be valid (expose.type_enum)", func() {
			for _, exposeType := range []string{"ClusterIP", "NodePort", "LoadBalancer"} {
				rs := replicaSet("rs0")
				rs.Expose = &KubernetesMongodbExpose{
					Enabled: true,
					Type:    stringPtr(exposeType),
				}
				input.Spec.ReplicaSets = []*KubernetesMongodbReplicaSet{rs}
				gomega.Expect(protovalidate.Validate(input)).To(gomega.BeNil())
			}
		})

		ginkgo.It("sharding with config_server and mongos should be valid (enabled_needs_topology)", func() {
			input.Spec.ReplicaSets = []*KubernetesMongodbReplicaSet{
				replicaSet("rs0"),
				replicaSet("rs1"),
			}
			input.Spec.Sharding = validSharding()
			gomega.Expect(protovalidate.Validate(input)).To(gomega.BeNil())
		})

		ginkgo.It("sharding block present but disabled with one replica set should be valid", func() {
			input.Spec.Sharding = &KubernetesMongodbSharding{Enabled: false}
			gomega.Expect(protovalidate.Validate(input)).To(gomega.BeNil())
		})

		ginkgo.It("each allowed TLS mode should be valid (mode_enum)", func() {
			for _, mode := range []string{"allowTLS", "preferTLS", "requireTLS"} {
				input.Spec.Tls = &KubernetesMongodbTls{Mode: stringPtr(mode)}
				gomega.Expect(protovalidate.Validate(input)).To(gomega.BeNil())
			}
		})

		ginkgo.It("disabled TLS mode with unsafe.tls should be valid (tls_disabled_or_unsafe)", func() {
			input.Spec.Tls = &KubernetesMongodbTls{Mode: stringPtr("disabled")}
			input.Spec.Unsafe = &KubernetesMongodbUnsafe{Tls: true}
			gomega.Expect(protovalidate.Validate(input)).To(gomega.BeNil())
		})

		ginkgo.It("single-member replica set with unsafe.replset_size should be valid (replset_size_or_unsafe)", func() {
			input.Spec.ReplicaSets[0].Size = int32Ptr(1)
			input.Spec.Unsafe = &KubernetesMongodbUnsafe{ReplsetSize: true}
			gomega.Expect(protovalidate.Validate(input)).To(gomega.BeNil())
		})

		ginkgo.It("single-member config server with unsafe.replset_size should be valid (config_server_size_or_unsafe)", func() {
			input.Spec.ReplicaSets[0].Size = int32Ptr(1)
			sharding := validSharding()
			sharding.ConfigServer.Size = int32Ptr(1)
			input.Spec.Sharding = sharding
			input.Spec.Unsafe = &KubernetesMongodbUnsafe{ReplsetSize: true}
			gomega.Expect(protovalidate.Validate(input)).To(gomega.BeNil())
		})

		ginkgo.It("TLS with a cert-manager issuer should be valid", func() {
			input.Spec.Tls = &KubernetesMongodbTls{
				Mode:                 stringPtr("requireTLS"),
				Issuer:               literal("org-ca-issuer"),
				IssuerKind:           stringPtr("ClusterIssuer"),
				CertValidityDuration: "2160h",
			}
			gomega.Expect(protovalidate.Validate(input)).To(gomega.BeNil())
		})

		ginkgo.It("users with distinct names and roles should be valid (unique_names)", func() {
			input.Spec.Users = []*KubernetesMongodbUser{
				{
					Name:  "app-writer",
					Db:    stringPtr("admin"),
					Roles: []*KubernetesMongodbUserRole{{Name: "readWrite", Db: "orders"}},
				},
				{
					Name:     "app-reader",
					Password: "reader-pass",
					Roles:    []*KubernetesMongodbUserRole{{Name: "read", Db: "orders"}},
				},
			}
			gomega.Expect(protovalidate.Validate(input)).To(gomega.BeNil())
		})

		ginkgo.It("backup to S3 with the keyless posture should be valid", func() {
			input.Spec.Backup = validBackup()
			gomega.Expect(protovalidate.Validate(input)).To(gomega.BeNil())
		})

		ginkgo.It("two backup storages with exactly one main should be valid (storages.single_main)", func() {
			backup := validBackup()
			backup.Storages[0].Main = true
			backup.Storages = append(backup.Storages, s3Storage("secondary"))
			input.Spec.Backup = backup
			gomega.Expect(protovalidate.Validate(input)).To(gomega.BeNil())
		})

		ginkgo.It("backup to an S3-compatible endpoint with access keys should be valid (compatible_needs_keys)", func() {
			backup := validBackup()
			s3 := backup.Storages[0].GetS3()
			s3.EndpointUrl = "http://minio.minio-system.svc:9000"
			s3.InsecureSkipTlsVerify = true
			s3.AccessKeys = &KubernetesMongodbS3AccessKeys{
				AccessKeyId:     "minio",
				SecretAccessKey: "minio123",
			}
			input.Spec.Backup = backup
			gomega.Expect(protovalidate.Validate(input)).To(gomega.BeNil())
		})

		ginkgo.It("backup to R2 composed from the Cloudflare kinds should be valid", func() {
			input.Spec.Backup = &KubernetesMongodbBackup{
				Storages: []*KubernetesMongodbBackupStorage{r2Storage("r2")},
			}
			gomega.Expect(protovalidate.Validate(input)).To(gomega.BeNil())
		})

		ginkgo.It("backup to R2 with a literal bucket, account, jurisdiction, and dashboard-minted key pair should be valid", func() {
			storage := r2Storage("r2")
			storage.GetR2().Bucket = literal("mongo-archive")
			storage.GetR2().AccountId = literal("4793d734c0b8e484dfc37ec392b5fa8a")
			storage.GetR2().Jurisdiction = literal("eu")
			storage.GetR2().Credentials.AccessKeyId = literal("f267e341f3dd4697bd3b9f71dd96247f")
			storage.GetR2().Credentials.SecretAccessKey = literal("ba7816bf8f01cfea414140de5dae2223b00361a396177a9cb410ff61f20015ad")
			input.Spec.Backup = &KubernetesMongodbBackup{Storages: []*KubernetesMongodbBackupStorage{storage}}
			gomega.Expect(protovalidate.Validate(input)).To(gomega.BeNil())
		})

		ginkgo.It("backup to R2 without a jurisdiction should be valid (empty is the default jurisdiction)", func() {
			storage := r2Storage("r2")
			storage.GetR2().Jurisdiction = nil
			input.Spec.Backup = &KubernetesMongodbBackup{Storages: []*KubernetesMongodbBackupStorage{storage}}
			gomega.Expect(protovalidate.Validate(input)).To(gomega.BeNil())
		})

		ginkgo.It("restore from an R2 storage's location with PITR to latest should be valid (the DR shape on R2)", func() {
			input.Spec.SystemUsersSecretName = "source-mongodb-secrets"
			input.Spec.Backup = &KubernetesMongodbBackup{
				Storages: []*KubernetesMongodbBackupStorage{r2Storage("r2")},
				Pitr:     &KubernetesMongodbPitr{Enabled: true},
			}
			input.Spec.Restore = &KubernetesMongodbRestore{
				Source: &KubernetesMongodbRestore_BackupSource{
					BackupSource: &KubernetesMongodbRestoreBackupSource{
						StorageName: "r2",
						Destination: "s3://mongo-archive/prod-mongo/2026-09-10T12:00:00Z",
						Type:        stringPtr("logical"),
					},
				},
				Pitr: &KubernetesMongodbRestorePitr{Type: "latest"},
			}
			gomega.Expect(protovalidate.Validate(input)).To(gomega.BeNil())
		})

		ginkgo.It("backup to GCS with a literal service-account key should be valid", func() {
			input.Spec.Backup = &KubernetesMongodbBackup{
				Storages: []*KubernetesMongodbBackupStorage{gcsStorage("gcs-store")},
			}
			gomega.Expect(protovalidate.Validate(input)).To(gomega.BeNil())
		})

		ginkgo.It("backup to GCS with the key referenced from a GcpServiceAccount should be valid", func() {
			storage := gcsStorage("gcs-store")
			storage.GetGcs().Credentials = &KubernetesMongodbGcsCredentials{
				Source: &KubernetesMongodbGcsCredentials_ServiceAccountKey{
					ServiceAccountKey: valueFrom(cloudresourcekind.CloudResourceKind_GcpServiceAccount, "mongo-backup", "status.outputs.key_base64"),
				},
			}
			input.Spec.Backup = &KubernetesMongodbBackup{Storages: []*KubernetesMongodbBackupStorage{storage}}
			gomega.Expect(protovalidate.Validate(input)).To(gomega.BeNil())
		})

		ginkgo.It("backup to GCS with an existing credentials Secret should be valid", func() {
			storage := gcsStorage("gcs-store")
			storage.GetGcs().Credentials = &KubernetesMongodbGcsCredentials{
				Source: &KubernetesMongodbGcsCredentials_ExistingSecretName{ExistingSecretName: "gcs-backup-credentials"},
			}
			input.Spec.Backup = &KubernetesMongodbBackup{Storages: []*KubernetesMongodbBackupStorage{storage}}
			gomega.Expect(protovalidate.Validate(input)).To(gomega.BeNil())
		})

		ginkgo.It("a bring-your-own system-users Secret should be valid", func() {
			input.Spec.SystemUsersSecretName = "source-mongodb-secrets"
			gomega.Expect(protovalidate.Validate(input)).To(gomega.BeNil())
		})

		ginkgo.It("restore from a same-namespace Backup object should be valid", func() {
			input.Spec.Backup = validBackup()
			input.Spec.Restore = &KubernetesMongodbRestore{
				Source: &KubernetesMongodbRestore_BackupName{BackupName: "nightly-2026-09-09"},
			}
			gomega.Expect(protovalidate.Validate(input)).To(gomega.BeNil())
		})

		ginkgo.It("restore from a declared storage's location with PITR to latest should be valid (the DR shape)", func() {
			input.Spec.SystemUsersSecretName = "source-mongodb-secrets"
			input.Spec.Backup = &KubernetesMongodbBackup{
				Storages: []*KubernetesMongodbBackupStorage{gcsStorage("gcs-store")},
				Pitr:     &KubernetesMongodbPitr{Enabled: true},
			}
			input.Spec.Restore = &KubernetesMongodbRestore{
				Source: &KubernetesMongodbRestore_BackupSource{
					BackupSource: &KubernetesMongodbRestoreBackupSource{
						StorageName: "gcs-store",
						Destination: "gs://mongo-backups/prod/2026-09-09T12:00:00Z",
						Type:        stringPtr("logical"),
					},
				},
				Pitr: &KubernetesMongodbRestorePitr{Type: "latest"},
			}
			gomega.Expect(protovalidate.Validate(input)).To(gomega.BeNil())
		})

		ginkgo.It("restore with PITR to a date and replica-set remapping should be valid", func() {
			input.Spec.Backup = validBackup()
			input.Spec.Restore = &KubernetesMongodbRestore{
				Source: &KubernetesMongodbRestore_BackupSource{
					BackupSource: &KubernetesMongodbRestoreBackupSource{
						StorageName: "primary",
						Destination: "s3://mongo-backups/2026-09-09T12:00:00Z",
					},
				},
				Pitr:             &KubernetesMongodbRestorePitr{Type: "date", Date: "2026-09-09 12:30:00"},
				ReplsetRemapping: map[string]string{"rs0": "rs1"},
			}
			gomega.Expect(protovalidate.Validate(input)).To(gomega.BeNil())
		})

		ginkgo.It("backup to Azure Blob should be valid", func() {
			input.Spec.Backup = &KubernetesMongodbBackup{
				Storages: []*KubernetesMongodbBackupStorage{
					{
						Name: "azure-store",
						Backend: &KubernetesMongodbBackupStorage_Azure{
							Azure: &KubernetesMongodbAzureStorage{
								Container:      "mongo-backups",
								StorageAccount: "myaccount",
								AccessKey:      "base64key==",
							},
						},
					},
				},
			}
			gomega.Expect(protovalidate.Validate(input)).To(gomega.BeNil())
		})

		ginkgo.It("backup tasks with distinct names and five-field crons should be valid", func() {
			backup := validBackup()
			backup.Tasks = []*KubernetesMongodbBackupTask{
				{Name: "daily", Schedule: "0 2 * * *", StorageName: "primary", Type: stringPtr("logical"), Keep: int32Ptr(7)},
				{Name: "weekly-physical", Schedule: "30 4 * * 0", StorageName: "primary", Type: stringPtr("physical"), Suspend: true},
			}
			input.Spec.Backup = backup
			gomega.Expect(protovalidate.Validate(input)).To(gomega.BeNil())
		})

		ginkgo.It("each allowed task compression should be valid (compression_enum)", func() {
			for _, compression := range []string{"gzip", "snappy", "lz4", "pgzip", "zstd", "s2", "none"} {
				backup := validBackup()
				backup.Tasks = []*KubernetesMongodbBackupTask{
					{Name: "daily", Schedule: "0 2 * * *", StorageName: "primary", Compression: stringPtr(compression)},
				}
				input.Spec.Backup = backup
				gomega.Expect(protovalidate.Validate(input)).To(gomega.BeNil())
			}
		})

		ginkgo.It("PITR with a tuned oplog span should be valid", func() {
			backup := validBackup()
			backup.Pitr = &KubernetesMongodbPitr{
				Enabled:      true,
				OplogSpanMin: int32Ptr(10),
				Compression:  stringPtr("zstd"),
			}
			input.Spec.Backup = backup
			gomega.Expect(protovalidate.Validate(input)).To(gomega.BeNil())
		})

		ginkgo.It("PDB with only max_unavailable should be valid (single_bound)", func() {
			input.Spec.ReplicaSets[0].PodDisruptionBudget = &KubernetesMongodbPodDisruptionBudget{MaxUnavailable: 1}
			gomega.Expect(protovalidate.Validate(input)).To(gomega.BeNil())
		})

		ginkgo.It("PDB with only min_available should be valid (single_bound)", func() {
			input.Spec.ReplicaSets[0].PodDisruptionBudget = &KubernetesMongodbPodDisruptionBudget{MinAvailable: 2}
			gomega.Expect(protovalidate.Validate(input)).To(gomega.BeNil())
		})

		ginkgo.It("each allowed update strategy should be valid (update_strategy_enum)", func() {
			for _, strategy := range []string{"SmartUpdate", "RollingUpdate", "OnDelete"} {
				input.Spec.UpdateStrategy = stringPtr(strategy)
				gomega.Expect(protovalidate.Validate(input)).To(gomega.BeNil())
			}
		})

		ginkgo.It("full-surface sharded spec with every block populated should be valid", func() {
			input.Spec = &KubernetesMongodbSpec{
				Namespace:       literal("databases"),
				CreateNamespace: true,
				ImageName:       "percona/percona-server-mongodb:8.0.19-7",
				ReplicaSets: []*KubernetesMongodbReplicaSet{
					{
						Name: "rs0",
						Size: int32Ptr(3),
						Storage: &KubernetesMongodbStorage{
							Size:         "100Gi",
							StorageClass: literal("fast-ssd"),
						},
						Resources: &kubernetes.ContainerResources{
							Requests: &kubernetes.CpuMemory{Cpu: "1", Memory: "4Gi"},
							Limits:   &kubernetes.CpuMemory{Cpu: "2", Memory: "8Gi"},
						},
						MongodConfig: "operationProfiling:\n  mode: slowOp\n",
						Arbiter:      &KubernetesMongodbArbiter{Enabled: true, Size: int32Ptr(1)},
						Expose: &KubernetesMongodbExpose{
							Enabled:     true,
							Type:        stringPtr("LoadBalancer"),
							Annotations: map[string]string{"service.beta.kubernetes.io/aws-load-balancer-type": "nlb"},
						},
						PodDisruptionBudget: &KubernetesMongodbPodDisruptionBudget{MaxUnavailable: 1},
						Scheduling: &KubernetesMongodbScheduling{
							AntiAffinityTopologyKey: "topology.kubernetes.io/zone",
							NodeSelector:            map[string]string{"workload": "databases"},
							Tolerations: []*kubernetes.WorkloadToleration{
								{Key: "dedicated", Operator: "Equal", Value: "databases", Effect: "NoSchedule"},
							},
							PriorityClassName: "database-critical",
						},
					},
					{
						Name:    "rs1",
						Size:    int32Ptr(3),
						Storage: &KubernetesMongodbStorage{Size: "100Gi"},
					},
				},
				Sharding: &KubernetesMongodbSharding{
					Enabled: true,
					ConfigServer: &KubernetesMongodbConfigServer{
						Size:    int32Ptr(3),
						Storage: &KubernetesMongodbStorage{Size: "5Gi"},
						Resources: &kubernetes.ContainerResources{
							Requests: &kubernetes.CpuMemory{Cpu: "250m", Memory: "1Gi"},
							Limits:   &kubernetes.CpuMemory{Cpu: "500m", Memory: "2Gi"},
						},
					},
					Mongos: &KubernetesMongodbMongos{
						Size: int32Ptr(3),
						Expose: &KubernetesMongodbExpose{
							Enabled: true,
							Type:    stringPtr("ClusterIP"),
						},
					},
					BalancerEnabled: boolPtr(true),
				},
				Tls: &KubernetesMongodbTls{
					Mode:       stringPtr("requireTLS"),
					Issuer:     literal("org-ca-issuer"),
					IssuerKind: stringPtr("ClusterIssuer"),
				},
				Users: []*KubernetesMongodbUser{
					{
						Name:     "app-writer",
						Db:       stringPtr("admin"),
						Password: "writer-pass",
						Roles:    []*KubernetesMongodbUserRole{{Name: "readWrite", Db: "orders"}},
					},
				},
				Backup: &KubernetesMongodbBackup{
					Storages: []*KubernetesMongodbBackupStorage{
						func() *KubernetesMongodbBackupStorage {
							storage := s3Storage("primary")
							storage.Main = true
							return storage
						}(),
						s3Storage("secondary"),
					},
					Tasks: []*KubernetesMongodbBackupTask{
						{
							Name:              "daily",
							Schedule:          "0 2 * * *",
							StorageName:       "primary",
							Type:              stringPtr("physical"),
							Keep:              int32Ptr(7),
							DeleteFromStorage: boolPtr(true),
							Compression:       stringPtr("zstd"),
						},
					},
					Pitr: &KubernetesMongodbPitr{
						Enabled:      true,
						OplogSpanMin: int32Ptr(10),
						Compression:  stringPtr("gzip"),
					},
				},
				UpdateStrategy: stringPtr("SmartUpdate"),
				LogCollector: &KubernetesMongodbLogCollector{
					Enabled: boolPtr(true),
					Resources: &kubernetes.ContainerResources{
						Requests: &kubernetes.CpuMemory{Cpu: "100m", Memory: "128Mi"},
						Limits:   &kubernetes.CpuMemory{Cpu: "200m", Memory: "256Mi"},
					},
				},
				Unsafe: &KubernetesMongodbUnsafe{
					BackupIfUnhealthy: true,
				},
				Pause:            false,
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

		ginkgo.It("no replica sets should fail (min_items 1)", func() {
			input.Spec.ReplicaSets = nil
			gomega.Expect(protovalidate.Validate(input)).ToNot(gomega.BeNil())
		})

		ginkgo.It("duplicate replica set names should fail (unique_names)", func() {
			input.Spec.ReplicaSets = []*KubernetesMongodbReplicaSet{
				replicaSet("rs0"),
				replicaSet("rs0"),
			}
			input.Spec.Sharding = validSharding()
			gomega.Expect(protovalidate.Validate(input)).ToNot(gomega.BeNil())
		})

		ginkgo.It("uppercase replica set name should fail (name_format — DNS label)", func() {
			input.Spec.ReplicaSets = []*KubernetesMongodbReplicaSet{replicaSet("RS0")}
			gomega.Expect(protovalidate.Validate(input)).ToNot(gomega.BeNil())
		})

		ginkgo.It("two replica sets without sharding should fail (single_replset_without_sharding)", func() {
			input.Spec.ReplicaSets = []*KubernetesMongodbReplicaSet{
				replicaSet("rs0"),
				replicaSet("rs1"),
			}
			gomega.Expect(protovalidate.Validate(input)).ToNot(gomega.BeNil())
		})

		ginkgo.It("two replica sets with sharding declared but disabled should fail (single_replset_without_sharding)", func() {
			input.Spec.ReplicaSets = []*KubernetesMongodbReplicaSet{
				replicaSet("rs0"),
				replicaSet("rs1"),
			}
			input.Spec.Sharding = &KubernetesMongodbSharding{Enabled: false}
			gomega.Expect(protovalidate.Validate(input)).ToNot(gomega.BeNil())
		})

		ginkgo.It("zero replica set size should fail (gte 1)", func() {
			input.Spec.ReplicaSets[0].Size = int32Ptr(0)
			gomega.Expect(protovalidate.Validate(input)).ToNot(gomega.BeNil())
		})

		ginkgo.It("single-member replica set without unsafe.replset_size should fail (replset_size_or_unsafe)", func() {
			input.Spec.ReplicaSets[0].Size = int32Ptr(1)
			gomega.Expect(protovalidate.Validate(input)).ToNot(gomega.BeNil())
		})

		ginkgo.It("single-member replica set with unsafe declared but replset_size false should fail (replset_size_or_unsafe)", func() {
			input.Spec.ReplicaSets[0].Size = int32Ptr(1)
			input.Spec.Unsafe = &KubernetesMongodbUnsafe{Tls: true}
			gomega.Expect(protovalidate.Validate(input)).ToNot(gomega.BeNil())
		})

		ginkgo.It("single-member config server without unsafe.replset_size should fail (config_server_size_or_unsafe)", func() {
			sharding := validSharding()
			sharding.ConfigServer.Size = int32Ptr(1)
			input.Spec.Sharding = sharding
			gomega.Expect(protovalidate.Validate(input)).ToNot(gomega.BeNil())
		})

		ginkgo.It("replica set without storage should fail (required)", func() {
			input.Spec.ReplicaSets[0].Storage = nil
			gomega.Expect(protovalidate.Validate(input)).ToNot(gomega.BeNil())
		})

		ginkgo.It("storage size '10GB' should fail (size_quantity — GB is not a Kubernetes suffix)", func() {
			input.Spec.ReplicaSets[0].Storage.Size = "10GB"
			gomega.Expect(protovalidate.Validate(input)).ToNot(gomega.BeNil())
		})

		ginkgo.It("zero arbiter size should fail (gte 1)", func() {
			input.Spec.ReplicaSets[0].Arbiter = &KubernetesMongodbArbiter{Enabled: true, Size: int32Ptr(0)}
			gomega.Expect(protovalidate.Validate(input)).ToNot(gomega.BeNil())
		})

		ginkgo.It("unknown expose type should fail (expose.type_enum)", func() {
			input.Spec.ReplicaSets[0].Expose = &KubernetesMongodbExpose{Type: stringPtr("ExternalName")}
			gomega.Expect(protovalidate.Validate(input)).ToNot(gomega.BeNil())
		})

		ginkgo.It("sharding enabled without config_server should fail (enabled_needs_topology)", func() {
			input.Spec.Sharding = &KubernetesMongodbSharding{
				Enabled: true,
				Mongos:  &KubernetesMongodbMongos{},
			}
			gomega.Expect(protovalidate.Validate(input)).ToNot(gomega.BeNil())
		})

		ginkgo.It("sharding enabled without mongos should fail (enabled_needs_topology)", func() {
			input.Spec.Sharding = &KubernetesMongodbSharding{
				Enabled: true,
				ConfigServer: &KubernetesMongodbConfigServer{
					Storage: &KubernetesMongodbStorage{Size: "2Gi"},
				},
			}
			gomega.Expect(protovalidate.Validate(input)).ToNot(gomega.BeNil())
		})

		ginkgo.It("config server without storage should fail (required)", func() {
			sharding := validSharding()
			sharding.ConfigServer.Storage = nil
			input.Spec.Sharding = sharding
			gomega.Expect(protovalidate.Validate(input)).ToNot(gomega.BeNil())
		})

		ginkgo.It("zero config server size should fail (gte 1)", func() {
			sharding := validSharding()
			sharding.ConfigServer.Size = int32Ptr(0)
			input.Spec.Sharding = sharding
			gomega.Expect(protovalidate.Validate(input)).ToNot(gomega.BeNil())
		})

		ginkgo.It("zero mongos size should fail (gte 1)", func() {
			sharding := validSharding()
			sharding.Mongos.Size = int32Ptr(0)
			input.Spec.Sharding = sharding
			gomega.Expect(protovalidate.Validate(input)).ToNot(gomega.BeNil())
		})

		ginkgo.It("unknown TLS mode should fail (mode_enum)", func() {
			input.Spec.Tls = &KubernetesMongodbTls{Mode: stringPtr("mutualTLS")}
			gomega.Expect(protovalidate.Validate(input)).ToNot(gomega.BeNil())
		})

		ginkgo.It("disabled TLS mode without unsafe.tls should fail (tls_disabled_or_unsafe)", func() {
			input.Spec.Tls = &KubernetesMongodbTls{Mode: stringPtr("disabled")}
			gomega.Expect(protovalidate.Validate(input)).ToNot(gomega.BeNil())
		})

		ginkgo.It("unknown issuer_kind should fail (issuer_kind_enum)", func() {
			input.Spec.Tls = &KubernetesMongodbTls{IssuerKind: stringPtr("Cluster")}
			gomega.Expect(protovalidate.Validate(input)).ToNot(gomega.BeNil())
		})

		ginkgo.It("duplicate user names should fail (unique_names)", func() {
			input.Spec.Users = []*KubernetesMongodbUser{
				{Name: "app", Roles: []*KubernetesMongodbUserRole{{Name: "read", Db: "orders"}}},
				{Name: "app", Roles: []*KubernetesMongodbUserRole{{Name: "read", Db: "billing"}}},
			}
			gomega.Expect(protovalidate.Validate(input)).ToNot(gomega.BeNil())
		})

		ginkgo.It("user without a name should fail (required)", func() {
			input.Spec.Users = []*KubernetesMongodbUser{
				{Roles: []*KubernetesMongodbUserRole{{Name: "read", Db: "orders"}}},
			}
			gomega.Expect(protovalidate.Validate(input)).ToNot(gomega.BeNil())
		})

		ginkgo.It("user without roles should fail (min_items 1)", func() {
			input.Spec.Users = []*KubernetesMongodbUser{
				{Name: "app"},
			}
			gomega.Expect(protovalidate.Validate(input)).ToNot(gomega.BeNil())
		})

		ginkgo.It("role without a name should fail (required)", func() {
			input.Spec.Users = []*KubernetesMongodbUser{
				{Name: "app", Roles: []*KubernetesMongodbUserRole{{Db: "orders"}}},
			}
			gomega.Expect(protovalidate.Validate(input)).ToNot(gomega.BeNil())
		})

		ginkgo.It("role without a database should fail (required)", func() {
			input.Spec.Users = []*KubernetesMongodbUser{
				{Name: "app", Roles: []*KubernetesMongodbUserRole{{Name: "readWrite"}}},
			}
			gomega.Expect(protovalidate.Validate(input)).ToNot(gomega.BeNil())
		})

		ginkgo.It("unknown update_strategy should fail (update_strategy_enum)", func() {
			input.Spec.UpdateStrategy = stringPtr("Recreate")
			gomega.Expect(protovalidate.Validate(input)).ToNot(gomega.BeNil())
		})

		ginkgo.It("backup without storages should fail (min_items 1)", func() {
			input.Spec.Backup = &KubernetesMongodbBackup{}
			gomega.Expect(protovalidate.Validate(input)).ToNot(gomega.BeNil())
		})

		ginkgo.It("duplicate storage names should fail (storages.unique_names)", func() {
			input.Spec.Backup = &KubernetesMongodbBackup{
				Storages: []*KubernetesMongodbBackupStorage{
					s3Storage("primary"),
					s3Storage("primary"),
				},
			}
			gomega.Expect(protovalidate.Validate(input)).ToNot(gomega.BeNil())
		})

		ginkgo.It("uppercase storage name should fail (storages.name_format — DNS label)", func() {
			backup := validBackup()
			backup.Storages[0].Name = "Primary"
			input.Spec.Backup = backup
			gomega.Expect(protovalidate.Validate(input)).ToNot(gomega.BeNil())
		})

		ginkgo.It("two backup storages with no main should fail (storages.single_main)", func() {
			backup := validBackup()
			backup.Storages = append(backup.Storages, s3Storage("secondary"))
			input.Spec.Backup = backup
			gomega.Expect(protovalidate.Validate(input)).ToNot(gomega.BeNil())
		})

		ginkgo.It("two backup storages both marked main should fail (storages.single_main)", func() {
			backup := validBackup()
			backup.Storages[0].Main = true
			backup.Storages = append(backup.Storages, s3Storage("secondary"))
			backup.Storages[1].Main = true
			input.Spec.Backup = backup
			gomega.Expect(protovalidate.Validate(input)).ToNot(gomega.BeNil())
		})

		ginkgo.It("storage without a backend arm should fail (oneof required)", func() {
			input.Spec.Backup = &KubernetesMongodbBackup{
				Storages: []*KubernetesMongodbBackupStorage{
					{Name: "primary"},
				},
			}
			gomega.Expect(protovalidate.Validate(input)).ToNot(gomega.BeNil())
		})

		ginkgo.It("s3 without a bucket should fail (required)", func() {
			backup := validBackup()
			backup.Storages[0].GetS3().Bucket = ""
			input.Spec.Backup = backup
			gomega.Expect(protovalidate.Validate(input)).ToNot(gomega.BeNil())
		})

		ginkgo.It("non-http endpoint_url should fail (endpoint_format)", func() {
			backup := validBackup()
			s3 := backup.Storages[0].GetS3()
			s3.EndpointUrl = "minio.minio-system.svc:9000"
			s3.AccessKeys = &KubernetesMongodbS3AccessKeys{
				AccessKeyId:     "minio",
				SecretAccessKey: "minio123",
			}
			input.Spec.Backup = backup
			gomega.Expect(protovalidate.Validate(input)).ToNot(gomega.BeNil())
		})

		ginkgo.It("S3-compatible endpoint without access keys should fail (compatible_needs_keys)", func() {
			backup := validBackup()
			backup.Storages[0].GetS3().EndpointUrl = "http://minio.minio-system.svc:9000"
			input.Spec.Backup = backup
			gomega.Expect(protovalidate.Validate(input)).ToNot(gomega.BeNil())
		})

		ginkgo.It("access keys without access_key_id should fail (required)", func() {
			backup := validBackup()
			backup.Storages[0].GetS3().AccessKeys = &KubernetesMongodbS3AccessKeys{SecretAccessKey: "secret"}
			input.Spec.Backup = backup
			gomega.Expect(protovalidate.Validate(input)).ToNot(gomega.BeNil())
		})

		ginkgo.It("access keys without secret_access_key should fail (required)", func() {
			backup := validBackup()
			backup.Storages[0].GetS3().AccessKeys = &KubernetesMongodbS3AccessKeys{AccessKeyId: "AKIAEXAMPLE"}
			input.Spec.Backup = backup
			gomega.Expect(protovalidate.Validate(input)).ToNot(gomega.BeNil())
		})

		ginkgo.It("r2 without a bucket should fail (required)", func() {
			storage := r2Storage("r2")
			storage.GetR2().Bucket = nil
			input.Spec.Backup = &KubernetesMongodbBackup{Storages: []*KubernetesMongodbBackupStorage{storage}}
			gomega.Expect(protovalidate.Validate(input)).ToNot(gomega.BeNil())
		})

		ginkgo.It("r2 without an account_id should fail (required)", func() {
			storage := r2Storage("r2")
			storage.GetR2().AccountId = nil
			input.Spec.Backup = &KubernetesMongodbBackup{Storages: []*KubernetesMongodbBackupStorage{storage}}
			gomega.Expect(protovalidate.Validate(input)).ToNot(gomega.BeNil())
		})

		ginkgo.It("r2 with a malformed literal account_id should fail (account_id_format)", func() {
			storage := r2Storage("r2")
			storage.GetR2().AccountId = literal("not-an-account-id")
			input.Spec.Backup = &KubernetesMongodbBackup{Storages: []*KubernetesMongodbBackupStorage{storage}}
			gomega.Expect(protovalidate.Validate(input)).ToNot(gomega.BeNil())
		})

		ginkgo.It("r2 with an unknown literal jurisdiction should fail (jurisdiction_valid)", func() {
			storage := r2Storage("r2")
			storage.GetR2().Jurisdiction = literal("europe")
			input.Spec.Backup = &KubernetesMongodbBackup{Storages: []*KubernetesMongodbBackupStorage{storage}}
			gomega.Expect(protovalidate.Validate(input)).ToNot(gomega.BeNil())
		})

		ginkgo.It("r2 without credentials should fail (required — R2 has no keyless posture)", func() {
			storage := r2Storage("r2")
			storage.GetR2().Credentials = nil
			input.Spec.Backup = &KubernetesMongodbBackup{Storages: []*KubernetesMongodbBackupStorage{storage}}
			gomega.Expect(protovalidate.Validate(input)).ToNot(gomega.BeNil())
		})

		ginkgo.It("r2 credentials missing the secret access key should fail (required)", func() {
			storage := r2Storage("r2")
			storage.GetR2().Credentials.SecretAccessKey = nil
			input.Spec.Backup = &KubernetesMongodbBackup{Storages: []*KubernetesMongodbBackupStorage{storage}}
			gomega.Expect(protovalidate.Validate(input)).ToNot(gomega.BeNil())
		})

		ginkgo.It("gcs without a bucket should fail (required)", func() {
			storage := gcsStorage("gcs-store")
			storage.GetGcs().Bucket = ""
			input.Spec.Backup = &KubernetesMongodbBackup{Storages: []*KubernetesMongodbBackupStorage{storage}}
			gomega.Expect(protovalidate.Validate(input)).ToNot(gomega.BeNil())
		})

		ginkgo.It("gcs without credentials should fail (required — there is no keyless GCS posture)", func() {
			storage := gcsStorage("gcs-store")
			storage.GetGcs().Credentials = nil
			input.Spec.Backup = &KubernetesMongodbBackup{Storages: []*KubernetesMongodbBackupStorage{storage}}
			gomega.Expect(protovalidate.Validate(input)).ToNot(gomega.BeNil())
		})

		ginkgo.It("gcs credentials with no source arm should fail (oneof required)", func() {
			storage := gcsStorage("gcs-store")
			storage.GetGcs().Credentials = &KubernetesMongodbGcsCredentials{}
			input.Spec.Backup = &KubernetesMongodbBackup{Storages: []*KubernetesMongodbBackupStorage{storage}}
			gomega.Expect(protovalidate.Validate(input)).ToNot(gomega.BeNil())
		})

		ginkgo.It("restore without the backup block should fail (restore_requires_backup)", func() {
			input.Spec.Restore = &KubernetesMongodbRestore{
				Source: &KubernetesMongodbRestore_BackupName{BackupName: "nightly-2026-09-09"},
			}
			gomega.Expect(protovalidate.Validate(input)).ToNot(gomega.BeNil())
		})

		ginkgo.It("restore from a storage the backup block does not declare should fail (restore_storage_declared)", func() {
			input.Spec.Backup = validBackup()
			input.Spec.Restore = &KubernetesMongodbRestore{
				Source: &KubernetesMongodbRestore_BackupSource{
					BackupSource: &KubernetesMongodbRestoreBackupSource{
						StorageName: "elsewhere",
						Destination: "s3://mongo-backups/2026-09-09T12:00:00Z",
					},
				},
			}
			gomega.Expect(protovalidate.Validate(input)).ToNot(gomega.BeNil())
		})

		ginkgo.It("restore with no source should fail (oneof required)", func() {
			input.Spec.Backup = validBackup()
			input.Spec.Restore = &KubernetesMongodbRestore{}
			gomega.Expect(protovalidate.Validate(input)).ToNot(gomega.BeNil())
		})

		ginkgo.It("restore destination without a URI scheme should fail (destination_uri)", func() {
			input.Spec.Backup = validBackup()
			input.Spec.Restore = &KubernetesMongodbRestore{
				Source: &KubernetesMongodbRestore_BackupSource{
					BackupSource: &KubernetesMongodbRestoreBackupSource{
						StorageName: "primary",
						Destination: "mongo-backups/2026-09-09T12:00:00Z",
					},
				},
			}
			gomega.Expect(protovalidate.Validate(input)).ToNot(gomega.BeNil())
		})

		ginkgo.It("restore type incremental should fail (type_enum — chains restore through their physical base)", func() {
			input.Spec.Backup = validBackup()
			input.Spec.Restore = &KubernetesMongodbRestore{
				Source: &KubernetesMongodbRestore_BackupSource{
					BackupSource: &KubernetesMongodbRestoreBackupSource{
						StorageName: "primary",
						Destination: "s3://mongo-backups/2026-09-09T12:00:00Z",
						Type:        stringPtr("incremental"),
					},
				},
			}
			gomega.Expect(protovalidate.Validate(input)).ToNot(gomega.BeNil())
		})

		ginkgo.It("restore PITR type date without a date should fail (pitr.date_pairing)", func() {
			input.Spec.Backup = validBackup()
			input.Spec.Restore = &KubernetesMongodbRestore{
				Source: &KubernetesMongodbRestore_BackupName{BackupName: "nightly"},
				Pitr:   &KubernetesMongodbRestorePitr{Type: "date"},
			}
			gomega.Expect(protovalidate.Validate(input)).ToNot(gomega.BeNil())
		})

		ginkgo.It("restore PITR type latest with a date should fail (pitr.date_pairing)", func() {
			input.Spec.Backup = validBackup()
			input.Spec.Restore = &KubernetesMongodbRestore{
				Source: &KubernetesMongodbRestore_BackupName{BackupName: "nightly"},
				Pitr:   &KubernetesMongodbRestorePitr{Type: "latest", Date: "2026-09-09 12:30:00"},
			}
			gomega.Expect(protovalidate.Validate(input)).ToNot(gomega.BeNil())
		})

		ginkgo.It("restore PITR date in RFC3339 form should fail (the operator's format is 'YYYY-MM-DD HH:MM:SS')", func() {
			input.Spec.Backup = validBackup()
			input.Spec.Restore = &KubernetesMongodbRestore{
				Source: &KubernetesMongodbRestore_BackupName{BackupName: "nightly"},
				Pitr:   &KubernetesMongodbRestorePitr{Type: "date", Date: "2026-09-09T12:30:00Z"},
			}
			gomega.Expect(protovalidate.Validate(input)).ToNot(gomega.BeNil())
		})

		ginkgo.It("restore PITR with an unknown type should fail (pitr.type_enum)", func() {
			input.Spec.Backup = validBackup()
			input.Spec.Restore = &KubernetesMongodbRestore{
				Source: &KubernetesMongodbRestore_BackupName{BackupName: "nightly"},
				Pitr:   &KubernetesMongodbRestorePitr{Type: "oplog"},
			}
			gomega.Expect(protovalidate.Validate(input)).ToNot(gomega.BeNil())
		})

		ginkgo.It("azure without a container should fail (required)", func() {
			input.Spec.Backup = &KubernetesMongodbBackup{
				Storages: []*KubernetesMongodbBackupStorage{
					{
						Name: "azure-store",
						Backend: &KubernetesMongodbBackupStorage_Azure{
							Azure: &KubernetesMongodbAzureStorage{
								StorageAccount: "myaccount",
								AccessKey:      "base64key==",
							},
						},
					},
				},
			}
			gomega.Expect(protovalidate.Validate(input)).ToNot(gomega.BeNil())
		})

		ginkgo.It("azure without a storage_account should fail (required)", func() {
			input.Spec.Backup = &KubernetesMongodbBackup{
				Storages: []*KubernetesMongodbBackupStorage{
					{
						Name: "azure-store",
						Backend: &KubernetesMongodbBackupStorage_Azure{
							Azure: &KubernetesMongodbAzureStorage{
								Container: "mongo-backups",
								AccessKey: "base64key==",
							},
						},
					},
				},
			}
			gomega.Expect(protovalidate.Validate(input)).ToNot(gomega.BeNil())
		})

		ginkgo.It("azure without an access_key should fail (required)", func() {
			input.Spec.Backup = &KubernetesMongodbBackup{
				Storages: []*KubernetesMongodbBackupStorage{
					{
						Name: "azure-store",
						Backend: &KubernetesMongodbBackupStorage_Azure{
							Azure: &KubernetesMongodbAzureStorage{
								Container:      "mongo-backups",
								StorageAccount: "myaccount",
							},
						},
					},
				},
			}
			gomega.Expect(protovalidate.Validate(input)).ToNot(gomega.BeNil())
		})

		ginkgo.It("duplicate task names should fail (tasks.unique_names)", func() {
			backup := validBackup()
			backup.Tasks = []*KubernetesMongodbBackupTask{
				{Name: "daily", Schedule: "0 2 * * *", StorageName: "primary"},
				{Name: "daily", Schedule: "0 4 * * *", StorageName: "primary"},
			}
			input.Spec.Backup = backup
			gomega.Expect(protovalidate.Validate(input)).ToNot(gomega.BeNil())
		})

		ginkgo.It("uppercase task name should fail (tasks.name_format — DNS label)", func() {
			backup := validBackup()
			backup.Tasks = []*KubernetesMongodbBackupTask{
				{Name: "Daily", Schedule: "0 2 * * *", StorageName: "primary"},
			}
			input.Spec.Backup = backup
			gomega.Expect(protovalidate.Validate(input)).ToNot(gomega.BeNil())
		})

		ginkgo.It("six-field cron should fail (cron_five_fields — seconds are not accepted)", func() {
			backup := validBackup()
			backup.Tasks = []*KubernetesMongodbBackupTask{
				{Name: "daily", Schedule: "0 0 2 * * *", StorageName: "primary"},
			}
			input.Spec.Backup = backup
			gomega.Expect(protovalidate.Validate(input)).ToNot(gomega.BeNil())
		})

		ginkgo.It("task without storage_name should fail (required)", func() {
			backup := validBackup()
			backup.Tasks = []*KubernetesMongodbBackupTask{
				{Name: "daily", Schedule: "0 2 * * *"},
			}
			input.Spec.Backup = backup
			gomega.Expect(protovalidate.Validate(input)).ToNot(gomega.BeNil())
		})

		ginkgo.It("unknown backup type should fail (type_enum)", func() {
			backup := validBackup()
			backup.Tasks = []*KubernetesMongodbBackupTask{
				{Name: "daily", Schedule: "0 2 * * *", StorageName: "primary", Type: stringPtr("differential")},
			}
			input.Spec.Backup = backup
			gomega.Expect(protovalidate.Validate(input)).ToNot(gomega.BeNil())
		})

		ginkgo.It("zero task keep should fail (gte 1)", func() {
			backup := validBackup()
			backup.Tasks = []*KubernetesMongodbBackupTask{
				{Name: "daily", Schedule: "0 2 * * *", StorageName: "primary", Keep: int32Ptr(0)},
			}
			input.Spec.Backup = backup
			gomega.Expect(protovalidate.Validate(input)).ToNot(gomega.BeNil())
		})

		ginkgo.It("unknown task compression should fail (compression_enum)", func() {
			backup := validBackup()
			backup.Tasks = []*KubernetesMongodbBackupTask{
				{Name: "daily", Schedule: "0 2 * * *", StorageName: "primary", Compression: stringPtr("brotli")},
			}
			input.Spec.Backup = backup
			gomega.Expect(protovalidate.Validate(input)).ToNot(gomega.BeNil())
		})

		ginkgo.It("zero PITR oplog span should fail (gte 1)", func() {
			backup := validBackup()
			backup.Pitr = &KubernetesMongodbPitr{Enabled: true, OplogSpanMin: int32Ptr(0)}
			input.Spec.Backup = backup
			gomega.Expect(protovalidate.Validate(input)).ToNot(gomega.BeNil())
		})

		ginkgo.It("unknown PITR compression should fail (pitr.compression_enum)", func() {
			backup := validBackup()
			backup.Pitr = &KubernetesMongodbPitr{Enabled: true, Compression: stringPtr("brotli")}
			input.Spec.Backup = backup
			gomega.Expect(protovalidate.Validate(input)).ToNot(gomega.BeNil())
		})

		ginkgo.It("PDB with both bounds should fail (single_bound)", func() {
			input.Spec.ReplicaSets[0].PodDisruptionBudget = &KubernetesMongodbPodDisruptionBudget{
				MaxUnavailable: 1,
				MinAvailable:   2,
			}
			gomega.Expect(protovalidate.Validate(input)).ToNot(gomega.BeNil())
		})

		ginkgo.It("negative PDB max_unavailable should fail (gte 0)", func() {
			input.Spec.ReplicaSets[0].PodDisruptionBudget = &KubernetesMongodbPodDisruptionBudget{MaxUnavailable: -1}
			gomega.Expect(protovalidate.Validate(input)).ToNot(gomega.BeNil())
		})
	})
})
