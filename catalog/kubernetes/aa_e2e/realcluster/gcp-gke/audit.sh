#!/usr/bin/env bash
# gcp-gke batch audit: enumerates every GCP resource class the batch can
# leave behind and fails on any survivor. Read-only; safe at any time.
set -uo pipefail

# Either the bootstrap-time variable or the env.sh the bootstrap wrote.
GCP_PROJECT_ID="${GCP_PROJECT_ID:-${PLANTON_E2E_GCP_PROJECT:-}}"
: "${GCP_PROJECT_ID:?set GCP_PROJECT_ID or source env.sh}"
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

if [ "${rc}" -eq 0 ]; then echo "==> audit clean: zero batch residue in ${GCP_PROJECT_ID}"; else echo "==> audit FAILED"; fi
exit "${rc}"
