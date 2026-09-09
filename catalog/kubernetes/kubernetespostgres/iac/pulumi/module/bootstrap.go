package module

import (
	postgresqlv1 "github.com/plantonhq/planton/pkg/kubernetes/kubernetestypes/cloudnativepg/kubernetes/postgresql/v1"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

// buildBootstrap renders the bootstrap stanza for whichever arm the spec
// declares. Bootstrap is how the cluster is BORN, so it is immutable — the
// operator ignores changes after the first reconcile.
func buildBootstrap(locals *Locals) postgresqlv1.ClusterSpecBootstrapPtrInput {
	bootstrap := locals.Spec.GetBootstrap()
	if bootstrap == nil {
		return nil
	}

	if initdb := bootstrap.GetInitdb(); initdb != nil {
		initdbArgs := postgresqlv1.ClusterSpecBootstrapInitdbArgs{}
		if initdb.GetDatabase() != "" {
			initdbArgs.Database = pulumi.String(initdb.GetDatabase())
		}
		if initdb.GetOwner() != "" {
			initdbArgs.Owner = pulumi.String(initdb.GetOwner())
		}
		if initdb.GetOwnerPassword() != "" {
			// The module materialized the declared password as a
			// basic-auth Secret; the operator adopts it as the app
			// credential instead of generating `<name>-app`.
			initdbArgs.Secret = postgresqlv1.ClusterSpecBootstrapInitdbSecretArgs{
				Name: pulumi.String(locals.ProvidedAppSecretName),
			}
		}
		if initdb.GetDataChecksums() {
			initdbArgs.DataChecksums = pulumi.Bool(true)
		}
		if initdb.GetEncoding() != "" {
			initdbArgs.Encoding = pulumi.String(initdb.GetEncoding())
		}
		if initdb.GetLocaleCollate() != "" {
			initdbArgs.LocaleCollate = pulumi.String(initdb.GetLocaleCollate())
		}
		if initdb.GetLocaleCtype() != "" {
			initdbArgs.LocaleCType = pulumi.String(initdb.GetLocaleCtype())
		}
		if len(initdb.GetPostInitSql()) > 0 {
			initdbArgs.PostInitSQL = pulumi.ToStringArray(initdb.GetPostInitSql())
		}
		if len(initdb.GetPostInitApplicationSql()) > 0 {
			initdbArgs.PostInitApplicationSQL = pulumi.ToStringArray(initdb.GetPostInitApplicationSql())
		}
		if importConfig := initdb.GetImport(); importConfig != nil {
			importArgs := postgresqlv1.ClusterSpecBootstrapInitdbImportArgs{
				Type: pulumi.String(importConfig.GetType()),
				Source: postgresqlv1.ClusterSpecBootstrapInitdbImportSourceArgs{
					ExternalCluster: pulumi.String(importConfig.GetSourceExternalCluster()),
				},
				Databases: pulumi.ToStringArray(importConfig.GetDatabases()),
			}
			if len(importConfig.GetRoles()) > 0 {
				importArgs.Roles = pulumi.ToStringArray(importConfig.GetRoles())
			}
			if importConfig.GetSchemaOnly() {
				importArgs.SchemaOnly = pulumi.Bool(true)
			}
			initdbArgs.Import = importArgs
		}
		return postgresqlv1.ClusterSpecBootstrapArgs{Initdb: initdbArgs}
	}

	if recovery := bootstrap.GetRecovery(); recovery != nil {
		// Recovery reads through the synthetic externalClusters entry the
		// module renders (buildExternalClusters), whose plugin block points
		// at the `<name>-recovery-source` ObjectStore with the SOURCE
		// cluster's serverName.
		recoveryArgs := postgresqlv1.ClusterSpecBootstrapRecoveryArgs{
			Source: pulumi.String(vars.RecoverySourceExternalClusterName),
		}
		if recovery.GetDatabase() != "" {
			recoveryArgs.Database = pulumi.String(recovery.GetDatabase())
		}
		if recovery.GetOwner() != "" {
			recoveryArgs.Owner = pulumi.String(recovery.GetOwner())
		}
		if recovery.GetOwnerSecretName() != "" {
			// Credential continuity: the recovered roles keep the SOURCE's
			// passwords, so the app Secret this cluster hands out is the
			// source's own (brought by name), not a freshly generated one.
			recoveryArgs.Secret = postgresqlv1.ClusterSpecBootstrapRecoverySecretArgs{
				Name: pulumi.String(recovery.GetOwnerSecretName()),
			}
		}
		if target := recovery.GetRecoveryTarget(); target != nil {
			targetArgs := postgresqlv1.ClusterSpecBootstrapRecoveryRecoveryTargetArgs{}
			if target.GetTargetTime() != "" {
				targetArgs.TargetTime = pulumi.String(target.GetTargetTime())
			}
			if target.GetTargetLsn() != "" {
				targetArgs.TargetLSN = pulumi.String(target.GetTargetLsn())
			}
			if target.GetTargetName() != "" {
				targetArgs.TargetName = pulumi.String(target.GetTargetName())
			}
			if target.GetTargetImmediate() {
				targetArgs.TargetImmediate = pulumi.Bool(true)
			}
			if target.GetBackupId() != "" {
				targetArgs.BackupID = pulumi.String(target.GetBackupId())
			}
			recoveryArgs.RecoveryTarget = targetArgs
		}
		return postgresqlv1.ClusterSpecBootstrapArgs{Recovery: recoveryArgs}
	}

	if pgBasebackup := bootstrap.GetPgBasebackup(); pgBasebackup != nil {
		return postgresqlv1.ClusterSpecBootstrapArgs{
			Pg_basebackup: postgresqlv1.ClusterSpecBootstrapPgBasebackupArgs{
				Source: pulumi.String(pgBasebackup.GetSource()),
			},
		}
	}

	return nil
}

// buildExternalClusters renders the externalClusters list: the
// user-declared entries (pg_basebackup / import sources) plus — when the
// bootstrap is a recovery — the synthetic entry that carries the
// recovery-source ObjectStore reference.
func buildExternalClusters(locals *Locals) postgresqlv1.ClusterSpecExternalClustersArray {
	externalClusters := postgresqlv1.ClusterSpecExternalClustersArray{}

	for _, external := range locals.Spec.GetExternalClusters() {
		externalArgs := postgresqlv1.ClusterSpecExternalClustersArgs{
			Name: pulumi.String(external.GetName()),
		}
		if len(external.GetConnectionParameters()) > 0 {
			externalArgs.ConnectionParameters = pulumi.ToStringMap(external.GetConnectionParameters())
		}
		if external.GetPassword() != "" {
			externalArgs.Password = postgresqlv1.ClusterSpecExternalClustersPasswordArgs{
				Name: pulumi.String(locals.ClusterName + "-ext-" + external.GetName()),
				Key:  pulumi.String("password"),
			}
		}
		externalClusters = append(externalClusters, externalArgs)
	}

	if recovery := locals.Spec.GetBootstrap().GetRecovery(); recovery != nil {
		externalClusters = append(externalClusters, postgresqlv1.ClusterSpecExternalClustersArgs{
			Name: pulumi.String(vars.RecoverySourceExternalClusterName),
			Plugin: postgresqlv1.ClusterSpecExternalClustersPluginArgs{
				Name: pulumi.String(vars.BarmanCloudPluginName),
				Parameters: pulumi.StringMap{
					"barmanObjectName": pulumi.String(locals.RecoveryObjectStoreName),
					// The folder in the store the SOURCE cluster wrote —
					// its cluster name (serverName rides the plugin
					// parameters; the ObjectStore CRD forbids it inline).
					"serverName": pulumi.String(recovery.GetSourceServerName()),
				},
			},
		})
	}

	return externalClusters
}
