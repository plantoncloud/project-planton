# KubernetesPostgres Terraform module.
#
# Deploys one CloudNativePG-managed PostgreSQL cluster:
#
#   1. the namespace (optional, create_namespace),
#   2. declared-credential Secrets (app/superuser/role passwords,
#      external-cluster passwords, object-store keys) — secrets always
#      travel via secretKeyRef, never inline in a custom resource,
#   3. the Barman Cloud ObjectStore CR(s) — the backup destination and,
#      for recovery bootstraps, the restore source,
#   4. the postgresql.cnpg.io/v1 Cluster CR itself,
#   5. one ScheduledBackup CR per declared schedule.
#
# The CRs apply through kubectl_manifest (alekc/kubectl): unlike the
# hashicorp provider's kubernetes_manifest resource it needs no cluster
# connection at plan time — a Cluster can be PLANNED before CloudNativePG's
# CRDs exist, which is what lets an infra chart deploy the operator and its
# databases in one run (and lets offline plan proofs work).
#
# No wait_for block, deliberately: cluster readiness depends on the
# operator (image pulls, initdb/recovery, replica sync) that is not part of
# applying the resource — the same never-block-on-a-controller posture as
# the cert-manager CR modules. Pulumi equivalent: CustomResource without
# await annotations.
#
# NAMING CONTRACT: every object CloudNativePG creates derives from
# metadata.name — pods `<name>-N`, services `<name>-rw/-ro/-r`, credential
# secrets `<name>-app` / `<name>-superuser`. The module's own satellites are
# equally deterministic (`<name>-backup-creds`, `<name>-recovery-source`,
# `<name>-role-<role>`, ...) so the import recipes can derive them blind and
# both engines agree byte-for-byte.

# The optional namespace. Created before everything (all module resources
# are namespaced); deleted with the resource. Pre-existing-namespace
# deployments leave create_namespace false.
resource "kubernetes_namespace_v1" "namespace" {
  count = try(var.spec.create_namespace, false) ? 1 : 0

  metadata {
    name   = local.namespace
    labels = local.labels
  }
}

# ---- credential Secrets -------------------------------------------------------
# Every DECLARED credential in the spec materializes as a deterministic
# Kubernetes Secret, so nothing sensitive ever appears inline in a rendered
# custom resource. The provider's `data` argument takes plaintext values
# (marked sensitive in state) and handles the base64 encoding — the Secret
# lands identical to the Pulumi module's stringData.

# Application-owner password (initdb bootstrap): kubernetes.io/basic-auth
# with the OWNER's username — CloudNativePG requires both keys and adopts
# this secret as the application credential instead of generating one.
resource "kubernetes_secret_v1" "provided_app_secret" {
  count = try(local.initdb.owner_password, "") != "" ? 1 : 0

  metadata {
    name      = local.provided_app_secret_name
    namespace = local.namespace
    labels    = local.labels
  }

  type = "kubernetes.io/basic-auth"
  data = {
    username = local.provided_app_username
    password = local.initdb.owner_password
  }

  depends_on = [kubernetes_namespace_v1.namespace]
}

# Superuser password: the operator only honors it while superuser access is
# enabled (the spec CEL enforces the pairing). Username is always postgres.
resource "kubernetes_secret_v1" "provided_superuser_secret" {
  count = try(var.spec.superuser.password, "") != "" ? 1 : 0

  metadata {
    name      = local.provided_superuser_secret_name
    namespace = local.namespace
    labels    = local.labels
  }

  type = "kubernetes.io/basic-auth"
  data = {
    username = "postgres"
    password = var.spec.superuser.password
  }

  depends_on = [kubernetes_namespace_v1.namespace]
}

# Managed-role passwords (`<name>-role-<role>`): basic-auth pairs the
# operator WATCHES — rotating the value rotates the database password.
resource "kubernetes_secret_v1" "role_password_secret" {
  for_each = local.role_password_secrets

  metadata {
    name      = each.key
    namespace = local.namespace
    labels    = local.labels
  }

  type = "kubernetes.io/basic-auth"
  data = each.value

  depends_on = [kubernetes_namespace_v1.namespace]
}

# External-cluster passwords (`<name>-ext-<external>`, single `password`
# key; the operator builds a passfile from it).
resource "kubernetes_secret_v1" "external_cluster_password_secret" {
  for_each = local.external_cluster_password_secrets

  metadata {
    name      = each.key
    namespace = local.namespace
    labels    = local.labels
  }

  data = {
    password = each.value
  }

  depends_on = [kubernetes_namespace_v1.namespace]
}

# Object-store credentials (`<name>-backup-creds` / `<name>-recovery-creds`)
# — keys depend on the backend arm (ACCESS_KEY_ID/SECRET_ACCESS_KEY,
# APPLICATION_CREDENTIALS, or AZURE_*). Keyless arms render the backend's
# ambient-identity flag instead and create NO creds Secret — except Azure
# keyless, which still carries AZURE_STORAGE_ACCOUNT (the endpoint identity).
resource "kubernetes_secret_v1" "backup_credentials_secret" {
  count = lookup(local.object_store_creds_data, "backup", null) != null ? 1 : 0

  metadata {
    name      = local.backup_creds_secret_name
    namespace = local.namespace
    labels    = local.labels
  }

  data = local.object_store_creds_data["backup"]

  depends_on = [kubernetes_namespace_v1.namespace]
}

resource "kubernetes_secret_v1" "recovery_credentials_secret" {
  count = lookup(local.object_store_creds_data, "recovery", null) != null ? 1 : 0

  metadata {
    name      = local.recovery_creds_secret_name
    namespace = local.namespace
    labels    = local.labels
  }

  data = local.object_store_creds_data["recovery"]

  depends_on = [kubernetes_namespace_v1.namespace]
}

# S3 region secrets (`<store>-region`, key AWS_REGION): the ObjectStore CRD
# models the region as a SecretKeySelector, not a plain string, so the
# literal region rides its own deterministic single-key Secret. Rendered for
# the s3 arm's declared region and for the r2 arm (always "auto");
# barman-cloud reads it only on the declared-key path (see locals.tf).
resource "kubernetes_secret_v1" "backup_region_secret" {
  count = lookup(local.object_store_region, "backup", "") != "" ? 1 : 0

  metadata {
    name      = "${local.backup_object_store_name}-region"
    namespace = local.namespace
    labels    = local.labels
  }

  data = {
    AWS_REGION = local.object_store_region["backup"]
  }

  depends_on = [kubernetes_namespace_v1.namespace]
}

resource "kubernetes_secret_v1" "recovery_region_secret" {
  count = lookup(local.object_store_region, "recovery", "") != "" ? 1 : 0

  metadata {
    name      = "${local.recovery_object_store_name}-region"
    namespace = local.namespace
    labels    = local.labels
  }

  data = {
    AWS_REGION = local.object_store_region["recovery"]
  }

  depends_on = [kubernetes_namespace_v1.namespace]
}

# Endpoint CA secrets (`ca.crt` key) for self-signed S3-compatible
# endpoints.
resource "kubernetes_secret_v1" "backup_endpoint_ca_secret" {
  count = try(local.backup.object_store.s3.endpoint_ca_pem, "") != "" ? 1 : 0

  metadata {
    name      = local.backup_endpoint_ca_name
    namespace = local.namespace
    labels    = local.labels
  }

  data = {
    "ca.crt" = local.backup.object_store.s3.endpoint_ca_pem
  }

  depends_on = [kubernetes_namespace_v1.namespace]
}

resource "kubernetes_secret_v1" "recovery_endpoint_ca_secret" {
  count = try(local.recovery.object_store.s3.endpoint_ca_pem, "") != "" ? 1 : 0

  metadata {
    name      = local.recovery_endpoint_ca_name
    namespace = local.namespace
    labels    = local.labels
  }

  data = {
    "ca.crt" = local.recovery.object_store.s3.endpoint_ca_pem
  }

  depends_on = [kubernetes_namespace_v1.namespace]
}

# ---- Barman Cloud ObjectStores ------------------------------------------------
# The BACKUP store (named after the cluster) when spec.backup is set — the
# Cluster's plugins entry points the WAL archiver at it. Ordered after its
# credential satellites: the plugin sidecar resolves the store (and its
# secretRefs) at instance startup.
resource "kubectl_manifest" "backup_object_store" {
  count = local.backup != null ? 1 : 0

  yaml_body = yamlencode(local.object_store_manifests["backup"])

  server_side_apply = true

  depends_on = [
    kubernetes_namespace_v1.namespace,
    kubernetes_secret_v1.backup_credentials_secret,
    kubernetes_secret_v1.backup_region_secret,
    kubernetes_secret_v1.backup_endpoint_ca_secret,
  ]
}

# The RECOVERY-SOURCE store (`<name>-recovery-source`) when the bootstrap
# restores from a backup — the synthetic "origin" externalClusters entry
# points at it. Never carries a retentionPolicy: recovery reads an EXISTING
# archive and must not prune the source cluster's backups.
resource "kubectl_manifest" "recovery_object_store" {
  count = local.recovery != null ? 1 : 0

  yaml_body = yamlencode(local.object_store_manifests["recovery"])

  server_side_apply = true

  depends_on = [
    kubernetes_namespace_v1.namespace,
    kubernetes_secret_v1.recovery_credentials_secret,
    kubernetes_secret_v1.recovery_region_secret,
    kubernetes_secret_v1.recovery_endpoint_ca_secret,
  ]
}

# ---- the Cluster --------------------------------------------------------------
# Waits for every satellite: credential Secrets must exist before the
# operator reads them, and the plugin resolves the ObjectStores at
# reconcile time.
resource "kubectl_manifest" "cluster" {
  yaml_body = yamlencode(local.cluster_manifest)

  server_side_apply = true

  depends_on = [
    kubernetes_namespace_v1.namespace,
    kubernetes_secret_v1.provided_app_secret,
    kubernetes_secret_v1.provided_superuser_secret,
    kubernetes_secret_v1.role_password_secret,
    kubernetes_secret_v1.external_cluster_password_secret,
    kubernetes_secret_v1.backup_credentials_secret,
    kubernetes_secret_v1.recovery_credentials_secret,
    kubernetes_secret_v1.backup_region_secret,
    kubernetes_secret_v1.recovery_region_secret,
    kubernetes_secret_v1.backup_endpoint_ca_secret,
    kubernetes_secret_v1.recovery_endpoint_ca_secret,
    kubectl_manifest.backup_object_store,
    kubectl_manifest.recovery_object_store,
  ]
}

# ---- ScheduledBackups -----------------------------------------------------------
# One per declared schedule (`<cluster>-<schedule>`). The operator tolerates
# ScheduledBackups arriving with the Cluster, but ordering them after keeps
# apply logs clean.
resource "kubectl_manifest" "scheduled_backup" {
  for_each = local.scheduled_backup_manifests

  yaml_body = yamlencode(each.value)

  server_side_apply = true

  depends_on = [
    kubernetes_namespace_v1.namespace,
    kubectl_manifest.cluster,
  ]
}
