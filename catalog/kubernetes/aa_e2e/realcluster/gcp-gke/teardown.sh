#!/usr/bin/env bash
# gcp-gke batch teardown: destroys the GCP- and Cloudflare-side set
# bootstrap.sh created, in reverse dependency order, from the set lane's own
# per-node workspaces (module copy + tfvars + local state under
# ~/.planton/setdeploy) — and only that. The GKE cluster is not this batch's
# to delete. Run audit.sh after.
set -euo pipefail

# Either the bootstrap-time variables or the env.sh the bootstrap wrote.
GCP_PROJECT_ID="${GCP_PROJECT_ID:-${PLANTON_E2E_GCP_PROJECT:-}}"
: "${GCP_PROJECT_ID:?set GCP_PROJECT_ID or source env.sh}"
: "${CLOUDFLARE_API_TOKEN:?set CLOUDFLARE_API_TOKEN (the Cloudflare nodes destroy through it)}"
state_dir="${HOME}/.planton-e2e/planton-e2e-gke"
setdeploy_root="${HOME}/.planton/setdeploy/default"
export GOOGLE_PROJECT="${GCP_PROJECT_ID}"

# R2 refuses to delete a bucket that still holds objects and the Cloudflare
# provider has no force-destroy, so the lanes' archives are emptied over the
# S3 API first — with the batch's own writer token, from env.sh (the same
# derivation bootstrap.sh performed). Without env.sh there is nothing to
# empty with; audit.sh reports a surviving bucket and names the remedy.
if [ -n "${PLANTON_E2E_GKE_R2_ACCESS_KEY_ID:-}" ] && [ -n "${PLANTON_E2E_GKE_R2_SECRET_ACCESS_KEY:-}" ]; then
  command -v aws >/dev/null 2>&1 || { echo "aws CLI not found: it empties the R2 bucket over the S3 API before the bucket can be destroyed (brew install awscli)" >&2; exit 1; }
  r2_endpoint="https://${PLANTON_E2E_GKE_R2_ACCOUNT_ID}.r2.cloudflarestorage.com"
  echo "==> emptying R2 bucket ${PLANTON_E2E_GKE_R2_BUCKET} over the S3 API"
  AWS_ACCESS_KEY_ID="${PLANTON_E2E_GKE_R2_ACCESS_KEY_ID}" AWS_SECRET_ACCESS_KEY="${PLANTON_E2E_GKE_R2_SECRET_ACCESS_KEY}" AWS_REGION=auto \
    aws s3 rm "s3://${PLANTON_E2E_GKE_R2_BUCKET}" --recursive --endpoint-url "${r2_endpoint}" >/dev/null \
    || echo "    (emptying the bucket reported an error; audit.sh decides)"
else
  echo "    env.sh not sourced: skipping the R2 bucket sweep (a non-empty bucket will refuse to destroy — audit.sh reports it)"
fi

destroy_node() {
  local kind_dir="$1" name="$2" ws="${setdeploy_root}/$1/$2"
  if [ ! -f "${ws}/terraform.tfstate" ]; then
    echo "    ${kind_dir}/${name}: no state (never deployed here) — skipping"
    return 0
  fi
  echo "==> destroying ${kind_dir}/${name}"
  (cd "${ws}" && tofu destroy -auto-approve -var-file=.terraform/terraform.tfvars) \
    || echo "    (destroy of ${kind_dir}/${name} reported an error; audit.sh decides)"
}

# Reverse of the apply order. Cloudflare side: the token (scoped to the
# bucket) then the emptied bucket. GCP side: the bucket (whose IAM grants
# reference the identities) first, then the bindings, then the identities.
destroy_node cloudflareaccountapitoken planton-e2e-gke-backups-writer
destroy_node cloudflarer2bucket planton-e2e-gke-backups-r2
destroy_node gcpgcsbucket planton-e2e-gke-backups
destroy_node gcpgkeworkloadidentitybinding planton-e2e-gke-pg-dr-wi
destroy_node gcpgkeworkloadidentitybinding planton-e2e-gke-pg-src-wi
destroy_node gcpserviceaccount planton-e2e-gke-mongo-backup
destroy_node gcpserviceaccount planton-e2e-gke-pg-backup

rm -f "${state_dir}/env.sh" "${state_dir}/kubeconfig"
echo "==> teardown finished; run audit.sh"
