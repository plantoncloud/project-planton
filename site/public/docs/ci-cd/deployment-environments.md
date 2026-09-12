---
title: "Deployment Environments"
description: "Which environments a service deploys to, how a push walks them, and how a branch can be pointed at exactly one."
icon: deployment
order: 50
tags:
  - Deployment
  - Environments
  - Service Hub
---

# Deployment Environments

A service deploys to the environments it declares configuration for, in your organization's promotion order. That is the right behavior for most services: a push to the trigger branch builds once and walks dev, then staging, then production, stopping at any protected environment until someone approves. Sometimes you need something narrower — a canary service that should only reach staging, a team that keeps one long-lived branch per environment and promotes by merging, a new service that should start in dev alone and grow. This page covers how the set of environments is decided and how a branch can be aimed at one of them.

<!-- VIDEO: Deployment environments walkthrough
  Source: Cloudflare Stream
  Alt: Video walkthrough showing a service's environments on the Configuration tab and a branch mapped to one environment
-->

## How It Works

Every service carries one configuration section with **one entry per environment it deploys to**. Each entry holds the environment's slug and the full manifests the service deploys there. When a pipeline reaches its deploy stage, it creates one deployment task per declared environment and runs them in the promotion order your environments define. An environment with no entry is not deployed to — there is no separate filter to maintain.

Who writes those entries depends on the service:

- **Platform-authored services** declare their environments on the Configuration tab (or in `service.yaml`). Add an environment and its resources, and the next run deploys there; remove its configuration and the next run does not.
- **Git-maintained services** (a `_kustomize` tree in the repository) get their entries from the repository: **the overlay set defines the environment set**. An `overlays/staging/` directory means the service deploys to staging; delete the overlay and the service stops deploying there on the next push. The platform syncs the rendered overlays onto the service record, and the record shows which branch and commit each environment's configuration came from.

Two sibling trees can sit beside the overlays without joining the environment set: `dev/<flavor>/` serves local development only (never deployed), and `previews/<env>/` holds the deltas a pull-request preview of that environment deploys with (rendered per run, never written onto the record).

<!-- SCREENSHOT: A service's environments on the Configuration tab
  Page: /orgs/{org}/service/{slug} (Configuration tab)
  Action: A git-maintained service with dev, staging, and prod entries, each panel headed by what is live there and listing the declared resources beneath, with the sync provenance (branch, commit, when) on each
  Focus: The three environment panels in promotion order
  Alt: Configuration tab showing one panel per environment the service deploys to
-->

## Choosing Which Environments a Service Deploys To

### Web Console

Open the service and go to **Configuration**. Each environment the service deploys to is one panel: the environment's name and what is live there at the top, the declared resources beneath. On a platform-authored service, **Add Environment** declares a new one and **Remove Configuration** on a panel stops deploying there; each declared resource has its own **Edit**. On a git-maintained service the panels are read-only and each carries **Open in Repository** — the overlay is where the change is made, and the next push brings it back.

### CLI

For a platform-authored service, the environment entries live under `spec.deploy.environments` in your service YAML:

```yaml
spec:
  deploy:
    environments:
      - env: dev
        resources:
          - apiVersion: kubernetes.planton.dev/v1alpha1
            kind: KubernetesDeployment
            # ...
      - env: staging
        resources:
          # ...
```

```bash
planton apply -f service.yaml
```

For a git-maintained service, edit the overlays and push — an apply cannot set or clear the synced entries, exactly as it cannot write a record's status.

## Branch-Based Targeting

By default a push to a trigger branch walks every declared environment in promotion order. Teams that keep a standing branch per environment can instead map a branch to exactly one environment. A push to a mapped branch builds and deploys into **that environment alone** — it never walks the promotion order — and inherits the environment's gates and protection. Promotion then happens the way those teams already work: by merging between branches.

A branch is either a trigger branch (walks everything) or a mapped branch (deploys to one environment), never both; a configuration that tries to make it both is refused when it is saved, naming the ambiguity. Mappings name a branch exactly — no patterns — because a mapping is a deliberate, auditable statement about a standing branch.

### Web Console

On the Configuration tab, the **Deploy Settings** panel's **Branch Deployments** row lists the mappings. **Edit** opens a dialog of branch → environment rows over the organization's environments; the environment must be one the service declares configuration for. If a branch you map is also in the trigger branches, the dialog says so above the field and offers a door to the Build Triggers dial to resolve it.

<!-- SCREENSHOT: Branch Deployments
  Page: /orgs/{org}/service/{slug} (Configuration tab)
  Action: The Deploy Settings panel with a Branch Deployments row reading "release-hotfix → prod" and its Edit dialog open
  Focus: The Branch Deployments row and the dialog's branch → environment rows
  Alt: Branch Deployments row on the Deploy Settings panel with its edit dialog
-->

### CLI

```yaml
spec:
  build:
    triggers:
      branches:
        - main            # walks dev → staging → prod
  deploy:
    branchDeployments:
      - branch: release-hotfix
        env: prod         # deploys to prod only
```

### Example: One Branch per Environment

```yaml
spec:
  build:
    triggers:
      branches: []        # no branch walks the whole order
  deploy:
    branchDeployments:
      - branch: dev
        env: dev
      - branch: staging
        env: staging
      - branch: release
        env: prod
```

With this setup:
- Push to `dev` → deploys only to dev
- Push to `staging` → deploys only to staging
- Push to `release` → deploys only to production

For a git-maintained service a mapping is also the **sync authority** for its environment: the mapped branch's push writes that environment's entry from its own tree, so an overlay that exists only on that branch lands on the record marked for its environment.

## Use Cases

### Branch-Based Deployments Without Merge Conflicts

Teams that keep separate branches per environment used to fight over per-environment directories at merge time. With branch mappings, every branch can carry the whole `_kustomize` tree: which overlay actually deploys is decided by the branch the push came from, not by what the tree contains.

### Progressive Rollout

Start a new service with one environment and add the rest as confidence grows — on a platform-authored service, Add Environment on the Configuration tab; on a git-maintained one, add the overlay directory and push. Nothing else changes: the walk includes the new environment at its promotion rank on the next run.

### Environment-Specific Services

Some services should never deploy everywhere — debug tooling in dev only, internal monitoring in the non-production environments. Declare configuration for exactly those environments and no others; the deploy stage has nothing to do for the rest.

## How It Appears in Pipelines

A run's page draws the pipeline as a strip: Build, then one tile per environment the run deploys to, in promotion order. Environments the service does not declare configuration for do not appear, and a push to a mapped branch shows exactly one environment tile.

<!-- SCREENSHOT: Pipeline run page stage strip
  Page: /orgs/{org}/service/{slug}/runs/{runId}
  Action: A run from a push to a mapped branch — the strip shows Build and one environment tile
  Focus: The stage strip under the run's header
  Alt: Run page whose stage strip shows Build and a single environment
-->

## Troubleshooting

### An Environment Is Not Deploying

- **Check the Configuration tab** — the environment must have a panel. On a git-maintained service, that means an `overlays/<env>/` directory on the branch that drives the environment.
- **Check Branch Deployments** — if the push came from a mapped branch, only that branch's environment deploys.
- **Check the environment exists in the organization** — an entry names an environment by its slug, and the slug must exist.

### A Push Deploys to One Environment When You Expected the Walk

The branch is mapped under Branch Deployments. Remove the mapping to let pushes on that branch walk the promotion order — or add the branch to the trigger branches after removing the mapping; it cannot be both.

## Preview Environments per Pull Request

Beside the durable environments you configure, the platform mints **preview environments** on its own: when a pull request opens on a service with pull-request previews enabled, a real, short-lived environment named `{service}-pr-{number}` is born from the environment the PR's target branch deploys to, and the changed service deploys into it alone. Rollout verification stamps a working URL for the pull request's own copy of the service.

Three things to know about them:

- **They manage their own lifecycle.** A preview is created by the pull request and destroyed by it — closing the PR (merged or not) tears down its cloud resources and records, and an untouched preview expires on its own (72 hours by default, tunable per service). Deleting a preview environment by hand is refused; closing the pull request is the delete button.
- **They never join promotion order.** Your dev → staging → production walk is unchanged no matter how many previews exist.
- **They are capped.** At most five previews per service exist at once; a pull request beyond the cap still builds, and its deploy explains the skip.

Check any pull request's preview — its phase, verified URL, and rollout verdict — with `planton service previews <service> --pr <n>`.

## Related Documentation

- [Deployment Stage](/docs/ci-cd/deployment-stage) — How manifests are resolved and deployed, including the Kustomize model
- [Deployment Targets](/docs/ci-cd/deployment-targets) — Supported platforms and Git-based vs inline configuration
- [What is a Service?](/docs/ci-cd/what-is-a-service) — Service configuration overview
- [Pipelines](/docs/ci-cd/pipelines) — The pipeline execution model, including pull-request previews
