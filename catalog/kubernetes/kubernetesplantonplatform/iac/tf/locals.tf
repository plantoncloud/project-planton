# Computed values for the KubernetesPlantonPlatform module. Every
# resolution here has an exact twin in the Pulumi module's locals.go /
# platform_cr.go — keep them in lockstep.
#
# THE SPEC BODY RENDERS ONLY WHAT THE MANIFEST DECLARED: every block below
# is a null-pruned object (`key = cond ? value : null` inside one literal,
# pruned with `{ for k, v in {...} : k => v if v != null }`), so the
# operator's own defaulting stays authoritative for everything unset — the
# same posture as the `planton` Helm chart's verbatim pass-through.
# Three-state optionals (the default-true toggles, the defaulted scalars)
# render exactly when PRESENT: an explicit `enabled: true` is faithfully
# forwarded even though it matches the CRD default, because presence is
# the user's deliberate statement.
#
# HCL DISCIPLINE: `cond ? {...} : {}` ternaries fail plan-time type
# unification when branches carry different attributes, and merge() of
# primitive-only sibling objects silently unifies them into map(string) —
# the null-prune form preserves every value's type. Optional nested blocks
# are read with try(): HCL's && does NOT short-circuit.

locals {
  # The PlantonPlatform CR identity. The CR name is THIS resource's
  # metadata.name — the prefix of every object the operator creates for
  # the platform.
  api_version   = "planton.ai/v1"
  cr_kind       = "PlantonPlatform"
  platform_name = var.metadata.name

  namespace = var.spec.namespace

  # The operator's deterministic per-platform naming — the consumer
  # handles this module exports. Twin of the Pulumi module's vars.
  gateway_service   = "${local.platform_name}-gateway"
  setup_code_secret = "${local.platform_name}-identity-setup-code"

  gateway_local_port = coalesce(try(var.spec.gateway.local_port, null), 8080)

  port_forward_command = "kubectl port-forward -n ${local.namespace} svc/${local.gateway_service} ${local.gateway_local_port}:80"
  setup_code_command   = "kubectl -n ${local.namespace} get secret ${local.setup_code_secret} -o jsonpath='{.data.setup-code}' | base64 -d"

  # Resource-identity labels stamped on the module-created objects (the
  # namespace and the CR itself).
  labels = merge(
    {
      "planton.ai/resource"      = "true"
      "planton.ai/resource-name" = var.metadata.name
      "planton.ai/resource-kind" = "KubernetesPlantonPlatform"
    },
    var.metadata.id != null && var.metadata.id != "" ? { "planton.ai/resource-id" = var.metadata.id } : {},
    var.metadata.org != null && var.metadata.org != "" ? { "planton.ai/organization" = var.metadata.org } : {},
    var.metadata.env != null && var.metadata.env != "" ? { "planton.ai/environment" = var.metadata.env } : {}
  )

  # ---- license ---------------------------------------------------------------
  license_body = {
    for k, v in {
      key = try(var.spec.license.key, "") != "" ? var.spec.license.key : null
      secretKeyRef = try(var.spec.license.secret_key_ref, null) == null ? null : {
        name = var.spec.license.secret_key_ref.name
        key  = var.spec.license.secret_key_ref.key
      }
    } : k => v if v != null
  }

  # ---- storage ---------------------------------------------------------------
  storage_body = {
    for k, v in {
      storageClassName = try(var.spec.storage.storage_class_name, "") != "" ? var.spec.storage.storage_class_name : null
      size             = try(var.spec.storage.size, "") != "" ? var.spec.storage.size : null
    } : k => v if v != null
  }

  # ---- database --------------------------------------------------------------
  postgresql_body = {
    for k, v in {
      replicas         = try(var.spec.database.postgresql.replicas, null)
      storageSize      = try(var.spec.database.postgresql.storage_size, "") != "" ? var.spec.database.postgresql.storage_size : null
      storageClassName = try(var.spec.database.postgresql.storage_class_name, "") != "" ? var.spec.database.postgresql.storage_class_name : null
    } : k => v if v != null
  }
  redis_body = {
    for k, v in {
      storageSize      = try(var.spec.database.redis.storage_size, "") != "" ? var.spec.database.redis.storage_size : null
      storageClassName = try(var.spec.database.redis.storage_class_name, "") != "" ? var.spec.database.redis.storage_class_name : null
    } : k => v if v != null
  }
  database_body = {
    for k, v in {
      postgresql = length(local.postgresql_body) > 0 ? local.postgresql_body : null
      redis      = length(local.redis_body) > 0 ? local.redis_body : null
    } : k => v if v != null
  }

  # ---- ingress ---------------------------------------------------------------
  ingress_tls_issuer = try(var.spec.ingress.tls.issuer, null) == null ? null : {
    for k, v in {
      name = var.spec.ingress.tls.issuer.name
      kind = try(var.spec.ingress.tls.issuer.kind, "") != "" ? var.spec.ingress.tls.issuer.kind : null
    } : k => v if v != null
  }
  ingress_tls = try(var.spec.ingress.tls, null) == null ? null : {
    for k, v in {
      secretName = try(var.spec.ingress.tls.secret_name, "") != "" ? var.spec.ingress.tls.secret_name : null
      issuer     = local.ingress_tls_issuer
    } : k => v if v != null
  }
  # The Gateway API front door: the fork's other arm (the CRD refuses it
  # beside ingressClassName). Rendered only when the manifest named a
  # Gateway; the operator reads the Gateway's listeners for everything else.
  ingress_gateway_ref = try(var.spec.ingress.gateway_ref, null) == null ? null : {
    for k, v in {
      name        = var.spec.ingress.gateway_ref.name
      namespace   = try(var.spec.ingress.gateway_ref.namespace, "") != "" ? var.spec.ingress.gateway_ref.namespace : null
      sectionName = try(var.spec.ingress.gateway_ref.section_name, "") != "" ? var.spec.ingress.gateway_ref.section_name : null
    } : k => v if v != null
  }
  ingress_body = {
    for k, v in {
      enabled          = try(var.spec.ingress.enabled, false) ? true : null
      hostname         = try(var.spec.ingress.hostname, "") != "" ? var.spec.ingress.hostname : null
      ingressClassName = try(var.spec.ingress.ingress_class_name, "") != "" ? var.spec.ingress.ingress_class_name : null
      gatewayRef       = local.ingress_gateway_ref
      annotations      = length(try(var.spec.ingress.annotations, {})) > 0 ? var.spec.ingress.annotations : null
      tls              = local.ingress_tls
      # Rendered on presence, like every defaulted three-state string: an
      # omitted value is left to the CRD's own default (auto).
      reachability = try(var.spec.ingress.reachability, "") != "" ? var.spec.ingress.reachability : null
    } : k => v if v != null
  }

  # ---- gateway / identity ------------------------------------------------------
  gateway_body = {
    for k, v in {
      localPort = try(var.spec.gateway.local_port, null)
    } : k => v if v != null
  }
  identity_body = {
    for k, v in {
      realm      = try(var.spec.identity.realm, "") != "" ? var.spec.identity.realm : null
      adminEmail = try(var.spec.identity.admin_email, "") != "" ? var.spec.identity.admin_email : null
    } : k => v if v != null
  }

  # ---- bootstrap -------------------------------------------------------------
  bootstrap_org = {
    for k, v in {
      slug = try(var.spec.bootstrap.organization.slug, "") != "" ? var.spec.bootstrap.organization.slug : null
      name = try(var.spec.bootstrap.organization.name, "") != "" ? var.spec.bootstrap.organization.name : null
    } : k => v if v != null
  }
  bootstrap_env = {
    for k, v in {
      slug = try(var.spec.bootstrap.environment.slug, "") != "" ? var.spec.bootstrap.environment.slug : null
      name = try(var.spec.bootstrap.environment.name, "") != "" ? var.spec.bootstrap.environment.name : null
    } : k => v if v != null
  }
  bootstrap_secret_backend = try(var.spec.bootstrap.secret_backend, null) == null ? null : {
    for k, v in {
      type = var.spec.bootstrap.secret_backend.type
      awsSecretsManager = try(var.spec.bootstrap.secret_backend.aws_secrets_manager, null) == null ? null : {
        region    = var.spec.bootstrap.secret_backend.aws_secrets_manager.region
        kmsKeyArn = var.spec.bootstrap.secret_backend.aws_secrets_manager.kms_key_arn
      }
    } : k => v if v != null
  }
  bootstrap_body = {
    for k, v in {
      organization   = length(local.bootstrap_org) > 0 ? local.bootstrap_org : null
      environment    = length(local.bootstrap_env) > 0 ? local.bootstrap_env : null
      admins         = length(try(var.spec.bootstrap.admins, [])) > 0 ? var.spec.bootstrap.admins : null
      iacProvisioner = try(var.spec.bootstrap.iac_provisioner, "") != "" ? var.spec.bootstrap.iac_provisioner : null
      secretBackend  = local.bootstrap_secret_backend
    } : k => v if v != null
  }

  # ---- runner / build / vault ----------------------------------------------------
  runner_body = {
    for k, v in {
      enabled                    = try(var.spec.runner.enabled, null)
      storageSize                = try(var.spec.runner.storage_size, "") != "" ? var.spec.runner.storage_size : null
      storageClassName           = try(var.spec.runner.storage_class_name, "") != "" ? var.spec.runner.storage_class_name : null
      serviceAccountAnnotations  = length(try(var.spec.runner.service_account_annotations, {})) > 0 ? var.spec.runner.service_account_annotations : null
      cloudCredentialsSecretName = try(var.spec.runner.cloud_credentials_secret_name, "") != "" ? var.spec.runner.cloud_credentials_secret_name : null
    } : k => v if v != null
  }
  build_body = {
    for k, v in {
      enabled = try(var.spec.build.enabled, null)
    } : k => v if v != null
  }
  remote_runners_body = {
    for k, v in {
      enabled = try(var.spec.remote_runners.enabled, null)
    } : k => v if v != null
  }

  # ---- email -----------------------------------------------------------------
  # One declaration for both senders. Each nested object is its own local so
  # the parent stays a flat null-prune; the two provider arms render only
  # when declared (the spec's CEL already holds exactly one). Defaulted
  # scalars (port, security, from.name) render on presence only, so an
  # omitted value is left to the CRD's own default.
  email_from = {
    for k, v in {
      address = try(var.spec.email.from.address, "") != "" ? var.spec.email.from.address : null
      name    = try(var.spec.email.from.name, "") != "" ? var.spec.email.from.name : null
    } : k => v if v != null
  }
  email_smtp_oauth2 = try(var.spec.email.smtp.oauth2, null) == null ? null : {
    user     = var.spec.email.smtp.oauth2.user
    tokenUrl = var.spec.email.smtp.oauth2.token_url
    scope    = var.spec.email.smtp.oauth2.scope
    clientId = var.spec.email.smtp.oauth2.client_id
    clientSecretRef = {
      name = var.spec.email.smtp.oauth2.client_secret_ref.name
      key  = var.spec.email.smtp.oauth2.client_secret_ref.key
    }
  }
  email_smtp = try(var.spec.email.smtp, null) == null ? null : {
    for k, v in {
      host                  = var.spec.email.smtp.host
      port                  = try(var.spec.email.smtp.port, null)
      security              = try(var.spec.email.smtp.security, "") != "" ? var.spec.email.smtp.security : null
      credentialsSecretName = try(var.spec.email.smtp.credentials_secret_name, "") != "" ? var.spec.email.smtp.credentials_secret_name : null
      oauth2                = local.email_smtp_oauth2
      caBundleSecretRef = try(var.spec.email.smtp.ca_bundle_secret_ref, null) == null ? null : {
        name = var.spec.email.smtp.ca_bundle_secret_ref.name
        key  = var.spec.email.smtp.ca_bundle_secret_ref.key
      }
    } : k => v if v != null
  }
  email_resend = try(var.spec.email.resend, null) == null ? null : {
    apiKeySecretRef = {
      name = var.spec.email.resend.api_key_secret_ref.name
      key  = var.spec.email.resend.api_key_secret_ref.key
    }
  }
  email_body = {
    for k, v in {
      from    = length(local.email_from) > 0 ? local.email_from : null
      replyTo = try(var.spec.email.reply_to, "") != "" ? var.spec.email.reply_to : null
      smtp    = local.email_smtp
      resend  = local.email_resend
    } : k => v if v != null
  }
  vault_body = {
    for k, v in {
      enabled          = try(var.spec.vault.enabled, null)
      initMode         = try(var.spec.vault.init_mode, "") != "" ? var.spec.vault.init_mode : null
      storageSize      = try(var.spec.vault.storage_size, "") != "" ? var.spec.vault.storage_size : null
      storageClassName = try(var.spec.vault.storage_class_name, "") != "" ? var.spec.vault.storage_class_name : null
    } : k => v if v != null
  }

  # ---- components ------------------------------------------------------------
  components_graph = {
    for k, v in {
      enabled          = try(var.spec.components.graph.enabled, false) ? true : null
      storageSize      = try(var.spec.components.graph.storage_size, "") != "" ? var.spec.components.graph.storage_size : null
      storageClassName = try(var.spec.components.graph.storage_class_name, "") != "" ? var.spec.components.graph.storage_class_name : null
    } : k => v if v != null
  }
  components_body = {
    for k, v in {
      graph = length(local.components_graph) > 0 ? local.components_graph : null
    } : k => v if v != null
  }

  # ---- prerequisites ---------------------------------------------------------
  prerequisites_body = {
    for k, v in {
      postgresOperator = try(var.spec.prerequisites.postgres_operator, "") != "" ? var.spec.prerequisites.postgres_operator : null
      tektonPipelines  = try(var.spec.prerequisites.tekton_pipelines, "") != "" ? var.spec.prerequisites.tekton_pipelines : null
    } : k => v if v != null
  }

  # ---- controlPlane / console ----------------------------------------------------
  control_plane_image = {
    for k, v in {
      repository = try(var.spec.control_plane.image.repository, "") != "" ? var.spec.control_plane.image.repository : null
      tag        = try(var.spec.control_plane.image.tag, "") != "" ? var.spec.control_plane.image.tag : null
    } : k => v if v != null
  }
  control_plane_body = {
    for k, v in {
      image                     = length(local.control_plane_image) > 0 ? local.control_plane_image : null
      replicas                  = try(var.spec.control_plane.replicas, null)
      externalConfigSecretName  = try(var.spec.control_plane.external_config_secret_name, "") != "" ? var.spec.control_plane.external_config_secret_name : null
      serviceAccountAnnotations = length(try(var.spec.control_plane.service_account_annotations, {})) > 0 ? var.spec.control_plane.service_account_annotations : null
    } : k => v if v != null
  }
  console_image = {
    for k, v in {
      repository = try(var.spec.console.image.repository, "") != "" ? var.spec.console.image.repository : null
      tag        = try(var.spec.console.image.tag, "") != "" ? var.spec.console.image.tag : null
    } : k => v if v != null
  }
  console_body = {
    for k, v in {
      image                    = length(local.console_image) > 0 ? local.console_image : null
      replicas                 = try(var.spec.console.replicas, null)
      externalConfigSecretName = try(var.spec.console.external_config_secret_name, "") != "" ? var.spec.console.external_config_secret_name : null
    } : k => v if v != null
  }

  # ---- the CR spec (twin of the Pulumi module's platformSpecBody) -------------
  platform_spec = {
    for k, v in {
      version = var.spec.version

      license       = length(local.license_body) > 0 ? local.license_body : null
      storage       = length(local.storage_body) > 0 ? local.storage_body : null
      database      = length(local.database_body) > 0 ? local.database_body : null
      ingress       = length(local.ingress_body) > 0 ? local.ingress_body : null
      gateway       = length(local.gateway_body) > 0 ? local.gateway_body : null
      identity      = length(local.identity_body) > 0 ? local.identity_body : null
      bootstrap     = length(local.bootstrap_body) > 0 ? local.bootstrap_body : null
      runner        = length(local.runner_body) > 0 ? local.runner_body : null
      build         = length(local.build_body) > 0 ? local.build_body : null
      remoteRunners = length(local.remote_runners_body) > 0 ? local.remote_runners_body : null
      email         = length(local.email_body) > 0 ? local.email_body : null
      vault         = length(local.vault_body) > 0 ? local.vault_body : null
      components    = length(local.components_body) > 0 ? local.components_body : null
      prerequisites = length(local.prerequisites_body) > 0 ? local.prerequisites_body : null
      controlPlane  = length(local.control_plane_body) > 0 ? local.control_plane_body : null
      console       = length(local.console_body) > 0 ? local.console_body : null
    } : k => v if v != null
  }
}
