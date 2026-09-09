---
title: Build Failures — Every Way a Managed Build Fails, Who Fixes It, and the Exact Edit
description: The failure taxonomy for a Planton-managed build, by WHERE each failure surfaces — refused before a run exists (validate, publish, service apply), failed at compile on the run record (`The pipeline failed to compile:` + one `[code]` line per verdict), failed before any pod ran (`Failed to create pipeline resources:` — a Git or registry connection, a credential), no runner picked up the build (`No runner is serving this build connection ...`), a task that ran and failed (Tekton's own reason on the build graph, the real cause in the build log — a Dockerfile step, buildpacks detection, a registry push refused, a kustomize overlay, an image the cluster cannot pull), and a build that never finalized (`Build status could not be reconciled within 60m`). For each: the signal on the record, who fixes it, the exact edit or command, and what a rerun does. Read when a build failed or is stuck and someone asks why, when a run's `status_reason` starts with one of those sentences, when a deploy stage was skipped with `Build stage failed` or `No runner picked up the build`, when `planton service logs` says no logs are available, or when deciding whether a failure is the developer's, an administrator's, or the platform's.
---

# Build Failures — Every Way a Managed Build Fails, Who Fixes It, and the Exact Edit

How to READ a run — its two shapes, the status vocabulary, the build graph, the logs tool, the deploy stage and its explained skips — is `references/service.reading-a-run.md`; read it first. This reference is the DIAGNOSIS layer on top: given a failed or stuck build, which of six classes it is, who owns the fix, and what to change. Every compile verdict's fix is `references/service.pipeline-authoring.md` ("Every verdict → the fix"); every publish refusal's fix is `references/service.org-publishing.md`; the platform's tracks, contract, and pinned image set are `references/service.managed-pipelines.md`. Compose with those; never restate them.

The one rule that decides everything: **read `status.status_reason` first.** For four of the six classes it names the class in its first words and IS the diagnosis; only a task that ran and failed sends you to the build log, and only a deploy failure sends you to a stack job.

## 1. Refused before a run exists

Nothing here reaches a run record. `planton service pipeline validate` refuses a pipeline with the same verdicts a dispatch would (the developer fixes the file — authoring reference); `planton apply -f` of a `TektonPipeline`/`TektonTask` record is refused before the RPC with `TektonPipeline '<name>' would not compile, N problem(s): code(subject): ...` (the publisher fixes the record — publishing reference); `planton apply -f service.yaml` naming a published pipeline that does not exist is refused `NOT_FOUND` with `build.tektonPipeline.pipeline '<name>' names no published TektonPipeline in organization '<org>' ...` (publish it first, or `planton search --kind TektonPipeline`). Relay each sentence verbatim; each names its own fix.

## 2. Failed at compile — the verdicts ARE the cause

The run's `status.status_reason` and `status.build_stage.status_reason` carry the same text:

```
The pipeline failed to compile:
- [unresolved_task_ref] eslint-check: <message naming the pipeline task and where a Task may live>
- [undeclared_param] git-branch: <message saying the platform supplies it to every build>
```

One line per verdict, every problem in one pass; the deploy stage is `skipped` with `The pipeline failed to compile`; `spec.resolved_pipeline` is ABSENT (nothing was stamped); there are no build logs (the logs tool answers `No logs available for this run yet.` — a fact, not a gap). Owner: the developer (for `source: repo`) or the record's publisher (for `source: org_published`; the publish check should have caught it — a record that compiled at publish and fails at dispatch usually lost a Task it depended on, which is `unresolved_task_ref` at the next compile). Fix: map each `[code]` through the authoring reference's table, edit, `planton service pipeline validate` until clean, push a NEW commit. A rerun of a compile-failed run has no stamp to replay: it recompiles the SAME commit and fails identically — never offer it as the remedy.

## 3. Failed before any pod ran — a connection or a credential

`status.status_reason` starts with `Failed to create pipeline resources:`; the deploy stage is `skipped` with `Build stage failed`. The runner could not assemble the build's secrets. Owner: an administrator of the organization, on the connection the sentence names:

- `github connection '<slug>' not found in organization '<org>'` / `container registry connection '<slug>' not found in organization '<org>'` — `spec.gitRepo.gitConnection` / `spec.build.registry` names a connection that is gone or misspelled. Fix the slug on the service (`planton get <Kind> ...` to find the right one) or recreate the connection; rerun.
- `building docker-auth secret: resolving gcp artifact registry service account key: ...` / `... resolving ghcr username: ...` / `... resolving ghcr personal access token: ...` — a registry connection on the **stored-keys** arm references a secret that cannot be read. Fix the secret reference or the secret; rerun.
- `GitHub connection '<slug>' could not provide a credential for GHCR — ...` / `AWS connection '<slug>' could not sign in on this runner — ...` / `GCP connection '<slug>' could not sign in on this runner — ...` / `Azure connection '<slug>' could not sign in on this runner — ...` — a registry connection on the **trusted-connection** arm: the runner derives the push credential from the connection the registry names, and that connection could not sign in (a machine signed out of `gh`, an expired cloud session, a sibling deleted since the registry was created). The sentence after the dash is the trusted connection's own diagnosis — relay it verbatim; the fix is on that connection (`gh auth login`, `aws sso login`, ...), never on the registry. `Container registry connection 'X' signs in to GHCR through GitHub connection 'Y', which does not exist in organization 'Z' — create that connection first, or point this registry at one that exists.` is the same class, refused at input assembly before any pod starts.
- `GHCR refused the GitHub sign-in behind connection '<slug>' — this sign-in can read code but not packages; ...` — the machine's `gh` sign-in lacks the packages scopes. The sentence names the command: `gh auth refresh -s read:packages,write:packages`. `<source> signs in to AWS account 111..., but this registry is in account 222... — point the registry at a connection for account 222..., or change its account id.` / `Registry <host> refused the Azure identity ... — it needs the AcrPush role` / `Google did not issue a token for GCP connection ...` are the per-provider refusals; each names its remedy.
- `AWS refused to issue an ECR token for ... — the identity has no ecr:GetAuthorizationToken permission, or its credentials are no longer valid.` / `... no authorization data returned from AWS ECR` — the ECR credential (stored keys, or the trusted AWS connection's identity) cannot obtain a registry token. Fix the credential or grant the permission; rerun. Every registry connection can be proven before a build tries it: **Verify This Registry** on its page (or the verdict `planton connect registry detect` reads right after saving) derives the credential on the runner that would push and makes one benign registry call — a registry that would fail the push fails the verify first, with the same sentence.

A rerun replays the stamped pipeline and re-mints the secrets, so once the connection is fixed the rerun is the right verb.

## 4. No runner picked up the build

`status.status_reason`:

```
No runner is serving this build connection '<connection>': runner '<runner>' did not pick up the build within 2 minutes (task queue pipeline-build.<channel>). Start that runner, or point the connection at one that is running (planton build-connection resolve).
```

deploy stage `skipped` with `No runner picked up the build`. Nothing but a runner polls that queue, so the sentence means exactly what it says: the runner the routing chose is scaled to zero, crashed, or has its build worker disabled. Owner: whoever runs that runner (the organization for its own build connection; a platform operator for the platform default). Diagnose with `resolve_build_connection` (CLI `planton build-connection resolve`) — the effective connection, its runner, the tier that decided, and the connection's recorded readiness — and `get_runner_registration` / `search_runner_registrations` (CLI `planton runner get <name>` / `planton runner list`). Fix: start the runner, or point the connection at one that is running; then `rerun` — the stamped pipeline needs no recompile. A run that was never CREATED because routing itself failed carries the resolver's sentences instead (`No build connection available for org '<org>': ... Set an org default (DefaultTektonConnection) ...`, `TektonConnection '<org>/<slug>' references runner '<r>', which no longer exists ...`, `Runner '<r>' reported at its last startup that its build capability is disabled ... Enable BUILD_WORKER_ENABLED ...`) — relay them; each names its fix.

## 5. A task ran and failed — Tekton's words, then the log

`status.build_stage.result` is `failed`; the first node in `status.build_stage.dag.nodes[]` with `execution.result == failed` is the culprit (a node's `execution.error` is Tekton's condition message and is present whatever the outcome — key on `result`, never on the presence of `error`). `build_stage.status_reason` is the failed tasks' Tekton messages verbatim (one message, or `task: message` per line). Those name WHICH task and Tekton's reason; WHY is in the log — `get_service_pipeline_logs` (CLI `planton service logs <run-id>`), relayed verbatim per `reading-a-run`. The classes, by what the log (or Tekton's reason) says:

| Signal | Who fixes | The edit |
|---|---|---|
| A Dockerfile `RUN` step fails (`exit code: 127`, a compiler error, `not found`) | developer | the Dockerfile or the source; check `spec.build.dockerfile.dockerfilePath` and `context` if the file or its inputs are missing at build time |
| Buildpacks detection fails (no language marker files, unsupported stack) | developer | the marker files at `spec.gitRepo.projectRoot` (`package.json`, `go.mod`, `pom.xml`, ...), or switch the builder to `dockerfile` |
| A repository Task fails (tests, lint, a scan) | developer | the code the task checks, or the Task itself (authoring reference) |
| The push is `denied` / `unauthorized` / `authentication required` | administrator | the registry connection's credential and its push permission; `spec.build.imageRepositoryPath` |
| `kustomize-build` fails on a missing overlay or a bad path | developer | the `_kustomize/overlays/<env>` tree (`references/service.kustomize-authoring.md`) |
| Tekton reason `TaskRunImagePullFailed` / `PullImageFailed`, no step output | administrator (the build cluster) | egress from the cluster to the registries the task images live on, or mirror the pinned image set (`references/service.managed-pipelines.md` — the images a build cluster pulls); `verify_tekton_connection` names the unreachable host and lists the images |
| Tekton reason `TaskRunTimeout` / `PipelineRunTimeout` | developer (a hang) or administrator (a starved cluster) | the step that hung (last lines of its log), or the cluster |
| `importing cache manifest ... not found` in a BuildKit log | nobody | a cold-cache miss — NEVER report it as the cause; keep reading |

After a code fix: a NEW commit (the push starts the run). After a credential or egress fix: `rerun` is right — the stamped pipeline and the same params, re-minted secrets.

**Not a build failure, though it looks like one**: a DEPLOYED pod in `ImagePullBackOff` / `ErrImagePull` — the build pushed fine; the cluster cannot pull. The rollout verdict carries the kubelet's line and the platform's remedy (*declare the registry login on the workload, or a pull secret beside it, or pull from a registry the cluster's own identity reaches*); the run's environment row says whether the deploy filled a login from the registry connection and, if not, why (a GHCR connection through a sign-in with no pull token; ECR on any arm). A rerun changes nothing until that is fixed — `references/service.pulling-private-images.md` has the diagnosis order and each case's fix.

## 6. Never finalized — the outcome could not be read

`status.build_stage.status_reason`:

```
Build status could not be reconciled within 60m
Build status could not be reconciled within 60m: runner '<runner>' stopped polling its build queue, so the build's outcome could not be read
```

Tekton's events are best-effort; the platform polls the runner every minute and gives up after an hour. The second form means the runner vanished MID-build — the platform does not fail the run early because the cluster may still be running the build; the administrator restarts the runner (`planton runner ...`) and reruns. The first form with a healthy runner points at the events path: `verify_tekton_connection` reports whether Tekton's events sink is configured (`events-sink`) and whether the runner's log streamer covers the build's namespace (`logs-coverage`) — both `WARN` with the fix in the sentence.

## When it is the platform, not the user

Say so plainly and hand it to the platform's operators with the run id and the field: a build task `failed` whose logs come back empty (a log-capture gap); `Build status could not be reconciled` with a healthy runner and a build that finished on the cluster (the events path); a run still `running` long after its build finished with no reason anywhere (the record's own law says this cannot happen); a status that contradicts its stages. Never guess a cause the record does not state.

## When the person is on the run's page

The run page's "Help me fix it" opens you with the failure already stated in the engine's words; `references/service.fixing-a-failed-run.md` composes this taxonomy into that room's read order, adds the consequence beat ("is anything running because of this?"), and covers the repository's own GitHub Actions run read through Planton.

## Asked for → what you do

- **"Why did my build fail?"** — `get_service_pipeline`; read `status.status_reason` first; class 2/3/4/6 is answered from the record alone; class 5 is `get_service_pipeline_logs` for the failing task's lines, relayed verbatim — then the files the build actually used, at the run's commit: `read_repo_files` with the service and `ref` = `spec.git_commit.sha` (the Dockerfile and the lockfile it installs; the pipeline and its tasks), so the explanation quotes what the build saw, never what the default branch says today. When the log and the files do not settle it, clone at that commit and reproduce (`references/service.reading-a-repository.md`).
- **"Just rerun it"** — only when the fix is outside the commit (a credential, a runner, egress) or the failure was transient; a compile failure or a code failure needs a NEW commit, and say why.
- **"Is this my fault or Planton's?"** — the class decides: 2 and 5 (code) are the developer's, 3, 4, and the egress row of 5 are an administrator's, and the signals in the section above are the platform's.
- **"My build is stuck"** — `queued` means waiting for a runner (class 4 will name it within two minutes if none serves the connection); `running` past the build's normal length with no node advancing — read the graph, then the log so far; past an hour the record explains itself (class 6).
