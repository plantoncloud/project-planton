---
title: "Monorepo Support"
description: "Configure multiple services from a single Git repository with per-service project roots, trigger paths, sparse checkout, and a build context that can reach shared code."
icon: settings
order: 65
tags:
  - Monorepo
  - Git
  - Configuration
  - Service Hub
---

# Monorepo Support

Modern codebases increasingly use monorepos for code sharing and atomic changes. But traditional CI/CD struggles with them: ten services in one repository means a shared library change triggers builds for all ten, even when only two need rebuilding. Clone times balloon because every build fetches the entire repository. Configuring path-based triggers requires deep knowledge of each CI system's quirks.

Service Hub provides built-in monorepo support. Each service is scoped to its own directory within the repository, triggers builds only when relevant files change, clones only the directories it needs, and can still build an image that reaches shared code elsewhere in the repository.

## Core Settings

Four settings control monorepo behavior. Two live on the service's Git repository binding (`spec.gitRepo`), one on its build triggers (`spec.build.triggers`), and one on the Dockerfile builder (`spec.build.dockerfile`).

### Project Root

The directory within the repository where the service's code lives. Builds use it as the working directory, the kustomize tree is found under it, and it is the default scope for triggers.

```yaml
spec:
  gitRepo:
    projectRoot: services/payment-api
```

Empty means the repository root. Whatever builds the image — the platform's Buildpacks or Dockerfile track, or the service's own pipeline — starts here; a Dockerfile path is relative to the build context, which defaults to this directory.

### Trigger Paths

Glob patterns that trigger builds when matched files change, **in addition to** the default rule that any change inside the project root triggers a build. Trigger rules live in one place, `spec.build.triggers`, beside the branch, pull-request, and tag rules.

```yaml
spec:
  gitRepo:
    projectRoot: services/payment-api
  build:
    triggers:
      paths:
        - "packages/shared-lib/**"
        - "proto/**"
```

How trigger paths work:

- Paths are matched against repository-root-relative file paths.
- If no trigger paths are set, builds run only when files inside the project root change.
- If trigger paths are set, builds run when files match **either** the project root **or** any trigger path.
- To trigger on every commit regardless of changed files, use `["**/*"]`.

Common patterns:

| Pattern | Triggers When |
|---------|--------------|
| `packages/shared-lib/**` | Shared library code changes |
| `proto/**` | Protobuf definitions change |
| `docker/base-image/**` | Base image definitions change |
| `**/*` | Any file in the repository changes |

### Sparse Checkout

Directories to clone during the build's git-clone step. When set, only these directories are fetched instead of the entire repository.

```yaml
spec:
  gitRepo:
    projectRoot: services/payment-api
    sparseCheckoutDirectories:
      - services/payment-api
      - packages/shared-lib
      - proto
```

Benefits for large monorepos:

- Faster clone times (fewer files to download)
- Less disk space used on the build cluster
- Only code the service needs is available during the build

If empty, the entire repository is cloned. Every directory the build reads — the project root, every trigger path you expect the build to consume, and a wider Docker build context — must be in this list, or the build will not find it.

### Docker Build Context

A Dockerfile that copies shared libraries from across the repository needs a build context wider than the service's own directory. `spec.build.dockerfile.context` is repository-root-relative and defaults to the project root; declare it only when the image needs MORE of the repository than the service's directory, and keep `projectRoot` truthful for triggers and kustomize.

```yaml
spec:
  gitRepo:
    projectRoot: services/payment-api
  build:
    dockerfile:
      dockerfilePath: services/payment-api/Dockerfile   # relative to the context
      context: "."                                     # the whole repository
    registry: my-registry
    imageRepositoryPath: acme/payment-api
```

## Kustomize Base Directory

The kustomize tree is found under the project root at `_kustomize` by default. When the tree lives elsewhere — for example, a service whose project root must be the repository root for yarn workspace resolution, with its overlays in a service-specific subdirectory — point at it explicitly:

```yaml
spec:
  gitRepo:
    projectRoot: ""                                  # repository root
  deploy:
    kustomize:
      baseDirectory: services/payment-api/_kustomize  # relative to the project root
```

## Practical Patterns

### Standard Monorepo

Multiple services, each in its own directory. Shared code triggers rebuilds for dependent services.

```yaml
# Service 1: User API
spec:
  gitRepo:
    ownerName: acmecorp
    name: backend-monorepo
    projectRoot: services/user-api
    sparseCheckoutDirectories:
      - services/user-api
      - packages/shared-lib
  build:
    buildpacks: {}
    registry: my-registry
    imageRepositoryPath: acmecorp/user-api
    triggers:
      paths:
        - "packages/shared-lib/**"
```

```yaml
# Service 2: Payment API (same repository, different service)
spec:
  gitRepo:
    ownerName: acmecorp
    name: backend-monorepo
    projectRoot: services/payment-api
    sparseCheckoutDirectories:
      - services/payment-api
      - packages/shared-lib
      - proto
  build:
    dockerfile: {}
    registry: my-registry
    imageRepositoryPath: acmecorp/payment-api
    triggers:
      paths:
        - "packages/shared-lib/**"
        - "proto/**"
```

A change to `services/user-api/` triggers only the User API build. A change to `packages/shared-lib/` triggers both.

### A Dockerfile That Copies Shared Code

A service whose image build needs the whole repository — a Dockerfile with `COPY packages/shared-lib ...` — keeps its own project root and widens only the build context:

```yaml
spec:
  gitRepo:
    projectRoot: apps/web
    sparseCheckoutDirectories:
      - apps/web
      - packages
      - package.json
      - yarn.lock
  build:
    dockerfile:
      dockerfilePath: apps/web/Dockerfile
      context: "."
    registry: my-registry
    imageRepositoryPath: acmecorp/web
```

Triggers and the kustomize tree still scope to `apps/web`; only the image build sees the whole repository.

## Editing These Settings

All four settings are fields on the Service record, and every door writes the same record: in the web console, the service's **Configuration** tab shows each of them as a row — the project root and build context under **Build Settings**, the trigger paths under **Build Triggers** — with an Edit dial that says the rule before saving; from a terminal, edit the service's YAML and `planton apply -f service.yaml`; or ask the Planton Assistant ("also rebuild payment-api when proto/ changes") and it makes the same edit. Read the current values with `planton get Service <slug> -o yaml`.

## Related Documentation

- [What is a Service?](/docs/ci-cd/what-is-a-service) — Service configuration overview
- [Build Methods](/docs/ci-cd/build-methods) — The builders and how they use the project root and the build context
- [Pipelines](/docs/ci-cd/pipelines) — How triggers control which pushes build
