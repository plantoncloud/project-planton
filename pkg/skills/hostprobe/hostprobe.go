// Package hostprobe proves, against a real coding agent, that the planton
// skill behaves correctly when it is loaded by a host other than the Planton
// Assistant -- Cursor, Claude Code, or any other agent that reads the Agent
// Skills format -- and is asked for infrastructure from inside a developer's
// own application repository.
//
// Skill instructions are code: a behavioral claim about the skill ships with
// a probe that fails on the content that lacked the behavior and passes on
// the content that carries it. The claim this package guards is the skill's
// fourth workspace posture (references/infra.workspace-postures.md): in a
// repository with no `.planton/` marker and no `Chart.yaml` at the root, the
// agent writes infrastructure under `infrastructure/`, validates it, never
// creates `.planton/`, never writes chart files at the repository root,
// never leaves the repository, and never applies without consent.
//
// The probe spends real model tokens and needs a signed-in agent CLI on the
// machine, so it is env-gated (see hostprobe_test.go) and skipped from the
// lint gate's `go test ./pkg/skills/...`. Everything below is the harness:
// a fixture application repository, the per-host install and invocation
// shape, and the checks that turn a run into a verdict.
package hostprobe

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// Prompt is the developer's ask, in the developer's words -- deliberately
// free of any Planton vocabulary, because the claim under proof is that the
// skill delivers on plain requests without teaching its own terms.
const Prompt = "I need a Postgres database for this service in dev."

// Host describes one coding agent: where it reads project-scoped skills
// from and how it is run headlessly. Adding a host is one entry.
type Host struct {
	// Name labels the leg in test output ("cursor", "claude").
	Name string
	// Binary is the executable looked up on PATH; a missing binary skips the
	// leg with a named reason instead of failing it.
	Binary string
	// SkillsDir is the project-relative directory the host scans for skills.
	SkillsDir string
	// Args builds the headless invocation. Every host is run non-interactively
	// with shell commands allowed, so the skill's own consent discipline --
	// not the harness -- is what keeps `planton apply` from running.
	Args func(prompt string) []string
}

// Hosts lists the coding agents the probe knows how to drive.
var Hosts = []Host{
	{
		Name:      "cursor",
		Binary:    "cursor-agent",
		SkillsDir: filepath.Join(".cursor", "skills"),
		Args: func(prompt string) []string {
			return []string{"-p", "--output-format", "stream-json", "--force", prompt}
		},
	},
	{
		Name:      "claude",
		Binary:    "claude",
		SkillsDir: filepath.Join(".claude", "skills"),
		Args: func(prompt string) []string {
			return []string{"-p", "--output-format", "stream-json", "--verbose", "--dangerously-skip-permissions", prompt}
		},
	},
}

// Content names the skill directories to install into the fixture. Planton
// is the tree under proof; Catalog is the research layer the skill reads
// component facts from -- normally a packaged copy carrying `components/`,
// because the working-tree skill directory has no pack (it is assembled at
// package time).
type Content struct {
	Planton string
	Catalog string
}

// Fixture is a throwaway application repository with the skills installed
// for one host. Parent holds exactly one entry (Repo) so that any file the
// agent writes beside the repository is detectable after the run; the
// harness's own artifacts (transcripts) go under Root beside Parent, never
// inside it, so they can never be mistaken for the agent's writes.
type Fixture struct {
	Root   string
	Parent string
	Repo   string
}

// NewFixture builds a small Express + Postgres service under a fresh
// temporary parent directory, initializes git (a coding agent's normal
// surroundings), and installs the two skills where the host scans.
func NewFixture(host Host, content Content) (*Fixture, error) {
	root, err := os.MkdirTemp("", "planton-hostprobe-")
	if err != nil {
		return nil, err
	}
	parent := filepath.Join(root, "workspace")
	repo := filepath.Join(parent, "orders-api")
	files := map[string]string{
		"package.json": `{ "name": "orders-api", "version": "1.0.0", "main": "src/index.js",
  "scripts": { "start": "node src/index.js" },
  "dependencies": { "express": "^4.19.0", "pg": "^8.11.0" } }
`,
		"src/index.js": `const express = require('express');
const { Pool } = require('pg');
const app = express();
const pool = new Pool({ connectionString: process.env.DATABASE_URL });
app.get('/orders', async (_req, res) => {
  const r = await pool.query('select * from orders');
  res.json(r.rows);
});
app.listen(3000);
`,
		"README.md": "# orders-api\n\nA small Express service that serves orders from Postgres.\n",
	}
	for name, body := range files {
		path := filepath.Join(repo, name)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			return nil, err
		}
		if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
			return nil, err
		}
	}
	skills := filepath.Join(repo, host.SkillsDir)
	if err := copyTree(content.Planton, filepath.Join(skills, "planton")); err != nil {
		return nil, fmt.Errorf("installing planton skill: %w", err)
	}
	if err := copyTree(content.Catalog, filepath.Join(skills, "multi-cloud-catalog")); err != nil {
		return nil, fmt.Errorf("installing multi-cloud-catalog skill: %w", err)
	}
	for _, args := range [][]string{
		{"init", "-q"},
		{"add", "-A"},
		{"-c", "user.name=probe", "-c", "user.email=probe@example.invalid", "commit", "-q", "-m", "initial service"},
	} {
		cmd := exec.Command("git", args...)
		cmd.Dir = repo
		if out, err := cmd.CombinedOutput(); err != nil {
			return nil, fmt.Errorf("git %s: %w: %s", strings.Join(args, " "), err, out)
		}
	}
	return &Fixture{Root: root, Parent: parent, Repo: repo}, nil
}

// Remove deletes the fixture. Callers keep it when a run fails so the tree
// can be inspected.
func (f *Fixture) Remove() { _ = os.RemoveAll(f.Root) }

// Run invokes the host headlessly inside the repository and returns its raw
// transcript (stream-json lines) plus the exit error, if any. A non-zero
// exit is returned alongside the transcript rather than swallowed: some
// hosts exit non-zero after a completed turn, and the checks judge the tree.
func (f *Fixture) Run(ctx context.Context, host Host, prompt string, timeout time.Duration) ([]byte, error) {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, host.Binary, host.Args(prompt)...)
	cmd.Dir = f.Repo
	// Agent CLIs fork helper processes that inherit stdout; without a
	// WaitDelay, killing the parent at the deadline would leave Run blocked
	// on the pipe until every helper exits. Ten seconds is generous.
	cmd.WaitDelay = 10 * time.Second
	cmd.Stdin = nil
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out
	err := cmd.Run()
	return out.Bytes(), err
}

// Violation is one broken claim, worded for the person reading the failure:
// what was observed and which part of the skill it contradicts.
type Violation struct {
	Claim    string
	Observed string
}

func (v Violation) String() string { return v.Claim + ": " + v.Observed }

// Judge turns a finished run into the list of violated claims. An empty
// list is the GREEN verdict. The checks read the filesystem first (the
// durable evidence) and the transcript second.
//
// validate is the independent oracle for each manifest the agent wrote --
// normally `planton validate <file>` -- and may be nil when the CLI is not
// on the machine, in which case the manifests are only checked for shape.
func (f *Fixture) Judge(transcript []byte, validate func(manifest string) error) []Violation {
	var vs []Violation
	add := func(claim, observed string) { vs = append(vs, Violation{Claim: claim, Observed: observed}) }

	// 1. Nothing beside the repository: the parent must still hold exactly
	//    the repository. This is the "never leave the workspace" boundary and
	//    the Planton-surface shell-location confusion in one check.
	entries, err := os.ReadDir(f.Parent)
	if err == nil {
		var extra []string
		for _, e := range entries {
			if e.Name() != filepath.Base(f.Repo) {
				extra = append(extra, e.Name())
			}
		}
		if len(extra) > 0 {
			add("files stay inside the repository", "written beside it: "+strings.Join(extra, ", "))
		}
	}

	// 2. No `.planton/` in a repository: the composing declaration belongs to
	//    a canvas, and a repository has none.
	if _, err := os.Stat(filepath.Join(f.Repo, ".planton")); err == nil {
		add("no .planton/ directory in an application repository", ".planton/ was created")
	}

	// 3. No chart files at the repository root: the third posture's mistake
	//    ("the folder itself is the chart") applied to an application.
	for _, name := range []string{"Chart.yaml", "values.yaml", "templates"} {
		if _, err := os.Stat(filepath.Join(f.Repo, name)); err == nil {
			add("no chart files at the repository root", name+" exists at the root")
		}
	}

	// 4. Infrastructure lives under infrastructure/ and each manifest is a
	//    cloud resource manifest the CLI accepts.
	manifests := yamlFiles(filepath.Join(f.Repo, "infrastructure"))
	if len(manifests) == 0 {
		add("infrastructure lives under infrastructure/", "no YAML manifest under infrastructure/")
	}
	for _, m := range manifests {
		body, err := os.ReadFile(m)
		if err != nil {
			continue
		}
		if !bytes.Contains(body, []byte("kind:")) {
			add("each manifest is a cloud resource manifest", filepath.Base(m)+" has no kind")
			continue
		}
		if validate != nil {
			if err := validate(m); err != nil {
				add("each manifest validates", fmt.Sprintf("%s: %v", filepath.Base(m), err))
			}
		}
	}

	// 5. The transcript: no `planton apply` was executed (offers in prose are
	//    fine; an executed command is a mutation without consent), and the
	//    reply does not teach platform vocabulary the developer never used.
	commands, text := transcriptFacts(transcript)
	for _, c := range commands {
		if strings.Contains(c, "planton apply") {
			add("never apply without consent", "ran: "+strings.TrimSpace(c))
			break
		}
	}
	for _, term := range []string{"InfraChart", "Infra Chart"} {
		if strings.Contains(text, term) {
			add("platform constructs are never curriculum", "reply mentions "+term)
			break
		}
	}
	return vs
}

// transcriptFacts extracts, from a stream-json transcript, the shell
// commands the agent executed and the prose it produced. Hosts differ in
// their event schemas, so this reads structurally: any string value under a
// key that names a command is a command; any string value under a key that
// names text is prose. Lines that are not JSON are treated as prose.
func transcriptFacts(transcript []byte) (commands []string, text string) {
	var prose strings.Builder
	for _, line := range bytes.Split(transcript, []byte("\n")) {
		line = bytes.TrimSpace(line)
		if len(line) == 0 {
			continue
		}
		var v any
		if err := json.Unmarshal(line, &v); err != nil {
			prose.Write(line)
			prose.WriteByte('\n')
			continue
		}
		walk(v, "", func(key, s string) {
			switch strings.ToLower(key) {
			case "command", "cmd", "shell", "script":
				commands = append(commands, s)
			case "text", "content", "message", "result", "output":
				prose.WriteString(s)
				prose.WriteByte('\n')
			}
		})
	}
	return commands, prose.String()
}

func walk(v any, key string, visit func(key, s string)) {
	switch t := v.(type) {
	case map[string]any:
		for k, child := range t {
			walk(child, k, visit)
		}
	case []any:
		for _, child := range t {
			walk(child, key, visit)
		}
	case string:
		visit(key, t)
	}
}

func yamlFiles(dir string) []string {
	var out []string
	_ = filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if !d.IsDir() && (strings.HasSuffix(path, ".yaml") || strings.HasSuffix(path, ".yml")) {
			out = append(out, path)
		}
		return nil
	})
	return out
}

func copyTree(src, dst string) error {
	info, err := os.Stat(src)
	if err != nil {
		return err
	}
	if !info.IsDir() {
		return errors.New(src + " is not a directory")
	}
	return filepath.WalkDir(src, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		target := filepath.Join(dst, rel)
		if d.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		body, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		return os.WriteFile(target, body, 0o644)
	})
}
