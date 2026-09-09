// Package aa_e2e implements the E2E provider harness for Kubernetes.
//
// Two cluster lanes are supported, selected by environment:
//
//   - kind (Kubernetes IN Docker), the default: the harness owns the cluster
//     lifecycle. The cluster PERSISTS across test runs -- Setup reuses a running
//     cluster with the configured name and only creates one when none exists,
//     because cluster create/destroy dominates run time and local runs execute
//     back-to-back. Set PLANTON_E2E_DESTROY_CLUSTER=1 (ephemeral CI runners) to
//     delete it at teardown.
//
//   - external cluster: PLANTON_E2E_KUBECONFIG points at a kubeconfig for a
//     cluster provisioned and owned OUTSIDE the run (batched real EKS/GKE/AKS
//     lanes -- one cluster per wave batch, reused across component lanes). The
//     harness never creates or deletes anything cluster-level in this lane.
//
// Component-level verify semantics (deployed / destroyed) are identical in both
// lanes: verification keys off the kubeconfig path, not the cluster's origin.
package aa_e2e

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/pkg/errors"
	"github.com/plantonhq/planton/catalog/kubernetes/aa_e2e/verify"
	"github.com/plantonhq/planton/e2e/framework/provider"
)

// Environment contract for lane selection.
const (
	// ExternalKubeconfigEnvVar selects the external-cluster lane: a path to the
	// kubeconfig of a cluster the harness does not own.
	ExternalKubeconfigEnvVar = "PLANTON_E2E_KUBECONFIG"

	// ExternalClusterProfileEnvVar declares WHAT the external cluster is (a
	// cluster-profile name, e.g. "aws-eks"), so profile-annotated scenarios can
	// be matched to it. Scenarios with no profile annotation always run in the
	// external lane; a profiled scenario runs only when this declaration equals
	// its profile and skips (with the reason) otherwise. Without this gate a
	// batch sweep would run every profiled scenario on the external cluster —
	// including profiles that would destroy it for later lanes (installing a
	// primary CNI on a live EKS cluster, for example).
	ExternalClusterProfileEnvVar = "PLANTON_E2E_CLUSTER_PROFILE"

	// DestroyClusterEnvVar opts INTO deleting the kind cluster at teardown ("1").
	// The default keeps it running for the next run (persistent local lane).
	DestroyClusterEnvVar = "PLANTON_E2E_DESTROY_CLUSTER"
)

// Cluster-profile contract: a scenario manifest may opt OUT of the default
// shared cluster by naming a profile in its metadata annotations. The test
// entrypoints route the scenario to a dedicated persistent cluster built for
// that profile (constructed lazily — runs that touch no profiled scenario
// never pay for the extra cluster). In the external-cluster lane
// (PLANTON_E2E_KUBECONFIG) profiles are ignored: the batched real cluster is
// assumed suitable for every scenario in its batch.
const (
	// ClusterProfileAnnotation names the cluster profile a scenario needs.
	// Absent = the default shared kind cluster.
	ClusterProfileAnnotation = "planton.dev/e2e-cluster-profile"

	// ClusterProfileCiliumCni is the CNI-less cluster profile: kind is created
	// with the default CNI disabled so Cilium can install as the PRIMARY CNI —
	// the posture a real Cilium cluster runs, impossible to reproduce on the
	// default cluster whose kindnet already owns pod networking. Scenarios on
	// this profile: the Cilium kind's own lanes and every behavioral scenario
	// that needs an ENFORCING CNI (NetworkPolicy deny proofs).
	ClusterProfileCiliumCni = "cilium-cni"

	// ClusterProfileAwsEks is a REAL-CLUSTER profile: it names cloud fabric no
	// local kind cluster can provide (EKS with IRSA-capable OIDC, cloud load
	// balancers, the EBS CSI driver with volume snapshots, real node groups).
	// It has no local constructor — on local runs a scenario carrying it skips
	// with the reason, and it runs when the external-cluster lane declares a
	// matching profile (PLANTON_E2E_KUBECONFIG + PLANTON_E2E_CLUSTER_PROFILE).
	// Use it on scenarios of otherwise-locally-green kinds whose assertions
	// need the real fabric (cloud-LB provisioning, IRSA identity hops,
	// snapshot-backed storage, node autoscaling).
	ClusterProfileAwsEks = "aws-eks"

	// ClusterProfileGcpGke is a REAL-CLUSTER profile: GKE with Workload
	// Identity (the keyless GCS path for the Barman Cloud plugin), the pd CSI
	// driver, multi-zone node pools with an autoscaler that adds nodes when a
	// database's anti-affinity asks for them — the substrate the multi-node
	// HA and object-store backup/restore proofs need. No local constructor:
	// scenarios on it skip locally with the reason and run when the external
	// lane declares the profile. Batch runbook: realcluster/gcp-gke/.
	ClusterProfileGcpGke = "gcp-gke"
)

// RealClusterProfiles lists the profiles no harness-owned cluster can
// satisfy — the ones a scenario declaring resident prerequisites must pin
// itself to (nothing is resident on a cluster the harness creates).
var RealClusterProfiles = []string{ClusterProfileAwsEks, ClusterProfileGcpGke}

// ciliumKindConfigYAML is the kind cluster configuration for the cilium-cni
// profile. disableDefaultCNI is the load-bearing line (upstream's own kind
// guidance): without it kindnet owns /etc/cni/net.d and Cilium could only
// chain. kube-proxy is deliberately KEPT (default iptables mode) so scenarios
// that do not enable kube-proxy replacement still have Service routing —
// kube-proxy-free is a per-scenario Cilium setting, not a cluster property
// the shared profile should force on every lane.
const ciliumKindConfigYAML = `kind: Cluster
apiVersion: kind.x-k8s.io/v1alpha4
nodes:
  - role: control-plane
networking:
  disableDefaultCNI: true
`

// NewCiliumCniHarness builds the harness for the cilium-cni profile: a
// dedicated persistent kind cluster (default name "<base>-cilium") created
// WITHOUT a CNI. Node readiness is deliberately NOT awaited at create time —
// a CNI-less node is NotReady BY DESIGN until Cilium installs, so the create
// path waits on API-server readiness instead (kind's --wait would time out
// on the node gate). The NotReady→Ready transition then happens inside the
// Cilium install itself and is asserted by the install verifier.
func NewCiliumCniHarness(baseClusterName string) *Harness {
	return &Harness{
		clusterName:    baseClusterName + "-cilium",
		kindConfigYAML: ciliumKindConfigYAML,
		skipNodeWait:   true,
	}
}

// Harness manages the test cluster for Kubernetes E2E tests.
type Harness struct {
	clusterName    string
	kubeconfigPath string
	tempDir        string

	// external marks the external-cluster lane: the cluster is provisioned and
	// owned outside the run; Setup/Teardown must not touch its lifecycle.
	external bool

	// kindConfigYAML, when non-empty, is written to a temp file and passed to
	// `kind create cluster --config` (cluster profiles: CNI-less clusters and
	// other topologies the default cluster cannot express).
	kindConfigYAML string

	// skipNodeWait drops kind's node-readiness wait at create time and polls
	// API-server readiness instead. Required for CNI-less profiles, whose
	// nodes stay NotReady until a CNI installs.
	skipNodeWait bool
}

// NewHarness creates a Kubernetes test harness with the given kind cluster name.
// The name is ignored when the external-cluster lane is selected via environment.
func NewHarness(clusterName string) *Harness {
	return &Harness{
		clusterName: clusterName,
	}
}

// Setup makes a cluster available and stores its kubeconfig path: adopting the
// external cluster when the lane is selected, reusing a running kind cluster with
// the configured name, or creating one when none exists.
func (h *Harness) Setup(ctx context.Context) error {
	if externalKubeconfig := os.Getenv(ExternalKubeconfigEnvVar); externalKubeconfig != "" {
		if _, err := os.Stat(externalKubeconfig); err != nil {
			return errors.Wrapf(err, "%s points at an unreadable kubeconfig", ExternalKubeconfigEnvVar)
		}
		h.external = true
		h.kubeconfigPath = externalKubeconfig
		fmt.Printf("  [cluster] Using external cluster via %s (%s)\n", ExternalKubeconfigEnvVar, externalKubeconfig)

		os.Setenv("KUBECONFIG", h.kubeconfigPath)
		return nil
	}

	tmpDir, err := os.MkdirTemp("", "planton-e2e-*")
	if err != nil {
		return errors.Wrap(err, "failed to create temp directory for kubeconfig")
	}
	h.tempDir = tmpDir
	h.kubeconfigPath = filepath.Join(tmpDir, "kubeconfig")

	running, err := kindClusterExists(ctx, h.clusterName)
	if err != nil {
		return err
	}

	if running {
		// Reuse: export a fresh kubeconfig for the already-running cluster.
		fmt.Printf("  [kind] Reusing running cluster %q\n", h.clusterName)
		cmd := exec.CommandContext(ctx, "kind", "export", "kubeconfig",
			"--name", h.clusterName, "--kubeconfig", h.kubeconfigPath)
		var stderr bytes.Buffer
		cmd.Stderr = &stderr
		if err := cmd.Run(); err != nil {
			return errors.Wrapf(err, "kind export kubeconfig failed: %s", stderr.String())
		}
	} else {
		args := []string{
			"create", "cluster",
			"--name", h.clusterName,
			"--kubeconfig", h.kubeconfigPath,
		}
		if h.skipNodeWait {
			// CNI-less profile: nodes CANNOT become Ready before a CNI
			// installs, so kind's node-readiness wait would always time out.
			// API-server readiness is polled below instead.
		} else {
			args = append(args, "--wait", "120s")
		}
		if h.kindConfigYAML != "" {
			configPath := filepath.Join(tmpDir, "kind-config.yaml")
			if err := os.WriteFile(configPath, []byte(h.kindConfigYAML), 0o600); err != nil {
				return errors.Wrap(err, "failed to write kind cluster config")
			}
			args = append(args, "--config", configPath)
		}

		cmd := exec.CommandContext(ctx, "kind", args...)
		var stderr bytes.Buffer
		cmd.Stderr = &stderr
		cmd.Stdout = os.Stdout

		fmt.Printf("  [kind] Creating cluster %q...\n", h.clusterName)
		start := time.Now()

		if err := cmd.Run(); err != nil {
			return errors.Wrapf(err, "kind create cluster failed: %s", stderr.String())
		}

		if h.skipNodeWait {
			if err := h.waitForApiServer(ctx, 2*time.Minute); err != nil {
				return err
			}
		}

		fmt.Printf("  [kind] Cluster %q ready in %s\n", h.clusterName, time.Since(start).Round(time.Second))
	}

	// Set KUBECONFIG globally so Pulumi's kubernetes provider picks it up
	os.Setenv("KUBECONFIG", h.kubeconfigPath)

	return nil
}

// waitForApiServer polls the cluster's /readyz endpoint until the API server
// accepts requests. This is the readiness gate for CNI-less profiles, where
// node readiness (kind's own --wait gate) is unreachable by design until the
// CNI component under test installs.
func (h *Harness) waitForApiServer(ctx context.Context, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	var lastErr error
	for time.Now().Before(deadline) {
		cmd := exec.CommandContext(ctx, "kubectl", "--kubeconfig", h.kubeconfigPath, "get", "--raw", "/readyz")
		var stderr bytes.Buffer
		cmd.Stderr = &stderr
		if err := cmd.Run(); err == nil {
			return nil
		} else {
			lastErr = errors.Wrap(err, strings.TrimSpace(stderr.String()))
		}
		time.Sleep(3 * time.Second)
	}
	return errors.Wrapf(lastErr, "API server for cluster %q not ready within %s", h.clusterName, timeout)
}

// ActivateKubeconfig points the process-global KUBECONFIG at this harness's
// cluster. The engines read cluster credentials through the environment
// (Pulumi's kubernetes provider directly; the Terraform lane forwards it to
// KUBE_CONFIG_PATH per scenario), and scenarios within one test process run
// strictly serially — the entrypoints call this before each scenario so
// multi-cluster runs cannot leak a scenario onto the wrong cluster.
func (h *Harness) ActivateKubeconfig() {
	os.Setenv("KUBECONFIG", h.kubeconfigPath)
}

// External reports whether the harness adopted an externally-owned cluster
// (PLANTON_E2E_KUBECONFIG lane). Cluster profiles do not apply in that lane.
func (h *Harness) External() bool {
	return h.external
}

// Teardown releases the cluster per lane: external clusters are never touched, and
// the kind cluster persists for the next run unless deletion is opted into.
func (h *Harness) Teardown(ctx context.Context) error {
	defer func() {
		if h.tempDir != "" {
			os.RemoveAll(h.tempDir)
		}
		os.Unsetenv("KUBECONFIG")
	}()

	if h.external {
		fmt.Println("  [cluster] External cluster left untouched (owned outside the run)")
		return nil
	}

	if os.Getenv(DestroyClusterEnvVar) != "1" {
		fmt.Printf("  [kind] Leaving cluster %q running for the next run (set %s=1 to delete)\n",
			h.clusterName, DestroyClusterEnvVar)
		return nil
	}

	fmt.Printf("  [kind] Deleting cluster %q...\n", h.clusterName)

	args := []string{"delete", "cluster", "--name", h.clusterName}
	cmd := exec.CommandContext(ctx, "kind", args...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return errors.Wrapf(err, "kind delete cluster failed: %s", stderr.String())
	}

	return nil
}

// kindClusterExists reports whether a kind cluster with the given name is running.
func kindClusterExists(ctx context.Context, name string) (bool, error) {
	cmd := exec.CommandContext(ctx, "kind", "get", "clusters")
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return false, errors.Wrapf(err, "kind get clusters failed: %s", stderr.String())
	}
	for _, line := range strings.Split(strings.TrimSpace(stdout.String()), "\n") {
		if strings.TrimSpace(line) == name {
			return true, nil
		}
	}
	return false, nil
}

// KubeconfigPath returns the path to the kubeconfig for the active cluster.
func (h *Harness) KubeconfigPath() string {
	return h.kubeconfigPath
}

// ClusterName returns the kind cluster name.
func (h *Harness) ClusterName() string {
	return h.clusterName
}

// VerifyDeployed delegates to resource-type-specific verification based on the manifest.
func (h *Harness) VerifyDeployed(ctx context.Context, component string, outputs map[string]interface{}) error {
	manifestPath, _ := ctx.Value(provider.ManifestPathKey{}).(string)
	if manifestPath == "" {
		return errors.New("manifest path not found in context -- cannot verify deployment")
	}
	verifier, err := verify.GetVerifierFromManifest(manifestPath)
	if err != nil {
		return err
	}
	return verifier.VerifyExists(ctx, h.kubeconfigPath)
}

// VerifyRuntimeFailureCause implements the framework's optional
// RuntimeCauseVerifier capability (expected-runtime-failure lanes): it
// dispatches to the kind's verifier, which must itself opt in via the local
// verify.RuntimeCauseVerifier interface.
func (h *Harness) VerifyRuntimeFailureCause(ctx context.Context, tc *provider.ComponentTestContext, cause string) error {
	manifestPath, _ := ctx.Value(provider.ManifestPathKey{}).(string)
	if manifestPath == "" {
		manifestPath = tc.ManifestPath
	}
	verifier, err := verify.GetVerifierFromManifest(manifestPath)
	if err != nil {
		return err
	}
	rcv, ok := verifier.(verify.RuntimeCauseVerifier)
	if !ok {
		return errors.Errorf("component %q's verifier does not implement verify.RuntimeCauseVerifier -- the scenario expects a runtime failure cause (%s) it cannot pin", tc.Component, cause)
	}
	return rcv.VerifyRuntimeFailureCause(ctx, h.kubeconfigPath, cause)
}

// VerifyExpectedDeployFailure implements the framework's optional
// DeployFailureVerifier capability (expect-deploy-failure and
// expect-upgrade-failure lanes): it dispatches to the kind's verifier, which
// must itself opt in via the local verify.DeployFailureVerifier interface.
// The manifest on ctx is the one whose deploy failed (the upgrade manifest
// for an upgrade lane), so the verifier reads the refused values from it.
func (h *Harness) VerifyExpectedDeployFailure(ctx context.Context, tc *provider.ComponentTestContext, expectation string, deployErr error) error {
	manifestPath, _ := ctx.Value(provider.ManifestPathKey{}).(string)
	if manifestPath == "" {
		manifestPath = tc.ManifestPath
	}
	verifier, err := verify.GetVerifierFromManifest(manifestPath)
	if err != nil {
		return err
	}
	dfv, ok := verifier.(verify.DeployFailureVerifier)
	if !ok {
		return errors.Errorf("component %q's verifier does not implement verify.DeployFailureVerifier -- the scenario expects a deploy failure (%s) it cannot pin", tc.Component, expectation)
	}
	return dfv.VerifyExpectedDeployFailure(ctx, h.kubeconfigPath, expectation, deployErr)
}

// VerifyDestroyed confirms that resources have been removed after destroy.
func (h *Harness) VerifyDestroyed(ctx context.Context, component string) error {
	manifestPath, _ := ctx.Value(provider.ManifestPathKey{}).(string)
	if manifestPath == "" {
		return errors.New("manifest path not found in context -- cannot verify cleanup")
	}
	verifier, err := verify.GetVerifierFromManifest(manifestPath)
	if err != nil {
		return err
	}
	return verifier.VerifyAbsent(ctx, h.kubeconfigPath)
}
