# Real-Cluster E2E Batches

Some Kubernetes E2E assertions need cloud fabric no local kind cluster can
provide: cloud load balancers, IRSA identity hops, snapshot-capable CSI
storage, real node autoscaling. Those scenarios carry a real-cluster value in
the `planton.dev/e2e-cluster-profile` annotation (e.g. `aws-eks`) — they skip
with a reason on local runs and execute against a batch-provisioned real
cluster through the harness's external-cluster lane.

## Lane contract

```bash
# 1. Provision the batch cluster + IRSA roles + batch test resources:
./aws-eks/bootstrap.sh

# 2. Source the generated env file (lane selection + identifier exports):
source ~/.planton-e2e/planton-e2e-eks/env.sh

# 3. Run targeted per-component lanes (NEVER tier-wide sweeps):
go test -tags=e2e -timeout=30m -v -count=1 -run 'Test.*KubernetesVelero_' ./e2e/

# 4. Tear down and verify zero residue:
./aws-eks/teardown.sh && ./aws-eks/audit.sh
```

The env file exports two kinds of variables:

- **Lane selection** — `PLANTON_E2E_KUBECONFIG` (adopts the external cluster)
  and `PLANTON_E2E_CLUSTER_PROFILE` (declares what the cluster IS, so
  profile-annotated scenarios are matched rather than assumed; a `cilium-cni`
  scenario can never run onto an EKS batch cluster by accident).
- **Batch identifiers** — `PLANTON_E2E_*` values (IRSA role ARNs, bucket,
  queue, zone id) that scenario manifests reference through
  `${E2E_ENV:PLANTON_E2E_*}` tokens. Committed manifests stay honest: they
  never hardcode one test account's identifiers.

## Why targeted invocations

Component-level create/verify/destroy is identical to the kind lanes; only
the cluster outlives the run. Run one component's entrypoints at a time:
capacity managers (Karpenter, ClusterAutoscaler) must never overlap, and a
tier-wide sweep wastes the batch on components already proven on kind.

## aws-eks batch

| Asset | Purpose |
|---|---|
| `cluster.eksctl.yaml` | Cluster shape: system + tainted `ca-scale` (min-0) node groups, OIDC, EBS CSI + snapshot-controller addons, Karpenter node-role identity mapping |
| `karpenter-prerequisites.cloudformation.yaml` | Node role, controller policies, interruption SQS queue — vendored from the pinned upstream Karpenter getting-started template |
| `bootstrap.sh` | Stack + cluster + discovery tags + IRSA roles + zone/secret/bucket/queue + storage classes + env file |
| `storage.yaml` | gp3 StorageClass + VolumeSnapshotClass (CSI snapshot lanes) |
| `teardown.sh` | Deletes everything, handling the non-obvious residues (zone records, bucket objects, tagged snapshots) |
| `audit.sh` | Enumerates every resource class by tag/name; fails on any survivor |

IRSA trust policies bind to each chart's **rendered** ServiceAccount name
(documented in the modules' `vars.go`/`locals.go`): `kube-system/karpenter`,
`cluster-autoscaler/cluster-autoscaler-aws-cluster-autoscaler`,
`velero/velero-server`, `keda/keda-operator`, plus the scenario-named
external-dns SA and the ESO store fixture's ServiceAccount. Renaming a
scenario that owns one of these means updating `bootstrap.sh` to match.

## gcp-gke batch

Runs against an EXISTING GKE cluster with Workload Identity (the batch never
creates or deletes the cluster). Everything the lanes need beside the cluster
is created FROM THE CATALOG through the CLI's set lane — one dependency-ordered
`planton apply -f <dir>` over the rendered manifests, references resolved
between them exactly as an infra chart would — so the batch is itself a proof
that the resource set the guides document composes.

| Asset | Purpose |
|---|---|
| `manifests/01-backup-identities.yaml` | Two `GcpServiceAccount`s: the keyless identity the Postgres pods assume, the keyed one PBM presents for MongoDB (`user_managed_key`) |
| `manifests/02-workload-identity.yaml` | `GcpGkeWorkloadIdentityBinding`s for the Postgres source and recovery clusters (KSA = cluster name) |
| `manifests/03-backup-bucket.yaml` | The GCS `GcpGcsBucket`, both identities granted `objectAdmin` AND `legacyBucketReader` |
| `manifests/04-r2-backup-store.yaml` | The Cloudflare R2 side: a `CloudflareR2Bucket` and a `CloudflareAccountApiToken` scoped to it with `Workers R2 Storage Bucket Item Write` — the databases' `r2` arms archive here |
| `bootstrap.sh` | Renders the placeholders, applies the set, reads the Mongo key and the R2 token's S3 pair from the set lane's node state, writes the kubeconfig and `env.sh` (mode 600) |
| `teardown.sh` | Empties the R2 bucket over the S3 API (R2 refuses to delete a non-empty bucket), then destroys every node in reverse order from its set-lane workspace |
| `audit.sh` | Enumerates the GCP and Cloudflare resource classes; fails on any survivor |

Inputs: `GCP_PROJECT_ID`, `GCP_REGION`, `KUBE_CONTEXT`, `CLOUDFLARE_API_TOKEN`
(with `Workers R2 Storage Write` and `Account API Tokens Write`), and
`CLOUDFLARE_ACCOUNT_ID`; the engines read their credentials ambiently (ADC for
GCP, the Cloudflare token for Cloudflare). Run the scripts with the shell's
`KUBECONFIG` unset (`env -u KUBECONFIG ./gcp-gke/bootstrap.sh`): the batch
writes its own kubeconfig and an inherited one changes what the lanes target.

```bash
env -u KUBECONFIG ./gcp-gke/bootstrap.sh
source ~/.planton-e2e/planton-e2e-gke/env.sh
go test -tags=e2e -timeout=60m -v -count=1 -run 'TestKubernetesPostgres_' ./e2e/
go test -tags=e2e -timeout=60m -v -count=1 -run 'TestKubernetesMongodb_' ./e2e/
./gcp-gke/teardown.sh && ./gcp-gke/audit.sh
```

The env file publishes `PLANTON_E2E_GKE_*`: the GCS bucket, both identities'
emails, the Mongo key, and the R2 side (`_R2_ACCOUNT_ID`, `_R2_BUCKET`,
`_R2_JURISDICTION`, `_R2_ACCESS_KEY_ID`, `_R2_SECRET_ACCESS_KEY`). The set lane
deploys the Cloudflare nodes from the PUBLISHED module, so `bootstrap.sh`
derives the token's S3 pair itself (Cloudflare's rule: the token id, and the
SHA-256 of the token value) — the same pair the token kind exports as
`r2_access_key_id` / `r2_secret_access_key`.
