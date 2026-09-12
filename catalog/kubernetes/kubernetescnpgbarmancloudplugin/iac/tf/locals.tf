# Computed values for the KubernetesCnpgBarmanCloudPlugin module. Every
# resolution here has an exact twin in the Pulumi module's locals.go /
# values.go / helpers.go -- keep them in lockstep.
#
# HCL DISCIPLINE (applies to every conditional object in this file):
# conditional entries are written as `key = cond ? value : null` inside ONE
# object literal, pruned with `{ for k, v in {...} : k => v if v != null }`.
# The two tempting alternatives are both broken: `cond ? {...} : {}`
# ternaries fail plan-time type unification when branches carry different
# attributes, and `merge(concat(cond ? [{...}] : [], ...)...)` silently
# UNIFIES primitive-only sibling objects into map(string) -- numbers and
# booleans arrive in the chart values as strings. The null-prune form
# preserves every value's type.
#
# Optional nested blocks are read with try(): HCL's && does NOT
# short-circuit, so chained null checks still dereference the null -- and
# var.spec is typed 'any', so an absent attribute is an error, not a null.

locals {
  # Chart identity -- must stay byte-identical with the Pulumi module's
  # vars: cross-engine chart drift deploys two different products from one
  # manifest. The CloudNativePG project's repository serves the plugin
  # chart beside the operator chart.
  helm_chart_repo = "https://cloudnative-pg.github.io/charts"
  helm_chart_name = "plugin-barman-cloud"

  # Release name FIXED to "plugin-barman-cloud": the chart bakes the
  # plugin's gRPC Service name ("barman-cloud"), its two TLS Secrets, its
  # Certificates and its config ConfigMap into fixed names -- a second
  # release in the same namespace would fight the first over all of them --
  # and the ObjectStore CRD the chart keeps on uninstall is adopted by a
  # later install ONLY when the release name and namespace match. One plugin
  # per operator namespace is a chart constraint; the release name never
  # derives from metadata.name.
  release_name = "plugin-barman-cloud"

  # The CNPG-I identifier the operator discovers the plugin under (the
  # `cnpg.io/pluginName` label the chart stamps on the Service) and the
  # name a Cluster's `plugins` list carries.
  plugin_name = "barman-cloud.cloudnative-pg.io"

  # The plugin's gRPC Service name -- fixed by the chart (baked into the TLS
  # certificate) and re-pinned after the helm_values merge.
  service_name = "barman-cloud"

  # Chart version resolved to the pinned default when unset, so both
  # engines install the same chart whether or not the platform's
  # defaulting middleware ran -- mirror of the Pulumi module's
  # DefaultChartVersion. Chart and app versions move SEPARATELY (chart
  # 0.7.0 ships plugin v0.13.0) -- the chart pin governs.
  chart_version = coalesce(try(var.spec.chart_version, null), "0.7.0")

  # The operator's namespace, by requirement (the platform resolves a
  # reference to the operator resource's namespace output before the module
  # runs; a literal names a resident operator's namespace).
  namespace = var.spec.namespace

  # Resource-identity labels stamped on the namespace this module creates
  # (never injected into the chart's own resources -- Helm owns those).
  labels = merge(
    {
      "planton.ai/resource"      = "true"
      "planton.ai/resource-name" = var.metadata.name
      "planton.ai/resource-kind" = "KubernetesCnpgBarmanCloudPlugin"
    },
    var.metadata.id != null && var.metadata.id != "" ? { "planton.ai/resource-id" = var.metadata.id } : {},
    var.metadata.org != null && var.metadata.org != "" ? { "planton.ai/organization" = var.metadata.org } : {},
    var.metadata.env != null && var.metadata.env != "" ? { "planton.ai/environment" = var.metadata.env } : {}
  )

  # ---- CRD lifecycle -------------------------------------------------------
  # crds.create matches the chart's own default (true) -- rendered only on
  # explicit opt-out (something else manages the ObjectStore CRD). No keep
  # knob is needed: the chart stamps `helm.sh/resource-policy: keep` on the
  # CRD, so uninstalling never deletes the ObjectStore resources databases
  # point at -- the upstream safety posture, kept as-is.
  crds_values = try(var.spec.crds.install, null) == false ? { create = false } : null

  # ---- container resources (shared ContainerResources shape) ----------------
  # Twin of the Pulumi module's resourcesMap.
  plugin_resources = try(var.spec.resources, null) == null ? null : {
    for k, v in {
      limits = try(var.spec.resources.limits, null) == null ? null : {
        for lk, lv in {
          cpu    = try(var.spec.resources.limits.cpu, "") != "" ? var.spec.resources.limits.cpu : null
          memory = try(var.spec.resources.limits.memory, "") != "" ? var.spec.resources.limits.memory : null
        } : lk => lv if lv != null
      }
      requests = try(var.spec.resources.requests, null) == null ? null : {
        for rk, rv in {
          cpu    = try(var.spec.resources.requests.cpu, "") != "" ? var.spec.resources.requests.cpu : null
          memory = try(var.spec.resources.requests.memory, "") != "" ? var.spec.resources.requests.memory : null
        } : rk => rv if rv != null
      }
    } : k => v if v != null && length(v) > 0
  }

  # ---- images (the chart's SPLIT registry + repository shape) ---------------
  # Twin of the Pulumi module's splitImageRepository / imageMap: the first
  # path segment of a combined reference is a registry only when it carries
  # a dot or a colon or is "localhost"; otherwise the whole value is the
  # repository and the chart's default registry (ghcr.io at the pin) stays
  # in force. Mapping a combined reference verbatim onto a split-shaped
  # chart renders "<default-registry>/<mirror>/..." -- an ImagePullBackOff
  # identical on both engines that no parity review can see.
  image_repository_raw   = try(var.spec.image.repository, "")
  image_parts            = local.image_repository_raw != "" ? split("/", local.image_repository_raw) : []
  image_first_is_registry = length(local.image_parts) > 1 && (can(regex("[.:]", local.image_parts[0])) || local.image_parts[0] == "localhost")
  image_values = {
    for k, v in {
      registry   = local.image_first_is_registry ? local.image_parts[0] : null
      repository = local.image_repository_raw == "" ? null : (local.image_first_is_registry ? join("/", slice(local.image_parts, 1, length(local.image_parts))) : local.image_repository_raw)
      tag        = try(var.spec.image.tag, "") != "" ? var.spec.image.tag : null
    } : k => v if v != null
  }

  # The sidecar image is what the DATABASE pods pull -- the plugin publishes
  # it through its config ConfigMap and injects the container into every
  # instance pod. Same split rule.
  sidecar_repository_raw   = try(var.spec.sidecar_image.repository, "")
  sidecar_parts            = local.sidecar_repository_raw != "" ? split("/", local.sidecar_repository_raw) : []
  sidecar_first_is_registry = length(local.sidecar_parts) > 1 && (can(regex("[.:]", local.sidecar_parts[0])) || local.sidecar_parts[0] == "localhost")
  sidecar_image_values = {
    for k, v in {
      registry   = local.sidecar_first_is_registry ? local.sidecar_parts[0] : null
      repository = local.sidecar_repository_raw == "" ? null : (local.sidecar_first_is_registry ? join("/", slice(local.sidecar_parts, 1, length(local.sidecar_parts))) : local.sidecar_repository_raw)
      tag        = try(var.spec.sidecar_image.tag, "") != "" ? var.spec.sidecar_image.tag : null
    } : k => v if v != null
  }

  # ---- typed chart values (twin of the Pulumi module's buildHelmValues) ------
  typed_values = {
    for k, v in {
      crds = local.crds_values

      replicaCount = try(var.spec.replicas, null)
      resources    = local.plugin_resources != null && length(local.plugin_resources) > 0 ? local.plugin_resources : null

      image        = length(local.image_values) > 0 ? local.image_values : null
      sidecarImage = length(local.sidecar_image_values) > 0 ? local.sidecar_image_values : null
      # Pull secrets are name references in the chart's values
      # ([{name: ...}]); they cover the plugin pod only -- the sidecar is
      # pulled by the database pods in their own namespaces.
      imagePullSecrets = length(try(var.spec.image_pull_secrets, [])) > 0 ? [
        for s in var.spec.image_pull_secrets : { name = s }
      ] : null

      priorityClassName = try(var.spec.priority_class_name, "") != "" ? var.spec.priority_class_name : null
      nodeSelector      = length(try(var.spec.node_selector, {})) > 0 ? var.spec.node_selector : null
      tolerations = length(try(var.spec.tolerations, [])) > 0 ? [
        for t in var.spec.tolerations : {
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

  # ---- fixed identities, re-pinned AFTER the escape hatch ----------------------
  # The Service name is baked into the plugin's TLS certificate (the chart
  # says so itself: "DO NOT CHANGE THE SERVICE NAME"), and the release's
  # derived names are what a later install adopts the kept CRD through and
  # what the E2E verifier and import recipes key on. None of them is
  # configuration; a helm_values document that set them would break the
  # operator handshake silently, so this document is merged LAST. Twin of
  # the Pulumi module's fixedIdentityPins.
  fixed_identity_pins = {
    nameOverride      = ""
    fullnameOverride  = ""
    namespaceOverride = ""
    service = {
      name = local.service_name
    }
  }
}
