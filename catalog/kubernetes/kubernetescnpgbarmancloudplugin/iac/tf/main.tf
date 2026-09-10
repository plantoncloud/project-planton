# KubernetesCnpgBarmanCloudPlugin Terraform module.
#
# Installs the Barman Cloud plugin for CloudNativePG from the official Helm
# chart as ONE Helm release, "plugin-barman-cloud", in the operator's
# namespace.
#
# WHY ITS OWN RELEASE, ITS OWN KIND: the plugin is a different chart with a
# different pin and a different dependency set than the operator (it needs
# cert-manager; the operator does not), and on many clusters a different
# owner -- a CloudNativePG installed by Helm, GitOps, or a platform operator
# has no plugin, and this kind is how that cluster gets one. Upstream
# forbids folding the plugin into the operator's release (Helm ownership of
# shared resources would conflict), so one kind per release is also the
# honest grain.
#
# WHY THE NAMESPACE IS THE OPERATOR'S: CloudNativePG discovers plugins
# through Services labeled `cnpg.io/pluginName` in its OWN namespace only
# (the operator's plugin predicate rejects any other namespace), and the
# chart fixes that Service's name to "barman-cloud" because it is baked into
# the plugin's TLS certificate. A plugin anywhere else is invisible to the
# operator, and every backup-declaring database stays parked in the
# "unknown plugin being required" phase.
#
# CERT-MANAGER DEPENDENCY (deliberate, documented): the chart renders a
# self-signed Issuer and two Certificates for the operator <-> plugin gRPC
# TLS UNCONDITIONALLY. Without cert-manager on the cluster
# (KubernetesCertManager) the Certificates never become ready and the
# release times out; atomic rolls it back cleanly with a clear message.
#
# ORDERING: the operator (and its CRDs) must exist before the plugin
# registers over CNPG-I. When the operator is declared in the same
# composition, the `namespace` reference onto the operator resource IS the
# ordering edge; against a resident operator the namespace is a literal and
# the operator is already running.
#
# The typed spec renders into chart values (locals.typed_values); the
# helm_values escape hatch is passed as a SECOND values document, which the
# provider merges over the first with Helm -f semantics; the chart's fixed
# identities are re-pinned by a THIRD document -- the exact semantic twin of
# the Pulumi module's buildHelmValues (typed -> escape hatch -> pins).

# The optional installation namespace. Almost always pre-existing here (it
# is the OPERATOR's namespace); created only when create_namespace is true
# and then deleted with the resource.
resource "kubernetes_namespace_v1" "barman_cloud_plugin" {
  count = try(var.spec.create_namespace, false) ? 1 : 0

  metadata {
    name   = local.namespace
    labels = local.labels
  }
}

# The plugin release.
resource "helm_release" "barman_cloud_plugin" {
  name       = local.release_name
  repository = local.helm_chart_repo
  chart      = local.helm_chart_name
  version    = local.chart_version
  namespace  = local.namespace

  # The module owns namespace creation (create_namespace flag).
  create_namespace = false

  # Wait for the plugin to become Available -- a plugin that never becomes
  # ready (cert-manager absent, so its Certificates never issue) should
  # fail THIS apply with a readiness timeout, not surface later as
  # databases that mysteriously never reconcile.
  wait            = true
  atomic          = true
  cleanup_on_fail = true
  timeout         = 600

  # Three documents, merged in order by the provider (helm -f semantics):
  # the typed rendering first, the user's escape hatch second, the fixed
  # identity pins last -- so the escape hatch can never rename the Service
  # the operator handshake depends on.
  values = concat(
    length(local.typed_values) > 0 ? [yamlencode(local.typed_values)] : [],
    try(var.spec.helm_values, "") != "" ? [var.spec.helm_values] : [],
    [yamlencode(local.fixed_identity_pins)]
  )

  depends_on = [kubernetes_namespace_v1.barman_cloud_plugin]
}
