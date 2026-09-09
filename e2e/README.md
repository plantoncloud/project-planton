# Planton E2E Test Framework

End-to-end tests that deploy real infrastructure using Planton IaC modules and
verify the results against real providers.

## What This Framework Does

Every Planton component ships with Pulumi and Terraform modules that create
cloud infrastructure. These E2E tests prove that those modules actually work by
executing the full lifecycle against real providers:

1. **VALIDATE** -- load the manifest and build the stack input
2. **DEPLOY** -- run the IaC module (Pulumi up or Terraform apply)
3. **VERIFY-OUT** -- check that stack outputs are populated
4. **VERIFY-RES** -- confirm resources exist using provider-native tools
5. **DESTROY** -- tear down all created resources
6. **VERIFY-CLN** -- confirm resources are gone

If any phase fails, the framework still attempts DESTROY to avoid leaking
resources.

When a component has dependencies (see "Component Dependencies" below), the
framework wraps this lifecycle with a **DEPENDENCIES-UP** phase before VALIDATE
and a **DEPENDENCIES-DOWN** phase after VERIFY-CLN (teardown in reverse order).

**IDEMPOTENCY phase (per-provider opt-in):** when a provider's E2E profile
sets `assert_apply_idempotency: true`, every lifecycle gains an IDEMPOTENCY
phase right after DEPLOY: the runner re-plans the just-applied configuration
(`terraform plan -detailed-exitcode` must exit 0 / `pulumi preview
--expect-no-changes`) and fails the lane on any pending change. A dirty
second plan means the module and the provider disagree about applied state —
the send-omitted-value and Optional+Computed echo defect classes, which
users otherwise meet as a perpetual diff on every re-apply (first live
catch: the Identity Platform config's server-materialized
`sign_in.phone_number` block). The gate covers the component under test
only; prerequisite fixtures belong to other kinds' contracts. Arm it per
provider after the catalog's known no-op re-plan classes are burned down.

**The gate also catches OUTPUT-ONLY drift — an exported attribute whose
read-back form differs from its apply-time form.** `terraform plan
-detailed-exitcode` exits 2 for a plan whose only change is `Changes to
Outputs` (zero resource changes), and some provider attributes are stored
as the user's input at apply but read back normalized on the first refresh
— typically a short name flipping to the fully qualified resource path
(twice-seen: an Eventarc trigger's `name`, a Spanner instance's `config`
flipping to `projects/{p}/instanceConfigs/{c}`). The resource itself
re-plans clean (the provider diff-suppresses the attribute); only the
module OUTPUT drifts, which users meet as a perpetual "changes to outputs"
on every re-apply. The remedy is never to silence the gate: export a
module-derived value (built from spec/identity), or normalize the
attribute to the form the output contract documents — in BOTH engines, so
stack outputs stay byte-identical.

## Directory Layout

### Component E2E Structure

Test scenarios, profiles, and fixtures live **next to their components** at the
component root's `e2e/` level:

```
catalog/{provider}/{component}/
  e2e/
    manifest.yaml          <-- the canonical validated example manifest
    profile.yaml           <-- E2E profile (tier, status, provisioners, timeout)
    scenarios/             <-- test scenario manifests
      minimal.yaml
      with-probes.yaml
      with-hpa.yaml
    prerequisite.yaml      <-- optional: this kind's install profile, used when it
                               is itself a prerequisite of another component
  iac/
    pulumi/                <-- Pulumi module
    tf/                    <-- Terraform module
  v1alpha1/
    spec.proto
```

### Provider Harness Structure

Each cloud provider has a harness that manages test infrastructure and
verification, plus a provider-level E2E profile:

```
catalog/{provider}/aa_e2e/
  profile.yaml             <-- Provider E2E profile (credentials, substrate, tools)
  harness.go               <-- Provider lifecycle (setup/teardown)
  verify/                  <-- Resource verification logic
```

For Kubernetes, the harness creates a `kind` cluster and uses `kubectl` for
verification.

## Component Dependencies

Some components need other resources installed before they can be applied -- an
operator that owns their CRD, or the CRDs themselves. The harness deploys these
dependencies (via Pulumi) before the component under test and tears them down in
reverse order afterward, resolved by `ResolveDependencies`
([dependencies.go](framework/runner/dependencies.go)) from the proto registry:

Each kind declares its prerequisites in the proto registry
(`CloudResourceKindMeta.prerequisites` in `cloud_resource_kind.proto`). The
harness resolves them transitively and installs each one using, in order of
preference, a consumer-scoped override at the consuming component's
`e2e/prerequisites/<dep>.yaml` (for when the same prerequisite kind needs a
different install shape per consumer — e.g. GcpGlobalAddress as an EXTERNAL
VIP for a forwarding rule vs an INTERNAL VPC_PEERING range for a service
networking connection), then the dependency's `e2e/prerequisite.yaml` (its
published install profile), then its `e2e/scenarios/minimal.yaml`. Declaring
`prerequisites: [X]` is all that is needed -- no per-component wiring.

**Transitive prerequisites resolve against the component under test, not against
intermediate dependencies.** If kind A depends on B and B depends on C, the install
manifest for C is looked up under A's `e2e/prerequisites/c.yaml` (then C's published
profile), NOT under B's consumer-scoped overrides. So when B's install profile
references a specifically shaped C (e.g. a GKE cluster prerequisite referencing a
subnetwork with named secondary ranges), the consuming kind A must ship its own
consumer-scoped copy of that transitive prerequisite with matching `metadata.name`s —
B's consumer-scoped shape does not carry over.

**Cloud SQL chain note:** `GcpCloudSql` declares
`prerequisites: [GcpServiceNetworkingConnection]`, so every instance/database/user
scenario transitively installs VPC → VPC_PEERING range → connection before the
instance. The Cloud SQL kinds ship consumer-scoped overrides for those three
prerequisites with `${E2E_RUN_ID}` in the cloud-side VPC and address names (fixed
`metadata.name` for FK resolution) so parallel runs and stale orphans from a
half-finished teardown do not 409 on recreate.

*Example:* every Gateway API kind declares `KubernetesGatewayApiCrds`, so the
harness installs the Gateway API CRDs (experimental channel, version-pinned)
before applying a GatewayClass / Gateway / route / ReferenceGrant. The Tier 3
operator-dependent components (Postgres, Kafka, ...) likewise declare their
operator kind, which installs from the operator's `scenarios/minimal.yaml`.

### Dependency lifecycle robustness (asynchronous producer cleanup)

Two behaviors in the runner exist because cloud-side cleanup is not always
synchronous with the IaC engine's view of it (first hit: the Cloud SQL →
service networking connection chain):

- **Dependency destroys retry with backoff** (6 attempts, 60s apart, in
  [dependencies.go](framework/runner/dependencies.go)). Deleting a Cloud SQL
  instance returns minutes before the service producer releases its hold on
  the service networking connection; until then the connection destroy fails
  with "Producer services are still using this connection". Skipping past
  that failure is worse than an orphan: the framework would then force-delete
  the VPC, stranding a producer-side connection record that poisons the next
  scenario's same-named prerequisite chain (its connection create silently
  attaches to the stale record and no peering ever materializes).
  **A prerequisite whose producer release is measured in TENS OF MINUTES
  can declare its own wider budget** via `planton.dev/e2e-teardown-attempts:
  "N"` on its install manifest (total attempts, same 60s spacing) — the
  global default stays tight so genuinely failed teardowns keep reporting
  fast. Know the class boundary before reaching for it: the Cloud SQL
  chain's connection hold, historically ~4 minutes, was live-measured
  UNRELEASED after 43 minutes of solo-verified retries (2026-08-12) — for
  an EPHEMERAL fixture chain that unbounded wait buys nothing, so the
  fixture instead sets `deletionPolicy: ABANDON` on the connection: the
  teardown succeeds instantly, the (scenario+run-scoped, never-reused) VPC
  delete clears the consumer-side peering, and the producer record is
  unreachable, unbillable residue GCP reaps on its own schedule — the
  undeletable-classes zero-orphan posture, chosen at the fixture, never
  forced on customers (the kind's own DELETE semantics stay live-proven).
  The ghost-poisoning above was ALSO live-confirmed the same day: after a
  budget-exhausted teardown force-deleted the VPC, the NEXT scenario's
  same-named fresh chain deployed clean but its connection teardown failed
  "still using" with zero producer instances ever attached — the fix is
  scenario-scoped cloud-side names (`${E2E_SCENARIO}` beside
  `${E2E_RUN_ID}`) on the chain's VPC and range, so no scenario ever
  recreates a predecessor's name.
- **Dependency deploys run `pulumi up --refresh`** ([pulumi.go](framework/runner/pulumi.go)).
  Dependency stacks are keyed by run id, so every scenario in a run reuses the
  same stack name; if an earlier scenario's teardown half-completed, stale
  state would otherwise make a later `up` a silent no-op while the actual
  cloud resource is gone.

The SCENARIO's own DESTROY phase (distinct from the fixture-chain teardown
above) is single-attempt by default — a destroy failure is usually a real
module or provider bug and must fail loudly. A scenario whose resource class
carries a CLOUD-SIDE delete guard (the provider's DELETE is refused for a
bounded window no matter how correct the module is) opts into bounded destroy
retries with the reason-carrying `planton.dev/e2e-destroy-retry` annotation
on the scenario manifest ([runner.go](framework/runner/runner.go): full
destroy re-runs 60s apart under a 15-minute budget, each retry printing the
declared reason into the lane log). First user: Cloudflare email-routing
destination addresses, whose delete answers 400 code 2032 "created too
recently" until ~10 minutes after create (measured 2026-08-26: refused at
9m14s, accepted at 10m15s). An empty annotation value does not opt in —
state the guard, its error signature, and the measured window.

**Run ONE live-lane process at a time per provider account.** Two `go test`
processes hammering the same provider API concurrently can push a create into
the retry path of the provider's HTTP client: the first POST lands
server-side but the client sees a transient error, the automatic retry POSTs
again, and a non-idempotent create answers "already exists" (Cloudflare zone
error 1061, live-caught 2026-08-26 when two engine lanes ran as parallel
processes) — the lane then fails its OWN fixture create against its own
run-scoped name, which looks impossible from the log. Engine lanes within one
process are already sequential; keep processes sequential too.

Verifiers that read a resource through a *different* API than the one that
created it should poll briefly before declaring it absent (see the
service-networking-connection verifier: the peering is created via the
Service Networking API but read back through the Compute API, and that
cross-API view is eventually consistent).

**Dependency stack names are digest-capped — never truncate a stack name at a
call site.** `GenerateStackName` caps names at 50 chars by replacing the tail
with a short digest of the full name, because the tail is exactly where
uniqueness lives (a multi-instance install profile's `-a`/`-b`/`-c` instance
suffix and the run id). The live-caught failure class this kills: three
same-kind install-profile instances whose names truncated identically shared
ONE dependency stack, so each successive `pulumi up` silently REPLACED the
previous instance's cloud resource — the component under test then failed
with a stale resolved reference ("InvalidSubnet ... does not exist" moments
after the fixture "deployed and verified"), and teardown destroyed one stack
then burned its full retry budget on "no stack named" ghosts. That signature
— a fixture that verified cleanly, a component create rejecting the fixture's
id, and repeated "no stack named <truncated-name>" destroys — means stack-name
collision, not a module defect.

**Undeletable resource classes redefine "zero orphans".** Some GCP resources
have NO delete API — KMS key rings and crypto keys are the canonical class:
destroy removes a ring from state only, and destroys a key's *versions*
(disabling rotation) while the key object persists forever. For these
classes: (1) every cloud-side name MUST carry `${E2E_RUN_ID}` — the leftover
objects are permanent, so a fixed name can never deploy twice; (2) the
verifier's absent-check asserts the honest destroyed posture (all key
versions `DESTROYED`/`DESTROY_SCHEDULED`, rotation off) instead of absence;
(3) the post-run sweep expectation is "no ACTIVE material", not "no objects"
— run-scoped rings/keys accumulate in the test project as inert, zero-cost
residue by GCP design. Do not hand-sweep them; there is nothing to sweep.
(4) A component whose PREREQUISITE is undeletable gets ONE live scenario:
prerequisites redeploy per scenario with the same engine-scoped run id, so
a second scenario re-creates the just-"destroyed" (state-only) prerequisite
and 409s on its own leftover. Fold the arms into one scenario and record
the rest as offline-proven.

**Deletes that finalize asynchronously make FIXED cloud-side names 409 across
engines.** Some control planes acknowledge a delete before the name is
actually reusable (first hit: Cloud Scheduler jobs — the destroy returns, the
verifier's GET already 404s, yet a create of the same name minutes later can
still 409 `already exists`; Cloud Tasks goes further and documents a queue-ID
reservation of up to 7 days after deletion). The dual-engine runner recreates
every scenario back-to-back under both engines, so any scenario whose
cloud-side name is FIXED — including one that deliberately omits the explicit
name to prove the metadata.name fallback — sits exactly in this window. The
rule: the fallback-proof scenario carries `${E2E_RUN_ID}` in `metadata.name`
itself (the fallback then makes it the cloud-side name, proving the contract
without a fixed identifier); everything else carries the token in the spec's
explicit name field.

**Identifier classes that forbid hyphens take the `${E2E_RUN_ID_UNDERSCORE}`
token.** The engine-scoped run id carries a hyphenated engine suffix
(`-p`/`-t`), so the plain token cannot be embedded in fields whose character
class is letters/numbers/underscores only (first hit: a Vertex AI
`deployed_index_id`, which must start with a letter and forbids hyphens).
The runner expands `${E2E_RUN_ID_UNDERSCORE}` with every hyphen replaced by
an underscore ([tokens.go](framework/runner/tokens.go)); use it whenever a
run-scoped identifier lives in an underscore-only field. Do NOT fall back to
a fixed identifier to dodge the character class — see the next paragraph for
why that 400s.

**Some holds are keyed by a user-chosen SUB-resource ID and survive deleting
the parent.** A Vertex AI DeployedIndex that is in a failed state or still
undeploying holds its user-chosen `deployed_index_id` with a 400 ("It will
be deleted automatically, please retry again later or use a different ID")
— and the hold survives DELETING THE PARENT index endpoint, so neither the
dual-engine back-to-back recreate nor a fresh prerequisite chain escapes a
fixed ID. This is a distinct class from the fixed-NAME 409s above: the held
identifier is a spec field, not a resource name, so the run-scoped token
must go into the spec field itself (`deployedIndexId:
planton_oss_e2e_${E2E_RUN_ID_UNDERSCORE}` — the live-proven shape).

**Retries cannot cover releases the cloud documents in HOURS.** The retry
budget is bounded (~6 minutes) on purpose: some cloud-side holds outlive any
reasonable wait — a direct-VPC Cloud Run service leaves a serverless address
reservation (`serverless-ipv4-*`) in its subnetwork for **1-2 hours** after
the service is destroyed, and until GCP garbage-collects it the subnetwork
destroy fails with `resourceInUseByAnotherResource` (the reservation itself
cannot be deleted — it is held by the serverless service agent). A scenario
whose prerequisite teardown depends on such a release must NOT run live:
record it as an E2E exclusion in the component's `e2e/profile.yaml` with the
specific reason, and prove the surface offline instead. Fixed prerequisite
names make this worse (a stranded subnet collides with the next run), so any
scenario in an async-release blast radius should carry `${E2E_RUN_ID}` in its
prerequisites' cloud-side names. Probe-confirmed 2026-08-11: the hold applies
even to an IDLE service (never scaled, deleted seconds after create) and
equally to Cloud Functions Gen 2 (they run on Cloud Run infrastructure) — do
not expect a short-lived fixture to dodge it, and budget beyond the old
"1-2 hours" figure: the probe's reservation was still held at 2h35m and
released somewhere before the 4h mark. When in doubt, the 10-minute probe
is: scratch VPC + subnet, `gcloud run deploy --network/--subnet`, delete
the service, attempt the subnet delete and read the error.

### Resolving `valueFrom` references in composed scenarios

Before a scenario (or a prerequisite manifest) is applied, the runner resolves
every `valueFrom` reference in the spec against already-deployed prerequisite
outputs ([refresolve.go](framework/runner/refresolve.go)). Resolution walks the
full spec tree — nested messages, repeated message elements such as
`backends[0].group`, and repeated ref fields themselves (a `repeated
StringValueOrRef` like a target HTTPS proxy's `ssl_certificates` resolves each
list element in place) — and honors an explicit `valueFrom.kind` when the
field has no `default_kind` (e.g. a URL map's `default_service` pointing at
either a backend service or a backend bucket). Top-level refs with a
field-level `default_kind` behave exactly as before; existing manifests are
unchanged.

### Scenario-declared extra fixtures (optional composition seams)

Registry `prerequisites` carry a strict meaning -- the parents a resource cannot
exist without -- and they double as deploy-ordering metadata, so **optional**
composition seams must never be encoded there: adding an optional kind to a
registry prerequisite list would force every downstream kind's fixture chain to
deploy it forever. When a scenario needs to live-prove an optional edge (a
subnet attaching a route table, a NAT gateway associating a public IP, a
network peering to a second network), it declares the extra fixtures itself via
the `planton.dev/e2e-prerequisites` annotation on the scenario manifest:

```yaml
metadata:
  annotations:
    # Kind names install through the kind's standard install profile;
    # repo-relative manifest paths deploy an EXTRA INSTANCE of their declared
    # kind (for scenarios needing more instances than the profiles provide).
    planton.dev/e2e-prerequisites: "AzureRouteTable, AzureNetworkSecurityGroup"
```

Kind-name entries join the registry prerequisite graph and deploy in
topological order, expanded through their own prerequisite edges and
deduplicated against the chain (one resource group serves everything);
entries the chain already deploys are skipped. Manifest-path entries deploy
in listed order after the kind-driven chain, each preceded by any of its own
transitive prerequisites not already deployed -- and always deploy, because
they exist precisely to add another instance; a path entry never substitutes
for the kind's install profile. The ONE exception: a path entry naming the
very FILE the kind-driven chain already resolved (the typical slip:
annotating the consumer-scoped override of a registry prerequisite) is
skipped, because an identical manifest is never a meaningful extra
instance -- before the resolver deduped this, the duplicate deployed one
stack twice and BROKE the Pulumi teardown (the second destroy retries
against a stack the first destroy removed, "no stack named ... found"
through the whole ladder; live-caught 2026-08-26 on a DLM role fixture).
The Terraform path MASKS the class -- destroying an already-empty state
succeeds -- so a duplicate that passes offline TF proofs still fails its
first live Pulumi lane. The resolver also honors the same annotation
(kind names only) on each prerequisite's OWN install manifest, ordering the
fixtures an install profile's `value_from` references compose BEFORE the
declaring kind -- recursively and cycle-checked. All fixtures join the same
transitive `value_from` reference resolution the registry chain uses, and
teardown runs in reverse across the merged chain. Every kind that appears in
the annotation needs a verifier and an install profile, exactly like a
registry prerequisite.

**A path-declared fixture manifest's own annotation is NEVER read.** "Its
own transitive prerequisites" above means the path entry's KIND-level
registry edges only -- the resolver does not open a path-declared manifest
to read a further `e2e-prerequisites` annotation from it (that reading
happens only for the scenario under test and for install manifests). A
fixture chain that hops through extra instances (an extra instance A whose
`value_from` references another extra instance B, which references fixture
C) must therefore be declared IN FULL on the scenario under test, in
deploy-first order: `"C-kind, path/to/B.yaml, path/to/A.yaml"`. An
annotation left on the intermediate manifest is silently ignored -- the
lane fails at reference resolution with the hop's instance missing, which
reads like a naming defect and is actually a never-resolved fixture.

### Real-cluster lanes: substitute install profiles and resident kinds

Two more scenario annotations exist for lanes on REAL clusters, where the
chain must fit what is already running rather than build everything from
scratch. Both are per-scenario truths; consumer-wide profiles stay untouched.

```yaml
metadata:
  annotations:
    planton.dev/e2e-cluster-profile: "gcp-gke"
    # For THIS scenario, install the named kind from this manifest instead
    # of its consumer-scoped or published profile -- it takes the kind's
    # slot in the chain (a substitute, not an extra instance).
    planton.dev/e2e-prerequisite-install-manifest: "KubernetesCloudNativePgOperator=catalog/kubernetes/kubernetespostgres/e2e/prerequisites/kubernetescloudnativepgoperator.gke-plugin-only.yaml"
    # These kinds are already on the lane cluster: neither deployed nor
    # torn down, pruned from the chain together with their own edges.
    planton.dev/e2e-resident-prerequisites: "KubernetesCertManager"
```

`e2e-prerequisite-install-manifest` is `<Kind>=<repo-relative path>` entries.
It exists because a manifest-path entry in `e2e-prerequisites` only ever ADDS
an instance -- a scenario could never say "install this kind DIFFERENTLY
here". A GKE cluster that already runs CloudNativePG must install the
operator kind in its plugin-only posture (the backup plugin beside the
resident operator); that is the very shape under test, and no consumer-wide
profile can express a per-lane fact. The substitute's own edges expand
normally and its `e2e-prerequisites` annotation is read like any install
manifest's. The manifest must declare the kind whose slot it takes -- one
declaring another kind would leave the real prerequisite silently missing --
and a kind cannot be both substituted and resident.

`e2e-resident-prerequisites` is a promise about the cluster the scenario
targets (the batch EKS cluster carried its own load-balancer controller; the
GKE management cluster carries cert-manager and CloudNativePG from the
Planton operator), and deploying a second copy beside a resident is exactly
the collision the singleton kinds forbid. The runner cannot verify the
promise, so the Kubernetes entrypoint REFUSES the annotation on any scenario
not pinned to a real-cluster `e2e-cluster-profile` -- nothing is resident on
a cluster the harness creates -- and a false promise fails at deploy with
the missing resident's own error.

A dependency whose `pulumi up` FAILS is still tracked for teardown: a failed
update may have created any number of resources before erroring, and skipping
its destroy would orphan them -- and, because Azure-style parents refuse to
delete while children exist, a single orphaned fixture (say, a load balancer
holding a frontend in the fixture subnet) blocks the entire reverse teardown
chain behind it. Destroying a stack whose update failed is safe; it removes
whatever was actually created.

### Scenario-declared data-plane setup (the SETUP phase)

Some scenarios need an asset that no catalog kind can create because it is
DATA-PLANE CONTENT inside a fixture, not a control-plane object -- the first
user: Azure ML refuses to provision a managed online deployment without a
registered model, and a model registration is a file upload into the fixture
workspace, unreachable by any fixture manifest. Such a scenario declares a
setup script:

```yaml
metadata:
  annotations:
    planton.dev/e2e-setup-script: "catalog/azure/<kind>/e2e/scenarios/minimal.setup.sh"
```

The filename MUST end in `.setup.sh`. The repo's blanket `*.sh` gitignore would otherwise leave the script only on the machine that wrote it; a matching exception tracks `catalog/**/e2e/scenarios/*.setup.sh`. A different suffix is an untracked file and a broken lane on every fresh checkout.

The runner executes it as the `SETUP` phase -- after DEPENDENCIES-UP and
reference resolution (the fixtures the script seeds into exist), before
VALIDATE (a seeding failure stops the lane before any component deploy). The
script runs via bash from the repo root, once per engine lane, inheriting the
process environment (cloud CLI logins, the harness's `ARM_*`/`PLANTON_E2E_*`
exports) plus `E2E_RUN_ID` (engine-scoped), `E2E_SCENARIO`, and
`E2E_SETUP_OUTPUT` (below). A non-zero exit fails the lane; the dependency
chain still tears down.

**Publishing seeded facts to the manifest under test.** Some facts exist only
AFTER seeding and are exactly what the component must declare: the storage
path of the backup the script just took, the id of a snapshot it cut, a name
a fixture's controller generated. The script publishes them as `NAME=value`
lines (`NAME` matching `[A-Z][A-Z0-9_]*`) into the file at
`$E2E_SETUP_OUTPUT`, and the scenario manifest references them as
`${E2E_SETUP:NAME}` tokens, expanded after the script returns and before
VALIDATE. Every referenced token must be published (a residual one fails the
lane), values are single-line, and the expanded copy keeps the scenario's
basename so verifier dispatch is unaffected. This is the one channel by
which the data plane may inform the manifest -- the restore proofs' "restore
from THE backup the seed wrote" is the motivating class, and it keeps
committed scenarios honest: no manifest hardcodes a backup name only one run
ever produced.

```bash
# in gke-gcs-restore.setup.sh, after the backup reaches ready:
echo "MONGO_BACKUP_DESTINATION=$(kubectl get psmdb-backup "$name" -o jsonpath='{.status.destination}')" >> "$E2E_SETUP_OUTPUT"
```

Rules that keep the seam honest:

- **Control-plane objects never enter here.** Anything a catalog kind can
  create must keep entering through registry prerequisites or the
  `e2e-prerequisites` annotation -- those paths carry the teardown and
  orphan-sweep guarantees a script dodges.
- **Seed ONLY into fixture-owned resources.** There is deliberately no
  teardown pair: everything the script creates must die with the fixture
  chain at DEPENDENCIES-DOWN, or the zero-orphan sweep is lying.
- **Idempotent per lane** (an in-lane retry may re-run it), and any remote
  artifact it fetches is pinned (commit SHA + checksum) so the lane's inputs
  cannot drift under it.
- **Bash 3.2 portable.** The runner invokes `bash`, and macOS ships 3.2 --
  no associative arrays (`declare -A`), no `mapfile`. A `set -u` script
  that uses `[MLmodel]=` as an array key dies as "unbound variable"
  before any fetch (live-caught on the first SETUP-phase lane).

### Failure-mode lanes: a deliberate failure as machine-verified evidence

Some scenarios PROVE by failing: a component deployed with a credential a
real service rejects (the canonical case: a Planton runner appliance with a
fake enrollment token). The framework carries two annotation-activated
shapes, both dispatching to optional harness capabilities
(`provider.DeployFailureVerifier` / `provider.RuntimeCauseVerifier`,
implemented per kind through each provider's verify registry):

- **`planton.dev/e2e-expect-deploy-failure: <class>`** for substrates that
  gate resource creation on workload health (Cloud Run gates service
  creation on first-revision readiness). The lifecycle becomes VALIDATE ->
  DEPLOY-EXPECT-FAIL -> DESTROY -> VERIFY-CLN: the deploy MUST fail, the
  harness classifies the engine error and runs post-mortem assertions
  against the partially-created resource BEFORE destroy, and a deploy that
  unexpectedly SUCCEEDS fails the phase (with the cleanup destroy).
- **`planton.dev/e2e-expected-runtime-failure: <cause>`** for substrates
  that accept the deploy and fail asynchronously (ECS tasks, Container Apps
  replicas, Kubernetes pods). The standard lifecycle gains a VERIFY-CAUSE
  phase after VERIFY-RES.

The evidence bar is CAUSE-PINNING with the provider's own APIs -- "everything
worked except exactly the thing the scenario sabotaged", never a bare
tolerance of any failure. The runner appliance's lanes are the worked
example on all four substrates, and their first runs justified the bar
loudly: the arm caught a reserved `PORT` env rejected by Cloud Run, a
Cloud-Run `command` field silently REPLACING the image entrypoint
("Application exec likely failed" with zero container logs), an image
entrypoint that baked the subcommand and broke every consumer's argv, and a
Fargate fixture topology with no egress (ResourceInitializationError before
the container ever starts) -- every one invisible to provisioning-level
verification. Two authoring rules earned there: an expected-deploy-failure
harness implementation must STORE the manifest-derived identity after its
post-mortem (VERIFY-CLN has no VerifyDeployed state to reuse), and
runtime-cause classifiers should fail IMMEDIATELY on recognizable
wrong-cause states (pull failures) rather than polling them into a timeout.

### Lifecycle lanes: proving a component's second act

The standard lifecycle proves one install. Some promises are about what
happens NEXT -- a version bump re-applies what the module owns, a destroy
keeps what it must keep and the next install re-adopts it, a change the
module must refuse is refused before anything is touched. Three
annotations on a scenario extend the lifecycle; all three reuse the same
engine input binding the first deploy used, so a second manifest reaches
the engine exactly the way the first did (a fresh stack input for Pulumi, a
regenerated tfvars in the same working directory for Terraform):

- **`planton.dev/e2e-upgrade-manifest: <file beside the scenario>`** adds
  UPGRADE -> VERIFY-UPGRADED after VERIFY-RES: the second manifest (same
  `metadata.name`, changed fields) is deployed against the same stack, then
  the kind's verifier runs against the SECOND manifest, so a verifier that
  reads its expectations from the manifest (a chart version, a replica
  count) proves the upgrade took. DESTROY and VERIFY-CLN then run against
  the upgraded state.
- **`planton.dev/e2e-expect-upgrade-failure: <class>`** (with the upgrade
  manifest) turns the second deploy into UPGRADE-EXPECT-FAIL: it MUST fail,
  the harness's `DeployFailureVerifier` pins the class (the second manifest
  is on the context, so the verifier reads the refused values from it), and
  the FIRST manifest's inputs are restored before DESTROY so the teardown
  matches what was built.
- **`planton.dev/e2e-reinstall: "true"`** appends REINSTALL ->
  VERIFY-REINSTALLED -> DESTROY-AGAIN -> VERIFY-CLN-AGAIN after the first
  VERIFY-CLN: the same manifest deployed onto a cluster that may still
  carry what the first install deliberately kept.

The second-act manifest carries **`planton.dev/e2e-second-act: <first-act
file>`** so discovery never runs it as a lane of its own; it is reachable
only through the first act's annotation. Every kind on the catalog's CRD
primitive runs the same five shapes (the OpenTelemetry, Solr and OpenSearch
operators, and the generic Helm release on a CRD-bearing chart): `minimal`
keeps its CRDs and reinstalls onto them, the cleanup lane turns keep off so
the shared cluster is left clean, `upgrade` bumps the chart and checks the
CRDs' source-version stamp moved, `version-not-published` is refused at
deploy, `downgrade-refused` is refused at upgrade -- and the shared
refusal verifier (`verify.helmCRDLifecycle`) asserts the three-part
message (what was observed, what it means, the next step) on both engines.
The failure classes a scenario may name are `chart-version-not-published`,
`crd-schema-downgrade`, `helm-managed-crds` (a chart that templates
CRDs without Helm's keep mark, refused by the generic Helm kind unless the
spec accepts it), `chart-repository-unreachable` (the repository host does
not resolve from where the plan runs), `crd-owned-elsewhere` (a CRD the
module would apply already exists without the module's stamp; the refusal
names its owner and the verifier checks the CRD gained no stamp), and
`crd-apply-denied` (the deploy's identity may not write CRDs; the refusal
names the identity, the verb, and the module's `iac/permissions.yaml`, and
the verifier checks no CRD was stamped at the lane's version). The generic
Helm kind's chart is arbitrary, so its scenarios declare the CRDs they
expect the module to own (or a refusal to name) with
**`planton.dev/e2e-expect-crds: <comma-separated CRD names>`**; a scenario
on a chart without CRDs omits it.

Two of those classes need something only a lane can stage. The
owned-elsewhere lane plants its foreign CRD through a `KubernetesManifest`
fixture declared in `e2e-prerequisites` (a stub definition with the real
CRD's name and none of the module's stamps, standing in for a colleague's
`kubectl apply`), so the fixture chain deletes it at teardown and no
teardown script exists; it uses a chart no other lane keeps CRDs for on the
shared cluster (metacontroller), because a kept CRD would be re-adopted,
not refused. The denied lane runs as a different identity, below.

Before the verifier sees a deploy error, the runner passes the engine's
output through `pkg/failure.Explain` (the same step the CLI takes): a
module refuses in place wherever HCL can see the fact, and a failure that
kills the read itself (an unresolvable host, an API server answering
Forbidden) arrives as the provider's raw text and is explained by the
layer that runs the engine. Lanes therefore assert the exact text a CLI
user reads, on both engines.

### Deploying as a different identity: `planton.dev/e2e-identity`

A scenario may deploy as an identity the harness creates for it instead of
the harness's own administrative credentials:

```yaml
metadata:
  annotations:
    planton.dev/e2e-identity: "declared"
    # or, withholding named verbs from one resource (the core group is ""):
    planton.dev/e2e-identity: "declared-minus:apiextensions.k8s.io/customresourcedefinitions:create,update,patch,delete"
```

The value is provider-interpreted (`provider.IdentityProvisioner`). The
Kubernetes harness builds a ServiceAccount bound, through one ClusterRole,
to exactly the rules the component's `iac/permissions.yaml` declares
(`declared`), or those rules with the named verbs withheld
(`declared-minus`), mints a short-lived token, and hands the lane a
`self_managed` provider configuration that authenticates as it. The
configuration reaches both engines through the same stack-input path a
console deploy uses (Pulumi through the stack input; Terraform as
`KUBECONFIG`/`KUBE_CONFIG_PATH` written into the lane's working directory),
so nothing about the process environment changes and the fixture chain
keeps the harness's posture. The IDENTITY phase runs after SETUP and before
VALIDATE; the identity's objects are removed after the lane, whatever its
verdict. Rules bind cluster-wide whether the permissions file marks them
cluster scoped or not (the lane's namespace does not exist before the
deploy that creates it), so the identity is exact in verbs and resources
and wider than production only in scope. A withhold that another declared
rule grants anyway is refused up front: the generic Helm kind honestly
declares a wildcard for the arbitrary chart it installs, so a denied lane
must live on a typed kind whose rules are exact (the OpenSearch operator
carries `crds-apply-denied`). `declared` alone is how a kind proves its
least-privilege claim by running under it.

Two cluster facts the lanes must respect. Kept CRDs are cluster-scoped and
outlive their lane, so a lane that leaves CRDs at a HIGHER version than a
later lane pins would make that later lane's install a refused downgrade,
in whatever order the lanes happen to run: every lane that raises the
version turns `keepOnUninstall` off so it leaves the cluster as it found
it, and the one lane that keeps its CRDs pins the LOWEST version any lane
of that kind installs (one step below the default when the upgrade lane
starts there). And the Kubernetes `TestMain` isolates Helm's repository
configuration and cache per run (`runner.IsolateHelmEnvironment`): Helm
consults the machine's repository list even for a URL-addressed chart, and a
stale entry there fails every render and install with "no cached repo
found" -- a laptop's `helm repo add` from months ago must never fail a lane.

### A kind node caches `:latest` -- a re-pushed tag needs the cache cleared

The local kind cluster persists across suite runs and its containerd keeps
pulled images. When a lane's image rides a MOVING tag (the runner's
`:latest`) and the tag is re-pushed mid-session, the next lane silently
runs the STALE cached image (recognize it by a suspiciously fast DEPLOY);
`docker exec <kind-node> crictl rmi <image:tag>` clears it, and the re-run
pulls fresh. Real clouds resolve tags at create and are not exposed.

### Bare polymorphic references need an explicit `kind:` in scenario valueFrom

Reference resolution determines the referenced kind from the `valueFrom.kind`
field, falling back to the spec field's `default_kind` annotation. A **bare
polymorphic reference** (a `StringValueOrRef` deliberately carrying NO
`default_kind` because no kind dominates -- alias-record targets, diagnostic
setting targets, metric-alert scopes) has no fallback, and the resolver leaves
a kind-less reference on such a field UNTOUCHED rather than erroring. The
module then receives an unresolved ref, sends nothing (or an empty string),
and the failure surfaces at DEPLOY as a provider validation error that reads
like a module defect ("one of either X or Y must be specified"). In scenario
manifests, every reference on a bare polymorphic field must therefore carry
the explicit `kind:` alongside `name:` and `fieldPath:` -- no offline gate
catches the omission, because the manifest validates and the plan renders
without it.

## E2E Profiles

Profiles are KRM-style YAML files (`apiVersion: qa.planton.dev/v1`) that
declare how E2E tests are executed. The CI workflow reads these profiles to
dynamically generate the test matrix -- no hardcoded component lists.

### Provider Profile (`aa_e2e/profile.yaml`)

Configures provider-wide E2E behavior:

```yaml
apiVersion: qa.planton.dev/v1
kind: ProviderE2EProfile
metadata:
  name: kubernetes
spec:
  credential_approach: none
  test_substrate: kind
  default_cost_class: free_local
  default_schedule_lane: weekly
  required_tools: [kind, kubectl, pulumi, tofu]
  github_environment: e2e-kubernetes
  max_concurrent_tests: 8
```

### Component Profile (`e2e/profile.yaml`)

Declares a component's E2E readiness:

```yaml
apiVersion: qa.planton.dev/v1
kind: ComponentE2EProfile
metadata:
  name: kubernetesvalkey
spec:
  tier: 1
  status: green
  validated_provisioners: [pulumi, terraform]
  timeout_minutes: 25
```

Status values:
- **green** -- passes on CI, included in scheduled runs
- **deferred** -- known failure with documented reason, skipped in CI
- **skip** -- intentionally excluded (needs cloud credentials, etc.)
- **stub** -- module is a stub with no real deployment logic

The status is enforced at two layers: CI matrices are built from `planton
e2e discover` filters, and the provider test runners load the component's
profile and `t.Skip` any non-green component (with the profile's
`deferred_reason` in the skip message) -- so a full-provider suite run
never fails on a documented deferral.

## Discovering Components

The `planton e2e discover` CLI command scans profiles and displays component
readiness:

```bash
# Interactive TUI (default in terminal)
planton e2e discover --provider kubernetes

# Plain table (default when piped)
planton e2e discover --provider kubernetes --output table

# GitHub Actions matrix JSON (for CI consumption)
planton e2e discover --provider kubernetes --output github-matrix

# Filter to GREEN Pulumi Tier 1 only
planton e2e discover --provider kubernetes --status green --tier 1 --provisioner pulumi
```

## Running Tests

Prerequisites: Docker running, `kind`, `kubectl` installed, plus at least one
of `pulumi` (for Pulumi E2E) or `tofu`/`terraform` (for Terraform E2E).

```bash
# All Kubernetes E2E tests (Pulumi + Terraform, all tiers)
make e2e-test-kubernetes

# Pulumi-only, single component
make e2e-test-component component=KubernetesNamespace_Pulumi

# Terraform-only, Tier 1
make e2e-test-kubernetes-terraform-tier1

# Terraform-only, single component
go test -tags=e2e -timeout=30m -v -count=1 \
  -run "TestKubernetesNamespace_Terraform/minimal$" ./e2e/...
```

### Pulumi CLI minimum version: the destroy path passes `--run-program`

The runner's `pulumi destroy` invocations pass `--run-program` (it keeps the
program available during destroy so BeforeDelete/AfterDelete resource hooks
fire). Older Pulumi CLIs do not know the flag, and the failure mode is nasty:
every phase up to and including VERIFY-RES passes, then DESTROY fails
instantly with `unknown flag: --run-program` -- so the lane fails AFTER
creating real cloud resources, whose stack state lives in the run's temp
backend and is discarded when the process exits. The resources must then be
swept by hand (`az group list` / the provider's own list commands) before a
re-run. Verified live: v3.137.0 fails exactly this way; v3.256.0 works.
Check `pulumi destroy --help | grep run-program` before the first lane on
any machine, and upgrade the CLI rather than editing the runner -- the flag
is load-bearing for delete-hook correctness.

### Sensitive stack outputs: the Pulumi output reader passes `--show-secrets`

The runner reads Pulumi outputs with `pulumi stack output --json
--show-secrets`. The flag is load-bearing: without it Pulumi masks every
secret output as the literal string `[secret]`, which corrupts a
sensitive SCALAR output silently (the sentinel populates the proto field
and counts as verified) and fails a sensitive MAP output's JSON parse
with `invalid character 's' looking for beginning of value` (first live
hit: a name-keyed `authorization_keys` map — the transform skipped the
field and VERIFY-OUT under-reported N-1/N populated). The Terraform path
reads real values (`terraform output -json` includes sensitive outputs),
so the flag keeps both engines' verification bar identical. Dependency
output capture rides the same helper, so without the flag a scenario's
`value_from` reference to a FIXTURE's sensitive output resolves to the
sentinel and deploys silently wrong. Runner code never prints raw output
values (counts and names only), so unmasked values stay out of logs —
keep it that way when touching output-path logging.

### Terraform binary selection

Terraform E2E defaults to `tofu` (OpenTofu), matching the Planton CLI.
To use HashiCorp Terraform instead:

```bash
PLANTON_E2E_TF_BINARY=terraform make e2e-test-kubernetes-terraform-tier1
```

### Background suite lanes: smoke-check the launch, never trust nohup blindly

Long real-cloud suites are best launched as background processes writing to a
log file (a crashed observer then loses only observation, never the run). One
failure mode recurs: a `nohup`-spawned child of a short-lived shell can be
reaped with its parent BEFORE the suite starts, leaving a zero-byte log and no
error -- the launch "succeeds" and nothing runs. After launching any background
lane, smoke-check it within a minute: the PID must still be alive AND the log
must be non-empty (the harness prints its authentication line first). If the
process is gone with an empty log, relaunch under a supervised/managed shell
rather than retrying `nohup` from another transient shell.

A harder grain of the same class under agent tooling (live-caught
2026-08-13, twice in one hour): detachment does NOT guarantee survival.
Both a plain `&` child AND a double-forked `(nohup ... &)` process were
killed when the agent tool-shell that launched them was cleaned up -- one
died mid-scenario minutes after launch (orphaning a half-deployed
resource outside any engine state), the other silently stopped an
evidence watcher between lanes. The reliable shape is the agent harness's
own MANAGED background terminal (the launch stays the foreground command
of a persistent session the harness supervises), where both survived the
full session. Signature of the class: a lane log that simply stops
mid-phase with no `ok`/`FAIL` trailer and no error, while an identical
managed relaunch runs clean.

### `AzureCLICredential: signal: killed` is host starvation, not Azure

The Azure verifiers authenticate through the ambient `az` login, which
shells out to `az account get-access-token` on every credential refresh.
On a machine under extreme CPU load (observed live: 1-minute load average
in the hundreds while a concurrent whole-tree build ran on the same host),
that child process can be killed by the OS or its watchdog before it
answers, and the lane fails with
`verify-exists failed ...: AzureCLICredential: signal: killed` -- typically
on a fixture that deploys fine, minutes into a slow dependency chain. This
is a MACHINE class, not a credential or module defect: `az account show`
still works once load recedes. The tell to check is `uptime` -- if the
1-minute load average is far above the core count, wait for the spike to
pass (do not relaunch into it; the retry burns the whole chain again), then
re-run only the failed scenario. Memory can read healthy throughout; CPU
scheduler starvation alone is enough to trigger it.

### Long-running Azure components (AKS)

AKS clusters take roughly 5–10 minutes to create and a similar time to delete.
Component E2E profiles for `azureakscluster` and `azureaksnodepool` set
`timeout_minutes: 60–75`. When invoking tests directly, size `-timeout` beyond
the default 30m:

```bash
go test -tags=e2e -timeout=90m -v -count=1 \
  -run 'TestAzureAksCluster_Pulumi/minimal' ./e2e/azure/...
```

### Long-running Azure components (VPN gateways)

Classic virtual network gateways are the suite's slowest single
resource: measured live (VpnGw1AZ, eastus), the create ran **36m** and
the delete **15m** — a full single-engine lane (fixture chain up, deploy,
verify, destroy, verify-gone, chain down) totals **~60m**. Budget
`-timeout=120m` per engine, and budget the SAME cycle inside any lane
whose prerequisite chain deploys the fixture VPN gateway (the gateway
connection's does). Lanes for gateways in the same VNet must run
SEQUENTIALLY — ARM allows one VPN-type gateway per VNet, so the scenario
gateway and the fixture gateway can never coexist.

Burstable VM sizes in `eastus` may support only availability zone `1` — multi-zone
lists fail with `AvailabilityZoneNotSupported`. AKS E2E scenarios in this repo
use `zones: ["1"]` for the test subscription.

### Long-running Azure components (ExpressRoute circuits)

An ExpressRoute circuit CREATE is a slow ARM long-running operation:
measured live at **~17-19 minutes** per create (Equinix / "Washington
DC", 50 Mbps metered, Standard SKU -- consistent across four
consecutive creates on both engines), while the delete completes in a
minute or two. Budget it in every lane whose prerequisite chain deploys
the fixture circuit (the circuit-peering lane pays it inside
DEPENDENCIES-UP), and note the peering's own DELETE runs ~7-8 minutes.
The circuit-family component profiles carry `timeout_minutes: 45` for
exactly this class.

### Long-running Azure components (Virtual WAN hubs)

A Virtual WAN hub create is dominated by its managed router reaching a
Provisioned routing state: measured live (Standard hub, eastus/eastus2),
the create ran **17-35 minutes** (plain fixture hub 17-20m; a hub whose
scenario composes a route map ran ~35m -- the route-map child ALONE ran
**~17 minutes**, because the provider polls the router state before and
after the child create) and the delete **11-15 minutes**. A full
single-engine hub lane totals **~50 minutes**; budget `-timeout=120m`
per engine. Budget the SAME hub cycle inside any lane whose prerequisite
chain deploys the fixture hub -- the hub-connection, ExpressRoute-gateway,
and vWAN VPN-gateway families all pay ~17-20m up and ~13m down for the
fixture before their own resource starts. One hub per region per WAN:
scenario hubs must live in a different region than the fixture hub
(eastus2 vs eastus in this repo) so overlapping lanes can never collide.

### Long-running Azure components (ExpressRoute gateways in vWAN hubs)

An ExpressRoute gateway in a Virtual WAN hub is the vWAN family's second
slow class: measured live (one scale unit, eastus), the create ran
**~27 minutes** and the delete **~13 minutes**, and the gateway bills
(~$0.42/hr per scale unit) from creation. Its lane also pays the full
fixture-hub cycle first (see above) -- a single-engine lane totals
**~70-75 minutes**; budget `-timeout=180m` per engine.

### Long-running Azure components (vWAN VPN gateways)

A site-to-site VPN gateway in a Virtual WAN hub is the vWAN family's
third slow class: measured live (one scale unit, eastus, consistent
across four consecutive creates on both engines), the create ran
**32-36 minutes** and the delete **11-13 minutes**, and the gateway
bills (~$0.36/hr per scale unit) from creation. Its lane also pays the
full fixture-hub cycle first (see the Virtual WAN hubs section) -- a
single-engine gateway lane totals **~80 minutes**; budget
`-timeout=180m` per engine (more with the import round-trip enabled).
The VPN-gateway-connection lane pays the ENTIRE family inside
DEPENDENCIES-UP (hub ~18m + gateway ~36m deploy-and-verify) before its
own minutes-fast tunnel -- its lane totals ~90 minutes per engine;
budget `-timeout=210m`. ARM allows ONE VPN gateway per hub: the
fixture gateway occupies the fixture hub's slot, so the gateway and
connection lanes must run SEQUENTIALLY, and a wedged gateway teardown
blocks every subsequent lane needing that slot until swept.

### Long-running Azure components (point-to-site VPN gateways)

A point-to-site VPN gateway in a Virtual WAN hub is the vWAN family's
fourth slow class, timing-identical to its site-to-site sibling:
measured live (one scale unit, eastus, consistent across both
engines), the create ran **32-33 minutes** and the delete **11-14
minutes**, and the gateway bills (~$0.36/hr per scale unit) from
creation. Its lane pays the fixture-hub cycle first (hub ~17-23m up,
~14m down) plus a seconds-fast fixture VPN server configuration -- a
single-engine lane totals **~80 minutes**; budget `-timeout=180m` per
engine (240m with the import round-trip enabled). ARM allows ONE P2S
gateway per hub -- a slot SEPARATE from the hub's site-to-site VPN
gateway slot, so a P2S lane and an S2S lane never collide on the slot,
but two P2S lanes sharing a fixture hub must run SEQUENTIALLY, and a
wedged gateway teardown blocks the hub's P2S slot AND the hub's own
deletion until swept.

### Long-running Azure components (ML managed online deployments)

A managed online deployment provisions a real VM (no scale-to-zero).
Measured live (one Standard_F2s_v2, eastus, both engines): create
**17-18 minutes**, delete **~6 minutes**. The lane also pays the
fixture workspace chain plus a fixture endpoint (~10-12 min up,
~8 min down) and a SETUP-phase model registration (~20 s). A
single-engine lane totals **~42-44 minutes**; budget `-timeout=90m`
per engine. The instance bills from provisioning to destroy.

Azure's MFE can answer the create LRO with `InternalServerError`
("Internal error. Please see troubleshooting guide") in under a
minute -- `percentComplete: 0`, no field detail. The same body
succeeds on retry (and succeeded on the other engine minutes
earlier). Treat that 500 as a transient service fault: wait out
the failed lane's teardown, retry once, do not debug the module.
A second identical 500 is the finding (quota, image-build, or a
real body defect) and gets its own recorded boundary.

### A dirty `e2e/profile.yaml` on a shared checkout is a LIVE proof lane's state

The proof workflow flips a component's profile `pending_proof` -> `green`
immediately before its lanes and keeps the flip UNCOMMITTED until the
session's wrap-up commit -- so on a checkout shared by concurrent agent
sessions, an uncommitted `status: green` on a pending-proof component is
the signature of a proof lane running RIGHT NOW, not stray drift. A
concurrent session that discards it (observed live: an authoring session's
wrap-up reset the flip mid-lane) makes the proof session's next lane
silently skip with "profile status is pending_proof". Before discarding
any dirty profile you did not edit, check for a live proof session; the
proof session guards itself by re-checking the profile status right before
each lane launch.

### Offer-restricted services on free/PAYG subscriptions (probe, don't roulette)

Some Azure database services restrict provisioning PER REGION on free-tier
and new pay-as-you-go subscriptions: the ARM API returns
`LocationIsOfferRestricted` (PostgreSQL/MySQL Flexible Server) or
`ProvisioningDisabled` (Microsoft.Sql logical servers) in the restricted
regions. Verified on the test subscription: `eastus`, `eastus2`, and
`westus2` are blocked for PostgreSQL while `westus3`, `centralus`,
`canadacentral`, `northeurope`, and `uksouth` are clean — the restriction
is REGIONAL, not subscription-wide, so do not record a deferral after two
failures; find a clean region instead.

For PostgreSQL the per-region flag is queryable UP FRONT — probe before
picking (or moving) a scenario region:

```bash
az rest --method get \
  --url "https://management.azure.com/subscriptions/{sub}/providers/Microsoft.DBforPostgreSQL/locations/{region}/capabilities?api-version=2024-08-01" \
  --query "value[0].restricted"   # "Disabled" == provisioning allowed
```

Each RDBMS carries its OWN restriction footprint — do not assume they
match. Verified live on the test subscription: `westus3` is clean for
PostgreSQL but blocked for MySQL (`ProvisionNotSupportedForRegion`),
while `westus2` is blocked for PostgreSQL but clean for MySQL. MySQL's
capabilities endpoint is itself the probe, just with a different signal:
a restricted region answers 500 `InternalServerError`, a usable region
returns its edition ladder:

```bash
az rest --method get \
  --url "https://management.azure.com/subscriptions/{sub}/providers/Microsoft.DBforMySQL/locations/{region}/capabilities?api-version=2023-12-30" \
  --query "value[0].supportedFlexibleServerEditions[].name"
```

Microsoft.Sql has no public equivalent — fall back to one live attempt.
Verified on the test subscription: `eastus` is blocked for Microsoft.Sql
logical servers (`ProvisioningDisabled`) while `westus3` and `centralus`
are clean.

Microsoft.Sql adds its own TRANSIENT failure shape on top of the
restriction class: a logical-server create in a CLEAN region can return
`OperationTimedOut` ("The operation timed out and automatically rolled
back. Please retry the operation."). The rollback is asynchronous — for
several minutes afterward the half-created server is invisible to
`az sql server show`/`az resource list` yet still blocks its resource
group's deletion (`ResourceDeletionFailed`), and an explicit
`az sql server delete` answers `DropLogicalServerAlreadyInProgress`. A
`pulumi destroy` of the resource-group stack issued during that window
can hang far past the RG-delete norm. Treat it as transient: kill the
hung destroy if needed, wait for the in-progress drop to finish (the RG
delete then succeeds), and retry the lane — do not debug the module.
A server may live in a different region than its resource group, so the
shared fixture RG serves unchanged when a scenario pins a different
region. True subscription-wide gates do exist (the quota-increase link
in the ARM error is the fix) — but conclude that only after a
probe-verified clean region also fails.

App Service adds its own shape: new PAYG subscriptions carry ZERO
Basic-tier VM quota in some regions, and the rejection is a misleading
`401 Unauthorized` whose body says "Operation cannot be completed
without additional quota ... Current Limit (B1 VMs): 0" — a quota
signal wearing an auth status code, not a credential problem. The
restriction is per-region (verified on the test subscription: `eastus`
blocked, `westus3` clean) and the cheap probe is a one-off
`az appservice plan create --sku B1` in a throwaway resource group. An
App Service PLAN pins the region for every app on it — apps must live
in their plan's region — so re-regioning an app scenario means
re-regioning its plan fixture (the fixture resource group serves
unchanged; a plan may live in a different region than its group).

Cosmos DB adds a THIRD failure shape to this class: transient CAPACITY
rejection rather than offer restriction. `eastus` answered
`ServiceUnavailable` ("high demand in East US region for the zonal
redundant (Availability Zones) accounts") for a plain single-region
account that never asked for zones — Azure routes new accounts through
zonal placement internally, so the error hits non-zonal manifests too.
There is no up-front probe (the provider `locations` list still
advertises the region); treat `ServiceUnavailable` on Cosmos account
creation as a region-capacity signal and move the scenario-local
accounts to a quieter region (`westus3` verified clean) instead of
recording a deferral or filing quota requests.

Cosmos DB SQL data-plane RBAC (`cosmosdb_sql_role_assignment`) adds a
Pulumi-module constraint azurerm does not surface: pulumi-azure enforces
a 24-character logical resource name on `SqlRoleAssignment`, and when
the Azure `name` (a GUID) is unset it autogenerates from that logical
name — producing invalid non-UUID values like `mainaa5aa87`. Forge the
Pulumi module to (1) use a short logical name (`"main"`, mirroring the
Terraform module's single resource) and (2) generate an explicit UUID
for the Azure `name` when the spec does not pin one, matching azurerm's
create-time UUID generation. The sibling `SqlRoleDefinition` needs the
same explicit-GUID treatment for `role_definition_id` when unset; its
logical name can stay on `metadata.name`.

### Plan-gated Cloudflare products: probe the wall, run on an operator-arranged paid zone

Several Cloudflare products refuse EVERY create on a free-plan zone, each
with its own error shape (all measured live 2026-08-27): Snippets answer a
code-less "snippets are not allowed", standalone Health Checks answer 400
code 1002 "health checks disabled for zone", Waiting Rooms answer 400 code
1034 "Zone not entitled to this functionality". Two disciplines: (1) probe
the wall on BOTH a throwaway zone and the operator's sanctioned ACTIVE zone
before concluding -- the same refusal on both proves plan tier, a
difference proves delegation state (the DNSSEC precedent); (2) when the
operator arranges a paid-plan zone, the lanes run on it through the
env-gated arm (`planton.dev/e2e-required-env` +
`${E2E_ENV:PLANTON_E2E_CLOUDFLARE_ZONE_ID}`) -- run-scoped fixture zones
are always free-plan, so a registry zone prerequisite cannot carry a
paid-product lane, and any registry-installed fixture of a plan-gated kind
(the snippet fixture consumed by snippet rules) must itself reference the
env-injected zone. Products the account's plans cannot reach stay recorded
entitlement deferrals with the plan named as the unblock.

Five refinements to the same class (all measured live 2026-08-27):

- **The entitlement refusal can wear QUOTA clothing.** Logpush job creation
  below Enterprise answers 403 code 1004 "creating a new job (for
  http_requests dataset) is not allowed: exceeded max jobs allowed" -- on
  zones carrying ZERO jobs, Free and Pro alike. A plan without the product
  entitlement has a job quota of zero, so the "quota exceeded" message IS
  the plan wall. Read a quota message on a fresh fixture as an entitlement
  suspect and settle it with one probe on the sanctioned paid zone.
- **A full-WRITE token policy is not full access: some product families
  split read from write.** The harness token carries every write
  permission group and its creates/deletes succeed everywhere -- but Web
  Analytics (RUM) reads answer 403 code 10000 "Authentication error" under
  it: `Account Settings Write` creates sites while reading them back needs
  the separate `Account Settings Read` group (probe-proven by minting a
  temporary token with just that group). Consequences: a 403 on a READ
  under a write-loaded token is a missing READ GROUP suspect before it is
  a user-actor-endpoint suspect -- distinguish them by probing with a
  purpose-built temporary token (cheap, self-cleaning, and decisive),
  because the two diagnoses have opposite outcomes (a token edit vs an
  honest deferral). Terraform providers need the read side for EVERY
  refresh, so a read-walled family breaks mid-lane with orphans that only
  direct API DELETEs (writes -- which still work) can sweep.

- **`editable=true` in the zone-settings list is NOT a write guarantee.**
  The flag describes the settings class's plan-editability; a setting can
  still carry a separate product gate. `ciphers` reads editable=true on a
  Free zone yet every write answers 400 code 1023 "Advanced Certificate
  Manager is required". Probe the WRITE, not the read, before calling a
  setting free.
- **Per-zone SUBSCRIPTIONS gate independently of plan tier.** Advanced
  Certificate Manager gates Total TLS and every per-hostname TLS override
  with 401 code 1450 on Free and Pro zones alike -- upgrading the plan
  does not open them; only the per-zone ACM subscription does. Name the
  subscription (not a plan) as the unblock in these deferrals.
- **A pending zone can answer 400 code 1000 "Invalid zone identifier" for
  a setting that does not exist until activation** (`auto_origin_tls_kex`).
  The zone id is fine -- the same write succeeds on an ACTIVE zone. Treat
  1000-on-a-fresh-fixture-zone as a delegation-state suspect, not an id
  bug, and settle it with one write probe on the sanctioned active zone.
- **Product ENROLLMENT gates fire before everything -- even reads -- and
  neither plan tier nor a subscription opens them** (measured live
  2026-08-28). Cloudflare for SaaS answers 400 code 1404 "No quota has
  been allocated for this zone or for this account" on custom-hostname
  creates AND the bare LIST, identically on a pending fixture zone and the
  active Pro zone; the fallback-origin PUT answers its own 400 code 1456
  "This feature is available with SSL for SaaS". The unblock is a
  dashboard enrollment on the zone (SSL/TLS -> Custom Hostnames, card on
  file; free tier included), not a plan upgrade -- name the enrollment as
  the unblock, and expect the gate to mask every downstream risk (the
  fallback origin's documented token-PUT 403 cannot even fire until the
  enrollment opens the surface).
- **The delegation-state wall can wear an OWNERSHIP message.** Origin CA
  certificate requests for a hostname under a PENDING zone answer 400
  code 1010 "This zone is either not part of your account, or you do not
  have access to it" -- even though the zone IS on the account; the
  identical request for an ACTIVE-zone hostname succeeds with the same
  token (probe-proven both ways, 2026-08-28). Same discipline as the
  1000 class: settle ownership-flavored refusals on fresh fixture zones
  with one probe against the sanctioned active zone.

### Settings-singleton verifiers: pick a surface that answers on every plan

A settings-family verifier probes ONE endpoint as the family's existence
surface, and that endpoint must answer on the cheapest zone a lane can
run on. The natural-looking choice can be read-gated: Cloudflare's
`cache_reserve` GET answers 400 code 1135 "not available for your plan
type" on Free AND Pro zones, so a verifier registered on it would fail
every honest verify phase against a free fixture zone. Probe the
verifier's own GET on a free zone before registering (the cache-settings
verifier probes `cache/tiered_cache_smart_topology_enable`, measured
answering editable:true on every plan even before any write).

### No-op-destroy singletons on a SHARED zone: write the default value

An env-gated arm that manages a no-op-destroy singleton on the operator's
sanctioned (shared, long-lived) zone abandons whatever it wrote when the
lane destroys. The hygiene idiom: declare Cloudflare's DEFAULT value in
the scenario -- the lane still proves the full lifecycle (deploy, verify,
idempotency, blind import round-trip, destroy) while the shared zone ends
exactly as it started. Never write a non-default value to a no-op-destroy
surface on a zone the lane does not own.


### Long-running Azure components (Bastion hosts)

A Bastion host is the ~10-minute duration class outside gateways:
measured live (BASIC SKU, eastus, dedicated AzureBastionSubnet), the
create ran **10-14 minutes** (13m33s Pulumi, 9m59s Terraform) and the
delete **6-9 minutes** (6m16s / 8m41s), and the host bills (~$0.19/h)
from creation. A full single-engine lane (fixture chain up, deploy,
verify, destroy, verify-gone, chain down) totals **~35-45 minutes**
because the shared subnet and address install profiles deploy every
document (~13 fixture deploys at ~1 minute each) before the host
starts; budget `-timeout=90m` per engine (120m with the import
round-trip enabled).

### Private DNS Resolver family: serialized lanes, ~16-minute class

The Private DNS Resolver family (resolver, forwarding ruleset, virtual
network link) is a hard-serialization family: Azure allows ONE resolver
per virtual network and ONE endpoint per delegated subnet, and every
lane in the family -- the resolver's own smoke AND the fixture resolver
the ruleset/link chains deploy -- anchors the same fixture network and
delegated subnets. **Never run any two of this family's lanes
concurrently**, and never use the `make e2e-test-component` wrapper for
the resolver (its unanchored `-run "Test.*AzurePrivateDnsResolver"`
regex matches all three kinds' test entries at once); use exact
anchored filters like `-run 'TestAzurePrivateDnsResolver_Pulumi$'`.
Measured live (eastus, both engines): every lane in the family lands in
a tight **~15.5-17.5 minute** band -- resolver deploy 2m40s-3m08s and
destroy ~2m30s (endpoint deletes poll past ARM's first answer), ruleset
and link deploys/destroys 20-35 SECONDS each, with the ~7-8 minute
fixture chain up and down dominating everything. Budget `-timeout=90m`
per lane; endpoints bill hourly (cents for a lane-length life),
everything else is free at rest.

### Silently dropped fixture-chain deletes after resolver-endpoint teardowns: the resurfacing resource group

Twice in one proof session, the lane teardown following a fixture
RESOLVER destroy reported every dependency destroyed -- `pulumi
destroy` succeeded per stack, no retries consumed -- while Azure kept
the fixture VNet, most of its subnets, and the resource group. The
delete-side signature to recognize:

- The engines' subnet/VNet/RG deletes are ACCEPTED (destroy exits
  clean) but ARM never runs them -- the session-055
  DeleteThenPoll-returned-success-with-no-delete-job class, here hitting
  the network chain within minutes of the resolver's endpoint deletes
  (whose subnet-association cleanup lags server-side).
- The resource group can RESURFACE: an `az group list` probe right
  after teardown read clean, and a minute later the RG was back
  (provisioningState `Succeeded`) holding the VNet -- ARM hid it during
  the accepted-then-failed group delete. Probe AGAIN ~90 seconds later
  before trusting a clean read between this family's lanes.
- Nothing genuinely refuses deletion: an explicit
  `az group delete -n <fixture-rg> --yes` clears the whole chain in
  ~1-4 minutes, first try.

The next lane fails loud and cheap at DEPENDENCIES-UP (`a resource with
the ID ... already exists`, ~17s in) -- that gate working as designed.
The remedy is environmental, never a module edit: delete the fixture
resource group explicitly, re-probe after the resurface window, re-run
the failed lane. Both occurrences' re-runs passed with zero changes.

The trigger is NOT resolver-specific: the same signature (teardown
reports every dependency destroyed, the fixture RG later answers
`az group list` again, the next lane's fixture create collides) fired
after a plain Container App Environment chain teardown whose
DEPENDENCIES-DOWN passed cleanly. Treat it as a general Azure
delete-side class on fixture resource groups; the remedy is identical
and the explicit group delete has cleared it first-try every time.

### Bare `ServiceUnavailable` on an ARM create LRO: transient, retry the lane once

Some ARM operations fail their create's long-running poll with a bare
`ServiceUnavailable` -- empty error code, message "The service is
unavailable now. Please retry the request later." -- minutes after the
resource reads as materializing (a mid-poll GET saw it `Updating`).
First live member: a Network Watcher flow log create with Traffic
Analytics polled 12 minutes into that answer, Azure rolled the
half-created flow log back cleanly (NotFound moments later, nothing
stranded), and the IDENTICAL manifest created in 36 seconds on the
retry. This is the ML MFE `InternalServerError` class wearing a 503:
treat the first occurrence as a transient service fault -- wait out the
failed lane's teardown, verify the half-created resource rolled back
(flow logs live under the watcher in `NetworkWatcherRG`, NOT the
fixture resource group, so check there), and retry once before touching
the module. A second identical failure is the finding.

### Front Door: fast creates, ~18-minute profile deletes

Azure Front Door (Standard/Premium) inverts the usual timing profile:
every resource in the family CREATES in seconds-to-minutes (profile
~1-2 min; endpoints, origin groups, origins, and routes each well under
a minute), but PROFILE DELETION runs ~18 minutes wall time (verified
live on both engines; the service tears the global edge deployment down
before ARM confirms). Consequences for scenario budgeting:

- Any scenario whose chain includes a Front Door profile fixture costs
  ~20-27 minutes per engine, dominated entirely by the fixture teardown.
  Child-resource deletes inside a live profile are fast; it is only the
  profile itself that is slow.
- Deleting the fixture RESOURCE GROUP does not dodge this: the RG delete
  waits on the profile delete inside it.
- Estimate a full family suite accordingly (10 dual-engine runs measured
  ~3.7 h total) rather than assuming CDN-family resources are cheap
  because they create quickly.

### Vendor-catalog IDs in fixtures: look them up, never infer them

When a scenario (or preset, or hack manifest) carries an ID from a
provider-curated catalog -- a managed WAF rule ID, a curated rule-set
name/version, any value whose legal vocabulary lives server-side -- look
the real IDs up before writing the fixture; a plausible-looking ID
passes every offline gate (the schema types it as a free string) and
fails only at live apply. The lookup sources, in order: the service's
own catalog API (e.g. Azure's
`GET .../providers/Microsoft.Network/frontDoorWebApplicationFirewallManagedRuleSets`
lists every rule group and rule ID per set version) and the provider
clone's acceptance-test fixtures. The trap that motivated this: Azure
bot-manager rule IDs carry a `Bot` prefix (`Bot300700`) while the
default-rule-set IDs are bare numerics (`942100`) -- an inferred bare
`300700` was rejected by ARM on both engines with "managed rule IDs are
not supported".

### Angle-bracket placeholders in canonical manifests fail the offline plan gate

A kind's `e2e/manifest.yaml` is the canonical validated example — refgen's
Example source AND an offline-plan fixture — so every reference-shaped
value in it must be syntactically REAL, never a `<placeholder>`. Provider
schemas validate resource-identifier syntax at plan time (the AWS provider
runs `ValidARN` on every ARN-typed argument), so `value: "<sns-topic-arn>"`
passes proto validation (a free string) and fails the offline `tofu plan`
with "invalid ARN: arn: invalid prefix". Use realistic well-formed values
(`arn:aws:sns:us-west-2:123456789012:order-events`,
`subnet-0a1b2c3d4e5f60001`) — they document the expected shape for users
and agents, and they keep the manifest a working plan fixture. The same
rule already governs presets; the trap here is that a manifest predating
the offline gate can hide placeholders for years.

### Placeholder domains in receiver/endpoint URLs are server-validated

When a fixture (or preset, or hack manifest) carries a URL the SERVICE
will call back to -- a notification webhook, an alert receiver, an
integration endpoint -- do not use documentation placeholder domains.
Some services validate the URI server-side at create and reject blocked
domains outright: Azure Monitor action groups return 400
`WebhookServiceUriBlocked` for webhook receivers on `example.com`, on
both engines, while every offline gate passes (the receiver URL fields
are sensitive-annotated and deliberately carry no value rules, so the
schema cannot screen domains). Use a real domain the fixture plausibly owns
(the project's own domain works; the service does not probe the URL at
create, it only screens the domain). Presets and hack manifests should
carry a domain-you-own shape (e.g. `hooks.yourcompany.com`) so users
never copy a blocked placeholder.

### A stale local `az` CLI silently drops new API surface (false-negative evidence)

When a lane captures evidence with `az` on a NEWLY-modeled field and the
field reads back ABSENT while its same-wave siblings are present, verify
the CLI's own API model knows the field before treating the absence as a
module or provider defect. The installed CLI's bundled service models lag
new provider surface, and unknown response members are silently
discarded -- a false negative that pattern-matches an engine silent-drop
bug and burns diagnosis time (the AWS section's CloudFront
`CacheTagConfig` incident is the worked example, with the skeleton-probe
technique). Prefer evidence readers whose model is pinned WITH the repo:
the engine's own refresh/plan diff, or a probe through the repo's pinned
Azure SDK. The class is the sibling of the stale-installed-`planton`
binary lesson -- stale local tooling produces false readings in both
directions.

### Vendor-constant casing: an SDK constant's Go identifier is not its wire value

When a module maps a spec enum to a provider's string vocabulary, read
the SDK constant's STRING VALUE, never its Go identifier -- the two can
differ in casing, and provider schemas validate case-sensitively (e.g.
azurerm's `StringInSlice(..., false)`). The trap that motivated this:
the frontdoor SDK declares `TransformTypeURLDecode TransformType =
"UrlDecode"` -- an identifier-derived `"URLDecode"` passes every offline
gate that does not render a plan and fails at plan time on both
engines. Two defenses: (1) verify every enum map row against the SDK
constants file (or the provider schema's validator list), and (2) make
sure at least one scenario or hack manifest exercises every mapping row
whose casing is irregular, so the offline plan gate and the live suite
keep it covered -- a mapping row no fixture uses is dead code that
validation cannot see.

### Service-created default sub-resources cannot be adopted declaratively

Some Azure services auto-create a well-known child resource the moment a
parent exists -- Service Bus subscriptions get a catch-all filter rule
named `$Default`, and similar service-created defaults exist elsewhere.
It is tempting to model "replace the default" as declaring a resource
with the same name, expecting ARM's CreateOrUpdate to upsert it. That
never reaches ARM: both engines' azurerm create paths run an
import-existence check first and fail with "a resource with the ID ...
already exists -- needs to be imported" (live-confirmed on both engines;
the check exists precisely so Terraform-model tools never silently adopt
state they did not create). The honest modeling: reserve the
service-created name in the spec (a CEL rejecting it with a message that
teaches the alternative), document declared siblings as ADDITIVE
alongside the default, and teach the one-time out-of-band removal (an
`az` CLI delete) where restrictive behavior is wanted. No offline gate
catches this class -- the plan renders fine and only the live create
collides -- so treat every "declare over a service-created default"
design as suspect until a live run proves it.

### Provider-accepted values can be SERVER-RETIRED: SKU consolidations reject at create

The pinned provider's static vocabulary lags Azure's retirement schedule,
so a value every offline gate accepts can be rejected by ARM at create
with a retirement error. Live-confirmed classes on the VPN gateway
family: `NonAzSkusNotAllowedForVPNGateway` (new non-AZ VpnGw1-5 creates
blocked since 2025-11-01 -- only the AZ tiers and BASIC remain
creatable) and, chained behind it,
`VmssVpnGatewayPublicIpsMustHaveZonesConfigured` (an AZ-SKU gateway
demands ZONES on its Standard public IP -- a no-zone fixture address
that served the non-AZ world fails the AZ world). When a create rejects
with a retirement-class error: fix the SPEC (retire the values with
reserved numbers/names per the catalog's removal-hygiene precedent),
move scenarios/fixtures/presets to the successor values, and land the
retirement on the kind's user surfaces -- never band-aid just the
scenario. Retirements also arrive in CHAINS: re-run the lane after each
fix expecting the next constraint in the family to surface.

### Some ARM contracts exist ONLY server-side: budget one live probe for the flagship combination

The azurerm provider is the completeness floor, but it is NOT a complete
map of ARM's rejection surface: some cross-field contracts appear in no
schema validator, no CustomizeDiff, and no create-path check -- they live
only in ARM's regional service. Example (live-confirmed): a firewall
attached to a firewall policy must not carry firewall-level DNS
parameters -- ARM rejects the create with
`AzureFirewallDNSConfigNotAllowedForVhubOrVnetWithPolicy` ("DNS
configuration should be managed by policy"), yet the provider happily
plans and sends the combination. The consequence for scenario design:
make the FIRST live scenario for a kind exercise the flagship FIELD
COMBINATION users will actually deploy (here: policy-attached firewall
plus every side-channel knob the policy also owns), because a
source-diff of the provider cannot prove combinations the provider never
validates. When such a rejection surfaces, front-load it as a spec CEL
(with the ARM error code in the message trail), fix the scenario, and
record the contract in the component docs -- the next component in the
same service family should check for sibling "managed by the parent"
exclusions up front.

### Create-only fields the provider never reads back make blind imports plan a REPLACE

A provider's Read function is not guaranteed to populate every argument
it accepts at create -- some create-only inputs (a snapshot's
`source_resource_id`/`source_uri` at the azurerm v5 pin is the
live-caught case) are simply never set from the ARM response. On the
normal apply path nothing surfaces: the value persists in state from
the apply, so even the IDEMPOTENCY re-plan stays clean. The blind
import round-trip is the ONLY gate that catches it: a fresh import
holds a null for the field, the config supplies a value, and when the
field is ForceNew the post-import plan proposes destroy+create -- which
the round-trip oracle rightly refuses (replaces always fail; declared
tolerances cover in-place updates only). The remedy is NEVER a
tolerance: make the field create-time-only as a CONTRACT -- lifecycle
`ignore_changes` in BOTH engines, the spec field comment rewritten to
teach that edits are ignored and a new capture is a new resource, and
the kind's GUIDE carrying the operational judgment. For artifact-class
kinds (snapshots, images) this is also the safer contract: a ForceNew
source edit would silently DELETE the artifact and recapture from
current state. Check the provider's Read function (does it `d.Set` the
field?) the moment a round-trip reports a replace on a create-only
argument. Second AWS instance: `aws_acmpca_certificate`'s five
issue-request arguments (csr, signing_algorithm, template_arn,
validity, api_passthrough — upstream's own import test ignores the
full set). The stakes are maximal there: the blind round-trip planned
a delete+create on the CA's own ACTIVATION certificate — re-issuing a
trust anchor adopters pin by fingerprint — and the install resource
cascaded. Note a `config_only_attributes` declaration CANNOT absorb
this class (tolerances cover in-place diffs; these fields force
replaces) — the module-side ignore is the only honest fix, after which
the tolerance is dead weight and gets retired.

The SECRET sub-class takes the OPPOSITE remedy. When the never-read-back
ForceNew field is a write-only SECRET (first live case: a container
group's secure environment variables), `ignore_changes` is wrong --
rotating the secret is SUPPOSED to replace the resource, and ignoring it
turns rotation into a silent no-op. There the truth is that a
secret-bearing config can only plan a one-time replace after any import;
the secret-bearing SCENARIO opts out of the round-trip with the
reason-carrying `planton.dev/e2e-import-roundtrip-skip` annotation, the
recipes keep proving on the kind's secret-free scenarios, and the kind's
docs teach the one-time convergence replace after adoption. Choose by
asking "should editing this field replace the resource?" -- no
(immutable creation history) means ignore_changes; yes (rotatable
secret) means the annotation.

The annotation's second structural class: a SINGLETON COMPANION with no
upstream importer whose re-create the cloud REFUSES while the live object
exists. A not-importable type normally rides the catalog's
`not_importable_upstream_reason` -- the round-trip skips its import and
proves the adopter's re-create path instead -- but that tolerance is
honest only for upsert-convergent creates (idempotent PUTs). "No upstream
importer" includes an importer that EXISTS but cannot run: measured
2026-08-27, cloudflare_snippet's ImportState seeds identity only and the
resource's Read then dereferences a pointer field tagged no_refresh that
only a create populates -- every imported snippet panics the provider
("Plugin did not respond"). A crashing importer is declared with the
same machine field, evidence in the reason, and the upsert-adopt path
carries the proof (snippet create is an upsert by name). When the
create is a plain POST and the service enforces one-per-parent, the
re-create is rejected with the object still live, so NO recipe can
converge: the companion-bearing scenario opts out with the annotation
(reason naming the measured error), the parent's importer keeps proving
on a companion-free scenario, and the kind's docs teach the real adoption
path (delete the live companion first, or leave it managed outside).
First users, both measured 2026-08-26: Cloudflare queue consumers --
POST `.../consumers` answers 400 code 11004 "already has a consumer"
(one consumer per queue by design) -- and Cloudflare R2 bucket event
notifications -- the per-queue PUT MERGES rules instead of replacing
them, so an identical re-put answers 400 code 11020 "rules have invalid
overlap".

### The Stainless decode-over-prior-state asymmetry: import-only diffs in BOTH directions

Auto-generated (Stainless-style) providers decode API responses OVER the
prior model, so an attribute the GET omits keeps whatever the state
already held. On the create path that means the configured value
survives every refresh -- the IDEMPOTENCY re-plan stays clean and
nothing looks wrong. On the import path there IS no prior value, so the
same attribute lands null, and the post-import plan shows an in-place
update the normal lifecycle never produces. The MIRROR direction also
exists: an attribute the GET ECHOES (a server default like `false`)
gets restored on import where a guarded module deliberately omitted it,
and the plan proposes "unsetting" it. Both directions are the
import-restore gap class: no module fix is right (the writes are no-ops
against the cloud's real state), the remedy is a
`write_normalized_attributes` declaration on the provider catalog row
citing the upstream import test's own `ImportStateVerifyIgnore` list,
plus a GUIDE note teaching adopters the one-time first-apply update.
First measured users (Cloudflare v5.23.0, live 2026-08-26): Access group
`is_default` and Access policy `approval_required`/`isolation_required`/
`purpose_justification_required` (omitted-by-GET direction); Access
application `auto_redirect_to_identity`/`enable_binding_cookie`/
`options_preflight_bypass` (echoed-by-GET direction). The upstream ignore
list is the map: check it during the PRE-LANE pass and declare from
measurement, never blanket-tolerate.

### The create response omits computed attributes the GET returns: the read-after-create class (module-fixable)

Some Cloudflare create endpoints answer with a MINIMAL body (the alerting
webhook POST returns only `{id}`; the Web Analytics site POST omits the
`ruleset` object -- both measured by direct POST-then-GET, 2026-08-27),
while the resource GET returns the full body. Auto-generated providers
decode create responses without a read-after-create, so every computed
attribute the create response omitted sits NULL in state until the first
refresh backfills it. Three failure shapes, worst first: (1) a module
expression dereferencing the attribute (`resource.ruleset.id` feeding a
folded child resource) HARD-FAILS the apply; (2) a stack output riding the
attribute ships EMPTY on first deploy on both engines -- a wrong output,
not just noise; (3) on Terraform, the first refresh backfills the value
and flips the output, failing the strict idempotency re-plan with a
"Changes to Outputs"-only diff. The fix is module-side and cross-engine: a
read-after-create (Terraform data source keyed by the created resource's
id; the Pulumi Lookup...Output invoke) carries those attributes instead of
the resource -- and on Pulumi the lookup results need explicit
`pulumi.ToSecret` re-marking, because `AdditionalSecretOutputs` on the
resource does not cover lookup results. Diagnose in one probe: direct API
POST, inspect the create response body, GET the object, diff the two.
First measured users: `cloudflare_notification_policy_webhooks` (`type`),
`cloudflare_web_analytics_site` (`snippet`).

The same diagnosis can uncover a sharper sibling truth: a null sub-object
may be IDENTITY-DEPENDENT rather than read-dependent. The Web Analytics
site's `ruleset` is not omitted-by-create -- it NEVER exists for
host-identified sites and ALWAYS exists (create response included) for
zone-linked ones, which makes any host+rules configuration structurally
undeployable (the rules attach to the ruleset). That class is fixed in the
SPEC (a CEL wall forbidding the impossible combination, taught on the
fields) plus null-safe outputs -- not by a read-after-create, which cannot
conjure an object the API never creates. Run the probe on BOTH identity
variants before concluding which class you have.

### A transitioning phase is never a stack output: the async-phase-output class

Distinct from the read-after-create class above (where a stable value
merely arrives late): some resources carry a status attribute that
GENUINELY TRANSITIONS server-side after create (a certificate deployment
moving pending_deployment -> active seconds later). Exporting it as a
stack output makes strict idempotency structurally unpassable -- the
refresh after the transition flips the output, `-detailed-exitcode`
answers 2 on a "Changes to Outputs"-only diff, and a real customer sees a
phantom pending change on every plan after the transition. No
read-after-create fixes it (the create-time read captures the WRONG
phase), and no catalog tolerance can absorb an idempotency failure. The
fix is contract-level, in both engines: transitioning phases are not
deployment facts, so they are not stack outputs -- drop the output and
teach readers to query the provider's API for live phase. Outputs carry
only values that are stable once the apply returns (ids, names, expiry
timestamps). First measured user (live 2026-08-28):
`cloudflare_authenticated_origin_pulls_hostname_certificate` `status`
(the sibling `cloudflare_custom_ssl` `status` is the same class).

The class LURKS in kinds authored before the lesson existed: a
pre-lane verification pass caught it dormant in three certificate/SaaS
kinds (certificate pack, custom hostname, fallback origin) authored
weeks before the first live measurement. Before running any kind's
first lanes, grep its outputs for phase-named fields (`status`,
`state`, `phase`) whose provider docs describe an asynchronous
transition, and remove them proto-side (reserve the field number and
name) BEFORE the lane measures the failure for you. One honest nuance:
a phase output survives live proof when the transition never happens
inside a lane (a run-scoped zone's `status` stays `pending` for the
lane's whole life) -- that green is real but conditional on the
fixture's lifecycle, so leave proven kinds alone and catch the class in
kinds that have not yet run.

The class is not only enum-shaped phases -- it has a DIAGNOSTIC-LIST
form and a SCALAR form, both measured live 2026-08-29: a custom
hostname's `verification_errors` list deploys EMPTY and the server
appends "zone is not active yet" seconds later (the failing re-plan is
"Changes to Outputs"-only, with the resource itself converged), and a
certificate pack's `primary_certificate` is absent in the create
response, reads "0" seconds later, and becomes the real certificate id
as issuance progresses. The phase-name grep therefore under-catches:
audit every output whose value the SERVER can move without a config
change (error/warning lists, progress counters, ids of objects the
service creates asynchronously). A watched-and-cleared suspect list is
not a defect record -- the first lane decides; these two were left in
deliberately by the session that removed the `status` outputs, and the
next session's first lane convicted both.

### Multi-line ${E2E_ENV:...} values render as quoted YAML scalars, whole-value position only

Scenario token expansion is plain text substitution -- which corrupts the
manifest the first time an env value spans lines (PEM certificate/key
material: the second line lands outside the field's YAML scalar and
parsing fails at load). The framework renders a multi-line env value as a
double-quoted single-line YAML scalar (`\n` escapes) instead, and ONLY
when the token occupies a whole mapping value (`certificates:
${E2E_ENV:...}`); any other placement (mid-string, inside a block
scalar) has no safe mechanical rendering and fails loudly at expansion.
Authoring rule: give a PEM-carrying field the token as its entire value,
never compose it into a larger string. Single-line values keep
byte-identical plain substitution. Shell corollary: `$(cat cert.pem)`
strips the trailing newline -- harmless where modules canonicalize (the
mTLS certificate store), but keep it in mind when a service is
byte-exact about PEM form.

### Computed attributes without state-preserving plan modifiers: the perpetual re-plan class

Some auto-generated resources ship Computed (and Computed+Optional)
attributes with NO `UseStateForUnknown`-style plan modifiers, and pair that
with an API that echoes only the fields you sent. The combination is a
resource that CANNOT pass a refresh-inclusive idempotent plan on any
configuration shape: null computed members re-plan as "(known after apply)"
forever, and the first one inside a config-provided nested object turns
every plan into an in-place update. Know the two non-fixes before burning a
lane on them (both measured live 2026-08-26): sending the field explicitly
does not converge (the API accepts-and-drops the write, and the create
path's computed decode nulls it right back), and `lifecycle.ignore_changes`
cannot help (it filters the CONFIG merge, while this unknown is planned by
the PROVIDER afterward). Diagnose with three probes -- config shape absent /
empty-object / populated, plus a direct API POST-then-GET to see the sparse
echo -- and if all drift, the honest outcome is a DEFERRAL on the upstream
defect, never a weakened idempotency gate. First measured user:
`cloudflare_zero_trust_gateway_policy` at v5.23.0 (upstream issue #7106;
every shape drifts via `rule_settings.ignore_cname_category_matches` and
friends). Second measured user (live 2026-08-27): `cloudflare_ai_gateway` --
the WRITE-ONLY-BODY variant, where the API accepts whole object surfaces on
write but no read EVER returns them (`otel`/`spend_limits`; probe by direct
POST-then-GET) while the provider models them computed_optional and
refreshes them, so the unknown-flip fires even on configurations that never
set the fields, and a SET value is worse (refresh nulls state, planning a
REAL update forever). Its folded sibling `cloudflare_ai_gateway_dynamic_routing`
escalates the same asymmetry into a perpetual DESTROY-AND-RECREATE: the read
returns the graph only under a response path the provider never consults
(`version.data`), and the un-restorable attribute is RequiresReplace. Both
remedies re-measured non-fixing there, matching #7106. Contrast with the
decode-over-prior-state class above: that one is
import-only and tolerable by declaration; this one breaks the NORMAL
lifecycle and no catalog declaration can absorb an idempotency failure.
When only SOME of a resource's attributes are in this class, the deferral
still takes the whole kind on the affected engine -- but fix the honest
echo classes first (always-send booleans, server-default coalesces): the
diagnosis is only clean once every config-carried diff is dead and the
surviving drift is provably the provider's own unknown promotion.

### A PURE-computed attribute planning a phantom update: the ignore-changes class (module-fixable)

The fixable sibling of the perpetual re-plan class above. Some auto-generated
resources ship ONE pure-Computed attribute (never user-set, stable
server-side) with no `UseStateForUnknown` modifier, and the provider proposes
an in-place update on EVERY plan solely to re-mark it "(known after apply)" --
with a clean refresh (`plan -refresh-only` reports no changes) and zero config
drift. Two things distinguish it from the deferral class: the attribute is
NEVER SENT (pure computed, so ignoring it cannot mask a real write), and
`ignore_changes` DOES fix it (the unknown is the framework's computed-null
promotion on that one attribute, not a provider-planned value under a config
merge). The remedy is `lifecycle.ignore_changes` on exactly that attribute in
the Terraform module AND `pulumi.IgnoreChanges` on the same property in the
Pulumi module -- the BRIDGED provider surfaces the same phantom update in
previews (a preview never refreshes, but it DOES run the provider's plan
against stored state, so "Pulumi previews hide refresh drift" is NOT immunity
to this class). The stack output still reads the real value from state.
First measured user (live 2026-08-27, v5.23.0):
`cloudflare_zero_trust_device_default_profile.policy_id` -- both engines
measured drifting, both converged by the ignore.

A second, judgment-heavier member (live 2026-08-29):
`cloudflare_custom_hostname.ssl.certificate_authority`. Not pure-computed --
the field is USER-SETTABLE, but only on Enterprise (400 code 1459 rejects an
explicit value on any other plan, probe-measured), and when unset the server
assigns a CA AT RANDOM per create (consecutive identical creates measured
`ssl_com` then `google`), so config can never mirror the stored value and
always-send / server-default-coalesce fixes are impossible. The deciding
evidence for the scoped ignore was UPSTREAM'S OWN acceptance tests: every
ssl-bearing config in the provider's test suite wraps
`ignore_changes = [ssl.certificate_authority, ...]` -- the provider's authors
know the attribute cannot converge. When you mirror that recipe, document the
one real trade-off ON THE SPEC FIELD: an in-place change of the ignored field
no longer applies (here: Enterprise users changing CA must recreate). Never
widen the ignore beyond the attributes upstream's own tests convict.

### Settings endpoints that REJECT identical writes: the no-op-write wall (probe before reusing the write-the-default discipline)

The shared-zone discipline for no-op-destroy settings ("write the default
value -- prove the lifecycle while leaving the zone as found") silently
assumes the endpoint ACCEPTS a write that changes nothing. Not all do:
Cloudflare's Total TLS configure answers 400 code 1467 "No state between
current settings and new settings has changed" to a value identical to the
stored record (measured live 2026-08-29 -- the zone's first-ever configure
of the as-found value succeeded because it CREATED the record, and the
identical rewrite minutes later was refused, so the trap fires only from
the second run onward). A write-the-default arm on such an endpoint is
green exactly once and then fails every re-run at deploy. The re-runnable
shape is a REAL-CHANGE arm with a documented restore duty: write a value
that differs from the captured baseline, and restore the baseline
out-of-band after EACH engine's lane (also BETWEEN engines -- the second
engine's identical write hits the same wall). Contrast the zone-settings
PATCH surfaces and the kex toggle, which tolerate identical rewrites
(measured across many lanes) -- the wall is per-endpoint, so probe the
rewrite before designing the arm.

### The canonical LOCK decides the engine, not the pin RANGE -- and an upstream "fix" can change the failure mode

Every Terraform module here carries a committed `.terraform.lock.hcl`
pinning the exact provider build; the `~> X.Y` constraint in provider.tf
only bounds what a DELIBERATE pin bump may select. Two traps measured live
2026-08-29 on `cloudflare_hostname_tls_setting`: (1) a defect fix released
within the pin range (v5.24.0 fixing the v5.23.0 Read) is NOT in your lane
-- the lock still selects v5.23.0 from the shared plugin cache, so verify
defect claims against the LOCKED version and record unblocks as "a release
carrying the fix + the canonical pin bump", never "already fixed upstream";
and (2) before citing a newer release as the unblock, RUN it once in a
disposable workdir (copy the module, drop the lock, pin the exact version)
-- v5.24.0's rewritten Read turned out to GET a per-hostname path Cloudflare
answers with 405 Method Not Allowed, replacing v5.23.0's silent
state-zeroing with a hard refresh error: the failure mode changed, the
defect did not close, and an unblock recorded from the diff alone would
have been false.

### IMPORT-ONLY convergence gaps: schema defaults and API auto-marks that refresh forgives but import does not

A lane can pass DEPLOY and the refresh-inclusive IDEMPOTENCY plan on every
scenario and still fail the blind import round-trip with a genuine in-place
update -- because Optional+Computed semantics forgive an omitted config
against a refreshed state (prior value kept, no diff) while the post-import
plan compares the raw imported read against the config's DEFAULTS. Two
measured shapes (live 2026-08-28, `cloudflare_load_balancer` at v5.23.0/24.0):
(1) the provider's schema `Default` differs from the API's CANONICAL stored
value -- `steering_policy` defaults to `""` while the API stores `"off"` (or
`"geo"` with geo-pool maps), so an omitted policy re-plans `"off" -> null`
only after import; contrast `session_affinity`, whose default `"none"` equals
the canonical echo and never diffs. (2) the API AUTO-MARKS a member inside a
list attribute -- a `fixed_response` rule is stored with `terminates: true`,
so a module that omits `terminates` re-plans the whole `rules` list only
after import. Both are MODULE-FIXABLE by sending the canonical value
explicitly (mirror the API's documented mapping; send `terminates: true`
exactly when the rule carries a `fixed_response`) -- prefer that over a
catalog tolerance, because these are real steering semantics a tolerance
would blind the oracle to. Diagnosis note: ONE real diff makes the provider's
plan flip every other Optional+Computed attribute to "(known after apply)",
so the round-trip oracle's changed-list looks enormous -- find the entries
whose before/after are concrete values (not unknown-flips), fix those, and
the rest of the list collapses to a no-op. Related teardown hazard: a failed
import round-trip aborts the lane BEFORE its DESTROY phase, so the deployed
object outlives the lane and can block fixture teardown (a live load
balancer holds its pool -- the pool's delete 400s until the zone cascade
removes the LB); sweep the account after any import-RT failure instead of
trusting retry exhaustion.

A settings-singleton trap for capture-and-restore lanes. Auto-generated
schemas may attach a static default (`stringdefault.StaticString("")`) to an
Optional field, so ANY apply that omits the field actively writes the empty
value -- resetting live account state the scenario never mentioned. Measured
live 2026-08-27: a default-WARP-profile apply that omitted `tunnel_protocol`
blanked the account's `masque` setting to `""`. Two duties follow: the
VERIFY-CLN restore diff must compare EVERY field of the captured body against
the live read (a restore that only re-asserts the fields the scenarios set
silently ships the reset), and the spec field that carries such a default
must teach that unset means reset-to-default, never leave-untouched.

### Computed-optional attributes with RAW model types: the unknown-crash class (module-fixable)

A sibling of the perpetual re-plan class, but FIXABLE module-side. Some
auto-generated resources declare an attribute `Optional+Computed` with a
customfield type in the SCHEMA while the Go MODEL types it as a raw pointer
or raw slice that cannot hold "unknown". A config that leaves the attribute
null plans it as unknown (computed-null promotion), and the apply CRASHES
decoding the plan into the model: `Value Conversion Error ... Received
unknown value, however the target type cannot handle unknown values`. The
offline bar never sees it -- plan alone doesn't run the conversion. The
remedy is a module-side coalesce to the attribute's DOCUMENTED server
default so the planned value stays known (an explicit `{mode = "inherit"}`
where omission means inherit; an empty list where absence means
no-restriction) -- semantics unchanged, crash unreachable. Diagnose by
reading the resource's model.go: any `computed_optional` field typed
`*Model` / `*[]*Model` instead of `customfield.NestedObject[...]` is a
carrier. First measured user (live 2026-08-26, unfixed through v5.24.0 and
provider main): `cloudflare_zero_trust_dns_location` (`max_ttl`, top-level
`networks`, and every endpoint `networks`).

### Empty lists the API drops on read: [] beats null, real rows beat both

The companion trap to the unknown-crash class: after coalescing a null list
to `[]` to keep the plan known, the refresh may STILL drift when the API
omits empty lists from its read answer -- state refreshes the list to null,
config re-asserts `[]`, and the re-plan proposes a cosmetic `+ networks =
[]` forever. No module change can fix a refresh decode. The hierarchy: a
scenario (or customer config) that declares REAL rows is drift-free and
proves more; `[]` is the correct module behavior anyway (a cosmetic no-op
diff beats an apply crash); document the permanent-diff wall on the spec
field and GUIDE so nobody reads the diff as their own bug. First measured
user (live 2026-08-26): the DNS location's endpoint `networks` lists.

### Singletons by ACCRETION: the first object becomes an undeletable account default

Some Cloudflare families auto-promote the first object ever created on an
account to a mandatory account default that the API then refuses to delete
(error 1217) or demote in place (error 1216) -- the default can only be
MOVED by promoting another object, so the account can never return to zero
objects. A lane running on a previously-empty account trips this on
whichever scenario runs first: its destroy fails, and the "orphan" cannot
be swept by any API call. The honest pattern: park the default on ONE
permanent, clearly-named placeholder object (created once, named for what
it is, recorded), after which every run-scoped object creates as
non-default and lifecycles cleanly. Never point the placeholder dance at a
customer account without teaching it in the kind's GUIDE -- customers hit
the same wall on their first object. First measured user (live 2026-08-26):
Gateway DNS locations (`gateway/locations`). The DELETABLE mirror of this
class (measured 2026-08-27): an AUTO-PROVISIONED default container occupying
a hard singleton slot -- the Secrets Store's `default_secrets_store` fills
the account's only slot (a second create answers 1003
maximum_stores_exceeded) even on accounts that never used the product. When
the auto-provisioned object is verifiably EMPTY, the honest lane pattern is
delete-to-free-the-slot under owner approval, run the run-scoped lifecycles,
and recreate the same-named container at session end (the recreated id
differs -- record it); when it is not empty, it is production state and the
kinds defer.

### Terraform outputs: never null-guard a partially-sensitive object

An output expression like `x.scim_config != null ? x.scim_config.base_url :
null` fails at APPLY time with "Output refers to sensitive values" whenever
ANY member of the object is schema-sensitive: comparing the whole object
yields a sensitive boolean, and a sensitive condition taints the entire
conditional -- even though the selected leaf is not sensitive. Plan does not
evaluate output sensitivity, so the offline bar cannot catch it; it fires on
the first live apply. Write `try(x.scim_config.base_url, null)` instead --
direct leaf access carries only the leaf's own sensitivity and degrades to
null when the object is absent. First measured user (live 2026-08-26): the
Cloudflare Access identity provider's `scim_base_url` output (`scim_config`
carries the sensitive `secret` member).

### VERIFY-CLN is per-id; only a NAME-based census catches duplicate-create leaks

The verify-destroyed phase GETs the stack's recorded object id and can pass
honestly while a LEAKED DUPLICATE of the same object survives under a
different id (measured 2026-08-28: an account-token Pulumi lane passed
every phase including VERIFY-CLN, yet the end-of-session census found a
second ACTIVE token carrying the same run-scoped name and an id the stack
never knew -- most plausibly a create retry that succeeded twice). The
end-of-session sweep must therefore census by RUN-SCOPED NAME across every
object family the session touched, never only re-check the ids the lanes
verified; anything wearing a run id that the lanes did not destroy is an
orphan to delete by API.

### The round-trip's plan echo prints sensitive output VALUES to the local test log

The IMPORT-RT phase's oracle plan runs through terratest's default
logger, and `tofu show -json` exposes sensitive outputs in plaintext
(`prior_state`/`planned_values` carry the real values -- only the
`sensitive` flag marks them). Run-scoped throwaway credentials (tunnel
run tokens and the like) die with their objects at DESTROY, so the local
log exposure is inert for lanes built on run-scoped fixtures -- but
never point a round-trip-enabled lane at a long-lived credential
expecting log hygiene, and treat captured lane logs as
credential-bearing until the run's objects are destroyed.

### Revoke-is-not-delete surfaces, and the every-dispatch rule for new absence shapes

Origin CA certificates never 404: destroy is a REVOKE, and the revoked
certificate keeps answering GET 200 with its full body plus `revoked_at`
set, indefinitely (measured live 2026-08-28 -- revoked certificates from
hours earlier still answer). The verifier's `RevokedAt` envelope probe
reads a non-empty `result.revoked_at` as absence, the fourth absence
shape after `deleted_at` (tunnels), `is_deleted` (Workflows), and the
status enums (certificate packs / custom hostnames).

The lesson that cost an hour of misdiagnosis when this shape landed: **a
new `EnvelopePresence` field must join EVERY dispatch site, including the
client's fast-path guard** -- the short-circuit that returns "present" on
any 2xx when no absence shape is requested. A shape missing from that
guard is silently bypassed: the probe never parses the body, every
destroy reports a false "still exists", and the failure masquerades as
server-side propagation lag (this shipped once and was chased through
fresh-connection and cache-busting theories before the two-line direct
client probe against a known-revoked id exposed it). When a new absence
shape misbehaves, FIRST prove the client parses it -- one `go run` with
the real client against a known-absent object -- before theorizing about
the cloud.


### Echo maps built from resource state are PARTIAL mid-import -- eager indexing kills the import

Terraform modules that export import-derivation echo maps (for_each keys
-> resource ids) typically build them FROM the resource instances
(`{ for p, r in aws_x.this : p => r.id }`). At plan/apply the map is
total -- every instance is in the graph -- but during a blind round-trip
`tofu import` adds instances ONE AT A TIME, and evaluating a local that
eagerly indexes the partial map for every config key hard-errors the
import with "Invalid index" (live-caught on the REST gateway's path
tree: importing `/health` evaluated the echo map's `/orders` entry
before `/orders` was imported). The remedy is `try(map[key], null)` at
the echo-map CONSUMPTION sites with a comment teaching why the try() is
load-bearing -- the null entries heal as the remaining imports land, and
the key can never be genuinely missing at plan time because map and
consumer derive from the same spec. Resource-config references to the
same maps are safe without try() (imports evaluate the target resource's
config against the planned graph, not state); only locals-fed OUTPUTS
evaluate against partial state.

### Engine-side trigger arguments make a resource honestly not-importable even when an importer ships

A resource whose behavior-bearing argument is ENGINE-side metadata the
cloud never stores (the API Gateway deployment's `triggers` redeploy
hash is the live-caught case; upstream's own docs say "the `triggers`
argument cannot be imported") can never survive a blind round-trip: the
import holds null, the config supplies the hash, and `triggers` forces
replacement -- a guaranteed replace no tolerance may cover. The honest
declaration is `not_importable_upstream_reason` (skip-and-recreate) even
though an importer EXISTS upstream: the adopter's contract is that the
first reconcile-apply mints a fresh instance from the adopted definition
(for deployments, a behavioral no-op that repoints the stage). State the
importer-exists-but nuance in the reason text -- the field name says
"upstream" but the class is "cannot round-trip by construction", and the
next reader deserves the distinction.

### "no stack named ..." for a fixture that just deployed: backend state loss, not a module defect

When a scenario fails at DEPENDENCIES-UP with `failed to read outputs for
dependency ...: no stack named '<stack>' found` -- for a fixture whose
`pulumi up` just returned success (or one that "deployed and verified"
moments earlier, then fails teardown the same way) -- the suite's LOCAL
PULUMI FILE BACKEND has lost its stack state mid-run. Nothing is wrong
with the module, the manifest, or the cloud: the resources were created,
but the state tracking them vanished. The backend lives in a per-run temp
directory (`TestMain` creates it with `os.MkdirTemp`), so it is exposed to
anything that disturbs temp storage on the host. Diagnose by the
signature: SEVERAL stacks of the same scenario reporting "no stack named"
at once -- including ones that already deployed and verified cleanly (a
real per-stack failure never spreads to unrelated stacks) -- and an
identical re-run passing end to end. On a SINGLE-fixture chain the
"several stacks" half of the signature collapses to one: the fixture's
`pulumi up` succeeds and the output read seconds later reports "no stack
named" for that same stack (seen live on the runner appliance's RG-only
chain; the identical re-run passed end to end). The exposure is worst on
a shared workstation -- another program's suite churning host temp
storage is exactly the disturbance this class feeds on.

Recovery: the stackless fixtures' destroys were skipped, so sweep the
cloud for the failed run's fixtures before re-running. A same-named
fixture RESOURCE GROUP in a later scenario can mask the orphans -- ARM's
resource-group PUT is an upsert, so the later run's fixture "creates" the
existing group and its teardown then deletes it, orphans included -- but
never rely on that; sweep explicitly (`az group list`, plus the service's
own list for account-level orphans).

### A second "no stack named" cause: uniquifying suffixes truncated off stack names (fixed in the runner)

The same "no stack named" signature at DEPENDENCY destroy -- for stacks
whose deploys all "deployed and verified" -- once had a different cause
than backend state loss: stack names were blindly truncated to a length
cap AFTER their uniquifying suffixes (an install profile's per-document
index, the run id) were appended, so every document of a long-named
multi-document profile collapsed onto ONE shared stack name. Each
document's `pulumi up` then silently REPLACED the previous document's
resources (each "deployed and verified" against a sibling's grave --
the gateway subnet was gone by the time the gateway deployed), the
first destroy removed the shared stack, and every later destroy failed
"no stack named". `GenerateStackName` now enforces the cap by replacing
the tail with a short hash of the full composed name (deterministic,
collision-free -- see `stackname_test.go`); never reintroduce a blind
`name[:N]` truncation on stack names, and treat "several same-kind
dependencies destroy-failing on ONE shared name" as this class, not
state loss.

### How the Terraform path works

The Terraform runner uses [Terratest](https://github.com/gruntwork-io/terratest)
as its execution layer. For each test scenario:

1. The TF module (`iac/tf/`) is copied to a temp directory
2. `terraform.tfvars` is generated from the manifest proto via `ProtoToTFVars()`
3. `backend.tf` is written with a local backend
4. Provider env vars are extracted from the stack-input YAML by the same loader the CLI and the platform runner use. For Kubernetes lanes there is no provider config, so the loader's local-workflow branch hands the Terraform providers the harness's own `KUBECONFIG` under the names they read (`KUBE_CONFIG_PATH`, `KUBE_CTX`); the harness adds nothing of its own, so a green lane proves what a laptop gets
5. Terratest runs `tofu init` + `tofu apply` with built-in transient error retry
6. The same kubectl verifiers validate the deployed infrastructure
7. Terratest runs `tofu destroy`
8. The same kubectl verifiers confirm cleanup
9. The temp directory is removed

## CI Workflow

The `e2e-kubernetes.yaml` GitHub Actions workflow automates E2E on a weekly
schedule:

1. **build-check** -- compiles E2E code + go vet (runs on every PR too)
2. **discover** -- runs `planton e2e discover --output github-matrix` to
   generate the test matrix from profiles
3. **e2e** -- dynamic matrix of (tier x provisioner) cells, each with its own
   kind cluster, using `gotestsum` for JUnit output
4. **summary** -- aggregates JUnit XML into GitHub Step Summary

To trigger manually: Actions > e2e-kubernetes > Run workflow > select branch.

## AWS E2E

AWS tests live under `e2e/aws/` and verify through the AWS SDK against a real
test account. Credentials are keyless: the harness probes the AMBIENT
credential chain (`sts:GetCallerIdentity`) — locally an AWS SSO session, in
CI the OIDC role.

```bash
AWS_PROFILE=planton-aws-e2e go test -tags=e2e -timeout=45m -v -count=1 \
  -run '^TestAwsIamRole_(Pulumi|Terraform)$' ./e2e/aws/...
```

**Preflight the chain the harness actually reads.** `aws sts
get-caller-identity --profile <name>` proving a NAMED profile valid says
nothing about the ambient chain the SDK resolves — a stale default profile
fails the harness with `InvalidClientTokenId` minutes after the named-profile
preflight passed. Export `AWS_PROFILE` for the test process (as above) so the
CLI preflight, the harness, and both engines' providers all resolve the same
identity, and preflight with that same environment.

**A passing CLI preflight can still hide an expired SSO REFRESH token.** The
CLI serves `get-caller-identity` from its cached SSO access token, but the Go
SDK inside the harness refreshes the token itself — and when the sso-session's
refresh token has expired, the harness fails its very first call with
`InvalidGrantException` ("refresh cached SSO token failed") seconds after the
CLI preflight passed on the same profile. The signature is exactly that pair:
CLI preflight green, harness Setup red with InvalidGrantException. The fix is
a fresh `aws sso login --sso-session <name>` (a browser approval); re-run the
preflight afterward and start the lanes while the session is young rather
than letting a stale login sit ahead of a multi-hour suite.

**A mid-run token death is rescuable IN PLACE — do not kill a retrying
lane.** When the refresh token dies while a lane is running (live hit
2026-08-12: the fixture chain's Pulumi provider looped on "Failed to refresh
cached SSO credentials" between dependency deploys), a fresh `aws sso login`
writes a new token into the shared SSO cache and the running process's SDK
picks it up on its next refresh — the lane recovers and proceeds without a
restart. Trigger the login immediately and let the lane's own retries absorb
the gap; kill and re-run only if the retries exhaust first, and expect the
failed run's teardown to ALSO have failed on credentials — sweep its fixture
chain before relaunching (deployed dependencies stay cloud-side when
DEPENDENCIES-DOWN never got usable credentials).

The rescue's granularity is PER PROCESS, not per lane (live hit
2026-08-13): a `pulumi destroy` SPAWNED while the token was dead can wedge
inside its provider-configure refresh loop and never consult the fresh
cache — observed hung 40+ minutes after the login landed, while sibling
CLI calls succeeded. When a dependency-teardown retry ladder is cycling
and one attempt's child process outlives its plausible runtime, kill THAT
process (SIGKILL if needed — destroys are state-safe to interrupt) and
let the ladder's next attempt relaunch with a fresh process that reads
the new token. Killing the hung child is the in-place rescue; killing the
lane is still the last resort.

Timing observed live (us-west-2): IAM/EIP/Cognito lanes run 30-90s per
engine; a zonal NAT gateway scenario ~7 min per engine end-to-end (create
~2 min, delete ~1 min, fixture chain ~1.5 min up + ~2 min down); a regional
NAT gateway is the same order; an NLB scenario ~6 min per engine (create
2-3 min, delete similar). Budget `-timeout` for scenarios × engines plus the
import round-trip when `PLANTON_E2E_IMPORT_ROUNDTRIP=1` is set (it re-imports
and re-plans every resource, roughly doubling a component's Terraform lane).

**Probe Service Quotas BEFORE any instance-backed SageMaker lane — most
endpoint instance quotas default to ZERO on every account.** The per-type
`... for endpoint usage` quota is 0 by AWS DEFAULT for nearly every
instance family (ml.m5.large and ml.c6i.large included — verified against
`get-aws-default-service-quota`, 2026-08-25): CreateEndpoint fails with
ResourceLimitExceeded and no module or fixture change can fix it. The
entry-level exceptions whose default is 2 are ml.t2.\* (x86) and
ml.m6g.large (Graviton — only for arm64 images); serverless needs no
per-type quota (defaults: 5 endpoints / 10 concurrency per region), and
notebook `ml.t3.medium` defaults healthy. A scenario that is supposed to
run on any fresh account must therefore ride a default-quota type — a
quota-blocked type in a canonical scenario is an authoring defect, not an
environment deferral (the endpoint full-surface scenario was re-pointed
ml.m5.large → ml.t2.medium on exactly this). Probe with
`aws service-quotas list-service-quotas --service-code sagemaker` and
check the DEFAULT (not just the applied value) before concluding a type
is portable: an applied nonzero value can be an account-specific grant.

**When a ValidationException NAMES a regex, the spec's pattern must MIRROR
it exactly — never re-derive the character class from convention.** AWS's
maintenance-window day tokens looked like the uppercase `DDD:HH:MM`
convention siblings use (EKS, ElastiCache accept `sun:23:00`/`mon:...`
lowercase), but SageMaker's MLflow APIs reject `TUE:03:30` with a 400
naming their exact contract: `(Mon|Tue|Wed|Thu|Fri|Sat|Sun):([01]\d|2[0-3]):([0-5]\d)`
— MIXED-case days (live hit 2026-08-25; the kind's own CEL had taught the
uppercase form, so every manifest obeying the spec failed at create). When
a create error quotes the service regex, copy it into the spec pattern
verbatim and add the rejected casing as an explicit invalid-case spec
test; casing conventions do not transfer across AWS service families.

**A CreateModel 400 naming the sagemaker.amazonaws.com service-principal
grant means WRONG REGISTRY ACCOUNT, not a fixable policy.** AWS's prebuilt
framework images live in per-region registry ACCOUNTS (scikit-learn/
XGBoost library: us-west-2 = 246618743249, us-west-1 = 746614075791), and
a cross-region transposition fails CreateModel with "The repository of
your image ... does not grant ecr:GetDownloadUrlForLayer, ecr:BatchGetImage,
ecr:BatchCheckLayerAvailability permission to sagemaker.amazonaws.com
service principal" — which reads like a repo-policy problem on a repo you
could never edit anyway (live hit 2026-08-25, identical 400 both engines).
The fix is the scenario/manifest literal; derive accounts from the pinned
provider's `prebuilt_ecr_image_data_source.go` region maps or the
sagemaker-python-sdk image_uri_config files, never from another region's
example.

**Server-side-only AWS contracts: budget one live probe before a module
debugging spiral.** Some AWS create/update contracts appear in no provider
schema, validator, or CustomizeDiff — they live only in the service (the
Azure section's "ARM contracts exist ONLY server-side" class, AWS edition).
First hits: on ECS Managed Instances, CreateCapacityProvider requires
security groups in the network configuration (the provider schema marks
them optional), and PutClusterCapacityProviders neither attaches nor
detaches MI providers (AWS binds them to their cluster at create) yet
rejects a PUT that merely NAMES one still in its seconds-long PROVISIONING
window; on API Gateway custom domains, CreateApiMapping rejects HTTP-API
mappings on rule-mode domains, and CreateRoutingRule rejects everything
but REST-protocol targets ("Only the REST protocol type is supported" —
that one IS in the provider's website doc, three times, while the schema
types api_id as a plain string: read the resource's DOC page during
design, not only its schema); on SageMaker endpoints, CreateEndpoint
rejects a RollingUpdatePolicy DeploymentConfig when the fleet is a single
instance ("Cannot update endpoint with single instance using
RollingUpdatePolicy ... use BlueGreenUpdatePolicy or removing
DeploymentConfig" — 2026-08-25; the kind's CEL now front-loads the
single-variant case), and an endpoint whose model has NO artifacts parks
at Failed after ~20 minutes ("Unable to successfully stand up your model
within the allotted 180 second timeout" — an artifact-less framework
container passes CreateModel but can never answer ping health checks, so
an ENDPOINT fixture model must be SERVABLE: the harness stages a
~650-byte XGBoost booster via the endpoint's consumer-scoped object-set
override), and note the failed-create endpoint STRANDS cloud-side outside
the engine's state (Pulumi never registered it, the stack destroy missed
it, and the sibling engine's same-name create then 409s "Cannot create
already existing endpoint" — delete the Failed endpoint explicitly before
any relaunch); on RDS, AddRoleToDBCluster/AddRoleToDBInstance
validate that the associated role's trust policy allows rds.amazonaws.com
to assume it and 400 InvalidParameterValue otherwise ("IAM role ARN value
is invalid or does not include the required permissions") — a composed
role fixture therefore needs the RDS-trusting shape (consumer-scoped
override), and note the error names the generic AWS_ROLE_INTEGRATION
class even when a feature_name WAS sent: the trust check runs first, so
the message is not evidence the feature name was dropped; on Route 53,
CreateHealthCheck rejects reserved/documentation IP addresses
("InvalidInput: IPv4 address 192.0.2.1 is forbidden" — the RFC 5737
TEST-NET ranges included, and DISABLED does not exempt the check), so an
endpoint-check fixture must place its placeholder in `fqdn`, never in
`ip_address` (AWS resolves domains at probe time and a disabled check
never probes — the domain placeholder deploys cleanly); on SageMaker,
CreateSpace rejects a space idle timeout wherever idle shutdown resolves
DISABLED for the space through the domain/owner-profile inheritance chain
("Idle Shutdown is disabled for this space, SpaceIdleSettings cannot be set
for this space" — identical 400 on both engines, 2026-08-13: a scenario that
proves the defined-but-disabled profile plane must NOT also set an idle
timeout on a space that profile owns; cross-RESOURCE contracts like this
live in no provider schema, so scenario design must resolve the inheritance
chain by hand — the kind's spec now CEL-rejects the in-manifest
contradiction); on Bedrock guardrails, CreateGuardrail rejects a
STANDARD policy tier without cross-region inference ("Can't configure
guardrail policy tier. Enable cross-Region inference..." — 2026-08-13;
the pairing probe also settled that the API accepts the
geography-qualified profile id "us.guardrail.v1:0" directly while the
PROVIDER's schema demands an ARN — provider-stricter-than-API, the
inverse of the usual gap — so the modules compose the account-scoped
ARN from a caller-identity lookup and committed manifests never embed
an account id); on
Bedrock inference profiles, CreateInferenceProfile rejects a
foundation-model source whose model supports only INFERENCE_PROFILE
invocation ("The provided foundation model does not support On Demand
inference" — the Nova family and most 2025+ models; probe a model's arms
with `list-foundation-models --by-inference-type ON_DEMAND` before
fixing a scenario source); on CloudWatch Logs account policies,
PutAccountPolicy accepts `selectionCriteria` ONLY for
SUBSCRIPTION_FILTER_POLICY in the one grammar `LogGroupName NOT IN
["..."]` — every criteria string on the other four types fails
"Invalid selection criteria provided" (2026-08-25; a five-variant $0
probe settled it — the prefix form some AWS material shows is
rejected too, so the other types are account-wide, period); on
CloudWatch Logs cross-account destinations, PutDestinationPolicy
requires AWS principals as BARE ACCOUNT IDs — the usual
`arn:aws:iam::<id>:root` spelling fails "Principal section of policy
contains ARN instead of account ID" (2026-08-25, identical both
engines; upstream's own test spells `"000000000000"`) — and the bare
id must stay a STRING: an unquoted YAML id parses as a number and the
service then fails "Error occurred while parsing accessPolicy ...
using IAM grammar" (quote it, the sibling of the y-key class); on
Managed Prometheus scrapers, CreateScraper requires at least TWO
subnets ("Number of subnets must be at least 2" — 2026-08-25; the
spec's min_items now mirrors it); and on Managed Prometheus resource
policies, PutResourcePolicy requires every statement's Resource to be
exactly the workspace's own ARN ("Resource in policy does not match
workspace resource ARN" — 2026-08-25; the AgentCore-runtime fix shape
applies: both modules compose the ARN after create and manifests
never carry Resource). When a lane fails with a 4xx the offline
gates never produced, probe the contract directly with the AWS CLI on
throwaway resources (the default VPC makes MI-class probes fixture-free)
before touching the module — ten minutes of probing settled both the
defect and the correct design.

**Parent-serializing control planes: conflict-sensitivity is
PER-OPERATION — probe it, and treat a single probe success as
non-immunity.** Redshift Serverless holds a per-workgroup operation
lock, and the visible failure is 400 `ConflictException` ("An operation
is running on the serverless workgroup") on a satellite operation
seconds after the workgroup went available. Three traps inside the
class, all live-caught in one session: (1) ordering satellite groups
serially is NOT enough, because an operation holds the lock
ASYNCHRONOUSLY after its call returns — a usage-limit create/delete
returns in <1s but flips the workgroup to MODIFYING for ~15-30s
afterward, so the "serialized" next call still conflicts; (2) the
sensitivity is per-operation — CLI probes on throwaway resources
(default-VPC subnets, ~$0, ten minutes) showed usage-limit create/delete
are conflict-immune while endpoint-access create AND delete are
conflict-sensitive; (3) a probe that PASSES once does not prove
immunity — the endpoint DELETE passed a 2-second-gap probe and then
failed the identical crossing in the real lane (the async window's
onset varies); only a conflict OBSERVED proves sensitivity, and repeated
lane greens are what prove a crossing safe. The fix shape that survived:
conflict-sensitive creates first (on the provider-waiter-fresh idle
parent), immune calls last, and the destroy crossing protected
per-engine — Pulumi rides the parent's cascading, conflict-retried
delete via `DeletedWith` (AWS cascades live endpoint accesses;
live-probed), Terraform crosses behind a `time_sleep` destroy settle.
The provider (v6.58.0) retries the conflict only on the workgroup's own
delete/update — never on satellites (upstream gap, recorded). A
provisioned Redshift cluster does NOT serialize this way (its
satellites apply concurrently, live-proven) — never generalize the
class across a service family without evidence.

**Values that advance monotonically across same-name recreates cannot be
pinned literally in scenarios.** Lambda never reuses version numbers for a
function NAME — a recreated function's first publish CONTINUES the deleted
predecessor's numbering. The dual-engine runner recreates every fixed-name
scenario back-to-back, so an alias pinned to version "1" passes on the
first engine (publishes 1) and 404s at CreateAlias on the second (publishes
2): "Function not found ...:function:<name>:1". Point scenario aliases at
"$LATEST" (deterministic regardless of history) and prove the publish arm
through the version OUTPUT, which carries whatever number AWS actually
assigned. The class is any property whose value depends on a name's
history rather than the current resource.

**A sibling of the same class: some services RETAIN per-resource settings
across delete/recreate of the same name — never read a fixed-name
scenario's echo as the service's default.** SES retains an email
identity's feedback-forwarding value keyed by the identity NAME, surviving
DeleteEmailIdentity and re-creation (live-verified 2026-08-12: a fresh
name echoes FeedbackForwardingStatus=true — AWS's default — while the
fixed e2e domain echoes false, inherited from earlier lanes whose feedback
satellite's DESTROY reset it; the provider's satellite delete writes the
API's unset/zero value). Two consequences: (1) an evidence claim about a
service DEFAULT must be probed on a FRESH name (a three-call CLI probe —
create, get, delete — settles it), because a fixed-name scenario's echo is
history-dependent; (2) a provider whose attribute-satellite delete writes
a zero value plants that value permanently on the name — expect
"unmanaged" reads on long-lived fixed-name fixtures to reflect the LAST
manager, not the service default.

**Settings whose revert path validates the PREVIOUS state need that old
dependency ALIVE at revert time — a per-lane fixture cannot carry them.**
The AgentCore token vault is the canonical case (live-caught 2026-08-24):
SetTokenVaultCMK validates the OLD key's state on EVERY write, so
reverting a CustomerManagedKey vault to ServiceManagedKey fails with "Old
KMS Key validation failed ... expected KeyState:ENABLED" once the old key
is disabled or in its deletion window. A chain-deployed KMS fixture dies
into exactly that window at the CMK lane's DEPENDENCIES-DOWN, wedging the
shared account's vault before the revert lane runs. The lane shape for
this class is a STANDING fixture (the S3 Vectors seam):
`aa_e2e.EnsureTokenVaultKMSKeyFixture` keeps one alias-addressed key
(`alias/planton-e2e-acetv`, ~USD 1/month) ENABLED across lanes and runs,
self-healing a key found mid-deletion (cancel + re-enable — the same
recipe that un-wedges a stranded vault: cancel-key-deletion, enable-key,
apply ServiceManagedKey, re-schedule).

**Services WRITE zero-byte permission-check canaries into policy-granted
buckets at CONFIGURE time — and the canaries outlive the configuration.**
Bedrock invocation logging is the canonical case (live-caught
2026-08-24): PutModelInvocationLoggingConfiguration validates the bucket
policy by writing `amazon-bedrock-logs-permission-check` objects under
EVERY configured S3 prefix (archive AND CloudWatch large-data spillover)
with zero model invocations ever made, and deleting the configuration
does not remove them. Second instance (2026-08-25): CreateTrail writes
zero-byte `AWSLogs/` prefix markers into the trail's delivery bucket
before any event is ever delivered, and they survive the trail's
delete. Any bucket fixture a service configuration ever
points at must ship `forceDestroy: true` or its DEPENDENCIES-DOWN fails
BucketNotEmpty — that failure is this class, not a leaked lane resource.

**Services ASSUME the manifest's execution role at CREATE time and
probe bucket ACL verbs — role fixtures need the service's own
managed-policy verb set, not just object read/write.** SageMaker
Feature Store is the canonical case (live-caught 2026-08-25, Q36):
CreateFeatureGroup assumes the spec's role_arn and calls
s3:GetBucketAcl on the offline-store bucket before creating anything
(writes also carry s3:PutObjectAcl — the exact pair AWS's
AmazonSageMakerFeatureStoreAccess managed policy grants). A role
fixture granting only object verbs plus GetBucketLocation fails the
create on BOTH engines with ValidationException "Invalid S3Uri"
wrapping the S3 AccessDenied — the bucket exists and the URI is fine;
the words are misdirection. When a lane's create names a role AND a
bucket, read the service's own managed policy for that feature and
grant the fixture role that verb set verbatim.

**Some AWS APIs MASK not-found as AccessDeniedException — a verifier's
absent-check must learn the service's real gone-signal.** AWS Backup's
DescribeBackupVault is the canonical case (live-probed 2026-08-25): a
deleted (or never-existing) vault name answers 403 "Insufficient
privileges to perform this action", never ResourceNotFoundException —
under credentials that describe an EXISTING vault fine. A verify-absent
that only accepts ResourceNotFound fails every clean teardown of such a
kind. Accepting the masked signal is safe when VerifyExists uses the
SAME call earlier in the lane: real permission loss fails loudly there.
Probe the pair (describe an existing resource, then a nonexistent name)
before writing the verifier's absent arm for a new AWS service.

**Cloud Map-linked Route 53 health checks are undeletable drain, not
orphans.** Health checks Cloud Map creates for health-checked
instances carry `LinkedService: servicediscovery.amazonaws.com`, and
DeleteHealthCheck refuses them with AccessDenied naming the owning
service — even after that service is deleted (live-probed
2026-08-26). AWS's own cleanup sweeps them minutes after the parents
die. A zero-orphan sweep treats lingering linked checks like the FSR
"disabling" reads and KMS scheduled deletions: verify every check is
LinkedService-owned and its parent is gone, then move on.

**S3 Express wraps a deleted directory bucket's not-found inside a
CREDENTIAL error — `errors.As` never reaches it.** Every S3 Express
(directory bucket) operation acquires session credentials via
CreateSession first, so HeadBucket on a deleted bucket fails as
"get identity: get credentials: operation error S3: CreateSession,
... 404 ... NoSuchBucket" (live-caught 2026-08-26, identical both
engines — the destroy had fully succeeded). The credential-acquisition
wrapper chain does not unwrap to a `smithy.APIError`, so the typed
match that works for a standard bucket's 404 silently misses; the
verifier must ALSO match the gone-code textually (`strings.Contains`
on "NoSuchBucket" — the same fallback the bedrock and backup verifiers
use). This is the unmodeled-not-found class's third face: typed
exception missing (Synthetics GetCanary), masked as AccessDenied
(Backup), and wrapped in a credential path (S3 Express).

**The `${E2E_ENV:...}` token expander scans the WHOLE manifest —
comments included.** A fixture comment that quotes the token syntax
with an empty variable name (writing the literal token shape as prose)
fails the manifest at deploy with "invalid environment token"
(live-caught 2026-08-25 on the GuardDuty KMS fixture's own
documentation comment). Name the token in prose; never write the
`${...}` shape in a comment unless it is a complete, valid token.

**Manifests parse under YAML 1.2 rules — only `true`/`false` are
booleans, and duplicate mapping keys are load errors.** The manifest
loader's YAML-to-JSON step (`pkg/protobufyaml`) keeps `y`, `n`, `yes`,
`no`, `on`, and `off` as ordinary strings in both key and value
position, so a CloudWatch dashboard widget authored `y: 2` reaches AWS
exactly as written and `country: NO` stays the string `NO`. History
worth knowing when diagnosing old records: the loader spoke YAML 1.1
until 2026-08-29, which silently rewrote a bare `y` key to `"true"`
(live-caught 2026-08-25 — PutDashboard 400'd with the misdirecting
"Should have property y when property x is present" while the engine's
plan showed `+ true = 2`). Quoting those tokens remains legal and
harmless; it is no longer required. Pasted JSON is always safe (JSON
keys are quoted by definition). One behavior the 1.2 loader ADDS: a
manifest that repeats a mapping key now fails loudly at load instead
of silently keeping the last value.

**A kind whose manifests can deploy OFF the ambient region must export
`region` in its stack outputs — the harness verifies where the outputs
say, not where the scenario deployed.** The harness's VerifyDeployed
resolves the verifier's region from `outputs["region"]`; when the
output is absent the SDK falls back to the ambient region, and a
fixture or scenario deployed elsewhere fails verification with
misleading errors (NoSuchConfigurationRecorder for a healthy us-east-2
recorder; NoSuchConformancePack for a healthy us-east-2 pack —
live-caught 2026-08-25, twice in one session). Any regional
settings-singleton or fixture-serving kind that region-isolated lanes
compose off-ambient needs the output; the kind's own same-region lane
passing proves nothing about this gap.

**AWS can PRE-OCCUPY an account singleton — census the occupant before
designing a create lane, because the collision is unavoidable, not
schedulable.** The known account-singleton classes (one
FIELD_INDEX_POLICY per region, one Config recorder, one default DLM
policy per type) are collision risks BETWEEN lanes — serialization
fixes them. A distinct sub-class exists where AWS ITSELF creates the
singleton occupant: every account that enabled Cost Explorer on or
after 2023-03-27 carries an auto-created DIMENSIONAL/SERVICE anomaly
monitor ("Default-Services-Monitor"), so CreateAnomalyMonitor for that
shape fails with "ValidationException: Limit exceeded on dimensional
spend monitor creation" on effectively EVERY modern account, ours
included (live-caught 2026-08-25, identical on both engines). No
serialization or re-run helps; a canonical scenario riding such a
shape is an authoring defect (the zero-default-quota class, cost
edition). The fix is scenario-level: ride a non-singleton arm (the
CUSTOM monitor), record the pre-occupied arm as deliberately
unexercised with its unblock in the profile, and teach adopters to
IMPORT the auto-created occupant (the kinds' import maps already
derive it). Census probe: `aws ce get-anomaly-monitors` — an occupant
created near the account's Cost Explorer enablement date is AWS's,
not an orphan; never sweep it.

**Raw-JSON Expression documents must be authored in the provider's
CANONICAL STORED form — copy the upstream test's document byte shape
verbatim, nulls and all.** Two normalizations compose on `ce:`
Expression strings (anomaly monitors' monitor_specification; expect
the same on sibling ce: surfaces), each alone enough to fail the
idempotency gate with a proposed replacement (both live-caught
2026-08-25 in one lane): (1) the provider re-marshals the SDK's
Expression struct on read, emitting EVERY member with `null` for
unused ones — and null-vs-absent is not JSON-equivalent to
`SuppressEquivalentJSONDiffs`, so a sparse document (only the members
you use) never matches state; (2) CE echoes user cost-allocation tag
keys back `user:`-prefixed (`aws:` for AWS-generated), so an
unprefixed key never matches either. The upstream acceptance test
spells the fully-populated document with `user:CostCenter` for exactly
these reasons — deviating from the tested form to "clean it up" is how
this class ships. Second family (2026-08-25): AMP's resource-policy
stored form collapses a single-element `Action` array to the scalar
string — an IAM-shaped document authored `"Action": ["aps:RemoteWrite"]`
re-imports as `"Action": "aps:RemoteWrite"` and the round-trip plans an
update the semantic-equality type does not suppress; author the string
form. The general rule for any raw-JSON-string attribute:
the canonical form is whatever the provider's READ path re-marshals,
not the minimal document AWS accepts at create.

**Server-side MATERIALIZED FAMILIES break post-apply plan idempotency —
send what AWS materializes, or declare what it fills in.** Three shapes
of one class, all live-caught 2026-08-25 under the armed idempotency
gate: (1) GuardDuty materializes a detector feature's FULL sub-toggle
family (RUNTIME_MONITORING's trio) with undeclared members DISABLED —
the modules now always send the complete family, declared values over
explicit DISABLED (the fix lives in the module because the family is a
small pinned enum). (2) AWS Backup region settings echo the region's
FULL resource-type opt-in map — the provider owns the whole map, so the
only idempotent manifest declares every type (the fix lives in the
MANIFEST because the map is the spec surface; the spec comment teaches
it). (3) Backup Audit Manager materializes a scope-less resource
control's default all-supported-types scope, and the pinned provider
ships no diff suppression — scenarios declare the scope explicitly
(upstream's own acceptance tests only exercise the scoped form; the
default list grows with AWS, so hardcoding it in a module would freeze
semantics — upstream gap recorded). A fourth instance (2026-08-25,
Q43): Cost Explorer materializes a cost category rule's `type` as
`REGULAR` when omitted — both modules now always send it, declared
value over the REGULAR default (a single pinned enum value: the
module-always-send shape). Choose the fix shape by where the
truth lives: pinned enum → module always-send; spec-surface map →
manifest completeness; growing service default → explicit declaration
plus teaching.

**Cross-region satellites deleted ASYNCHRONOUSLY by the service are an
orphan class the primary-region check never sees.** Secrets Manager is the
canonical case (live-caught 2026-08-13): RemoveRegionsFromReplication only
REQUESTS replica deletion — AWS performs it asynchronously (~40-90s,
probed) — and the provider deletes the primary immediately after, without
waiting. Usually the deletion completes anyway, but a force-deleted
primary (recovery window 0) can outrun it and strand the replica as a
live standalone secret in ITS region. Three grains: (1) verify-absent for
such kinds must check the SATELLITE regions, recorded at exists-time (the
AWS secret verifier keeps an ARN→replica-regions map across the
lifecycle — the stateless per-region probe cannot know them after the
primary is gone); (2) the strand REJECTS a direct delete ("Operation not
permitted on a replica secret") even with its primary deleted — recover
with stop-replication-to-replica in the replica's region, then delete;
(3) a stranded ex-replica BLOCKS the same name's next replication with a
status-only failure ("currently replicated to <region> with a different
arn") that force_overwrite does not clear and NO engine surfaces at apply
(the provider has no replication waiter in either direction) — so
verify-exists must poll every declared replica to InSync, or a green
deploy carries a dead replica claim; (4) a SWEPT replica can be
RE-MATERIALIZED by the deleted primary's replication machinery tens of
minutes later (observed live: fresh CreatedDate 40+ min after the
promote-and-delete sweep, PrimaryRegion pointing at the long-deleted
primary) — the end-of-session sweep must RE-CHECK replica regions after
a settle, not just once. Scenario design: a replicated fixed-name
scenario sits in all these traps across the dual-engine recreate —
run-scope the name and destroy with a recovery window.

**A 4xx from a create call does not mean nothing was created — sweep before
re-running.** Some AWS creates are not atomic: the service materializes the
resource, then validates a later parameter and answers 4xx (first hit:
CreateFunction with `publish_to` in a region where `$LATEST.PUBLISHED` has
not rolled out — the 400 named the parameter, yet the function came up
Active). The engine treats the create as failed, so the resource exists
OUTSIDE engine state: the lane's own destroy cannot remove it, and the
dual-engine runner's second engine then fails its create with a 409
"already exist" on the fixed cloud-side name. The 400-then-409 pair across
engines IS the signature; the recovery is deleting the half-created
resource with the CLI before re-running — and the arm that triggered the
rejection gets trimmed with a recorded deferral, not retried against the
same endpoint.

**Mid-rollout parameters can be ACCEPTED-BUT-INERT — a create accepting a
new parameter is not proof the feature rolled out.** The same feature's
second live contact (Lambda `publish_to`, us-west-2, 2026-08-13, two days
after the rejection above): CreateFunction ACCEPTED
`PublishTo: LATEST_PUBLISHED` with a clean 201, yet the `$LATEST.PUBLISHED`
qualifier answered ResourceNotFoundException after publishing versions
through BOTH the explicit publish-version path and update-code
`--publish` — the head pointer never materialized — while UpdateFunctionCode
still rejected the identical value with the original
InvalidParameterValueException. Rollouts are per-OPERATION and acceptance
is not activation. Before re-arming a deferred arm whose unblock condition
is "the region accepts it": probe the feature's OBSERVABLE EFFECT (here,
the qualifier resolving after a publish on a fresh throwaway function),
never the create call's status code alone — and probe the sibling
operations too, because a divergent reject (update rejecting what create
accepts) is itself the mid-rollout signature.

**ECS Managed Instances provider deletion can FAIL SILENTLY, and the
cluster cannot delete until every MI provider is INACTIVE.**
`delete-capacity-provider` answers DEPROVISIONING even when the delete is
doomed: minutes later the provider bounces back to ACTIVE with
`updateStatus: DELETE_FAILED` ("Cannot remove capacity provider. It is
either part of the default strategy or has non stopped tasks") when the
cluster's default strategy still names it. Clear the strategy first
(`put-cluster-capacity-providers` with a strategy that does not name it),
then delete — an unreferenced provider deprovisions in about a minute —
and only then delete the cluster (`DeleteCluster` fails with "Cluster
cannot be deleted while Cluster Scoped Capacity Providers are attached"
until then). Module destroys are safe on both counts — the association
resource (which owns the strategy) destroys before the providers, and the
provider's delete path waits — but MANUAL sweeps (CLI probes, orphan
cleanup) must follow that order and check `updateStatus` for
DELETE_FAILED rather than trusting the delete call's answer. A zero-orphan
sweep that finds an ECS cluster lingering should check its capacity
providers' status before suspecting a failed teardown.

**A committed fixed S3 bucket name is a lane time bomb — the namespace is
global and this repo is public.** A scenario's fixed bucket name can be
claimed by ANY AWS account (the repo publishes it), and a recently-deleted
name can sit in a propagation window where CreateBucket answers 409
`BucketAlreadyExists` while HeadBucket answers 404 and `list-buckets`
proves the account owns nothing by that name (live-hit 2026-08-11: the
awss3bucket full-surface name 409'd across retries with no visible owner).
The signature is exactly that 404-but-409 pair; the recovery is NOT
waiting out the window but making the class impossible: put
`${E2E_RUN_ID}` in the scenario's metadata.name so every run's bucket name
is globally fresh. S3 is the recorded exception to the
names-stay-stable token guidance — its cloud identifier IS the metadata
name (the module derives the bucket name from it) — legitimate only for
scenarios no prerequisite chain references by name. **S3 Tables has the
same class with a sharper signature**: a just-deleted table bucket's
name answers CreateTableBucket with 409 ConflictException "The bucket
is in a transitional state because of a previous deletion attempt"
(live-caught 2026-08-26 — the second engine's create collided with the
first engine's teardown seconds earlier; the run id's engine suffix is
exactly what makes the run-scoped name immune). Directory buckets do
NOT hold the window — a back-to-back same-name recreate succeeded live
— and vector buckets were not observed either way.

**Fixed-name shared fixtures collide across CONCURRENT sessions on one
account.** IAM roles are account-global and the shared install profiles
(e.g. the 13-role `awsiamrole` prerequisite set) use fixed names by
design, so two sessions' lanes composing the same fixture race:
the second `CreateRole` fails 409 `EntityAlreadyExists` while the first
session's lane holds the deployment (its teardown removes the set at
DEPENDENCIES-DOWN). Diagnose with the role's `CreateDate` (minutes old =
a live sibling lane, not an orphan) before deleting ANYTHING — deleting a
concurrent lane's live fixture wrecks that lane's destroy. The recovery
is to wait for the holder's teardown window (poll `get-role` until
NoSuchEntity) and launch immediately; the collision recurs at most until
the sibling session's lanes finish. Kinds that expose no creation
timestamp (an SNS topic, say) cannot use the CreateDate test — there the
diagnosis is process evidence: with no concurrent session holding lanes,
an already-existing fixed-name fixture is an orphan of an earlier
interrupted teardown; delete it by CLI and re-run.

**Before diagnosing a cloud-side anomaly, rule out a SIBLING EXECUTION of
your own lane.** A lane launch that your tooling REPORTS as failed can
still have spawned (live hit 2026-08-12: a launcher errored on argument
validation after forking — the "failed" lane deployed its fixture chain
minutes later), and an interrupted supervising shell can kill a lane
mid-DEPENDENCIES-DOWN, stranding the whole fixture chain. The signatures:
already-exists 409s on fixed-name fixtures nothing should hold, evidence
watchers capturing MORE resource lifecycles than your lanes explain, a
lane log growing after its process "finished", or two `ok`/`FAIL` package
trailers in one log file. The checks are cheap and decisive: `ps` for
`go test`/`aws.test`, `lsof` on the lane log, and RUN-header/trailer
counts in the log. This is the session-level "an interrupted session is
alive until its window is CLOSED" rule at process granularity — and a
killed-mid-teardown lane means a manual dependency-ordered sweep
(attachment before gateway, subnets before VPC) before any relaunch.

**The ps check is a HARD PRE-LAUNCH GATE, not just a diagnosis aid — and
it must gate, not merely print.** Second and third live hits of the class
(2026-08-13): an agent-tooling launcher reported a lane invocation
complete-with-failure after 23 seconds while the invocation's `go test`
kept running for TEN MORE MINUTES (deploying and destroying the full
13-role IAM fixture set), and a relaunch issued on the strength of that
false completion collided 409 with the invisible sibling's live fixtures;
later the same day one launched command produced TWO complete trailer
sets in one log file — two full executions, which stayed green only
because they shared run-scoped dependency stacks. Binding rules: (1)
before ANY lane launch or relaunch, run `ps` for the exact `-run` pattern
and let a non-empty result BLOCK the launch (a chained command that
prints the count and proceeds anyway is not a check — the same
gate-vs-print failure the lock protocol records); (2) after every
launch, verify exactly ONE `go test` driver + ONE `*.test` binary exist
for the pattern; (3) count `^ok |^FAIL` package trailers in the lane log
before citing it as evidence — two trailer sets means two executions and
the log's phases interleave; (4) in a PERSISTENT/stateful agent shell, the
gate must block by SKIPPING the launch (an `if` that simply does not run
it), never by `exit` — an `exit` kills the shell itself, agent tooling
restarts it and can RE-RUN the whole chain, and the re-execution races the
first (two live hits 2026-08-13: a "blocked" gate report belonged to the
duplicate execution while the first was already running the lanes; the
truth is always the process table + the log file, never the launcher's
printed verdict).

**A re-executed launch can TRUNCATE its own log, making the first
execution invisible — anchor forensics on cloud-side timestamps, not the
log.** The duplicate-execution class above (one launch, two runs) has a
harder grain when the lane command pipes through `tee` WITHOUT `-a`: the
re-execution truncates the shared log file, so the trailer count reads ONE
and the log looks like a single clean run — while the first execution's
fixtures are live in the cloud, colliding 409 with the second's and
stranding orphans when the first dies teardown-less (live hit 2026-08-13:
a Bedrock flow lane's first execution created the fixed-name MANAGED-KB
prerequisite six minutes before the reported execution's own start, which
then failed 409 against it; the stranded KB outlived both). The signature
is a cloud resource whose `createdAt` PRECEDES the reported command's
possible start window. Defenses: pipe lane logs through `tee -a` with a
per-launch header line (echo a timestamped RUN marker before `go test`) so
a truncation-invisible predecessor cannot exist, and treat any
fixture-collision 409 as unattributed until the resource's create time is
checked against the launch window.

**When an authoring session holds the checkout, run proof lanes from a
dedicated detached worktree at the proven HEAD — and never sync edits
into it while a lane runs.** A live authoring session's in-flight edits
to shared packages (the verify package, `go.mod`) can leave the main
tree uncompilable for `go test` at any moment; `git worktree add
--detach <path> HEAD` gives the lanes the last committed (proven) tree,
with the proof session's own files copied in. Two hard rules from the
first live use (2026-08-14): (1) copy files into the worktree ONLY
between lanes — a stub synced mid-run broke the running lane's own
DESTROY, because `pulumi destroy` recompiles the module program against
the tree as it stands and a new CEL rejected the in-flight stack-input
(the never-edit-what-a-lane-reads class via the sync side door); and
(2) the main tree stays the record tree — profile flips and spec/module
fixes land there first and copy over, so the wrap commit never depends
on worktree state. Expect a sibling's commit to overwrite YOUR
uncommitted edits to shared files (import catalog, verifier map) —
re-apply on top after their commit lands; the worktree copy keeps the
lanes honest meanwhile.

**A stale AWS CLI silently DROPS new API surface from its output — verify
the CLI's model before diagnosing a missing field.** The CLI parses
responses against its bundled service model and discards members it does
not know, so evidence gathered with `aws <svc> get-*` can show a
just-modeled field as absent while the cloud resource carries it (live
hit 2026-08-12: `get-distribution-config` from aws-cli 2.33.24 omitted
CloudFront's `CacheTagConfig` — same-wave `ResponseCompletionTimeout` and
`IpAddressType` appeared fine — while the Terraform destroy-refresh read
the header back through the provider's own SDK, proving it landed; the
false negative briefly looked like a Pulumi-engine silent drop). Before
treating a missing field in CLI output as a module or provider defect,
check whether the CLI even knows it:
`aws <svc> <create-op> --generate-cli-skeleton | grep <Field>`. When the
model is stale, read the evidence through this repo's own pinned
`aws-sdk-go-v2` (a `go doc` check plus a small probe) or through the
engine's refresh diff — the same stale-local-tooling class as the
installed `planton` binary misreporting new spec fields (use the
working-tree CLI). **Check the OUTPUT skeleton, not just the input one**
(`--generate-cli-skeleton output` on the describe/get op): the two halves
of the CLI's model update independently, and a CLI that ACCEPTS a new
field on create can still DROP it from describe output (live hit
2026-08-12: aws-cli 2.33.24 knew `TargetControlPort` in
`create-target-group` input while `describe-target-groups` output
silently omitted it — the pinned elbv2 SDK read it fine).

**Short-lived resources need a PRE-ARMED evidence watcher, not an
after-the-fact probe.** Target groups, IAM objects, and other
seconds-scale resources are deployed, verified, and destroyed faster
than a human-in-the-loop probe can react — a mid-lane evidence capture
attempted after noticing the deploy line typically answers NotFound
(live hit 2026-08-12: a target group's whole lifecycle ran in ~70s and
the first probe missed the window). Start a background poller BEFORE
launching the lane — poll the resource's fixed cloud-side name (or its
`planton.ai/resource-id` tag) every few seconds, capture the evidence
calls the moment it exists, and let it idle through both engines' windows
so each lane's instance is sampled independently. The poller doubles as
per-engine attribution: two capture blocks with different resource IDs
prove BOTH engines' instances carried the arm.

Four watcher-authoring traps, all live-caught 2026-08-13: (0) **zsh
RESERVED parameters carry setter semantics** — assigning `GID` (also
`UID`, `EUID`, `USERNAME`) in a zsh watcher script attempts to change
the process's group id and kills the script with "failed to change
group ID: operation not permitted"; a watcher log that contains only
its header line with the process gone is this class — name loop
variables defensively (`G_ID`), same family as the `:s`/`:l` modifier
trap below. (1) **watch
the name the MODULE derives, not metadata.name** — some kinds name their
cloud resource from a spec field (ECR's `spec.repository_name`), and a
watcher armed on metadata.name polls a resource that never exists,
producing an all-NotFound log that looks like a timing miss (28 clean
iterations, zero captures — beyond plausible bad luck is the signature;
read the module's locals before arming). (2) **In zsh, BRACE every
variable expansion that a colon follows inside a composed ARN** —
`"$ACCT:stateMachine:x"` silently applies the history-style `:s`
substitution modifier and `"$ARN:live"` the `:l` lowercase modifier,
mangling the ARN into a shape the service rejects (or worse, a DIFFERENT
valid ARN); write `"${ACCT}:stateMachine:x"` / `"${ARN}:live"`. (3)
**Size the poll interval to the resource's ALIVE window, and keep the
loop single-purpose** — a multi-service loop whose per-iteration API
latency exceeds a seconds-scale resource's lifetime misses every window;
give sub-minute kinds a dedicated 1-2s watcher and re-run their
minutes-cheap lanes when a window was missed (the re-run doubles as a
repeatability proof).

**Destroy-ORDER claims need the engine's own log, never a poller.** A
polling watcher cannot order two deletions that land inside one poll
interval (live hit 2026-08-12: an event archive and its bus both went
absent within one 5s window, leaving the archive-before-bus ordering
unprovable from the watcher alone). Terraform's destroy log states the
order explicitly per resource ("Destruction complete" lines); Pulumi
guarantees child-before-parent structurally when the module parents the
dependent resource (`pulumi.Parent`). Cite those; keep the watcher for
per-engine attribute evidence, not sequencing.

**A change-deduping watcher must RE-ARM on absence, or the second
engine's instance is silently skipped.** Deduping captures by snapshot
hash keeps the evidence file readable, but kinds with no status
transitions in their describe output (a CodePipeline pipeline, a
CodeBuild project — fully-formed on first describe, byte-identical
across engines on a fixed name) produce ONE capture for two engines:
the dual-engine runner destroys and recreates the same-named resource,
and the second instance's snapshot hashes identically to the first
(live hit 2026-08-12: the pipeline watcher captured only the Pulumi
window; the Terraform window's evidence had to be recovered from the
destroy-refresh in the lane log). Clear the dedupe hash whenever the
probe answers NotFound — the absence between lanes is exactly the
engine boundary — so each engine's instance captures even when the
payloads match. Status-cycling kinds (caches: creating → available →
deleting) mask this bug by hashing differently anyway; do not conclude
from them that a watcher is correct. Two hardenings that make the class
structurally impossible (live-proven 2026-08-12): capture RAW snapshots
continuously and dedupe at ANALYSIS time instead of capture time — the
raw file is cheap for a lane-length window and nothing is lost to a
buggy live dedupe — and key the analysis on a per-instance discriminator
rather than the config payload: `creationTime`/`CreateDate` when the
kind reports one (a fixed-name log group's two engine windows hash
identically but carry distinct creation times), or the AWS-generated ID
when the kind mints one per create (health checks, Cognito clients —
those need no re-arm at all).

## GCP E2E

GCP tests live under `e2e/gcp/` and use the shared `aa_e2e` harness with
real GCP API verification. Set `E2E_GCP_PROJECT` to a dedicated test project
with Application Default Credentials.

```bash
E2E_GCP_PROJECT=planton-e2e go test -tags=e2e -timeout=120m -v ./e2e/gcp/...
```

**ADC preflight must assert a NON-EMPTY token, not just exit code 0:** with a
stale-but-present ADC file, `gcloud auth application-default print-access-token`
can exit 0 while printing an EMPTY token (observed live on a credential file
whose refresh token had aged out) — so `print-access-token >/dev/null && echo OK`
false-passes and the failure surfaces minutes later inside a lane. A machine
preflight should capture the token, assert it is non-empty, and make one live
API call with it (e.g. GET the test project via Cloud Resource Manager). The
harness `Setup` itself fail-fasts correctly — this trap only bites pre-session
checks that trust the exit code.

**Read-after-delete is eventually consistent on several GCP APIs — poll,
never single-GET, in VERIFY-CLN.** Two live-caught members so far: IAM
service accounts (a GET within seconds of a successful
`serviceAccounts.delete` can still answer 200 from a stale replica —
destroy succeeded, VERIFY-CLN 1.5s later still saw the account, while an
identical cycle minutes earlier read 404 immediately) and Cloud Run
domain mappings (the regional Knative API kept answering 200 for ~40s
after the provider's destroy returned clean — the poll passed at 43.5s).
Both verifiers poll for up to 90s at 10s intervals; the SA verifier
additionally treats 403 as a recently-deleted signal alongside 404. When
a new kind's VERIFY-CLN fails with "still exists" seconds after a clean
destroy, probe the object manually before touching module code — if it
is gone minutes later, the kind is a new member of this class and its
verifier needs the poll posture, not the modules a fix.

**Backend contract:** every scenario's test context must carry the run-scoped
Pulumi file backend URL — even Terraform scenarios, because dependency
prerequisites always deploy via Pulumi. An empty backend URL silently falls
back to the machine's ambient `pulumi login`, coupling runs to developer
state that can vanish mid-run (e.g. a stale `/tmp` backend).

**Timeout guidance:** scenarios with private services access prerequisite chains
(VPC → global address → service networking connection) or Memorystore
create/destroy need substantial headroom. Budget **≥35m per PSA-backed scenario**
and **≥90m** when batching multiple PSA or Memorystore scenarios in one `go test`
run — Redis instance destroy alone can exceed 15 minutes. GKE clusters are the
slowest resources in the harness: a control-plane create runs 10-25 minutes and
a destroy 5-10, per scenario per engine — budget **≥150m** when batching the
GKE cluster scenarios across both engines. Dataproc clusters boot real VMs:
budget **≥90m** when batching multiple cluster scenarios across both engines.
Cloud Composer environments are the longest single resource: a create runs
25-45 minutes and a destroy 10-15, per scenario per engine — budget **≥240m**
for a both-engines batch, and the same for any kind whose chain installs an
environment as a prerequisite (the user-workloads Secret/ConfigMap).

**Managed services that VALIDATE runtime identity at create:** some control
planes reject creation unless the workload identity already holds the right
role — Dataproc requires the VM service account to hold `roles/dataproc.worker`
(hardened projects grant the default compute service account nothing), and
Composer 3 requires an explicitly specified workloads service account holding
`roles/composer.worker`. Model the identity as a registry prerequisite with a
consumer-scoped `GcpServiceAccount` profile carrying the additive grant in
`project_iam_roles` — a plain SA prerequisite without the grant fails the
scenario at create with a permissions error, not at verify.

**Test-side data staging (kinds whose deploy needs pre-existing object bytes):**
Some resources cannot apply until a blob already exists in cloud storage — Gen
2 Cloud Functions read their source from GCS at deploy time, and object bytes
cannot be expressed as IaC. The runner has no hook between prerequisite deploy
and the scenario apply, and `valueFrom` resolution only touches
`StringValueOrRef` fields (not plain-string bucket/object paths). The pattern is:
check in a minimal source tree under `e2e/fixtures/`, and in the test
entrypoint (before calling the runner) zip it and upload to a run-scoped bucket
whose name uses the same engine-scoped `${E2E_RUN_ID}` expansion the runner
applies (`runner.EngineScopedRunID`). Register cleanup on the test handle — the
harness `Teardown` is a no-op by design. See `e2e/gcp/gcp_test.go`
`stageCloudFunctionSource` and `gcpcloudfunction/e2e/scenarios/*.yaml`.
Serverless VPC Access connectors are slow: budget **≥15m per connector scenario**
(CREATING → READY is typically 5-10 minutes). Cloud Functions Gen 2 deploys
(including Cloud Build) need **≥20m per scenario**.

**One manifest per prerequisite kind — scenarios needing TWO instances of the
same kind cannot express the second live.** The dependency resolver installs
each prerequisite kind from exactly one manifest (consumer-scoped override,
else the published profile, else the minimal scenario), so a scenario that
composes two instances of one kind — e.g. a Pub/Sub subscription whose parent
topic AND dead-letter topic are distinct topics — can only resolve the first
by reference. Prove the second-instance arm with the offline converter plan
(the reference-resolution mechanism is identical to the live-proven first
instance) and record the exclusion, rather than pointing both references at
one instance (semantically wrong) or hand-rolling a second install path.

**Reservation-window resources need consumer-unique AND scenario-unique
prerequisite names:** `${E2E_RUN_ID}` makes cloud-side names unique per run
— but two scenario chains in the SAME run that install the same
prerequisite profile still produce the same name, and for resource classes
that reserve a deleted ID for minutes after destroy (Firestore database
IDs are held ~3-5 minutes), the second chain's create collides with the
first chain's just-deleted ghost ("not available ... retry in N seconds").
Two grains of the same trap:
- *Multiple consumers* chaining one prerequisite kind: give each consumer a
  consumer-scoped override whose cloud-side name embeds the consumer
  (e.g. `e2e-fsidx-db-...` vs `e2e-fsbs-db-...`).
- *Multiple scenarios of ONE kind*: the kind's own override redeploys per
  scenario, so even a consumer-unique run-scoped name is deleted and
  recreated at every scenario boundary. Embed `${E2E_SCENARIO}` (expanded
  to the running scenario's slug) alongside the run id
  (`e2e-fsidx-db-${E2E_SCENARIO}-${E2E_RUN_ID}`) so no name is ever
  recreated within a run.

**Same-kind dependency stacks and the 50-character bound:** dependency
stack names carry the manifest name precisely so two instances of one kind
get two stacks — and the name bound is enforced by a uniqueness-preserving
truncation (`boundStackName`: readable head + stable hash of the full
name). This is load-bearing: a plain head-truncate once collapsed a
two-database Firestore chain onto ONE stack (the kind slug alone is 20
characters), so the second fixture's `up` silently REPLACED the first and
teardown destroyed the shared stack once, then failed `no stack named` on
the other. A `no stack named` error during DEPENDENCIES-DOWN therefore has
TWO known causes: backend state loss mid-run (see the Azure section) and —
if two same-kind dependencies' stack names agree — a truncation collision
in a modified stack-naming path.

**Identity-derived cloud IDs need run-scoped metadata names:** when a module
derives the cloud-side resource ID deterministically from the resource
identity (org/env/metadata.name — Vertex AI endpoints were first) AND the
service reserves deleted IDs (a destroyed endpoint's numeric ID 409s on
recreate while its GET returns 404), a fixed `metadata.name` makes the
scenario single-shot: the second engine — deriving the byte-identical ID —
collides with the first engine's ghost. Put `${E2E_RUN_ID}` in
`metadata.name` itself, an explicit exception to the fixed-name rule that is
only legal for leaf kinds nothing FK-references (prerequisite resolution
keys off metadata names). Record the exception in the scenario comment.
Corollary: such a cross-engine collision is itself the strongest live proof
that both engines derive the same ID.

**Validate every scenario and prerequisite manifest offline before the first
live run:** manifests load through `protojson.Unmarshal`, which hard-rejects
unknown fields — a mistyped field name (`subnetwork` for `subnet`,
`network` for `vpcSelfLink`) fails at manifest load after minutes of
prerequisite deploys. `planton validate <manifest>` (with `${E2E_RUN_ID}`
text-substituted) catches this in seconds; run it on every new scenario,
prerequisite, and preset YAML as part of the offline gate.

**Offline fixture-integrity gate (FK resolvability):** a `valueFrom` that
names a prerequisite the chain never deploys — or an ambiguous name among
several instances of the same kind — fails only deep into a live run (or
worse, silently: an unresolvable ref is left untouched, and DEPLOY fails as
a provider validation error that reads like a module defect). The gate at
`e2e/framework/runner/fixtureintegrity.go` (`TestCatalogFixtureIntegrity`)
replays `ResolveDependencies` + `forEachRefField` statically against every
committed scenario, with no cloud and no credentials. It mirrors
`lookupRefValue` one for one — including the sole-instance fallback (a
topology-named reference still resolves when exactly one instance of the
kind exists; that is design, not a defect). Pre-existing findings live in
`fixture_integrity_baseline.yaml` and only ever burn down; a new finding is
a manifest fix, never a new baseline entry. Run it with
`go test ./e2e/framework/runner/ -run TestCatalogFixtureIntegrity`.
Components whose profile records `status: deferred` are skipped by the
catalog walk: a deferral is the kind's own record that its lanes cannot run
(some wall-class prerequisites are structurally unshippable — e.g. an AWS
Organization fixture would mutate the shared account irreversibly), so the
gate asserts only what the kind's records claim. It re-arms automatically
when the profile leaves `deferred` — exactly when the chain must work. A
missing or unreadable profile never earns the skip.

**Inherited fixture orphans 409 the whole chain — sweep the fixture
families, not just the kinds' own objects:** prerequisite fixtures use
FIXED names by design (FK resolution keys off `metadata.name`), so a
leftover fixture from ANY earlier era — a crashed run, a pre-harness
experiment — collides with a fresh chain as `Error 409: already exists`
at DEPENDENCIES-UP, from a run-scoped backend that has no state for it.
Live-caught with a month-old `planton-oss-e2e-gcphc-prereq` health check
plus its backend service and a serverless NEG: three sequential 409s,
each surfacing only after the previous one was cleared. The fix is
always to DELETE the leftover (a pre-existing fixture object is by
definition unmanaged — no lane is running when a session starts), and
the honest end-of-session sweep must enumerate every fixture FAMILY the
session's chains deployed (for LB chains: health checks, backend
services, NEGs, URL maps, proxies, addresses), not only the proven
kinds' own resource classes.

**Named image families are a staleness trap in scenarios:** GCP retires
image families (the deep-learning-VM notebook families like
`common-cpu-notebooks` no longer resolve), and a scenario pinned to one
fails live years after it was written. Prefer the service's default image
(omit the image block) in E2E scenarios and presets; pin a family only when
the arm under test requires it, and prefer the service's maintained family
(e.g. `cloud-notebooks-managed/workbench-instances`) over frozen ones.

**First-ever API activation outruns in-module enablement:** every module
enables its service APIs with a dependency edge before creating resources,
and that is sufficient for a project where the API has been active before.
But the FIRST-ever activation of a service on a project propagates slowly —
the create call can land minutes before the activation is visible and fail
with a 403 "API has not been used in project ... before or it is disabled",
even though the enablement resource completed. When a new service family
first joins the harness, pre-enable its APIs on the test project once
(`gcloud services enable <api> --project <test-project>`) before the first
live run; from then on the in-module enablement carries every future run.

**The same class has a SERVICE-AGENT flavor:** some services also provision
a per-project service agent asynchronously after enablement, and the first
create can race it — Workflows rejects with 400 "Workflows service agent
does not exist" minutes after the API itself reads ENABLED (live-caught:
the very next scenario, 40 seconds later, passed). Alongside the API
pre-enable, pre-provision the agent explicitly and idempotently via
Service Usage (`POST .../v1beta1/projects/{p}/services/{api}:generateServiceIdentity`
— also the right pre-step for Eventarc's P4SA); a failure of this shape on
a first-ever run is environmental, never a module defect — re-run before
touching code.

**Fleet-backed managed services can fail on ZONAL CAPACITY, wearing an
"internal error" mask:** a Serverless VPC Access connector create failed
repeatedly with `Error code 13 ... VPC Access connector failed to get
healthy. Please check GCE quotas, logs and org policies and recreate` while
quotas, IAM, and org policies were all verified healthy. The truth was in
the AUDIT LOG: every `v1.compute.instances.insert` for the connector's
managed `aet-*` fleet failed with `ZONE_RESOURCE_POOL_EXHAUSTED` across
us-central1-b/c/f — a GCP-side stockout of the zonal machine pools, hit for
e2-micro AND e2-standard-4 in the same window, while the identical connector
went READY in us-east1 minutes later. The diagnostic recipe: query Cloud
Logging for `severity>=WARNING resource.type="gce_instance"` in the failure
window and read the insert errors — never debug the module first. The fix
is regional: a self-contained fixture chain (the connector's own VPC/subnet)
moves to a quieter region; a chain shared with other kinds records the
deferral with the stockout evidence instead. A failed connector also
CASCADES: its dead fleet's auto-created `aet-*` firewalls and instance
groups hold the VPC/subnetwork against deletion past the framework's
6×60s teardown retries, so sweep connectors (state ERROR) FIRST, wait for
GCP to reap the fleet, then delete subnets and networks.

The same class has a GKE-CREATE flavor that needs no audit-log query — the
create error itself carries `[GCE_STOCKOUT]` verbatim: a Standard cluster
create boots TRANSIENT default-pool instances (one per zone on a regional
cluster) even when the module removes the default pool, and the cluster
create fails after ~35 minutes of IGM waiting ("Not all instances running
in IGM ... [GCE_STOCKOUT] ... does not have enough resources available")
when any zone is stocked out. A zonal cluster in the same region can pass
minutes earlier while the regional shape fails — regional spreads across
three zones, so ONE exhausted zone fails the whole create. The fix is the
same regional relocation, and because clusters must be region-matched to
their subnetwork fixture, the kind's WHOLE chain (all scenarios + the
consumer-scoped subnetwork + the cluster install profile, including
same-shaped copies under consuming kinds) moves together.

The class also has a DATAPROC flavor, and auto-zone placement does NOT
dodge it: with no zone pinned, Dataproc's own placement still chose the
exhausted us-central1-a and the create failed with
ZONE_RESOURCE_POOL_EXHAUSTED / "does not have enough resources" on
e2-standard-2 (live-hit 2026-08-12; single-node AND multi-node shapes
alike). Same response: relocate the kind's whole region-matched chain.

**Some GCP APIs require a quota project on user-credential calls:** the
Identity Toolkit API (Identity Platform) rejects calls made with plain
user ADC — 403 "requires a quota project, which is not set by default" —
and the Terraform provider sends the `X-Goog-User-Project` header ONLY
when `user_project_override` is armed in the provider config (the ADC
file's own `quota_project_id` is NOT honored by the provider transport;
live-verified). Kinds whose APIs carry this requirement wire
`user_project_override = true` in their TF provider block and build their
Pulumi provider via `pulumigoogleprovider.GetWithUserProjectOverride` —
a module-level, customer-facing fix, never a harness export. The typed
verifier clients are unaffected (google.golang.org/api transports honor
the ADC quota project).

**Once-only initialization singletons need an adopt arm for every lane
after the first:** some project singletons initialize exactly once EVER —
Identity Platform's `initializeAuth` hard-fails a second call with 400
"Identity Platform has already been enabled for this project"
(live-verified), and destroy only abandons. The first live lane spends
the project's single create; every later lane (the same engine's second
scenario, the other engine, every prerequisite chain) must ADOPT instead.
The catalog pattern is a spec-level `adopt_existing` switch (TF: a
for_each-gated `import` block on the deterministic singleton ID; Pulumi:
a conditional `pulumi.Import` resource option) with the kind's e2e
fixtures arming it permanently once the test project is initialized —
Pulumi's import matches because the modules only specify spec-driven
inputs, and Terraform's config-driven import applies spec drift as an
update in the same apply.

**Typed-client gaps in the pinned Google API line:** a brand-new GCP service
may have no typed client in the repo's pinned `google.golang.org/api`
version (Memorystore for Valkey was first). Do not bump the shared
dependency mid-component for one verifier — the harness carries an
ADC-authenticated plain HTTP client (`Services.RestClient`) for exactly
this: probe the service's documented REST GET path and decode the few
fields the posture assertions need. Swap to the typed client whenever the
dependency is next upgraded for its own reasons.

**Shell harnesses can DOUBLE-SPAWN a lane invocation — and an interrupted
invocation orphans the second spawn's fixtures.** Four-times-observed
signature: one launch produces TWO OR MORE `go test` processes with
different run ids; the first runs to a clean PASS while the second either
409s on the first's fixtures or — if the invocation is interrupted/killed —
dies mid-scenario with its run-scoped chain undestroyed. The third
observation adds two facts: it strikes WITHOUT any interrupt (a chained
`go test ... && go test ...` command was doubled whole, both siblings ran
to clean completion), and the collision surface includes the SHARED module
source directories — the sibling's dependency deploy overwrote the kind's
`stack-input.yaml` between a lane's apply and its IDEMPOTENCY preview, so
the preview compiled against the sibling's prerequisite manifest and
reported a bogus replace (a different cloud-side name from a run id no
logged lane used is the tell). The fourth observation adds a SERIAL
flavor with two new tells: a single un-chained invocation was spawned
THREE times back-to-back (edges overlapping), and because the launch
piped through `tee` without `-a`, each respawn TRUNCATED the log — the
surviving file and the reported wall-clock describe only the LAST spawn,
so the invocation looks singular unless you reconstruct from the cloud
audit log (query the service's create/delete methods for the window and
group by the run-id embedded in resource names; every run id beyond the
logged one is a phantom spawn). Mitigations: launch ONE `go test`
invocation per shell command (never chain lanes — a doubled chain
multiplies the overlap window), and smoke-check for a sibling process
right after launch.
When killing a sibling spawn, kill its PROCESS GROUP (`kill -- -<pgid>`),
not just the `go test`/test-binary PIDs: an in-flight `pulumi up` child
survives a parent-only kill and COMPLETES its deploy minutes later,
leaving a fully-created orphan chain (live-observed: a killed secret
lane's surviving child finished a 20-minute Composer environment create
with nobody left to destroy it).
Diagnosis and recovery:
before sweeping anything after a lane failure or interrupt, `ps` for a
concurrent sibling process (a live sibling's "orphans" are its own
resources mid-teardown — racing it corrupts both runs); once no process
remains, sweep BY RUN ID — list each fixture family and delete objects
whose names carry a run-id suffix that matches no logged lane (creation
timestamps from the audit log date the dead spawn precisely).

**Run GCP e2e batches SEQUENTIALLY — never two `go test` processes at once:**
the GCP dependency deploys write each prerequisite's stack-input into the
SHARED module source directories under `apis/.../iac/pulumi`, so a second
concurrent `go test` process picks up the first's on-disk manifests and
collides on the other run's names (409 already-exists / cross-contaminated
stacks). Run one component's batch to completion (or one scenario at a time)
before starting the next; parallelism within a single process is bounded by
the framework, but two processes are not isolated from each other.

**Intra-cluster firewall is part of a multi-node cluster's minimum
composition on a custom VPC:** custom-mode VPCs have no default firewall
rules, and managed multi-node clusters (Dataproc among them) refuse to reach
RUNNING because master↔worker traffic is dropped — the create hangs, then
times out, rather than failing fast. Model an intra-cluster `GcpFirewallRule`
(allow ingress within the VPC/subnet) as a registry prerequisite for such
kinds, exactly like the identity and networking prerequisites.

**Dataproc's regional staging/temp buckets persist and carry per-creator
ACLs:** Dataproc auto-creates `dataproc-staging-<region>-<project#>-<hash>`
and `dataproc-temp-...` buckets (a deterministic name per project+region) and
reuses them across runs; it never deletes them. If an earlier run created them
under a since-deleted VM service account, a later run's identity can be locked
out ("Permissions are missing ... on the staging_bucket") even with correct
project-level IAM, because the bucket's fine-grained ACLs still name the old
principal. When a Dataproc live batch fails on bucket permissions after an
identity change, delete the leftover `dataproc-staging-*`/`dataproc-temp-*`
buckets so Dataproc recreates them fresh under the current identity, then
rerun. (The `roles/dataproc.worker` role already carries the full
`storage.buckets.get` + `storage.objects.*` set a custom VM identity needs.)

## Build Tag Isolation

All E2E test files use `//go:build e2e`. This means:

- `go test ./...` and `make test` never trigger E2E tests
- `go build ./...` never compiles E2E test binaries
- You must pass `-tags=e2e` explicitly to run them

The framework packages under `e2e/framework/` have **no** build tag -- they are
ordinary Go libraries that get compiled normally. Only the test files that
create real infrastructure are gated.

## How Verification Works

The framework does not hardcode resource names. Instead, it parses each test
manifest at runtime to extract the resource name, namespace, and kind, then
builds the appropriate verification dynamically. This means adding a new test
scenario is as simple as dropping a YAML file into the component's
`e2e/scenarios/` folder -- no Go code changes needed.

## Adding a New Test Scenario

1. Create a YAML manifest in `{component}/e2e/scenarios/` with a descriptive
   filename
2. Use a unique `metadata.name` (and unique namespace if the component creates
   one) to avoid collisions with other scenarios
3. Run `make e2e-test-component component={ComponentName}` to verify it works
4. That's it -- the framework discovers and runs it automatically

## Adding a New Component

1. Create the IaC modules (`iac/pulumi/`, `iac/tf/`)
2. Create `e2e/profile.yaml` with the component's E2E profile
3. Create at least `e2e/scenarios/minimal.yaml` with a minimal test manifest
4. If the component needs other resources installed first, declare them as
   `prerequisites` on the kind in `cloud_resource_kind.proto` (the harness
   installs them automatically -- see "Component Dependencies").
5. Add a `Test{ComponentName}_{Provisioner}` function in the appropriate test
   file (e.g., `kubernetes_test.go`), and -- if the component name does not
   PascalCase trivially -- a `toPascalCase` entry in
   `pkg/e2e/profile/discover.go` so the CI matrix regex matches it
6. The CI workflow picks up the new component automatically from the profile

> **A kind with scenarios, a verifier, and a `pending_proof` profile is still unrunnable without step 5.** Shipping `e2e/scenarios/`, a registered verifier, and an import map is not enough: `go test -run 'Test{Kind}_...'` matches zero tests until the two `Test{Kind}_{Pulumi,Terraform}` wrappers exist in the provider's `e2e/{provider}/{provider}_test.go`. First caught on a kind whose only live exercise for weeks was as a chained VIP fixture; caught again on a wave-closing kind that shipped the full offline bar (scenarios, verifier, profile) and omitted only the two wrappers -- `discover` listed it `pending_proof` while no `-run` filter could ever start a lane. A BUILD.bazel `srcs` miss on a new verifier file is the sibling class (gazelle-managed -- run `make gazelle`, never hand-edit). The cheap structural fix is an offline gate that diffs each provider's tier-1 profiles against its test-entry list the same way `TestCatalogFixtureIntegrity` guards prerequisite chains; that spans every provider harness, so it is a framework proposal, not a mid-proof edit. Until it exists, the authoring checklist's step 5 is load-bearing: grep the test file for the kind name before enqueueing.
>
> A prerequisite-only kind still needs steps 3 and 5 to be provable. A kind consumed by other kinds' chains gets deployed and verified as a FIXTURE (its `prerequisite.yaml` + registered verifier), which makes it easy to believe it is covered -- but fixture duty proves another kind's lane, not this one's. Scenario discovery reads ONLY `e2e/scenarios/` (`DiscoverTestScenarios` returns nil when the directory is absent, and the runner then skips), and the canonical `e2e/manifest.yaml` is a documented example and offline-plan fixture that never runs live.

## Adding a New Provider

1. Create `catalog/{provider}/aa_e2e/` with harness, verify
   files, and `profile.yaml`
2. Implement the `provider.Harness` interface (Setup, Teardown, VerifyDeployed,
   VerifyDestroyed)
3. Add a test entry point in `e2e/` that creates the harness and discovers
   scenarios for that provider
4. Add Makefile targets
5. Create `.github/workflows/e2e-{provider}.yaml` with the appropriate trigger
   schedule and credential configuration

### Real-cloud harness Setup: validate credentials and export preconditions

For a real-cloud provider (`test_substrate: real_cloud`), the framework builds
every stack input with a **nil provider config**, so the IaC modules resolve
credentials from the SDK's ambient chain (a keyless CLI/SSO login locally, OIDC
federation in CI) rather than from a stored secret. The harness `Setup` therefore
owns two responsibilities beyond wiring verifiers:

- **Fail fast on credentials.** Probe the ambient chain with a zero-permission,
  side-effect-free identity call (e.g. AWS `sts:GetCallerIdentity`; Azure: acquire
  a management token) so a missing/expired login is reported before any deploy.
- **Export the environment preconditions both engines need.** The Terraform path's
  provider block is empty and the Pulumi builders leave unset args to env-var
  fallbacks, so any value the provider cannot infer must be exported into the
  process environment (the Pulumi runner inherits `os.Environ()`; the Terraform
  path layers extracted vars on top of it). Two categories recur:
  - *Identity/scope the provider cannot infer* — e.g. Azure `azurerm` v4 no longer
    infers the subscription, so the harness must export `ARM_SUBSCRIPTION_ID`.
  - *Test-environment behavior that differs from production defaults* — e.g. the
    Azure providers auto-register a broad set of resource providers at init and
    fire the registrations concurrently, which returns HTTP 409 on a subscription
    whose providers are not yet registered. Resource-provider registration is a
    one-time subscription bootstrap, orthogonal to whether a module creates its
    resource correctly (the contract E2E validates), so the harness opts out for
    the ephemeral run via `ARM_SKIP_PROVIDER_REGISTRATION=true`. Keep such opt-outs
    scoped to the harness and documented, so the test stays honest about what it
    proves.
  - *Because of that opt-out, a kind whose ARM namespace has never been used on
    the subscription fails its FIRST live run with a 409
    `MissingSubscriptionRegistration`* (e.g. `Microsoft.App` before the first
    Container Apps run). The fix is the same one-time subscription bootstrap the
    opt-out defers: `az provider register --namespace <ns>` (registration takes
    a minute or two; poll `az provider show -n <ns>` until `Registered`), then
    re-run. Expect this once per new resource-provider family, never per kind.

### Authoring verifiers and prerequisite fixtures

- **Never call `ctx.Export` inside an `ApplyT` callback in a Pulumi module.**
  Apply callbacks run on output-resolution goroutines, and the exports map
  they write is the same map the SDK's end-of-program stack-output
  marshaling reads — a data race that kills the whole program with
  `fatal error: concurrent map read and map write`, timing-dependent and
  therefore FLAKY (first hit: the subnetwork module's per-index
  secondary-range exports crashed one chain deploy and passed the next,
  identical code). A module that needs per-element export keys derives each
  key's index space from the spec (known before the program returns) and
  registers every export synchronously with a value DERIVED via `ApplyT`
  (`ctx.Export(key, out.ApplyT(...))` is safe — it is the export
  registration itself that must stay on the program goroutine). When a
  fixture deploy dies this way mid-chain, the created cloud resource
  usually IS recorded in the run-scoped stack state, so the next scenario's
  `up --refresh` adopts it — but the failed scenario's own teardown can
  strand it against the chain's parent (a VPC refusing deletion while the
  crashed subnet exists) until a later scenario's teardown sweeps both.
- **Never name a verifier (or any production `.go` file) with a `_test.go`
  suffix.** Go treats every file ending in `_test.go` as a test file and
  excludes it from the normal package build, so a verifier in, say,
  `foo_web_test.go` compiles fine under `go test` but is INVISIBLE to
  `go build` — the registration in `verifier.go` then fails with a
  bewildering "undefined: fooVerifier" even though the type is plainly
  defined. Name such files to avoid the suffix (e.g.
  `application_insights_standard_webtest.go`, not `..._web_test.go`).
- **Every kind that appears in another kind's `kind_meta.prerequisites` needs its
  own verifier registered in the provider harness** — the dependency deployer
  verifies each fixture right after installing it, so a missing verifier fails the
  composed scenario at DEPENDENCIES-UP with "no verifier registered", not at the
  component under test. Wiring a new composed component therefore means wiring
  verifiers for its whole prerequisite chain (plus a `prerequisite.yaml` or minimal
  scenario for each prerequisite).
- **Prefer a GET-by-ID existence probe over a HEAD unless the service is known to
  support HEAD.** ARM's generic `CheckExistenceByID` (HEAD) is not implemented by
  every resource provider — e.g. `Microsoft.ManagedIdentity` answers HEAD with
  405 Method Not Allowed while GET works fine. A GET with the typed 404 as the
  absence signal works everywhere; treat every non-404 failure as a real error so
  auth/network problems never masquerade as "absent".
- **Data-plane resources need a data-plane grant AND sometimes a data-plane
  verifier.** Some resources live behind a service's own endpoint rather than the
  control plane (e.g. Azure Key Vault keys/certificates behind
  `{vault}.vault.azure.net`): the deploying credential needs an explicit
  data-plane authorization even when it owns the subscription. Grant it once at
  the subscription scope as an idempotent harness-Setup bootstrap rather than
  per-ephemeral-resource — data-plane RBAC takes minutes to propagate, so a
  per-run grant makes every scenario race its own authorization. For
  verification, check whether the control plane exposes a read proxy first
  (Azure vault KEYS have one; CERTIFICATES do not) and fall back to the service's
  data-plane SDK only when it does not.
- **Some services access customer resources with the PROVIDER'S OWN service
  principal, which needs its own bootstrap.** E.g. Azure Front Door reads
  customer Key Vaults for bring-your-own TLS certificates as the
  `Microsoft.AzureFrontDoor-Cdn` enterprise application (a well-known,
  tenant-invariant app id) — NOT as the deploying credential. Two one-time
  steps, both idempotent harness-Setup bootstraps in the same class as the
  test-principal grant above: (1) the principal may not exist in a fresh
  tenant until instantiated (`az ad sp create --id <well-known-app-id>`;
  resolve with `az ad sp show` first — create fails when it already exists),
  and (2) it needs the read role granted at the subscription scope
  (`--assignee-principal-type ServicePrincipal`). Without both, the dependent
  resource's create fails with an access-denied error naming the vault, which
  looks like a module defect but is tenant bootstrap.
- **Azure auto-creates `NetworkWatcherRG` the first time a virtual network
  exists in a region** -- it appears mid-run without any manifest creating it
  and survives every teardown (it is subscription furniture, not a test
  orphan). The zero-orphan sweep should recognize it, and deleting it at
  session end is safe and keeps the zero-resource-group baseline exact:
  Azure recreates it on demand the next time a VNet appears.
- **Soft-delete/retention services add an orphan class the resource list does not
  show.** A destroyed Azure Key Vault lingers soft-deleted (its globally unique
  name stays reserved) unless purged; both IaC engines purge on destroy by
  default, but an interrupted run can strand one. The zero-orphan sweep for such
  services must check the recycle bin too (`az keyvault list-deleted`), and
  scenarios should keep purge protection OFF so teardown can actually purge.
  Expect destroys to be slow (a vault purge runs ~10 minutes) and size test
  timeouts accordingly.
- **ML workspaces are the same soft-delete class, with two extra twists.**
  Destroy is a soft delete (the name stays reserved) unless the provider
  features flag `machine_learning.purge_soft_deleted_workspace_on_destroy`
  is on -- the provider default is OFF (unlike Key Vault, which defaults
  ON). Both Planton workspace modules enable the flag (Terraform in the
  kind's `provider.tf`; Pulumi via `pulumiazureprovider.GetWithFeatures`).
  There is no CLI or REST listing of ML ghosts (`az ml workspace list`
  has no `--archived` / soft-delete flag; Resource Graph indexes active
  resources only). The portal's Azure Machine Learning "Recently deleted"
  view is the one listing surface. Dual-engine lanes with a fixed name
  prove the purge: the second engine recreates the same name after the
  first engine's destroy. An interrupted run can still strand a ghost;
  purge it with `az ml workspace delete --name … --resource-group … --yes --permanently-delete`.
  First `az ml` on a machine without the `ml` extension hangs forever on
  a dynamic-install prompt no non-interactive shell can answer -- install
  it explicitly (`az extension add --name ml`) before any sweep.
  The class covers the whole workspace FAMILY: AI Foundry hubs and
  projects are ML workspaces at ARM (kind "Hub" / "Project") and ghost
  the same way. The hub resource honors the purge features flag (the
  hub module enables it, dual-engine fixed-name recreate proven); the
  PROJECT resource has NO purge seam -- its delete never consults the
  flag, so setting it in a project module is a silent no-op. Measured
  live: a project ghost does not block recreating the same name (the
  hub's purge sweeps its children's ghosts); a stranded standalone
  project ghost, should one ever surface, is a portal-only purge.
- **Recovery Services protected-item deletes are asynchronous BEYOND the
  engine's poller, and can silently never land.** Measured live (session 055,
  VM protection): after a `pulumi destroy` the provider reported successful,
  ARM reads kept answering 200 on the item for minutes (a smoke item has ZERO
  recovery points, so a landed delete removes it outright -- the 14-day
  soft-delete ghost class only applies to items WITH recovery points). One
  run went further: the provider's DeleteThenPoll returned success while the
  vault ran NO DeleteBackupData job at all -- the item survived ACTIVE, still
  policy-attached, indefinitely. A surviving active item then wedges the whole
  teardown chain: the policy delete fails `BMSUserErrorPolicyObjectInUse`, the
  vault delete fails `BMSUserErrorVaultDeletionNotAllowed`, and the resource
  group destroy hangs until the test binary's timeout panics (eating the
  stage summary). The absence bar these verifiers use: a bounded poll (10
  min) where 404 is absent, a 200 with
  `properties.isScheduledForDeferredDelete: true` is ALSO absent (the azurerm
  provider's own ghost bar), and an item still active at the deadline fails
  honestly. Manual recovery for the dropped-delete flake:
  `az backup protection disable --delete-backup-data true --yes` with
  FRIENDLY names (`--container-name <vm-name> --item-name <vm-name>` -- the
  full semicolon container ID fails `BMSUserErrorContainerNameIncorrectFormat`
  from the CLI) lands in seconds, then vault and RG delete normally. The
  orphan sweep owns the recycle-bin listing (`az backup item list
  --backup-management-type AzureIaaSVM` on any surviving vault); the modules
  deliberately leave the provider's `recover_soft_deleted_backup_protected_vm`
  feature off (silently adopting ghost recovery points is not smoke-lane
  behavior). The FILE-SHARE sibling measured the same read-lag class
  (session 056: VERIFY-CLN saw a 200 seconds after a clean destroy; the item
  cleared before the registration's unregister, which succeeded) -- both
  protected-item verifiers now share the bounded-poll absence bar.
- **A registered storage account's DoNotDelete lock blocks deleting its
  SHARES, and prerequisite ORDER is the fix.** Azure Backup locks a storage
  account while it is registered with a vault (AzureBackupContainerStorageAccount);
  the lock's scope covers children, so deleting ANY file share in the account
  -- protected or not -- fails `ScopeLocked` naming the account. Measured
  live (session 056): the protected-file-share lane's teardown deleted the
  share fixture BEFORE unregistering (destroy reverses deploy order, and the
  registration was listed first in the kind's registry prerequisites) and
  409-looped for 11 minutes. The fix is the prerequisite ORDER on
  `AzureBackupProtectedFileShare` -- the share lists BEFORE the registration,
  so teardown unregisters first (lock released), then deletes the share; the
  reorder is commented in `cloud_resource_kind.proto` as load-bearing. The
  general rule: when a prerequisite kind LOCKS another prerequisite's
  resources while alive (registrations, guards, attachments), the locking
  kind must list LAST among its siblings so it dies first.
- **A workspace managed VNet (approved-outbound + provision-on-creation)
  is a new medium-slow AND fragile class.** Create of the workspace object
  is ~1 min; adding the managed VNet plus a private-endpoint outbound
  rule stretches create to ~20 min and then destroy 409-loops
  (`privateEndpointConnectionProxies/validate` failing, workspace delete
  returning `InternalServerError` / 409) for 40+ minutes without the
  workspace ever leaving `Succeeded`. Even without the PE rule, ARM can
  roll the workspace back mid-create (`Bad request to get identity
  secret: The workspace identity has been deleted`). The smoke lane
  therefore proves the workspace OBJECT on the default network; the
  managed-network arm stays offline-proven. Size timeouts at 90 min if
  you re-open that arm, and do not debug a 409-loop as a module defect.
- **Application Insights leaves an "Application Insights Smart Detection"
  action group in the resource group after the Insights component is
  deleted.** The Azure provider's default
  `prevent_deletion_if_contains_resources = true` then refuses to delete
  the group. Planton's Azure resource-group modules flip that flag off
  so destroy means ARM-delete the group (the same contract as
  `az group delete`). Do not "fix" this by deleting the action group
  from a sweep script and leaving the flag on -- the next Insights
  fixture will plant it again.
- **Some ARM resources have no true delete — destroy flips a state field and the
  object stays GETtable, so verify-absent must be STATE-AWARE, not 404-based.**
  An Azure Storage encryption scope is the canonical case: "delete" PATCHes
  `properties.state` to `Disabled`, a GET keeps answering 200, and the name stays
  reserved inside its parent (recreating the same name re-enables it). A plain
  GET-with-typed-404 probe reports such a resource as still-existing after a
  clean destroy and fails the run spuriously. Check the provider's own Read/Delete
  source for state-flip semantics when writing a verifier: if delete is a
  soft-disable, the verifier must read the state field and treat the disabled
  value as absent (mirroring the provider's own removed-from-state behavior),
  and verify-exists should require the ENABLED state, not mere presence. No
  recycle-bin sweep exists for this class — the object intentionally persists;
  parent teardown is what actually removes it.
- **A sibling of the state-flip class: some association resources have NO ARM
  object at all — their state lives in a property of the resources they
  associate.** An Azure Managed Redis geo-replication group is the canonical
  case: creating it links existing databases (no new ARM object; the resource ID
  is just the managing cluster's ID) and destroying it unlinks them (nothing is
  deleted, so a 404 probe can NEVER pass verify-absent — and verify-exists
  against the resource ID would pass even when the association never took
  effect). The verifier must GET the carrying resource and read the association
  property (here `properties.geoReplication.linkedDatabases` on the managing
  default database): verify-exists requires the association to be genuinely
  established, verify-absent requires it collapsed. Read the provider's Read
  source to find which resource carries the property.
- **Never let sequential scenarios destroy and recreate the same globally unique
  parent name.** When a kind's registry prerequisite chain deploys a fixture
  whose name is globally unique (an Azure SQL logical server, a Key Vault),
  every scenario of a multi-scenario component tears the fixture down and the
  next scenario recreates it — and Azure can hold the just-deleted name long
  enough that the recreate hangs indefinitely (a `Microsoft.Sql/servers` create
  stuck 20+ minutes with no write in the activity log). Give each scenario its
  own uniquely named parent instead: declare the parent through the
  `e2e-prerequisites` annotation with a scenario-local manifest (kept
  OUTSIDE `e2e/scenarios/`, which the discoverer treats as test cases), and
  drop the registry prerequisite if it would force the shared fixture chain in
  anyway. Registry prerequisites are for parents a kind cannot exist without
  AND that are safe to recreate per scenario. The post-delete name hold is
  SERVICE-SPECIFIC: SQL logical servers hold the name after the delete
  returns, while Azure Cache for Redis frees the name the moment its (slow,
  several-minute) delete completes — same-name recreates across sequential
  engine runs verified live. Scenario-local parents are still the right shape
  for very expensive fixtures regardless of name-hold behavior: a Redis cache
  runs 15-40 minutes per creation, and a scenario that owns its parent never
  serializes against another scenario recreating a shared one.
- **The tfvars wire format drops zero-valued proto fields — TF object attributes
  where zero is meaningful must be `optional()` with the zero default.**
  `ProtoToTFVars()` serializes from protojson, which omits scalar zeros
  (proto3 implicit presence). A required attribute like
  `per_database_settings.min_capacity = number` then fails
  `terraform apply` with "attribute is required" whenever the manifest
  legitimately sets `0`. Declare such attributes
  `optional(number, 0)` so the dropped zero round-trips.

## Architecture

```
e2e/
  e2e_test.go             -- TestMain: shared infrastructure lifecycle
  kubernetes_test.go      -- Kubernetes test entry points (per-component)
  framework/
    runner/               -- 6-phase lifecycle engine, Pulumi/Terraform execution
    provider/             -- Harness interface definition
    discovery/            -- Filesystem scanner for components and scenarios
    reporter/             -- JSON + Markdown report generation

pkg/e2e/profile/          -- E2E profile loader and discovery
  loader.go               -- YAML→proto loading for provider and component profiles
  discover.go             -- Profile scanning, filtering, GitHub matrix generation
  paths.go                -- Well-known filesystem paths

qa/      -- Proto schema for E2E profiles (KRM-style)
  shared/                 -- Shared enums (CostClass)
  providere2eprofile/v1/  -- ProviderE2EProfile KRM API
  componente2eprofile/v1/ -- ComponentE2EProfile KRM API
```

The framework is engine-agnostic. The runner supports both Pulumi and Terraform
execution paths. Each component test runs through the same lifecycle regardless
of which engine is used.
