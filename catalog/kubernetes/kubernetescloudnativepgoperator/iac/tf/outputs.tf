# Stack outputs — flattened onto KubernetesCloudNativePgOperatorStackOutputs
# by the platform. Keep in lockstep with the Pulumi module's exports.

output "namespace" {
  description = "Kubernetes namespace the operator (and the plugin, when enabled) was installed into"
  value       = local.namespace
}

output "release_name" {
  description = "Helm release name of the operator (fixed \"cnpg\" — one installation per cluster; cluster-scoped CRDs and the fixed webhook service name are singletons); empty in the plugin-only posture, where the operator release is someone else's"
  value       = local.install_operator ? local.release_name : ""
}

output "barman_plugin_release_name" {
  description = "Helm release name of the Barman Cloud plugin when enabled; empty otherwise — KubernetesPostgres backup blocks key off this handle"
  value       = local.barman_plugin_enabled ? local.plugin_release_name : ""
}
