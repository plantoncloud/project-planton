package verify

import (
	"context"
	"encoding/base64"
	"fmt"
	"os/exec"
	"strings"
	"time"

	"github.com/pkg/errors"
)

// PsmdbClusterVerifier checks a Percona-operator-managed MongoDB cluster to
// the point it is actually serving: the PerconaServerMongoDB resource in
// state "ready" and the replica-set Service present.
//
// When Behavioral is set (the behavioral-failover scenario), the verifier
// additionally proves DATA DURABILITY through a failover: it writes a
// marker document with majority write concern through the current primary,
// DELETES the primary pod, waits for the replica set to elect a new
// primary, and reads the marker back through it. Writes ride `kubectl
// exec` + mongosh against localhost on the member pods, authenticated with
// the operator-managed databaseAdmin system user — no port-forwards to
// flake on.
//
// When BackupProof is set (the with-backup scenario), the verifier drives
// a verifier-owned PerconaServerMongoDBBackup resource to state "ready" —
// a REAL PBM backup landing in the declared object store. The driver
// resource is verifier-owned because its CRD is installed by the operator
// fixture; it is deleted in a defer so a failed assertion never blocks
// later lanes.
type PsmdbClusterVerifier struct {
	Namespace   string
	ClusterName string
	// ReplsetName is the first declared replica set (the scenario's
	// manifest shape — the verifier asserts against its Service and pods).
	ReplsetName string
	// Size is the declared member count of that replica set.
	Size int64
	// Behavioral switches on the live failover proof.
	Behavioral bool
	// BackupProof drives a real PBM backup to completion. BackupStorage
	// names the declared storage the driver Backup writes to.
	BackupProof   bool
	BackupStorage string
	// UsersSecretName is the system-users Secret the cluster runs with:
	// `<cluster>-secrets` when the operator generates it, or the Secret
	// the manifest brought (system_users_secret_name — the restore
	// target's credential-continuity seam, pointing at the SOURCE's).
	UsersSecretName string
	// RestoreProof switches on THE RESTORE PROOF (the gke-gcs-restore
	// scenario): this cluster declared a restore from another cluster's
	// backup, and must show the Restore run reached ready, both seeded
	// markers readable with the brought (source) credentials — A from the
	// base backup, B only from the archived oplog replayed past it — and
	// its members spread over distinct nodes.
	RestoreProof bool
}

// psmdbRestoreMarkers are the documents the restore scenario's seed script
// wrote into the source cluster around the backup: A before, B after.
var psmdbRestoreMarkers = []string{"marker-a", "marker-b"}

func (v *PsmdbClusterVerifier) VerifyExists(ctx context.Context, kubeconfig string) error {
	fmt.Printf("  [verify] percona mongodb cluster %q in namespace %q (rs=%s size=%d)\n",
		v.ClusterName, v.Namespace, v.ReplsetName, v.Size)

	// state "ready" flips once every declared component reached its
	// desired size. First boot pulls images and initializes the replica
	// set, so the window is generous.
	if err := kubectlWait(ctx, kubeconfig, "psmdb", v.ClusterName, v.Namespace,
		"jsonpath={.status.state}=ready", 10*time.Minute); err != nil {
		return errors.Wrapf(err, "PerconaServerMongoDB %q never reached state ready", v.ClusterName)
	}

	// The replica-set Service is the application-facing discovery contract.
	if err := KubectlResourceExists(ctx, kubeconfig, "service",
		v.ClusterName+"-"+v.ReplsetName, v.Namespace); err != nil {
		return errors.Wrap(err, "replica-set service not found")
	}

	if v.BackupProof {
		if err := v.proveBackupCompleted(ctx, kubeconfig); err != nil {
			return err
		}
	}
	if v.RestoreProof {
		if err := v.proveRestore(ctx, kubeconfig); err != nil {
			return err
		}
	}
	if !v.Behavioral {
		return nil
	}
	return v.proveFailoverDurability(ctx, kubeconfig)
}

// proveRestore is THE RESTORE PROOF: (1) the Restore object the module
// rendered for this cluster reaches state ready (the operator replayed the
// backup and, with PITR, the archived oplog into the running set); (2) both
// seeded markers are readable — A proves the base backup restored, B proves
// the oplog replayed past it — authenticated with the brought system-users
// Secret, which IS the credential-continuity contract (a restored database
// carries the source's users, so the source's passwords must log in); (3)
// the members landed on distinct nodes — the multi-node HA posture the
// operator's hostname anti-affinity promises, which kind never shows.
func (v *PsmdbClusterVerifier) proveRestore(ctx context.Context, kubeconfig string) error {
	// The Restore object's name hashes its declaration; find it by the
	// cluster it targets rather than guessing the hash.
	deadline := time.Now().Add(15 * time.Minute)
	var restoreName, last string
	for time.Now().Before(deadline) {
		out, err := exec.CommandContext(ctx, "kubectl", "--kubeconfig", kubeconfig,
			"get", "psmdb-restore", "-n", v.Namespace,
			"-o", `jsonpath={range .items[?(@.spec.clusterName=="`+v.ClusterName+`")]}{.metadata.name}={.status.state}={.status.error}{"\n"}{end}`).CombinedOutput()
		if err == nil {
			for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
				parts := strings.SplitN(strings.TrimSpace(line), "=", 3)
				if len(parts) < 2 || parts[0] == "" {
					continue
				}
				restoreName = parts[0]
				switch parts[1] {
				case "ready":
					fmt.Printf("  [verify] RESTORE PROOF: restore %q reached ready — the operator replayed the backup into %s\n", restoreName, v.ClusterName)
					goto restored
				case "error":
					return errors.Errorf("RESTORE PROOF failed: restore %q errored: %s", restoreName, strings.Join(parts[2:], "="))
				}
				last = line
			}
		} else {
			last = fmt.Sprintf("err=%v %s", err, string(out))
		}
		time.Sleep(10 * time.Second)
	}
	return errors.Errorf("no restore targeting cluster %q reached ready (last: %s)", v.ClusterName, last)

restored:
	// The restore replaced the data; the operator reconciles the set back
	// to ready afterwards (for a logical restore the members stay up, for a
	// physical one they are recreated).
	if err := kubectlWait(ctx, kubeconfig, "psmdb", v.ClusterName, v.Namespace,
		"jsonpath={.status.state}=ready", 8*time.Minute); err != nil {
		return errors.Wrap(err, "cluster never returned to ready after the restore")
	}

	user, password, err := v.adminCredentials(ctx, kubeconfig)
	if err != nil {
		return err
	}
	pod0 := fmt.Sprintf("%s-%s-0", v.ClusterName, v.ReplsetName)
	primary, err := v.currentPrimaryPod(ctx, kubeconfig, pod0, user, password)
	if err != nil {
		return errors.Wrapf(err, "CREDENTIAL CONTINUITY failed: the brought system-users Secret %q could not authenticate against the restored set", v.UsersSecretName)
	}
	fmt.Printf("  [verify] CREDENTIAL CONTINUITY: the source's databaseAdmin (from %s) authenticated against the restored set, primary %q\n", v.UsersSecretName, primary)

	for _, marker := range psmdbRestoreMarkers {
		out, err := v.mongosh(ctx, kubeconfig, primary, user, password,
			fmt.Sprintf("print(db.getSiblingDB('e2e').dr_markers.countDocuments({_id: '%s'}));", marker))
		if err != nil {
			return errors.Wrapf(err, "reading %s from the restored set", marker)
		}
		lines := strings.Fields(strings.TrimSpace(out))
		if len(lines) == 0 || lines[len(lines)-1] != "1" {
			return errors.Errorf("RESTORE PROOF failed: %s is missing from the restored set (got %q) — %s",
				marker, strings.TrimSpace(out), restoreMarkerMeaning(marker))
		}
		fmt.Printf("  [verify] RESTORE PROOF: %s present in the restored set — %s\n", marker, restoreMarkerMeaning(marker))
	}

	return v.proveNodeSpread(ctx, kubeconfig)
}

// proveNodeSpread asserts the replica set's members occupy as many distinct
// nodes as there are members — what the operator's default hostname
// anti-affinity promises, and what a single-node kind cluster never shows.
func (v *PsmdbClusterVerifier) proveNodeSpread(ctx context.Context, kubeconfig string) error {
	nodes, err := kubectlGetJSONPathList(ctx, kubeconfig, "pod", v.Namespace,
		"app.kubernetes.io/instance="+v.ClusterName+",app.kubernetes.io/replset="+v.ReplsetName+",app.kubernetes.io/component=mongod",
		"{range .items[*]}{.spec.nodeName}{\"\\n\"}{end}")
	if err != nil {
		return errors.Wrap(err, "listing the member pods' nodes")
	}
	distinct := map[string]bool{}
	for _, n := range nodes {
		distinct[n] = true
	}
	if int64(len(distinct)) < v.Size {
		return errors.Errorf("NODE SPREAD failed: %d members landed on only %d distinct nodes (%v) — the hostname anti-affinity was not honored",
			v.Size, len(distinct), nodes)
	}
	fmt.Printf("  [verify] NODE SPREAD: %d members on %d distinct nodes %v\n", v.Size, len(distinct), nodes)
	return nil
}

func restoreMarkerMeaning(marker string) string {
	if marker == "marker-a" {
		return "the base backup restored"
	}
	return "oplog archived after the backup was replayed (PITR to latest, not a copy)"
}

func (v *PsmdbClusterVerifier) VerifyAbsent(ctx context.Context, kubeconfig string) error {
	return KubectlResourceAbsent(ctx, kubeconfig, "psmdb", v.ClusterName, v.Namespace)
}

// proveFailoverDurability runs the write → primary-loss → election →
// read-back cycle. The marker collection is verifier-owned in a scratch
// database; it dies with the cluster, so no cleanup is needed.
func (v *PsmdbClusterVerifier) proveFailoverDurability(ctx context.Context, kubeconfig string) error {
	if v.Size < 3 {
		return errors.New("behavioral failover needs a replica set of 3 — a majority must survive the primary loss")
	}

	user, password, err := v.adminCredentials(ctx, kubeconfig)
	if err != nil {
		return err
	}

	pod0 := fmt.Sprintf("%s-%s-0", v.ClusterName, v.ReplsetName)
	oldPrimary, err := v.currentPrimaryPod(ctx, kubeconfig, pod0, user, password)
	if err != nil {
		return err
	}
	fmt.Printf("  [verify] behavioral failover: current primary %q\n", oldPrimary)

	// 1. Write the marker THROUGH THE PRIMARY with majority write concern
	//    — the write is guaranteed on a majority of members before the
	//    primary is killed.
	writeJS := `db.getSiblingDB('e2e').proof.updateOne({_id: 1}, {$set: {note: 'mongodb-failover-round-trip'}}, {upsert: true, writeConcern: {w: 'majority'}});`
	if _, err := v.mongosh(ctx, kubeconfig, oldPrimary, user, password, writeJS); err != nil {
		return errors.Wrap(err, "failed to write the marker document through the primary")
	}

	// 2. The disaster: the primary pod is deleted outright. The replica
	//    set holds an election among the survivors.
	if out, err := exec.CommandContext(ctx, "kubectl", "--kubeconfig", kubeconfig,
		"delete", "pod", oldPrimary, "-n", v.Namespace, "--wait=false").CombinedOutput(); err != nil {
		return errors.Errorf("failed to delete primary pod: %v: %s", err, string(out))
	}

	// 3. Election: a DIFFERENT member must report as primary. The probe
	//    runs on a surviving member (never the deleted pod).
	probe := pod0
	if oldPrimary == pod0 {
		probe = fmt.Sprintf("%s-%s-1", v.ClusterName, v.ReplsetName)
	}
	newPrimary := ""
	deadline := time.Now().Add(4 * time.Minute)
	for time.Now().Before(deadline) {
		current, err := v.currentPrimaryPod(ctx, kubeconfig, probe, user, password)
		if err == nil && current != "" && current != oldPrimary {
			newPrimary = current
			break
		}
		time.Sleep(5 * time.Second)
	}
	if newPrimary == "" {
		return errors.Errorf("the replica set never elected a new primary off %q", oldPrimary)
	}
	fmt.Printf("  [verify] elected: new primary %q\n", newPrimary)

	// 4. Full recovery (the old primary's pod is recreated by the
	//    StatefulSet and rejoins), then the marker read back through the
	//    NEW primary.
	if err := kubectlWait(ctx, kubeconfig, "psmdb", v.ClusterName, v.Namespace,
		"jsonpath={.status.state}=ready", 6*time.Minute); err != nil {
		return errors.Wrap(err, "cluster never returned to ready after the failover")
	}
	readJS := `print(db.getSiblingDB('e2e').proof.findOne({_id: 1}).note);`
	out, err := v.mongosh(ctx, kubeconfig, newPrimary, user, password, readJS)
	if err != nil {
		return errors.Wrap(err, "failed to read the marker document from the new primary")
	}
	if !strings.Contains(out, "mongodb-failover-round-trip") {
		return errors.Errorf("marker document wrong after failover (got %q)", strings.TrimSpace(out))
	}
	fmt.Printf("  [verify] marker document intact on the new primary — data survived the failover\n")
	return nil
}

// proveBackupCompleted applies a verifier-owned PerconaServerMongoDBBackup
// and waits for PBM to drive it to state "ready" — a real backup written
// into the declared store.
func (v *PsmdbClusterVerifier) proveBackupCompleted(ctx context.Context, kubeconfig string) error {
	backupName := fmt.Sprintf("e2e-proof-%d", time.Now().Unix())
	manifest := fmt.Sprintf(`apiVersion: psmdb.percona.com/v1
kind: PerconaServerMongoDBBackup
metadata:
  name: %s
  namespace: %s
spec:
  clusterName: %s
  storageName: %s
`, backupName, v.Namespace, v.ClusterName, v.BackupStorage)

	fmt.Printf("  [verify] driving a real PBM backup %q to storage %q\n", backupName, v.BackupStorage)
	apply := exec.CommandContext(ctx, "kubectl", "--kubeconfig", kubeconfig, "apply", "-f", "-")
	apply.Stdin = strings.NewReader(manifest)
	if out, err := apply.CombinedOutput(); err != nil {
		return errors.Errorf("failed to apply the driver backup: %v: %s", err, string(out))
	}
	defer func() {
		_ = exec.Command("kubectl", "--kubeconfig", kubeconfig,
			"delete", "psmdb-backup", backupName, "-n", v.Namespace, "--ignore-not-found").Run()
	}()

	// PBM needs its agents to settle after cluster readiness before the
	// first backup can start, then streams the dump — generous window.
	deadline := time.Now().Add(8 * time.Minute)
	var last string
	for time.Now().Before(deadline) {
		out, err := exec.CommandContext(ctx, "kubectl", "--kubeconfig", kubeconfig,
			"get", "psmdb-backup", backupName, "-n", v.Namespace,
			"-o", "jsonpath={.status.state}").CombinedOutput()
		state := strings.TrimSpace(string(out))
		if err == nil {
			switch state {
			case "ready":
				fmt.Printf("  [verify] backup %q ready — PBM wrote a real backup to the store\n", backupName)
				return nil
			case "error":
				desc, _ := exec.CommandContext(ctx, "kubectl", "--kubeconfig", kubeconfig,
					"get", "psmdb-backup", backupName, "-n", v.Namespace,
					"-o", "jsonpath={.status.error}").CombinedOutput()
				return errors.Errorf("backup %q failed: %s", backupName, string(desc))
			}
		}
		last = fmt.Sprintf("state=%q err=%v", state, err)
		time.Sleep(10 * time.Second)
	}
	return errors.Errorf("backup %q never reached ready (last: %s)", backupName, last)
}

// adminCredentials reads the cluster's system-users Secret (the operator-
// managed `<cluster>-secrets`, or the one the manifest brought) for the
// databaseAdmin account — readWrite on every database, the right privilege
// level for the marker write.
func (v *PsmdbClusterVerifier) adminCredentials(ctx context.Context, kubeconfig string) (string, string, error) {
	secretName := v.UsersSecretName
	if secretName == "" {
		secretName = v.ClusterName + "-secrets"
	}
	read := func(key string) (string, error) {
		out, err := exec.CommandContext(ctx, "kubectl", "--kubeconfig", kubeconfig,
			"get", "secret", secretName, "-n", v.Namespace,
			"-o", fmt.Sprintf("jsonpath={.data.%s}", key)).CombinedOutput()
		if err != nil {
			return "", errors.Errorf("failed to read system-users secret key %s: %v: %s", key, err, string(out))
		}
		decoded, err := base64.StdEncoding.DecodeString(strings.TrimSpace(string(out)))
		if err != nil {
			return "", errors.Wrapf(err, "failed to decode secret key %s", key)
		}
		return string(decoded), nil
	}
	user, err := read("MONGODB_DATABASE_ADMIN_USER")
	if err != nil {
		return "", "", err
	}
	password, err := read("MONGODB_DATABASE_ADMIN_PASSWORD")
	if err != nil {
		return "", "", err
	}
	return user, password, nil
}

// currentPrimaryPod asks a member (over localhost) who the primary is and
// maps the returned host:port back to a pod name (the first DNS label of
// the member's FQDN).
func (v *PsmdbClusterVerifier) currentPrimaryPod(ctx context.Context, kubeconfig, viaPod, user, password string) (string, error) {
	out, err := v.mongosh(ctx, kubeconfig, viaPod, user, password, "print(db.hello().primary);")
	if err != nil {
		return "", err
	}
	// The last non-empty line is the primary's "host:port"; the pod name
	// is the first label of the host.
	lines := strings.Fields(strings.TrimSpace(out))
	if len(lines) == 0 {
		return "", errors.New("db.hello() returned no primary")
	}
	host := strings.Split(lines[len(lines)-1], ":")[0]
	return strings.Split(host, ".")[0], nil
}

// specKey reads a spec map key tolerating both the snake_case scenario
// convention and protojson camelCase.
func specKey(spec map[string]interface{}, snake, camel string) (interface{}, bool) {
	if v, ok := spec[snake]; ok {
		return v, true
	}
	v, ok := spec[camel]
	return v, ok
}

// mongodbFirstReplset reads the first declared replica set's name and size
// from a scenario manifest's spec map (verifier-construction input).
func mongodbFirstReplset(spec map[string]interface{}) (string, int64) {
	name, size := "rs0", int64(3)
	if spec == nil {
		return name, size
	}
	raw, ok := specKey(spec, "replica_sets", "replicaSets")
	if !ok {
		return name, size
	}
	list, ok := raw.([]interface{})
	if !ok || len(list) == 0 {
		return name, size
	}
	first, ok := list[0].(map[string]interface{})
	if !ok {
		return name, size
	}
	if n, ok := first["name"].(string); ok && n != "" {
		name = n
	}
	switch v := first["size"].(type) {
	case int:
		size = int64(v)
	case int64:
		size = v
	case float64:
		size = int64(v)
	}
	return name, size
}

// mongodbFirstBackupStorage reads the first declared backup storage's name
// — the storage the with-backup scenario's driver Backup writes to.
func mongodbFirstBackupStorage(spec map[string]interface{}) string {
	if spec == nil {
		return ""
	}
	backup, ok := spec["backup"].(map[string]interface{})
	if !ok {
		return ""
	}
	raw, ok := backup["storages"]
	if !ok {
		return ""
	}
	list, ok := raw.([]interface{})
	if !ok || len(list) == 0 {
		return ""
	}
	first, ok := list[0].(map[string]interface{})
	if !ok {
		return ""
	}
	name, _ := first["name"].(string)
	return name
}

// mongodbUsersSecretName reads a brought system-users Secret name from the
// scenario manifest's spec map ("" = the operator-generated default).
func mongodbUsersSecretName(spec map[string]interface{}) string {
	if spec == nil {
		return ""
	}
	raw, ok := specKey(spec, "system_users_secret_name", "systemUsersSecretName")
	if !ok {
		return ""
	}
	name, _ := raw.(string)
	return name
}

// mongosh runs a JS snippet on a member pod against localhost with the
// operator-managed admin credentials.
func (v *PsmdbClusterVerifier) mongosh(ctx context.Context, kubeconfig, podName, user, password, js string) (string, error) {
	uri := fmt.Sprintf("mongodb://%s:%s@localhost:27017/admin?directConnection=true", user, password)
	out, err := exec.CommandContext(ctx, "kubectl", "--kubeconfig", kubeconfig,
		"exec", podName, "-n", v.Namespace, "-c", "mongod", "--",
		"mongosh", "--quiet", uri, "--eval", js).CombinedOutput()
	if err != nil {
		return "", errors.Errorf("mongosh on %s: %v: %s", podName, err, string(out))
	}
	return string(out), nil
}
