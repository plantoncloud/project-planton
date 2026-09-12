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
  description = "KubernetesCnpgBarmanCloudPlugin specification"
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
    image = optional(object({
      repository = optional(string, "")
      tag        = optional(string, "")
    }))
    sidecar_image = optional(object({
      repository = optional(string, "")
      tag        = optional(string, "")
    }))
    image_pull_secrets  = optional(list(string), [])
    priority_class_name = optional(string, "")
    node_selector       = optional(map(string), {})
    tolerations = optional(list(object({
      key                = optional(string, "")
      operator           = optional(string, "")
      value              = optional(string, "")
      effect             = optional(string, "")
      toleration_seconds = optional(number)
    })), [])
    helm_values = optional(string, "")
  })
}
