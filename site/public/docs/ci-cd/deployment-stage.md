---
title: "Deployment Stage"
description: "How pipelines turn built artifacts into running services — manifest resolution, Kustomize overlays, environment mapping, and local development."
icon: deployment
order: 45
tags:
  - Deployment
  - Kustomize
  - Service Hub
---

# Deployment Stage

After the build stage produces a container image or worker script, the deployment stage takes over. It resolves deployment manifests, creates a deployment task for each target environment, and provisions each one through a Stack Job. This page explains what happens during that process — how manifests are produced, how environments are matched, and how you can use the same system for local development.

For the high-level pipeline model (triggers, stages, cancellation, approval gates), see [Pipelines](/docs/ci-cd/pipelines). For supported platforms and the choice between Git-based and inline configuration, see [Deployment Targets](/docs/ci-cd/deployment-targets).

## How Manifests Are Resolved

The deployment stage produces cloud resource manifests through one of two paths, depending on how the service is configured:

**Git-based** (default): The pipeline reads deployment manifests from a `_kustomize` directory in your repository. During the build stage, a kustomize-build task processes each overlay directory and stores the merged manifests. The deployment stage reads those manifests and creates one deployment task per environment.

**Inline** (UI-based): Deployment targets are defined directly in the Service configuration. The deployment stage reads them from the service spec, substitutes template variables (such as `{{ .Image }}` for the built container image reference and `{{ .CommitSHA }}` for the Git commit), and creates one deployment task per target.

Both paths converge at the same point: a set of cloud resource manifests, one per environment, ready to be provisioned through Planton's infrastructure layer.

## The Kustomize Model

Kustomize is a YAML patching tool built into `kubectl`. Planton uses it because it works with plain YAML patches — no templating language, no chart packaging, no extra toolchain. You write a base manifest and patch it per environment using overlays. The output is deterministic and version-controlled.

### Directory Structure

The `_kustomize` directory lives in your repository (by default at the project root, configurable in service settings):

```
_kustomize/
  base/
    kustomization.yaml
    service.yaml
  overlays/
    local/
      kustomization.yaml
      service.yaml
    dev/
      kustomization.yaml
      service.yaml
    production/
      kustomization.yaml
      service.yaml
```

### Base Configuration

The base defines your service's default resource specification — the settings shared across all environments:

```yaml
# _kustomize/base/kustomization.yaml
apiVersion: kustomize.config.k8s.io/v1beta1
kind: Kustomization
resources:
  - service.yaml
```

```yaml
# _kustomize/base/service.yaml
apiVersion: kubernetes.planton.dev/v1alpha1
kind: KubernetesDeployment
metadata:
  name: my-service
  org: my-org
spec:
  container:
    app:
      env:
        variables:
          PORT: "8080"
      resources:
        requests:
          cpu: 50m
          memory: 100Mi
        limits:
          cpu: 500m
          memory: 500Mi
      ports:
        - name: rest-api
          appProtocol: http
          networkProtocol: TCP
          servicePort: 80
          containerPort: 8080
          isIngressPort: true
  availability:
    minReplicas: 1
  version: main
```

### Environment Overlays

Each overlay directory patches the base with environment-specific values. An overlay inherits everything from the base and overrides only what differs:

```yaml
# _kustomize/overlays/production/kustomization.yaml
apiVersion: kustomize.config.k8s.io/v1beta1
kind: Kustomization
resources:
  - ../../base
patches:
  - path: service.yaml
```

```yaml
# _kustomize/overlays/production/service.yaml
apiVersion: kubernetes.planton.dev/v1alpha1
kind: KubernetesDeployment
metadata:
  name: my-service
  env: app-production
spec:
  container:
    app:
      resources:
        requests:
          cpu: 200m
          memory: 500Mi
        limits:
          cpu: "2"
          memory: 2Gi
      env:
        variables:
          LOG_LEVEL: warn
        secrets:
          DATABASE_PASSWORD: $secret/@production/database/password
  availability:
    minReplicas: 3
    horizontalPodAutoscaling:
      isEnabled: true
      maxReplicas: 10
```

The `metadata.env` field determines which Planton environment receives this deployment — not the overlay directory name. You can name directories however you like (`prod/`, `us-east/`, `blue/`), but the `env` field must match a real environment in your organization.

### Variables and Secrets in Overlays

Overlay manifests can reference organization-scoped secrets and variables using a substitution syntax:

- `$secret/<slug>` (or `$secret/@<env>/<slug>/<key>`) — Resolved at deployment time. The value is read just-in-time in the Runner from your secret backend, never stored in the platform's database.
- `$variables-group/<group>/<key>` — Resolved at deployment time. Variables support literal values or dynamic references to infrastructure outputs.

See [Secrets](/docs/secrets) for the full scoping model, backends, and lifecycle.

## The Local Overlay

The `local` overlay is a special case — it is never deployed. The deployment stage skips it entirely.

Its purpose is local development: generating `.env` files so developers can run services locally with the same variable and secret configuration used in deployed environments.

```bash
# Generate .env and .env_export files from the local overlay
planton service dot-env

# Override specific values for local testing
planton service dot-env --set API_KEY=test-key --set DEBUG=true
```

This keeps local development configuration version-controlled alongside deployment configuration, without any risk of it accidentally deploying to a real environment.

## How Deployment Tasks Execute

For each resolved manifest (excluding the local overlay), the deployment stage creates a deployment task:

1. **Environment matching** — If the service has deployment environment filters configured, only matching overlays produce tasks. See [Deployment Environments](/docs/ci-cd/deployment-environments).
2. **Ordering** — Tasks execute sequentially following the organization's promotion policy (for example: dev, then staging, then production).
3. **Manual gates** — If a deployment target requires manual approval, the pipeline pauses at that task until a team member approves or rejects. See [Pipelines](/docs/ci-cd/pipelines#manual-approval-gates).
4. **Stack Job creation** — Each task provisions the cloud resource manifest through a [Stack Job](/docs/infrastructure/stack-jobs). The Stack Job applies the infrastructure changes and reports completion.
5. **Failure handling** — If a deployment task fails, all subsequent tasks are cancelled. No partial rollouts across environments.

## How the Image Is Pulled

The deployment stage injects the built image into every manifest that receives one — a blank container image is the slot; explicitly authored sidecar images are left alone. The pull is the cluster's or runtime's own act, and the stage prepares for it in the open:

- **Kubernetes workloads** get their registry login filled onto `pod.imageRegistries` from the service's registry connection when that connection holds a login a cluster can keep — a stored token or key, or GHCR's read-only pull token — with the password as a `$secret/` reference the runner resolves inside the cluster's account. The run's environment row states what was filled, or why nothing was (*ECR issues only twelve-hour tokens — the cluster pulls with its own AWS identity*; *add a read-only pull token to the registry connection, or declare the login on the workload's imageRegistries*). A login you already declared for the same registry is never overwritten.
- **Cloud Run** pulls private images only from Artifact Registry; the service wizard warns at authoring time when the registry is anything else. **ECS** pulls from ECR with the task execution role and from other registries with the Secrets Manager credential the task definition declares.
- **A reference that has no value yet** — a pull secret named in `pod.imagePullSecrets` that was never deployed — is refused before the stack job is created, naming the field and the resource, instead of producing a pod stuck in `ImagePullBackOff`.

The three ways a workload can pull, and when to use each, are in [Pulling Private Images](/docs/connections/container-registries#pulling-private-images).

## How the URL Is Found

Once an environment's resources have applied, the deployment record lists every address the environment answers at, and the service page's environment card links the first one. An address comes from two places, and only two:

- **What a resource reported back after it applied.** Each kind's module exports what it knows: a Cloud Run service its `run.app` URL, a load balancer its DNS name, an Ingress or an HTTPRoute the first hostname it routes. Discovery reads those outputs — never the manifest you wrote — because for most kinds the address does not exist until the cloud creates it.
- **What the environment declares.** When the environment carries a serving domain and the service a `deploy.hostname`, the platform composes the hostname, fills it into the carrier beside your workload (an empty Ingress or HTTPRoute host, a Cloud Run domain mapping, an ALB listener rule), and lists that address.

A workload alone — a Deployment, an ECS service, a Cloud Function without a URL — is never an address. When neither source yields one, the card and the run say so in one sentence and name the kinds that would give the environment an address, so the fix is one of two moves: declare a serving domain on the environment, or add a resource that carries an address beside the workload. Rollout verification probes the addresses it finds and records whether each answered; an environment with nothing to probe is `unverifiable` with that reason, never a false green.

<!-- SCREENSHOT: Pipeline deployment stage
  Page: /{org}/service/{slug}/pipelines/runs/{id}
  Action: Show a pipeline run with the deployment stage in progress or completed
  Focus: The deployment stage section showing environment-specific deployment tasks
  Alt: Pipeline run detail showing the deployment stage with per-environment deployment tasks and their status
-->

## CLI Reference

```bash
# Initialize a new _kustomize directory
planton service kustomize init --new

# Initialize from an existing cloud resource
planton service kustomize init <resource-kind> <resource-id>

# Build and inspect the resolved manifests
planton service kustomize build

# Generate .env files from the local overlay
planton service dot-env

# Deploy using the _kustomize configuration
planton service deploy --project .
```

## Related Documentation

- [Pipelines](/docs/ci-cd/pipelines) — The full pipeline model including build stage and triggers
- [Deployment Targets](/docs/ci-cd/deployment-targets) — Supported platforms and the Git-based vs inline choice
- [Deployment Environments](/docs/ci-cd/deployment-environments) — Controlling which environments a service deploys to
- [Stack Jobs](/docs/infrastructure/stack-jobs) — How infrastructure changes are provisioned
- [Secrets](/docs/secrets) — Variable and secret management
