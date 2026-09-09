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
  description = "KubernetesPlantonPlatform specification"
  type = object({
    namespace        = string
    create_namespace = optional(bool, false)
    version          = string
    license = optional(object({
      key = optional(string, "")
      secret_key_ref = optional(object({
        name = string
        key  = string
      }))
    }))
    storage = optional(object({
      storage_class_name = optional(string, "")
      size               = optional(string, "")
    }))
    database = optional(object({
      postgresql = optional(object({
        replicas           = optional(number)
        storage_size       = optional(string, "")
        storage_class_name = optional(string, "")
      }))
      redis = optional(object({
        storage_size       = optional(string, "")
        storage_class_name = optional(string, "")
      }))
    }))
    ingress = optional(object({
      enabled            = optional(bool, false)
      hostname           = optional(string, "")
      ingress_class_name = optional(string, "")
      # name and namespace are KubernetesGateway foreign keys in the spec;
      # they arrive here already resolved to plain strings.
      gateway_ref = optional(object({
        name         = string
        namespace    = optional(string, "")
        section_name = optional(string, "")
      }))
      annotations = optional(map(string), {})
      tls = optional(object({
        secret_name = optional(string, "")
        issuer = optional(object({
          name = string
          kind = optional(string)
        }))
      }))
      # auto | public | private; empty rides the CRD default (auto).
      reachability = optional(string, "")
    }))
    gateway = optional(object({
      local_port = optional(number)
    }))
    identity = optional(object({
      realm       = optional(string)
      admin_email = optional(string, "")
    }))
    bootstrap = optional(object({
      organization = optional(object({
        slug = optional(string)
        name = optional(string, "")
      }))
      environment = optional(object({
        slug = optional(string)
        name = optional(string, "")
      }))
      admins          = optional(list(string), [])
      iac_provisioner = optional(string)
      secret_backend = optional(object({
        type = string
        aws_secrets_manager = optional(object({
          region      = string
          kms_key_arn = string
        }))
      }))
    }))
    runner = optional(object({
      enabled                       = optional(bool)
      storage_size                  = optional(string, "")
      storage_class_name            = optional(string, "")
      service_account_annotations   = optional(map(string), {})
      cloud_credentials_secret_name = optional(string, "")
    }))
    build = optional(object({
      enabled = optional(bool)
    }))
    vault = optional(object({
      enabled            = optional(bool)
      init_mode          = optional(string)
      storage_size       = optional(string, "")
      storage_class_name = optional(string, "")
    }))
    components = optional(object({
      authorization = optional(object({
        enabled = optional(bool, false)
      }))
      search = optional(object({
        enabled            = optional(bool, false)
        mode               = optional(string)
        storage_size       = optional(string, "")
        storage_class_name = optional(string, "")
        zookeeper = optional(object({
          replicas           = optional(number)
          storage_size       = optional(string, "")
          storage_class_name = optional(string, "")
        }))
      }))
      graph = optional(object({
        enabled            = optional(bool, false)
        storage_size       = optional(string, "")
        storage_class_name = optional(string, "")
      }))
    }))
    prerequisites = optional(object({
      postgres_operator = optional(string)
      solr_operator     = optional(string)
      tekton_pipelines  = optional(string)
    }))
    control_plane = optional(object({
      image = optional(object({
        repository = optional(string, "")
        tag        = optional(string, "")
      }))
      replicas                    = optional(number)
      external_config_secret_name = optional(string, "")
      service_account_annotations = optional(map(string), {})
    }))
    console = optional(object({
      image = optional(object({
        repository = optional(string, "")
        tag        = optional(string, "")
      }))
      replicas                    = optional(number)
      external_config_secret_name = optional(string, "")
    }))
  })
}