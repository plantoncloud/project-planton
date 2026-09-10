package module

import (
	kubernetesprovider "github.com/plantonhq/planton/catalog/kubernetes"
	kubernetesmongodbv1alpha1 "github.com/plantonhq/planton/catalog/kubernetes/kubernetesmongodb/v1alpha1"
	"github.com/plantonhq/planton/pkg/cloudflare/r2"
	"github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes"
	"github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/apiextensions"
	kubernetesmeta "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/meta/v1"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

// createCluster renders the psmdb.percona.com/v1 PerconaServerMongoDB
// resource as an UNTYPED CustomResource (apiextensions.NewCustomResource +
// kubernetes.UntypedArgs), not the typed crd2pulumi SDK.
//
// WHY untyped: crd2pulumi mistypes the CRD's spec.backup.storages — a map
// of storage names to nested objects carrying s3/gcs/azure sub-blocks — as
// a flat map[string]map[string]string (pulumi.StringMapMapInput). The typed
// path therefore CANNOT render nested storages at all: any adapter that
// smuggles nested maps through the mis-typed field panics at runtime, both
// when converting to the declared output type and when the engine marshals
// the inputs ("cannot convert an input of type pulumi.Map to a value of
// type map[string]string"). A backup-enabled cluster would crash every
// deployment. Plain map[string]interface{} carries the exact nested shape
// the operator expects.
//
// The rendered body's exact twin is local.mongodb_manifest in the Terraform
// module's locals.tf — keep the two in lockstep: same keys, same presence
// discipline, same coalesced defaults, numbers as numbers. Unset optionals
// are omitted entirely so the apiserver applies the CRD's own defaults.
// What the typed SDK's compile-time checks used to catch is still caught
// loudly: the operator's CRD schema validates the applied spec on the
// server, and the live E2E lanes exercise the rendered arms end to end.
//
// PRESENCE-SENSITIVE KEYS (rendered only when they carry signal):
//   - sharding: omitted entirely unless spec.sharding.enabled — the
//     operator's zero value for an absent key is sharding disabled.
//   - logcollector: omitted when the spec block is absent (operator
//     v1.22.0 treats an absent key as DISABLED).
//   - unsafeFlags: only flags that are true render; the whole block is
//     omitted when none are.
//   - affinity: antiAffinityTopologyKey renders only when the spec sets
//     one; the literal "none" passes through verbatim — it is the
//     operator's own OFF switch.
//
// The cluster resource never awaits the operator -- EXCEPT when the spec
// declares a restore. The Restore object is a one-shot the operator does not
// retry: its controller validates the backup as soon as its own agent check
// passes, and on a FRESH cluster that check passes while the pbm-agents are
// still crash-looping through the replica set's first authentication (their
// heartbeats are written before the crash). The resync command it then sends
// reaches no listening agent, the controller's 30-second resync-start
// deadline expires, and the Restore is parked in a terminal error ("get
// backup meta: not found") that nothing revisits -- live-caught on a
// three-member target. status.state == ready is the signal after which every
// agent is listening and the cluster controller has already synced the main
// storage, so a restore declared in the same manifest as its target waits
// for it here (pulumi.com/waitFor, the provider's await on a JSONPath) and
// the Restore is created after. A cluster without a restore keeps the
// never-block-on-a-controller posture.
func createCluster(ctx *pulumi.Context, locals *Locals,
	kubernetesProvider pulumi.ProviderResource,
	dependencies []pulumi.ResourceOption,
) (pulumi.Resource, error) {
	annotations := map[string]string{}
	if locals.Spec.GetRestore() != nil {
		annotations["pulumi.com/waitFor"] = "jsonpath={.status.state}=ready"
		// Three members on an autoscaled cluster take minutes each; the
		// provider's default await of ten minutes is too tight for a
		// restore target's first start.
		annotations["pulumi.com/timeoutSeconds"] = "1800"
	}
	return apiextensions.NewCustomResource(ctx, locals.ClusterName,
		&apiextensions.CustomResourceArgs{
			ApiVersion: pulumi.String("psmdb.percona.com/v1"),
			Kind:       pulumi.String("PerconaServerMongoDB"),
			Metadata: &kubernetesmeta.ObjectMetaArgs{
				Name:        pulumi.String(locals.ClusterName),
				Namespace:   pulumi.String(locals.Namespace),
				Labels:      pulumi.ToStringMap(locals.Labels),
				Annotations: pulumi.ToStringMap(annotations),
				Finalizers:  pulumi.StringArray{pulumi.String(vars.Finalizer)},
			},
			OtherFields: kubernetes.UntypedArgs{
				"spec": buildSpec(locals),
			},
		}, append([]pulumi.ResourceOption{pulumi.Provider(kubernetesProvider)}, dependencies...)...)
}

// buildSpec is the twin of mongodb_manifest.spec in locals.tf: one map,
// nil-valued keys never inserted (the Go twin of HCL's null-prune).
func buildSpec(locals *Locals) map[string]interface{} {
	spec := locals.Spec

	out := map[string]interface{}{
		"crVersion":      vars.CRVersion,
		"image":          imageName(spec),
		"updateStrategy": updateStrategy(spec),
		// Module-owned constants: the version service is upstream's, and
		// automated version application is deliberately not modeled —
		// versions change by editing image_name, never behind the
		// module's back.
		"upgradeOptions": map[string]interface{}{
			"versionServiceEndpoint": vars.VersionServiceEndpoint,
			"apply":                  "disabled",
		},
		"secrets": map[string]interface{}{
			"users": locals.UsersSecretName,
		},
		"replsets": buildReplsets(locals),
	}

	if spec.GetPause() {
		out["pause"] = true
	}

	if len(spec.GetImagePullSecrets()) > 0 {
		pullSecrets := make([]interface{}, 0, len(spec.GetImagePullSecrets()))
		for _, name := range spec.GetImagePullSecrets() {
			pullSecrets = append(pullSecrets, map[string]interface{}{"name": name})
		}
		out["imagePullSecrets"] = pullSecrets
	}

	if unsafeFlags := buildUnsafeFlags(spec.GetUnsafe()); len(unsafeFlags) > 0 {
		out["unsafeFlags"] = unsafeFlags
	}

	if tls := buildTLS(spec.GetTls()); tls != nil {
		out["tls"] = tls
	}

	if locals.ShardingEnabled {
		out["sharding"] = buildSharding(locals)
	}

	if users := buildUsers(locals); len(users) > 0 {
		out["users"] = users
	}

	if backup := buildBackup(locals); backup != nil {
		out["backup"] = backup
	}

	if logcollector := buildLogCollector(spec.GetLogCollector()); logcollector != nil {
		out["logcollector"] = logcollector
	}

	return out
}

func imageName(spec *kubernetesmongodbv1alpha1.KubernetesMongodbSpec) string {
	if spec.GetImageName() != "" {
		return spec.GetImageName()
	}
	return vars.DefaultImage
}

// SmartUpdate (the upstream default) unless the spec diverges — rendered
// explicitly so the update posture is visible in the CR.
func updateStrategy(spec *kubernetesmongodbv1alpha1.KubernetesMongodbSpec) string {
	if spec.UpdateStrategy != nil && *spec.UpdateStrategy != "" {
		return *spec.UpdateStrategy
	}
	return "SmartUpdate"
}

// buildUnsafeFlags renders only the flags that are TRUE; the caller omits
// the block when none are. tls.mode "disabled" REQUIRES unsafeFlags.tls —
// deliberately NOT auto-set here: unsafe.tls is the user's explicit opt-in,
// and the operator rejecting a disabled-TLS cluster without it is the
// designed loud failure.
func buildUnsafeFlags(unsafe *kubernetesmongodbv1alpha1.KubernetesMongodbUnsafe) map[string]interface{} {
	if unsafe == nil {
		return nil
	}

	out := map[string]interface{}{}
	if unsafe.GetTls() {
		out["tls"] = true
	}
	if unsafe.GetReplsetSize() {
		out["replsetSize"] = true
	}
	if unsafe.GetMongosSize() {
		out["mongosSize"] = true
	}
	if unsafe.GetBackupIfUnhealthy() {
		out["backupIfUnhealthy"] = true
	}
	return out
}

// buildTLS renders only when the spec declares a TLS posture; an absent
// block leaves the operator's own default (preferTLS with self-generated
// certificates). issuerConf points cert-manager at an organization-trusted
// chain; group is always cert-manager.io.
func buildTLS(tls *kubernetesmongodbv1alpha1.KubernetesMongodbTls) map[string]interface{} {
	if tls == nil {
		return nil
	}

	mode := "preferTLS"
	if tls.GetMode() != "" {
		mode = tls.GetMode()
	}

	out := map[string]interface{}{
		"mode": mode,
	}

	if tls.GetCertValidityDuration() != "" {
		out["certValidityDuration"] = tls.GetCertValidityDuration()
	}

	if tls.GetIssuer().GetValue() != "" {
		kind := "ClusterIssuer"
		if tls.GetIssuerKind() != "" {
			kind = tls.GetIssuerKind()
		}
		out["issuerConf"] = map[string]interface{}{
			"name":  tls.GetIssuer().GetValue(),
			"kind":  kind,
			"group": "cert-manager.io",
		}
	}

	return out
}

// buildReplsets renders one entry per declared replica set (each becomes a
// shard when sharding is enabled). Sizes render with the spec's declared
// defaults applied (3 members, 1 arbiter) so both engines emit identical
// bodies.
func buildReplsets(locals *Locals) []interface{} {
	replsets := make([]interface{}, 0, len(locals.Spec.GetReplicaSets()))
	for _, rs := range locals.Spec.GetReplicaSets() {
		size := 3
		if rs.Size != nil {
			size = int(*rs.Size)
		}

		replset := map[string]interface{}{
			"name":       rs.GetName(),
			"size":       size,
			"volumeSpec": buildVolumeSpec(rs.GetStorage()),
		}

		// Extra mongod configuration merged over the operator's defaults
		// — passed VERBATIM (mongod.conf YAML shape).
		if rs.GetMongodConfig() != "" {
			replset["configuration"] = rs.GetMongodConfig()
		}

		if resources := buildContainerResources(rs.GetResources()); resources != nil {
			replset["resources"] = resources
		}

		if scheduling := rs.GetScheduling(); scheduling != nil {
			// The operator's anti-affinity spreads members across
			// kubernetes.io/hostname by default; only a declared topology
			// key renders. "none" is the operator's own OFF switch and
			// passes through verbatim.
			if scheduling.GetAntiAffinityTopologyKey() != "" {
				replset["affinity"] = map[string]interface{}{
					"antiAffinityTopologyKey": scheduling.GetAntiAffinityTopologyKey(),
				}
			}
			if len(scheduling.GetNodeSelector()) > 0 {
				replset["nodeSelector"] = scheduling.GetNodeSelector()
			}
			if tolerations := buildTolerations(scheduling.GetTolerations()); len(tolerations) > 0 {
				replset["tolerations"] = tolerations
			}
			if scheduling.GetPriorityClassName() != "" {
				replset["priorityClassName"] = scheduling.GetPriorityClassName()
			}
		}

		if pdb := buildPodDisruptionBudget(rs.GetPodDisruptionBudget()); pdb != nil {
			replset["podDisruptionBudget"] = pdb
		}

		// Per-member Services (the managed-cloud LoadBalancer /
		// cross-cluster recipe surface).
		if expose := buildReplsetExpose(rs.GetExpose()); expose != nil {
			replset["expose"] = expose
		}

		if arbiter := buildArbiter(rs); arbiter != nil {
			replset["arbiter"] = arbiter
		}

		replsets = append(replsets, replset)
	}
	return replsets
}

// buildVolumeSpec renders one PVC per member; grows are applied in place,
// shrinks rejected by the operator.
func buildVolumeSpec(storage *kubernetesmongodbv1alpha1.KubernetesMongodbStorage) map[string]interface{} {
	pvc := map[string]interface{}{
		"resources": map[string]interface{}{
			"requests": map[string]interface{}{
				"storage": storage.GetSize(),
			},
		},
	}
	if storage.GetStorageClass().GetValue() != "" {
		pvc["storageClassName"] = storage.GetStorageClass().GetValue()
	}
	return map[string]interface{}{
		"persistentVolumeClaim": pvc,
	}
}

func buildContainerResources(resources *kubernetesprovider.ContainerResources) map[string]interface{} {
	if resources == nil {
		return nil
	}

	out := map[string]interface{}{}
	if resources.GetLimits() != nil {
		out["limits"] = cpuMemoryMap(resources.GetLimits())
	}
	if resources.GetRequests() != nil {
		out["requests"] = cpuMemoryMap(resources.GetRequests())
	}
	return out
}

func cpuMemoryMap(rl *kubernetesprovider.CpuMemory) map[string]interface{} {
	out := map[string]interface{}{}
	if rl.GetCpu() != "" {
		out["cpu"] = rl.GetCpu()
	}
	if rl.GetMemory() != "" {
		out["memory"] = rl.GetMemory()
	}
	return out
}

func buildTolerations(tolerations []*kubernetesprovider.WorkloadToleration) []interface{} {
	if len(tolerations) == 0 {
		return nil
	}

	out := make([]interface{}, 0, len(tolerations))
	for _, toleration := range tolerations {
		entry := map[string]interface{}{}
		if toleration.GetKey() != "" {
			entry["key"] = toleration.GetKey()
		}
		if toleration.GetOperator() != "" {
			entry["operator"] = toleration.GetOperator()
		}
		if toleration.GetValue() != "" {
			entry["value"] = toleration.GetValue()
		}
		if toleration.GetEffect() != "" {
			entry["effect"] = toleration.GetEffect()
		}
		if toleration.TolerationSeconds != nil {
			entry["tolerationSeconds"] = int(*toleration.TolerationSeconds)
		}
		out = append(out, entry)
	}
	return out
}

// buildPodDisruptionBudget renders only when a bound is declared (the spec
// CEL forbids both); an absent key leaves the operator default (max one
// member down).
func buildPodDisruptionBudget(pdb *kubernetesmongodbv1alpha1.KubernetesMongodbPodDisruptionBudget) map[string]interface{} {
	if pdb == nil {
		return nil
	}

	maxUnavailable := pdb.GetMaxUnavailable()
	minAvailable := pdb.GetMinAvailable()
	if maxUnavailable <= 0 && minAvailable <= 0 {
		return nil
	}

	out := map[string]interface{}{}
	if maxUnavailable > 0 {
		out["maxUnavailable"] = int(maxUnavailable)
	}
	if minAvailable > 0 {
		out["minAvailable"] = int(minAvailable)
	}
	return out
}

func buildReplsetExpose(expose *kubernetesmongodbv1alpha1.KubernetesMongodbExpose) map[string]interface{} {
	if expose == nil || !expose.GetEnabled() {
		return nil
	}

	out := map[string]interface{}{
		"enabled": true,
		"type":    exposeTypeValue(expose.GetType()),
	}
	if len(expose.GetAnnotations()) > 0 {
		out["annotations"] = expose.GetAnnotations()
	}
	return out
}

func exposeTypeValue(exposeType string) string {
	if exposeType != "" {
		return exposeType
	}
	return "ClusterIP"
}

// buildArbiter renders the arbiter: votes, no data. Its affinity mirrors
// the set's so the arbiter spreads across the same topology as the members
// it breaks ties for.
func buildArbiter(rs *kubernetesmongodbv1alpha1.KubernetesMongodbReplicaSet) map[string]interface{} {
	if rs.GetArbiter() == nil || !rs.GetArbiter().GetEnabled() {
		return nil
	}

	size := 1
	if rs.GetArbiter().Size != nil {
		size = int(*rs.GetArbiter().Size)
	}

	out := map[string]interface{}{
		"enabled": true,
		"size":    size,
	}

	if rs.GetScheduling().GetAntiAffinityTopologyKey() != "" {
		out["affinity"] = map[string]interface{}{
			"antiAffinityTopologyKey": rs.GetScheduling().GetAntiAffinityTopologyKey(),
		}
	}

	return out
}

// buildSharding is called only when sharding is enabled — the key is
// OMITTED otherwise: the operator's zero value for an absent key is
// sharding disabled, and only an enabled topology requires configsvr/mongos
// declarations (spec CEL mirrors that).
func buildSharding(locals *Locals) map[string]interface{} {
	sharding := locals.Spec.GetSharding()

	// The balancer default is enabled upstream; the flag renders
	// explicitly either way so flipping it is a clean diff.
	balancerEnabled := true
	if sharding.BalancerEnabled != nil {
		balancerEnabled = *sharding.BalancerEnabled
	}

	configSize := 3
	if sharding.GetConfigServer().Size != nil {
		configSize = int(*sharding.GetConfigServer().Size)
	}

	configsvr := map[string]interface{}{
		"size":       configSize,
		"volumeSpec": buildVolumeSpec(sharding.GetConfigServer().GetStorage()),
	}
	if resources := buildContainerResources(sharding.GetConfigServer().GetResources()); resources != nil {
		configsvr["resources"] = resources
	}

	mongosSize := 3
	if sharding.GetMongos().Size != nil {
		mongosSize = int(*sharding.GetMongos().Size)
	}

	mongos := map[string]interface{}{
		"size": mongosSize,
	}
	if resources := buildContainerResources(sharding.GetMongos().GetResources()); resources != nil {
		mongos["resources"] = resources
	}
	if expose := buildMongosExpose(sharding.GetMongos().GetExpose()); expose != nil {
		mongos["expose"] = expose
	}

	return map[string]interface{}{
		"enabled": true,
		"balancer": map[string]interface{}{
			"enabled": balancerEnabled,
		},
		"configsvrReplSet": configsvr,
		"mongos":           mongos,
	}
}

// buildMongosExpose: the mongos Service always exists (upstream
// MongosExpose has NO enabled field — unlike the per-set ExposeTogglable);
// the spec's expose.enabled gates whether the module renders customization
// over the operator's ClusterIP default.
func buildMongosExpose(expose *kubernetesmongodbv1alpha1.KubernetesMongodbExpose) map[string]interface{} {
	if expose == nil || !expose.GetEnabled() {
		return nil
	}

	out := map[string]interface{}{
		"type": exposeTypeValue(expose.GetType()),
	}
	if len(expose.GetAnnotations()) > 0 {
		out["annotations"] = expose.GetAnnotations()
	}
	return out
}

// buildUsers renders the declarative application users. passwordSecretRef
// renders ONLY when a password is declared (the module materializes that
// Secret); otherwise the operator generates a password into its own
// per-user Secret.
func buildUsers(locals *Locals) []interface{} {
	users := make([]interface{}, 0, len(locals.Spec.GetUsers()))
	for _, user := range locals.Spec.GetUsers() {
		db := "admin"
		if user.Db != nil && *user.Db != "" {
			db = *user.Db
		}

		roles := make([]interface{}, 0, len(user.GetRoles()))
		for _, role := range user.GetRoles() {
			roles = append(roles, map[string]interface{}{
				"name": role.GetName(),
				"db":   role.GetDb(),
			})
		}

		entry := map[string]interface{}{
			"name":  user.GetName(),
			"db":    db,
			"roles": roles,
		}

		if user.GetPassword() != "" {
			entry["passwordSecretRef"] = map[string]interface{}{
				"name": locals.ClusterName + "-user-" + user.GetName(),
				"key":  "password",
			}
		}

		users = append(users, entry)
	}
	return users
}

func buildBackup(locals *Locals) map[string]interface{} {
	backup := locals.Spec.GetBackup()
	if backup == nil {
		return nil
	}

	out := map[string]interface{}{
		"enabled":  true,
		"image":    vars.BackupImage,
		"storages": buildBackupStorages(backup.GetStorages(), locals.ClusterName),
	}

	if pitr := buildPitr(backup.GetPitr()); pitr != nil {
		out["pitr"] = pitr
	}

	if tasks := buildBackupTasks(backup.GetTasks()); len(tasks) > 0 {
		out["tasks"] = tasks
	}

	return out
}

// buildBackupStorages renders storages as a MAP keyed by storage name (the
// CRD shape); tasks and PITR reference entries by that name.
// credentialsSecret renders for every arm that carries credentials — the
// module-materialized `<name>-backup-<storage>` or the user's existing
// Secret — and is omitted only on the keyless S3 arm (the pods' ambient AWS
// identity). GCS always renders one: the CRD requires it.
func buildBackupStorages(storages []*kubernetesmongodbv1alpha1.KubernetesMongodbBackupStorage, clusterName string) map[string]interface{} {
	out := map[string]interface{}{}
	for _, storage := range storages {
		entry := map[string]interface{}{}

		if storage.GetMain() {
			entry["main"] = true
		}

		// Exactly one backend arm exists (spec oneof).
		switch {
		case storage.GetS3() != nil:
			entry["type"] = "s3"
			entry["s3"] = buildBackupS3(storage.GetS3(), clusterName, storage.GetName())
		case storage.GetGcs() != nil:
			entry["type"] = "gcs"
			entry["gcs"] = buildBackupGcs(storage.GetGcs(), clusterName, storage.GetName())
		case storage.GetAzure() != nil:
			entry["type"] = "azure"
			entry["azure"] = buildBackupAzure(storage.GetAzure(), clusterName, storage.GetName())
		case storage.GetR2() != nil:
			// R2 speaks S3: the operator's `s3` block, with the R2-specific
			// values composed here rather than typed by the user.
			entry["type"] = "s3"
			entry["s3"] = buildBackupR2(storage.GetR2(), clusterName, storage.GetName())
		}

		out[storage.GetName()] = entry
	}
	return out
}

// buildBackupR2 renders a Cloudflare R2 storage onto the operator's `s3`
// block. The spec is in R2's own vocabulary (bucket, account, jurisdiction,
// a Cloudflare credential); this is where the S3 translation happens:
//   - endpointUrl: the jurisdiction's host, from the shared pkg/cloudflare/r2
//     helper (an eu/fedramp/us bucket is served ONLY through
//     <account>.<jurisdiction>.r2.cloudflarestorage.com);
//   - region: "auto", the only region R2 accepts (PBM would default to
//     us-east-1, which Cloudflare aliases -- rendered explicitly so the CR
//     says what it means);
//   - forcePathStyle: true -- PBM's own default when unset, pinned so the
//     addressing style the store was proven with is what the CR declares;
//   - credentialsSecret: the module-materialized `<name>-backup-<storage>`
//     Secret carrying AWS_ACCESS_KEY_ID / AWS_SECRET_ACCESS_KEY as declared
//     (by default the CloudflareAccountApiToken's r2_access_key_id /
//     r2_secret_access_key outputs, which that kind derives -- no hashing
//     happens here). There is no keyless posture for R2.
func buildBackupR2(r2Storage *kubernetesmongodbv1alpha1.KubernetesMongodbR2Storage, clusterName, storageName string) map[string]interface{} {
	out := map[string]interface{}{
		"bucket":            r2Storage.GetBucket().GetValue(),
		"region":            r2.Region,
		"endpointUrl":       r2.S3Endpoint(r2Storage.GetAccountId().GetValue(), r2Storage.GetJurisdiction().GetValue()),
		"forcePathStyle":    true,
		"credentialsSecret": clusterName + "-backup-" + storageName,
	}
	if r2Storage.GetPrefix() != "" {
		out["prefix"] = r2Storage.GetPrefix()
	}
	return out
}

func buildBackupS3(s3 *kubernetesmongodbv1alpha1.KubernetesMongodbS3Storage, clusterName, storageName string) map[string]interface{} {
	out := map[string]interface{}{
		"bucket": s3.GetBucket(),
	}
	if s3.GetRegion() != "" {
		out["region"] = s3.GetRegion()
	}
	if s3.GetPrefix() != "" {
		out["prefix"] = s3.GetPrefix()
	}
	if s3.GetEndpointUrl() != "" {
		out["endpointUrl"] = s3.GetEndpointUrl()
	}
	if s3.GetInsecureSkipTlsVerify() {
		out["insecureSkipTLSVerify"] = true
	}
	if s3.GetAccessKeys() != nil {
		out["credentialsSecret"] = clusterName + "-backup-" + storageName
	}
	return out
}

func buildBackupGcs(gcs *kubernetesmongodbv1alpha1.KubernetesMongodbGcsStorage, clusterName, storageName string) map[string]interface{} {
	out := map[string]interface{}{
		"bucket":            gcs.GetBucket(),
		"credentialsSecret": gcsCredentialsSecretName(gcs, clusterName, storageName),
	}
	if gcs.GetPrefix() != "" {
		out["prefix"] = gcs.GetPrefix()
	}
	return out
}

// gcsCredentialsSecretName is the Secret the operator's GCS client reads: the
// module-materialized `<name>-backup-<storage>` when the spec declares a
// service-account key, or the user's own Secret when it names one. The spec
// oneof guarantees exactly one arm.
func gcsCredentialsSecretName(gcs *kubernetesmongodbv1alpha1.KubernetesMongodbGcsStorage, clusterName, storageName string) string {
	if existing := gcs.GetCredentials().GetExistingSecretName(); existing != "" {
		return existing
	}
	return clusterName + "-backup-" + storageName
}

func buildBackupAzure(azure *kubernetesmongodbv1alpha1.KubernetesMongodbAzureStorage, clusterName, storageName string) map[string]interface{} {
	out := map[string]interface{}{
		"container":         azure.GetContainer(),
		"credentialsSecret": clusterName + "-backup-" + storageName,
	}
	if azure.GetPrefix() != "" {
		out["prefix"] = azure.GetPrefix()
	}
	if azure.GetEndpointUrl() != "" {
		out["endpointUrl"] = azure.GetEndpointUrl()
	}
	return out
}

// buildPitr: oplog chunks land on the main storage. Rendered with the
// spec's declared defaults applied (10-minute chunks, gzip). oplogSpanMin
// is a NUMBER in the CR — rendered as a Go int, never a float.
func buildPitr(pitr *kubernetesmongodbv1alpha1.KubernetesMongodbPitr) map[string]interface{} {
	if pitr == nil {
		return nil
	}

	oplogSpanMin := 10
	if pitr.OplogSpanMin != nil {
		oplogSpanMin = int(*pitr.OplogSpanMin)
	}

	compression := "gzip"
	if pitr.Compression != nil && *pitr.Compression != "" {
		compression = *pitr.Compression
	}

	out := map[string]interface{}{
		"enabled":         pitr.GetEnabled(),
		"oplogSpanMin":    oplogSpanMin,
		"compressionType": compression,
	}

	if pitr.GetOplogOnly() {
		out["oplogOnly"] = true
	}

	return out
}

// buildBackupTasks renders the scheduled tasks: enabled is the inverse of
// the spec's suspend (the declaration survives a suspension); retention
// renders only when keep is declared, always type "count" (the only
// retention the operator models).
func buildBackupTasks(tasks []*kubernetesmongodbv1alpha1.KubernetesMongodbBackupTask) []interface{} {
	if len(tasks) == 0 {
		return nil
	}

	out := make([]interface{}, 0, len(tasks))
	for _, task := range tasks {
		taskType := "logical"
		if task.Type != nil && *task.Type != "" {
			taskType = *task.Type
		}

		compression := "gzip"
		if task.Compression != nil && *task.Compression != "" {
			compression = *task.Compression
		}

		entry := map[string]interface{}{
			"name":            task.GetName(),
			"enabled":         !task.GetSuspend(),
			"schedule":        task.GetSchedule(),
			"storageName":     task.GetStorageName(),
			"type":            taskType,
			"compressionType": compression,
		}

		if task.Keep != nil {
			deleteFromStorage := true
			if task.DeleteFromStorage != nil {
				deleteFromStorage = *task.DeleteFromStorage
			}
			entry["retention"] = map[string]interface{}{
				"count":             int(*task.Keep),
				"type":              "count",
				"deleteFromStorage": deleteFromStorage,
			}
		}

		out = append(out, entry)
	}
	return out
}

// buildLogCollector renders only when the spec declares the block; the
// enabled flag defaults true within it. An ABSENT key means no sidecar
// (operator v1.22.0: IsLogCollectorEnabled() requires the block present
// AND enabled).
func buildLogCollector(logCollector *kubernetesmongodbv1alpha1.KubernetesMongodbLogCollector) map[string]interface{} {
	if logCollector == nil {
		return nil
	}

	enabled := true
	if logCollector.Enabled != nil {
		enabled = *logCollector.Enabled
	}

	out := map[string]interface{}{
		"enabled": enabled,
		"image":   vars.LogcollectorImage,
	}

	if resources := buildContainerResources(logCollector.GetResources()); resources != nil {
		out["resources"] = resources
	}

	return out
}
