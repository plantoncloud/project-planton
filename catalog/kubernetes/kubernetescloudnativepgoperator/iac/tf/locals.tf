# Computed values for the KubernetesCloudNativePgOperator module. Every
# resolution here has an exact twin in the Pulumi module's locals.go /
# values.go — keep them in lockstep.
#
# HCL DISCIPLINE (applies to every conditional object in this file):
# conditional entries are written as `key = cond ? value : null` inside ONE
# object literal, pruned with `{ for k, v in {...} : k => v if v != null }`.
# The two tempting alternatives are both broken: `cond ? {...} : {}`
# ternaries fail plan-time type unification when branches carry different
# attributes, and `merge(concat(cond ? [{...}] : [], ...)...)` silently
# UNIFIES primitive-only sibling objects into map(string) — numbers and
# booleans arrive in the chart values as strings. The null-prune form
# preserves every value's type.
#
# Optional nested blocks are read with try(): HCL's && does NOT
# short-circuit, so chained null checks still dereference the null — and
# var.spec is typed 'any', so an absent attribute is an error, not a null.

locals {
  # Chart identity — must stay byte-identical with the Pulumi module's vars:
  # cross-engine chart drift deploys two different products from one
  # manifest. The CloudNativePG project's repository also serves the Barman
  # Cloud plugin chart, which its own kind installs.
  helm_chart_repo = "https://cloudnative-pg.github.io/charts"
  helm_chart_name = "cloudnative-pg"

  # Release name FIXED to "cnpg": the operator registers cluster-scoped
  # CRDs and mutating/validating webhooks whose service name is baked into
  # the chart ("cnpg-webhook-service" — embedded in the webhook certificate
  # and not configurable), so a second installation would fight over both.
  # One operator per cluster is an upstream constraint.
  release_name = "cnpg"

  # Chart version resolved to the pinned default when unset, so both
  # engines install the same chart whether or not the platform's
  # defaulting middleware ran — mirror of the Pulumi module's
  # DefaultChartVersion. Chart and app versions move SEPARATELY (chart
  # 0.29.0 ships operator 1.30.0) — the chart pin governs.
  chart_version = coalesce(try(var.spec.chart_version, null), "0.29.0")

  # The Barman Cloud plugin kind installs into this same namespace.
  namespace = var.spec.namespace

  # Resource-identity labels stamped on the namespace this module creates
  # (never injected into the chart's own resources — Helm owns those).
  labels = merge(
    {
      "planton.ai/resource"      = "true"
      "planton.ai/resource-name" = var.metadata.name
      "planton.ai/resource-kind" = "KubernetesCloudNativePgOperator"
    },
    var.metadata.id != null && var.metadata.id != "" ? { "planton.ai/resource-id" = var.metadata.id } : {},
    var.metadata.org != null && var.metadata.org != "" ? { "planton.ai/organization" = var.metadata.org } : {},
    var.metadata.env != null && var.metadata.env != "" ? { "planton.ai/environment" = var.metadata.env } : {}
  )

  # ---- CRD lifecycle -------------------------------------------------------
  # crds.create matches the chart's own default (true) — rendered only on
  # explicit opt-out (something else manages the CRDs). No keep knob is
  # needed here: the chart stamps `helm.sh/resource-policy: keep` on every
  # CRD UNCONDITIONALLY, so uninstalling the release never cascade-deletes
  # the Cluster resources (and the databases behind them) — the upstream
  # safety posture, kept as-is.
  crds_values = try(var.spec.crds.install, null) == false ? { create = false } : null

  # ---- operator container resources (shared ContainerResources shape) -------
  # Twin of the Pulumi module's resourcesMap.
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

  # ---- operator configuration (watch scope + config.data) --------------------
  # The chart folds three typed concerns into ONE `config` block:
  # clusterWide (RBAC scope), data (the operator's configmap entries), and
  # maxConcurrentReconciles.
  #
  # WATCH_NAMESPACE PRECEDENCE: the typed watch field OWNS the
  # WATCH_NAMESPACE key. A user entry under that key in operator_config is
  # always stripped; the key is rendered ONLY from watch.namespaces
  # (comma-joined) when cluster_wide is false. Spec CEL rules guarantee
  # namespaces are present exactly when cluster_wide is false.
  watch_cluster_wide = try(var.spec.watch.cluster_wide, null) != null ? var.spec.watch.cluster_wide : true

  # Both halves of config.data are map(string) — this merge cannot hit the
  # type-unification trap the header warns about.
  config_data = {
    for k, v in merge(
      { for ck, cv in try(var.spec.operator_config, {}) : ck => cv if ck != "WATCH_NAMESPACE" },
      { WATCH_NAMESPACE = local.watch_cluster_wide ? null : join(",", try(var.spec.watch.namespaces, [])) }
    ) : k => v if v != null
  }

  config_values = {
    for k, v in {
      # config.clusterWide matches the chart's own default (true) —
      # rendered only when fencing the operator into namespaces.
      clusterWide             = local.watch_cluster_wide ? null : false
      data                    = length(local.config_data) > 0 ? local.config_data : null
      maxConcurrentReconciles = try(var.spec.max_concurrent_reconciles, null)
    } : k => v if v != null
  }

  # ---- own telemetry ---------------------------------------------------------
  # Both flags default false upstream — rendered only when on. The
  # PodMonitor requires the Prometheus operator CRDs on the cluster; the
  # release FAILS to install without them (atomic rolls it back).
  monitoring_values = {
    for k, v in {
      podMonitorEnabled = try(var.spec.monitoring.pod_monitor_enabled, false) ? true : null
      grafanaDashboard  = try(var.spec.monitoring.grafana_dashboard, false) ? { create = true } : null
    } : k => v if v != null
  }

  # ---- image ------------------------------------------------------------------
  # Pull secrets are name references in the chart's values ([{name: ...}]);
  # the image override renders only the halves that are set — an empty tag
  # keeps the chart's appVersion default.
  image_values = {
    for k, v in {
      repository = try(var.spec.image.repository, "") != "" ? var.spec.image.repository : null
      tag        = try(var.spec.image.tag, "") != "" ? var.spec.image.tag : null
    } : k => v if v != null
  }

  # ---- typed chart values (twin of the Pulumi module's buildHelmValues) ------
  # No fullnameOverride: the chart hard-codes the names that matter (the
  # webhook service is "cnpg-webhook-service" regardless of release name)
  # — there is nothing for an override to pin.
  typed_values = {
    for k, v in {
      crds = local.crds_values

      replicaCount = try(var.spec.replicas, null)
      resources    = local.operator_resources != null && length(local.operator_resources) > 0 ? local.operator_resources : null

      config = length(local.config_values) > 0 ? local.config_values : null

      monitoring = length(local.monitoring_values) > 0 ? local.monitoring_values : null

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

      imagePullSecrets = length(try(var.spec.image_pull_secrets, [])) > 0 ? [
        for s in var.spec.image_pull_secrets : { name = s }
      ] : null
      image = length(local.image_values) > 0 ? local.image_values : null
    } : k => v if v != null
  }
}
