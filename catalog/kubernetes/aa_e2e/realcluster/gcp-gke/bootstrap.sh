#!/usr/bin/env bash
# gcp-gke batch bootstrap: the GCP-side prerequisites of the Kubernetes
# real-cluster lanes on an EXISTING GKE cluster, created from Planton's own
# catalog through the CLI's set lane (dependency-ordered, references resolved
# between the manifests exactly as an infra chart would), then the env file
# the lanes source.
#
# What it creates (manifests/): two backup identities (a keyless one the
# Postgres pods assume via Workload Identity, a keyed one PBM presents for
# MongoDB), the Workload Identity bindings for the Postgres source and
# recovery clusters, the GCS backup bucket granting both identities
# object-admin, and the Cloudflare R2 side the databases' `r2` arms archive
# to (a bucket and an account API token scoped to it) — one cross-provider
# set, dependency-ordered by the set lane. The GKE cluster itself is NOT
# created here — this batch runs against a cluster the operator already owns
# (Workload Identity must be enabled on it), which is the adopter's real
# shape.
#
# Inputs (environment):
#   GCP_PROJECT_ID          project the cluster and the batch resources live in (required)
#   GCP_REGION              bucket location, e.g. asia-south1 (required)
#   KUBE_CONTEXT            kubectl context of the GKE cluster (required)
#   CLOUDFLARE_API_TOKEN    the Cloudflare credential both engines read (required;
#                           needs Workers R2 Storage Write + Account API Tokens Write)
#   CLOUDFLARE_ACCOUNT_ID   the Cloudflare account the R2 side lives in (required)
#   PLANTON_BIN             the planton CLI to use (default: planton on PATH; point it
#                           at a tree build when the lanes exercise unreleased specs)
#
# Idempotent: re-running converges the same set (pulumi up on the same local
# backend) and rewrites the env file.
set -euo pipefail

here="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
: "${GCP_PROJECT_ID:?set GCP_PROJECT_ID to the project the GKE cluster lives in}"
: "${GCP_REGION:?set GCP_REGION to the bucket location (e.g. asia-south1)}"
: "${KUBE_CONTEXT:?set KUBE_CONTEXT to the kubectl context of the GKE cluster}"
: "${CLOUDFLARE_API_TOKEN:?set CLOUDFLARE_API_TOKEN to a Cloudflare token with Workers R2 Storage Write and Account API Tokens Write}"
: "${CLOUDFLARE_ACCOUNT_ID:?set CLOUDFLARE_ACCOUNT_ID to the Cloudflare account the R2 side lives in}"
PLANTON_BIN="${PLANTON_BIN:-planton}"

state_dir="${HOME}/.planton-e2e/planton-e2e-gke"
mkdir -p "${state_dir}/rendered"
# The set lane keeps one tofu workspace per node (module copy, tfvars, local
# state) under this root — teardown.sh destroys from the same workspaces.
setdeploy_root="${HOME}/.planton/setdeploy/default"

# Every lane and every pulumi subprocess must agree on the project; the GCP
# provider reads GOOGLE_PROJECT and the ambient ADC chain.
export GOOGLE_PROJECT="${GCP_PROJECT_ID}"
gcloud auth application-default print-access-token >/dev/null 2>&1 \
  || { echo "no Application Default Credentials: run 'gcloud auth application-default login'" >&2; exit 1; }

# The manifests carry ${GCP_PROJECT_ID}/${GCP_REGION}/${CLOUDFLARE_ACCOUNT_ID}
# placeholders so the committed set names no one test account's identifiers;
# render them into the state dir and hand the directory to the set lane.
for f in "${here}"/manifests/*.yaml; do
  GCP_PROJECT_ID="${GCP_PROJECT_ID}" GCP_REGION="${GCP_REGION}" CLOUDFLARE_ACCOUNT_ID="${CLOUDFLARE_ACCOUNT_ID}" \
    envsubst '${GCP_PROJECT_ID} ${GCP_REGION} ${CLOUDFLARE_ACCOUNT_ID}' < "${f}" > "${state_dir}/rendered/$(basename "${f}")"
done

# The set lane resolves each node's module from a PUBLISHED release (it
# refuses --local-module by design — one flag cannot name modules for many
# kinds), so the GCP-side kinds this batch uses must be released ones; they
# are, and the lanes never change them. Pin with PLANTON_MODULE_VERSION when
# a specific release must be exercised. Nodes without a provisioner
# annotation route to tofu with node-local state (the set lane's default,
# stated in its preflight report). Provider credentials are ambient per
# node: ADC for the GCP kinds, CLOUDFLARE_API_TOKEN for the Cloudflare
# kinds (the set lane has no Cloudflare preflight probe — a bad token
# surfaces at the node's apply, not at the wall).
echo "==> applying the gcp-gke batch set from the catalog (project ${GCP_PROJECT_ID}, region ${GCP_REGION}, Cloudflare account ${CLOUDFLARE_ACCOUNT_ID})"
"${PLANTON_BIN}" apply -f "${state_dir}/rendered" --yes \
  ${PLANTON_MODULE_VERSION:+--module-version "${PLANTON_MODULE_VERSION}"}

# The lanes read the outputs through env tokens. Emails and the bucket name
# are deterministic from the manifests; the Mongo key is the identity node's
# stack output, read from its set-lane workspace state (a live credential —
# the env file is mode 600 and lives outside the repo).
mongo_gsa="planton-e2e-gke-mongo-backup@${GCP_PROJECT_ID}.iam.gserviceaccount.com"
pg_gsa="planton-e2e-gke-pg-backup@${GCP_PROJECT_ID}.iam.gserviceaccount.com"
bucket="planton-e2e-gke-backups-${GCP_PROJECT_ID}"

mongo_state="${setdeploy_root}/gcpserviceaccount/planton-e2e-gke-mongo-backup/terraform.tfstate"
[ -f "${mongo_state}" ] || { echo "set-lane state for the mongo backup identity not found at ${mongo_state}" >&2; exit 1; }
key_b64="$(python3 -c 'import json,sys; print(json.load(open(sys.argv[1]))["outputs"]["key_base64"]["value"])' "${mongo_state}")"
[ -n "${key_b64}" ] || { echo "the mongo backup identity exported no key_base64" >&2; exit 1; }

# The R2 side: the bucket name and jurisdiction are fixed by the manifest;
# the token's S3 pair is Cloudflare's rule applied to the token node's
# outputs — access key id = the token's id, secret access key = the SHA-256
# of its value. The token kind exports exactly this pair
# (r2_access_key_id / r2_secret_access_key); the set lane deploys the node
# from the PUBLISHED module, so until a release carries those outputs the
# derivation is repeated here, from the same two facts.
r2_bucket="planton-e2e-gke-backups"
r2_jurisdiction="default"
token_state="${setdeploy_root}/cloudflareaccountapitoken/planton-e2e-gke-backups-writer/terraform.tfstate"
[ -f "${token_state}" ] || { echo "set-lane state for the R2 writer token not found at ${token_state}" >&2; exit 1; }
r2_access_key_id="$(python3 -c 'import json,sys; print(json.load(open(sys.argv[1]))["outputs"]["token_id"]["value"])' "${token_state}")"
r2_token_value="$(python3 -c 'import json,sys; print(json.load(open(sys.argv[1]))["outputs"]["value"]["value"])' "${token_state}")"
[ -n "${r2_access_key_id}" ] && [ -n "${r2_token_value}" ] || { echo "the R2 writer token exported no token_id/value" >&2; exit 1; }
r2_secret_access_key="$(printf '%s' "${r2_token_value}" | shasum -a 256 | awk '{print $1}')"

kubeconfig="${state_dir}/kubeconfig"
kubectl config view --minify --flatten --context "${KUBE_CONTEXT}" > "${kubeconfig}"
chmod 600 "${kubeconfig}"

cat > "${state_dir}/env.sh" <<EOF
# gcp-gke batch — source before running the lanes. Generated by bootstrap.sh.
export PLANTON_E2E_KUBECONFIG="${kubeconfig}"
export PLANTON_E2E_CLUSTER_PROFILE="gcp-gke"
export GOOGLE_PROJECT="${GCP_PROJECT_ID}"
export PLANTON_E2E_GCP_PROJECT="${GCP_PROJECT_ID}"
export PLANTON_E2E_GCP_REGION="${GCP_REGION}"
export PLANTON_E2E_GKE_BACKUP_BUCKET="${bucket}"
export PLANTON_E2E_GKE_PG_BACKUP_GSA="${pg_gsa}"
export PLANTON_E2E_GKE_MONGO_BACKUP_GSA="${mongo_gsa}"
export PLANTON_E2E_GKE_MONGO_BACKUP_KEY_B64="${key_b64}"
export PLANTON_E2E_GKE_R2_ACCOUNT_ID="${CLOUDFLARE_ACCOUNT_ID}"
export PLANTON_E2E_GKE_R2_BUCKET="${r2_bucket}"
export PLANTON_E2E_GKE_R2_JURISDICTION="${r2_jurisdiction}"
export PLANTON_E2E_GKE_R2_ACCESS_KEY_ID="${r2_access_key_id}"
export PLANTON_E2E_GKE_R2_SECRET_ACCESS_KEY="${r2_secret_access_key}"
EOF
chmod 600 "${state_dir}/env.sh"

echo "==> batch ready. Next:"
echo "    source ${state_dir}/env.sh"
echo "    go test -tags=e2e -timeout=60m -v -count=1 -run 'TestKubernetesPostgres_' ./e2e/"
echo "    go test -tags=e2e -timeout=60m -v -count=1 -run 'TestKubernetesMongodb_' ./e2e/"
