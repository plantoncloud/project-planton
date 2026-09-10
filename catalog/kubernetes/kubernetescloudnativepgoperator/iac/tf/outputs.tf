# Stack outputs — flattened onto KubernetesCloudNativePgOperatorStackOutputs
# by the platform. Keep in lockstep with the Pulumi module's exports.

output "namespace" {
  description = "Kubernetes namespace the operator was installed into — the Barman Cloud plugin kind installs into this same namespace"
  value       = local.namespace
}

output "release_name" {
  description = "Helm release name of the operator (fixed \"cnpg\" — one installation per cluster; cluster-scoped CRDs and the fixed webhook service name are singletons)"
  value       = local.release_name
}
