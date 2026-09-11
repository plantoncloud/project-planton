# Planton Operator Helm Chart

Deploy the [Planton Operator](https://github.com/plantonhq/planton) to any Kubernetes cluster.

The Planton Operator watches for `PlantonPlatform` custom resources and deploys the full
Planton platform stack: PostgreSQL, Valkey, OpenFGA, Temporal, the control plane
monolith, and the web console. A single YAML file is all it takes to go from an empty cluster
to a running Planton instance.

This chart installs the operator and the definitions it serves (`PlantonPlatform`,
`PlantonIdentityProvider`), and owns their lifecycle: upgrading the chart upgrades the
schema with the operator that reads it. The platform itself is a `PlantonPlatform`
resource you create afterwards -- by hand, through GitOps, or as its own Helm release
with the [`planton` chart](../planton), which carries proven defaults and per-cloud
values files. Run exactly one Planton operator per cluster: a second installation
refuses to start and its log explains why.

## Prerequisites

- Kubernetes 1.24+
- Helm 3.x
- cert-manager, only for backups of the platform's database: the backup plugin mints
  its TLS through it. Without cert-manager the platform installs and runs; the
  `Backup` column says `Unavailable` and why until cert-manager arrives.

## Installation

The chart is published to GitHub Container Registry as an OCI artifact:

```bash
helm install planton-operator oci://ghcr.io/plantonhq/charts/planton-operator \
  --namespace planton \
  --create-namespace
```

Pin a specific chart version with `--version <x.y.z>`. The chart and the operator share
one version line: chart `x.y.z` deploys operator image `vx.y.z`, both published from the
same release tag (override the image with `--set image.tag=<tag>`). A checkout of this
directory is a development build whose version and image tag are placeholders; install
it with `--set image.tag=<published tag>` or an image you built yourself.

After the operator is running, create a `PlantonPlatform` resource to deploy the platform:

```yaml
apiVersion: planton.ai/v1
kind: PlantonPlatform
metadata:
  name: planton
  namespace: planton
spec:
  version: v0.0.45
```

`spec.version` names a Planton platform release as `vMAJOR.MINOR.PATCH`; the API server
refuses any other shape. The operator runs releases from a floor upward: a version older
than the oldest it supports is refused before anything is created, with the reason in the
resource's `MESSAGE` column and a `VersionSupported` condition, and a platform already
running is left untouched. The operator's first log line (`Platform version floor`) names
the floor. To run a custom build, keep `spec.version` at a release and set `image.tag` on
the component: the version names the contract, the tag names the bytes.

Apply it with `kubectl apply -f` and watch progress:

```bash
kubectl get plantonplatform -n planton -w
```

Or declare it as a Helm release with proven defaults:

```bash
helm install planton oci://ghcr.io/plantonhq/charts/planton --namespace planton
```

### Publishing Planton at a URL

External access is a friction ladder in `spec.ingress` -- each rung is one field
up from the last, and every rung is a working deployment:

```yaml
spec:
  ingress:
    enabled: true                # rung 1: URL auto-derived from the ingress
                                 #   controller's address (magic DNS), plain HTTP
    hostname: planton.corp.com   # rung 2: your hostname, plain HTTP
    ingressClassName: nginx      # optional; omit to use the cluster default
    tls:                         # rung 3/4: HTTPS
      secretName: planton-tls    #   EITHER a kubernetes.io/tls Secret you bring
      # issuer:                  #   OR a cert-manager issuer
      #   name: lets-encrypt
      #   kind: ClusterIssuer
```

**If your cluster's front door is a Gateway API Gateway** (Istio, Envoy Gateway,
Cilium, a cloud Gateway), point the platform at it instead of an Ingress class:

```yaml
spec:
  ingress:
    enabled: true
    hostname: planton.corp.com
    gatewayRef:
      name: main                 # your Gateway
      namespace: gateways        # its namespace (defaults to the platform's)
      sectionName: https         # optional: pin one listener
```

The Gateway stays yours; Planton never edits it. It attaches one route for the
hostname and reads your listeners: if an HTTPS listener already serves the
hostname, the platform advertises `https://` with no `tls` block. To have
Planton obtain the certificate instead, add `tls.issuer`: Planton issues it into
its own namespace, grants your Gateway's namespace permission to use it, and the
resource's status tells you the one `certificateRefs` line to add to your
listener (`tls.secretName` does not apply on this door). Until the Gateway
accepts the route, the status relays your Gateway controller's own reason.

Never route a Gateway of your own to the platform's port-forward Service: pages
load, but the platform still believes it lives at `http://localhost:8080` and
sign-in sends the browser there. Declare the Gateway on the platform instead.

One hostname serves the web console, the API the browser calls (under the `/rpc`
path of that origin), the keyless identity issuer's discovery documents (under
`/.well-known`), and inbound webhooks (under `/webhooks`). That one hostname is also
what the platform tells third parties about itself: a customer's own GitHub App is
pointed at the door's pages and webhook receiver, never at a hosted address. The
platform reports its URL in `status.consoleUrl` (the `URL` column of `kubectl get plantonplatform`) and
whether the internet reaches it in `status.reachability` (the `Reachability`
column; declared with `spec.ingress.reachability`, resolved from the door's shape
when left at `auto`), and the
ingress component's status explains any misconfiguration in plain language
(missing class, missing TLS secret, cert-manager not installed, a Gateway that
does not admit the hostname or the namespace).

## Configuration

| Parameter | Description | Default |
|-----------|-------------|---------|
| `crds.enabled` | Install and upgrade the `PlantonPlatform` and `PlantonIdentityProvider` definitions with this release | `true` |
| `crds.keep` | Keep the definitions (and every platform they define) when the release is uninstalled | `true` |
| `image.repository` | Container image repository | `ghcr.io/plantonhq/planton/operator` |
| `image.tag` | Container image tag | Chart `appVersion` |
| `image.pullPolicy` | Image pull policy | `IfNotPresent` |
| `replicaCount` | Number of operator replicas | `1` |
| `leaderElection.enabled` | Enable leader election for HA | `true` |
| `healthProbe.port` | Health check endpoint port | `8081` |
| `janitor.sweepInterval` | How often the operator re-checks for cluster-scoped objects it installed that no platform needs any more (see Uninstallation) | `10m` |
| `resources.requests.cpu` | CPU request | `10m` |
| `resources.requests.memory` | Memory request | `256Mi` |
| `resources.limits.cpu` | CPU limit | `500m` |
| `resources.limits.memory` | Memory limit | `512Mi` |
| `serviceAccount.create` | Create a ServiceAccount | `true` |
| `serviceAccount.annotations` | ServiceAccount annotations | `{}` |
| `serviceAccount.name` | Override ServiceAccount name | `""` |
| `nodeSelector` | Node selector for scheduling | `{}` |
| `tolerations` | Tolerations for scheduling | `[]` |
| `affinity` | Affinity rules for scheduling | `{}` |

## Architecture

The operator runs as a single Deployment and watches for `PlantonPlatform` resources
in any namespace. When a resource is created, the operator:

1. Deploys the data layer (PostgreSQL via CloudNativePG, Valkey)
2. Deploys supporting services (OpenFGA with authorization model, Temporal with schema)
3. Deploys the application layer (control plane monolith, web console)

The platform's own PostgreSQL can back itself up to an object store you own
(`spec.database.postgresql.backup`: an S3, GCS, Azure Blob, or Cloudflare R2 bucket,
keyless cloud identity preferred, a schedule, a retention policy). The operator
installs CloudNativePG's backup engine -- the Barman Cloud plugin -- beside
CloudNativePG whenever cert-manager is on the cluster, so every PostgreSQL deployed
through Planton can back itself up too; a `Backup` column on `kubectl get
plantonplatform` reads `Healthy`, `Deploying`, `Failing` (in the plugin's own words),
`Unavailable`, or `NotConfigured`, and a platform declared with
`spec.database.postgresql.recoverFrom` restores its database from another platform's
archive. A failing backup never takes a working platform out of `Ready`.

Each component is reconciled independently with explicit dependency tracking.
The operator reports per-component status, an aggregate `Ready` condition whose
message is the `MESSAGE` column, and a `VersionSupported` condition:

```
$ kubectl get plantonplatform
NAME      PHASE   VERSION   URL                          LICENSE     MESSAGE                              AGE
planton   Ready   v0.0.45   https://planton.example.com  Community   All enabled components are healthy   5m
```

### When something is stuck

A platform that is not Ready names the component, what is wrong, and what to do in
that same `MESSAGE` column -- for example
`console: image ghcr.io/plantonhq/console:v0.0.61 for container "console" of pod planton-console-7d9f-x1 cannot be pulled (ImagePullBackOff: manifest unknown) -- check that the tag exists ...`.
The full detail is on the resource:

```bash
kubectl get plantonplatform planton -n planton -o yaml   # every component: phase, reason, object, message, lastTransitionTime
kubectl describe plantonplatform planton -n planton      # one Warning Event per failure the platform entered, a Normal one when it recovered
```

Each component's `reason` is a stable word (`ImagePullFailed`, `CrashLooping`,
`OutOfMemory`, `Unschedulable`, `VolumeUnprovisionable`, `ContainerConfigInvalid`,
`ConfigurationRefused`, ...) and its `object` is the Pod, PersistentVolumeClaim, or Job to
`kubectl describe` next. Reasons that describe a boot still in progress (`StartingUp`,
`WaitingForSchema`, `VolumeProvisioning`, `WaitingForDependency`) are not failures and
raise no Event; a fresh install's control plane takes about two minutes to answer its
health check and Temporal's pods restart until its schema job finishes -- both read as
the wait they are. When the message prints a `kubectl logs` command, that log is the
component's own account.

## CRD Management

The operator's definitions (`PlantonPlatform` and `PlantonIdentityProvider`) are
resources of this release, rendered from `templates/crds/`:

- `helm install` creates them and `helm upgrade` upgrades them, so the schema the
  cluster enforces is always the one the installed operator was built against. A
  `helm rollback` rolls the definitions back with the operator, the same way.
- `helm uninstall` keeps them (`crds.keep`, default `true`) because deleting a
  definition deletes every resource of that kind -- every platform on the cluster.
  Set `crds.keep=false` only when that is what you want.
- `crds.enabled=false` renders none of them, for the one case where another
  release on the cluster already owns them (one operator per cluster).

**Source of truth:** the files in `templates/crds/` and the manager's permissions in
`rbac/manager-role.yaml` are controller-gen output, written by
`make -C operator manifests` in this repository from the operator's Go types and RBAC
markers (the CRD templates add only the `crds.enabled` guard and the keep annotation).
CI regenerates and diffs them on every change, so they are never edited by hand: change
the Go source, regenerate, and the chart follows in the same commit.

### Coming from a chart that installed the definitions once

Chart releases before 0.8.0 shipped the `PlantonPlatform` definition through Helm's
install-once `crds/` directory, which leaves it outside any release. Upgrading such an
install stops with a message from this chart that names the definition and repeats the
two commands below with your release name and namespace filled in. Run them, then
`helm upgrade` again; the release adopts the definition and upgrades its schema:

```bash
kubectl label crd plantonplatforms.planton.ai app.kubernetes.io/managed-by=Helm
kubectl annotate crd plantonplatforms.planton.ai \
  meta.helm.sh/release-name=<release> meta.helm.sh/release-namespace=<namespace>
```

## Uninstallation

The operator installs two kinds of things beyond the platforms it runs: for every
platform, a cluster-wide grant its control plane needs (a ClusterRole and
ClusterRoleBinding), and for the cluster, the shared database operator (CloudNativePG),
its backup plugin (Barman Cloud, when cert-manager is present), and the build engine
(Tekton Pipelines) the first platform needs and every later platform reuses. All are
taken back by the operator itself, so a full uninstall leaves nothing behind:

- Deleting a platform removes its own grant right away, and its own backup store and
  schedule with it (the archive in your bucket is yours and stays).
- Deleting the LAST platform on the cluster removes the backup plugin, CloudNativePG,
  and Tekton -- their definitions, admission webhooks, cluster RBAC, and namespaces --
  unless something else still uses them (a database of your own on that CloudNativePG,
  an object store of your own on the plugin, a pipeline run). Then they stay, and
  `kubectl describe crd clusters.postgresql.cnpg.io` (or
  `objectstores.barmancloud.cnpg.io`, `pipelineruns.tekton.dev`) carries an Event
  naming exactly what is using them and
  the two ways out: delete those objects and the operator finishes on its next pass,
  or keep the engine as your own. A CloudNativePG or Tekton this operator did not
  install is never touched.

Order matters only in one way: delete the platforms while the operator is still
running, then remove the operator release. An operator uninstalled first cannot
clean up after platforms deleted later.

```bash
# 1. Delete every platform (or `helm uninstall planton` for one installed by the
#    planton chart), then wait for the operator's sweep:
kubectl delete plantonplatform --all -A
kubectl get crd -l app.kubernetes.io/managed-by=planton-operator   # empty when done

# 2. Remove the operator. The definitions stay (crds.keep) so a later install of
#    the same release adopts them; nothing else of the operator's remains.
helm uninstall planton-operator -n planton

# 3. To remove the definitions too -- this destroys every PlantonPlatform still on
#    the cluster and the platforms they describe -- delete them after the release:
kubectl delete crd plantonplatforms.planton.ai plantonidentityproviders.planton.ai
```

---

© Planton. Licensed under [Apache-2.0](https://github.com/plantonhq/planton/blob/main/LICENSE).
