#!/usr/bin/env bash
# SETUP for the gke-gcs-recovery scenario: seeds the SOURCE cluster
# (fixture-gke-source.yaml, already deployed and healthy) with the evidence
# the recovery proof reads back.
#
#   1. marker A  -> a row written BEFORE the base backup
#   2. a Backup   -> a real base backup into the GCS archive, waited to Completed
#   3. marker B  -> a row written AFTER the base backup, then a WAL switch so
#                   the segment carrying it reaches the archive
#
# A recovered cluster carrying A proves the base backup; carrying B proves
# WAL replay past it (continuous archiving). Everything lands inside the
# fixture-owned cluster and namespace, so DEPENDENCIES-DOWN removes it.
# Idempotent: markers upsert by name, the Backup is named per engine lane.
set -euo pipefail

ns="e2e-pg-gke"
cluster="e2e-pg-gke-src"
backup="${cluster}-seed-${E2E_RUN_ID}"
plugin="barman-cloud.cloudnative-pg.io"

primary() {
  kubectl get cluster "${cluster}" -n "${ns}" -o jsonpath='{.status.currentPrimary}'
}

sql() {
  # psql as the postgres OS user inside the primary (peer auth) — no
  # credentials or port-forwards, the pattern the verifiers use.
  kubectl exec -n "${ns}" "$(primary)" -c postgres -- psql -U postgres -d appdb -v ON_ERROR_STOP=1 -qtAc "$1"
}

echo "  [seed] source primary: $(primary)"
# The marker table belongs to the APPLICATION OWNER, not to postgres: the
# credential-continuity proof reads it as the source's app user through the
# recovered cluster, and a table owned by the superuser would refuse that
# read for the wrong reason (live-caught: "permission denied for table" on
# a credential that had authenticated perfectly).
sql "CREATE TABLE IF NOT EXISTS dr_markers (name text PRIMARY KEY, written_at timestamptz NOT NULL DEFAULT now()); ALTER TABLE dr_markers OWNER TO appuser"
sql "INSERT INTO dr_markers (name) VALUES ('marker-a') ON CONFLICT (name) DO UPDATE SET written_at = now()"
echo "  [seed] marker A written"

# WAL archiving must be healthy before a backup is meaningful.
for i in $(seq 1 60); do
  archiving="$(kubectl get cluster "${cluster}" -n "${ns}" -o jsonpath='{.status.conditions[?(@.type=="ContinuousArchiving")].status}')"
  [ "${archiving}" = "True" ] && break
  sleep 5
done
[ "${archiving}" = "True" ] || { echo "  [seed] ContinuousArchiving never became True on ${cluster}" >&2; kubectl get cluster "${cluster}" -n "${ns}" -o yaml | tail -40 >&2; exit 1; }

# The base backup: the same plugin-method Backup the kind's ScheduledBackups
# render, owned by the fixture cluster.
kubectl apply -n "${ns}" -f - <<EOF
apiVersion: postgresql.cnpg.io/v1
kind: Backup
metadata:
  name: ${backup}
spec:
  cluster:
    name: ${cluster}
  method: plugin
  pluginConfiguration:
    name: ${plugin}
EOF

for i in $(seq 1 120); do
  phase="$(kubectl get backup "${backup}" -n "${ns}" -o jsonpath='{.status.phase}' 2>/dev/null || true)"
  case "${phase}" in
    completed) break ;;
    failed) echo "  [seed] backup ${backup} FAILED: $(kubectl get backup "${backup}" -n "${ns}" -o jsonpath='{.status.error}')" >&2; exit 1 ;;
  esac
  sleep 5
done
[ "${phase}" = "completed" ] || { echo "  [seed] backup ${backup} never completed (last phase: ${phase:-none})" >&2; exit 1; }
echo "  [seed] base backup ${backup} completed (backupId $(kubectl get backup "${backup}" -n "${ns}" -o jsonpath='{.status.backupId}'), backupName $(kubectl get backup "${backup}" -n "${ns}" -o jsonpath='{.status.backupName}'))"

sql "INSERT INTO dr_markers (name) VALUES ('marker-b') ON CONFLICT (name) DO UPDATE SET written_at = now()"
# Close the WAL segment carrying marker B and give the archiver a moment:
# a recovery to the archive's end must be able to see it.
sql "SELECT pg_switch_wal()" >/dev/null
sleep 20
echo "  [seed] marker B written and its WAL segment switched for archiving"

echo "PG_SEED_BACKUP=${backup}" >> "${E2E_SETUP_OUTPUT}"
