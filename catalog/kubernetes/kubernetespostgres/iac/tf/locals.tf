# Computed values for the KubernetesPostgres module. Every resolution here
# has an exact twin in the Pulumi module's locals.go / cluster.go /
# bootstrap.go / backup.go / secrets.go — keep them in lockstep: same
# resource names, same rendered CR bodies, same Secret names/keys/types.
#
# HCL DISCIPLINE (applies to every conditional object in this file):
# conditional entries are written as `key = cond ? value : null` inside ONE
# object literal, pruned with `{ for k, v in {...} : k => v if v != null }`.
# The two tempting alternatives are both broken: `cond ? {...} : {}`
# ternaries fail plan-time type unification when branches carry different
# attributes, and `merge(concat(cond ? [{...}] : [], ...)...)` silently
# UNIFIES primitive-only sibling objects into map(string) — numbers and
# booleans arrive at the API as strings and server-side validation rejects
# the object. The null-prune form preserves every value's type: instances,
# synchronous.number, maxParallel, jobs, tolerationSeconds, connectionLimit
# render as YAML numbers; the presence-gated booleans as booleans.
#
# Optional nested blocks are read with try(): HCL's && does NOT
# short-circuit, so chained null checks still dereference the null.
#
# PRESENCE-SENSITIVE FIELDS (rendered only on divergence from the CRD/
# operator default, exactly like the Pulumi module):
#   - enablePDB: absent already means true to the operator — only an
#     explicit false is rendered.
#   - storage.resizeInUseVolumes: CRD default true — only false renders.
#   - affinity.podAntiAffinityType: operator default "preferred" — only
#     "required" renders.
#   - roles[].ensure: "present" is the default — only "absent" renders.
#   - roles[].connectionLimit: -1 (unlimited) is the engine default — only
#     a real limit renders.

locals {
  # ClusterName is metadata.name — the naming root CloudNativePG derives
  # every object from: pods `<name>-N`, services `<name>-rw/-ro/-r`,
  # credential secrets `<name>-app` / `<name>-superuser`.
  cluster_name = var.metadata.name
  namespace    = var.spec.namespace

  # Resource-identity labels stamped on every module-created object
  # (namespace, Cluster, ObjectStores, ScheduledBackups, credential
  # Secrets). CloudNativePG derives ITS objects' identity from the Cluster
  # name; these labels tie the whole family back to the Planton resource.
  labels = merge(
    {
      "planton.ai/resource"      = "true"
      "planton.ai/resource-name" = var.metadata.name
      "planton.ai/resource-kind" = "KubernetesPostgres"
    },
    var.metadata.id != null && var.metadata.id != "" ? { "planton.ai/resource-id" = var.metadata.id } : {},
    var.metadata.org != null && var.metadata.org != "" ? { "planton.ai/organization" = var.metadata.org } : {},
    var.metadata.env != null && var.metadata.env != "" ? { "planton.ai/environment" = var.metadata.env } : {}
  )

  # Traffic service names (operator-created; exported as outputs).
  rw_service_name = "${local.cluster_name}-rw"
  ro_service_name = "${local.cluster_name}-ro"
  r_service_name  = "${local.cluster_name}-r"
  kube_endpoint   = "${local.cluster_name}-rw.${local.namespace}.svc.cluster.local:5432"

  # BarmanCloudPluginName is the CNPG-I plugin identifier the Cluster's
  # plugins list and every ScheduledBackup reference — the plugin's
  # registered name, fixed by upstream.
  barman_cloud_plugin_name = "barman-cloud.cloudnative-pg.io"

  # Name of the synthetic externalClusters entry recovery bootstraps read
  # from. Any stable label works (the entry only exists to carry the
  # recovery ObjectStore reference); "origin" follows upstream's examples.
  recovery_source_external_cluster_name = "origin"

  # Deterministic names for the module-created satellites. Deterministic
  # (never engine-generated suffixes) so the import recipes can derive
  # them blind and both engines agree byte-for-byte.
  backup_object_store_name       = local.cluster_name
  recovery_object_store_name     = "${local.cluster_name}-recovery-source"
  backup_creds_secret_name       = "${local.cluster_name}-backup-creds"
  recovery_creds_secret_name     = "${local.cluster_name}-recovery-creds"
  backup_endpoint_ca_name        = "${local.cluster_name}-backup-endpoint-ca"
  recovery_endpoint_ca_name      = "${local.cluster_name}-recovery-endpoint-ca"
  provided_app_secret_name       = "${local.cluster_name}-app-provided"
  provided_superuser_secret_name = "${local.cluster_name}-superuser-provided"
  operator_superuser_secret_name = "${local.cluster_name}-superuser"
  app_secret_name                = "${local.cluster_name}-app"

  # Bootstrap arms (the spec's oneof guarantees at most one is non-null).
  initdb        = try(var.spec.bootstrap.initdb, null)
  recovery      = try(var.spec.bootstrap.recovery, null)
  pg_basebackup = try(var.spec.bootstrap.pg_basebackup, null)
  backup        = try(var.spec.backup, null)

  # Where the application credential actually lives: the operator-generated
  # `<name>-app` normally, the module-provided secret when initdb declares
  # an owner password, or the brought Secret when a recovery names the
  # source's app Secret (credential continuity) — in both latter cases the
  # operator adopts the Secret instead of generating its own.
  effective_app_secret_name = (
    try(local.recovery.owner_secret_name, "") != "" ? local.recovery.owner_secret_name :
    try(local.initdb.owner_password, "") != "" ? local.provided_app_secret_name :
    local.app_secret_name
  )

  # basic-auth username for the provided app secret: the OWNER's name —
  # falling back to the database name and then the upstream initdb default
  # ("app"), the same chain the operator itself applies.
  provided_app_username = (
    try(local.initdb.owner, "") != "" ? local.initdb.owner :
    try(local.initdb.database, "") != "" ? local.initdb.database :
    "app"
  )

  # Populated only when superuser access is enabled — the operator deletes
  # the secret (and blanks the password) otherwise.
  superuser_secret_name_output = try(var.spec.superuser.enabled, false) ? (
    try(var.spec.superuser.password, "") != "" ? local.provided_superuser_secret_name : local.operator_superuser_secret_name
  ) : ""

  # ---- credential Secret payloads -----------------------------------------
  # Per-role basic-auth pairs (`<name>-role-<role>`) — the exact shape
  # CloudNativePG watches; rotating the value rotates the database password.
  # Keyed by the FULL Secret name (not the bare role name): the state
  # address key is what the import recipes derive the live object name from
  # (from_address_key), so the key and the rendered name must be identical.
  role_password_secrets = {
    for r in var.spec.roles : "${local.cluster_name}-role-${r.name}" => {
      username = r.name
      password = r.password
    } if try(r.password, "") != ""
  }

  # Per-external-cluster passwords (single `password` key; the operator
  # renders a passfile from it).
  # Keyed by the FULL Secret name — same from_address_key contract as the
  # role secrets above.
  external_cluster_password_secrets = {
    for e in var.spec.external_clusters : "${local.cluster_name}-ext-${e.name}" => e.password if try(e.password, "") != ""
  }

  # ---- Barman Cloud ObjectStores -------------------------------------------
  # One rendering context per store so the backup store (named after the
  # cluster) and the recovery-source store (`<name>-recovery-source`)
  # share the exact same configuration rendering. Recovery reads an
  # EXISTING archive: retention never applies to it (the plugin must not
  # prune the source cluster's backups).
  object_store_contexts = merge(
    local.backup != null ? {
      backup = {
        store_name              = local.backup_object_store_name
        object_store            = local.backup.object_store
        retention_policy        = try(local.backup.retention_policy, "")
        creds_secret_name       = local.backup_creds_secret_name
        endpoint_ca_secret_name = local.backup_endpoint_ca_name
      }
    } : {},
    local.recovery != null ? {
      recovery = {
        store_name              = local.recovery_object_store_name
        object_store            = local.recovery.object_store
        retention_policy        = ""
        creds_secret_name       = local.recovery_creds_secret_name
        endpoint_ca_secret_name = local.recovery_endpoint_ca_name
      }
    } : {}
  )

  # Cloudflare R2 arms, translated into what Barman Cloud's S3 code path
  # needs. The spec is in R2's own vocabulary (account, jurisdiction, a
  # Cloudflare credential); this local derives the S3 endpoint from the
  # account and the bucket's jurisdiction -- an eu/fedramp/us bucket is
  # served ONLY through <account>.<jurisdiction>.r2.cloudflarestorage.com,
  # the default host fails rather than redirects. The host table mirrors
  # the Go helper package pkg/cloudflare/r2 (the source of truth both
  # engines follow). The jurisdiction is an optional scalar inside a
  # present block, so it is read null-safely (tfvars carries null for an
  # unset optional; coalesce rejects it and try() turns that into "").
  object_store_r2 = {
    for key, ctx in local.object_store_contexts : key => (
      try(ctx.object_store.r2, null) == null ? null : {
        jurisdiction = coalesce(try(coalesce(ctx.object_store.r2.jurisdiction), ""), "default")
        endpoint = (
          coalesce(try(coalesce(ctx.object_store.r2.jurisdiction), ""), "default") == "default"
          ? "https://${ctx.object_store.r2.account_id}.r2.cloudflarestorage.com"
          : "https://${ctx.object_store.r2.account_id}.${ctx.object_store.r2.jurisdiction}.r2.cloudflarestorage.com"
        )
      }
    )
  }

  # The region each store's S3 credentials carry: "auto" for R2 (the only
  # region it accepts -- Cloudflare aliases "" and us-east-1 to it, but the
  # plugin needs an explicit value to set AWS_DEFAULT_REGION), the declared
  # region for the s3 arm, "" (no region Secret) otherwise.
  object_store_region = {
    for key, ctx in local.object_store_contexts : key => (
      local.object_store_r2[key] != null ? "auto" : try(coalesce(ctx.object_store.s3.region), "")
    )
  }

  # Declared credentials for each store, materialized as one deterministic
  # Opaque Secret per store (`<name>-backup-creds` / `<name>-recovery-creds`)
  # whose keys depend on the backend arm. Keyless arms render the backend's
  # ambient-identity flag instead and need NO creds Secret — except Azure
  # keyless, where the storage account still identifies the endpoint and
  # rides the Secret (AZURE_STORAGE_ACCOUNT) even without a key. The r2
  # arm's pair is the CloudflareAccountApiToken's r2_access_key_id /
  # r2_secret_access_key outputs as declared (that kind derives them; no
  # hashing happens here).
  # All values are strings, so this chained ternary unifies safely to
  # map(string) — no number/bool stringification risk here.
  object_store_creds_data = {
    for key, ctx in local.object_store_contexts : key => (
      try(ctx.object_store.s3.access_keys, null) != null ? {
        ACCESS_KEY_ID     = ctx.object_store.s3.access_keys.access_key_id
        SECRET_ACCESS_KEY = ctx.object_store.s3.access_keys.secret_access_key
        } : local.object_store_r2[key] != null ? {
        ACCESS_KEY_ID     = ctx.object_store.r2.credentials.access_key_id
        SECRET_ACCESS_KEY = ctx.object_store.r2.credentials.secret_access_key
        } : try(ctx.object_store.gcs, null) != null && !try(ctx.object_store.gcs.keyless, false) ? {
        APPLICATION_CREDENTIALS = ctx.object_store.gcs.service_account_key_json
        } : try(ctx.object_store.azure_blob, null) == null ? null : (
        try(ctx.object_store.azure_blob.keyless, false) ? {
          AZURE_STORAGE_ACCOUNT = ctx.object_store.azure_blob.storage_account
          } : try(ctx.object_store.azure_blob.connection_string, "") != "" ? {
          AZURE_STORAGE_CONNECTION_STRING = ctx.object_store.azure_blob.connection_string
          } : {
          AZURE_STORAGE_ACCOUNT = ctx.object_store.azure_blob.storage_account
          AZURE_STORAGE_KEY     = ctx.object_store.azure_blob.storage_key
        }
      )
    )
  }

  # The ObjectStore CR bodies. The CRD FORBIDS configuration.serverName —
  # the plugin takes the source server name as a per-Cluster plugin
  # parameter instead (see the synthetic "origin" externalClusters entry)
  # — so this rendering never sets it.
  #
  # The CRD models the S3 region as a SecretKeySelector (not a plain
  # string), so the literal region rides its own deterministic single-key
  # Secret (`<store>-region`, key AWS_REGION). barman-cloud reads it ONLY
  # on the declared-key path (its credential resolver returns before the
  # region on inheritFromIAMRole, and the AWS SDK derives the region from
  # the pod's IRSA environment there), so on a keyless store the Secret is
  # rendered but inert -- kept for a uniform shape, not for effect.
  #
  # The r2 arm rides s3Credentials too (R2 speaks S3): the same creds and
  # region Secrets, the endpoint from object_store_r2, and the plugin
  # sidecar told to add data-integrity checksums only when an operation
  # requires them (barman-cloud's boto3 attaches them by default, which
  # S3-compatible stores may reject; the plugin's documented posture for
  # them, and free on a store that accepts the checksums).
  object_store_manifests = {
    for key, ctx in local.object_store_contexts : key => {
      apiVersion = "barmancloud.cnpg.io/v1"
      kind       = "ObjectStore"
      metadata = {
        name      = ctx.store_name
        namespace = local.namespace
        labels    = local.labels
      }
      spec = {
        for sk, sv in {
          configuration = {
            for ck, cv in {
              destinationPath = ctx.object_store.destination_path
              endpointURL     = try(ctx.object_store.s3.endpoint_url, "") != "" ? ctx.object_store.s3.endpoint_url : (local.object_store_r2[key] != null ? local.object_store_r2[key].endpoint : null)
              endpointCA      = try(ctx.object_store.s3.endpoint_ca_pem, "") != "" ? { name = ctx.endpoint_ca_secret_name, key = "ca.crt" } : null

              # One object literal carrying both S3-API arms' keys (s3 and
              # r2), each gated on its arm and null-pruned -- never a
              # per-arm ternary between differently shaped objects.
              s3Credentials = try(ctx.object_store.s3, null) == null && local.object_store_r2[key] == null ? null : {
                for k, v in {
                  inheritFromIAMRole = try(ctx.object_store.s3.keyless, false) ? true : null
                  accessKeyId        = try(ctx.object_store.s3.access_keys, null) != null || local.object_store_r2[key] != null ? { name = ctx.creds_secret_name, key = "ACCESS_KEY_ID" } : null
                  secretAccessKey    = try(ctx.object_store.s3.access_keys, null) != null || local.object_store_r2[key] != null ? { name = ctx.creds_secret_name, key = "SECRET_ACCESS_KEY" } : null
                  region             = local.object_store_region[key] != "" ? { name = "${ctx.store_name}-region", key = "AWS_REGION" } : null
                } : k => v if v != null
              }

              googleCredentials = try(ctx.object_store.gcs, null) == null ? null : {
                for k, v in {
                  gkeEnvironment         = try(ctx.object_store.gcs.keyless, false) ? true : null
                  applicationCredentials = try(ctx.object_store.gcs.keyless, false) ? null : { name = ctx.creds_secret_name, key = "APPLICATION_CREDENTIALS" }
                } : k => v if v != null
              }

              azureCredentials = try(ctx.object_store.azure_blob, null) == null ? null : {
                for k, v in {
                  inheritFromAzureAD = try(ctx.object_store.azure_blob.keyless, false) ? true : null
                  connectionString   = !try(ctx.object_store.azure_blob.keyless, false) && try(ctx.object_store.azure_blob.connection_string, "") != "" ? { name = ctx.creds_secret_name, key = "AZURE_STORAGE_CONNECTION_STRING" } : null
                  storageAccount     = try(ctx.object_store.azure_blob.keyless, false) || try(ctx.object_store.azure_blob.connection_string, "") == "" ? { name = ctx.creds_secret_name, key = "AZURE_STORAGE_ACCOUNT" } : null
                  storageKey         = !try(ctx.object_store.azure_blob.keyless, false) && try(ctx.object_store.azure_blob.connection_string, "") == "" ? { name = ctx.creds_secret_name, key = "AZURE_STORAGE_KEY" } : null
                } : k => v if v != null
              }

              wal = try(ctx.object_store.wal, null) == null ? null : {
                for k, v in {
                  compression = try(ctx.object_store.wal.compression, "") != "" ? ctx.object_store.wal.compression : null
                  maxParallel = try(ctx.object_store.wal.max_parallel, null)
                } : k => v if v != null
              }

              data = try(ctx.object_store.data, null) == null ? null : {
                for k, v in {
                  compression         = try(ctx.object_store.data.compression, "") != "" ? ctx.object_store.data.compression : null
                  jobs                = try(ctx.object_store.data.jobs, null)
                  immediateCheckpoint = try(ctx.object_store.data.immediate_checkpoint, false) ? true : null
                } : k => v if v != null
              }
            } : ck => cv if cv != null
          }
          # retentionPolicy only ever renders on the BACKUP store (the
          # recovery context pins it empty above).
          retentionPolicy = ctx.retention_policy != "" ? ctx.retention_policy : null
          # The r2 arm's sidecar posture (see the comment above this local).
          instanceSidecarConfiguration = local.object_store_r2[key] == null ? null : {
            env = [
              { name = "AWS_REQUEST_CHECKSUM_CALCULATION", value = "when_required" },
              { name = "AWS_RESPONSE_CHECKSUM_VALIDATION", value = "when_required" },
            ]
          }
        } : sk => sv if sv != null
      }
    }
  }

  # ---- Cluster CR: bootstrap ------------------------------------------------
  # Bootstrap is how the cluster is BORN, so it is immutable — the operator
  # ignores changes after the first reconcile.
  initdb_body = local.initdb == null ? null : {
    for k, v in {
      database = try(local.initdb.database, "") != "" ? local.initdb.database : null
      owner    = try(local.initdb.owner, "") != "" ? local.initdb.owner : null
      # The module materialized the declared owner password as a basic-auth
      # Secret; the operator adopts it as the app credential instead of
      # generating `<name>-app`.
      secret        = try(local.initdb.owner_password, "") != "" ? { name = local.provided_app_secret_name } : null
      dataChecksums = try(local.initdb.data_checksums, false) ? true : null
      encoding      = try(local.initdb.encoding, "") != "" ? local.initdb.encoding : null
      localeCollate = try(local.initdb.locale_collate, "") != "" ? local.initdb.locale_collate : null
      localeCType   = try(local.initdb.locale_ctype, "") != "" ? local.initdb.locale_ctype : null

      postInitSQL            = length(try(local.initdb.post_init_sql, [])) > 0 ? local.initdb.post_init_sql : null
      postInitApplicationSQL = length(try(local.initdb.post_init_application_sql, [])) > 0 ? local.initdb.post_init_application_sql : null

      import = try(local.initdb.import, null) == null ? null : {
        for ik, iv in {
          type       = local.initdb.import.type
          source     = { externalCluster = local.initdb.import.source_external_cluster }
          databases  = local.initdb.import.databases
          roles      = length(try(local.initdb.import.roles, [])) > 0 ? local.initdb.import.roles : null
          schemaOnly = try(local.initdb.import.schema_only, false) ? true : null
        } : ik => iv if iv != null
      }
    } : k => v if v != null
  }

  # Recovery reads through the synthetic externalClusters entry ("origin")
  # the module renders below, whose plugin block points at the
  # `<name>-recovery-source` ObjectStore with the SOURCE cluster's
  # serverName.
  # Credential continuity: the recovered roles keep the SOURCE's passwords,
  # so when the spec names the source's app Secret the operator adopts it
  # (`secret`) instead of generating a fresh `<name>-app`.
  recovery_body = local.recovery == null ? null : {
    for k, v in {
      source   = local.recovery_source_external_cluster_name
      database = try(local.recovery.database, "") != "" ? local.recovery.database : null
      owner    = try(local.recovery.owner, "") != "" ? local.recovery.owner : null
      secret   = try(local.recovery.owner_secret_name, "") != "" ? { name = local.recovery.owner_secret_name } : null
      recoveryTarget = try(local.recovery.recovery_target, null) == null ? null : {
        for tk, tv in {
          targetTime      = try(local.recovery.recovery_target.target_time, "") != "" ? local.recovery.recovery_target.target_time : null
          targetLSN       = try(local.recovery.recovery_target.target_lsn, "") != "" ? local.recovery.recovery_target.target_lsn : null
          targetName      = try(local.recovery.recovery_target.target_name, "") != "" ? local.recovery.recovery_target.target_name : null
          targetImmediate = try(local.recovery.recovery_target.target_immediate, false) ? true : null
          backupID        = try(local.recovery.recovery_target.backup_id, "") != "" ? local.recovery.recovery_target.backup_id : null
        } : tk => tv if tv != null
      }
    } : k => v if v != null
  }

  pg_basebackup_body = local.pg_basebackup == null ? null : {
    source = local.pg_basebackup.source
  }

  bootstrap_body = {
    for k, v in {
      initdb        = local.initdb_body
      recovery      = local.recovery_body
      pg_basebackup = local.pg_basebackup_body
    } : k => v if v != null
  }

  # ---- Cluster CR: externalClusters ------------------------------------------
  # The user-declared entries (pg_basebackup / import sources) plus — when
  # the bootstrap is a recovery — the synthetic "origin" entry that carries
  # the recovery-source ObjectStore reference. serverName rides the plugin
  # parameters: it is the folder the SOURCE cluster wrote in the store (its
  # cluster name), and the ObjectStore CRD forbids it inline.
  declared_external_clusters = [
    for e in var.spec.external_clusters : {
      for k, v in {
        name                 = e.name
        connectionParameters = length(try(e.connection_parameters, {})) > 0 ? e.connection_parameters : null
        password             = try(e.password, "") != "" ? { name = "${local.cluster_name}-ext-${e.name}", key = "password" } : null
      } : k => v if v != null
    }
  ]

  external_clusters = concat(
    local.declared_external_clusters,
    local.recovery == null ? [] : [{
      name = local.recovery_source_external_cluster_name
      plugin = {
        name = local.barman_cloud_plugin_name
        parameters = {
          barmanObjectName = local.recovery_object_store_name
          serverName       = local.recovery.source_server_name
        }
      }
    }]
  )

  # ---- Cluster CR: sub-blocks -------------------------------------------------
  storage_body = {
    for k, v in {
      size         = var.spec.storage.size
      storageClass = try(var.spec.storage.storage_class, "") != "" ? var.spec.storage.storage_class : null
      # resize_in_use_volumes renders only when it DIVERGES from the shared
      # default (true) — the CRD default already covers the true case.
      resizeInUseVolumes = try(var.spec.storage.resize_in_use_volumes, null) == false ? false : null
    } : k => v if v != null
  }

  wal_storage_body = try(var.spec.wal_storage, null) == null ? null : {
    for k, v in {
      size               = var.spec.wal_storage.size
      storageClass       = try(var.spec.wal_storage.storage_class, "") != "" ? var.spec.wal_storage.storage_class : null
      resizeInUseVolumes = try(var.spec.wal_storage.resize_in_use_volumes, null) == false ? false : null
    } : k => v if v != null
  }

  # Absent quantities are OMITTED, never rendered empty: CNPG's
  # mutating webhook rejects "" with `quantities must match the
  # regular expression ...` (verified live against a spec declaring
  # only limits.memory), so each cpu/memory key joins its block only
  # when the manifest actually set it.
  resources_limits_body = try(var.spec.resources.limits, null) == null ? null : {
    for k, v in {
      cpu    = try(var.spec.resources.limits.cpu, null)
      memory = try(var.spec.resources.limits.memory, null)
    } : k => v if v != null && v != ""
  }
  resources_requests_body = try(var.spec.resources.requests, null) == null ? null : {
    for k, v in {
      cpu    = try(var.spec.resources.requests.cpu, null)
      memory = try(var.spec.resources.requests.memory, null)
    } : k => v if v != null && v != ""
  }
  resources_body = try(var.spec.resources, null) == null ? null : {
    for k, v in {
      limits   = local.resources_limits_body
      requests = local.resources_requests_body
    } : k => v if try(length(v), 0) > 0
  }

  # postgresql stanza. The CRD's own field names are snake_case here
  # (pg_hba, pg_ident, shared_preload_libraries) — CloudNativePG mirrors
  # the PostgreSQL file names.
  postgresql_body = {
    for k, v in {
      parameters               = length(try(var.spec.postgresql.parameters, {})) > 0 ? var.spec.postgresql.parameters : null
      pg_hba                   = length(try(var.spec.postgresql.pg_hba, [])) > 0 ? var.spec.postgresql.pg_hba : null
      pg_ident                 = length(try(var.spec.postgresql.pg_ident, [])) > 0 ? var.spec.postgresql.pg_ident : null
      shared_preload_libraries = length(try(var.spec.postgresql.shared_preload_libraries, [])) > 0 ? var.spec.postgresql.shared_preload_libraries : null
      # The spec's declared defaults (method any, data_durability
      # required) are applied HERE, not in variables.tf: that file is
      # generator-produced and carries no defaults for optional-with-
      # default fields, so the module renders the same body whether or
      # not the platform's defaulting middleware ran.
      synchronous = try(var.spec.postgresql.synchronous, null) == null ? null : {
        method         = coalesce(try(var.spec.postgresql.synchronous.method, null), "any")
        number         = var.spec.postgresql.synchronous.number
        dataDurability = coalesce(try(var.spec.postgresql.synchronous.data_durability, null), "required")
      }
      enableAlterSystem = try(var.spec.postgresql.enable_alter_system, false) ? true : null
    } : k => v if v != null
  }

  managed_roles = [
    for r in var.spec.roles : {
      for k, v in {
        name    = r.name
        comment = try(r.comment, "") != "" ? r.comment : null
        ensure  = try(r.ensure, "") != "" && try(r.ensure, "") != "present" ? r.ensure : null
        # The declared password lives in the `<name>-role-<role>` basic-auth
        # Secret this module materializes; the operator watches it.
        passwordSecret  = try(r.password, "") != "" ? { name = "${local.cluster_name}-role-${r.name}" } : null
        disablePassword = try(r.disable_password, false) ? true : null
        login           = try(r.login, false) ? true : null
        superuser       = try(r.superuser, false) ? true : null
        createdb        = try(r.createdb, false) ? true : null
        createrole      = try(r.createrole, false) ? true : null
        replication     = try(r.replication, false) ? true : null
        bypassrls       = try(r.bypassrls, false) ? true : null
        inRoles         = length(try(r.in_roles, [])) > 0 ? r.in_roles : null
        connectionLimit = try(r.connection_limit, null) != null && try(r.connection_limit, -1) != -1 ? r.connection_limit : null
      } : k => v if v != null
    }
  ]

  # Keyless cloud identity rides the instance ServiceAccount: the operator
  # creates one ServiceAccount per cluster (named after it), and the
  # template's annotations are what the cloud webhooks key on. All values
  # are strings — this merge cannot hit the type-unification trap.
  workload_identity_annotations = merge(
    try(var.spec.workload_identity.gke, null) != null ? {
      "iam.gke.io/gcp-service-account" = var.spec.workload_identity.gke.service_account_email
    } : {},
    try(var.spec.workload_identity.eks, null) != null ? {
      "eks.amazonaws.com/role-arn" = var.spec.workload_identity.eks.role_arn
    } : {},
    try(var.spec.workload_identity.aks, null) != null ? merge(
      { "azure.workload.identity/client-id" = var.spec.workload_identity.aks.client_id },
      try(var.spec.workload_identity.aks.tenant_id, "") != "" ? {
        "azure.workload.identity/tenant-id" = var.spec.workload_identity.aks.tenant_id
      } : {}
    ) : {}
  )

  certificates_body = {
    for k, v in {
      serverTLSSecret   = try(var.spec.certificates.server_tls_secret, "") != "" ? var.spec.certificates.server_tls_secret : null
      serverCASecret    = try(var.spec.certificates.server_ca_secret, "") != "" ? var.spec.certificates.server_ca_secret : null
      serverAltDNSNames = length(try(var.spec.certificates.server_alt_dns_names, [])) > 0 ? var.spec.certificates.server_alt_dns_names : null
    } : k => v if v != null
  }

  monitoring_body = {
    for k, v in {
      tls                   = try(var.spec.monitoring.tls_enabled, false) ? { enabled = true } : null
      disableDefaultQueries = try(var.spec.monitoring.disable_default_queries, false) ? true : null
    } : k => v if v != null
  }

  # The operator's anti-affinity is on by default; only the TYPE (when it
  # diverges from "preferred") and the topology key need rendering.
  affinity_body = {
    for k, v in {
      podAntiAffinityType = try(var.spec.scheduling.anti_affinity_type, "") != "" && try(var.spec.scheduling.anti_affinity_type, "") != "preferred" ? var.spec.scheduling.anti_affinity_type : null
      topologyKey         = try(var.spec.scheduling.topology_key, "") != "" ? var.spec.scheduling.topology_key : null
      nodeSelector        = length(try(var.spec.scheduling.node_selector, {})) > 0 ? var.spec.scheduling.node_selector : null
      tolerations = length(try(var.spec.scheduling.tolerations, [])) > 0 ? [
        for t in var.spec.scheduling.tolerations : {
          for tk, tv in {
            key               = try(t.key, "") != "" ? t.key : null
            operator          = try(t.operator, "") != "" ? t.operator : null
            value             = try(t.value, "") != "" ? t.value : null
            effect            = try(t.effect, "") != "" ? t.effect : null
            tolerationSeconds = try(t.toleration_seconds, null)
          } : tk => tv if tv != null
        }
      ] : null
    } : k => v if v != null
  }

  # ---- the Cluster CR ----------------------------------------------------------
  cluster_manifest = {
    apiVersion = "postgresql.cnpg.io/v1"
    kind       = "Cluster"
    metadata = {
      name      = local.cluster_name
      namespace = local.namespace
      labels    = local.labels
    }
    spec = {
      for k, v in {
        # The spec's declared default (1) applied here — variables.tf is
        # generator-produced and carries no default for the optional field.
        instances = coalesce(try(var.spec.instances, null), 1)
        imageName = try(var.spec.image_name, "") != "" ? var.spec.image_name : null

        storage    = local.storage_body
        walStorage = local.wal_storage_body
        resources  = local.resources_body

        postgresql = length(local.postgresql_body) > 0 ? local.postgresql_body : null
        bootstrap  = length(local.bootstrap_body) > 0 ? local.bootstrap_body : null

        externalClusters = length(local.external_clusters) > 0 ? local.external_clusters : null

        # Superuser posture: the enable flag and (optionally) the provided
        # password secret. The operator blanks the postgres password and
        # deletes the secret whenever access is disabled.
        enableSuperuserAccess = try(var.spec.superuser.enabled, false) ? true : null
        superuserSecret       = try(var.spec.superuser.enabled, false) && try(var.spec.superuser.password, "") != "" ? { name = local.provided_superuser_secret_name } : null

        managed = length(local.managed_roles) > 0 ? { roles = local.managed_roles } : null

        # The Barman Cloud plugin wiring: designating the plugin as the WAL
        # archiver is what starts continuous archiving into the ObjectStore.
        # PLUGIN-BASED, deliberately: CloudNativePG's in-tree barmanObjectStore
        # backup method is deprecated upstream and not modeled here.
        plugins = local.backup != null ? [{
          name          = local.barman_cloud_plugin_name
          isWALArchiver = true
          parameters    = { barmanObjectName = local.backup_object_store_name }
        }] : null

        serviceAccountTemplate = length(local.workload_identity_annotations) > 0 ? {
          metadata = { annotations = local.workload_identity_annotations }
        } : null

        certificates = length(local.certificates_body) > 0 ? local.certificates_body : null
        monitoring   = length(local.monitoring_body) > 0 ? local.monitoring_body : null
        affinity     = length(local.affinity_body) > 0 ? local.affinity_body : null

        priorityClassName = try(var.spec.scheduling.priority_class_name, "") != "" ? var.spec.scheduling.priority_class_name : null

        primaryUpdateStrategy = try(var.spec.update_strategy.primary_update_strategy, "") != "" ? var.spec.update_strategy.primary_update_strategy : null
        primaryUpdateMethod   = try(var.spec.update_strategy.primary_update_method, "") != "" ? var.spec.update_strategy.primary_update_method : null

        # enable_pdb defaults to true both here and upstream; only an
        # explicit false needs rendering (presence-sensitive — an absent
        # key already means true to the operator).
        enablePDB = try(var.spec.enable_pdb, null) == false ? false : null

        imagePullSecrets = length(var.spec.image_pull_secrets) > 0 ? [
          for s in var.spec.image_pull_secrets : { name = s }
        ] : null
      } : k => v if v != null
    }
  }

  # ---- ScheduledBackups ----------------------------------------------------------
  # One ScheduledBackup per declared schedule, named `<cluster>-<schedule>`,
  # each explicitly method=plugin against the cluster's backup ObjectStore —
  # never the deprecated in-tree barmanObjectStore method.
  # Keyed by the FULL CR name (`<cluster>-<schedule>`) — the state address
  # key is what the import recipes derive the live object name from
  # (from_address_key), so the key and metadata.name must be identical.
  scheduled_backup_manifests = {
    for s in try(local.backup.schedules, []) : "${local.cluster_name}-${s.name}" => {
      apiVersion = "postgresql.cnpg.io/v1"
      kind       = "ScheduledBackup"
      metadata = {
        name      = "${local.cluster_name}-${s.name}"
        namespace = local.namespace
        labels    = local.labels
      }
      spec = {
        for k, v in {
          schedule = s.schedule
          cluster  = { name = local.cluster_name }
          method   = "plugin"
          pluginConfiguration = {
            name       = local.barman_cloud_plugin_name
            parameters = { barmanObjectName = local.backup_object_store_name }
          }
          # Scheduled backups belong to their schedule: deleting the
          # schedule garbage-collects its Backup records while the stored
          # objects in the bucket survive either way.
          backupOwnerReference = "self"
          immediate            = try(s.immediate, false) ? true : null
          suspend              = try(s.suspend, false) ? true : null
          target               = try(s.target, "") != "" ? s.target : null
        } : k => v if v != null
      }
    }
  }
}
