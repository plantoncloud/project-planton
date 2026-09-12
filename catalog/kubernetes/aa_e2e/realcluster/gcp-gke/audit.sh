#!/usr/bin/env bash
# gcp-gke batch audit: enumerates every GCP and Cloudflare resource class the
# batch can leave behind and fails on any survivor. Read-only; safe at any
# time.
set -uo pipefail

# Either the bootstrap-time variables or the env.sh the bootstrap wrote.
GCP_PROJECT_ID="${GCP_PROJECT_ID:-${PLANTON_E2E_GCP_PROJECT:-}}"
: "${GCP_PROJECT_ID:?set GCP_PROJECT_ID or source env.sh}"
CLOUDFLARE_ACCOUNT_ID="${CLOUDFLARE_ACCOUNT_ID:-${PLANTON_E2E_GKE_R2_ACCOUNT_ID:-}}"
: "${CLOUDFLARE_ACCOUNT_ID:?set CLOUDFLARE_ACCOUNT_ID or source env.sh}"
: "${CLOUDFLARE_API_TOKEN:?set CLOUDFLARE_API_TOKEN (the Cloudflare side is read through it)}"
rc=0

echo "==> service accounts"
for sa in planton-e2e-gke-pg-backup planton-e2e-gke-mongo-backup; do
  if gcloud iam service-accounts describe "${sa}@${GCP_PROJECT_ID}.iam.gserviceaccount.com" --project "${GCP_PROJECT_ID}" >/dev/null 2>&1; then
    echo "  SURVIVOR: service account ${sa}"; rc=1
  fi
done

echo "==> bucket"
if gcloud storage buckets describe "gs://planton-e2e-gke-backups-${GCP_PROJECT_ID}" >/dev/null 2>&1; then
  echo "  SURVIVOR: bucket planton-e2e-gke-backups-${GCP_PROJECT_ID}"; rc=1
fi

# Workload Identity bindings live ON the service accounts (IAM policy of the
# GSA); a surviving binding implies a surviving account, caught above.

cf_api="https://api.cloudflare.com/client/v4/accounts/${CLOUDFLARE_ACCOUNT_ID}"
echo "==> R2 bucket"
if curl -sf -H "Authorization: Bearer ${CLOUDFLARE_API_TOKEN}" "${cf_api}/r2/buckets/planton-e2e-gke-backups" >/dev/null 2>&1; then
  echo "  SURVIVOR: R2 bucket planton-e2e-gke-backups (if it still holds objects: empty it over the S3 API with the writer token, then destroy the node)"; rc=1
fi

echo "==> R2 writer token"
if curl -s -H "Authorization: Bearer ${CLOUDFLARE_API_TOKEN}" "${cf_api}/tokens?per_page=50" | python3 -c 'import json,sys; sys.exit(0 if any(t.get("name")=="planton-e2e-gke-backups-writer" for t in json.load(sys.stdin).get("result") or []) else 1)' 2>/dev/null; then
  echo "  SURVIVOR: account API token planton-e2e-gke-backups-writer"; rc=1
fi

if [ "${rc}" -eq 0 ]; then echo "==> audit clean: zero batch residue in ${GCP_PROJECT_ID} and Cloudflare account ${CLOUDFLARE_ACCOUNT_ID}"; else echo "==> audit FAILED"; fi
exit "${rc}"
