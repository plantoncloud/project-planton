package runner

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/pkg/errors"

	"github.com/plantonhq/planton/e2e/framework/provider"
)

// SetupScriptAnnotation names a repo-committed bash script the runner
// executes as the SETUP phase -- after the dependency chain is deployed and
// the scenario's references are resolved, before the component under test
// deploys. Its value is a repo-root-relative path.
//
// The phase exists for DATA-PLANE seeding: assets a scenario needs that no
// catalog kind can create because they are content inside a fixture, not an
// ARM/cloud control-plane object (the first user: a trivial MLflow model
// registered into the fixture ML workspace, which Azure requires before it
// will provision a managed online deployment). Everything a catalog kind CAN
// create must keep entering through registry prerequisites or the
// e2e-prerequisites annotation -- this hook is not an alternative fixture
// mechanism, and a script that creates control-plane resources would dodge
// the teardown and orphan-sweep guarantees those paths carry.
//
// Contract:
//   - The script runs once per engine lane, via bash, from the repo root.
//   - It inherits the process environment (cloud CLI logins, ARM_* exports)
//     plus E2E_RUN_ID (engine-scoped), E2E_SCENARIO, and E2E_SETUP_OUTPUT
//     (see below).
//   - A non-zero exit fails the lane BEFORE the component deploys; the
//     dependency chain still tears down.
//   - It must be IDEMPOTENT per lane (an in-lane retry may re-run it) and
//     must seed ONLY into fixture-owned resources, so DEPENDENCIES-DOWN
//     destroys everything it created and the zero-orphan sweep stays honest.
//     No teardown pair exists by design.
//
// Publishing values to the manifest under test: some seeded facts exist only
// AFTER seeding and are exactly what the component must declare -- the
// storage path of a backup the script just took, the id of a snapshot it
// cut, a name a fixture's controller generated. The script publishes them as
// `NAME=value` lines (one per line, NAME matching [A-Z][A-Z0-9_]*) into the
// file at $E2E_SETUP_OUTPUT, and the scenario manifest references them as
// `${E2E_SETUP:NAME}` tokens, which the runner expands after the script
// returns and before VALIDATE. Every token must be published (a residual one
// fails the lane loudly); values are single-line text. This is the one
// channel by which the data plane may inform the manifest -- a restore
// proof's "restore from THE backup the seed just wrote" is the motivating
// class, and it keeps the committed scenario honest: no manifest ever
// hardcodes a backup name that only one run ever produced.
const SetupScriptAnnotation = "planton.dev/e2e-setup-script"

// SetupOutputEnvVar is the environment variable carrying the path the setup
// script publishes `NAME=value` lines into.
const SetupOutputEnvVar = "E2E_SETUP_OUTPUT"

// setupTokenPattern matches ${E2E_SETUP:NAME} occurrences in a manifest.
var setupTokenPattern = regexp.MustCompile(`\$\{E2E_SETUP:([A-Z][A-Z0-9_]*)\}`)

// runSetupScript executes the scenario's setup script, then expands any
// ${E2E_SETUP:NAME} tokens in the manifest under test from the values the
// script published. It returns the manifest path to continue with: the
// original when the manifest carries no setup tokens, else an expanded copy
// (keeping the scenario's basename -- verifier dispatch keys off it). The
// caller resolves the annotation; an empty scriptRel is the caller's bug,
// not a skip.
func runSetupScript(tc *provider.ComponentTestContext, scriptRel, engineScopedRunID string) (string, error) {
	scriptPath := filepath.Join(tc.RepoRoot, scriptRel)
	if _, err := os.Stat(scriptPath); err != nil {
		return "", errors.Wrapf(err, "setup script %s (from the %s annotation) is not readable", scriptRel, SetupScriptAnnotation)
	}

	outputFile, err := os.CreateTemp("", "planton-e2e-setup-output-*.env")
	if err != nil {
		return "", errors.Wrap(err, "failed to create the setup output file")
	}
	outputPath := outputFile.Name()
	if err := outputFile.Close(); err != nil {
		return "", errors.Wrap(err, "failed to close the setup output file")
	}
	defer os.Remove(outputPath)

	fmt.Printf("  [setup] running %s\n", scriptRel)
	cmd := exec.Command("bash", scriptPath)
	cmd.Dir = tc.RepoRoot
	cmd.Env = append(os.Environ(),
		"E2E_RUN_ID="+engineScopedRunID,
		"E2E_SCENARIO="+ScenarioSlug(tc.ManifestPath),
		SetupOutputEnvVar+"="+outputPath,
	)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return "", errors.Wrapf(err, "setup script %s failed", scriptRel)
	}
	fmt.Printf("  [setup] %s completed\n", scriptRel)

	published, err := readSetupOutputs(outputPath)
	if err != nil {
		return "", errors.Wrapf(err, "reading what setup script %s published to $%s", scriptRel, SetupOutputEnvVar)
	}
	return expandSetupTokens(tc.ManifestPath, published)
}

// readSetupOutputs parses the `NAME=value` lines a setup script published.
// Blank lines and `#` comments are ignored; anything else must be a
// well-formed assignment.
func readSetupOutputs(path string) (map[string]string, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	published := map[string]string{}
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		name, value, ok := strings.Cut(line, "=")
		name = strings.TrimSpace(name)
		if !ok || !regexp.MustCompile(`^[A-Z][A-Z0-9_]*$`).MatchString(name) {
			return nil, errors.Errorf("malformed setup output line %q: expected NAME=value with NAME matching [A-Z][A-Z0-9_]*", line)
		}
		published[name] = strings.TrimSpace(value)
	}
	return published, scanner.Err()
}

// expandSetupTokens substitutes ${E2E_SETUP:NAME} tokens in the manifest
// from the published values. A manifest without tokens passes through; a
// token no value was published for fails loudly -- the manifest would
// otherwise deploy the literal token as a field value.
func expandSetupTokens(manifestPath string, published map[string]string) (string, error) {
	raw, err := os.ReadFile(manifestPath)
	if err != nil {
		return "", errors.Wrapf(err, "failed to read manifest %s for setup-token expansion", manifestPath)
	}
	if !strings.Contains(string(raw), "${E2E_SETUP:") {
		return manifestPath, nil
	}

	var missing []string
	expanded := setupTokenPattern.ReplaceAllStringFunc(string(raw), func(token string) string {
		name := setupTokenPattern.FindStringSubmatch(token)[1]
		value, ok := published[name]
		if !ok || value == "" {
			missing = append(missing, name)
			return token
		}
		if strings.Contains(value, "\n") {
			missing = append(missing, name+" (multi-line values are not supported)")
			return token
		}
		return value
	})
	if len(missing) > 0 {
		return "", errors.Errorf("manifest %s references setup tokens the setup script did not publish to $%s: %s",
			manifestPath, SetupOutputEnvVar, strings.Join(missing, ", "))
	}
	if idx := strings.Index(expanded, "${E2E_SETUP:"); idx >= 0 {
		return "", errors.Errorf("manifest %s carries a malformed setup token near %q (names must match [A-Z][A-Z0-9_]*)",
			manifestPath, expanded[idx:min(idx+40, len(expanded))])
	}

	tmpDir, err := os.MkdirTemp("", "planton-e2e-setup-expanded-*")
	if err != nil {
		return "", errors.Wrap(err, "failed to create temp dir for the setup-expanded manifest")
	}
	tmpPath := filepath.Join(tmpDir, filepath.Base(manifestPath))
	if err := os.WriteFile(tmpPath, []byte(expanded), 0o600); err != nil {
		return "", errors.Wrap(err, "failed to write the setup-expanded manifest")
	}
	return tmpPath, nil
}
