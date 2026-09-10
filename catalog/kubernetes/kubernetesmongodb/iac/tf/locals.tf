# Computed values for the KubernetesMongodb module. Every resolution here
# has an exact twin in the Pulumi module's locals.go / mongodb.go /
# secrets.go — keep them in lockstep: same resource names, same rendered
# CR body, same Secret names/keys/types.
#
# HCL DISCIPLINE (applies to every conditional object in this file):
# conditional entries are written as `key = cond ? value : null` inside ONE
# object literal, pruned with `{ for k, v in {...} : k => v if v != null }`.
# The two tempting alternatives are both broken: `cond ? {...} : {}`
# ternaries fail plan-time type unification when branches carry different
# attributes, and `merge(concat(cond ? [{...}] : [], ...)...)` silently
# UNIFIES primitive-only sibling objects into map(string) — numbers and
# booleans arrive at the API as strings and server-side validation rejects
# the object. The null-prune form preserves every value's type: sizes,
# oplogSpanMin, retention.count, tolerationSeconds, PDB bounds render as
# YAML numbers; the presence-gated booleans as booleans.
#
# Optional nested blocks are read with try(): HCL's && does NOT
# short-circuit, so chained null checks still dereference the null.
#
# PRESENCE-SENSITIVE KEYS (rendered only when they carry signal, exactly
# like the Pulumi module):
#   - sharding: OMITTED entirely unless spec.sharding.enabled — the
#     operator's zero value for an absent key is sharding disabled (the
#     CRD declares no default; psmdb_defaults.go only errors on a missing
#     configsvr/mongos when Enabled is true).
#   - logcollector: OMITTED when the spec block is absent. NOTE: operator
#     v1.22.0 treats an absent key as DISABLED (IsLogCollectorEnabled()
#     requires the block to be present AND enabled) — the sidecar only
#     runs when the spec declares it.
#   - unsafeFlags: only flags that are true render; the whole block is
#     omitted when none are.
#   - affinity: antiAffinityTopologyKey renders only when the spec sets
#     one (upstream default kubernetes.io/hostname); the literal "none"
#     passes through verbatim — it is the operator's own OFF switch
#     (AffinityOff = "none" in psmdb_defaults.go).

locals {
  # ClusterName is metadata.name — the naming root the operator derives
  # every object from: pods `<name>-<rs>-N`, the per-set headless
  # Services `<name>-<rs>`, the mongos Service `<name>-mongos`, and the
  # system-users Secret `<name>-secrets`.
  cluster_name = var.metadata.name
  namespace    = var.spec.namespace

  # Resource-identity labels stamped on every module-created object
  # (namespace, the PerconaServerMongoDB CR, credential Secrets). The
  # operator derives ITS objects' identity from the CR name; these labels
  # tie the whole family back to the Planton resource.
  labels = merge(
    {
      "planton.ai/resource"      = "true"
      "planton.ai/resource-name" = var.metadata.name
      "planton.ai/resource-kind" = "KubernetesMongodb"
    },
    var.metadata.id != null && var.metadata.id != "" ? { "planton.ai/resource-id" = var.metadata.id } : {},
    var.metadata.org != null && var.metadata.org != "" ? { "planton.ai/organization" = var.metadata.org } : {},
    var.metadata.env != null && var.metadata.env != "" ? { "planton.ai/environment" = var.metadata.env } : {}
  )

  # Module-owned constants, pinned to the operator release the module
  # targets (KubernetesPerconaMongoOperator at v1.22.0). crVersion tells
  # the operator which schema/behavior contract the CR was written
  # against; the images are the upstream-published companions of that
  # release. upgradeOptions is deliberately constant: automated version
  # application is not modeled — version changes happen by editing
  # image_name (a SmartUpdate rolling upgrade), never behind the module's
  # back.
  cr_version               = "1.22.0"
  default_image            = "percona/percona-server-mongodb:8.0.19-7"
  backup_image             = "percona/percona-backup-mongodb:2.12.0"
  logcollector_image       = "percona/fluentbit:4.0.1-2"
  version_service_endpoint = "https://check.percona.com"

  # The operator deletes member pods in order on CR deletion — the safe
  # teardown for a replica set (primary last).
  finalizers = ["percona.com/delete-psmdb-pods-in-order"]

  # The system-users Secret. Rendered EXPLICITLY as spec.secrets.users:
  # the operator's own fallback for an unset name is the static
  # "percona-server-mongodb-users" (psmdb_defaults.go) — shared across
  # every cluster in the namespace — so per-cluster naming requires the
  # module to pin `<name>-secrets` (the upstream cr.yaml convention).
  # When the spec brings its own (system_users_secret_name — the
  # disaster-recovery path, where the restored data carries the SOURCE
  # cluster's users and the operator must log in with the source's
  # passwords), that Secret is referenced and the operator generates
  # nothing.
  users_secret_name = try(var.spec.system_users_secret_name, "") != "" ? var.spec.system_users_secret_name : "${local.cluster_name}-secrets"

  sharding_enabled       = try(var.spec.sharding.enabled, false)
  first_replica_set_name = var.spec.replica_sets[0].name

  # The Service applications connect to: the mongos router Service when
  # sharding, otherwise the first replica set's headless Service (drivers
  # discover every member through it).
  service_name  = local.sharding_enabled ? "${local.cluster_name}-mongos" : "${local.cluster_name}-${local.first_replica_set_name}"
  kube_endpoint = "${local.service_name}.${local.namespace}.svc.cluster.local:27017"

  # The driver's replicaSet parameter — empty for sharded clusters
  # (mongos needs none).
  replica_set_output = local.sharding_enabled ? "" : local.first_replica_set_name

  backup = try(var.spec.backup, null)

  # ---- credential Secret payloads -----------------------------------------
  # Declared user passwords (`<name>-user-<username>`, single `password`
  # key) — the exact Secret shape the operator watches for declarative
  # users; rotating the value rotates the database password. Users with
  # no declared password get NO Secret and NO passwordSecretRef: the
  # operator generates a password into its own Secret.
  # Keyed by the FULL Secret name (not the bare username): the state
  # address key is what the import recipes derive the live object name
  # from (from_address_key), so the key and the rendered name must be
  # identical.
  user_password_secrets = {
    for u in var.spec.users : "${local.cluster_name}-user-${u.name}" => u.password
    if try(u.password, "") != ""
  }

  # Declared backup-storage credentials (`<name>-backup-<storage>`), one
  # Opaque Secret per storage whose keys are EXACTLY what the operator's
  # PBM integration reads (pkg/psmdb/backup/pbm.go):
  #   - s3:    AWS_ACCESS_KEY_ID / AWS_SECRET_ACCESS_KEY
  #   - gcs:   GCS_CLIENT_EMAIL / GCS_PRIVATE_KEY — the operator reads
  #            the two FIELDS, not the JSON key file, so the module
  #            extracts them from the declared service-account key with
  #            jsondecode (malformed JSON fails the plan loudly).
  #   - azure: AZURE_STORAGE_ACCOUNT_NAME / AZURE_STORAGE_ACCOUNT_KEY
  # The keyless S3 arm (no access_keys) creates NO Secret and renders NO
  # credentialsSecret — the PBM agents use the pods' ambient AWS identity.
  # GCS has no keyless arm (PBM's Google client requires a key); a GCS
  # storage naming an existing_secret_name creates no Secret either — the
  # operator reads the one the user brought. The r2 arm always carries a
  # key pair (R2 has no keyless posture): the CloudflareAccountApiToken's
  # r2_access_key_id / r2_secret_access_key outputs as declared (that kind
  # derives them; no hashing happens here), under the same AWS_* keys.
  # All values are strings, so this chained ternary unifies safely to
  # map(string) — no number/bool stringification risk here.
  backup_credential_secrets = local.backup == null ? {} : {
    for s in local.backup.storages : "${local.cluster_name}-backup-${s.name}" => (
      try(s.s3.access_keys, null) != null ? {
        AWS_ACCESS_KEY_ID     = s.s3.access_keys.access_key_id
        AWS_SECRET_ACCESS_KEY = s.s3.access_keys.secret_access_key
        } : try(s.r2, null) != null ? {
        AWS_ACCESS_KEY_ID     = s.r2.credentials.access_key_id
        AWS_SECRET_ACCESS_KEY = s.r2.credentials.secret_access_key
        } : try(s.gcs.credentials.service_account_key, "") != "" ? {
        GCS_CLIENT_EMAIL = jsondecode(local.gcs_key_json[s.name]).client_email
        GCS_PRIVATE_KEY  = jsondecode(local.gcs_key_json[s.name]).private_key
        } : try(s.azure, null) != null ? {
        AZURE_STORAGE_ACCOUNT_NAME = s.azure.storage_account
        AZURE_STORAGE_ACCOUNT_KEY  = s.azure.access_key
      } : null
    )
    if(
      try(s.s3.access_keys, null) != null ||
      try(s.r2, null) != null ||
      try(s.gcs.credentials.service_account_key, "") != "" ||
      try(s.azure, null) != null
    )
  }

  # Every storage that rides the operator's `s3` block, seen through one
  # S3-API view: the s3 arm as declared, and the r2 arm TRANSLATED from R2's
  # own vocabulary -- the jurisdiction's endpoint host (an eu/fedramp/us
  # bucket is served ONLY through <account>.<jurisdiction>.r2.cloudflarestorage.com;
  # the host table mirrors the Go helper package pkg/cloudflare/r2, the
  # source of truth both engines follow), region "auto" (the only region R2
  # accepts; PBM would default to us-east-1, which Cloudflare aliases --
  # rendered explicitly so the CR says what it means), and path-style
  # addressing pinned (PBM's own default when unset). Both branches carry
  # the SAME attribute set and types, so the ternary unifies cleanly; the
  # jurisdiction is an optional scalar inside a present block, read
  # null-safely (coalesce rejects the null tfvars carries for an unset
  # optional; try() turns that into "").
  backup_s3_api_storages = local.backup == null ? {} : {
    for s in local.backup.storages : s.name => (
      try(s.r2, null) != null ? {
        bucket = s.r2.bucket
        region = "auto"
        prefix = try(s.r2.prefix, "")
        endpoint_url = (
          coalesce(try(coalesce(s.r2.jurisdiction), ""), "default") == "default"
          ? "https://${s.r2.account_id}.r2.cloudflarestorage.com"
          : "https://${s.r2.account_id}.${s.r2.jurisdiction}.r2.cloudflarestorage.com"
        )
        force_path_style  = true
        insecure_skip_tls = false
        has_credentials   = true
        } : {
        bucket            = s.s3.bucket
        region            = try(s.s3.region, "")
        prefix            = try(s.s3.prefix, "")
        endpoint_url      = try(s.s3.endpoint_url, "")
        force_path_style  = false
        insecure_skip_tls = try(s.s3.insecure_skip_tls_verify, false)
        has_credentials   = try(s.s3.access_keys, null) != null
      }
    )
    if try(s.s3, null) != null || try(s.r2, null) != null
  }

  # A declared GCS service-account key arrives two ways — the raw JSON
  # key file, or its base64 encoding (the GcpServiceAccount `key_base64`
  # output, so a chart wires identity and database in one run). Raw JSON
  # is recognized by its opening brace; anything else must decode as
  # standard base64 (a malformed value fails the plan loudly). Twin:
  # decodeServiceAccountKey in the Pulumi module's secrets.go.
  gcs_key_json = local.backup == null ? {} : {
    for s in local.backup.storages : s.name => (
      startswith(trimspace(s.gcs.credentials.service_account_key), "{")
      ? trimspace(s.gcs.credentials.service_account_key)
      : base64decode(trimspace(s.gcs.credentials.service_account_key))
    )
    if try(s.gcs.credentials.service_account_key, "") != ""
  }

  # ---- CR: unsafeFlags ------------------------------------------------------
  # Only flags that are TRUE render; the block is omitted when none are.
  # tls.mode "disabled" REQUIRES unsafeFlags.tls — deliberately NOT
  # auto-set here: unsafe.tls is the user's explicit opt-in, and the
  # operator rejecting a disabled-TLS cluster without it is the designed
  # loud failure.
  unsafe_flags = {
    for k, v in {
      tls               = try(var.spec.unsafe.tls, false) ? true : null
      replsetSize       = try(var.spec.unsafe.replset_size, false) ? true : null
      mongosSize        = try(var.spec.unsafe.mongos_size, false) ? true : null
      backupIfUnhealthy = try(var.spec.unsafe.backup_if_unhealthy, false) ? true : null
    } : k => v if v != null
  }

  # ---- CR: tls ----------------------------------------------------------------
  # Rendered only when the spec declares a TLS posture; an absent block
  # leaves the operator's own default (preferTLS with self-generated
  # certificates). issuerConf points cert-manager at an
  # organization-trusted chain; group is always cert-manager.io.
  tls_body = try(var.spec.tls, null) == null ? null : {
    for k, v in {
      mode                 = coalesce(try(var.spec.tls.mode, null), "preferTLS")
      certValidityDuration = try(var.spec.tls.cert_validity_duration, "") != "" ? var.spec.tls.cert_validity_duration : null
      issuerConf = try(var.spec.tls.issuer, "") != "" ? {
        name  = var.spec.tls.issuer
        kind  = coalesce(try(var.spec.tls.issuer_kind, null), "ClusterIssuer")
        group = "cert-manager.io"
      } : null
    } : k => v if v != null
  }

  # ---- CR: replsets --------------------------------------------------------
  # One entry per declared replica set (each becomes a shard when
  # sharding is enabled). Sizes render with the spec's declared defaults
  # applied (3 members, 1 arbiter) so both engines emit identical bodies.
  replsets = [
    for rs in var.spec.replica_sets : {
      for k, v in {
        name = rs.name
        size = coalesce(try(rs.size, null), 3)

        # Extra mongod configuration merged over the operator's defaults
        # — passed VERBATIM (mongod.conf YAML shape).
        configuration = try(rs.mongod_config, "") != "" ? rs.mongod_config : null

        resources = try(rs.resources, null) == null ? null : {
          for rk, rv in {
            limits = try(rs.resources.limits, null) == null ? null : {
              for lk, lv in {
                cpu    = try(rs.resources.limits.cpu, "") != "" ? rs.resources.limits.cpu : null
                memory = try(rs.resources.limits.memory, "") != "" ? rs.resources.limits.memory : null
              } : lk => lv if lv != null
            }
            requests = try(rs.resources.requests, null) == null ? null : {
              for rk2, rv2 in {
                cpu    = try(rs.resources.requests.cpu, "") != "" ? rs.resources.requests.cpu : null
                memory = try(rs.resources.requests.memory, "") != "" ? rs.resources.requests.memory : null
              } : rk2 => rv2 if rv2 != null
            }
          } : rk => rv if rv != null
        }

        # One PVC per member; grows are applied in place, shrinks
        # rejected by the operator.
        volumeSpec = {
          persistentVolumeClaim = {
            for pk, pv in {
              storageClassName = try(rs.storage.storage_class, "") != "" ? rs.storage.storage_class : null
              resources        = { requests = { storage = rs.storage.size } }
            } : pk => pv if pv != null
          }
        }

        # The operator's anti-affinity spreads members across
        # kubernetes.io/hostname by default; only a declared topology key
        # renders. "none" is the operator's own OFF switch and passes
        # through verbatim.
        affinity = try(rs.scheduling.anti_affinity_topology_key, "") != "" ? {
          antiAffinityTopologyKey = rs.scheduling.anti_affinity_topology_key
        } : null

        nodeSelector = length(try(rs.scheduling.node_selector, {})) > 0 ? rs.scheduling.node_selector : null

        tolerations = length(try(rs.scheduling.tolerations, [])) > 0 ? [
          for t in rs.scheduling.tolerations : {
            for tk, tv in {
              key               = try(t.key, "") != "" ? t.key : null
              operator          = try(t.operator, "") != "" ? t.operator : null
              value             = try(t.value, "") != "" ? t.value : null
              effect            = try(t.effect, "") != "" ? t.effect : null
              tolerationSeconds = try(t.toleration_seconds, null)
            } : tk => tv if tv != null
          }
        ] : null

        priorityClassName = try(rs.scheduling.priority_class_name, "") != "" ? rs.scheduling.priority_class_name : null

        # PDB renders only when a bound is declared (the spec CEL forbids
        # both); an absent key leaves the operator default (max one
        # member down).
        podDisruptionBudget = (
          try(rs.pod_disruption_budget.max_unavailable, 0) > 0 || try(rs.pod_disruption_budget.min_available, 0) > 0
          ) ? {
          for pk, pv in {
            maxUnavailable = try(rs.pod_disruption_budget.max_unavailable, 0) > 0 ? rs.pod_disruption_budget.max_unavailable : null
            minAvailable   = try(rs.pod_disruption_budget.min_available, 0) > 0 ? rs.pod_disruption_budget.min_available : null
          } : pk => pv if pv != null
        } : null

        # Per-member Services (the managed-cloud LoadBalancer /
        # cross-cluster recipe surface).
        expose = try(rs.expose.enabled, false) ? {
          for ek, ev in {
            enabled     = true
            type        = coalesce(try(rs.expose.type, null), "ClusterIP")
            annotations = length(try(rs.expose.annotations, {})) > 0 ? rs.expose.annotations : null
          } : ek => ev if ev != null
        } : null

        # Arbiter: votes, no data. Its affinity mirrors the set's so the
        # arbiter spreads across the same topology as the members it
        # breaks ties for.
        arbiter = try(rs.arbiter.enabled, false) ? {
          for ak, av in {
            enabled = true
            size    = coalesce(try(rs.arbiter.size, null), 1)
            affinity = try(rs.scheduling.anti_affinity_topology_key, "") != "" ? {
              antiAffinityTopologyKey = rs.scheduling.anti_affinity_topology_key
            } : null
          } : ak => av if av != null
        } : null
      } : k => v if v != null
    }
  ]

  # ---- CR: sharding ----------------------------------------------------------
  # OMITTED entirely unless enabled: the operator's zero value for an
  # absent key is sharding disabled, and only an enabled topology
  # requires configsvr/mongos declarations (spec CEL mirrors that).
  sharding_body = !local.sharding_enabled ? null : {
    enabled = true

    # The balancer default is enabled upstream; the flag renders
    # explicitly either way so flipping it is a clean diff.
    balancer = {
      enabled = try(var.spec.sharding.balancer_enabled, null) == null ? true : var.spec.sharding.balancer_enabled
    }

    configsvrReplSet = {
      for k, v in {
        size = coalesce(try(var.spec.sharding.config_server.size, null), 3)

        resources = try(var.spec.sharding.config_server.resources, null) == null ? null : {
          for rk, rv in {
            limits = try(var.spec.sharding.config_server.resources.limits, null) == null ? null : {
              for lk, lv in {
                cpu    = try(var.spec.sharding.config_server.resources.limits.cpu, "") != "" ? var.spec.sharding.config_server.resources.limits.cpu : null
                memory = try(var.spec.sharding.config_server.resources.limits.memory, "") != "" ? var.spec.sharding.config_server.resources.limits.memory : null
              } : lk => lv if lv != null
            }
            requests = try(var.spec.sharding.config_server.resources.requests, null) == null ? null : {
              for rk2, rv2 in {
                cpu    = try(var.spec.sharding.config_server.resources.requests.cpu, "") != "" ? var.spec.sharding.config_server.resources.requests.cpu : null
                memory = try(var.spec.sharding.config_server.resources.requests.memory, "") != "" ? var.spec.sharding.config_server.resources.requests.memory : null
              } : rk2 => rv2 if rv2 != null
            }
          } : rk => rv if rv != null
        }

        volumeSpec = {
          persistentVolumeClaim = {
            for pk, pv in {
              storageClassName = try(var.spec.sharding.config_server.storage.storage_class, "") != "" ? var.spec.sharding.config_server.storage.storage_class : null
              resources        = { requests = { storage = var.spec.sharding.config_server.storage.size } }
            } : pk => pv if pv != null
          }
        }
      } : k => v if v != null
    }

    mongos = {
      for k, v in {
        size = coalesce(try(var.spec.sharding.mongos.size, null), 3)

        resources = try(var.spec.sharding.mongos.resources, null) == null ? null : {
          for rk, rv in {
            limits = try(var.spec.sharding.mongos.resources.limits, null) == null ? null : {
              for lk, lv in {
                cpu    = try(var.spec.sharding.mongos.resources.limits.cpu, "") != "" ? var.spec.sharding.mongos.resources.limits.cpu : null
                memory = try(var.spec.sharding.mongos.resources.limits.memory, "") != "" ? var.spec.sharding.mongos.resources.limits.memory : null
              } : lk => lv if lv != null
            }
            requests = try(var.spec.sharding.mongos.resources.requests, null) == null ? null : {
              for rk2, rv2 in {
                cpu    = try(var.spec.sharding.mongos.resources.requests.cpu, "") != "" ? var.spec.sharding.mongos.resources.requests.cpu : null
                memory = try(var.spec.sharding.mongos.resources.requests.memory, "") != "" ? var.spec.sharding.mongos.resources.requests.memory : null
              } : rk2 => rv2 if rv2 != null
            }
          } : rk => rv if rv != null
        }

        # The mongos Service always exists (upstream MongosExpose has NO
        # enabled field — unlike the per-set ExposeTogglable); the spec's
        # expose.enabled gates whether the module renders customization
        # over the operator's ClusterIP default.
        expose = try(var.spec.sharding.mongos.expose.enabled, false) ? {
          for ek, ev in {
            type        = coalesce(try(var.spec.sharding.mongos.expose.type, null), "ClusterIP")
            annotations = length(try(var.spec.sharding.mongos.expose.annotations, {})) > 0 ? var.spec.sharding.mongos.expose.annotations : null
          } : ek => ev if ev != null
        } : null
      } : k => v if v != null
    }
  }

  # ---- CR: users --------------------------------------------------------------
  # Declarative application users. passwordSecretRef renders ONLY when a
  # password is declared (the module materializes that Secret); otherwise
  # the operator generates a password into its own per-user Secret.
  users = [
    for u in var.spec.users : {
      for k, v in {
        name  = u.name
        db    = coalesce(try(u.db, null), "admin")
        roles = [for r in u.roles : { name = r.name, db = r.db }]
        passwordSecretRef = try(u.password, "") != "" ? {
          name = "${local.cluster_name}-user-${u.name}"
          key  = "password"
        } : null
      } : k => v if v != null
    }
  ]

  # ---- CR: backup ---------------------------------------------------------------
  # storages is a MAP keyed by storage name (the CRD shape); tasks and
  # PITR reference entries by that name. credentialsSecret renders for
  # every arm that carries credentials — the module-materialized
  # `<name>-backup-<storage>` or the user's existing Secret — and is
  # omitted only on the keyless S3 arm (the pods' ambient AWS identity).
  # GCS always renders one: the CRD requires it.
  backup_storages = local.backup == null ? null : {
    for s in local.backup.storages : s.name => {
      for k, v in {
        main = try(s.main, false) ? true : null
        # Exactly one backend arm exists (spec oneof). The r2 arm rides the
        # operator's s3 block (R2 speaks S3) through backup_s3_api_storages.
        type = try(s.s3, null) != null || try(s.r2, null) != null ? "s3" : try(s.gcs, null) != null ? "gcs" : "azure"

        s3 = !contains(keys(local.backup_s3_api_storages), s.name) ? null : {
          for sk, sv in {
            bucket                = local.backup_s3_api_storages[s.name].bucket
            region                = local.backup_s3_api_storages[s.name].region != "" ? local.backup_s3_api_storages[s.name].region : null
            prefix                = local.backup_s3_api_storages[s.name].prefix != "" ? local.backup_s3_api_storages[s.name].prefix : null
            endpointUrl           = local.backup_s3_api_storages[s.name].endpoint_url != "" ? local.backup_s3_api_storages[s.name].endpoint_url : null
            forcePathStyle        = local.backup_s3_api_storages[s.name].force_path_style ? true : null
            insecureSkipTLSVerify = local.backup_s3_api_storages[s.name].insecure_skip_tls ? true : null
            credentialsSecret     = local.backup_s3_api_storages[s.name].has_credentials ? "${local.cluster_name}-backup-${s.name}" : null
          } : sk => sv if sv != null
        }

        gcs = try(s.gcs, null) == null ? null : {
          for gk, gv in {
            bucket            = s.gcs.bucket
            prefix            = try(s.gcs.prefix, "") != "" ? s.gcs.prefix : null
            credentialsSecret = try(s.gcs.credentials.existing_secret_name, "") != "" ? s.gcs.credentials.existing_secret_name : "${local.cluster_name}-backup-${s.name}"
          } : gk => gv if gv != null
        }

        azure = try(s.azure, null) == null ? null : {
          for ak, av in {
            container         = s.azure.container
            prefix            = try(s.azure.prefix, "") != "" ? s.azure.prefix : null
            endpointUrl       = try(s.azure.endpoint_url, "") != "" ? s.azure.endpoint_url : null
            credentialsSecret = "${local.cluster_name}-backup-${s.name}"
          } : ak => av if av != null
        }
      } : k => v if v != null
    }
  }

  # Scheduled tasks: enabled is the inverse of the spec's suspend (the
  # declaration survives a suspension); retention renders only when keep
  # is declared, always type "count" (the only retention the operator
  # models).
  backup_tasks = local.backup == null ? null : [
    for t in local.backup.tasks : {
      for k, v in {
        name            = t.name
        enabled         = !try(t.suspend, false)
        schedule        = t.schedule
        storageName     = t.storage_name
        type            = coalesce(try(t.type, null), "logical")
        compressionType = coalesce(try(t.compression, null), "gzip")
        retention = try(t.keep, null) == null ? null : {
          count             = t.keep
          type              = "count"
          deleteFromStorage = try(t.delete_from_storage, null) == null ? true : t.delete_from_storage
        }
      } : k => v if v != null
    }
  ]

  # PITR: oplog chunks land on the main storage. Rendered with the
  # spec's declared defaults applied (10-minute chunks, gzip).
  pitr_body = try(local.backup.pitr, null) == null ? null : {
    for k, v in {
      enabled         = local.backup.pitr.enabled
      oplogOnly       = try(local.backup.pitr.oplog_only, false) ? true : null
      oplogSpanMin    = coalesce(try(local.backup.pitr.oplog_span_min, null), 10)
      compressionType = coalesce(try(local.backup.pitr.compression, null), "gzip")
    } : k => v if v != null
  }

  backup_body = local.backup == null ? null : {
    for k, v in {
      enabled  = true
      image    = local.backup_image
      storages = local.backup_storages
      pitr     = local.pitr_body
      tasks    = length(local.backup_tasks) > 0 ? local.backup_tasks : null
    } : k => v if v != null
  }

  # ---- CR: logcollector -----------------------------------------------------
  # Rendered only when the spec declares the block; the enabled flag
  # defaults true within it. An ABSENT key means no sidecar (operator
  # v1.22.0: IsLogCollectorEnabled() requires the block present AND
  # enabled).
  logcollector_body = try(var.spec.log_collector, null) == null ? null : {
    for k, v in {
      enabled = try(var.spec.log_collector.enabled, null) == null ? true : var.spec.log_collector.enabled
      image   = local.logcollector_image

      resources = try(var.spec.log_collector.resources, null) == null ? null : {
        for rk, rv in {
          limits = try(var.spec.log_collector.resources.limits, null) == null ? null : {
            for lk, lv in {
              cpu    = try(var.spec.log_collector.resources.limits.cpu, "") != "" ? var.spec.log_collector.resources.limits.cpu : null
              memory = try(var.spec.log_collector.resources.limits.memory, "") != "" ? var.spec.log_collector.resources.limits.memory : null
            } : lk => lv if lv != null
          }
          requests = try(var.spec.log_collector.resources.requests, null) == null ? null : {
            for rk2, rv2 in {
              cpu    = try(var.spec.log_collector.resources.requests.cpu, "") != "" ? var.spec.log_collector.resources.requests.cpu : null
              memory = try(var.spec.log_collector.resources.requests.memory, "") != "" ? var.spec.log_collector.resources.requests.memory : null
            } : rk2 => rv2 if rv2 != null
          }
        } : rk => rv if rv != null
      }
    } : k => v if v != null
  }

  # ---- CR: restore -------------------------------------------------------------
  # A Restore object is a RUN, not a state: the operator drives it to
  # ready (or error) once and never re-reads its spec. Declarative
  # semantics therefore hinge on the NAME — `<cluster>-restore-<8 hex>`
  # hashing the declaration itself, so an unchanged declaration is a
  # no-op on every apply and a changed one is a new object and a new run.
  # The canonical string joins every field that changes WHAT is restored
  # in a fixed order (remapping pairs sorted); the Pulumi twin
  # (restoreName in restore.go) hashes the identical string, so both
  # engines name the same run the same way.
  restore = try(var.spec.restore, null)

  restore_source_type = coalesce(try(local.restore.backup_source.type, null), "logical")

  restore_name = local.restore == null ? null : "${local.cluster_name}-restore-${substr(sha256(join("|", [
    try(local.restore.backup_name, ""),
    try(local.restore.backup_source.storage_name, ""),
    try(local.restore.backup_source.destination, ""),
    try(local.restore.backup_source, null) != null ? local.restore_source_type : "",
    try(local.restore.pitr.type, ""),
    try(local.restore.pitr.date, ""),
    join(",", sort([for k, v in try(local.restore.replset_remapping, {}) : "${k}=${v}"])),
  ])), 0, 8)}"

  # Exactly one source arm exists (spec oneof): a same-namespace Backup
  # object by name, or a location in one of this cluster's declared
  # storages. The backup-source arm renders the storage TWICE, deliberately:
  # as storageName AND as the storage's full block inside backupSource. The
  # operator reads them on two different paths — validation resolves the
  # storage through storageName, but the metadata resync it runs when the
  # backup is unknown to the fresh cluster's PBM (every DR restore) resolves
  # it from backupSource.{gcs|s3|azure} alone, and a Restore without that
  # block dies terminal with "unsupported backup storage type" — live-caught
  # on GKE. The block is the cluster's own backup.storages rendering (minus
  # `main`/`type`, which the Restore's schema does not carry), so credentials
  # and prefix can never drift between the two.
  restore_source_storage = try(local.restore.backup_source, null) == null ? null : {
    for k, v in local.backup_storages[local.restore.backup_source.storage_name] : k => v if k != "main" && k != "type"
  }

  restore_manifest = local.restore == null ? null : {
    apiVersion = "psmdb.percona.com/v1"
    kind       = "PerconaServerMongoDBRestore"
    metadata = {
      name      = local.restore_name
      namespace = local.namespace
      labels    = local.labels
    }
    spec = {
      for k, v in {
        clusterName = local.cluster_name
        backupName  = try(local.restore.backup_name, "") != "" ? local.restore.backup_name : null
        storageName = try(local.restore.backup_source, null) != null ? local.restore.backup_source.storage_name : null
        backupSource = try(local.restore.backup_source, null) == null ? null : merge({
          destination = local.restore.backup_source.destination
          type        = local.restore_source_type
        }, local.restore_source_storage)
        pitr = try(local.restore.pitr, null) == null ? null : {
          for pk, pv in {
            type = local.restore.pitr.type
            date = try(local.restore.pitr.date, "") != "" ? local.restore.pitr.date : null
          } : pk => pv if pv != null
        }
        replsetRemapping = length(try(local.restore.replset_remapping, {})) > 0 ? local.restore.replset_remapping : null
      } : k => v if v != null
    }
  }

  # ---- the PerconaServerMongoDB CR ---------------------------------------------
  mongodb_manifest = {
    apiVersion = "psmdb.percona.com/v1"
    kind       = "PerconaServerMongoDB"
    metadata = {
      name       = local.cluster_name
      namespace  = local.namespace
      labels     = local.labels
      finalizers = local.finalizers
    }
    spec = {
      for k, v in {
        crVersion = local.cr_version
        image     = try(var.spec.image_name, "") != "" ? var.spec.image_name : local.default_image

        # SmartUpdate (the upstream default) unless the spec diverges —
        # rendered explicitly so the update posture is visible in the CR.
        updateStrategy = coalesce(try(var.spec.update_strategy, null), "SmartUpdate")

        # Module-owned constants: the version service is upstream's, and
        # automated version application is deliberately not modeled —
        # versions change by editing image_name, never behind the
        # module's back.
        upgradeOptions = {
          versionServiceEndpoint = local.version_service_endpoint
          apply                  = "disabled"
        }

        pause = try(var.spec.pause, false) ? true : null

        imagePullSecrets = length(var.spec.image_pull_secrets) > 0 ? [
          for s in var.spec.image_pull_secrets : { name = s }
        ] : null

        unsafeFlags = length(local.unsafe_flags) > 0 ? local.unsafe_flags : null

        tls = local.tls_body

        secrets = { users = local.users_secret_name }

        replsets = local.replsets
        sharding = local.sharding_body

        users = length(local.users) > 0 ? local.users : null

        backup       = local.backup_body
        logcollector = local.logcollector_body
      } : k => v if v != null
    }
  }
}
