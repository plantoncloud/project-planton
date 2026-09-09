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

// CnpgOperatorInstallVerifier checks a CloudNativePG operator installation:
// the operator Deployment Available and the core CRDs Established — the
// preconditions every KubernetesPostgres apply depends on. When the Barman
// Cloud plugin is enabled, the plugin Deployment and its ObjectStore CRD
// are part of the contract too (backup blocks are dead letters without
// them).
type CnpgOperatorInstallVerifier struct {
	Namespace string
	// InstallOperator mirrors the manifest's install_operator (default
	// true). False is the plugin-only posture: the operator on the cluster
	// is someone else's (a resident install), so its Deployment is neither
	// asserted present under the chart's name nor asserted gone on destroy
	// — only the CRDs it must have registered (the plugin needs them) are.
	InstallOperator bool
	// PluginEnabled mirrors the manifest's barman_cloud_plugin.enabled —
	// the plugin ships as a second release, so its health is asserted
	// only when the spec asked for it.
	PluginEnabled bool
}

// cnpgOperatorDeployment is the chart fullname with the module's fixed
// release name "cnpg" (`<release>-<chart>`), one per cluster.
const cnpgOperatorDeployment = "cnpg-cloudnative-pg"

// cnpgPluginDeployment is the plugin chart's fullname with the module's
// fixed release name (release == chart name, so the fullname collapses).
const cnpgPluginDeployment = "plugin-barman-cloud"

func (v *CnpgOperatorInstallVerifier) VerifyExists(ctx context.Context, kubeconfig string) error {
	fmt.Printf("  [verify] cloudnative-pg operator in namespace %q (operator=%v plugin=%v)\n", v.Namespace, v.InstallOperator, v.PluginEnabled)

	if err := KubectlResourceExists(ctx, kubeconfig, "namespace", v.Namespace, ""); err != nil {
		return errors.Wrapf(err, "namespace %q not found for cloudnative-pg", v.Namespace)
	}

	if v.InstallOperator {
		if err := kubectlWait(ctx, kubeconfig, "deployment", cnpgOperatorDeployment, v.Namespace,
			"condition=Available", 3*time.Minute); err != nil {
			return errors.Wrap(err, "cloudnative-pg operator deployment not available")
		}
	}

	// The CRDs KubernetesPostgres renders against: the Cluster itself and
	// the two backup-facing kinds. In the plugin-only posture they are the
	// resident operator's — their presence is what proves a CloudNativePG
	// is actually there for the plugin to register with.
	for _, crd := range []string{
		"clusters.postgresql.cnpg.io",
		"scheduledbackups.postgresql.cnpg.io",
		"backups.postgresql.cnpg.io",
	} {
		if err := kubectlWait(ctx, kubeconfig, "crd", crd, "",
			"condition=Established", 2*time.Minute); err != nil {
			return errors.Wrapf(err, "CRD %s not established", crd)
		}
	}

	if !v.PluginEnabled {
		return nil
	}

	if err := kubectlWait(ctx, kubeconfig, "deployment", cnpgPluginDeployment, v.Namespace,
		"condition=Available", 3*time.Minute); err != nil {
		return errors.Wrap(err, "barman-cloud plugin deployment not available (is cert-manager installed? the plugin's TLS depends on it)")
	}
	if err := kubectlWait(ctx, kubeconfig, "crd", "objectstores.barmancloud.cnpg.io", "",
		"condition=Established", 2*time.Minute); err != nil {
		return errors.Wrap(err, "ObjectStore CRD not established")
	}

	return nil
}

func (v *CnpgOperatorInstallVerifier) VerifyAbsent(ctx context.Context, kubeconfig string) error {
	// The CRDs intentionally SURVIVE uninstall (the chart stamps
	// helm.sh/resource-policy: keep on them so removing the operator never
	// cascade-deletes Cluster resources) — only the deployments' absence
	// is asserted. In the plugin-only posture the operator Deployment is
	// the resident's and must STILL be there after this resource is gone:
	// a plugin-only destroy that took the operator with it would be the
	// defect this posture exists to rule out.
	if v.InstallOperator {
		if err := KubectlResourceAbsent(ctx, kubeconfig, "deployment", cnpgOperatorDeployment, v.Namespace); err != nil {
			return err
		}
	} else {
		// The resident is found by the label every CloudNativePG install
		// carries (the Helm chart's and the Planton operator's alike), not
		// by the chart's Deployment name — a resident installed by another
		// hand is named by that hand.
		residents, err := kubectlGetJSONPathList(ctx, kubeconfig, "deployment", v.Namespace,
			"app.kubernetes.io/name=cloudnative-pg", "{range .items[*]}{.metadata.name}{\"\\n\"}{end}")
		if err != nil {
			return errors.Wrap(err, "listing the resident cloudnative-pg operator deployments")
		}
		if len(residents) == 0 {
			return errors.New("the resident cloudnative-pg operator must survive a plugin-only destroy, but no deployment labeled app.kubernetes.io/name=cloudnative-pg remains in the namespace")
		}
		fmt.Printf("  [verify] resident cloudnative-pg operator %v untouched by the plugin-only destroy\n", residents)
	}
	if v.PluginEnabled {
		return KubectlResourceAbsent(ctx, kubeconfig, "deployment", cnpgPluginDeployment, v.Namespace)
	}
	return nil
}

// CnpgClusterVerifier checks a CloudNativePG-managed PostgreSQL cluster to
// the point it is actually serving: the Cluster Ready condition, every
// declared instance ready, and the -rw Service present.
//
// When Behavioral is set (the behavioral-failover scenario), the verifier
// additionally proves DATA DURABILITY through a failover: it writes a
// marker row through the current primary, DELETES the primary pod, waits
// for the operator to promote a replica, and reads the marker back through
// the new primary. The write path is `kubectl exec` + psql as the postgres
// OS user (peer auth inside the pod) — no credentials or port-forwards to
// flake on.
type CnpgClusterVerifier struct {
	Namespace   string
	ClusterName string
	// Instances is the declared instance count (read from the scenario
	// manifest) — readiness means ALL of them, not just the primary.
	Instances int64
	// Behavioral switches on the live failover proof (see above).
	Behavioral bool
	// BackupProof waits for the immediate schedule's Backup to reach
	// Completed (the with-backup scenario) — a REAL base backup landing
	// in the object store through the Barman Cloud plugin.
	BackupProof bool
	// RecoveryProof switches on THE RECOVERY PROOF (the gke-gcs-recovery
	// scenario): this cluster was bootstrapped from another cluster's
	// archive, and must carry both seeded markers — A (in the base backup)
	// and B (written after it, so only WAL replay past the base backup can
	// have delivered it) — readable with the SOURCE's application
	// credentials (credential continuity), with its instances spread over
	// distinct nodes. RecoverySourceCluster names the source whose `-app`
	// Secret holds those credentials.
	RecoveryProof         bool
	RecoverySourceCluster string
}

// cnpgRecoveryMarkers are the rows the recovery scenario's seed script wrote
// into the source cluster's appdb, in the order it wrote them around the base
// backup: A before, B after.
var cnpgRecoveryMarkers = []string{"marker-a", "marker-b"}

func (v *CnpgClusterVerifier) VerifyExists(ctx context.Context, kubeconfig string) error {
	fmt.Printf("  [verify] cloudnative-pg cluster %q in namespace %q (instances=%d)\n",
		v.ClusterName, v.Namespace, v.Instances)

	// Ready flips once the bootstrap completed and the topology matches
	// the spec. First-boot includes an image pull plus initdb, and on a
	// real cluster each instance may also wait for the autoscaler to add
	// the node its anti-affinity demands and pull the images onto it
	// (live-measured ~4 min per instance on GKE), so the window is sized
	// for three instances on fresh nodes, not one on a warm kind node.
	if err := kubectlWait(ctx, kubeconfig, "cluster.postgresql.cnpg.io", v.ClusterName, v.Namespace,
		"condition=Ready", 15*time.Minute); err != nil {
		return errors.Wrapf(err, "cluster %q never became Ready", v.ClusterName)
	}

	if err := v.waitForReadyInstances(ctx, kubeconfig, 8*time.Minute); err != nil {
		return err
	}

	// The read-write Service is the application-facing contract.
	if err := KubectlResourceExists(ctx, kubeconfig, "service", v.ClusterName+"-rw", v.Namespace); err != nil {
		return errors.Wrap(err, "read-write service not found")
	}

	if v.BackupProof {
		if err := v.proveBackupCompleted(ctx, kubeconfig); err != nil {
			return err
		}
	}

	if v.RecoveryProof {
		if err := v.proveRecovery(ctx, kubeconfig); err != nil {
			return err
		}
	}

	if !v.Behavioral {
		return nil
	}
	return v.proveFailoverDurability(ctx, kubeconfig)
}

// proveRecovery is THE RECOVERY PROOF: (1) both seeded markers are present in
// the recovered application database — A proves the base backup restored, B
// proves WAL replay continued past it; (2) the SOURCE cluster's application
// credential authenticates against the recovered cluster's read-write
// Service — the credential-continuity contract the recovery declared by
// referencing the source's `-app` Secret; (3) the instances landed on as
// many distinct nodes as there are instances — the multi-node HA posture
// the required anti-affinity asked for, which only a real cluster can show.
func (v *CnpgClusterVerifier) proveRecovery(ctx context.Context, kubeconfig string) error {
	primary, err := v.currentPrimary(ctx, kubeconfig)
	if err != nil {
		return err
	}

	for _, marker := range cnpgRecoveryMarkers {
		out, err := v.psqlDB(ctx, kubeconfig, primary, "appdb",
			fmt.Sprintf("SELECT count(*) FROM dr_markers WHERE name = '%s'", marker))
		if err != nil {
			return errors.Wrapf(err, "reading %s from the recovered database", marker)
		}
		if strings.TrimSpace(out) != "1" {
			return errors.Errorf("RECOVERY PROOF failed: %s is missing from the recovered database (count=%q) — %s",
				marker, strings.TrimSpace(out), recoveryMarkerMeaning(marker))
		}
		fmt.Printf("  [verify] RECOVERY PROOF: %s present in the recovered database — %s\n", marker, recoveryMarkerMeaning(marker))
	}

	// Credential continuity: the source's app Secret (username/password),
	// presented over the network to the recovered cluster's -rw Service
	// from inside an instance pod.
	if v.RecoverySourceCluster != "" {
		username, err := kubectlGetJSONPath(ctx, kubeconfig, "secret", v.RecoverySourceCluster+"-app", v.Namespace, "{.data.username}")
		if err != nil {
			return errors.Wrapf(err, "reading the source cluster's app Secret %s-app", v.RecoverySourceCluster)
		}
		password, err := kubectlGetJSONPath(ctx, kubeconfig, "secret", v.RecoverySourceCluster+"-app", v.Namespace, "{.data.password}")
		if err != nil {
			return errors.Wrapf(err, "reading the source cluster's app Secret %s-app", v.RecoverySourceCluster)
		}
		user, _ := base64.StdEncoding.DecodeString(strings.TrimSpace(username))
		pass, _ := base64.StdEncoding.DecodeString(strings.TrimSpace(password))
		out, err := exec.CommandContext(ctx, "kubectl", "--kubeconfig", kubeconfig,
			"exec", primary, "-n", v.Namespace, "-c", "postgres", "--",
			"env", "PGPASSWORD="+string(pass),
			"psql", "-h", v.ClusterName+"-rw", "-U", string(user), "-d", "appdb", "-tA",
			"-c", "SELECT count(*) FROM dr_markers").CombinedOutput()
		if err != nil {
			return errors.Errorf("CREDENTIAL CONTINUITY failed: the source cluster's application credential (%s-app, user %s) was refused by the recovered cluster's -rw Service: %v: %s",
				v.RecoverySourceCluster, string(user), err, string(out))
		}
		fmt.Printf("  [verify] CREDENTIAL CONTINUITY: the source's application user %q authenticated against %s-rw and read %s marker rows\n",
			string(user), v.ClusterName, strings.TrimSpace(string(out)))
	}

	return v.proveNodeSpread(ctx, kubeconfig)
}

// proveNodeSpread asserts the instance pods occupy as many distinct nodes as
// there are instances — what a REQUIRED hostname anti-affinity promises and
// what a single-node kind cluster can never demonstrate.
func (v *CnpgClusterVerifier) proveNodeSpread(ctx context.Context, kubeconfig string) error {
	nodes, err := kubectlGetJSONPathList(ctx, kubeconfig, "pod", v.Namespace,
		"cnpg.io/cluster="+v.ClusterName+",cnpg.io/podRole=instance", "{range .items[*]}{.spec.nodeName}{\"\\n\"}{end}")
	if err != nil {
		return errors.Wrap(err, "listing the instance pods' nodes")
	}
	distinct := map[string]bool{}
	for _, n := range nodes {
		distinct[n] = true
	}
	if int64(len(distinct)) < v.Instances {
		return errors.Errorf("NODE SPREAD failed: %d instances landed on only %d distinct nodes (%v) — the required anti-affinity was not honored",
			v.Instances, len(distinct), nodes)
	}
	fmt.Printf("  [verify] NODE SPREAD: %d instances on %d distinct nodes %v\n", v.Instances, len(distinct), nodes)
	return nil
}

func recoveryMarkerMeaning(marker string) string {
	if marker == "marker-a" {
		return "the base backup restored"
	}
	return "WAL archived after the base backup was replayed (continuous archiving, not a copy)"
}

// psqlDB is psql against a named database (peer auth as the postgres OS user).
func (v *CnpgClusterVerifier) psqlDB(ctx context.Context, kubeconfig, podName, database, sql string) (string, error) {
	out, err := exec.CommandContext(ctx, "kubectl", "--kubeconfig", kubeconfig,
		"exec", podName, "-n", v.Namespace, "-c", "postgres", "--",
		"psql", "-U", "postgres", "-d", database, "-tA", "-c", sql).CombinedOutput()
	if err != nil {
		return "", errors.Errorf("psql on %s (%s): %v: %s", podName, database, err, string(out))
	}
	return string(out), nil
}

// proveBackupCompleted waits for a Backup owned by this cluster to reach
// phase Completed — the immediate ScheduledBackup fires one on creation,
// and Completed means the plugin actually wrote a base backup into the
// object store (WAL archiving is a precondition the plugin enforces).
func (v *CnpgClusterVerifier) proveBackupCompleted(ctx context.Context, kubeconfig string) error {
	fmt.Printf("  [verify] waiting for a Completed base backup of cluster %q\n", v.ClusterName)
	deadline := time.Now().Add(6 * time.Minute)
	var last string
	for time.Now().Before(deadline) {
		out, err := exec.CommandContext(ctx, "kubectl", "--kubeconfig", kubeconfig,
			"get", "backups.postgresql.cnpg.io", "-n", v.Namespace,
			"-l", "cnpg.io/cluster="+v.ClusterName,
			"-o", "jsonpath={.items[*].status.phase}").CombinedOutput()
		phases := strings.Fields(strings.TrimSpace(string(out)))
		if err == nil {
			for _, phase := range phases {
				if phase == "completed" || phase == "Completed" {
					fmt.Printf("  [verify] base backup Completed — the plugin wrote a real backup to the store\n")
					return nil
				}
				if phase == "failed" || phase == "Failed" {
					return errors.Errorf("a backup of cluster %q reached terminal phase %q", v.ClusterName, phase)
				}
			}
		}
		last = fmt.Sprintf("phases=%v err=%v", phases, err)
		time.Sleep(10 * time.Second)
	}
	return errors.Errorf("no backup of cluster %q reached Completed (last: %s)", v.ClusterName, last)
}

func (v *CnpgClusterVerifier) VerifyAbsent(ctx context.Context, kubeconfig string) error {
	return KubectlResourceAbsent(ctx, kubeconfig, "cluster.postgresql.cnpg.io", v.ClusterName, v.Namespace)
}

// proveFailoverDurability runs the write → primary-loss → promotion →
// read-back cycle. The marker table is verifier-owned in the `postgres`
// database (always present regardless of the bootstrap shape); it dies
// with the cluster, so no cleanup is needed.
func (v *CnpgClusterVerifier) proveFailoverDurability(ctx context.Context, kubeconfig string) error {
	if v.Instances < 2 {
		return errors.New("behavioral failover needs instances >= 2 — there must be a replica to promote")
	}

	oldPrimary, err := v.currentPrimary(ctx, kubeconfig)
	if err != nil {
		return err
	}
	fmt.Printf("  [verify] behavioral failover: current primary %q\n", oldPrimary)

	// 1. Write the marker through the primary. synchronous_commit rides
	//    the cluster's own settings; the row must survive whatever the
	//    spec declared.
	const markerSQL = "CREATE TABLE IF NOT EXISTS e2e_failover_proof(id int PRIMARY KEY, note text); " +
		"INSERT INTO e2e_failover_proof(id, note) VALUES (1, 'postgres-failover-round-trip') " +
		"ON CONFLICT (id) DO UPDATE SET note = EXCLUDED.note;"
	if _, err := v.psql(ctx, kubeconfig, oldPrimary, markerSQL); err != nil {
		return errors.Wrap(err, "failed to write the marker row through the primary")
	}

	// 2. The disaster: the primary pod is deleted outright. The operator
	//    detects the loss and promotes the most advanced replica.
	if out, err := exec.CommandContext(ctx, "kubectl", "--kubeconfig", kubeconfig,
		"delete", "pod", oldPrimary, "-n", v.Namespace, "--wait=false").CombinedOutput(); err != nil {
		return errors.Errorf("failed to delete primary pod: %v: %s", err, string(out))
	}

	// 3. Promotion: status.currentPrimary must move off the deleted pod.
	//    (The deleted pod's PVC makes it eligible to REJOIN as a replica
	//    later — the check is for a DIFFERENT primary, not pod absence.)
	newPrimary := ""
	deadline := time.Now().Add(4 * time.Minute)
	for time.Now().Before(deadline) {
		current, err := v.currentPrimary(ctx, kubeconfig)
		if err == nil && current != "" && current != oldPrimary {
			newPrimary = current
			break
		}
		time.Sleep(5 * time.Second)
	}
	if newPrimary == "" {
		return errors.Errorf("operator never promoted a replica off %q", oldPrimary)
	}
	fmt.Printf("  [verify] promoted: new primary %q\n", newPrimary)

	// 4. Full recovery: every instance ready again (the old primary
	//    rejoins as a replica), then the marker read back through the NEW
	//    primary.
	if err := v.waitForReadyInstances(ctx, kubeconfig, 4*time.Minute); err != nil {
		return errors.Wrap(err, "cluster never returned to full strength after the failover")
	}
	out, err := v.psql(ctx, kubeconfig, newPrimary,
		"SELECT note FROM e2e_failover_proof WHERE id = 1;")
	if err != nil {
		return errors.Wrap(err, "failed to read the marker row from the new primary")
	}
	if strings.TrimSpace(out) != "postgres-failover-round-trip" {
		return errors.Errorf("marker row wrong after failover (got %q)", strings.TrimSpace(out))
	}

	fmt.Printf("  [verify] marker row intact on the new primary — data survived the failover\n")
	return nil
}

// currentPrimary reads status.currentPrimary — the operator maintains it
// through failovers and switchovers.
func (v *CnpgClusterVerifier) currentPrimary(ctx context.Context, kubeconfig string) (string, error) {
	out, err := exec.CommandContext(ctx, "kubectl", "--kubeconfig", kubeconfig,
		"get", "cluster.postgresql.cnpg.io", v.ClusterName, "-n", v.Namespace,
		"-o", "jsonpath={.status.currentPrimary}").CombinedOutput()
	if err != nil {
		return "", errors.Errorf("failed to read currentPrimary: %v: %s", err, string(out))
	}
	return strings.TrimSpace(string(out)), nil
}

func (v *CnpgClusterVerifier) waitForReadyInstances(ctx context.Context, kubeconfig string, timeout time.Duration) error {
	want := fmt.Sprintf("%d", v.Instances)
	deadline := time.Now().Add(timeout)
	var last string
	for time.Now().Before(deadline) {
		out, err := exec.CommandContext(ctx, "kubectl", "--kubeconfig", kubeconfig,
			"get", "cluster.postgresql.cnpg.io", v.ClusterName, "-n", v.Namespace,
			"-o", "jsonpath={.status.readyInstances}").CombinedOutput()
		got := strings.TrimSpace(string(out))
		if err == nil && got == want {
			return nil
		}
		last = fmt.Sprintf("readyInstances=%q err=%v", got, err)
		time.Sleep(5 * time.Second)
	}
	return errors.Errorf("cluster %q never reached %s ready instances (last: %s)", v.ClusterName, want, last)
}

// psql runs a SQL string on an instance pod as the postgres OS user (peer
// auth inside the pod — the same path the operator's own probes use).
func (v *CnpgClusterVerifier) psql(ctx context.Context, kubeconfig, podName, sql string) (string, error) {
	out, err := exec.CommandContext(ctx, "kubectl", "--kubeconfig", kubeconfig,
		"exec", podName, "-n", v.Namespace, "-c", "postgres", "--",
		"psql", "-U", "postgres", "-d", "postgres", "-tA", "-c", sql).CombinedOutput()
	if err != nil {
		return "", errors.Errorf("psql on %s: %v: %s", podName, err, string(out))
	}
	return string(out), nil
}
