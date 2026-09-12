package module

import (
	"fmt"
	"strconv"

	kubernetesmongodbv1alpha1 "github.com/plantonhq/planton/catalog/kubernetes/kubernetesmongodb/v1alpha1"
	"github.com/plantonhq/planton/pkg/iac/pulumi/pulumimodule/provider/kubernetes/kuberneteslabelkeys"
	"github.com/plantonhq/planton/shared/cloudresourcekind"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

// Locals holds computed values derived from the stack input for use across
// the module. Every resolution here has an exact twin in the Terraform
// module's locals.tf — keep them in lockstep: same resource names, same
// rendered CR body, same Secret names/keys/types.
type Locals struct {
	KubernetesMongodb *kubernetesmongodbv1alpha1.KubernetesMongodb
	Spec              *kubernetesmongodbv1alpha1.KubernetesMongodbSpec

	// Resource-identity labels stamped on every module-created object
	// (namespace, the PerconaServerMongoDB CR, credential Secrets). The
	// operator derives ITS objects' identity from the CR name; these labels
	// tie the whole family back to the Planton resource.
	Labels map[string]string

	// Namespace the cluster lives in (resolved literal from the spec's
	// value-or-ref).
	Namespace string

	// ClusterName is metadata.name — the naming root the operator derives
	// every object from: pods `<name>-<rs>-N`, the per-set headless
	// Services `<name>-<rs>`, the mongos Service `<name>-mongos`, and the
	// system-users Secret `<name>-secrets`.
	ClusterName string

	// UsersSecretName is the system-users Secret rendered explicitly as
	// spec.secrets.users — per-cluster naming, not the operator's shared
	// "percona-server-mongodb-users" fallback. When the spec brings its
	// own (system_users_secret_name — the disaster-recovery path, where
	// the restored data carries the SOURCE cluster's users and the
	// operator must log in with the source's passwords), that Secret is
	// referenced instead and the operator generates nothing.
	UsersSecretName string

	ShardingEnabled     bool
	FirstReplicaSetName string
	ServiceName         string
	KubeEndpoint        string
	ReplicaSetOutput    string
	PortForwardCommand  string
}

func initializeLocals(_ *pulumi.Context, stackInput *kubernetesmongodbv1alpha1.KubernetesMongodbStackInput) *Locals {
	target := stackInput.Target
	spec := target.Spec

	labels := map[string]string{
		kuberneteslabelkeys.Resource:     strconv.FormatBool(true),
		kuberneteslabelkeys.ResourceName: target.Metadata.Name,
		kuberneteslabelkeys.ResourceKind: cloudresourcekind.CloudResourceKind_KubernetesMongodb.String(),
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

	shardingEnabled := spec.GetSharding().GetEnabled()
	firstReplicaSetName := spec.GetReplicaSets()[0].GetName()

	serviceName := fmt.Sprintf("%s-%s", clusterName, firstReplicaSetName)
	if shardingEnabled {
		serviceName = clusterName + "-mongos"
	}

	replicaSetOutput := firstReplicaSetName
	if shardingEnabled {
		replicaSetOutput = ""
	}

	kubeEndpoint := fmt.Sprintf("%s.%s.svc.cluster.local:%d", serviceName, namespace, vars.MongoDBPort)

	usersSecretName := clusterName + "-secrets"
	if spec.GetSystemUsersSecretName() != "" {
		usersSecretName = spec.GetSystemUsersSecretName()
	}

	return &Locals{
		KubernetesMongodb:   target,
		Spec:                spec,
		Labels:              labels,
		Namespace:           namespace,
		ClusterName:         clusterName,
		UsersSecretName:     usersSecretName,
		ShardingEnabled:     shardingEnabled,
		FirstReplicaSetName: firstReplicaSetName,
		ServiceName:         serviceName,
		KubeEndpoint:        kubeEndpoint,
		ReplicaSetOutput:    replicaSetOutput,
		PortForwardCommand: fmt.Sprintf(
			"kubectl port-forward svc/%s -n %s %d:%d",
			serviceName, namespace, vars.MongoDBPort, vars.MongoDBPort),
	}
}
