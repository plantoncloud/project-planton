---
title: "Self-Hosting"
description: "Run the entire Planton platform on your own Kubernetes cluster — two helm installs or two manifests through OpenTofu or Pulumi; batteries included, no external services, no configuration required"
icon: server
order: 70
tags:
  - Self-Hosting
  - Kubernetes
  - Helm
  - Operator
---

# Self-Hosting Planton

Planton runs entirely on your own Kubernetes cluster: the control plane, the web console, the identity server, the secrets manager, the databases, and an in-cluster runner — installed with two Helm commands (or one Planton CLI command), managed by a Kubernetes operator, reachable in minutes.

## Why self-host

Your infrastructure manifests, deployment history, secrets, and cloud credentials never leave your cluster. Self-hosting fits teams with data-residency requirements, air-gap-adjacent environments, or a platform team that wants Planton inside the same trust boundary as the infrastructure it manages.

## Install

Two commands install everything — the operator first, then the platform at the release you name:

```bash
helm install planton-operator oci://ghcr.io/plantonhq/charts/planton-operator \
  --namespace planton --create-namespace

helm install planton oci://ghcr.io/plantonhq/charts/planton \
  --namespace planton \
  --set platform.spec.version=v0.0.59
```

The first chart installs the Planton operator together with the `PlantonPlatform` definition it serves. The second creates one `PlantonPlatform` resource, and the operator reconciles the whole stack from that single resource: PostgreSQL, the workflow engine, the control plane, the console, the identity server, the secrets manager (OpenBAO, initialized and unsealed automatically), and the in-cluster runner. No license key, no admin account, no database, and no values file are required — the one value is the platform release, because the chart pins none of its own. The published releases are the [control-plane image's tags](https://github.com/orgs/plantonhq/packages/container/package/planton%2Fcontrol-plane); an operator runs releases from a floor upward and refuses an older one on the resource with the floor named. The Planton desktop's guided install does the same two steps for you — operator chart, then the platform declared directly — preselecting the release the desktop shipped with, and hands you the manifest and commands to keep.

Watch it converge (typically 7–11 minutes):

```bash
kubectl get plantonplatform -n planton -w
```

Every component reports its own plain-language status on the resource — a stuck component names the problem and the fix in `kubectl describe plantonplatform`.

## Install through infrastructure as code

The same two steps exist as catalog resources, so an agent or a pipeline installs Planton the way it installs everything else: two manifests, applied in order, through OpenTofu or Pulumi. The operator kind installs the operator chart (and the `PlantonPlatform` definition it owns); the platform kind declares one platform against it.

```yaml
# planton-operator.yaml
apiVersion: kubernetes.planton.dev/v1alpha1
kind: KubernetesPlantonOperator
metadata:
  name: planton-operator
  annotations:
    planton.dev/provisioner: tofu      # or pulumi
spec:
  namespace:
    value: planton-operator
  create_namespace: true
---
# planton.yaml
apiVersion: kubernetes.planton.dev/v1alpha1
kind: KubernetesPlantonPlatform
metadata:
  name: planton
  annotations:
    planton.dev/provisioner: tofu      # or pulumi
spec:
  namespace:
    value: planton
  create_namespace: true
  version: v0.0.59
```

```bash
planton apply -f planton-operator.yaml
planton apply -f planton.yaml
```

The CLI downloads each kind's published module for its own release and runs the engine the annotation names; with `KUBECONFIG` pointing at the cluster, nothing else is configured for OpenTofu, and Pulumi additionally takes its state backend from the `pulumi.planton.dev/backend.url` annotation (with `pulumi.planton.dev/stack.fqdn` naming the stack). The operator kind's chart version and the platform kind's `version` are the two upgrade levers: change one and re-apply. The operator kind's two `crds` dials (`install`, `keep_on_uninstall`) govern whether the operator release installs the definitions and whether they survive its removal, both on by default. Field-by-field reference: the two kinds' pages in the open-source catalog, [KubernetesPlantonOperator](https://github.com/plantonhq/planton/tree/main/catalog/kubernetes/kubernetesplantonoperator) and [KubernetesPlantonPlatform](https://github.com/plantonhq/planton/tree/main/catalog/kubernetes/kubernetesplantonplatform), or the same kinds in the console's deployment-components store.

## First sign-in

The install's `NOTES` print the exact commands. In short:

```bash
kubectl -n planton port-forward svc/planton-gateway 8080:80
```

Open `http://localhost:8080`. The first person to open the console becomes the administrator: enter your email plus the cluster setup code (the `NOTES` print the `kubectl` command that reads it — holding cluster access IS the admin proof), and receive a one-time password.

## Publish at your own URL

The built-in port-forward front door works on every cluster with zero configuration. Going public is a ladder of one field at a time on the `PlantonPlatform` resource:

```yaml
spec:
  ingress:
    enabled: true                      # auto-derives a working URL from your ingress controller
    hostname: planton.example.com      # or serve your own domain
    tls:
      issuer: {name: letsencrypt, kind: ClusterIssuer}   # cert-manager HTTPS
```

With `enabled: true` alone, the operator derives a magic-DNS hostname from your ingress controller's published address — a working URL with zero DNS setup, unique to the platform's name and namespace. The desktop app offers this whole journey as a guided experience: pick a cluster from your kubeconfig, preflight it, choose the front door, and watch the install converge — driving the exact same chart underneath.

## Several Plantons, one cluster

Platforms are namespaced, and one operator serves the whole cluster — it watches every namespace. Teams can run separate Planton platforms side by side (staging and production, or one per team), each fully confined to its own namespace:

```bash
# The operator is already on the cluster; every platform is one more release.
helm install planton oci://ghcr.io/plantonhq/charts/planton \
  --namespace planton-team-b --create-namespace
```

Never install two operators (the operator itself refuses to start beside another and says so), and give each platform its own namespace. Two cluster-level facts are shared by design: build events (the CI event stream Tekton delivers) can feed only one platform per cluster, and all platforms ride the one installed operator version — each platform still pins its own `spec.version`.

## Upgrades and uninstall

Config changes are edits to the `PlantonPlatform` resource; the operator reconciles them. The platform version is `spec.version` on that resource — `helm upgrade planton --set platform.spec.version=<version>` rolls the platform with its data intact. The operator upgrades through its own chart, and that chart carries the `PlantonPlatform` definition with it, so the schema always matches the operator that reads it. An operator runs platform releases from a floor upward: declare a version older than the oldest it supports and the resource goes to phase `Error` with the reason in its `MESSAGE` column, nothing is created, and a platform already running is left as it is — move the version, or install an operator release built for it.

`helm uninstall` removes the platform's workloads; data volumes deliberately survive. To remove everything including data, delete the namespace. The cluster-scoped badge-verification grant (`<namespace>-<name>-control-plane-token-reviewer`) is the one manual cleanup step of a full teardown.

## Requirements

- Kubernetes 1.24+, amd64 nodes (arm64 works only under emulation, e.g. local Docker Desktop)
- A default StorageClass whose storage driver is actually installed (or pin one via `spec.storage.storageClassName`)
- 6 GiB+ allocatable memory is comfortable; smaller evaluation clusters work with resource floors
