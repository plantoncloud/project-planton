package runner

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/plantonhq/planton/e2e/framework/provider"
)

func writeSetupScript(t *testing.T, dir, body string) string {
	t.Helper()
	rel := "e2e-setup-test.sh"
	if err := os.WriteFile(filepath.Join(dir, rel), []byte("#!/usr/bin/env bash\nset -euo pipefail\n"+body+"\n"), 0o644); err != nil {
		t.Fatalf("writing script: %v", err)
	}
	return rel
}

// writeSetupManifest writes a scenario manifest into repoRoot and returns
// its path; body is the full YAML text.
func writeSetupManifest(t *testing.T, repoRoot, name, body string) string {
	t.Helper()
	path := filepath.Join(repoRoot, name)
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatalf("writing manifest: %v", err)
	}
	return path
}

func TestRunSetupScript_PassesEnvAndSucceeds(t *testing.T) {
	repoRoot := t.TempDir()
	outPath := filepath.Join(repoRoot, "captured.txt")
	rel := writeSetupScript(t, repoRoot, `echo "$E2E_RUN_ID $E2E_SCENARIO $PWD" > captured.txt`)
	manifest := writeSetupManifest(t, repoRoot, "minimal.yaml", "kind: X\n")

	tc := &provider.ComponentTestContext{
		RepoRoot:     repoRoot,
		ManifestPath: manifest,
	}
	got, err := runSetupScript(tc, rel, "run42-p")
	if err != nil {
		t.Fatalf("expected success, got %v", err)
	}
	if got != manifest {
		t.Fatalf("a manifest without setup tokens must pass through unchanged, got %q", got)
	}
	captured, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatalf("script did not write its capture file: %v", err)
	}
	text := strings.TrimSpace(string(captured))
	if !strings.HasPrefix(text, "run42-p minimal ") {
		t.Fatalf("script env/cwd wrong: %q", text)
	}
	// The script must run from the repo root -- that is the path contract
	// every annotation value is written against. macOS reports TempDir
	// through /private symlinks, so compare resolved paths.
	wantDir, _ := filepath.EvalSymlinks(repoRoot)
	gotDir, _ := filepath.EvalSymlinks(strings.Fields(text)[2])
	if wantDir != gotDir {
		t.Fatalf("script ran from %q, want repo root %q", gotDir, wantDir)
	}
}

func TestRunSetupScript_FailureIsAnError(t *testing.T) {
	repoRoot := t.TempDir()
	rel := writeSetupScript(t, repoRoot, `echo "seeding failed" >&2; exit 3`)

	tc := &provider.ComponentTestContext{RepoRoot: repoRoot, ManifestPath: "minimal.yaml"}
	if _, err := runSetupScript(tc, rel, "run42-t"); err == nil {
		t.Fatal("expected a non-zero script exit to error the phase")
	}
}

func TestRunSetupScript_MissingScriptIsAnError(t *testing.T) {
	tc := &provider.ComponentTestContext{RepoRoot: t.TempDir(), ManifestPath: "minimal.yaml"}
	if _, err := runSetupScript(tc, "does/not/exist.sh", "run42-p"); err == nil {
		t.Fatal("expected a missing script to error the phase")
	}
}

// TestRunSetupScript_PublishesValuesIntoTheManifest guards the
// $E2E_SETUP_OUTPUT channel: a `NAME=value` line the script publishes lands
// where the manifest references `${E2E_SETUP:NAME}`, in an expanded copy
// that keeps the scenario's basename (verifier dispatch keys off it).
func TestRunSetupScript_PublishesValuesIntoTheManifest(t *testing.T) {
	repoRoot := t.TempDir()
	rel := writeSetupScript(t, repoRoot, `
echo "# the backup the seed just wrote" >> "$E2E_SETUP_OUTPUT"
echo "BACKUP_DESTINATION=gs://bucket/prefix/2026-09-09T12:00:00Z" >> "$E2E_SETUP_OUTPUT"
echo "BACKUP_NAME = seed-backup" >> "$E2E_SETUP_OUTPUT"`)
	manifest := writeSetupManifest(t, repoRoot, "gke-gcs-restore.yaml",
		"kind: X\nspec:\n  destination: ${E2E_SETUP:BACKUP_DESTINATION}\n  name: ${E2E_SETUP:BACKUP_NAME}\n")

	tc := &provider.ComponentTestContext{RepoRoot: repoRoot, ManifestPath: manifest}
	got, err := runSetupScript(tc, rel, "run42-p")
	if err != nil {
		t.Fatalf("expected success, got %v", err)
	}
	if got == manifest || filepath.Base(got) != "gke-gcs-restore.yaml" {
		t.Fatalf("expected an expanded copy keeping the basename, got %q", got)
	}
	expanded, _ := os.ReadFile(got)
	want := "kind: X\nspec:\n  destination: gs://bucket/prefix/2026-09-09T12:00:00Z\n  name: seed-backup\n"
	if string(expanded) != want {
		t.Fatalf("expanded manifest =\n%s\nwant\n%s", expanded, want)
	}
}

// A token the script never published must fail the lane, never deploy as
// literal text.
func TestRunSetupScript_UnpublishedTokenIsAnError(t *testing.T) {
	repoRoot := t.TempDir()
	rel := writeSetupScript(t, repoRoot, `echo "OTHER=x" >> "$E2E_SETUP_OUTPUT"`)
	manifest := writeSetupManifest(t, repoRoot, "s.yaml", "spec:\n  destination: ${E2E_SETUP:BACKUP_DESTINATION}\n")

	tc := &provider.ComponentTestContext{RepoRoot: repoRoot, ManifestPath: manifest}
	if _, err := runSetupScript(tc, rel, "run42-p"); err == nil || !strings.Contains(err.Error(), "BACKUP_DESTINATION") {
		t.Fatalf("expected the unpublished-token error naming the token, got %v", err)
	}
}

// A malformed published line (a name outside [A-Z][A-Z0-9_]*) is a script
// defect, reported as such.
func TestRunSetupScript_MalformedOutputLineIsAnError(t *testing.T) {
	repoRoot := t.TempDir()
	rel := writeSetupScript(t, repoRoot, `echo "backup-destination=gs://x/y" >> "$E2E_SETUP_OUTPUT"`)
	manifest := writeSetupManifest(t, repoRoot, "s.yaml", "spec:\n  destination: ${E2E_SETUP:BACKUP_DESTINATION}\n")

	tc := &provider.ComponentTestContext{RepoRoot: repoRoot, ManifestPath: manifest}
	if _, err := runSetupScript(tc, rel, "run42-p"); err == nil || !strings.Contains(err.Error(), "malformed setup output line") {
		t.Fatalf("expected the malformed-line error, got %v", err)
	}
}
