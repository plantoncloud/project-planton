# Stack outputs -- flattened onto KubernetesCnpgBarmanCloudPluginStackOutputs
# by the platform. Keep in lockstep with the Pulumi module's exports.

output "namespace" {
  description = "Kubernetes namespace the plugin was installed into -- the CloudNativePG operator's namespace, by requirement"
  value       = local.namespace
}

output "release_name" {
  description = "Helm release name of the plugin (fixed \"plugin-barman-cloud\" -- one plugin per operator namespace; the chart's Service, Secret and ConfigMap names are fixed)"
  value       = local.release_name
}

output "plugin_name" {
  description = "The CNPG-I plugin identifier a Cluster's plugins list names (fixed \"barman-cloud.cloudnative-pg.io\")"
  value       = local.plugin_name
}
