#!/usr/bin/env bash
# gcp-gke batch teardown: destroys the GCP-side set bootstrap.sh created, in
# reverse dependency order, from the set lane's own per-node workspaces
# (module copy + tfvars + local state under ~/.planton/setdeploy) — and only
# that. The GKE cluster is not this batch's to delete. Run audit.sh after.
set -euo pipefail

# Either the bootstrap-time variable or the env.sh the bootstrap wrote.
GCP_PROJECT_ID="${GCP_PROJECT_ID:-${PLANTON_E2E_GCP_PROJECT:-}}"
: "${GCP_PROJECT_ID:?set GCP_PROJECT_ID or source env.sh}"
state_dir="${HOME}/.planton-e2e/planton-e2e-gke"
setdeploy_root="${HOME}/.planton/setdeploy/default"
export GOOGLE_PROJECT="${GCP_PROJECT_ID}"

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

# Reverse of the apply order: the bucket (whose IAM grants reference the
# identities) first, then the bindings, then the identities.
destroy_node gcpgcsbucket planton-e2e-gke-backups
destroy_node gcpgkeworkloadidentitybinding planton-e2e-gke-pg-dr-wi
destroy_node gcpgkeworkloadidentitybinding planton-e2e-gke-pg-src-wi
destroy_node gcpserviceaccount planton-e2e-gke-mongo-backup
destroy_node gcpserviceaccount planton-e2e-gke-pg-backup

rm -f "${state_dir}/env.sh" "${state_dir}/kubeconfig"
echo "==> teardown finished; run audit.sh"
