---
title: Where a Service Answers — URLs and Rollout Verification
description: The addresses on a deployment record (discovery, not endorsement) and the staged rollout verdict (resources created, workload online, endpoint answering) folded into verified/failed/unverifiable. Read when someone asks where their service is running, whether a deploy is really up, or what a rollout verdict means — relaying the verdict words accurately matters more than anything else here.
---

# Where a Service Answers — URLs and Rollout Verification

Every successful deployment record carries two answers a person otherwise digs for in a cloud console: the addresses the deployed service serves at, and whether anything actually answered there. Read this when someone asks where their service is running, whether a deploy is really up, or what a rollout verdict on a deployment record means.

## The URLs on a deployment record

A deployment record lists every address its deployed resources declared — the serverless platform's generated URL, the load balancer's DNS name, the ingress host, a function's endpoint. Each entry names its source (which resource and which of its outputs produced it), so you can always say WHERE an address came from.

Two things to hold when relaying them:

- The list is **discovery, not endorsement**. An address appears because the deployment target declared it, including addresses that did not answer the probe — the address that did not answer is exactly what a person debugging needs, so never hide it.
- A bare hostname (a load balancer's DNS name) appears under both https and http, because the scheme is not knowable from the target's outputs. Whichever one answers is the one that serves.

Some targets declare no address at all — a Kubernetes workload with no ingress has only in-cluster DNS, and container-orchestrator services expose their address through the load balancer deployed beside them. A record with no URLs is honest, not broken; say what would add one (an ingress, a load balancer) rather than treating it as a failure.

## The rollout verdict, and how to read it

Verification is staged the way a person checks by hand, in the order a person checks: **resources created on the provider** (the apply landed), **workload online** (the deployed workload actually running, in the provider's own terms), and **endpoint answering** (the probe's report, with per-address outcomes — probed only after the workload settles, so a warming service is never marked unreachable by a probe that fired too early). Every step executes on the organization's own runner, inside their infrastructure, and the verdict's summary names which runner watched.

**The workload check** exists for targets the platform has a deep verifier for — an ECS service watched until it reaches its desired running count with a stable rollout, a Cloud Run revision until it reports Ready, a Kubernetes Deployment or StatefulSet until its NEW generation's replicas are ready (the previous generation's green is never trusted, the same guard `kubectl rollout status` applies; for StatefulSets the update revision must also converge, so old pods serving while new ones crash-loop never reads as online) — using the same provider connection the deployment itself used, nothing extra to configure (managed Kubernetes clusters mint their short-lived cluster token from the sibling cloud connection, exactly as their deploys do). The watch is bounded: a healthy workload verifies in seconds, and one that never comes online records a failed check carrying the provider's own diagnostic words (the rollout state reason, recent service events, the revision's condition message — and for Kubernetes the stuck pods' own reasons: CrashLoopBackOff, ImagePullBackOff, the scheduler's insufficient-resources message) plus the resource's identity — the exact starting point for an investigation. Targets without a deep verifier carry no workload check at all: absent is honest, never a silent pass. A check reading "the captured stack outputs did not carry ..." means the deployed module version predates an output the verifier needs — redeploying with a current module closes it.

Each check also carries structured **evidence** rows beside its prose detail — one per watched workload (kind, slug, cloud-resource id, its own status, the provider's words) or probed address. When investigating a failure, dispatch on the rows, not the prose: a failed Kubernetes workload row hands you the exact namespace-scoped object and its pod-level reasons; a failed address row with a verified workload row points at routing or DNS, not the service.

The overall verdict is one of three words, folded from the checks, and relaying them accurately matters more than anything else in this file:

- **verified** — every check that ran affirmed: the workload came online and/or a declared address answered. A target with no address of its own (an ECS service with no load balancer beside it) verifies through its workload check alone.
- **failed** — a check reported a real problem: the workload never came online within the watch window, or addresses existed and none answered from the runner's vantage. Say this carefully: **the deployment itself succeeded** — resources applied, the record exists, later environments still deployed. Read the failed check's detail before relaying: a workload failure carries the provider's own words (start there), while an endpoint failure with a HEALTHY workload usually means DNS is not set up yet or the runner has no route to that address. Investigate; never rephrase it as "the deploy failed".
- **unverifiable** — nothing could be checked, with each reason recorded: no declared address, no runner available, outputs missing. Never treat it as a warning to fix urgently; treat it as the platform declining to guess.

Any HTTP status code counts as an answer — a 404 from a load balancer's default action still proves the address serves. The status code is in the check's detail; if someone expected a healthy page and the probe recorded a 500, that distinction is yours to surface.

## Answering "where is my service?"

The one-command answer is `planton service urls <service>` — each environment's current deployment with its addresses and verdict. For the full story on one deployment (per-address outcomes, the probe's vantage, the staged checks), read the deployment record itself. Promotions and rollbacks produce records with the same URLs and verdicts as any push-triggered deploy — no special case.

For a pull request's PREVIEW, prefer `list_service_previews` (CLI: `planton service previews <service> --pr <n>`) — it carries these same URLs and the same verdict joined with the preview's life state and the run's explanations, so one call answers "is my preview live, and where?" (see `preview-environments.md`).
