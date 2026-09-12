# Draft 1 — Planton Desktop: landing page (`/desktop`) and install page (`/download`)

**Date:** 2026-09-11
**Status:** Approved for implementation by the founder's plan approval; open to redlines.

Every factual claim below carries a bracketed trace to where it was verified in the product. Nothing here is aspirational. Numbers that were never measured (total disk footprint, cold first-run time) are deliberately absent.

Voice: direct, concrete, a little proud, never breathless. Title Case for headings and buttons; sentence case for prose. Real em dashes. The reader is an engineer who already has a cloud account and a coding agent open.

---

## Page 1 — `/desktop` (the landing page)

### Hero

**Badge:** Free Forever

**Headline (direction 1, recommended):**
Your coding agent can already create cloud infrastructure. Planton makes it verifiable, recorded, and reusable — in your account, on your laptop, free.

**Alternate headline (direction 2):**
Give your coding agent rails you can inspect.

**Subhead:**
The whole Planton platform runs on your laptop and deploys to your cloud with the logins already on your machine. No account. Free for individuals, including commercial use.
[trace: local runtime is the full control plane + runner as native processes — `planton.architecture.local-runtime-plantond.md`; ambient-login detection — `planton.domain.connect.local-cloud-credential-autodetect.md`; no account — `planton.architecture.desktop-identity.md` law 1 and 3; free incl. commercial — recorded decision, `wiki/product.desktop.feature-availability.md`]

**Primary CTA:** Download for macOS  *(platform-aware: "Download for Windows", "Download for Linux"; falls back to "Download Planton Desktop")*
**Secondary CTA (copyable):** `brew install plantonhq/tap/planton-desktop`
**Under the CTAs, one line:** macOS 12 or newer, Windows 10 or newer, Linux with glibc 2.35 or newer. Signed and notarized on macOS.
[trace: tauri.conf `minimumSystemVersion` 12.0; Ubuntu 22.04 build image → glibc 2.35; notarization + stapling — `wiki/product.release-pipeline.md`]

**Hero visual:** screenshot of the local home — the single prompt "What do you want to build?" (session 2)

---

### Section 1b — Two ways you already do this, and what each costs you

**Section title:** You Already Have Two Ways to Do This

**Left column — A platform that hides the cloud**
Convenience, in exchange for control. Your app runs in someone else's account, under IAM you did not write, on a bill you cannot itemize. When you outgrow it, you start over.

**Right column — Your agent and the cloud CLI**
Control, in exchange for a record. The agent writes the Terraform from memory and you learn at apply time what it got wrong. Permissions nobody derived. A cost you find out about next month. A database password that went through the chat to reach the shell. No pipeline. The second environment is a second conversation.

**The third way (one line under both columns):**
Planton is a platform too — it just runs on your machine, in your account, and gives the agent rails you can inspect.

**The concession (small, muted, and load-bearing):**
For a one-off bucket, the agent alone wins. Planton earns its place on anything you will still be running in a month.

---

### Section 2 — Two Ways to Ask, One Craft

**Section title:** Two Ways to Ask. One Craft.
**Subtitle:** Ask from inside the tool you already use, or ask the assistant built in. Both run the same skills.
[trace: `/docs/coding-agents` — "The same skills power the Planton Assistant in Planton Desktop and the web console"]

**Left column (leading) — Your Coding Agent**
Install the Planton skills once. From then on Cursor, Claude Code, or Codex composes infrastructure grounded in the real schema, validates it offline, and applies it through the platform on your laptop — while you stay in your editor.

```
npx skills add plantonhq/skills
```

What Planton adds to the agent you already use:

- **A typed catalog, not memory.** 700+ resource kinds with validation rules. `planton explain` and `planton validate` run offline, so a wrong field fails before it touches your cloud.
  [trace: `planton.domain.infra-hub.planton-cli.md` (explain is offline, schemas compiled in); `/docs/coding-agents` (validate offline, no account); count from `PLATFORM_STATS.DEPLOYMENT_MODULE_COUNT`]
- **Verified before created.** The monthly cost from the providers' own price documents, stating exactly what it covers. The least-privilege permission policy derived from what is being composed, downloadable per provider.
  [trace: cost odometer + Runner Policy panel — `planton.domain.infra-hub.infra-chart.md`]
- **A record, not a transcript.** Every deploy is a stack job with a live log and a revision history. State lives under a path you can `ls`. The Infrastructure Map answers "where did this come from."
  [trace: server-minted revisions — desktop GTM changelog 069; state under `iac/state/` — `wiki/product.desktop.feature-availability.md`; Infrastructure Map — `planton.architecture.estate.md`]
- **Secrets the agent never reads.** Encrypted in the local database, key in your OS keychain, resolved on the runner at the moment of use. The agent references a secret by name; it never sees a value.
  [trace: `planton.architecture.security.just-in-time-secret-resolution.md`; local backend — `planton.architecture.local-runtime-plantond.md`]
- **Push-to-deploy, which no agent gives you.** Your laptop watches GitHub, builds the commit in a pod, deploys to your cloud, and writes the status back to the commit. No public URL, no GitHub App, no pipeline YAML.
  [trace: `/docs/ci-cd/on-your-laptop`; `planton.architecture.local-runtime-plantond.md` (polling watch, `GITHUB_WEBHOOKS_REACHABLE=false`)]
- **What you built becomes a template.** An Infra Chart redeploys into the next environment with a click or a sentence. Agent commands do not compose. Charts do.
  [trace: `planton.domain.infra-hub.infra-chart.md`; `POSITIONING.infraHub.line`]
- **When you become a team, nothing is redone.** Same manifests, same model, on planton.ai or your own cluster.
  [trace: "one model, no forks" — `planton.architecture.local-runtime-plantond.md`; `wiki/product.desktop.feature-availability.md`]

**Right column — The Built-In Assistant**
Open the app and ask. The Planton Assistant converses with zero keys and zero accounts — Planton funds it. Prefer to keep everything on your machine? Bring your own Anthropic or Cursor key in the engine chooser, and the conversation and the model traffic never leave the laptop.
[trace: zero-key hosted assistant — `_projects/.completed/20260801.01.desktop-zero-key-ai`, `planton.architecture.desktop-identity.md`; BYO key providers Anthropic/Cursor only, 0600 `ai.yaml` — `planton.stigmer.technical-integration.md`]

**Closing line:** Your agent and the platform's own assistant share one craft — the same skills, the same catalog, the same rules about what never happens without your say-so.

---

### Section 3 — What Actually Runs on Your Machine

**Section title:** What Actually Runs on Your Machine
**Subtitle:** A full platform as ordinary processes. No containers to babysit.

Four short tiles:

- **The control plane and one runner.** The same control plane that serves planton.ai, running as a local process, with a single runner that is ready the moment the app is — no slug to name, no registration, no credentials to hand over.
  [trace: `wiki/product.desktop.feature-availability.md` (one runner, no ceremony); "same control-plane application" — `planton.architecture.local-runtime-plantond.md`]
- **Postgres, Temporal, and a cache — as native processes.** Downloaded once on first launch, verified, and supervised by the app. Docker is not required.
  [trace: native `pg_ctl`, Temporal dev-server, Valkey — `planton.architecture.local-runtime-plantond.md`; runtime manifest — `planton.architecture.runtime-artifact-distribution.md`]
- **OpenTofu and Pulumi as your tools.** Planton honors the `tofu` or `pulumi` already on your PATH. Only when nothing usable exists does it install one — through a version manager you already use, or from the tool's official distribution, checksum-verified.
  [trace: `planton.architecture.local-iac-toolchain.md`]
- **Your data stays here.** Secrets encrypted in the local database with the key in your OS keychain. State on your disk. Logs on your disk. Local data never leaves the laptop.
  [trace: `planton.architecture.desktop-identity.md` law 2 ("Local data never leaves the laptop" is product copy); KEK in keychain — `planton.architecture.local-runtime-plantond.md`]

---

### Section 4 — Infra Hub on Your Laptop

**Section title:** Infra Hub on Your Laptop
**Eyebrow (the hub's one analogy, allowed here only):** Cursor for Cloud Infrastructure
[trace: `POSITIONING.infraHub.analogy`]

**Lead:** Describe what you need, watch it compose on a live canvas, see the cloud bill and the IAM policy before anything is created, deploy, and publish it as an Infra Chart — a template your team reuses.
[trace: `POSITIONING.infraHub.line`, verbatim]

Three proof points:

- **Your clouds, found, not pasted.** Planton reads the cloud logins already on your machine — AWS profiles, gcloud configurations, az subscriptions, kubeconfig contexts — and turns each into a ready connection. Cloudflare and DigitalOcean tokens are detected from your environment and verified against the provider before they are offered. You never paste a key into a form.
  [trace: `planton.domain.connect.local-cloud-credential-autodetect.md`]
- **700+ resource kinds across 8 providers.** The full catalog, seeded into your local instance at first boot.
  [trace: `PLATFORM_STATS`; catalog seeding — desktop GTM changelogs 008/011]
- **A map of everything you run.** Accounts, environments, projects, resources, and the services that span them — one living picture, drill-down to each project's diagram.
  [trace: `_projects/.completed/20260819.04.infrastructure-map/README.md`]

**Visual:** screenshot of Planton Studio's canvas with the cost odometer and the Runner Policy panel (session 2)

---

### Section 5 — Service Hub on Your Laptop

**Section title:** Service Hub on Your Laptop
**Eyebrow (the hub's one analogy, allowed here only):** Vercel for Backend, In Your Own Cloud
[trace: `POSITIONING.serviceHub.analogy`]

**Lead:** Push to GitHub. Your laptop notices, builds the commit in a pod, deploys it to the cluster or cloud you connected, and puts a status on the commit. No public URL, no GitHub App, no pipeline YAML — the local instance asks GitHub with your own `gh` sign-in.
[trace: `/docs/ci-cd/on-your-laptop`]

**The measured record (a `MetricsStrip`):**
- 35 s — push to running
- 30 s — build
- 34 s — deploy
- ~800 MiB — build cluster at rest
[trace: green record and idle measurement — `_projects/.completed/20260904.01.sp.desktop-service-hub-with-polling/changelogs/011-…` and `proof/substrate-spike/measurements/04-idle-slim.txt`; caption must say "measured on an M-series Mac"]

**The plain note (muted, not hidden):** Building on your own machine needs Docker Desktop. Planton detects it and never installs it. Everything else on this page runs without Docker.
[trace: `_projects/.completed/20260904.01.sp.desktop-service-hub-with-polling/README.md`]

---

### Section 6 — The Same Planton Everywhere

**Section title:** The Same Planton Everywhere
**Lead:** This laptop, planton.ai, or your company's cluster — pick where in the instance switcher. One data model, one set of code paths; only which capabilities run differs.
[trace: `wiki/product.desktop.feature-availability.md`; `planton.architecture.desktop-identity.md` (WHERE vs WHO)]

**One more line:** Your laptop can also deploy into your team's Planton, with your cloud credentials still never leaving your machine.
[trace: personal machine runner coexisting with the local runner — `planton.architecture.runner.md`, `planton.domain.connect.local-cloud-credential-autodetect.md`]

---

### Section 7 — Why It Is Free

**Section title:** Why It Is Free
**Body:**
Planton Desktop is free for individuals, including commercial use. Not a trial, not a tier with the good parts removed — the full catalog, the wizards, the IaC pipelines, service CI/CD, and secrets management are the core product on every edition.
[trace: recorded decision (carry; do not relitigate); breadth-is-free — `planton.pricing-and-business-model.md`]

Here is the business model, stated plainly: Planton makes its money when a team adopts it — hosted seats beyond the free tier, self-hosted licenses for larger teams, and prepaid AI credits. The solo developer is not a funnel stage. You are the person this was built to delight, and if you one day bring a team, nothing you built gets redone.
[trace: `pricing.ts` constants — `FREE_TIER_SEATS`, `SELF_HOSTED_LICENSE_SEAT_CEILINGS`, `MARKETS.*.creditPackStart`; the displayed sentence must read the constants]

---

### Section 8 — Final CTA

**Title:** Install It. Ask It Something.
**Body:** Download, open, pick "Run on this computer," and ask for what you need in your own words. Or install the skills and ask from your editor.
**Primary CTA:** Download Planton Desktop → `/download`
**Secondary links:** CI/CD on your laptop (`/docs/ci-cd/on-your-laptop`) · Coding agents (`/docs/coding-agents`)

---

## Page 2 — `/download` (the install page)

### Header

**Badge:** Free Forever
**Title:** Download Planton Desktop
**Subhead:** Free for individuals, including commercial use. No account, no sign-up. Pick your platform; the rest is one launch.
**Version line (only when known):** Latest: v0.0.56  *(from the build-time pointer; refreshes in the browser once the bucket permits it)*
[trace: `desktop/latest/version.txt`]

### The platform chooser

The detected platform's card is promoted to the top with the primary button; the other two follow. A "Not your platform?" switch is always visible.

**macOS card**
- **Button:** Download for macOS
- **Sub-line:** Universal — Apple Silicon and Intel · macOS 12 or newer · 158 MB
- **Alternative:** Or with Homebrew, which also installs the `planton` CLI:
  `brew install plantonhq/tap/planton-desktop`
- **Install note:** Open the disk image and drag Planton to Applications. The app is signed and notarized by Apple, so it opens without a warning.
[trace: universal DMG, size from live CDN headers (`content-length: 157797644`); cask `depends_on formula: "planton"`; notarized + stapled — `wiki/product.release-pipeline.md`]

**Windows card**
- **Button:** Download for Windows
- **Sub-line:** Installer — x64 · Windows 10 or newer
- **Install note:** Windows SmartScreen may show "Windows protected your PC" because the installer is not yet code-signed. Choose **More info**, then **Run anyway**. Auto-updates are signed by Planton and verified by the app.
[trace: no Authenticode step in the Windows build job; updater minisign key — `tauri.conf.json` `plugins.updater.pubkey`]
- **Availability:** rendered only when the platform's `available` flag is true (see handoff)

**Linux card**
- **Button:** Download AppImage
- **Second button (secondary):** Download .deb
- **Sub-line:** x86_64 · glibc 2.35 or newer — Ubuntu 22.04, Debian 12, Fedora 38, RHEL 9, or newer
- **Install note:** For the AppImage, make it executable and run it: `chmod +x planton-desktop-linux-amd64.AppImage`. For Debian and Ubuntu, install the .deb with your package manager.
[trace: Ubuntu 22.04 build runner → glibc floor; alias filenames — publish workflow]

### After You Install

**Section title:** After You Install
**Lead:** One launch does the rest.

Three numbered beats:

1. **Pick where Planton runs.** First launch asks one question — this computer, planton.ai, or a self-hosted deployment. Choose **Run on This Computer**. No account is asked for, then or later.
   [trace: three-option chooser, no account wall — `planton.architecture.desktop-identity.md`]
2. **Watch it set itself up.** The app downloads its runtime once — a Java runtime, Postgres, Temporal, a cache, the control plane — a few hundred megabytes with a real progress bar, verified against the release's checksums. Later upgrades fetch only what changed.
   [trace: `planton.architecture.runtime-artifact-distribution.md` (SHA-256, concurrent, content-addressed hardlinks); pins in `runtime-deps.pins.json`]
3. **Your clouds are already there.** Planton finds the AWS, Google Cloud, Azure, and Kubernetes sign-ins on your machine and offers each as a ready connection. Then ask for what you need.
   [trace: `planton.domain.connect.local-cloud-credential-autodetect.md`]

**Then, three tabs (`CodeTabs`):**

**Tab: Your Coding Agent**
```
# Install the Planton skills into Cursor, Claude Code, Codex, and any other agent
npx skills add plantonhq/skills

# Then, in your editor, ask:
# "I need a Postgres database for this service in dev."
```
Your agent composes the manifest from the real schema, validates it offline, and applies it through the platform on this laptop — with one confirmation from you.
[trace: `/docs/coding-agents`]

**Tab: The CLI**
```
# Installed with the Homebrew cask. From a direct download, Planton offers to install it for you.
planton --version

# Schema lookups and validation work offline, with no account
planton explain aws-vpc
planton validate -f infrastructure/
```
If you installed from the disk image, the app places `planton` in `~/.local/bin` and shows the one line to add to your shell if that folder is not on your PATH.
[trace: `PlantonCliController` detect-first ensure, `PathVisible`, consent-gated PATH line — release-pipeline wiki and desktop GTM changelog 061]

**Tab: Build Services Here (Optional)**
```
# Only needed to build your own services on this machine.
# Everything else on this page runs without Docker.
planton local build-cluster status
```
Say yes to "Build on this machine?" once, with Docker Desktop running. Planton creates a small build cluster inside it, sleeps it after ten idle minutes, and wakes it on your next push.
[trace: `/docs/ci-cd/on-your-laptop`; `planton.architecture.local-runtime-plantond.md`]

### Verify and Update

**Section title:** Verify the Download
**Body:** Every release publishes checksums. On macOS you can also ask Gatekeeper and the notarization ticket directly:
```
spctl --assess --type open --context context:primary-signature -v ~/Downloads/planton-desktop-universal-macos.dmg
xcrun stapler validate /Applications/Planton.app
```
**Link:** Checksums for v0.0.56 → `desktop/v0.0.56/checksums.txt` (only when the version is known)

**Section title:** Updates
**Body:** The app checks for updates and installs them with one click. On Homebrew, `brew upgrade planton-desktop` does the same. Updates are signed by Planton and verified before they are applied.
[trace: updater endpoint `desktop/latest/update.json`; `desktop-update-discovery` changelog 061]

### Footer strip

**Line:** Want the platform without the app? The CLI installs on its own — `brew install plantonhq/tap/planton` — and the self-hosted edition runs on your own cluster. → `/docs/cli`, `/docs/self-hosting`
