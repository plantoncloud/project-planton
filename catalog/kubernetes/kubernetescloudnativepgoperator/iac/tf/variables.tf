variable "metadata" {
  description = "Cloud resource metadata"
  type = object({
    name        = string
    id          = optional(string, "")
    org         = optional(string, "")
    env         = optional(string, "")
    labels      = optional(map(string), {})
    annotations = optional(map(string), {})
    tags        = optional(list(string), [])
  })
}

variable "spec" {
  description = "KubernetesCloudNativePgOperator specification"
  type = object({
    namespace        = string
    create_namespace = optional(bool, false)
    chart_version    = optional(string)
    crds = optional(object({
      install = optional(bool)
    }))
    replicas = optional(number)
    resources = optional(object({
      limits = optional(object({
        cpu    = optional(string, "")
        memory = optional(string, "")
      }))
      requests = optional(object({
        cpu    = optional(string, "")
        memory = optional(string, "")
      }))
    }))
    watch = optional(object({
      cluster_wide = optional(bool)
      namespaces   = optional(list(string), [])
    }))
    operator_config           = optional(map(string), {})
    max_concurrent_reconciles = optional(number)
    barman_cloud_plugin = optional(object({
      enabled       = optional(bool, false)
      chart_version = optional(string)
      resources = optional(object({
        limits = optional(object({
          cpu    = optional(string, "")
          memory = optional(string, "")
        }))
        requests = optional(object({
          cpu    = optional(string, "")
          memory = optional(string, "")
        }))
      }))
    }))
    monitoring = optional(object({
      pod_monitor_enabled = optional(bool, false)
      grafana_dashboard   = optional(bool, false)
    }))
    priority_class_name = optional(string, "")
    node_selector       = optional(map(string), {})
    tolerations = optional(list(object({
      key                = optional(string, "")
      operator           = optional(string, "")
      value              = optional(string, "")
      effect             = optional(string, "")
      toleration_seconds = optional(number)
    })), [])
    image_pull_secrets = optional(list(string), [])
    image = optional(object({
      repository = optional(string, "")
      tag        = optional(string, "")
    }))
    helm_values      = optional(string, "")
    install_operator = optional(bool)
  })
}
