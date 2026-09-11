---
title: Building on Your Laptop — What a Local Instance Does With a Push
description: How a service builds and deploys from a laptop running Planton Desktop — the build cluster and its verbs, the repository watch as the reason a run starts, the sign-in as the only credential, the commit status instead of a check, the desktop notification, the measured costs, and every laptop failure with its next step.
---

# Building on Your Laptop — What a Local Instance Does With a Push

On a local instance (Planton Desktop; `planton instance current` shows `127.0.0.1` / `localhost`), a push to GitHub becomes a build in a pod on the person's own machine and a deploy to the cluster or cloud they connected — with nothing hosted, no public endpoint, no GitHub App. Everything downstream is the ordinary delivery lane, so `references/service.reading-a-run.md`, `references/service.build-failures.md`, and `references/service.urls-and-rollout-verification.md` apply unchanged. This reference is the LAPTOP layer on top: what is different about where the run starts and where it builds, which verbs answer laptop questions, and which sentences are laptop sentences.

## The doctrine in three sentences

The trigger is the **repository watch**, not a webhook — the laptop asks GitHub whether watched branches moved and hands each push to the same run door hosted uses; a run "not starting on a push" on a laptop is a watch question, never a missing webhook. The build runs in a **build cluster** the local daemon keeps inside Docker Desktop — an offer the person accepted once ("Build on this machine?"), never a first-run cost — that stops itself after ten idle minutes and wakes on the next push. The only credential is the person's **`gh` sign-in**: it clones inside the pod and pushes the image, it is never written anywhere that outlives the run, and it is deleted from the cluster at the run's terminal on every road.

## What a laptop needs, and how to check each

| Need | Check | If missing |
|---|---|---|
| Docker Desktop running | `planton local build-cluster status` — the **Runtime** row names it; a stopped cluster with *Docker Desktop isn't running, so the next build waits until it is* means it is off | Start Docker Desktop; the cluster wakes on the next push. Never propose another runtime install when Docker Desktop is installed and merely stopped. |
| A `gh` sign-in with the packages scopes | `gh auth status`; a build refused before any pod with *GHCR refused the GitHub sign-in behind connection '…' — this sign-in can read code but not packages* | `gh auth refresh -s read:packages,write:packages`, then `planton service rerun <run>` |
| A build cluster | `planton local build-cluster status` — **Not Set Up** means the person has not said yes | Point at Service Hub's enable moment in the desktop, or `planton local build-cluster enable` (the CLI arm of the same consent) |
| Somewhere to deploy | a runner-mode Kubernetes connection (`planton connect kubernetes detect --context <ctx> --runner local`) or a cloud connection from the machine's sign-in; and BOTH an authorization and a default for the environment (an authorization says what MAY be used, a default says what IS used — one without the other routes nothing) | `planton connection auth create --provider kubernetes --connection <slug> --scope environment --environments <env>` and `planton connection default set --provider kubernetes --connection <slug> --env <env>` |

Deploy targets from a laptop are the person's clusters and clouds only. There is no "deploy to my laptop" — never propose a local deploy target, a local registry, or a `localhost` URL story.

## The verbs (the desktop assistant runs on these)

```bash
planton local build-cluster status        # phase · runtime · cluster name · installed tooling · keep-warm · set-up time
planton local build-cluster enable        # the consent, from the terminal
planton local build-cluster start | stop  # wake now / stop now (stop is refused while a build runs — the sentence says so)
planton local build-cluster keep-warm on|off
planton local build-cluster remove        # the cluster and its files go; the next consent sets it up again
planton service watch <service>           # the watch: status, repository, connection, "Checked N s ago · next in M s", branches, PRs, tags, Actions, the GitHub request budget
planton daemon status                     # every local component, and "build cluster: <phase>"
planton follow <run>                      # the run live: the wake as its own beat, the build, the deploy, the GitHub line, the platform line
planton service urls <service>            # the environment card's truth: URL and rollout verdict, or why there is no address
```

Reading `planton local build-cluster status`: **Not Set Up** (no cluster, no cost) · **Setting Up · <note>** (the ladder is narrating) · **Ready · idle for N minutes** / **Building · N builds running** · **Stopped — starts in about fifteen seconds when a build is dispatched** (asleep, costs nothing; the estimate is this machine's own last start) · **Stopped — <cause>** (an unexpected stop, or Docker off — the cause is the sentence) · **Failed · <sentence>** (Try Again in the desktop or `enable` again; the sentence names the next step).

## What a push does, in order (so you can say where it is)

1. The watch notices the branch head within its cadence (20 s; 10 s for two minutes after a change; backing off to 5 min while GitHub refuses; a reserve of 200 requests is never spent). `planton service watch` shows the last check and the head it knows.
2. One run per branch that moved, carrying the whole commit range — a laptop asleep through five pushes wakes to ONE run for that branch, not five.
3. If the cluster is stopped, the run wakes it first (fifteen to eighteen seconds, measured). `planton follow` shows the beat.
4. The clone at the exact commit with the sign-in; the build in a pod; the push to the registry with the credential derived from the same sign-in.
5. The build declares the platform its deploy target runs. On Apple Silicon building for an amd64 target the run states *Built for linux/amd64 on an arm64 machine — emulated, slower than a native build* — a fact, not a failure (about 2.5× slower per unit of CPU work, measured). `build.target_platforms` on the service overrides. The Buildpacks track is always emulated on Apple Silicon (its builder image is amd64-only).
6. The deploy stage applies the declared environment to the connected cluster; the URL is read back from what the resources reported or from the serving domain (`references/service.urls-and-rollout-verification.md`).
7. GitHub gets a **commit status** `planton/<service>` from the sign-in — never a check run (the checks API refuses user tokens; only an App can write checks). Its description is the run's verdict sentence. `planton follow` prints the GitHub line, orange when it needs the person.
8. The laptop's owner is notified: a macOS banner and an in-app card, under the `desktop` toggle; `planton notifications list` reads the same feed.

## Measured costs (say these; never invent numbers)

Measured on an Apple M3 Max with Docker Desktop (8 CPUs / 24 GB to containers), a small Node service: set-up once ≈ 3 min idle machine, ≈ 5 min under load · wake 15–18 s · up-and-idle ≈ 800 MB and a fraction of one core · stopped = nothing · a Dockerfile build 30–45 s after the wake, image on the registry ≈ 80 s from the push · emulated amd64 ≈ 2.5× slower. When the person's machine differs, say so and point at `planton local build-cluster status`'s **Set-Up Time**, which is their own measurement.

## Laptop failures — the sentence, the meaning, the next step

Relay the sentence verbatim (they are stable), then the next step. All of them live on the run, the card, and the terminal alike.

| Sentence (verbatim) | Means | Next step |
|---|---|---|
| *Planton couldn't find a running container runtime on this machine. Install Docker Desktop, or start it if it's installed, then try again.* | No runtime | Start or install Docker Desktop; Try Again |
| *Docker Desktop isn't running, so the next build waits until it is. Start Docker Desktop and the cluster starts again with your next push.* | The cluster exists; Docker is off | Start Docker Desktop; nothing else |
| *The build cluster stopped unexpectedly — most likely the container runtime stopped. Start Docker Desktop and the cluster starts again with your next build.* | Docker stopped under a running cluster | Nothing; the next push wakes it |
| *The build cluster stopped unexpectedly — its container exited while the container runtime kept running. It starts again with your next build; if it keeps happening, the build cluster log has the details.* | The node died on its own | Nothing once; read the log if it repeats |
| *Restarting the build cluster — it stopped answering…* / *Setting the build cluster up again — it didn't recover from a restart…* | Self-healing under its budget (a confirmed death, never a readiness dip) | Wait; it narrates |
| *The build cluster keeps stopping answering, even after Planton restarted and set it up again. …* | The recovery budget (3 per hour) is spent | Read the log it names; `remove` and `enable` again, or later |
| *A build is running. Stop is available when it finishes, or cancel the run first.* | Stop refused | Wait or `planton service cancel <run>` |
| *GHCR refused the GitHub sign-in behind connection '…' — this sign-in can read code but not packages; run `gh auth refresh -s read:packages,write:packages`, then check again.* | Token lacks packages scopes; refused before any pod | The `gh auth refresh`, then rerun |
| *GitHub rejected this connection's sign-in while checking <repo> — the token is expired or revoked, or this machine signed out. Run `gh auth status` and sign in again; checks resume on their own.* | The watch cannot read GitHub | `gh auth status`, `gh auth login` |
| *GitHub can't see <repo> with this connection's sign-in — the repository was renamed or deleted, or the token has no access to it (a private repository needs the `repo` scope). …* | Repository moved or unreadable | Check the repository; `gh auth refresh -s repo` |
| *GitHub's hourly request limit for this account is nearly used up (N requests left) — checks pause until it refills in about N minutes, so your own `gh` keeps working. Pushes made meanwhile are picked up at the next check.* | The reserve held back | Nothing; the watch resumes |
| *provider connection kubernetes/<slug> is not authorized for environment <env>* | An authorization is missing (a default alone does not authorize) | `planton connection auth create …` for that environment, then rerun |
| *No address to show — none of this environment's resources carries one. Declare a serving domain on the environment, or include a resource that carries an address (…)* | The deploy succeeded; nothing reported a URL | A serving domain, or an address-carrying kind |

A run that fails to hand its build to the deploy stage ends **failed** with that step's own words within about a minute; a deploy stage that reads `queued` for longer than that on a laptop is a defect to report, never a state to wait on.

## What never to propose on a laptop

- A GitHub App, a webhook URL, a tunnel, or a PAT pasted into a connection — the sign-in family is the path.
- A pull secret or any credential of the person's placed into a cluster to make a deploy work — the sign-in enters a build pod for the clone and push only.
- A local deploy target, a local registry, or a `localhost` URL.
- Turning off scale-to-zero "to make builds faster" without saying the cost — Keep Warm is the person's choice, stated as ≈ 800 MB held.
