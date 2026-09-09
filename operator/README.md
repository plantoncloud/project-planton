# Planton Operator

A Kubernetes operator that turns one `PlantonPlatform` resource into a complete, running Planton platform on any Kubernetes cluster, and keeps it converged.

## What It Does

The operator carries every deployment decision a self-hosted Planton needs -- which components exist, the order they come up in, their readiness gates, the credentials wired between them, and the first-boot seeding -- so an adopter declares a platform release and nothing else. It installs the prerequisite sub-operators (CloudNativePG, Tekton Pipelines), provisions the data services (PostgreSQL, Valkey, Temporal, the bundled OpenBAO secrets manager; OpenFGA and Neo4j when opted in), renders the control plane, console, in-cluster runner, and identity server, and reports one phase and one plain-language message per platform in `kubectl get plantonplatform`.

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

`config/samples/` holds a minimal declaration, a lite profile for Kind, and a full profile that exercises every optional arm.

## How It Reconciles

The controller in `internal/controller/` is a thin orchestrator. Each reconcile fetches the resource, initializes status when the spec's shape changed, judges `spec.version` against the operator's floor (`internal/platformversion/`; a refused version ends the reconcile without a requeue and touches no component), then walks every registered component in `internal/component/`: a component whose dependencies are not yet Ready is marked Pending with the dependency named, the rest reconcile their own resources and report Ready, Deploying, or Error. One status write per reconcile; a 30-second requeue for ongoing convergence.

A component backed by a Deployment is Ready only when its rollout has finished -- the spec observed, every desired replica on the current template, no stale replica remaining, every replica available -- so a version change never reads as Ready while the previous release still serves.

`PlantonIdentityProvider` has no controller of its own: a change to one re-enqueues the platforms in its namespace, and the identity component resolves the binding inside the same loop.

## The Front Door

A platform is reached through one public origin, rendered from one route table (`internal/resources/front_door_routes.go`) onto whichever door the declaration chooses: an Ingress object, a Gateway API HTTPRoute attached to a Gateway the cluster already runs, or the built-in nginx gateway served over `kubectl port-forward`. The browser API (gRPC-Web) lives under `/rpc`, the storage relay under `/storage`, the identity server under `/idp`, and the console answers everything else.

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
internal/keycloaklogintheme/     The sign-in theme served by the identity server (README)
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
