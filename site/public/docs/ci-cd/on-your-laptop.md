---
title: "CI/CD on Your Laptop"
sidebar_title: "On Your Laptop"
description: "Push to GitHub and your own laptop builds the commit in a pod and deploys it to your cloud — with Planton Desktop, Docker Desktop, and the gh sign-in you already have. No Planton account, no public endpoint, no GitHub App, no YAML."
icon: rocket
order: 36
tags:
  - CI/CD
  - Planton Desktop
  - Laptop
  - Zero Configuration
---

# CI/CD on Your Laptop

Planton Desktop turns a laptop into a complete CI/CD control plane. You push a commit to GitHub, and the machine on your desk notices the push, builds the commit inside a pod on a build cluster it runs for you, pushes the image to your registry with your own sign-in, deploys to the cluster or cloud you connected, puts a status on your commit, and tells you when it is done. Nothing is hosted, nothing has a public address, and there is no configuration step on this page.

## Why this works with nothing hosted

Hosted CI starts from a webhook: GitHub calls a public URL when you push. A laptop has no public URL — GitHub refuses `localhost` as a webhook target and could not route to it anyway. Planton Desktop turns the direction around: your laptop's own control plane **asks GitHub** whether the branches it watches have moved, using conditional requests that cost nothing against your rate limit when nothing changed, and turns each push into exactly the same run a hosted webhook would have started. The same run view, the same rooms, the same rules.

The build itself runs in a **build cluster** — a small Kubernetes cluster Planton Desktop sets up inside Docker Desktop the first time you say yes, with the platform's own build tooling installed. It stops itself after ten minutes with no build and wakes again on your next push, so it costs nothing while you are not building. Your `gh` sign-in is what clones the repository inside the build pod and what pushes the image to GitHub Container Registry; it is handed to the build once, is never written into the source or a volume, and is deleted from the cluster the moment the run ends — whether the run succeeded, failed, or was cancelled.

Because everything downstream is the platform's ordinary delivery lane, a laptop deploys exactly as hosted Planton does: through the same modules, to the same targets, with the same rollout verification and the same URL on the environment card.

## What a laptop needs

- **Docker Desktop**, running. Planton Desktop detects it and never installs it; if it is not there, the enable moment says so and offers the download.
- **A `gh` sign-in** (`gh auth login`). That sign-in becomes your GitHub connection — it stores no token and stops working the second you sign out. To push images to GitHub Container Registry it needs the packages scopes: `gh auth refresh -s read:packages,write:packages`.
- **Somewhere to deploy**: any cluster in your kubeconfig (`planton connect kubernetes detect --runner local`), or a cloud connection made from the sign-in your machine already holds (see [Connections](/docs/connections)). Deploy targets from a laptop are your clusters and clouds — a laptop never deploys to itself.

That is the whole list. There is no Planton account to create and no YAML to write for the pipeline; the build method is detected from your repository (a Dockerfile, or Buildpacks when there is none — see [Build Methods](/docs/ci-cd/build-methods)).

## The first push, beat by beat

**1. Register the service.** The service wizard reads the repository through your connection, proposes the build method it found, and asks where the service deploys; you confirm. Or register from a `service.yaml` with `planton service register`. From this moment your laptop watches the repository's trigger branches; `planton service watch <service>` shows when GitHub was last checked and what it saw.

<!-- SCREENSHOT: the repository watch card
  Page: Planton Desktop → a service → Overview, the "last checked" line under the service name
  Action: a service whose repository is being watched, a few seconds after a check
  Focus: the freshness line ("Checked 12 s ago · next in 8 s") and the branch it follows
  Alt: The service page's repository watch line showing when GitHub was last checked
-->

**2. Say yes to "Build on this machine?".** Opening Service Hub on a laptop for the first time asks one question and states the cost in plain words. On yes, a timeline narrates the setup — checking this machine, downloading the build tools, creating the build cluster, installing the build tools, fetching the build images once — and ends at Ready. A developer who never registers a service never meets this question.

<!-- SCREENSHOT: the enable moment
  Page: Planton Desktop → Service Hub, on a local instance without a build cluster
  Action: Docker Desktop detected, before the person clicks Use This Machine
  Focus: the question, the detected-runtime card with the machine's budget, the trust footnote
  Alt: The "Build on this machine?" card showing Docker Desktop detected and the cost stated
-->

**3. Push.** Within the watch's cadence (twenty seconds, ten for two minutes after a change) your laptop notices the commit and a run appears. If the build cluster is asleep, the run wakes it first — the run view shows *Starting the build cluster…* as its own beat. The build clones the exact commit with your sign-in, builds it in a pod, and pushes the image to your registry.

**4. Deploy.** The run hands the image to the deploy stage, which applies the environment you declared to the cluster you connected, waits for the rollout, and reads back the URL from what the resources reported (an HTTPRoute's hostname, an Ingress host, a Cloud Run URL) or from the environment's serving domain. The environment card shows the URL and the rollout verdict; `planton service urls <service>` prints the same. When nothing carries an address, the card says so and names the kinds that would give it one.

**5. GitHub and your desk.** The run reports itself on the commit as a **commit status** named `planton/<service>` — *Succeeded — deployed to laptop-lab · built for linux/amd64 on arm64 — emulated* — so the pull request shows it. Your laptop shows a banner when the run ends, and the in-app card carries the run's verdict; both live under the desktop toggle in Notifications.

<!-- SCREENSHOT: the run view on a laptop
  Page: Planton Desktop → a service → Runs → a push-born run that deployed
  Action: the run completed, the build and deploy stages green, the platform sentence visible
  Focus: the build stage, the deploy stage's resources, the GitHub line and the platform line
  Alt: A completed laptop run showing the build in a pod and the deploy to the connected cluster
-->

## What it costs, measured

Every figure below was measured on an Apple M3 Max with Docker Desktop (8 CPUs and 24 GB given to containers), on the platform's own build cluster, with a small Node service. Your numbers will differ with your machine and your Dockerfile; the shape will not.

- **Setting up the build cluster, once:** about three minutes on an idle machine (the tooling download is most of it); about five when Docker is already busy with other clusters.
- **Waking a stopped cluster:** fifteen to eighteen seconds to ready for a build. Set-up happens once; every later push pays only the wake.
- **While up and idle:** about 800 MB of memory and a small fraction of one CPU core. **While stopped:** nothing.
- **During a build:** up to about a gigabyte more and a burst of CPU; a Dockerfile build of a small service takes thirty to forty-five seconds after the wake, and the image is on the registry about eighty seconds after the push.
- **Idle policy:** the cluster stops itself after ten minutes with no build. **Keep Warm** — in the health dashboard or `planton local build-cluster keep-warm on` — holds it up for people who build all day; quitting Planton Desktop always stops it.
- **Building for another architecture:** the build produces the platform your deploy target runs. When that differs from your machine (an Apple Silicon laptop building for an amd64 cluster), the build runs under emulation — about two and a half times slower per unit of CPU work — and the run says so as a fact: *built for linux/amd64 on an arm64 machine — emulated*. The Buildpacks track is always emulated on Apple Silicon, because its builder image is amd64-only.

> **Note:** The commit status is a *status*, not a *check*. GitHub's checks API refuses tokens that come from a person's sign-in; only a GitHub App can write checks. Statuses show on the pull request and gate branch protection the same way.

## The build cluster from the terminal

The desktop assistant runs on the same commands, so everything the health dashboard shows is a verb:

```bash
planton local build-cluster status        # phase, runtime, installed tooling, keep-warm, set-up time
planton local build-cluster start         # wake it now instead of on the next push
planton local build-cluster stop          # stop it now (refused while a build runs)
planton local build-cluster keep-warm on  # hold it up between builds (off to return to scale-to-zero)
planton local build-cluster remove        # remove it and its files; the next "yes" sets it up again
planton service watch <service>           # when GitHub was last checked and what it saw
planton daemon status                     # every component of the local instance, the build cluster included
```

## When something goes wrong

Every failure on this road says what the platform observed, what it most likely means, and the exact next step — the same words on the card, in the run, and in the terminal. The ones you are most likely to meet:

| You will see | What it means | What to do |
|---|---|---|
| *Planton couldn't find a running container runtime on this machine. Install Docker Desktop, or start it if it's installed, then try again.* | Docker Desktop is not installed or not running. | Start it (or install it) and try again; the card offers the download. |
| *Stopped — Docker Desktop isn't running, so the next build waits until it is.* | The build cluster exists but Docker is off, so a push cannot wake it. | Start Docker Desktop; the cluster wakes on your next push. |
| *The build cluster stopped unexpectedly — most likely the container runtime stopped.* | Docker stopped (or the machine slept through it) while the cluster was up. | Nothing — it starts again with your next build. |
| *The build cluster keeps stopping answering, even after Planton restarted and set it up again.* | Planton restarted it and rebuilt it under its own budget, and it still dies. | Read the build cluster log it points at; remove and set up again, or try later. |
| *GHCR refused the GitHub sign-in behind connection '…' — this sign-in can read code but not packages.* | Your `gh` token lacks the packages scopes, so the image cannot be pushed. | `gh auth refresh -s read:packages,write:packages`, then rerun. |
| *GitHub rejected this connection's sign-in while checking acme/storefront — the token is expired or revoked, or this machine signed out.* | The watch cannot read GitHub with your sign-in any more. | `gh auth status`, sign in again; checks resume on their own. |
| *GitHub can't see acme/storefront with this connection's sign-in — … a private repository needs the `repo` scope.* | The repository moved, or the token cannot read it. | Check the repository, or `gh auth refresh -s repo`; checks resume on their own. |
| *GitHub's hourly request limit for this account is nearly used up (N requests left) — checks pause until it refills in about N minutes, so your own `gh` keeps working.* | The watch reserves the last requests for you. | Nothing — pushes made meanwhile are picked up at the next check. |
| *Built for linux/amd64 on an arm64 machine — emulated, slower than a native build.* | Not a failure: your target's platform differs from your machine's. | Nothing, or set `build.target_platforms` on the service to override. |
| *No address to show — none of this environment's resources carries one.* | The deploy succeeded but nothing reported a URL. | Declare a serving domain on the environment, or include a resource that carries an address (the sentence names the kinds). |

Two things a laptop does honestly rather than silently: a run that fails to hand its build to the deploy stage ends **failed** with that step's own words within about a minute — never a stage that reads "queued" forever; and a laptop that was asleep through several pushes catches up on its next check — one run per branch that moved, carrying the whole range of commits since the last head it saw, never one run per missed push.

## Details

- **The trigger.** Your laptop's control plane runs a repository watch per GitHub connection that GitHub cannot deliver webhooks to. It asks for branch heads, open pull requests, and — when the repository has them — GitHub Actions runs, with conditional requests (a `304` costs nothing). Pull-request polling drives [preview environments](/docs/ci-cd/deployment-environments); Actions runs appear in the service's feed exactly as hosted mirroring shows them. Cadence: 20 s, 10 s for two minutes after a change, backing off to 5 min while GitHub refuses, 60 s when nothing is watched; a reserve of 200 requests is never spent.
- **The build cluster.** A [k3d](https://k3d.io) cluster (k3s in Docker) with Tekton Pipelines installed, pinned and shipped through Planton Desktop's runtime manifest, running the same open-source build content hosted Planton runs — byte for byte. It presents itself to the local instance as an ordinary build connection; the local runner builds in it, wakes it on demand, streams its logs, and receives its events over an authenticated loopback channel. It is judged alive by its own API, never by Docker's word, and heals itself under a bounded budget before it reports a failure.
- **The credential.** The build pod receives your sign-in's token through Tekton's credential initialization for the clone and the registry push only. The run's credential Secrets are deleted at the run's terminal on every road — success, failure, cancel. Nothing of yours is written into the source workspace, a persistent volume, or a log.
- **Loopback only.** Every listener the local instance opens binds `127.0.0.1`. The one channel from the cluster back to the host — the runner's event receiver — is reached through Docker's host gateway and authenticated by a per-process token; the bind address never opens.
- **The record is the same.** A service registered on a laptop is the same `Service` record as anywhere else — see [What is a Service?](/docs/ci-cd/what-is-a-service) and [Deployment Stage](/docs/ci-cd/deployment-stage). Move the record to a hosted or self-hosted instance and nothing about it changes.

## Where to next

- [Getting Started with Service Hub](/docs/ci-cd/getting-started) — registering your first service.
- [Git Providers](/docs/connections/git-providers) — the sign-in connection and what it can and cannot do.
- [Deploy from GitHub Actions](/docs/ci-cd/deploy-from-github-actions) — the other road with no hosted backend, when the build should run on GitHub's machines instead of yours.
- [Deployment Stage](/docs/ci-cd/deployment-stage) — how a URL is found after a deploy.
- [Secret Backends](/docs/secrets/backends) — the laptop's built-in secret store.
