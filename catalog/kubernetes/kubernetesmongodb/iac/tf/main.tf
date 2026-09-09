# KubernetesMongodb Terraform module.
#
# Deploys one Percona-operator-managed MongoDB cluster:
#
#   1. the namespace (optional, create_namespace),
#   2. declared-credential Secrets (user passwords, backup-storage keys)
#      — secrets always travel via secret references, never inline in a
#      custom resource,
#   3. the psmdb.percona.com/v1 PerconaServerMongoDB CR itself,
#   4. the PerconaServerMongoDBRestore run, when the spec declares a
#      restore (see locals.tf for the run-once naming contract).
#
# The CR applies through kubectl_manifest (alekc/kubectl): unlike the
# hashicorp provider's kubernetes_manifest resource it needs no cluster
# connection at plan time — the cluster can be PLANNED before the Percona
# operator's CRDs exist, which is what lets an infra chart deploy the
# operator and its databases in one run (and lets offline plan proofs
# work).
#
# No wait_for block, deliberately: cluster readiness depends on the
# operator (image pulls, replica-set initialization, mongos rollout) that
# is not part of applying the resource — the same never-block-on-a-
# controller posture as the KubernetesPostgres exemplar. Pulumi
# equivalent: CustomResource without await annotations.
#
# NAMING CONTRACT: every object the operator creates derives from
# metadata.name — pods `<name>-<rs>-N`, per-set headless Services
# `<name>-<rs>`, the mongos Service `<name>-mongos`, the system-users
# Secret `<name>-secrets`. The module's own satellites are equally
# deterministic (`<name>-user-<username>`, `<name>-backup-<storage>`) so
# the import recipes can derive them blind and both engines agree
# byte-for-byte.

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
# Kubernetes Secret, so nothing sensitive ever appears inline in the
# rendered custom resource. The provider's `data` argument takes
# plaintext values (marked sensitive in state) and handles the base64
# encoding — the Secret lands identical to the Pulumi module's
# stringData.

# Declared user passwords (`<name>-user-<username>`, single `password`
# key) — the shape the CR's passwordSecretRef points at; the operator
# watches the Secret, so rotating the value rotates the database
# password.
resource "kubernetes_secret_v1" "user_password_secret" {
  for_each = local.user_password_secrets

  metadata {
    name      = each.key
    namespace = local.namespace
    labels    = local.labels
  }

  data = {
    password = each.value
  }

  # The operator CO-OWNS this Secret's annotations: after applying a
  # password it stamps a `percona.com/<cluster>-<user>-hash` marker (its
  # rotation detector) onto the object. The module owns the data; the
  # annotations are the operator's — never fight them.
  lifecycle {
    ignore_changes = [metadata[0].annotations]
  }

  depends_on = [kubernetes_namespace_v1.namespace]
}

# Declared backup-storage credentials (`<name>-backup-<storage>`) — keys
# are exactly what the operator's PBM integration reads per backend arm
# (AWS_ACCESS_KEY_ID/AWS_SECRET_ACCESS_KEY, GCS_CLIENT_EMAIL/
# GCS_PRIVATE_KEY, AZURE_STORAGE_ACCOUNT_NAME/AZURE_STORAGE_ACCOUNT_KEY).
# Keyless arms create NO Secret — the PBM agents use the pods' ambient
# cloud identity.
resource "kubernetes_secret_v1" "backup_credentials_secret" {
  for_each = local.backup_credential_secrets

  metadata {
    name      = each.key
    namespace = local.namespace
    labels    = local.labels
  }

  data = each.value

  depends_on = [kubernetes_namespace_v1.namespace]
}

# ---- the PerconaServerMongoDB CR ------------------------------------------------
# Waits for every satellite: the operator reads the user-password and
# backup-credential Secrets at reconcile time, so they must exist before
# the CR does.
resource "kubectl_manifest" "mongodb" {
  yaml_body = yamlencode(local.mongodb_manifest)

  server_side_apply = true

  depends_on = [
    kubernetes_namespace_v1.namespace,
    kubernetes_secret_v1.user_password_secret,
    kubernetes_secret_v1.backup_credentials_secret,
  ]
}

# ---- the PerconaServerMongoDBRestore run -----------------------------------------
# Rendered only when the spec declares a restore; names the cluster and is
# applied after it. The operator gates the run on the members and their PBM
# agents being up and then replays the backup INTO the running cluster — the
# module never waits on that controller-driven verb (no wait_for), the same
# posture as the cluster itself; the run's outcome is the Restore object's
# own status.
resource "kubectl_manifest" "restore" {
  count = local.restore == null ? 0 : 1

  yaml_body = yamlencode(local.restore_manifest)

  server_side_apply = true

  depends_on = [kubectl_manifest.mongodb]
}
