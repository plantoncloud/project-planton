# Kubernetes Planton Platform

## When NOT to Use This

- **Without the operator** — this kind declares a `PlantonPlatform`
  custom resource; only the Planton operator's CRD admits it and only the
  operator reconciles it. Declare a KubernetesPlantonOperator resource
  first (one operator serves every platform on the cluster).
- **For a guided first install** — the Planton desktop app's self-hosted
  deploy journey walks a cluster from kubeconfig to a signed-in console
  with preflight checks and a managed tunnel. This kind is the
  declarative/GitOps motion, and the day-2 home for any install.
- **For Planton's hosted SaaS** — this deploys YOUR platform on YOUR
  cluster; app.planton.ai needs nothing from you.

## Overview

**KubernetesPlantonPlatform** declares a complete self-hosted Planton
platform — control plane, web console, identity server (Keycloak),
PostgreSQL (CloudNativePG), cache, workflow engine (Temporal), secrets
manager (OpenBAO), and an in-cluster deployment runner — in one
namespace, reconciled by the Planton operator. Several platforms can
share one cluster, each in its own namespace with its own URL, identity,
and databases.

**Zero-config by design:** `version` is the only required choice. A
version-only platform serves console, API, and sign-in on one origin
through a built-in gateway reachable with a single `kubectl
port-forward`, and the first console visitor becomes the admin using a
setup code read from a Secret. The exact commands for both are this
resource's outputs.

**Key design points:**

- **`version` is required, never defaulted** — a default that moved with
  catalog updates would silently upgrade a running platform (databases
  and all) on an ordinary re-apply. Upgrading is a deliberate one-line
  edit.
- **Everything else is opt-in refinement** — a real hostname and TLS
  through the cluster's Ingress controller or its Gateway API Gateway
  (`ingress`), storage classes and sizes (`storage` + per-component
  overrides), database HA (`database.postgresql.replicas: 2` grows a
  streaming-replication pair LIVE), cloud identity for the runner
  (workload-identity annotations or a customer-owned Secret — the
  platform stores no cloud credentials).
- **The spec renders only what you declare** — the operator's own
  defaulting stays authoritative for everything unset, so the manifest
  reads as your decisions and nothing else.
- **Honest shared facts** — one build-events sink per cluster (Tekton
  upstream), so builds feed only ONE build-enabled platform per cluster;
  one operator/CRD schema version per cluster, while each platform pins
  its own `version`.

## Spec Highlights

| Field | Required | Default | Description |
|---|---|---|---|
| `namespace` | yes | — | The platform's namespace (literal or KubernetesNamespace reference) |
| `version` | yes | — | The platform version — THE deliberate choice |
| `ingress` | no | off | Real URL via the cluster's front door — an Ingress controller (`ingress_class_name`) XOR a Gateway API Gateway (`gateway_ref`, whose `name` and `namespace` are KubernetesGateway references: `valueFrom` for a Planton-managed Gateway, `value:` for one created outside Planton); `tls` takes a Secret XOR a cert-manager issuer and requires `hostname` (with `gateway_ref` the listener owns the certificate, so only `issuer` applies); `reachability` (`auto`/`public`/`private`) declares whether the public internet reaches the door — the one fact the operator cannot see from inside the cluster, and what decides the doors that need an inbound path (keyless cloud connections, GitHub webhooks); `auto` reads a hostname over HTTPS as public |
| `gateway.local_port` | no | `8080` | The port-forward door's port — baked into sign-in at first boot |
| `storage` | no | cluster default | Platform-wide class + size; every component can override |
| `database.postgresql.replicas` | no | `1` | 2+ = streaming replication with automatic failover, live |
| `identity.admin_email` | no | setup-code flow | Pre-seed a known admin instead of first-visitor setup |
| `email` | no | no email | Outbound email through your own provider — `smtp` (any relay; `security` `starttls`/`tls`/`none`; a basic-auth Secret, OAuth2, or no credential; an optional private-CA bundle) XOR `resend`; `from.address` is the sending identity both the control plane and the identity server use; credentials are Secret names and references, never values. Absent, invitations are shared as links and the sign-in page has no "Forgot password?" |
| `bootstrap` | no | sane seeds | First org/env, extra admins, IaC provisioner (`tofu`/`terraform`), secret backend (`platform`/`awsSecretsManager`) |
| `runner`, `build`, `vault` | no | ON | The default-on arms; explicit `enabled: false` is the deliberate opt-out |
| `components` | no | off | Opt-ins: search (Solr), graph (Neo4j) |
| `control_plane`, `console` | no | — | Sizing, image mirrors, extra env via Secret, the platform's own cloud identity |

## Example

```yaml
apiVersion: kubernetes.planton.dev/v1alpha1
kind: KubernetesPlantonPlatform
metadata:
  name: planton
  org: acme-corp
  env: platform
spec:
  namespace:
    value: planton
  create_namespace: true
  version: v0.0.45
```

Apply it, wait for the operator to reach Ready (`kubectl get
plantonplatforms -A` shows phase and URL), then run the
`port_forward_command` output and open `http://localhost:8080`.

## Outputs

| Output | Description |
|---|---|
| `namespace` | The platform's namespace |
| `platform_name` | The CR name — prefix of every operator-created object |
| `gateway_service` | The front-door Service (`{name}-gateway`) |
| `setup_code_secret` | The first-run setup-code Secret |
| `port_forward_command` | The exact command that opens the door |
| `setup_code_command` | The exact command that reads the setup code |

## Destroy

Deleting the resource deletes the platform — databases included. Every
operator-created object is owner-referenced to the declaration and
garbage-collected by Kubernetes, so teardown completes even when the
operator itself is already gone; the database layer removes its volumes
and credentials together. Build caches and workflow volumes can survive
in the namespace (its deletion — automatic with `create_namespace: true`
— sweeps them), and the platform's namespace-qualified token-review
ClusterRole/Binding lingers inert until an operator release adds the
janitor.

---

© Planton. Licensed under [Apache-2.0](https://github.com/plantonhq/planton/blob/main/LICENSE).
