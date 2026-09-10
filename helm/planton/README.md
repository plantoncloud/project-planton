# planton

A Planton platform as a Helm release: this chart declares one
`PlantonPlatform` resource with proven defaults, and the
[Planton operator](../planton-operator) turns it into the running platform --
console, API, sign-in, and an in-cluster runner -- reachable over a single
`kubectl port-forward` command that the install prints for you.

The operator comes first. It brings the `PlantonPlatform` definition this
chart's resource needs, and Helm validates every resource of a release
against the cluster before creating any of them, so the two cannot share one
release:

```bash
helm install planton-operator oci://ghcr.io/plantonhq/charts/planton-operator \
  --namespace planton \
  --create-namespace

helm install planton oci://ghcr.io/plantonhq/charts/planton \
  --namespace planton
```

No values are required. The first person to open the console becomes the
administrator (the setup page asks for their email plus a setup code read
from the cluster). Installing this chart before the operator fails with
Helm's `ensure CRDs are installed first`; install the operator chart and
run it again.

Prefer to manage the `PlantonPlatform` resource yourself (GitOps, custom
manifests)? Apply it with `kubectl` -- the operator chart's NOTES print the
minimal one -- and skip this chart. Prefer one command that does both? The
Planton CLI's self-hosted install runs both charts in order.

## Values

```yaml
platform:
  name: planton          # name of the PlantonPlatform resource
  spec: {}               # the PlantonPlatform spec, passed through VERBATIM
```

`platform.spec` is not curated by this chart: every field the
`PlantonPlatform` CRD supports works here, today and in future operator
versions. `spec.version` defaults to this chart's `appVersion`. See the
[operator chart's README](../planton-operator/README.md) for the full
resource reference (exposure ladder, identity, runner, components).

## Per-cloud values files

The base install needs no values file on any cloud. The files below are
recipes for the graduation step -- publishing Planton at your own URL and
giving the runner a cloud identity -- with only universally-true facts active
(e.g. RKE2's bundled nginx ingress class) and everything account-specific as
annotated comments:

| File | For |
|---|---|
| [`values.eks.yaml`](values.eks.yaml) | Amazon EKS (ALB/nginx ingress, IRSA runner identity, Secrets Manager backend) |
| [`values.gke.yaml`](values.gke.yaml) | Google GKE (built-in `gce` ingress active, Workload Identity runner) |
| [`values.aks.yaml`](values.aks.yaml) | Azure AKS (application routing add-on, Entra Workload ID runner) |
| [`values.doks.yaml`](values.doks.yaml) | DigitalOcean Kubernetes (nginx ingress, credentials Secret runner) |
| [`values.rke2.yaml`](values.rke2.yaml) | RKE2 / Rancher (bundled nginx ingress active) |

Use them straight from GitHub:

```bash
helm install planton oci://ghcr.io/plantonhq/charts/planton \
  --namespace planton \
  --values https://raw.githubusercontent.com/plantonhq/planton/main/helm/planton/values.rke2.yaml
```

## Storage

Planton's data services (PostgreSQL, Valkey, the runner's
IaC-state volume, and any optional components you enable) all request
persistent volumes. Two prerequisites decide whether an install works out of
the box on your cluster:

1. **A StorageClass Planton can provision from.** Either your cluster has a
   default StorageClass whose storage driver is actually installed, or you
   pin one explicitly. A default class that merely *exists* is not enough --
   on some managed clusters (fresh EKS is the famous case) the default class
   points at a CSI driver nobody installed, and every volume request hangs.
2. **Volume sizes at or above your backend's minimum.** Some enterprise
   backends enforce large minimum volume sizes (some NAS systems refuse
   anything under 800Gi). Planton's defaults (1-10Gi per volume) are sized
   for ordinary clusters; on such backends set sizes explicitly.

Both are one block in the values -- the class and one size for every volume,
with per-component overrides when volumes should differ:

```yaml
platform:
  spec:
    storage:
      storageClassName: your-class   # every volume; omit to use the cluster default
      size: 800Gi                    # every volume; omit for per-component defaults
    # Per-component settings win over the storage block:
    # database:
    #   postgresql: {storageSize: 2Ti, storageClassName: fast-ssd}
    #   redis:      {storageSize: 1Gi}
    # runner:
    #   storageSize: 2Gi
    #   storageClassName: your-class
    # vault:
    #   storageSize: 2Gi
    #   storageClassName: your-class
```

Set storage before installing: Kubernetes fixes a volume's class and size at
creation, so changing them on a running install requires recreating the
volume (scale the component down, delete its PersistentVolumeClaim, let the
operator recreate it -- destroying that volume's data). The install preflight
checks the default class can provision, and if a volume ever sticks, the
`PlantonPlatform` component status names the exact problem and fix:

```bash
kubectl get plantonplatform planton -n planton -o jsonpath='{.status.components}' | jq
```

## Secrets

Every install ships with a bundled secrets manager (OpenBAO, the open-source
Vault fork), deployed and bootstrapped automatically: the operator
initializes it, unseals it after every restart, and registers it as the
organization's default secret store -- pasting a cloud credential into a
connection works on a fresh install with zero configuration. The unseal keys
and root token live in the `<name>-openbao-init` Secret next to the platform
(the Secret's annotation explains itself); teams that prefer to hold their
own keys set `spec.vault.initMode: manual` and run the unseal ceremony
themselves.

Preferring a cloud secret store is a layered choice, not an opt-out: declare
it and it wins (see `values.eks.yaml` for AWS Secrets Manager with pod
identity -- no stored keys at all). The vault still runs underneath, serving
the platform's own signing and encryption keys.

Opting out entirely is supported but deliberate:

```yaml
platform:
  spec:
    vault:
      enabled: false   # loses pasted-credential storage and keyless connections
```

## One operator per cluster

Run exactly one Planton operator per cluster; it watches every namespace and
serves every `PlantonPlatform` on the cluster, so this chart can be installed
many times (one platform per namespace) beside one operator. A second
operator refuses to start and its log names the conflict and the way out.

## Upgrades

The platform and the operator upgrade independently:

- **The platform version** is `spec.version` on the resource. Change it and
  `helm upgrade planton` -- the operator rolls the platform to the new version
  with its data intact. An operator runs platform releases from a floor
  upward; a version below it is refused in the resource's status (phase
  `Error`, the reason in the `MESSAGE` column) with nothing changed, and the
  fix is the version or an operator release built for it:

  ```bash
  helm upgrade planton oci://ghcr.io/plantonhq/charts/planton \
    --namespace planton --set platform.spec.version=<version>
  ```

- **The operator** upgrades through its own chart, which carries the
  `PlantonPlatform` definition with it:

  ```bash
  helm upgrade planton-operator oci://ghcr.io/plantonhq/charts/planton-operator \
    --namespace planton
  ```

### Coming from a release that bundled the operator

Releases of this chart before 0.4.0 installed the operator as part of the
same release and left the `PlantonPlatform` definition outside any release
(Helm's install-once `crds/` behavior). Moving such an install to the
two-release shape is three commands, in this order, and never runs two
operators at once. `<release>` is the name you will give the operator release
(`planton-operator` below) and `<namespace>` its namespace:

```bash
# 1. Hand the definition to the future operator release. The operator chart
#    refuses to take over a definition that belongs to no release, and its
#    message repeats exactly these two commands with your names filled in.
kubectl label crd plantonplatforms.planton.ai app.kubernetes.io/managed-by=Helm
kubectl annotate crd plantonplatforms.planton.ai \
  meta.helm.sh/release-name=planton-operator meta.helm.sh/release-namespace=planton

# 2. Upgrade this release: Helm removes the bundled operator and keeps the
#    PlantonPlatform resource. The platform keeps running, unmanaged, for the
#    minute until step 3.
helm upgrade planton oci://ghcr.io/plantonhq/charts/planton --namespace planton

# 3. Install the operator on its own release. It adopts the definition,
#    upgrades its schema, and resumes reconciling the platform.
helm install planton-operator oci://ghcr.io/plantonhq/charts/planton-operator \
  --namespace planton
```

## Teardown

```bash
helm uninstall planton -n planton
kubectl delete namespace planton
```

`helm uninstall` removes the `PlantonPlatform` resource and, through the
operator, the platform's own workloads (control plane, console, front door,
identity, runner). The PostgreSQL database goes WITH the platform: CloudNativePG owns
the database's volumes and credentials as one unit, so removing the platform
removes the database cleanly -- credentials can never survive apart from the
volumes they unlock. The remaining data-service volumes (the Valkey cache,
Temporal's chart-rendered claims) linger until the namespace is deleted,
which is why deleting the namespace is part of teardown. To REINSTALL,
always delete the namespace first: a new install against leftover volumes
would mint new credentials that the old data refuses. When this was the last
platform on the cluster, the operator also removes the shared CloudNativePG
and Tekton it installed for it -- unless something else still uses them, in
which case they stay and their definitions carry an Event saying what. The
operator and the `PlantonPlatform` definition belong to the `planton-operator`
release; see that chart's README for removing them after every platform on
the cluster is gone.

---

© Planton. Licensed under [Apache-2.0](https://github.com/plantonhq/planton/blob/main/LICENSE).
