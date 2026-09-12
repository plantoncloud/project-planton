# Planton Operator

A Kubernetes operator that turns one `PlantonPlatform` resource into a complete, running Planton platform on any Kubernetes cluster, and keeps it converged.

## What It Does

The operator carries every deployment decision a self-hosted Planton needs -- which components exist, the order they come up in, their readiness gates, the credentials wired between them, and the first-boot seeding -- so an adopter declares a platform release and nothing else. It installs the prerequisite sub-operators (CloudNativePG, its Barman Cloud backup plugin when cert-manager is present, Tekton Pipelines), provisions the data services (PostgreSQL, Valkey, Temporal, the OpenFGA policy engine, the bundled OpenBAO secrets manager; Neo4j when opted in), renders the control plane, console, in-cluster runner, and identity server, and reports one phase and one plain-language message per platform in `kubectl get plantonplatform`.

Exactly one operator runs per cluster. A second installation refuses itself at startup, names the first, and says what to do; a cluster may run as many platforms as it likes under that one operator.

## Installing It

Adopters install the operator through its Helm chart, `helm/planton-operator` in this repository, published as an OCI chart. The chart and the operator image share one version line: chart `x.y.z` deploys operator image `vx.y.z`, both cut from the same tag. The chart also owns the two custom resource definitions the operator reconciles (`PlantonPlatform`, `PlantonIdentityProvider`): they are release resources, upgraded with the chart and kept on uninstall by default. The chart's README documents the values, the definition lifecycle, and the upgrade paths.

The same two steps exist as catalog kinds (`KubernetesPlantonOperator`, `KubernetesPlantonPlatform`) so an agent or a pipeline installs and upgrades a self-hosted Planton through OpenTofu or Pulumi from published modules. See `site/public/docs/self-hosting/` for the adopter-facing walk-through.

## Declaring a Platform

```yaml
apiVersion: planton.ai/v1
kind: PlantonPlatform
metadata:
  name: planton
  namespace: planton
spec:
  version: v0.0.45
```

`spec.version` names a Planton platform release as `vMAJOR.MINOR.PATCH`; the API server refuses any other shape. The operator runs releases from a floor upward: a version older than the oldest it supports is refused before anything is created, with the reason in the resource's `MESSAGE` column and a `VersionSupported` condition, and a platform already running is left untouched. The operator's first log line (`Platform version floor`) names the floor. To run a custom build, keep `spec.version` at a release and set `image.tag` on the component: the version names the contract, the tag names the bytes.

`spec.email` tells the platform how to send mail through the adopter's own provider, and one declaration powers both of the install's senders. It carries a sender identity (`from.address`, `from.name`, an optional `replyTo`) and exactly one provider: an SMTP relay (`host`, `port`, `security` as `starttls` | `tls` | `none`, and one way in -- a `kubernetes.io/basic-auth` Secret named by `credentialsSecretName`, an OAuth 2.0 client-credentials grant under `oauth2` for tenants that no longer accept passwords, or no credential for an allow-listed relay -- plus an optional private CA bundle by Secret reference) or a Resend account (`apiKeySecretRef`). Admission refuses a contradictory block in words: two providers or none, two ways in, credentials over a plaintext connection. The operator carries the declaration to the control plane as environment plus one projected volume of credential files that rotate in place (no secret value ever rides as an environment variable), preflights every referenced Secret and key -- a missing one renders the control plane as if no email were declared and reports the component not Ready with the Secret, its type, and the field to remove named -- and echoes what is declared in `status.email` and the `EMAIL` column (`NotConfigured` | `SMTP` | `Resend`; configuration, never a delivery verdict). The same declaration owns the bundled identity server's mail settings by key, with the credential rotated through a fingerprint and the relay CA in its truststore, so "Forgot password?" is on the sign-in page exactly when an email can be sent; with nothing declared the realm carries no relay and reset stays off, and a relay typed into the identity server's admin console is reverted on the next reconcile. With nothing declared the control plane is also handed two setup hints -- the Secret command and the `spec.email` fragment for this platform's own name and namespace -- which its Email settings page shows in place of a dead button.

`config/samples/` holds a minimal declaration, a lite profile for Kind, and a full profile that exercises every optional arm.

## How It Reconciles

The controller in `internal/controller/` is a thin orchestrator. Each reconcile fetches the resource, initializes status when the spec's shape changed, judges `spec.version` against the operator's floor (`internal/platformversion/`; a refused version ends the reconcile without a requeue and touches no component), then walks every registered component in `internal/component/`: a component whose dependencies are not yet Ready is marked Pending with the dependency named, the rest reconcile their own resources and report Ready, Deploying, or Error. One status write per reconcile; a 30-second requeue for ongoing convergence.

A component backed by a Deployment is Ready only when its rollout has finished -- the spec observed, every desired replica on the current template, no stale replica remaining, every replica available -- so a version change never reads as Ready while the previous release still serves.

A component that is not Ready explains itself. Every component's status carries a phase, a one-word `reason`, the `object` the reason is about, a sentence, and when that condition began; the platform's `Ready` condition -- the `MESSAGE` column of `kubectl get plantonplatform` -- speaks for the worst-off component in its own words, prefixed by its key. The not-ready branch of every component goes through one helper (`Base.NotReady` in `internal/component/workload_pending.go`) that reads the workload's own pods, the volume claims those pods reference, the workload's conditions, and the namespace's Events, and classifies them in order of certainty: a claim that will never provision, then what the kubelet states outright (a pull failure, a missing Secret key, an out-of-memory kill, a crash loop), then the scheduler's and the Deployment controller's verdicts, and last the calm "running, not yet answering its health check". The reasons are documented constants (`api/v1/component_reason.go`), not an enum, because an operator is upgraded before the platform and a new reason must never be refused by an older definition; failure reasons become a Warning Event on the platform when a component enters them, once, and a Normal one when it recovers. A component's `Refused(...)` result is the third shape: the declaration cannot be honored as written, and nothing deploys until the named field changes.

`PlantonIdentityProvider` has no controller of its own: a change to one re-enqueues the platforms in its namespace, and the identity component resolves the binding inside the same loop.

## The Front Door

A platform is reached through one public origin, rendered from one route table (`internal/resources/front_door_routes.go`) onto whichever door the declaration chooses: an Ingress object, a Gateway API HTTPRoute attached to a Gateway the cluster already runs, or the built-in nginx gateway served over `kubectl port-forward`. The browser API (gRPC-Web) lives under `/rpc`, the storage relay under `/storage`, the identity server under `/idp`, the keyless identity issuer's two discovery documents (`/.well-known/openid-configuration`, `/.well-known/jwks.json`) and inbound webhooks (`/webhooks`) reach the control plane's webhook port, and the console answers everything else.

The front door is the platform's keyless identity issuer, and it declares whether the public internet reaches it (`spec.ingress.reachability`: `auto` | `public` | `private`; `auto` reads a hostname served over HTTPS as public). The operator resolves that once, publishes the answer in `status.reachability` beside the URL, and renders every internet-facing posture of the control plane from it: the issuer URL is the door; keyless cloud connections are offered when the door is public, HTTPS, and the platform vault runs, and otherwise the connection wizard's keyless card names the one fact that closed the door; GitHub webhooks are delivered to `<door>/webhooks/github` when the door is public; and the door is the browser console, so the control plane hands out the door's own pages as the addresses a customer's GitHub App is configured with (its Setup URL and Callback URL) instead of any hosted address. No self-hosted install carries Planton's own GitHub App or its Google and Microsoft OAuth apps, so those doors are declared closed with a reason, never inferred from placeholder credentials.

The install's GitHub declaration (`spec.github`: the hosts a company uses, an install-wide GitHub App per host, whether each host can deliver webhooks) reaches the control plane the way the identity-federation facts do -- one facts file in a ConfigMap the control-plane component rewrites every pass and the Deployment mounts whole (`internal/resources/control_plane_github.go`), so a corrected declaration is live without a pod roll -- and the App keys ride the email credentials' shape, projected PEM files under one directory. Every App Secret is preflighted (`internal/component/github.go`); a missing one is `Refused` in words and that host is advertised without its App. The facts JSON is a cross-repo contract pinned by `testdata/github-facts.json` on both sides. Nothing declared is the github.com adopter's posture, stated out loud in the same file.

Native gRPC clients -- the `planton` CLI, the in-cluster runner, any grpc-go or grpc-java program -- reach the control plane's raw gRPC port at the same origin on the Gateway API door: a request whose `content-type` is `application/grpc` is routed there by an exact header match, a core Gateway API rule. The Ingress object and the nginx gateway have no portable header match, so they do not offer that row; on those doors a native client port-forwards the control plane Service's `grpc` port. The console publishes the address a native client should dial (`GRPC_ENDPOINT`, set only when the door routes it) and the deployment shape (`PLANTON_DEPLOYMENT_KIND`) in its device discovery document, so a client learns what an instance is from the instance itself.

## Quick Start (development)

```bash
# Create a Kind cluster for local development
make kind-create

# Install the CRDs into the cluster
make install

# Run the operator from your host against Kind's API server
make run

# In another terminal, declare a platform and watch it converge
kubectl apply -f config/samples/minimal.yaml
kubectl get plantonplatform -w
```

`make kind-deploy` builds the image, loads it into Kind, and deploys the operator in-cluster instead; `make kind-e2e-lite` does the whole lite journey in one command; `make kind-status` and `make kind-logs` read the result.

## Development

| Command | Description |
| --- | --- |
| `make manifests` | Regenerate the CRDs and the manager ClusterRole into `config/` AND into the Helm chart (the chart derives both; CI fails a stale chart) |
| `make generate` | Regenerate DeepCopy methods |
| `make test` | Unit tests, envtest, and the chart render test |
| `make test-e2e` | The Kind e2e suite on a dedicated cluster (`setup-test-e2e` / `cleanup-test-e2e` manage it) |
| `make test-chart-lifecycle` | The chart lifecycle suite on its own Kind cluster: fresh install, keep, reinstall, keep off, both upgrade paths from the last published charts |
| `make test-realm-convergence` | The Keycloak realm-convergence and federation suite (needs Docker) |
| `make lint` / `make lint-fix` | golangci-lint |
| `make docker-buildx` | Multi-platform image build |
| `make generate-manifests` | Refresh the embedded third-party manifests and chart archives the operator renders at runtime |

Run `make help` for every target with its description.

## Package Map

```
cmd/main.go                      Manager entry: floor logged, singleton guard, controller registration
api/v1/                          The PlantonPlatform and PlantonIdentityProvider types (README)
internal/controller/             The reconcile loop (README)
internal/component/              One file per platform component; the readiness rules
internal/resources/              Renders the Kubernetes objects each component owns (README)
internal/status/                 Status, conditions, and the version refusal (README)
internal/platformversion/        The platform-version floor and its verdicts
internal/singleton/              The one-operator-per-cluster guard
internal/bootstrap/              First-boot bootstrapping the operator performs for the platform
internal/keycloak/               Realm and client convergence for the bundled identity server
internal/keycloaklogintheme/     The sign-in and email themes served by the identity server (README)
config/                          Kubebuilder scaffolding: generated CRDs and RBAC, samples, kustomize
hack/                            Generators (the chart's CRD templates) and the lab directory fixture
test/chart                       The chart render test (inside make test)
test/chartlifecycle              The chart lifecycle suite (Kind)
test/e2e                         The kubebuilder e2e suite (Kind)
```

Packages marked (README) carry their own design notes.

## Related

- [The operator Helm chart](../helm/planton-operator/README.md): values, CRD lifecycle, upgrade paths.
- [The platform Helm chart](../helm/planton/README.md): declares a platform against a running operator.
- [Self-hosting Planton](../site/public/docs/self-hosting/index.md): the adopter-facing install and upgrade guide.
- [Repository architecture](../architecture/README.md): where the operator sits in the open-source repository.
