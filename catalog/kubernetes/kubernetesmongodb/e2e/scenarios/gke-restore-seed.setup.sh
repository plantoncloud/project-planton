#!/usr/bin/env bash
# SETUP for the gke-*-restore scenarios: seeds the SOURCE cluster (the lane's
# fixture-gke-*-source.yaml, already deployed and ready) with the evidence the
# restore proof reads back, and publishes where the backup landed. The script
# seeds the DATABASE, not the store: it takes the backup into whichever
# storage the source declares as main (GCS, R2, ...), read from the live
# cluster, so one script serves every store's lane.
#
#   1. marker A  -> a document written BEFORE the backup
#   2. a Backup   -> a real PBM logical backup into the main storage, waited to ready
#   3. marker B  -> a document written AFTER the backup, left to PITR's oplog
#                   archiving (the fixture archives one-minute chunks)
#
# A restored cluster carrying A proves the base backup; carrying B proves the
# archived oplog replayed past it (PITR to latest). The backup's destination
# is only known once PBM names it, so the script publishes it for the
# scenario manifest (${E2E_SETUP:MONGO_BACKUP_DESTINATION}). Everything lands
# inside the fixture-owned cluster and namespace, so DEPENDENCIES-DOWN
# removes it. Idempotent: markers upsert, the Backup is named per engine lane.
set -euo pipefail

ns="e2e-mdb-gke"
cluster="e2e-mdb-gke-src"
backup="${cluster}-seed-${E2E_RUN_ID}"

secret_key() {
  kubectl get secret "${cluster}-secrets" -n "${ns}" -o "jsonpath={.data.$1}" | base64 -d
}
user="$(secret_key MONGODB_DATABASE_ADMIN_USER)"
password="$(secret_key MONGODB_DATABASE_ADMIN_PASSWORD)"

mongosh_on() {
  # mongosh over localhost on a member pod with the operator-managed
  # databaseAdmin — the pattern the verifiers use; no port-forwards.
  local pod="$1" js="$2"
  kubectl exec -n "${ns}" "${pod}" -c mongod -- \
    mongosh --quiet "mongodb://${user}:${password}@localhost:27017/admin?directConnection=true" --eval "${js}"
}

primary() {
  # db.hello().primary is host:port; the pod is the host's first DNS label.
  mongosh_on "${cluster}-rs0-0" 'print(db.hello().primary)' | tail -1 | cut -d: -f1 | cut -d. -f1
}

# The storage to back up into: the one the source marks main, or its only
# one (the spec keeps exactly one main when several are declared).
storage="$(kubectl get psmdb "${cluster}" -n "${ns}" -o json | python3 -c '
import json, sys
storages = json.load(sys.stdin)["spec"]["backup"]["storages"]
main = [name for name, s in storages.items() if s.get("main")]
print(main[0] if main else next(iter(storages)))
')"
[ -n "${storage}" ] || { echo "  [seed] the source cluster declares no backup storage" >&2; exit 1; }
echo "  [seed] backing up into storage: ${storage}"

p="$(primary)"
echo "  [seed] source primary: ${p}"
mongosh_on "${p}" "db.getSiblingDB('e2e').dr_markers.updateOne({_id: 'marker-a'}, {\$set: {written_at: new Date()}}, {upsert: true, writeConcern: {w: 'majority'}});" >/dev/null
echo "  [seed] marker A written"

kubectl apply -n "${ns}" -f - <<EOF
apiVersion: psmdb.percona.com/v1
kind: PerconaServerMongoDBBackup
metadata:
  name: ${backup}
spec:
  clusterName: ${cluster}
  storageName: ${storage}
  type: logical
EOF

for i in $(seq 1 96); do
  state="$(kubectl get psmdb-backup "${backup}" -n "${ns}" -o jsonpath='{.status.state}' 2>/dev/null || true)"
  case "${state}" in
    ready) break ;;
    error) echo "  [seed] backup ${backup} FAILED: $(kubectl get psmdb-backup "${backup}" -n "${ns}" -o jsonpath='{.status.error}')" >&2; exit 1 ;;
  esac
  sleep 5
done
[ "${state}" = "ready" ] || { echo "  [seed] backup ${backup} never reached ready (last state: ${state:-none})" >&2; exit 1; }
destination="$(kubectl get psmdb-backup "${backup}" -n "${ns}" -o jsonpath='{.status.destination}')"
echo "  [seed] backup ${backup} ready -> ${destination}"

p="$(primary)"
mongosh_on "${p}" "db.getSiblingDB('e2e').dr_markers.updateOne({_id: 'marker-b'}, {\$set: {written_at: new Date()}}, {upsert: true, writeConcern: {w: 'majority'}});" >/dev/null
# PITR archives oplog in one-minute chunks on this fixture; give PBM two
# chunk windows so marker B's oplog is in the store before the restore asks
# for "latest".
sleep 130
echo "  [seed] marker B written and its oplog left to PITR archiving"

echo "MONGO_BACKUP_DESTINATION=${destination}" >> "${E2E_SETUP_OUTPUT}"
echo "MONGO_SEED_BACKUP=${backup}" >> "${E2E_SETUP_OUTPUT}"
