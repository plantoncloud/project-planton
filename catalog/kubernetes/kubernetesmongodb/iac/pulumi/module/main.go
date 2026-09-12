package module

import (
	"github.com/pkg/errors"
	kubernetesmongodbv1alpha1 "github.com/plantonhq/planton/catalog/kubernetes/kubernetesmongodb/v1alpha1"
	"github.com/plantonhq/planton/pkg/iac/pulumi/pulumimodule/provider/kubernetes/pulumikubernetesprovider"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

// Resources deploys one Percona-operator-managed MongoDB cluster:
//
//  1. the namespace (optional),
//  2. declared-credential Secrets (user passwords, backup-storage keys)
//     — secrets always travel via secret references, never inline in a
//     custom resource,
//  3. the PerconaServerMongoDB CR itself (rendered untyped — see
//     cluster.go for why — and validated server-side by the operator's
//     CRD schema),
//  4. the PerconaServerMongoDBRestore run, when the spec declares a
//     restore (see restore.go for the run-once naming contract).
//
// Ordering matters for the namespace (everything is namespaced), for
// credential Secrets (the operator reads them at reconcile time), and for
// the restore (it names the cluster; the operator gates it on the members
// being up).
func Resources(ctx *pulumi.Context, stackInput *kubernetesmongodbv1alpha1.KubernetesMongodbStackInput) error {
	locals := initializeLocals(ctx, stackInput)

	kubernetesProvider, err := pulumikubernetesprovider.GetWithKubernetesProviderConfig(ctx,
		stackInput.ProviderConfig, "kubernetes")
	if err != nil {
		return errors.Wrap(err, "failed to set up kubernetes provider")
	}

	createdNamespace, err := namespace(ctx, stackInput, locals, kubernetesProvider)
	if err != nil {
		return errors.Wrap(err, "failed to create namespace")
	}

	var namespaceDeps []pulumi.ResourceOption
	if createdNamespace != nil {
		namespaceDeps = append(namespaceDeps, pulumi.DependsOn([]pulumi.Resource{createdNamespace}))
	}

	credentialSecrets, err := createCredentialSecrets(ctx, locals, kubernetesProvider, namespaceDeps)
	if err != nil {
		return errors.Wrap(err, "failed to create credential secrets")
	}

	clusterDeps := namespaceDeps
	if len(credentialSecrets) > 0 {
		clusterDeps = append(clusterDeps, pulumi.DependsOn(credentialSecrets))
	}

	cluster, err := createCluster(ctx, locals, kubernetesProvider, clusterDeps)
	if err != nil {
		return errors.Wrap(err, "failed to create PerconaServerMongoDB cluster")
	}

	if _, err := createRestore(ctx, locals, kubernetesProvider,
		[]pulumi.ResourceOption{pulumi.DependsOn([]pulumi.Resource{cluster})}); err != nil {
		return errors.Wrap(err, "failed to create PerconaServerMongoDBRestore")
	}

	exportOutputs(ctx, locals)
	return nil
}

func exportOutputs(ctx *pulumi.Context, locals *Locals) {
	ctx.Export(OpNamespace, pulumi.String(locals.Namespace))
	ctx.Export(OpClusterName, pulumi.String(locals.ClusterName))
	ctx.Export(OpService, pulumi.String(locals.ServiceName))
	ctx.Export(OpKubeEndpoint, pulumi.String(locals.KubeEndpoint))
	ctx.Export(OpReplicaSet, pulumi.String(locals.ReplicaSetOutput))
	ctx.Export(OpPortForwardCommand, pulumi.String(locals.PortForwardCommand))
	ctx.Export(OpAdminPasswordSecret, pulumi.Map{
		"name": pulumi.String(locals.UsersSecretName),
		"key":  pulumi.String(vars.AdminPasswordKey),
	})
	// The restore run's handle — empty when no restore is declared, so the
	// output is an honest "nothing to look at" rather than a phantom name.
	restoreNameOutput := ""
	if restore := locals.Spec.GetRestore(); restore != nil {
		restoreNameOutput = restoreName(locals.ClusterName, restore)
	}
	ctx.Export(OpRestoreName, pulumi.String(restoreNameOutput))
}
