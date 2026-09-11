# Planton Platform

Declares a complete self-hosted Planton platform — control plane, web console, identity server, PostgreSQL, cache, workflow engine, secrets manager, and an in-cluster deployment runner — as a `PlantonPlatform` custom resource the Planton operator reconciles. Zero-config by design: `version` is the only required choice, the built-in gateway serves console + API + sign-in over a single port-forward, and the first console visitor becomes the admin. Several platforms share one cluster, each in its own namespace with its own URL, identity, and databases — all served by one operator.

## What Gets Created

When you deploy this Cloud Resource, the IaC module provisions:

- **Kubernetes Namespace** — created only when `createNamespace` is `true`; otherwise the namespace must already exist
- **PlantonPlatform CR** — the one declaration; the OPERATOR then creates the platform from it (workloads, Services, Secrets, volumes — all in the platform's namespace, all named from this resource's name and owner-referenced to the declaration)

## Before You Deploy

### Planton Setup

- **Kubernetes Provider Connection** — an active connection in the Connect module with credentials for the target cluster. Map it as the default for your environment, or specify it explicitly when creating the Cloud Resource.

### Kubernetes Cluster

- **The Planton operator** — a deployed Planton Operator resource (one per cluster serves every platform). Without it the declaration is never reconciled.
- **A default StorageClass that can actually provision volumes** (or set `storage.storageClassName`) — the operator verifies this before deploying and its status explains any storage problem in plain language.
- **An ingress controller or a Gateway API Gateway** (only for `ingress` — an IngressClass, or a Gateway named by `ingress.gatewayRef`) and **cert-manager** (only for `ingress.tls.issuer`).

## Deploy

### Console

Open the deployment store, find **Planton Platform**, and click **Deploy**. The creation wizard walks you through placement, the version, exposure (port-forward vs ingress/TLS), storage, identity and bootstrap seeding, the runner's cloud identity, and the opt-in components. Start from the **Zero Config** preset in the [Presets](#presets) tab.

### CLI

Create a manifest and apply it:

```yaml
apiVersion: kubernetes.planton.dev/v1alpha1
kind: KubernetesPlantonPlatform
metadata:
  name: planton
  org: acme-corp
  env: prod
spec:
  namespace:
    value: planton
  createNamespace: true
  version: v0.0.45
```

```shell
planton apply -f planton.yaml
```

This declares a full zero-config platform: the operator brings up the control plane, console, identity server, databases, secrets manager, and runner in the `planton` namespace. Watch it come up (`kubectl get plantonplatforms -A` — phase, version, URL), then use the `port_forward_command` output to open the door and the `setup_code_command` output for the first-visit setup page. A Stack Job tracks the provisioning in real time.

### InfraChart

When deploying as part of a multi-resource environment, use ValueFromRef to place the platform in a managed namespace:

```yaml
spec:
  namespace:
    valueFrom:
      kind: KubernetesNamespace
      name: planton
      fieldPath: spec.name
  createNamespace: false
  version: v0.0.45
```

The InfraPipeline creates the namespace first, then declares the platform into it. The operator itself is a `depends_on` edge declared through `metadata.relationships` — no spec field consumes an operator output. The `planton-on-kubernetes` InfraChart carries namespace + operator + platform as one deployable arm.

## Key Configuration

These are the most important decisions when configuring a Planton Platform. Explore the full field reference in the [API Explorer](#api-explorer) tab.

**`version` is required and never defaulted** — a default that moved with catalog updates would silently upgrade a running platform — databases and all — on an ordinary re-apply. Changing this one field IS the upgrade path, and upgrades of a system holding your data must always be a deliberate act.

**Set the URL before the first sign-in** — the identity server bakes the platform URL into its realm at first boot. For port-forward platforms that URL includes `gateway.localPort` (default 8080; two port-forward platforms on one machine need distinct ports); for ingress platforms it is `ingress.hostname`. Deciding exposure after the first visit means re-doing identity setup. `ingress.tls` requires a hostname (a certificate cannot be issued for an auto-derived address) and takes exactly one of `secretName` or a cert-manager `issuer`.

**Pick the front door the cluster already runs** — `ingress` serves the platform through either an Ingress controller (`ingressClassName`, or the cluster's default class) or a Gateway API Gateway (`gatewayRef`: Istio, Envoy Gateway, Cilium, a cloud Gateway); never both. On the Gateway door the operator attaches one route for the hostname and reads the Gateway's listeners — an HTTPS listener that already serves the hostname means `https://` with no `tls` block, and `tls.issuer` has cert-manager issue a certificate for the listener to reference (`tls.secretName` does not apply there). Never route a Gateway of your own to the platform's port-forward Service: pages load, but the platform still advertises `http://localhost:8080` and sign-in goes there. `gatewayRef.name` and `gatewayRef.namespace` are foreign keys to KubernetesGateway: when the Gateway is Planton's own, wire both with `valueFrom` in the same infra chart and the platform deploys after its Gateway, follows a rename, and shows the edge in the resource graph; a Gateway created outside Planton takes the literal names with `value:`.

**Say whether the internet reaches the door** — `ingress.reachability` is the one fact about the front door the operator cannot observe from inside the cluster, and it decides the doors that need an inbound path from the internet: keyless cloud connections (the cloud fetches the platform's identity documents from the door) and GitHub webhook delivery. `auto` (the default) reads a hostname served over HTTPS as public and anything else as private — the right answer for most installs. Declare `private` when only your network reaches an HTTPS address (split DNS, a corporate CA, an internal load balancer), so those doors stay honestly closed instead of failing at the cloud's first fetch; declare `public` when TLS terminates outside the cluster (an internet-facing ALB with an ACM certificate has no in-cluster `tls` block for `auto` to read). `public` on a disabled ingress is refused — a port-forward door is never reached from the internet.

**Declare email once and both senders use it** — `email` names the one mail provider the control plane (invitations, alerts) and the identity server (password resets) both send through, from `from.address`. Pick one arm: `smtp` for any relay (Exchange Online, Google Workspace, an internal smart host, or a vendor's SMTP endpoint) or `resend` for Resend's API. On a relay, `security` is kept as declared — `starttls` requires the upgrade, `tls` opens TLS from the first byte, `none` is plaintext for credential-free internal relays only — and sign-in is one way: a `kubernetes.io/basic-auth` Secret, an OAuth2 app registration, or no credential. Credentials are Secret names and Secret key references, never values; a rotated Secret is live on the next send. Without `email` the platform is fully usable — invitations are shared as links — and the console's Email settings hand you the exact fragment and Secret command for this install, then prove the relay on demand once it is declared.

**The runner's cloud identity is yours, never the platform's** — the platform stores no cloud credentials. Give the runner workload-identity annotations (`runner.serviceAccountAnnotations` — IRSA on EKS, Workload Identity on GKE/AKS) or name a customer-owned Secret in `runner.cloudCredentialsSecretName`; rotate by updating YOUR Secret. Disabling the runner leaves a platform that can model infrastructure but not deploy it.

**One build-enabled platform per cluster** — Tekton allows exactly one cluster-wide build-events sink, so builds can feed only one platform per cluster. When several platforms share a cluster, set `build.enabled: false` on all but one.

**Opting out of the bundled secrets manager needs a replacement** — `vault.enabled: false` is a deliberate opt-out; pair it with a cloud backend in `bootstrap.secretBackend` (e.g. `awsSecretsManager` with its region and KMS key, reached through the control plane's own workload identity) or connection secrets have nowhere to live.

**Storage is one dial with per-component overrides** — `storage.storageClassName` and `storage.size` govern every platform volume unless a component overrides them; one `size` value lifts every volume above a backend's minimum-size floor. On EKS, `storage.storageClassName: gp3` moves the whole platform off the legacy gp2 class in one line.

**Cluster-shared sub-operators are shared on purpose** — `prerequisites` defaults every sub-operator (CloudNativePG, Tekton Pipelines) to `auto`: installed only when absent, respected when something else manages them, and deliberately left behind on destroy because sibling platforms may ride them.

**Destroy takes the databases with it** — deleting the resource tears the whole platform down; every operator-created object is owner-referenced to the declaration, so garbage collection completes the teardown even when the operator is already gone, and the database layer removes its volumes and credentials together. Build caches and workflow volumes can survive in the namespace; when this resource owned the namespace (`createNamespace: true`), its deletion sweeps them.

## Outputs and Dependencies

### What This Component Consumes

| Dependency | Field | ValueFromRef Path |
|------------|-------|-------------------|
| **KubernetesNamespace** | `namespace` | `spec.name` |

### What This Component Provides

After provisioning, `status.outputs` contains values that downstream Cloud Resources can consume via ValueFromRef (all derive from the declaration itself — the operator's naming is deterministic per platform name, so they are stable from the first apply):

| Output | Description | Common Downstream Use |
|--------|-------------|----------------------|
| `platform_name` | The PlantonPlatform CR name — the prefix of every object the operator creates | kubectl inspection of the platform's workloads |
| `gateway_service` | The built-in front-door gateway Service (`<name>-gateway`) — console, API, and sign-in on one origin | Composing exposure from Ingress or Gateway API kinds |
| `setup_code_secret` | The Secret holding the first-run setup code (`<name>-identity-setup-code`) | Granting bootstrap access to the first-visit setup page |
| `port_forward_command` | The exact command that opens the platform's door on this machine | Workstation access; the Planton desktop app's connect-existing flow |
| `setup_code_command` | The exact command that reads the first-run setup code | First-visit admin setup |

## Common Patterns

Browse the [Presets](#presets) tab for ready-to-deploy configurations.

**Zero config** — a version-only platform behind the built-in gateway: one port-forward opens console, API, and sign-in, and the first visitor becomes the admin. The right start on any cluster, including laptops. Start from the **Zero Config** preset.

**Real hostname with in-cluster TLS** — the platform at your own domain through the cluster's ingress controller, with cert-manager issuing and renewing the certificate. Set the hostname before the first sign-in. Start from the **Ingress + TLS** preset.

**Real hostname through a Gateway API Gateway** — the platform behind the Gateway the cluster already runs (Istio, Envoy Gateway, Cilium, a cloud Gateway); the Gateway's HTTPS listener serves the hostname, so no in-cluster `tls` block is needed. Start from the **Gateway API Front Door** preset.

**EKS-shaped platform** — gp3 storage for every volume, the AWS Load Balancer Controller serving the hostname with an ACM certificate at the edge (no in-cluster `tls` block), and IRSA giving the runner keyless AWS identity. Start from the **EKS** preset.

## Works With

- [**Planton Operator**](/cloud-catalog/kubernetes-planton-operator) — the hard prerequisite: the manager that reconciles this declaration; one per cluster serves every platform
- [**Kubernetes Namespace**](/cloud-catalog/kubernetes-namespace) — provides the platform's namespace when composed in an InfraChart
- [**Cert Manager**](/cloud-catalog/kubernetes-cert-manager) — issues and renews the ingress certificate when `ingress.tls.issuer` is used
