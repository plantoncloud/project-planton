# Stack outputs — identical names and derivations in the Pulumi module's
# outputs.go / main.go exports (KubernetesMongodbStackOutputs).

output "namespace" {
  description = "Namespace the cluster runs in"
  value       = local.namespace
}

output "cluster_name" {
  description = "Name of the PerconaServerMongoDB resource (equals metadata.name) — every operator-created object derives from it"
  value       = local.cluster_name
}

output "service" {
  description = "The Service applications connect to: `<name>-mongos` when sharding is enabled, otherwise the first replica set's headless Service (`<name>-<rs>`)"
  value       = local.service_name
}

output "kube_endpoint" {
  description = "In-cluster connection endpoint (`<service>.<namespace>.svc.cluster.local:27017`); replica-set clusters connect with `?replicaSet=<replica_set>` so the driver follows failovers"
  value       = local.kube_endpoint
}

output "replica_set" {
  description = "The first replica set's name (the driver's replicaSet parameter) — empty for sharded clusters (mongos needs none)"
  value       = local.replica_set_output
}

output "port_forward_command" {
  description = "Port-forward command for reaching the database from a workstation when no exposure is composed"
  value       = "kubectl port-forward svc/${local.service_name} -n ${local.namespace} 27017:27017"
}

# The operator-managed system-users Secret (`<name>-secrets`): the module
# pins its name via spec.secrets.users, and the operator generates the
# built-in account passwords into it.
output "admin_password_secret" {
  description = "Secret key holding the database-admin password (paired username key: MONGODB_DATABASE_ADMIN_USER)"
  value = {
    name = local.users_secret_name
    key  = "MONGODB_DATABASE_ADMIN_PASSWORD"
  }
}

# The restore run's handle — empty when no restore is declared, so the output
# is an honest "nothing to look at" rather than a phantom name.
output "restore_name" {
  description = "Name of the PerconaServerMongoDBRestore rendered for spec.restore (`<name>-restore-<8 hex>`); empty when no restore is declared"
  value       = local.restore == null ? "" : local.restore_name
}
