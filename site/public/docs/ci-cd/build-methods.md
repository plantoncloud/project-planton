---
title: "Build Methods"
description: "How Service Hub transforms source code into deployable container images or Cloudflare Worker scripts."
icon: deployment
order: 25
tags:
  - Build
  - Buildpacks
  - Dockerfile
  - Service Hub
---

# Build Methods

A build method determines how your source code is transformed into a deployable artifact. The choice matters because it defines the trade-off between convenience and control: Buildpacks handle everything automatically but give you less customization, while Dockerfiles give you full control but require you to maintain the build definition yourself.

## Artifact Types

Every Service produces one of two artifact types, selected during service creation:

| Artifact Type | What It Produces | Deploys To |
|---------------|-----------------|-------------|
| Container image | Docker container image | Kubernetes, AWS ECS, GCP Cloud Run, DigitalOcean App Platform |
| Cloudflare Worker script | JavaScript/TypeScript bundle | Cloudflare Workers |

The artifact type determines which build methods and deployment targets are available.

## Build Methods for Container Images

When building container images, two methods are available for platform-managed pipelines.

### Cloud Native Buildpacks

Buildpacks analyze your source code, detect the language and framework, and produce an optimized container image without requiring a Dockerfile. No additional build files are needed in your repository.

Buildpacks handle language and framework detection, dependency installation, application compilation, base image selection with security patches, and container layer optimization for caching. Supported languages include Node.js, Python, Go, Java, Ruby, PHP, .NET, Rust, and static files served via nginx.

**When to use Buildpacks**: Standard web services, APIs, and microservices in well-supported languages. Buildpacks are the recommended default for most services — they produce production-quality images with no configuration overhead, and base image security patches are applied automatically by buildpack maintainers.

**When to choose something else**: Projects with custom native dependencies, multi-stage builds, GPU requirements, ML workloads, or specialized base image needs.

### Dockerfile

A Dockerfile gives you full control over the build process. Place a Dockerfile in your repository and Planton executes the build.

By default, the platform looks for a `Dockerfile` in the project root directory. You can specify a custom path (relative to the project root) if your Dockerfile lives elsewhere — for example, `docker/Dockerfile.prod` for teams that maintain multiple Dockerfile variants.

```yaml
spec:
  build:
    dockerfile:
      dockerfilePath: docker/Dockerfile.prod
    registry: my-registry
    imageRepositoryPath: acme/storefront
```

Buildpacks is the same shape with `buildpacks: {}` in place of the `dockerfile` block. Both select one of the platform's release-pinned build tracks, compiled at dispatch and stamped on every run.

**When to use Dockerfile**: Custom native dependencies, multi-stage builds, ML/AI workloads with CUDA or specific library versions, specific base image requirements, or any scenario where you need precise control over the build environment.

## Image Tagging

For container image services, the pipeline tags images to ensure traceability:

- **Branch builds**: Tagged with the full Git commit SHA (e.g., `a1b2c3d4e5f6`), ensuring every image is traceable to exactly one commit.
- **Tag builds**: Tagged with the Git tag name (e.g., `v1.0.0`), providing human-readable version references for release workflows.

The final image is pushed to the container registry configured on the Service. The registry host and authentication come from the container registry credential referenced by the Service (see [Container Registries](/docs/connections/container-registries)).

<!-- SCREENSHOT: Build configuration in service details
  Page: /orgs/{org}/service/{serviceId} (Overview or Settings tab)
  Action: Show the pipeline configuration section with image build method and repository path
  Focus: The image build method radio group and image repository path field
  Alt: Service pipeline configuration showing Buildpacks selected as build method with image repository path configured
-->

## Cloudflare Worker Builds

There is no platform track for Cloudflare Worker scripts today. A Worker service builds through its own repository pipeline (`spec.build.tektonPipeline`) — bundle with Wrangler, upload the bundle where your deployment reads it — and deploys through the Cloudflare Worker deployment target like any other service. See [Self-Managed Pipelines](/docs/ci-cd/self-managed-pipelines).

## Configuring Build Methods

### Web Console

During service creation, the build configuration step presents:

1. **Build method selection**: Buildpacks (recommended) or Dockerfile for container images.
2. **Container registry**: Select from configured registries in the organization.
3. **Image repository path**: The path within the registry for built images.
4. **Pipeline branches**: Branches that trigger builds on push.
5. **Advanced settings** (collapsed): Dockerfile path, tag build toggles, and tag patterns.

After creation, every one of these lives on the service's **Configuration** tab. **Build Settings** shows the builder (with the Dockerfile path and build context, or the custom pipeline and its params), the push registry and image path, the build cluster, the target platforms, and the project root; **Build Triggers** shows the builds switch, the trigger branches, the trigger paths, what pull requests do (and how long a preview lives), and what tag pushes do. Each row states its current value and what that value does for the service, and opens a dialog that says the rule before it saves — a branch cannot both trigger the full promotion walk and be mapped to a single environment, a target platform is written as `os/arch`, a preview lifetime is at most thirty days. A build that would fail on its next run — no builder chosen, or a Dockerfile build with no registry to push to — is called out at the top of the tab with the door that fixes it.

After creation, build configuration is editable in the service's **Settings** tab under Pipeline Configuration.

### CLI

Build method is part of the Service configuration, set during `planton service register` (interactive) or via the Service YAML.

```bash
# Initialize a kustomize directory with a service manifest
planton service kustomize init

# Build the kustomize output to inspect the resolved manifest
planton service kustomize build
```

## Self-Managed Pipelines

A service whose `spec.build` selects `tektonPipeline` instead of `dockerfile` or `buildpacks` builds with your own Tekton pipeline — from the repository, or a pipeline the organization published once and the service names with `tektonPipeline: {pipeline: <name>}` — the platform compiles it at dispatch and handles deployment orchestration exactly as for the platform tracks.

See [Self-Managed Pipelines](/docs/ci-cd/self-managed-pipelines) for details on writing custom build pipelines.

## Related Documentation

- [What is a Service?](/docs/ci-cd/what-is-a-service) — Service configuration overview
- [Pipelines](/docs/ci-cd/pipelines) — The pipeline execution model
- [Self-Managed Pipelines](/docs/ci-cd/self-managed-pipelines) — Custom Tekton pipelines
- [Deployment Targets](/docs/ci-cd/deployment-targets) — Where built artifacts are deployed
