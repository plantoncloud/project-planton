# Kubernetes E2E Provider Harness

This package (`aa_e2e`) implements the E2E test harness for the Kubernetes
provider. It manages the test cluster lifecycle and delegates resource
verification to the `verify/` subpackage.

## Cluster Lanes

The harness supports two cluster lanes, selected by environment variables:

| Lane | Selection | Lifecycle |
|------|-----------|-----------|
| kind (default) | — | Persistent: `Setup` reuses a running cluster with the configured name and creates one only when none exists. `Teardown` leaves it running unless `PLANTON_E2E_DESTROY_CLUSTER=1` (set on ephemeral CI runners). |
| External cluster | `PLANTON_E2E_KUBECONFIG=<path>` (+ `PLANTON_E2E_CLUSTER_PROFILE=<profile>` declaring what the cluster IS) | Never touched: the cluster (real EKS/GKE/AKS, batch-provisioned once per test wave and reused) is owned outside the run. Bootstrap/teardown runbooks live under `realcluster/`. |

`PLANTON_E2E_KIND_CLUSTER_NAME` overrides the default kind cluster name
(`planton-e2e`). The name is stable across runs by design — a run-unique name
would defeat reuse.

Component-level verification is identical in both lanes: verifiers key off the
kubeconfig path, not the cluster's origin. Every test still deploys, verifies,
destroys, and verifies cleanup of its own resources — only the cluster outlives
the run.

## Cluster Profiles

Some scenarios need a cluster the default kind cluster cannot be: a Cilium
lane needs a cluster created WITHOUT a CNI (kindnet already owns pod
networking on the default cluster), and a NetworkPolicy enforcement proof
needs a CNI that actually enforces. A scenario opts into such a cluster by
annotation:

```yaml
metadata:
  annotations:
    planton.dev/e2e-cluster-profile: "cilium-cni"
```

| Profile | Cluster | Purpose |
|---------|---------|---------|
| (absent) | the default shared cluster | everything else |
| `cilium-cni` | `<base>-cilium`, single-node, `disableDefaultCNI: true` | Cilium-as-primary-CNI lanes; NetworkPolicy behavioral enforcement |
| `aws-eks` | REAL cluster (no local constructor) — batch-provisioned EKS via `realcluster/aws-eks/` | Cloud-LB provisioning, IRSA identity hops, snapshot-capable CSI storage, real node autoscaling |
| `gcp-gke` | REAL cluster (no local constructor) — an existing GKE cluster with Workload Identity; the GCP side (identities, bindings, bucket) created from the catalog via `realcluster/gcp-gke/` | Keyless GCS backups and restores through Workload Identity, multi-node HA under real anti-affinity, lanes beside resident operators |

Mechanics, all verified against the framework's own contracts:

- The test entrypoints route each scenario to its profile's harness at the
  single choke point where a scenario binds to a cluster, constructing the
  profile cluster LAZILY (runs that touch no profiled scenario never pay for
  it). Profile clusters are persistent, exactly like the default cluster.
- Both engines read cluster credentials through the process `KUBECONFIG`
  (Pulumi directly; the Terraform lane forwards it to `KUBE_CONFIG_PATH` per
  scenario), and scenarios within one test process run strictly serially —
  activating the kubeconfig per scenario is therefore race-free. Cross-
  terminal parallelism is separate processes with separate environments.
- A CNI-less cluster's nodes are NotReady BY DESIGN until the CNI installs,
  so the profile's create path waits on API-server readiness instead of
  kind's node gate; the NotReady→Ready transition is asserted by the Cilium
  install verifier as part of the proof.
- Profiled scenarios are MATCHED, never assumed: in the external-cluster
  lane a profiled scenario runs only when `PLANTON_E2E_CLUSTER_PROFILE`
  equals its profile and skips (with the reason) otherwise — a profile names
  what a scenario would DO to a cluster as much as what it needs from it,
  and running an unmatched profile on a shared real cluster can destroy it
  for every later lane (installing a primary CNI on a live EKS cluster, for
  example). Real-cluster profiles (`aws-eks`, `gcp-gke`) have no local
  constructor and skip with the reason on local runs.
- Real clusters carry RESIDENTS (the GKE management cluster runs
  cert-manager and CloudNativePG installed by another hand), and the
  singleton kinds forbid a second copy. Two per-scenario annotations declare
  the fit, documented in full in `e2e/README.md`:
  `planton.dev/e2e-resident-prerequisites: "Kind, ..."` prunes kinds the
  cluster already has (refused on scenarios not pinned to a real-cluster
  profile), and `planton.dev/e2e-prerequisite-install-manifest: "Kind=path"`
  substitutes a prerequisite's install profile for this lane only.
- Component profiles whose EVERY lane needs a real cluster carry the
  `real_cluster` status in `e2e/profile.yaml` (the Karpenter family): the
  entrypoints skip them wherever no external cluster is supplied, and the
  CI matrix (built from `green`) never schedules them on kind runners.
- Scenarios restricted to one engine by documented PARITY-EXCEPTION design
  carry the `planton.dev/e2e-engines` annotation (e.g. the PVC data-source
  scenarios: the Terraform provider cannot express data sources); the
  excluded engine's lane skips with the reason instead of failing on its
  own designed rejection.
- Unknown profile values fail the scenario loudly (never a silent
  wrong-cluster run). Lanes that target the same profile cluster serialize
  exactly like lanes sharing the default cluster.

## Why `aa_e2e`?

Every cloud provider in Planton can have an E2E harness colocated alongside
its component directories. The directory is named `aa_e2e` (not `_e2e` or
`e2e`) for two reasons:

- **Go ignores directories starting with `_`**. A directory named `_e2e`
  would be silently excluded from the Go build, meaning the package would
  never compile or be importable.
- **`aa_` sorts first alphabetically** across all providers (aws, azure, gcp,
  kubernetes, etc.), making the harness directory immediately visible in file
  explorers without needing to scroll past dozens of component directories.

This naming convention applies to all providers. When adding E2E support for
a new provider (e.g., AWS), create `catalog/aws/aa_e2e/`.

## Directory Layout

```
aa_e2e/
  harness.go    -- Kind cluster lifecycle (Setup / Teardown / VerifyDeployed / VerifyDestroyed)
  README.md     -- This file
  verify/       -- Manifest-driven resource verification (separate package)
    manifest.go           -- ManifestInfo struct + ParseManifestInfo
    verifier.go           -- ResourceVerifier interface, GetVerifierFromManifest dispatch, kind maps
    kubectl.go            -- kubectl helper functions (retry, backoff, resource exist/absent)
    namespace.go          -- NamespaceVerifier (Tier 1)
    workload.go           -- WorkloadVerifier (Tier 1 deployments, statefulsets)
    resource_existence.go -- ResourceExistenceVerifier (Tier 1 secrets, services)
    operator.go           -- OperatorComponentVerifier (Tier 4 operators)
    crd_workload.go       -- CRDWorkloadVerifier (Tier 3 operator-dependent CRD workloads)
    helm.go               -- HelmComponentVerifier (Tier 2 Helm-based apps)
    valkey.go             -- ValkeyVerifier (Valkey install + persistence/replication behavioral proofs)
    perconamysql.go       -- PxcClusterVerifier (Percona MySQL cluster + Galera durability/backup proofs)
    perconamongodb.go     -- PsmdbClusterVerifier (Percona MongoDB cluster + failover/backup proofs)
    perconaoperator.go    -- PsmdbOperatorInstallVerifier / PxcOperatorInstallVerifier (Percona operators)
    generic.go            -- GenericVerifier (fallback, always passes)
```

## Harness (`harness.go`)

The `Harness` struct manages a kind (Kubernetes IN Docker) cluster. The
`e2e/e2e_test.go` TestMain creates one shared Harness for all Kubernetes
tests in a single run.

- `Setup` creates the kind cluster, writes a kubeconfig to a temp file, and
  sets the `KUBECONFIG` environment variable so Pulumi's Kubernetes provider
  finds it automatically.
- `Teardown` deletes the kind cluster and cleans up temp files.
- `VerifyDeployed` and `VerifyDestroyed` delegate to `verify.GetVerifierFromManifest`
  which parses the test manifest and returns the appropriate verifier.

The Harness implements the `provider.Harness` interface from
`e2e/framework/provider/provider.go`, keeping the framework provider-agnostic.

## Verification (`verify/`)

Verification is **manifest-driven**: the verifier reads the test manifest YAML
at runtime, extracts the `kind`, `metadata.name`, and `spec.namespace`, and
selects the appropriate verifier type. This means adding a new test scenario
(a YAML file in a component's `v1/e2e/` directory) never requires touching Go
code.

### Verifier Types

| Verifier | File | Tier | Checks |
|----------|------|------|--------|
| `NamespaceVerifier` | `namespace.go` | 1 | Namespace exists / absent |
| `WorkloadVerifier` | `workload.go` | 1 | Deployment or StatefulSet exists in namespace / absent |
| `ResourceExistenceVerifier` | `resource_existence.go` | 1 | Secret or Service exists in namespace / absent |
| `HelmComponentVerifier` | `helm.go` | 2 | Namespace + running pods + services |
| `OperatorComponentVerifier` | `operator.go` | 4 | Namespace + running pods (no service requirement) |
| `CRDWorkloadVerifier` | `crd_workload.go` | 3 | Namespace + running pods + services |
| `ValkeyVerifier` | `valkey.go` | 2 | Valkey workload ready + write Service. Behavioral proofs: persistence (write a marker key, DELETE the pod, read it back after restart) and replication (write through the write Service, read back through the read Service) |
| `PxcClusterVerifier` | `perconamysql.go` | 3 | PerconaXtraDBCluster in state `ready` + proxy write Service. Behavioral proof: Galera durability (write a marker row through the proxy, DELETE a database node, read it back at full strength). Backup proof: drives a real XtraBackup to `Succeeded` in the declared store |
| `PsmdbClusterVerifier` | `perconamongodb.go` | 3 | PerconaServerMongoDB in state `ready` + replica-set Service. Behavioral proof: failover durability (majority-write a marker document, DELETE the primary, read it back through the newly elected primary). Backup proof: drives a real PBM backup to `ready` in the declared store |
| `PsmdbOperatorInstallVerifier` / `PxcOperatorInstallVerifier` | `perconaoperator.go` | 4 | Operator Deployment Available + CRDs Established, for both Percona flavors (MongoDB and MySQL). On destroy, asserts only the Deployment's absence — the CRDs intentionally survive uninstall |
| `GenericVerifier` | `generic.go` | -- | Always passes (logs a skip message) |

### Dispatch (`verifier.go`)

`GetVerifierFromManifest` uses three kind classification maps plus a hardcoded
switch for Tier 1 native resources:

- **`operatorKinds`** -- Tier 4 operator/controller components (namespace + pods)
- **`crdWorkloadKinds`** -- Tier 3 CRD workloads (namespace + pods + services)
- **`helmTier2Kinds`** -- Tier 2 Helm-based applications (namespace + pods + services)

New component kinds are added to the appropriate map. Tier 1 native resources
(namespace, deployment, statefulset, secret, service) are routed via the switch.

### Retry Strategy (`kubectl.go`)

All kubectl operations use retry loops with progressive backoff to handle
Kubernetes eventual consistency:

- **Existence checks**: 5 attempts, 2-second base backoff
- **Absence checks**: 10 attempts, 2-second base backoff (resources take
  longer to finalize)
- **Pod readiness**: 15 attempts, 3-second base backoff (images must
  be pulled, init containers must complete, CRDs must reconcile)
- **Service existence**: 10 attempts, 2-second base backoff

## Adding a New Provider Harness

When extending E2E testing to a new provider (e.g., AWS):

1. Create `catalog/aws/aa_e2e/`
2. Implement `harness.go` with provider-specific infrastructure lifecycle
3. Create a `verify/` subpackage with provider-specific verification logic
4. Implement the `provider.Harness` interface from `e2e/framework/provider/`
5. Register the new harness in `e2e/e2e_test.go` TestMain

## Test Manifests

Test manifests live colocated with their components at
`{component}/v1alpha1/e2e/*.yaml`, not in this directory. The test framework
discovers them automatically via `e2e/framework/discovery/`. Adding a new
test scenario means dropping a YAML file -- zero Go code changes.
