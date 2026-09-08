---
title: Reading and Changing a Service's Deployment Configuration
description: What a service is DECLARED to deploy per environment and who writes that declaration — reading deploy.environments honestly, the two writers (a git kustomize tree vs platform-authored), editing through get-then-apply without destroying authored content, which resources belong on the service and which on an infra project (and how to move one across), adding and removing target environments, and the deployments on/off switch. Read when someone asks what an environment is configured to run, wants more memory or a new variable on a running service, wants a new target environment, asks why the service shows no URL while a route fronts it elsewhere, or asks why deployments stopped happening.
---

# Reading and Changing a Service's Deployment Configuration

Every service's per-environment deployment configuration lives in ONE place on the record: `spec.deploy.environments` — one entry per target environment, each carrying the full cloud-resource manifests that deploy there. There are not two kinds of configuration; there are two kinds of WRITER, and knowing which one writes a given service is the first fact to establish before promising any change.

## The two writers, and how to tell them apart

Read the service (`get_service`) and look at `spec.deploy`:

- **`kustomize` present** — the repository's kustomize tree maintains the environments section, and the platform's sync is its ONLY writer. Each entry carries `sync` provenance: which branch, which commit, when. **A direct write to this section does not error — it is silently preserved-over**: apply succeeds and the stored entries stand exactly as they were. Never edit a git-maintained service's environments through apply; the honest answer is "this service's configuration lives in its repository — edit the kustomize tree" (see `references/service.kustomize-authoring.md`, including taking authorship back with eject).
- **`kustomize` absent** — the section is platform-authored: the console, an agent, or a direct apply writes it. This is the posture the rest of this file is about.

Everything else on the deploy block stays caller-authored on EVERY service, git-maintained included: `disable_deployments` (the pause switch), `hostname` (the serving label), and `branch_deployments` (branch-to-environment mappings).

## Reading an environment's declaration honestly

An entry's `resources` are the exact manifests that deploy — plus two injection facts worth stating when reporting them:

- **A blank image IS the injection slot.** The deploy fills it with the built artifact. On `KubernetesDeployment` the injection is unconditional (a declared image is REPLACED at deploy); on multi-container kinds like `GcpCloudRun`, only blank-image containers receive the artifact (declared images — sidecars — deploy as written).
- **Config references stay unresolved.** `$var/...` and `$secret/...` values in manifests resolve just-in-time at deploy; the declaration never carries secret material.
- **The registry login is filled beside the image, on Kubernetes workloads.** When the service's registry connection holds a login a cluster can keep (a stored key or token, or GHCR's read-only pull token), the deploy writes it onto the applied workload's `spec.pod.imageRegistries` with the password as a `$secret/` reference — so the DECLARED manifest carries no login for the service's own registry and needs none; the run's environment row says what was filled or why nothing was. A login the declaration already carries for the same server is never overwritten (`references/service.pulling-private-images.md`).

The console renders all of this on the service page's **Configuration tab**: the writer story with sync provenance, each environment's resources with their facts and exact YAML, a resource diagram when an environment declares two or more resources (wires drawn from the declaration's own `valueFrom` references), and — for platform-authored services — the editors. When a person would rather look than read JSON, point them there.

## What the console can edit (so you never undersell it)

On a platform-authored service the Configuration tab edits ANY declared resource, one editor per shape: the simple single-resource Kubernetes/Cloud Run shape gets the guided dials; every other resource — any catalog kind, any member of a multi-resource set — edits through that kind's own authored form, or as YAML in place for kinds without one. The tab also adds a resource to an environment (the catalog picker honors the org's catalog policy by omission — a policy-disabled kind is simply not offered) and removes one (never the last — removing the environment's configuration is that door). Environment variables edit as three row classes: literal text, secret references through the platform's secret picker (the manifest stores the `$secret/...` pointer, and a key-value secret asks for its key), and locked rows for `valueFrom` wiring, which stays YAML-authored for now. A declaration carrying content outside the kind's schema as this console knows it stays read-only with the reason named — so if a person says the console "won't let them edit" something, that sentence is the reason to relay, not a bug to work around.

## Changing configuration: get, edit surgically, apply

The loop is `get_service` → edit the JSON → `apply_service` with the whole record. Two disciplines make it safe:

1. **Fetch fresh, edit only what you mean.** Apply sends the whole record, so start from a fresh get (never a stale copy) and touch only the fields the person asked about. A manifest is a declaration other hands may have authored — probes, autoscaling, secret variables, wired `valueFrom` references. Changing the memory limit means changing the memory limit, not regenerating the manifest.
2. **Nothing deploys when you save.** Configuration changes take effect on the next deployment to the environment — the next push, or a promote into it. Say this every time: a person who saves and sees nothing change will otherwise read success as failure. (A rollback does NOT pick up new configuration — it re-applies a captured pair; see `references/service.delivery-verbs.md`.)

Refusals arrive as the server's own sentences and always name the real fix — a serving-hostname collision names both services and the environment; a branch mapped twice names the branch. Relay them verbatim.

## What belongs on the service, and what belongs on the infra project

A service's environment declares the resources that exist BECAUSE of that service: its workload, and the traffic that reaches only it — the `KubernetesHttpRoute` or `KubernetesIngress` that fronts it, the Cloud Run domain mapping or ALB listener rule for its hostname, its own ConfigMap and ExternalSecret. Everything one service shares with others belongs to an infra project — the Gateway the routes attach to, the wildcard certificate, the DNS zone, the namespace, the database, the cache. The test is one question: if this service were deleted, should the resource go with it? Yes → the service's `resources[]`; no → the infra project.

The boundary matters for more than tidiness. The platform reads a service's URL from the resources in ITS OWN environment (the route's hostname, the Cloud Run service's URL, the ALB's DNS name); a route declared in the infra project gives the service no address, no endpoint check, and nothing to click on the service page. And the service's delete cascade tears down only what the service declares — a route left in the infra project outlives the service it fronted.

**Moving a resource from an infra project onto the service.** There is no verb that transfers a live resource between the two; the move is three steps, each an ordinary act: (1) declare the resource under the service's environment (`deploy.environments[].resources[]`, per the get-then-apply loop above), (2) remove it from the infra project's chart or manifest set and apply the project so the project's copy is destroyed, (3) deploy the service (a push, or `planton service deploy`) so the service's copy is created. Say the honest consequence before doing it: between steps 2 and 3 the resource does not exist, so traffic through a moved route is interrupted until the service's deploy applies it — order the two steps back to back, or move at a quiet hour. Do not "adopt" the live object into the service's DAG by hand; two owners of one resource is a state fight the next apply loses.

**Adding** an environment is adding one entry to `deploy.environments` — the slug plus its manifests. Copying an existing environment's resources and re-stamping `metadata.env` (and the `{service}-{env}` name convention, when the source followed it) is the faithful path for any resource shape. Nothing deploys at add time; the next push (or a promote into the new environment) performs the first deployment. Adding an entry can newly collide on the serving hostname — the refusal names it.

**Removing** an entry stops FUTURE deploys to that environment. State the no-teardown fact plainly: **anything already running there keeps running** — removing configuration never destroys resources. Tearing down deployed resources is the service delete cascade's job (`references/service.delete-cascade.md`), not a configuration edit's.

## The deployments on/off switch

`spec.deploy.disable_deployments: true` pauses deployment execution for the whole service: builds keep running and publishing artifacts, the configuration sync of git-maintained services continues, but nothing deploys and the delivery verbs refuse with a sentence pointing at the service's deploy settings. That switch lives on the console's Configuration tab, and it is caller-authored on every posture — flipping it through get-then-apply works on git-maintained services too. Resuming makes the next push deploy again; nothing deploys at the moment of resuming.

## What a configuration edit can never do

- It cannot change what is currently RUNNING (only the next deployment reads the declaration).
- It cannot touch a git-maintained service's environments (silently preserved-over — say so instead of trying).
- It cannot author `sync` provenance (user-supplied values are discarded) or resolve secrets into the record.
- It is refused entirely on a service that is being deleted.
