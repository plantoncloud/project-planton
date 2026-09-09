# KubernetesCloudNativePgOperator Terraform module.
#
# Installs CloudNativePG from the official Helm charts as up to TWO real
# Helm releases in the same namespace:
#
#   1. "cnpg" — the operator chart (cloudnative-pg). The release name is
#      FIXED: the operator registers cluster-scoped CRDs and webhooks whose
#      service name is baked into the chart (and into the webhook
#      certificate) — one installation per cluster is an upstream
#      constraint.
#   2. "plugin-barman-cloud" (when spec.barman_cloud_plugin.enabled) — the
#      Barman Cloud CNPG-I plugin chart, the object-store backup path for
#      every KubernetesPostgres on the cluster. A SEPARATE release in the
#      SAME namespace: upstream forbids folding the plugin into the
#      operator's release (Helm ownership of shared resources would
#      conflict). Installed AFTER the operator so the plugin's CNPG-I
#      registration always lands on a running operator.
#
# PLUGIN-ONLY POSTURE (spec.install_operator false): a CloudNativePG
# already runs on the cluster — the Planton operator installs one for the
# platform's own database, and Helm/GitOps installs are common — and the
# operator's CRDs and webhooks are cluster singletons, so no operator
# release renders at all. Only the plugin release is managed, into the
# declared namespace beside the resident operator, and it registers with
# that operator over CNPG-I exactly as it would with one this module
# installed. The operator-release output is then honestly empty.
#
# CERT-MANAGER DEPENDENCY (deliberate, documented): the plugin chart
# renders cert-manager Issuer/Certificate resources UNCONDITIONALLY — its
# operator↔sidecar TLS is issued by cert-manager. Without cert-manager on
# the cluster (KubernetesCertManager) the plugin release fails to install;
# atomic rolls it back cleanly.
#
# The typed spec renders into operator-chart values (locals.typed_values);
# the helm_values escape hatch is passed as a SECOND values document, which
# the provider merges over the first with Helm -f semantics — the exact
# semantic twin of the Pulumi module's buildHelmValues + mergeMaps.
# helm_values scopes to the OPERATOR chart only.

# The optional installation namespace. Created before the releases; deleted
# with the resource (pre-existing-namespace installs leave create_namespace
# false).
resource "kubernetes_namespace_v1" "cloudnative_pg" {
  count = try(var.spec.create_namespace, false) ? 1 : 0

  metadata {
    name   = local.namespace
    labels = local.labels
  }
}

# The operator release — absent in the plugin-only posture.
resource "helm_release" "cloudnative_pg" {
  count = local.install_operator ? 1 : 0

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

# The Barman Cloud plugin release. Ordered AFTER the operator release when
# this module installs one: the plugin registers itself with the operator
# over CNPG-I, so the operator (and its CRDs) must exist first — destroy
# unwinds in reverse for free. In the plugin-only posture the operator
# release has count 0 and the dependency is vacuous; the resident operator
# is already running.
resource "helm_release" "barman_cloud_plugin" {
  count = local.barman_plugin_enabled ? 1 : 0

  name       = local.plugin_release_name
  repository = local.helm_chart_repo
  chart      = local.plugin_chart_name
  version    = local.plugin_chart_version
  namespace  = local.namespace

  # The module owns namespace creation (create_namespace flag).
  create_namespace = false

  # Same atomic/wait posture as the operator release. This is also where
  # the cert-manager dependency surfaces: without cert-manager the
  # plugin's Certificate resources never become ready and the release
  # rolls back with a clear timeout.
  wait            = true
  atomic          = true
  cleanup_on_fail = true
  timeout         = 600

  values = length(local.plugin_typed_values) > 0 ? [yamlencode(local.plugin_typed_values)] : []

  depends_on = [kubernetes_namespace_v1.cloudnative_pg, helm_release.cloudnative_pg]
}
