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
  description = "KubernetesMongodb specification"
  type = object({
    namespace        = string
    create_namespace = optional(bool, false)
    image_name       = optional(string, "")
    replica_sets = list(object({
      name = string
      size = optional(number)
      storage = object({
        size          = string
        storage_class = optional(string, "")
      })
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
      mongod_config = optional(string, "")
      arbiter = optional(object({
        enabled = optional(bool, false)
        size    = optional(number)
      }))
      expose = optional(object({
        enabled     = optional(bool, false)
        type        = optional(string)
        annotations = optional(map(string), {})
      }))
      pod_disruption_budget = optional(object({
        max_unavailable = optional(number, 0)
        min_available   = optional(number, 0)
      }))
      scheduling = optional(object({
        anti_affinity_topology_key = optional(string, "")
        node_selector              = optional(map(string), {})
        tolerations = optional(list(object({
          key                = optional(string, "")
          operator           = optional(string, "")
          value              = optional(string, "")
          effect             = optional(string, "")
          toleration_seconds = optional(number)
        })), [])
        priority_class_name = optional(string, "")
      }))
    }))
    sharding = optional(object({
      enabled = optional(bool, false)
      config_server = optional(object({
        size = optional(number)
        storage = object({
          size          = string
          storage_class = optional(string, "")
        })
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
      mongos = optional(object({
        size = optional(number)
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
        expose = optional(object({
          enabled     = optional(bool, false)
          type        = optional(string)
          annotations = optional(map(string), {})
        }))
      }))
      balancer_enabled = optional(bool)
    }))
    tls = optional(object({
      mode                   = optional(string)
      issuer                 = optional(string, "")
      issuer_kind            = optional(string)
      cert_validity_duration = optional(string, "")
    }))
    users = optional(list(object({
      name     = string
      db       = optional(string)
      password = optional(string, "")
      roles = list(object({
        name = string
        db   = string
      }))
    })), [])
    backup = optional(object({
      storages = list(object({
        name = string
        main = optional(bool, false)
        s3 = optional(object({
          bucket                   = string
          region                   = optional(string, "")
          prefix                   = optional(string, "")
          endpoint_url             = optional(string, "")
          insecure_skip_tls_verify = optional(bool, false)
          access_keys = optional(object({
            access_key_id     = string
            secret_access_key = string
          }))
        }))
        gcs = optional(object({
          bucket = string
          prefix = optional(string, "")
          credentials = object({
            service_account_key  = optional(string, "")
            existing_secret_name = optional(string, "")
          })
        }))
        azure = optional(object({
          container       = string
          prefix          = optional(string, "")
          endpoint_url    = optional(string, "")
          storage_account = string
          access_key      = string
        }))
        r2 = optional(object({
          bucket       = string
          prefix       = optional(string, "")
          account_id   = string
          jurisdiction = optional(string, "")
          credentials = object({
            access_key_id     = string
            secret_access_key = string
          })
        }))
      }))
      tasks = optional(list(object({
        name                = string
        schedule            = string
        storage_name        = string
        type                = optional(string)
        keep                = optional(number)
        delete_from_storage = optional(bool)
        suspend             = optional(bool, false)
        compression         = optional(string)
      })), [])
      pitr = optional(object({
        enabled        = optional(bool, false)
        oplog_only     = optional(bool, false)
        oplog_span_min = optional(number)
        compression    = optional(string)
      }))
    }))
    update_strategy = optional(string)
    log_collector = optional(object({
      enabled = optional(bool)
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
    unsafe = optional(object({
      replset_size        = optional(bool, false)
      mongos_size         = optional(bool, false)
      tls                 = optional(bool, false)
      backup_if_unhealthy = optional(bool, false)
    }))
    pause              = optional(bool, false)
    image_pull_secrets = optional(list(string), [])
    restore = optional(object({
      backup_name = optional(string, "")
      backup_source = optional(object({
        storage_name = string
        destination  = string
        type         = optional(string)
      }))
      pitr = optional(object({
        type = string
        date = optional(string, "")
      }))
      replset_remapping = optional(map(string), {})
    }))
    system_users_secret_name = optional(string, "")
  })
}
