package runner

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	tt "github.com/gruntwork-io/terratest/modules/terraform"
	"github.com/pkg/errors"
	"github.com/plantonhq/planton/e2e/framework/provider"
	"github.com/plantonhq/planton/pkg/crkreflect"
)

// Phase represents a stage in the E2E test lifecycle.
type Phase string

const (
	PhaseDepsUp      Phase = "DEPENDENCIES-UP"
	PhaseSetup       Phase = "SETUP"
	PhaseValidate    Phase = "VALIDATE"
	PhaseDeploy      Phase = "DEPLOY"
	PhaseIdempotency Phase = "IDEMPOTENCY"
	PhaseVerifyOut   Phase = "VERIFY-OUT"
	PhaseVerifyRes   Phase = "VERIFY-RES"
	PhaseImportRT    Phase = "IMPORT-RT"
	PhaseDestroy     Phase = "DESTROY"
	PhaseVerifyCln   Phase = "VERIFY-CLN"
	PhaseDepsDn      Phase = "DEPENDENCIES-DOWN"

	// Failure-mode phases (see failuremode.go): DEPLOY-EXPECT-FAIL replaces
	// DEPLOY when the scenario expects the deploy itself to fail;
	// VERIFY-CAUSE follows VERIFY-RES when the scenario's workload fails by
	// design and the failure's cause must be pinned.
	PhaseExpectFail  Phase = "DEPLOY-EXPECT-FAIL"
	PhaseVerifyCause Phase = "VERIFY-CAUSE"
)

// PhaseResult captures the outcome of a single lifecycle phase.
type PhaseResult struct {
	Phase    Phase
	Passed   bool
	Duration time.Duration
	Error    error
}

// TestResult captures the full 6-phase lifecycle outcome for a component.
type TestResult struct {
	Component string
	Engine    string
	Phases    []PhaseResult
	Passed    bool
	Duration  time.Duration
}

// RunComponentTest executes the E2E lifecycle for a single component.
// If the component has dependencies (registry prerequisites), they are deployed
// first and torn down last, wrapping the standard 6-phase lifecycle.
func RunComponentTest(ctx context.Context, tc *provider.ComponentTestContext, harness provider.Harness) *TestResult {
	start := time.Now()
	result := &TestResult{
		Component: tc.Component,
		Engine:    tc.Engine,
		Passed:    true,
	}

	// Expand per-run unique-id tokens before anything parses the manifest, so
	// identifiers that cloud providers reserve across soft-delete windows get a
	// fresh value on every run (and on each engine within a run).
	expandRunID := EngineScopedRunID(tc.RunID, tc.Engine)
	expandedPath, err := ExpandManifestTokens(tc.ManifestPath, expandRunID, ScenarioSlug(tc.ManifestPath))
	if err != nil {
		result.Passed = false
		result.Phases = append(result.Phases, PhaseResult{
			Phase:  PhaseValidate,
			Passed: false,
			Error:  errors.Wrap(err, "failed to expand manifest tokens"),
		})
		result.Duration = time.Since(start)
		return result
	}
	tc.ManifestPath = expandedPath

	verifyCtx := context.WithValue(ctx, provider.ManifestPathKey{}, tc.ManifestPath)

	// Phase 0: deploy dependencies (registry prerequisites merged with any the
	// scenario manifest declares via its e2e-prerequisites annotation)
	var dependencyStates []DependencyState
	if tc.RepoRoot != "" {
		depStart := time.Now()
		var err error
		// The engine-scoped id is passed down so prerequisite manifests expand to
		// the same values as the scenario under test (their tokens must line up),
		// and so each engine's prerequisite deploys get distinct identifiers.
		dependencyStates, err = DeployDependencies(ctx, tc.RepoRoot, tc.Provider, tc.Component, tc.ManifestPath, tc.BackendURL, expandRunID, harness)
		pr := PhaseResult{
			Phase:    PhaseDepsUp,
			Duration: time.Since(depStart),
			Passed:   err == nil,
			Error:    err,
		}
		if len(dependencyStates) > 0 || err != nil {
			result.Phases = append(result.Phases, pr)
		}
		if err != nil {
			result.Passed = false
			// The run already failed, but a teardown failure still gets its own
			// phase entry: it means prerequisite resources may be leaking.
			if tdErr := TeardownDependencies(dependencyStates); tdErr != nil {
				result.Phases = append(result.Phases, PhaseResult{
					Phase:  PhaseDepsDn,
					Passed: false,
					Error:  tdErr,
				})
			}
			result.Duration = time.Since(start)
			return result
		}

		// Resolve the component manifest's value_from refs against the deployed
		// prerequisites' outputs -- the orchestrator's resolution step, performed
		// here so a composed topology (e.g. subnet -> vpc) can be tested standalone.
		if len(dependencyStates) > 0 {
			depOutputs := make(DependencyOutputs, len(dependencyStates))
			for _, depState := range dependencyStates {
				kind := crkreflect.KindFromString(depState.Dependency.KindSlug)
				if depOutputs[kind] == nil {
					depOutputs[kind] = make(map[string]map[string]interface{})
				}
				depOutputs[kind][depState.ManifestName] = depState.Outputs
			}
			resolvedPath, resolveErr := ResolveManifestRefs(tc.ManifestPath, depOutputs)
			if resolveErr != nil {
				result.Passed = false
				result.Phases = append(result.Phases, PhaseResult{
					Phase:    PhaseValidate,
					Duration: 0,
					Passed:   false,
					Error:    errors.Wrap(resolveErr, "failed to resolve manifest references from dependency outputs"),
				})
				if tdErr := TeardownDependencies(dependencyStates); tdErr != nil {
					result.Phases = append(result.Phases, PhaseResult{
						Phase:  PhaseDepsDn,
						Passed: false,
						Error:  tdErr,
					})
				}
				result.Duration = time.Since(start)
				return result
			}
			tc.ManifestPath = resolvedPath
			verifyCtx = context.WithValue(ctx, provider.ManifestPathKey{}, tc.ManifestPath)
		}
	}

	// SETUP: the scenario's data-plane seeding hook (see SetupScriptAnnotation).
	// It runs after the dependency chain and reference resolution because the
	// assets it seeds live INSIDE fixtures, and before VALIDATE because a
	// seeding failure must stop the lane before any component deploy.
	if setupScript, annErr := ManifestAnnotation(tc.ManifestPath, SetupScriptAnnotation); annErr == nil && setupScript != "" {
		setupStart := time.Now()
		// The script may publish values the manifest under test consumes
		// (${E2E_SETUP:NAME} tokens -- a restore proof's "the backup the
		// seed just wrote"); the expanded copy is what every later phase
		// deploys and verifies against.
		seededPath, setupErr := runSetupScript(tc, setupScript, expandRunID)
		if setupErr == nil {
			tc.ManifestPath = seededPath
			verifyCtx = context.WithValue(ctx, provider.ManifestPathKey{}, tc.ManifestPath)
		}
		result.Phases = append(result.Phases, PhaseResult{
			Phase:    PhaseSetup,
			Duration: time.Since(setupStart),
			Passed:   setupErr == nil,
			Error:    setupErr,
		})
		if setupErr != nil {
			result.Passed = false
			if tdErr := TeardownDependencies(dependencyStates); tdErr != nil {
				result.Phases = append(result.Phases, PhaseResult{
					Phase:  PhaseDepsDn,
					Passed: false,
					Error:  tdErr,
				})
			}
			result.Duration = time.Since(start)
			return result
		}
	}

	// IDENTITY: the identity the lane deploys as (see identity.go), created
	// after the fixtures and SETUP, before VALIDATE because the binding reads
	// it.
	identityStart := time.Now()
	identityCleanup, identityErr := provisionIdentity(ctx, tc, harness)
	if identityErr != nil {
		result.Passed = false
		result.Phases = append(result.Phases, PhaseResult{
			Phase:    PhaseIdentity,
			Duration: time.Since(identityStart),
			Passed:   false,
			Error:    identityErr,
		})
		if tdErr := TeardownDependencies(dependencyStates); tdErr != nil {
			result.Phases = append(result.Phases, PhaseResult{Phase: PhaseDepsDn, Passed: false, Error: tdErr})
		}
		result.Duration = time.Since(start)
		return result
	}
	// The identity outlives every phase that deploys or destroys as it (the
	// destroy must run as the same identity) and every early return below;
	// its objects live in the harness's own namespace, apart from the fixture
	// chain, so the order against DEPENDENCIES-DOWN does not matter.
	defer identityCleanup()
	if tc.IdentityProviderConfig != "" {
		result.Phases = append(result.Phases, PhaseResult{
			Phase:    PhaseIdentity,
			Duration: time.Since(identityStart),
			Passed:   true,
		})
	}

	// Failure-mode annotations (see failuremode.go). Mutually exclusive: one
	// says the deploy itself must fail, the other presupposes it succeeds.
	expectDeployFailure, _ := ManifestAnnotation(tc.ManifestPath, ExpectDeployFailureAnnotation)
	expectedRuntimeCause, _ := ManifestAnnotation(tc.ManifestPath, ExpectedRuntimeFailureAnnotation)
	if expectDeployFailure != "" && expectedRuntimeCause != "" {
		result.Passed = false
		result.Phases = append(result.Phases, PhaseResult{
			Phase:  PhaseValidate,
			Passed: false,
			Error: errors.Errorf("scenario carries both %s and %s -- they are mutually exclusive",
				ExpectDeployFailureAnnotation, ExpectedRuntimeFailureAnnotation),
		})
		if tdErr := TeardownDependencies(dependencyStates); tdErr != nil {
			result.Phases = append(result.Phases, PhaseResult{Phase: PhaseDepsDn, Passed: false, Error: tdErr})
		}
		result.Duration = time.Since(start)
		return result
	}

	// Lifecycle annotations (see lifecycle.go): a second deploy against the
	// same stack (upgrade, or an upgrade that must be refused) and a second
	// install after the first destroy. They presuppose a successful first
	// deploy, so they never combine with expect-deploy-failure.
	lifecycle, lifecycleErr := readLifecycleAnnotations(tc.ManifestPath)
	if lifecycleErr == nil && expectDeployFailure != "" && (lifecycle.upgradeManifest != "" || lifecycle.reinstall) {
		lifecycleErr = errors.Errorf("scenario carries %s together with a lifecycle annotation -- a deploy that must fail has no second act",
			ExpectDeployFailureAnnotation)
	}
	if lifecycleErr != nil {
		result.Passed = false
		result.Phases = append(result.Phases, PhaseResult{Phase: PhaseValidate, Passed: false, Error: lifecycleErr})
		if tdErr := TeardownDependencies(dependencyStates); tdErr != nil {
			result.Phases = append(result.Phases, PhaseResult{Phase: PhaseDepsDn, Passed: false, Error: tdErr})
		}
		result.Duration = time.Since(start)
		return result
	}
	originalManifest := tc.ManifestPath

	// Phases 1-6 (7 with the opt-in import round-trip): standard lifecycle.
	type lifecyclePhase struct {
		phase Phase
		fn    func() error
	}
	var phases []lifecyclePhase
	if expectDeployFailure != "" {
		// The expected-deploy-failure lifecycle: the deploy MUST fail, the
		// harness pins the cause post-mortem (before destroy, while the
		// partially-created resource is inspectable), then the tainted stack
		// is destroyed and absence verified. Idempotency, output and resource
		// verification, and import round-trips all presuppose a successful
		// deploy and are structurally excluded.
		phases = []lifecyclePhase{
			{PhaseValidate, func() error { return runValidate(tc) }},
			{PhaseExpectFail, func() error { return runExpectDeployFailure(verifyCtx, tc, harness, expectDeployFailure) }},
			{PhaseDestroy, func() error { return runDestroyMaybeRetry(tc) }},
			{PhaseVerifyCln, func() error { return runVerifyCleanup(verifyCtx, tc, harness) }},
		}
	} else {
		phases = []lifecyclePhase{
			{PhaseValidate, func() error { return runValidate(tc) }},
			{PhaseDeploy, func() error { return runDeploy(tc) }},
		}
		// The idempotency gate re-plans the configuration DEPLOY just applied and
		// fails on any pending change: a dirty second plan means the module and
		// the provider disagree about the applied state (the send-omitted-value /
		// Optional+Computed echo defect classes), which users would meet as a
		// perpetual diff on every re-apply. Provider profiles arm it via
		// assert_apply_idempotency; prerequisite fixtures are deliberately outside
		// its scope (they belong to other kinds' contracts).
		if tc.AssertApplyIdempotency {
			phases = append(phases, lifecyclePhase{PhaseIdempotency, func() error { return runIdempotency(tc) }})
		}
		phases = append(phases,
			lifecyclePhase{PhaseVerifyOut, func() error { return runVerifyOutputs(tc) }},
			lifecyclePhase{PhaseVerifyRes, func() error { return runVerifyResources(verifyCtx, tc, harness) }},
		)
		// VERIFY-CAUSE slots right after VERIFY-RES: the workload's designed
		// failure needs the deployed resource live, and its evidence (states,
		// logs) must be read before anything tears down.
		if expectedRuntimeCause != "" {
			phases = append(phases, lifecyclePhase{PhaseVerifyCause, func() error {
				return runVerifyRuntimeCause(verifyCtx, tc, harness, expectedRuntimeCause)
			}})
		}
		// The import round-trip slots between VERIFY-RES and DESTROY: it needs the
		// deployed fixture live (imports read the real cloud) and the destroy that
		// follows tears down through the re-imported state -- itself part of the
		// proof that the blind import fully owns the resources.
		if importRoundTripEnabled(tc) {
			phases = append(phases, lifecyclePhase{PhaseImportRT, func() error { return runImportRoundTrip(tc) }})
		}
		// The second act (lifecycle.go). An upgrade re-deploys the same stack
		// from the second manifest and verifies against THAT manifest; the
		// destroy that follows tears down the upgraded state. An expected
		// upgrade failure restores the first manifest's inputs before the
		// destroy, so the destroy matches what was built.
		cleanupCtx := verifyCtx
		if lifecycle.upgradeManifest != "" {
			upgradeManifest := lifecycle.upgradeManifest
			if lifecycle.expectUpgradeFailure != "" {
				expectation := lifecycle.expectUpgradeFailure
				phases = append(phases, lifecyclePhase{PhaseUpgradeExpectFail, func() error {
					return runUpgradeExpectFailure(verifyCtx, tc, harness, upgradeManifest, expectation, originalManifest)
				}})
			} else {
				upgradedCtx := context.WithValue(ctx, provider.ManifestPathKey{}, upgradeManifest)
				cleanupCtx = upgradedCtx
				phases = append(phases,
					lifecyclePhase{PhaseUpgrade, func() error { return runUpgrade(tc, upgradeManifest) }},
					lifecyclePhase{PhaseVerifyUpgraded, func() error {
						if err := runVerifyOutputs(tc); err != nil {
							return err
						}
						return runVerifyResources(upgradedCtx, tc, harness)
					}},
				)
			}
		}
		phases = append(phases,
			lifecyclePhase{PhaseDestroy, func() error { return runDestroyMaybeRetry(tc) }},
			lifecyclePhase{PhaseVerifyCln, func() error { return runVerifyCleanup(cleanupCtx, tc, harness) }},
		)
		// The third act: the same manifest again, on a cluster that may still
		// carry what the first install deliberately kept.
		if lifecycle.reinstall {
			phases = append(phases,
				lifecyclePhase{PhaseReinstall, func() error {
					if err := bindManifest(tc, originalManifest); err != nil {
						return err
					}
					return runReinstall(tc)
				}},
				lifecyclePhase{PhaseVerifyReinstalled, func() error {
					if err := runVerifyOutputs(tc); err != nil {
						return err
					}
					return runVerifyResources(verifyCtx, tc, harness)
				}},
				lifecyclePhase{PhaseDestroyAgain, func() error { return runDestroyMaybeRetry(tc) }},
				lifecyclePhase{PhaseVerifyClnAgain, func() error { return runVerifyCleanup(verifyCtx, tc, harness) }},
			)
		}
	}

	for _, p := range phases {
		phaseStart := time.Now()
		err := p.fn()
		pr := PhaseResult{
			Phase:    p.phase,
			Duration: time.Since(phaseStart),
			Passed:   err == nil,
			Error:    err,
		}
		result.Phases = append(result.Phases, pr)

		if err != nil {
			result.Passed = false
			switch p.phase {
			case PhaseDeploy, PhaseIdempotency, PhaseVerifyOut, PhaseVerifyRes, PhaseVerifyCause, PhaseImportRT, PhaseExpectFail,
				PhaseUpgrade, PhaseVerifyUpgraded, PhaseUpgradeExpectFail, PhaseReinstall, PhaseVerifyReinstalled:
				// A failed DEPLOY-EXPECT-FAIL needs the cleanup destroy on BOTH
				// branches: an unexpected deploy success leaves live resources,
				// and a failed post-mortem leaves the tainted partial stack.
				cleanupErr := runDestroyMaybeRetry(tc)
				if cleanupErr != nil {
					fmt.Printf("  [WARN] cleanup destroy also failed: %v\n", cleanupErr)
				}
			}
			break
		}
	}

	// Phase 7: teardown dependencies in reverse order. A teardown failure
	// FAILS the run even when every lifecycle phase passed: it means
	// prerequisite cloud resources may still exist, and a green result would
	// hide that leak until someone audits the account.
	if len(dependencyStates) > 0 {
		depStart := time.Now()
		tdErr := TeardownDependencies(dependencyStates)
		result.Phases = append(result.Phases, PhaseResult{
			Phase:    PhaseDepsDn,
			Duration: time.Since(depStart),
			Passed:   tdErr == nil,
			Error:    tdErr,
		})
		if tdErr != nil {
			result.Passed = false
		}
	}

	// Clean up Terraform working directory if one was created
	if tc.TerraformCleanup != nil {
		tc.TerraformCleanup()
	}

	result.Duration = time.Since(start)
	return result
}

func runValidate(tc *provider.ComponentTestContext) error {
	if tc.ManifestPath == "" {
		return errors.New("manifest path is empty")
	}

	// The Terraform lane needs its isolated module copy before any manifest
	// can be bound to it; the Pulumi lane runs in place.
	if tc.Engine == "terraform" {
		workDir, cleanup, err := PrepareWorkDir(tc.ModuleDir)
		if err != nil {
			return errors.Wrap(err, "validation failed: cannot prepare terraform working directory")
		}
		tc.TerraformWorkDir = workDir
		tc.TerraformCleanup = cleanup
	}

	// Binding the manifest (the provider-config fixture, the stack input or
	// the tfvars) is shared with the lifecycle lanes, which rebind a second
	// manifest to the same stack the same way.
	if err := bindManifest(tc, tc.ManifestPath); err != nil {
		if tc.TerraformCleanup != nil {
			tc.TerraformCleanup()
		}
		return errors.Wrap(err, "validation failed")
	}
	return nil
}

func runDeploy(tc *provider.ComponentTestContext) error {
	switch tc.Engine {
	case "pulumi":
		_, err := PulumiDeploy(tc.ModuleDir, tc.StackName, tc.BackendURL, tc.StackInputFilePath)
		return err
	case "terraform":
		opts, ok := tc.TerraformOpts.(*tt.Options)
		if !ok || opts == nil {
			return errors.New("terraform options not initialized (runValidate must run first)")
		}
		_, err := TerraformDeploy(tc.T, opts)
		return err
	default:
		return errors.Errorf("unsupported engine: %s", tc.Engine)
	}
}

// runIdempotency re-plans the configuration runDeploy just applied and fails
// on any pending change. See the phase-insertion comment in RunComponentTest
// for why this gate exists and what a failure means.
func runIdempotency(tc *provider.ComponentTestContext) error {
	switch tc.Engine {
	case "pulumi":
		_, err := PulumiPreviewExpectNoChanges(tc.ModuleDir, tc.StackName, tc.BackendURL, tc.StackInputFilePath)
		return err
	case "terraform":
		opts, ok := tc.TerraformOpts.(*tt.Options)
		if !ok || opts == nil {
			return errors.New("terraform options not initialized (runValidate must run first)")
		}
		_, err := TerraformPlanNoChanges(tc.T, opts)
		return err
	default:
		return errors.Errorf("unsupported engine: %s", tc.Engine)
	}
}

func runVerifyOutputs(tc *provider.ComponentTestContext) error {
	switch tc.Engine {
	case "pulumi":
		outputJSON, err := PulumiStackOutputs(tc.ModuleDir, tc.StackName, tc.BackendURL)
		if err != nil {
			return errors.Wrap(err, "failed to retrieve pulumi stack outputs")
		}
		rawOutputs, parseErr := parsePulumiOutputs(outputJSON)
		if parseErr != nil {
			return errors.Wrap(parseErr, "failed to parse pulumi stack outputs JSON")
		}
		tc.Outputs = rawOutputs

	case "terraform":
		opts, ok := tc.TerraformOpts.(*tt.Options)
		if !ok || opts == nil {
			return errors.New("terraform options not initialized (runValidate must run first)")
		}
		rawOutputs, err := TerraformOutputs(tc.T, opts)
		if err != nil {
			return errors.Wrap(err, "failed to retrieve terraform outputs")
		}
		tc.Outputs = rawOutputs

	default:
		return nil
	}

	if len(tc.Outputs) == 0 {
		fmt.Printf("  [outputs] %s: no outputs captured, skipping transformation validation\n", tc.Component)
		return nil
	}

	msg, flatOutputs, err := VerifyOutputTransformation(tc.Component, tc.Outputs, tc.ModuleDir)
	if err != nil {
		return err
	}
	tc.FlatOutputs = flatOutputs
	tc.TransformedOutputs = msg
	return nil
}

func runVerifyResources(ctx context.Context, tc *provider.ComponentTestContext, harness provider.Harness) error {
	return harness.VerifyDeployed(ctx, tc.Component, tc.Outputs)
}

// DestroyRetryAnnotation opts ONE scenario into bounded destroy retries; its
// value MUST state the reason (an empty value does not opt in). It exists for
// resource classes whose provider DELETE is refused for a bounded window by
// the cloud itself, so the engine's destroy fails no matter how correct the
// module is and succeeds verbatim once the window passes -- exactly what a
// human operator would do by re-running destroy. First user: Cloudflare
// email-routing destination addresses, whose delete answers 400 code 2032
// "Destination address has been created too recently" until ~10 minutes
// after create (measured 2026-08-26: refused at 9m14s, accepted at 10m15s).
// The retry is deliberately dumb -- full destroy re-runs on a fixed interval
// under a fixed budget -- because parsing engine output for specific error
// codes would couple the runner to provider error text. Scenarios without
// the annotation keep the single-attempt behavior, so a genuine destroy bug
// still fails loudly everywhere else.
const DestroyRetryAnnotation = "planton.dev/e2e-destroy-retry"

const (
	destroyRetryInterval = 60 * time.Second
	destroyRetryBudget   = 15 * time.Minute
)

// runDestroyMaybeRetry runs destroy once, then -- only for scenarios carrying
// DestroyRetryAnnotation -- keeps re-running it on destroyRetryInterval until
// it succeeds or destroyRetryBudget elapses. Every retry prints the declared
// reason so the lane log records why the phase is waiting.
func runDestroyMaybeRetry(tc *provider.ComponentTestContext) error {
	err := runDestroy(tc)
	if err == nil {
		return nil
	}
	reason, annErr := ManifestAnnotation(tc.ManifestPath, DestroyRetryAnnotation)
	if annErr != nil || reason == "" {
		return err
	}
	deadline := time.Now().Add(destroyRetryBudget)
	for attempt := 2; time.Now().Before(deadline); attempt++ {
		fmt.Printf("  [destroy-retry] attempt %d in %s (scenario-declared: %s); last error: %v\n",
			attempt, destroyRetryInterval, reason, err)
		time.Sleep(destroyRetryInterval)
		if err = runDestroy(tc); err == nil {
			return nil
		}
	}
	return errors.Wrapf(err, "destroy still failing after the %s retry budget", destroyRetryBudget)
}

func runDestroy(tc *provider.ComponentTestContext) error {
	switch tc.Engine {
	case "pulumi":
		_, err := PulumiDestroy(tc.ModuleDir, tc.StackName, tc.BackendURL, tc.StackInputFilePath)
		if err != nil {
			return err
		}
		return PulumiRemoveStack(tc.ModuleDir, tc.StackName, tc.BackendURL)
	case "terraform":
		opts, ok := tc.TerraformOpts.(*tt.Options)
		if !ok || opts == nil {
			return errors.New("terraform options not initialized")
		}
		_, err := TerraformDestroy(tc.T, opts)
		return err
	default:
		return errors.Errorf("unsupported engine: %s", tc.Engine)
	}
}

func runVerifyCleanup(ctx context.Context, tc *provider.ComponentTestContext, harness provider.Harness) error {
	return harness.VerifyDestroyed(ctx, tc.Component)
}

// parsePulumiOutputs converts the JSON string from `pulumi stack output --json`
// into a map[string]interface{} compatible with tc.Outputs.
func parsePulumiOutputs(outputJSON string) (map[string]interface{}, error) {
	if outputJSON == "" {
		return nil, nil
	}
	var raw map[string]interface{}
	if err := json.Unmarshal([]byte(outputJSON), &raw); err != nil {
		return nil, err
	}
	return raw, nil
}
