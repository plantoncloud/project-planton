# Computed values for the KubernetesPlantonOperator module. Every
# resolution here has an exact twin in the Pulumi module's locals.go /
# values.go — keep them in lockstep.
#
# HCL DISCIPLINE (applies to every conditional object in this file):
# conditional entries are written as `key = cond ? value : null` inside ONE
# object literal, pruned with `{ for k, v in {...} : k => v if v != null }`.
# The two tempting alternatives are both broken: `cond ? {...} : {}`
# ternaries fail plan-time type unification when branches carry different
# attributes, and merge() of primitive-only sibling objects silently
# unifies them into map(string). The null-prune form preserves every
# value's type.
#
# Optional nested blocks are read with try(): HCL's && does NOT
# short-circuit, so chained null checks still dereference the null — and
# var.spec is typed 'any', so an absent attribute is an error, not a null.

locals {
  # Chart identity — must stay byte-identical with the Pulumi module's
  # vars: cross-engine chart drift deploys two different operators from one
  # manifest. The chart is OCI-published; the Terraform helm provider takes
  # the repo as `repository` plus the bare chart name (unlike Pulumi's
  # joined string).
  helm_oci_repo   = "oci://ghcr.io/plantonhq/charts"
  helm_chart_name = "planton-operator"

  # default_chart_version is the chart this catalog release was validated
  # against (mirror of the proto field's default and the Pulumi module's
  # DefaultChartVersion; the three move together). min_chart_version is the
  # oldest chart whose definitions are release resources behind the
  # crds.enabled / crds.keep values this module renders; older charts have
  # no such values, so the crds dials would be silently dropped — refused
  # at plan time instead (main.tf precondition).
  default_chart_version = "0.13.0"
  min_chart_version     = "0.8.0"
  chart_version         = coalesce(try(var.spec.chart_version, null), local.default_chart_version)

  # Release name FIXED to "planton-operator": the operator enforces one
  # installation per cluster itself at startup (a label-matched Deployment
  # scan that refuses to start beside a sibling), so the release name
  # never derives from metadata.name — the fixed name makes the collision
  # impossible to express instead of merely refused. With the release name
  # equal to the chart name, the chart's fullname helper renders the
  # Deployment as plain "planton-operator".
  release_name = "planton-operator"

  namespace = var.spec.namespace

  # Resource-identity labels stamped on the one object the module creates
  # itself (the optional namespace) — never injected into the chart's own
  # resources; Helm owns those.
  labels = merge(
    {
      "planton.ai/resource"      = "true"
      "planton.ai/resource-name" = var.metadata.name
      "planton.ai/resource-kind" = "KubernetesPlantonOperator"
    },
    var.metadata.id != null && var.metadata.id != "" ? { "planton.ai/resource-id" = var.metadata.id } : {},
    var.metadata.org != null && var.metadata.org != "" ? { "planton.ai/organization" = var.metadata.org } : {},
    var.metadata.env != null && var.metadata.env != "" ? { "planton.ai/environment" = var.metadata.env } : {}
  )

  # ---- operator container resources (shared ContainerResources shape) --------
  # Twin of the Pulumi module's resourcesMap. Rendered values deep-merge
  # OVER the chart's own resource defaults (requests 10m/256Mi, limits
  # 500m/512Mi) — a partial spec keeps the untouched halves.
  operator_resources = try(var.spec.resources, null) == null ? null : {
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

  # ---- service account --------------------------------------------------------
  service_account_values = {
    for k, v in {
      # serviceAccount.create matches the chart's own default (true) —
      # rendered only on explicit opt-out (bring your own, named below).
      create      = try(var.spec.service_account.create, null) == false ? false : null
      name        = try(var.spec.service_account.name, "") != "" ? var.spec.service_account.name : null
      annotations = length(try(var.spec.service_account.annotations, {})) > 0 ? var.spec.service_account.annotations : null
    } : k => v if v != null
  }

  # ---- image ------------------------------------------------------------------
  # Pull secrets are name references in the chart's values ([{name: ...}],
  # toYaml'd verbatim into the pod spec); the image override renders only
  # the halves that are set — an empty tag keeps the chart's appVersion
  # default.
  image_values = {
    for k, v in {
      repository = try(var.spec.image.repository, "") != "" ? var.spec.image.repository : null
      tag        = try(var.spec.image.tag, "") != "" ? var.spec.image.tag : null
    } : k => v if v != null
  }

  # ---- typed chart values (twin of the Pulumi module's buildHelmValues) ------
  # No fullnameOverride and no nameOverride, deliberately: the chart's name
  # helpers feed the `app.kubernetes.io/name: planton-operator` label the
  # operator's OWN one-per-cluster startup guard matches on — renaming
  # would take the Deployment out of the guard's view.
  typed_values = {
    for k, v in {
      # The chart owns its two definitions as release resources behind these
      # values. Planton default: install them with the release and keep them
      # on uninstall (kept definitions preserve every PlantonPlatform
      # declaration and the platforms behind them; a later install under the
      # fixed release name adopts them). Always rendered, so the release's
      # values state the posture whichever way the dials were left.
      crds = {
        enabled = try(var.spec.crds.install, null) != null ? var.spec.crds.install : true
        keep    = try(var.spec.crds.keep_on_uninstall, null) != null ? var.spec.crds.keep_on_uninstall : true
      }

      replicaCount = try(var.spec.replicas, null)

      # leaderElection.enabled matches the chart's own default (true) —
      # rendered only on explicit opt-out (single-replica dev clusters).
      leaderElection = try(var.spec.leader_election, null) == false ? { enabled = false } : null

      resources = local.operator_resources != null && length(local.operator_resources) > 0 ? local.operator_resources : null

      serviceAccount = length(local.service_account_values) > 0 ? local.service_account_values : null

      commonLabels   = length(try(var.spec.common_labels, {})) > 0 ? var.spec.common_labels : null
      podAnnotations = length(try(var.spec.pod_annotations, {})) > 0 ? var.spec.pod_annotations : null
      nodeSelector   = length(try(var.spec.node_selector, {})) > 0 ? var.spec.node_selector : null
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

      imagePullSecrets = length(try(var.spec.image_pull_secrets, [])) > 0 ? [
        for s in var.spec.image_pull_secrets : { name = s }
      ] : null
      image = length(local.image_values) > 0 ? local.image_values : null
    } : k => v if v != null
  }
}
