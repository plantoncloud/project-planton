# KubernetesCloudNativePgOperator Terraform module.
#
# Installs CloudNativePG from the official Helm chart as ONE Helm release,
# "cnpg". The release name is FIXED: the operator registers cluster-scoped
# CRDs and webhooks whose service name is baked into the chart (and into
# the webhook certificate) — one installation per cluster is an upstream
# constraint.
#
# BACKUPS LIVE IN A SIBLING KIND: object-store backups run through the
# Barman Cloud CNPG-I plugin, which is a separate chart with its own pin and
# its own cert-manager dependency, installed into THIS release's namespace
# by KubernetesCnpgBarmanCloudPlugin. Upstream forbids folding the plugin
# into the operator's release (Helm ownership of shared resources would
# conflict), so this module never renders it — one kind per release.
#
# The typed spec renders into chart values (locals.typed_values); the
# helm_values escape hatch is passed as a SECOND values document, which
# the provider merges over the first with Helm -f semantics — the exact
# semantic twin of the Pulumi module's buildHelmValues + mergeMaps.

# The optional installation namespace. Created before the release; deleted
# with the resource (pre-existing-namespace installs leave create_namespace
# false).
resource "kubernetes_namespace_v1" "cloudnative_pg" {
  count = try(var.spec.create_namespace, false) ? 1 : 0

  metadata {
    name   = local.namespace
    labels = local.labels
  }
}

# The operator release.
resource "helm_release" "cloudnative_pg" {
  name       = local.release_name
  repository = local.helm_chart_repo
  chart      = local.helm_chart_name
  version    = local.chart_version
  namespace  = local.namespace

  # The module owns namespace creation (create_namespace flag).
  create_namespace = false

  # Wait for the operator to become Available — an operator that never
  # becomes ready (a PodMonitor rendered without the Prometheus operator
  # CRDs is THE classic install failure) should fail THIS apply with a
  # readiness timeout, not surface later as Cluster resources that
  # mysteriously never reconcile.
  wait            = true
  atomic          = true
  cleanup_on_fail = true
  timeout         = 600

  # Two documents, merged in order by the provider (helm -f semantics):
  # the typed rendering first, the user's escape hatch last.
  values = concat(
    [yamlencode(local.typed_values)],
    try(var.spec.helm_values, "") != "" ? [var.spec.helm_values] : []
  )

  depends_on = [kubernetes_namespace_v1.cloudnative_pg]
}
