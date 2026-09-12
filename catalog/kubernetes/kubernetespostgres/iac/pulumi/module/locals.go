package module

import (
	"fmt"
	"strconv"

	kubernetespostgresv1alpha1 "github.com/plantonhq/planton/catalog/kubernetes/kubernetespostgres/v1alpha1"
	"github.com/plantonhq/planton/pkg/iac/pulumi/pulumimodule/provider/kubernetes/kuberneteslabelkeys"
	"github.com/plantonhq/planton/shared/cloudresourcekind"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

// Locals holds computed values derived from the stack input for use across
// the module. Every resolution here has an exact twin in the Terraform
// module's locals.tf — keep them in lockstep.
type Locals struct {
	KubernetesPostgres *kubernetespostgresv1alpha1.KubernetesPostgres
	Spec               *kubernetespostgresv1alpha1.KubernetesPostgresSpec

	// Resource-identity labels stamped on every module-created object
	// (namespace, Cluster, ObjectStores, ScheduledBackups, credential
	// Secrets). CloudNativePG derives ITS objects' identity from the
	// Cluster name; these labels tie the whole family back to the Planton
	// resource.
	Labels map[string]string

	// Namespace the cluster lives in (resolved literal from the spec's
	// value-or-ref).
	Namespace string

	// ClusterName is metadata.name — the naming root CloudNativePG derives
	// every object from: pods `<name>-N`, services `<name>-rw/-ro/-r`,
	// credential secrets `<name>-app` / `<name>-superuser`.
	ClusterName string

	// Traffic service names (operator-created; exported as outputs).
	RwServiceName string
	RoServiceName string
	RServiceName  string

	// KubeEndpoint is the in-cluster FQDN of the read-write service —
	// the connection host applications use.
	KubeEndpoint string

	// Deterministic names for the module-created satellites. Deterministic
	// (never engine-generated suffixes) so the import recipes can derive
	// them blind and both engines agree byte-for-byte.
	BackupObjectStoreName   string
	RecoveryObjectStoreName string
	BackupCredsSecretName   string
	RecoveryCredsSecretName string
	BackupEndpointCaName    string
	RecoveryEndpointCaName  string
	ProvidedAppSecretName   string
	ProvidedSuperuserSecret string
	OperatorSuperuserSecret string
	AppSecretName           string
	// EffectiveAppSecretName is where the application credential actually
	// lives: the operator-generated `<name>-app` normally, the
	// module-provided secret when initdb declares an owner password, or
	// the brought Secret when a recovery names the source's app Secret
	// (credential continuity) — in both latter cases the operator adopts
	// the Secret instead of generating its own.
	EffectiveAppSecretName string
}

func initializeLocals(_ *pulumi.Context, stackInput *kubernetespostgresv1alpha1.KubernetesPostgresStackInput) *Locals {
	target := stackInput.Target
	spec := target.Spec

	labels := map[string]string{
		kuberneteslabelkeys.Resource:     strconv.FormatBool(true),
		kuberneteslabelkeys.ResourceName: target.Metadata.Name,
		kuberneteslabelkeys.ResourceKind: cloudresourcekind.CloudResourceKind_KubernetesPostgres.String(),
	}
	if target.Metadata.Id != "" {
		labels[kuberneteslabelkeys.ResourceId] = target.Metadata.Id
	}
	if target.Metadata.Org != "" {
		labels[kuberneteslabelkeys.Organization] = target.Metadata.Org
	}
	if target.Metadata.Env != "" {
		labels[kuberneteslabelkeys.Environment] = target.Metadata.Env
	}

	namespace := spec.Namespace.GetValue()
	clusterName := target.Metadata.Name

	appSecretName := clusterName + "-app"
	providedAppSecretName := clusterName + "-app-provided"
	effectiveAppSecretName := appSecretName
	if spec.GetBootstrap().GetInitdb().GetOwnerPassword() != "" {
		effectiveAppSecretName = providedAppSecretName
	}
	if brought := spec.GetBootstrap().GetRecovery().GetOwnerSecretName(); brought != "" {
		effectiveAppSecretName = brought
	}

	return &Locals{
		KubernetesPostgres: target,
		Spec:               spec,
		Labels:             labels,
		Namespace:          namespace,
		ClusterName:        clusterName,
		RwServiceName:      clusterName + "-rw",
		RoServiceName:      clusterName + "-ro",
		RServiceName:       clusterName + "-r",
		KubeEndpoint: fmt.Sprintf("%s-rw.%s.svc.cluster.local:5432",
			clusterName, namespace),
		BackupObjectStoreName:   clusterName,
		RecoveryObjectStoreName: clusterName + "-recovery-source",
		BackupCredsSecretName:   clusterName + "-backup-creds",
		RecoveryCredsSecretName: clusterName + "-recovery-creds",
		BackupEndpointCaName:    clusterName + "-backup-endpoint-ca",
		RecoveryEndpointCaName:  clusterName + "-recovery-endpoint-ca",
		ProvidedAppSecretName:   providedAppSecretName,
		ProvidedSuperuserSecret: clusterName + "-superuser-provided",
		OperatorSuperuserSecret: clusterName + "-superuser",
		AppSecretName:           appSecretName,
		EffectiveAppSecretName:  effectiveAppSecretName,
	}
}
