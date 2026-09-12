//go:build e2e

// Package e2e contains end-to-end tests that deploy real infrastructure
// using Planton IaC modules and verify the results.
//
// These tests require:
//   - kind CLI installed
//   - pulumi CLI installed
//   - kubectl CLI installed
//   - Docker running
//
// Run with: go test -tags=e2e -timeout=30m -v ./e2e/...
package e2e

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"sync"
	"testing"

	"github.com/google/uuid"
	kubernetese2e "github.com/plantonhq/planton/catalog/kubernetes/aa_e2e"
	"github.com/plantonhq/planton/e2e/framework/runner"
)

var (
	// testHarness is the shared kind cluster harness for all Kubernetes tests.
	testHarness *kubernetese2e.Harness

	// ciliumHarness is the lazily-created harness for scenarios annotated with
	// the cilium-cni cluster profile (a dedicated persistent kind cluster with
	// the default CNI disabled). Nil until the first such scenario runs; runs
	// that touch no profiled scenario never create the second cluster.
	ciliumHarness *kubernetese2e.Harness

	// ciliumHarnessMu guards lazy ciliumHarness construction. Scenarios run
	// serially within a process, but the guard keeps the invariant explicit.
	ciliumHarnessMu sync.Mutex

	// repoRoot is the absolute path to the planton repository root.
	repoRoot string

	// runID is a unique identifier for this test run.
	runID string

	// pulumiBackendURL is the file-based backend for Pulumi stacks.
	pulumiBackendURL string
)

// harnessForScenario routes a scenario to the cluster its manifest asks for
// via the planton.dev/e2e-cluster-profile annotation. The default (no
// annotation) is the shared cluster in every lane.
//
// Profiled scenarios are matched, never assumed: in the external-cluster lane
// they run only when the run declares a matching profile for its cluster
// (PLANTON_E2E_CLUSTER_PROFILE) — a profile names what a scenario would DO to
// a cluster as much as what it needs from it, and running an unmatched
// profile on a shared real cluster can destroy it for every later lane
// (e.g. installing a primary CNI on a live EKS cluster). Locally, profiles
// with a kind constructor build their dedicated persistent cluster lazily;
// real-cluster profiles (no local constructor) skip with the reason.
//
// A non-empty skip reason means the scenario cannot run in this lane by
// design; unknown profile values fail loudly rather than silently running on
// the wrong cluster.
func harnessForScenario(manifestPath string) (h *kubernetese2e.Harness, skipReason string, err error) {
	profile, err := runner.ManifestAnnotation(manifestPath, kubernetese2e.ClusterProfileAnnotation)
	if err != nil {
		return nil, "", fmt.Errorf("reading cluster profile from %s: %w", manifestPath, err)
	}

	// A resident-prerequisite declaration is a promise about a REAL cluster
	// (something already runs there that the chain must not reinstall). On a
	// harness-owned cluster nothing is resident, so the declaration is only
	// coherent on a scenario pinned to a real-cluster profile — refuse it
	// anywhere else rather than let a lane skip a prerequisite it needed.
	if declares, err := runner.ScenarioDeclaresResidents(manifestPath); err != nil {
		return nil, "", fmt.Errorf("reading resident prerequisites from %s: %w", manifestPath, err)
	} else if declares && !slices.Contains(kubernetese2e.RealClusterProfiles, profile) {
		return nil, "", fmt.Errorf("scenario %s declares resident prerequisites (%s) but is not pinned to a real-cluster profile via %s (one of %v) — nothing is resident on a harness-owned cluster",
			manifestPath, runner.ScenarioResidentPrerequisitesAnnotation, kubernetese2e.ClusterProfileAnnotation, kubernetese2e.RealClusterProfiles)
	}

	if profile == "" {
		return testHarness, "", nil
	}

	if testHarness.External() {
		declared := os.Getenv(kubernetese2e.ExternalClusterProfileEnvVar)
		if declared == profile {
			return testHarness, "", nil
		}
		return nil, fmt.Sprintf(
			"scenario requires cluster profile %q but the external cluster declares %q (set %s to match)",
			profile, declared, kubernetese2e.ExternalClusterProfileEnvVar), nil
	}

	switch profile {
	case kubernetese2e.ClusterProfileCiliumCni:
		ciliumHarnessMu.Lock()
		defer ciliumHarnessMu.Unlock()
		if ciliumHarness == nil {
			h := kubernetese2e.NewCiliumCniHarness(testHarness.ClusterName())
			if err := h.Setup(context.Background()); err != nil {
				return nil, "", fmt.Errorf("setting up %s cluster profile: %w", profile, err)
			}
			ciliumHarness = h
		}
		return ciliumHarness, "", nil
	case kubernetese2e.ClusterProfileAwsEks, kubernetese2e.ClusterProfileGcpGke:
		return nil, fmt.Sprintf(
			"scenario requires the %q real-cluster profile, which no local cluster can provide — runs in batched real-cluster lanes (%s + %s=%s)",
			profile, kubernetese2e.ExternalKubeconfigEnvVar, kubernetese2e.ExternalClusterProfileEnvVar, profile), nil
	default:
		return nil, "", fmt.Errorf("scenario %s names unknown cluster profile %q", manifestPath, profile)
	}
}

func TestMain(m *testing.M) {
	// Resolve repo root (this file lives at e2e/e2e_test.go)
	var err error
	repoRoot, err = filepath.Abs(filepath.Join(".."))
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to resolve repo root: %v\n", err)
		os.Exit(1)
	}

	// Generate unique run ID
	runID = uuid.New().String()[:8]

	// Set up local Pulumi backend
	backendDir, err := os.MkdirTemp("", "planton-e2e-pulumi-*")
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to create temp backend dir: %v\n", err)
		os.Exit(1)
	}
	pulumiBackendURL = "file://" + backendDir
	defer os.RemoveAll(backendDir)

	// Helm-based kinds render and install charts through whatever Helm
	// repository state the process carries; a per-run directory reproduces
	// the runner's clean state so a developer's stale `helm repo` entry can
	// never fail a lane.
	helmCleanup, err := runner.IsolateHelmEnvironment()
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to isolate the Helm environment: %v\n", err)
		os.Exit(1)
	}
	defer helmCleanup()

	// Log into local backend
	if err := runner.PulumiLogin(pulumiBackendURL); err != nil {
		fmt.Fprintf(os.Stderr, "failed to login to pulumi backend: %v\n", err)
		os.Exit(1)
	}

	// The cluster name is STABLE across runs (not run-ID-suffixed) so the harness's
	// persistent-kind lane can reuse the running cluster from the previous run.
	clusterName := os.Getenv("PLANTON_E2E_KIND_CLUSTER_NAME")
	if clusterName == "" {
		clusterName = "planton-e2e"
	}
	testHarness = kubernetese2e.NewHarness(clusterName)

	ctx := context.Background()
	if err := testHarness.Setup(ctx); err != nil {
		fmt.Fprintf(os.Stderr, "failed to create kind cluster: %v\n", err)
		os.Exit(1)
	}

	// Run tests
	code := m.Run()

	// Teardown kind cluster(s) — the profile cluster first (created last).
	if ciliumHarness != nil {
		if err := ciliumHarness.Teardown(ctx); err != nil {
			fmt.Fprintf(os.Stderr, "warning: failed to tear down cilium profile cluster: %v\n", err)
		}
	}
	if err := testHarness.Teardown(ctx); err != nil {
		fmt.Fprintf(os.Stderr, "warning: failed to delete kind cluster: %v\n", err)
	}

	os.Exit(code)
}
