package module

import (
	"crypto/sha256"
	"encoding/hex"
	"sort"
	"strings"

	kubernetesmongodbv1alpha1 "github.com/plantonhq/planton/catalog/kubernetes/kubernetesmongodb/v1alpha1"
	"github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes"
	"github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/apiextensions"
	kubernetesmeta "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/meta/v1"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

// createRestore renders the spec's restore declaration as a
// psmdb.percona.com/v1 PerconaServerMongoDBRestore against this cluster.
// Nothing renders when the spec declares no restore.
//
// A Restore object is a RUN, not a state: the operator drives it New ->
// requested -> running -> ready (or error) exactly once and never re-reads
// its spec afterwards. Declarative semantics therefore hinge on the object's
// NAME — `<cluster>-restore-<8 hex>` where the suffix hashes the declaration
// itself (source, point in time, remapping). An unchanged declaration keeps
// the same name and is a no-op on every apply; a changed declaration (a
// different backup, a later point in time) is a NEW object and a new run.
// The Terraform twin (local.restore_name in locals.tf) hashes the identical
// canonical string, so both engines name the same run the same way.
//
// The operator replays the backup INTO the running cluster (reading
// `backup.storages` for the storage the backup lives in — which is why the
// spec requires the backup block). Its own agent check is NOT a readiness
// check (see createCluster): the module therefore renders this object only
// after the cluster reports ready -- the cluster resource awaits that state
// when a restore is declared, and this resource depends on it -- and lets
// the operator own the run itself; the run's outcome is the Restore object's
// status.
func createRestore(ctx *pulumi.Context, locals *Locals,
	kubernetesProvider pulumi.ProviderResource,
	dependencies []pulumi.ResourceOption,
) (pulumi.Resource, error) {
	restore := locals.Spec.GetRestore()
	if restore == nil {
		return nil, nil
	}

	name := restoreName(locals.ClusterName, restore)
	return apiextensions.NewCustomResource(ctx, name,
		&apiextensions.CustomResourceArgs{
			ApiVersion: pulumi.String("psmdb.percona.com/v1"),
			Kind:       pulumi.String("PerconaServerMongoDBRestore"),
			Metadata: &kubernetesmeta.ObjectMetaArgs{
				Name:      pulumi.String(name),
				Namespace: pulumi.String(locals.Namespace),
				Labels:    pulumi.ToStringMap(locals.Labels),
			},
			OtherFields: kubernetes.UntypedArgs{
				"spec": buildRestoreSpec(locals.ClusterName, restore, locals.Spec.GetBackup().GetStorages()),
			},
		}, append([]pulumi.ResourceOption{pulumi.Provider(kubernetesProvider)}, dependencies...)...)
}

// buildRestoreSpec is the twin of local.restore_manifest.spec in locals.tf.
// Exactly one source arm exists (spec oneof): a same-namespace Backup object
// by name, or a location in one of this cluster's declared storages.
//
// The backup-source arm renders the storage TWICE, deliberately: as
// `storageName` AND as the storage's full block inside `backupSource`. The
// operator reads them on two different paths — validation resolves the
// storage through `storageName`, but the metadata resync it runs when the
// backup is unknown to the fresh cluster's PBM (every DR restore) resolves it
// from `backupSource.{gcs|s3|azure}` alone (`getStorage(cr, cluster, "")` in
// the restore controller), and a Restore without that block dies terminal
// with "unsupported backup storage type" — live-caught on GKE. The block is
// the same rendering the cluster's `backup.storages` entry gets, so the
// credentials Secret and prefix can never drift between the two.
func buildRestoreSpec(clusterName string, restore *kubernetesmongodbv1alpha1.KubernetesMongodbRestore,
	storages []*kubernetesmongodbv1alpha1.KubernetesMongodbBackupStorage,
) map[string]interface{} {
	out := map[string]interface{}{
		"clusterName": clusterName,
	}

	if restore.GetBackupName() != "" {
		out["backupName"] = restore.GetBackupName()
	}

	if source := restore.GetBackupSource(); source != nil {
		out["storageName"] = source.GetStorageName()
		backupSource := map[string]interface{}{
			"destination": source.GetDestination(),
			"type":        restoreSourceType(source),
		}
		for _, storage := range storages {
			if storage.GetName() != source.GetStorageName() {
				continue
			}
			// The spec's CEL guarantees the storage exists; the oneof
			// guarantees exactly one backend arm.
			switch {
			case storage.GetS3() != nil:
				backupSource["s3"] = buildBackupS3(storage.GetS3(), clusterName, storage.GetName())
			case storage.GetGcs() != nil:
				backupSource["gcs"] = buildBackupGcs(storage.GetGcs(), clusterName, storage.GetName())
			case storage.GetAzure() != nil:
				backupSource["azure"] = buildBackupAzure(storage.GetAzure(), clusterName, storage.GetName())
			case storage.GetR2() != nil:
				// R2 rides the operator's s3 block; the restore controller
				// validates an `s3` backupSource against an s3:// destination,
				// which is exactly what an R2 backup reports.
				backupSource["s3"] = buildBackupR2(storage.GetR2(), clusterName, storage.GetName())
			}
		}
		out["backupSource"] = backupSource
	}

	if pitr := restore.GetPitr(); pitr != nil {
		entry := map[string]interface{}{"type": pitr.GetType()}
		if pitr.GetDate() != "" {
			entry["date"] = pitr.GetDate()
		}
		out["pitr"] = entry
	}

	if len(restore.GetReplsetRemapping()) > 0 {
		remapping := make(map[string]interface{}, len(restore.GetReplsetRemapping()))
		for from, to := range restore.GetReplsetRemapping() {
			remapping[from] = to
		}
		out["replsetRemapping"] = remapping
	}

	return out
}

func restoreSourceType(source *kubernetesmongodbv1alpha1.KubernetesMongodbRestoreBackupSource) string {
	if source.Type != nil && *source.Type != "" {
		return *source.Type
	}
	return "logical"
}

// restoreName derives the Restore object's name from the declaration.
// The canonical string joins every field that changes WHAT is restored, in a
// fixed order, so the hash is stable across engines and across field
// reordering in the manifest; remapping pairs are sorted by source name.
func restoreName(clusterName string, restore *kubernetesmongodbv1alpha1.KubernetesMongodbRestore) string {
	parts := []string{restore.GetBackupName()}
	if source := restore.GetBackupSource(); source != nil {
		parts = append(parts, source.GetStorageName(), source.GetDestination(), restoreSourceType(source))
	} else {
		parts = append(parts, "", "", "")
	}
	parts = append(parts, restore.GetPitr().GetType(), restore.GetPitr().GetDate())

	remapping := make([]string, 0, len(restore.GetReplsetRemapping()))
	for from, to := range restore.GetReplsetRemapping() {
		remapping = append(remapping, from+"="+to)
	}
	sort.Strings(remapping)
	parts = append(parts, strings.Join(remapping, ","))

	sum := sha256.Sum256([]byte(strings.Join(parts, "|")))
	return clusterName + "-restore-" + hex.EncodeToString(sum[:])[:8]
}
