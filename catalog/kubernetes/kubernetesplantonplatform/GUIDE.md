# KubernetesPlantonPlatform Guide

The judgment this guide carries: the platform is zero-config on purpose —
`version` is the ONE decision, and every other field is a refinement of a
platform that already works. Resist the urge to configure upfront; the
two settings that genuinely reward deciding early are the ones sign-in
bakes at first boot.

## Two settings are sticky — decide them before the first sign-in

The identity server bakes the platform's URL into its realm at first
boot. That makes exactly two fields effectively first-boot-sticky:
`ingress.hostname` (when you know the platform will live at a real URL —
through an Ingress controller or a Gateway API Gateway — set it before
anyone signs in) and `gateway.local_port` (two
port-forward platforms on one laptop need distinct ports, chosen before
the first visit). Everything else — storage, replicas, the runner's
cloud identity, the opt-in components — changes cleanly on a running
platform.

## Tell the platform whether the internet reaches its door

`ingress.reachability` is the one fact about the front door the operator
cannot observe from inside the cluster. Everything the platform offers
that needs an inbound path from the internet — keyless cloud connections,
where the cloud fetches the platform's identity documents from the door,
and GitHub webhook delivery — is offered only where the door is public.
`auto` (the default) reads a hostname served over HTTPS as public and
anything else as private, which is right for most installs. Two shapes
need a word from you: an HTTPS address only your network reaches (split
DNS, a corporate CA, an internal load balancer) is `private`, so those
doors stay honestly closed instead of failing at the cloud's first fetch;
and a door whose TLS terminates outside the cluster (an internet-facing
ALB with an ACM certificate, hence no in-cluster `tls` block) is `public`,
or `auto` will read it as private. Changing the declaration is safe on a
running platform; it changes which doors the console offers, never the
platform's address.

## Declare email once, and both senders use it

A platform a team runs day to day needs to reach people: invitations
that land in inboxes, alerts someone reads, a "Forgot password?" that
works on the sign-in page. `email` is that one declaration. The control
plane and the identity server both send through it, from the address you
name, so there is no second place where a relay is configured and no way
for the two to disagree. Without it the platform is fully usable —
invitations are shared as links and the sign-in page offers no password
reset — and the console's Email settings show the exact `email` fragment
and the one `kubectl create secret` command for this install.

Pick one arm. `smtp` reaches every workplace mail system and every
transactional vendor's SMTP endpoint; `resend` uses Resend's API. On a
relay, `security` is a promise the platform keeps: `starttls` requires
the upgrade and fails a relay that will not offer it, `tls` opens TLS from
the first byte, and `none` is plaintext for a credential-free internal
smart host — the platform refuses credentials over it. Sign in one way: a
username and password in a `kubernetes.io/basic-auth` Secret, an OAuth2
app registration (Exchange Online after Microsoft's password retirement),
or no credential. Every credential is a Secret name or a Secret key
reference, never a value; the operator preflights each one and, when a
Secret is missing, says so in words while the platform runs as if no
email were declared. Credentials reach the control plane as mounted
files, so rotating a password is a Secret edit that is live on the next
send. The relay must permit sending as `from.address` — SPF and DKIM for
that domain are the domain owner's job — and the one failure only a real
send can reveal (a From the account may not send as) is what the
console's "Send Me a Test Email" is for. The platform never probes the
relay on a timer; the checks run when someone asks.

## Version is the upgrade lever, and it is never automated

`version` is required with no default, deliberately: a module-owned
default would turn a catalog update into a silent whole-platform upgrade
on the next apply. Day-2 for this kind IS editing `version` — the
operator rolls the platform to the new line. Pin it like you would pin a
database engine version.

## One operator, many platforms — and the two shared facts

Platforms are namespace-isolated (own URL, own identity server, own
databases) and one operator serves them all. Two cluster-level facts are
honest limits, not bugs: Tekton allows ONE cluster-wide build-events
sink, so keep `build.enabled` on for at most one platform per cluster;
and the cluster has one PlantonPlatform CRD schema (the operator's
version), while each platform still pins its own `spec.version`.

## The runner is where cloud credentials DON'T live

`runner.service_account_annotations` (workload identity — IRSA, GKE
Workload Identity, AKS) is the right answer wherever the cluster
supports it; `runner.cloud_credentials_secret_name` (a Secret YOU own in
the platform's namespace) is the static-keys fallback. Either way the
platform stores nothing — rotation is your annotation or your Secret,
never a platform record.

## Storage: one global dial, honest failures

`storage.size` + `storage.storage_class_name` lift EVERY platform volume
at once — built for backends with minimum-size floors (some NAS backends
refuse volumes under hundreds of Gi; one `size: 800Gi` satisfies them
all). The operator preflights that the chosen class can actually
provision and, when a volume sticks, the CR's per-component status names
the exact problem and fix — read `kubectl get plantonplatforms` status
before reading pod logs.

## Destroy and the reinstall truth

Teardown is Kubernetes garbage collection: every operator-created object
is owner-referenced to the declaration, so deleting the platform
completes with or without the operator running, and database credentials
and volumes die together — no orphaned volume can hold a password a
reinstall cannot match. Two residues to know: build caches and workflow
volumes may survive in the namespace, so a reinstall into the SAME
namespace should be preceded by deleting it (automatic when this
resource owned the namespace via `create_namespace`); and the platform's
namespace-qualified token-review ClusterRole/Binding lingers inert (its
subject ServiceAccount died with the platform) until an operator release
adds the janitor.

## On the diagram

The platform draws an explicit `depends_on` edge TO its
KubernetesPlantonOperator — no spec field consumes an operator output
(the coupling is the cluster-global CRD contract), so composed charts
declare the edge in metadata. The `planton-on-kubernetes` infra-chart
carries namespace + operator + platform as one deployable arm; its
multi-platform variant is one operator plus N namespace+platform pairs.
