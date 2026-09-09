# Connecting GitHub and a Container Registry — From the Sign-In the Machine Already Holds

Before a service can be registered, built, or pushed, the organization needs two connections: one to GitHub (to read and clone the repository) and one to a container registry (to push the image). Both come in two families, and the family decides what to offer, what works, and what to say when it does not. Read this when a person is about to register a service on a laptop or self-hosted server, when a build or module clone fails on a connection, or when they ask "how do I connect GitHub / my registry".

## The doctrine in one sentence

Where a sign-in already exists — on the machine running Planton, or in another connection the organization already trusts — Planton references it and stores nothing. Never propose pasting a token when a sign-in the machine holds would do.

## GitHub: two identities

| | Sign-in on this machine (`host_login`) | GitHub App (`platform_app` / `customer_app`) |
|---|---|---|
| Where it fits | A local instance (Planton Desktop on a laptop); a self-hosted server whose environment carries `GITHUB_TOKEN` | Hosted Planton; GitHub Enterprise Server with the customer's own App |
| What the record stores | The account login and the GitHub host — never a token; the control plane reads `gh`'s sign-in (or `GH_TOKEN` / `GITHUB_TOKEN`) at each use | The installation (and the customer App's references) |
| Clones, reads files, lists repositories, private IaC modules | Yes | Yes |
| GitHub starts runs on push (webhooks) | **No** — GitHub cannot call a machine it does not know; Planton **watches the repository** and hands new commits to the same run door | Yes |
| Check runs / commit statuses on pull requests | **No** — refused with one sentence before GitHub is called | Yes |
| Signing out | `gh auth logout` stops the connection at its next use, the same second | Tokens are minted from the App key |

Which family an instance offers is a fact about the deployment, read from the method catalog: a local instance offers the sign-in and never the Planton App; hosted Planton never offers the sign-in. Never suggest a family the instance would refuse — check what the connection wizard or `planton connect github detect` offers.

### Connecting GitHub on a machine

- **Desktop**: Connections → **GitHub** shows the detected card — *GitHub Account priya-dev · github.com · Signed in with gh* — and **Use This Account** writes the connection and proves it through the control plane (**Confirmed — GitHub attributes this sign-in to priya-dev**).
- **CLI**: `planton connect github detect` (add `--account <login>` when the machine holds several; `--yes` to skip the prompt; `--output json` to inspect). It writes `github.account.<login>` (slug `github-account-<login>`) and reads **GitHub Connection Ready** or the exact sentence to act on.
- **Not signed in yet?** Say the two ways: `gh auth login` (takes effect at once), or export `GH_TOKEN` in the shell profile and restart Planton (a shell token is read when Planton starts).
- **Proving it later**: **Verify Sign-In** on the connection's page answers a verdict — **Confirmed** with the account, or **Sign-In Not Working Yet** with the `gh auth login` to run. Prefer it to guessing: it is one live call.

### The GitHub sentences and what to do

Relay these verbatim; each names its remedy, and the fix is on the machine, never in the conversation.

- *This machine is not signed in to GitHub (`<host>`) — run `gh auth login`.*
- *GH_TOKEN / GITHUB_TOKEN is set but GitHub rejected it — it may be expired or revoked.*
- *The account `<login>` is not among this machine's sign-ins — `gh auth login` or `gh auth switch`.* (the connection names one account; a different `GH_TOKEN` is a different account)
- *The GitHub CLI is not on the instance's PATH — install it, or start Planton from a terminal.* (a Dock-launched app on a machine where `gh` is not where the login shell puts it)
- A capability refusal (*Runs on this connection start from Planton — GitHub can't call this machine*; check runs need an App): not a failure to fix — explain the family's limit and that pushes still arrive through the repository watch.

## Container registries: two credential arms

A registry connection is how Planton reaches a registry: builds push with it, and where its credential is a stored login a cluster can keep, clusters that cannot pull on their own pull with it too — by reference, never by copy. It describes how to sign in and never holds the result.

- **A connection you already trust** — the record names a sibling connection: a GitHub connection for **GHCR** (the sign-in must carry `write:packages`), an AWS connection for **ECR**, a GCP connection for **Artifact Registry**, an Azure connection for **ACR**. No key is stored; the runner derives a short-lived registry token from the sibling at the moment a build pushes. A GitHub **App** connection is never offered for GHCR — the trusted arm is a sign-in a machine holds. A token minted this way lasts minutes to an hour, so a cluster can never keep one: on this arm a Kubernetes deploy fills no pull login from the connection — except GHCR with a **pull token** (below).
- **Stored keys** — the record references organization secrets (a service account key, IAM keys, a service principal, a PAT). The arm for **JFrog**, and for any registry outside the connections the organization has. A stored login is also what a cluster can keep, so on this arm a Kubernetes deploy fills the workload's pull login from it — except ECR, whose tokens last twelve hours on every arm.
- **The GHCR pull token** — optional, with either arm: `spec.githubContainerRegistry.pullToken` is a bot account's `read:packages` token as an organization-secret reference. With it, builds push through the sign-in and clusters pull with the read-only token. Ask for it whenever a GHCR registry connected through a sign-in will serve a Kubernetes deploy — on the **You Already Trust These** card (**Add Pull Token**), the wizard's **Pull Token** step, or the connection page's **Pull Token** section. The whole pull story is `references/service.pulling-private-images.md`.

### Connecting a registry

- **Desktop and web console**: Connections → **Container Registry** opens on **You Already Trust These** — one card per registry the organization's connections reach (*GHCR as priya-dev · via GitHub connection github-account-priya-dev*; *ECR us-east-1 for 123456789012 · via AWS connection aws-profile-acme-dev*). A card missing a coordinate (a region, a repository, a registry name) asks for it on the card. **Use This Registry** writes the record, proves it, and lands on its page. **Set Up Manually** reaches the wizard, whose **Credential** step asks **A Connection You Already Trust** or **Stored Keys**.
- **CLI**: `planton connect registry detect` lists the same rows (the control plane owns the table — the CLI and the cards never disagree); `--from <connection-slug>` picks the row, `--region` / `--project` / `--repository` / `--registry` supply what the connection cannot know; `--yes` skips the prompt. It reads **Registry Connection Ready — ghcr.io accepted the credential derived from GitHub connection github-account-priya-dev**, or **Registry Not Working Yet** with the sentence.
- **Names** are the registry family's dotted shape — `ghcr.account.<login>`, `ecr.account.<id>.<region>`, `artifact-registry.<project>.<region>.<repo>`, `acr.<name>` — identical from the card and the CLI, so re-running never duplicates.
- **Proving it later**: **Verify This Registry** on the connection's page derives the credential on the runner that would push and asks the registry to accept it. When a person asks "will my build be able to push", this is the answer, before any build runs.
- A service names its registry in `spec.build.registry` by slug. When the organization has **no** registry yet, offer `planton connect registry detect` before asking anyone for a key (see `service.detection-first-registration.md`).

### The registry sentences and what to do

- *GitHub connection '<slug>' could not provide a credential for GHCR — <the GitHub sentence>* / *AWS|GCP|Azure connection '<slug>' could not sign in on this runner — …* — the trusted sibling is the problem; its own sentence follows the dash; fix the sibling, not the registry.
- *GHCR refused the GitHub sign-in behind connection '<slug>' — this sign-in can read code but not packages; run `gh auth refresh -s read:packages,write:packages`* — the scope, exactly as named.
- *<source> signs in to AWS account 111…, but this registry is in account 222… — point the registry at a connection for account 222…, or change its account id.*
- *Registry <host> refused the Azure identity … — it needs the AcrPush role* / *Google did not issue a token for GCP connection …* — the per-provider remedy is in the sentence.
- *Container registry connection 'X' signs in to GHCR through GitHub connection 'Y', which does not exist in organization 'Z' — create that connection first, or point this registry at one that exists.* — refused at input assembly, before any pod starts, and by Verify.

## What this means for how you work

- On a loopback instance (`planton instance current` shows `127.0.0.1` / `localhost`), the sign-in family IS the path: never walk a laptop user through installing a GitHub App or creating a PAT.
- When a run did not start on a push through a sign-in connection, the question is the repository watch, not a missing webhook.
- Reading a repository through any GitHub connection is `get_github_access_token` (`service.reading-a-repository.md`) — the token AND the git username come back together; use both exactly as given, and relay a signed-out sentence verbatim.
- Every connection page has a verify door. Use it — and tell the person to use it — before diagnosing a build or clone failure from its logs.
