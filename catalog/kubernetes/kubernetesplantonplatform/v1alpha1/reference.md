# KubernetesPlantonPlatform

> Generated from the protobuf schema by `make generate-reference` -- do not
> edit by hand. To change a fact on this page, change the proto field comment
> or validation rule it is derived from, then regenerate.

**apiVersion**: `kubernetes.planton.dev/v1alpha1`

**Guide**: [GUIDE.md](../GUIDE.md) -- authored operational judgment for this component: conventions, trade-offs, and what pairs well with it.

**KubernetesPlantonPlatformSpec** declares a self-hosted Planton
platform — control plane, web console, identity server (Keycloak),
PostgreSQL (CloudNativePG), cache, workflow engine (Temporal), secrets
manager (OpenBAO), and an in-cluster deployment runner — as a
`PlantonPlatform` custom resource that the Planton operator reconciles.

PREREQUISITE: the Planton operator must be installed first (declare a
KubernetesPlantonOperator resource). One operator serves EVERY platform
on the cluster; each platform lives in its own namespace with its own
URL, identity server, and databases.

ZERO-CONFIG BY DESIGN: `version` is the only required choice. A
version-only platform serves console, API, and sign-in on one origin
through a built-in gateway reachable with a single `kubectl
port-forward` (the exact command is this resource's
`port_forward_command` output), and the console's first visitor becomes
the admin using a setup code read from a Secret (the
`setup_code_command` output). Everything else — a real hostname and
TLS, storage classes and sizes, workload identity for the runner —
is opt-in refinement of a working platform.

VERSION IS A DELIBERATE CHOICE, ALWAYS: the field is required and never
defaulted, because a default that moved with catalog updates would
silently upgrade a running platform — databases and all — on an
ordinary re-apply. Upgrades are a one-line edit of this field.

SPEC SURFACE AND OPERATOR VERSION: this spec models the PlantonPlatform
schema as of the catalog release. A newer operator may accept fields
this catalog does not model yet — reach them by updating the catalog,
or through the `planton` umbrella Helm chart, whose values pass the
platform spec through verbatim.

DESTROY BEHAVIOR: deleting the resource deletes the platform — every
workload, Service, Secret, and volume the operator created is
owner-referenced to this declaration and garbage-collected by
Kubernetes (so teardown completes even when the operator itself is
already gone), and the database layer removes its volumes and
credentials together — no orphaned volume holds a password a reinstall
cannot match. Two residues are known: build caches and workflow
volumes can survive in the namespace (deleting the namespace, automatic
when this resource owned it via create_namespace, sweeps them), and the
platform's namespace-qualified token-review ClusterRole/Binding
lingers inert (its subject ServiceAccount is deleted with the
platform) until an operator release adds the janitor. Cluster-shared
sub-operators (CloudNativePG, Tekton) deliberately stay — sibling
platforms may ride them.

## Example

```yaml
# Full-surface hack manifest for the offline plan/preview proofs — every
# typed field exercised with realistic values. DELIBERATELY NOT a live
# scenario: this shape declares a real cluster's fittings (a `gp3`
# StorageClass, an nginx IngressClass, a Let's Encrypt ClusterIssuer, a
# license Secret), none of which exist on the lane's single-node kind
# cluster, so a live run here could only fail for reasons that say nothing
# about the platform. The zero-config `scenarios/minimal.yaml` is the live
# install proof; the full surface earns its live proof on a cluster that
# has these fittings.
apiVersion: kubernetes.planton.dev/v1alpha1
kind: KubernetesPlantonPlatform
metadata:
  name: planton
spec:
  namespace:
    value: planton
  create_namespace: true
  version: v0.0.50
  license:
    secret_key_ref:
      name: planton-license
      key: key
  storage:
    storage_class_name: gp3
    size: 20Gi
  database:
    postgresql:
      replicas: 2
      storage_size: 20Gi
    redis:
      storage_size: 2Gi
  ingress:
    enabled: true
    hostname: planton.example.com
    ingress_class_name: nginx
    annotations:
      nginx.ingress.kubernetes.io/proxy-body-size: 64m
    tls:
      issuer:
        name: letsencrypt
        kind: ClusterIssuer
    reachability: public
  gateway:
    local_port: 8080
  email:
    from:
      address: no-reply@planton.example.com
      name: Planton
    reply_to: it-help@example.com
    smtp:
      host: smtp.office365.com
      port: 587
      security: starttls
      credentials_secret_name: planton-email
      ca_bundle_secret_ref:
        name: corp-ca
        key: ca.crt
  identity:
    realm: planton
    admin_email: admin@example.com
  bootstrap:
    organization:
      slug: acme
      name: Acme Corp
    environment:
      slug: production
      name: Production
    admins:
      - platform-team@example.com
    iac_provisioner: tofu
    secret_backend:
      type: platform
  runner:
    enabled: true
    storage_size: 4Gi
    service_account_annotations:
      eks.amazonaws.com/role-arn: arn:aws:iam::123456789012:role/planton-runner
  build:
    enabled: true
  vault:
    enabled: true
    init_mode: auto
    storage_size: 2Gi
  components:
    authorization:
      enabled: true
    search:
      enabled: true
      mode: standalone
      storage_size: 10Gi
      zookeeper:
        replicas: 1
        storage_size: 5Gi
    graph:
      enabled: true
      storage_size: 10Gi
  prerequisites:
    postgres_operator: auto
    solr_operator: auto
    tekton_pipelines: auto
  control_plane:
    replicas: 1
  console:
    replicas: 1
```

## Spec Fields

| Path | Type | Required | Default | References |
|---|---|---|---|---|
| `spec.namespace` | `string \| valueFrom` | yes |  | KubernetesNamespace (`spec.name`) |
| `spec.createNamespace` | `bool` |  |  |  |
| `spec.version` | `string` | yes |  |  |
| `spec.license` | `KubernetesPlantonPlatformLicense` |  |  |  |
| `spec.license.key` | `string` (sensitive) |  |  |  |
| `spec.license.secretKeyRef` | `KubernetesPlantonPlatformSecretKeyRef` |  |  |  |
| `spec.license.secretKeyRef.name` | `string` | yes |  |  |
| `spec.license.secretKeyRef.key` | `string` | yes |  |  |
| `spec.storage` | `KubernetesPlantonPlatformStorage` |  |  |  |
| `spec.storage.storageClassName` | `string` |  |  |  |
| `spec.storage.size` | `string` |  |  |  |
| `spec.database` | `KubernetesPlantonPlatformDatabase` |  |  |  |
| `spec.database.postgresql` | `KubernetesPlantonPlatformPostgresql` |  |  |  |
| `spec.database.postgresql.replicas` | `int32` |  | `1` |  |
| `spec.database.postgresql.storageSize` | `string` |  |  |  |
| `spec.database.postgresql.storageClassName` | `string` |  |  |  |
| `spec.database.redis` | `KubernetesPlantonPlatformRedis` |  |  |  |
| `spec.database.redis.storageSize` | `string` |  |  |  |
| `spec.database.redis.storageClassName` | `string` |  |  |  |
| `spec.ingress` | `KubernetesPlantonPlatformIngress` |  |  |  |
| `spec.ingress.enabled` | `bool` |  |  |  |
| `spec.ingress.hostname` | `string` |  |  |  |
| `spec.ingress.ingressClassName` | `string` |  |  |  |
| `spec.ingress.annotations` | `map<string, string>` |  |  |  |
| `spec.ingress.tls` | `KubernetesPlantonPlatformIngressTls` |  |  |  |
| `spec.ingress.tls.secretName` | `string` |  |  |  |
| `spec.ingress.tls.issuer` | `KubernetesPlantonPlatformCertManagerIssuer` |  |  |  |
| `spec.ingress.tls.issuer.name` | `string` | yes |  |  |
| `spec.ingress.tls.issuer.kind` | `string` |  | `Issuer` |  |
| `spec.ingress.gatewayRef` | `KubernetesPlantonPlatformGatewayRef` |  |  |  |
| `spec.ingress.gatewayRef.name` | `string \| valueFrom` | yes |  | KubernetesGateway (`status.outputs.gateway_name`) |
| `spec.ingress.gatewayRef.namespace` | `string \| valueFrom` |  |  | KubernetesGateway (`status.outputs.namespace`) |
| `spec.ingress.gatewayRef.sectionName` | `string` |  |  |  |
| `spec.ingress.reachability` | `string` |  | `auto` |  |
| `spec.gateway` | `KubernetesPlantonPlatformGateway` |  |  |  |
| `spec.gateway.localPort` | `int32` |  | `8080` |  |
| `spec.identity` | `KubernetesPlantonPlatformIdentity` |  |  |  |
| `spec.identity.realm` | `string` |  | `planton` |  |
| `spec.identity.adminEmail` | `string` |  |  |  |
| `spec.bootstrap` | `KubernetesPlantonPlatformBootstrap` |  |  |  |
| `spec.bootstrap.organization` | `KubernetesPlantonPlatformBootstrapOrg` |  |  |  |
| `spec.bootstrap.organization.slug` | `string` |  | `default` |  |
| `spec.bootstrap.organization.name` | `string` |  |  |  |
| `spec.bootstrap.environment` | `KubernetesPlantonPlatformBootstrapEnv` |  |  |  |
| `spec.bootstrap.environment.slug` | `string` |  | `default` |  |
| `spec.bootstrap.environment.name` | `string` |  |  |  |
| `spec.bootstrap.admins` | `[]string` |  |  |  |
| `spec.bootstrap.iacProvisioner` | `string` |  | `tofu` |  |
| `spec.bootstrap.secretBackend` | `KubernetesPlantonPlatformSecretBackend` |  |  |  |
| `spec.bootstrap.secretBackend.type` | `string` | yes |  |  |
| `spec.bootstrap.secretBackend.awsSecretsManager` | `KubernetesPlantonPlatformAwsSecretsManager` |  |  |  |
| `spec.bootstrap.secretBackend.awsSecretsManager.region` | `string` | yes |  |  |
| `spec.bootstrap.secretBackend.awsSecretsManager.kmsKeyArn` | `string` | yes |  |  |
| `spec.runner` | `KubernetesPlantonPlatformRunner` |  |  |  |
| `spec.runner.enabled` | `bool` |  | `true` |  |
| `spec.runner.storageSize` | `string` |  |  |  |
| `spec.runner.storageClassName` | `string` |  |  |  |
| `spec.runner.serviceAccountAnnotations` | `map<string, string>` |  |  |  |
| `spec.runner.cloudCredentialsSecretName` | `string` |  |  |  |
| `spec.build` | `KubernetesPlantonPlatformBuild` |  |  |  |
| `spec.build.enabled` | `bool` |  | `true` |  |
| `spec.vault` | `KubernetesPlantonPlatformVault` |  |  |  |
| `spec.vault.enabled` | `bool` |  | `true` |  |
| `spec.vault.initMode` | `string` |  | `auto` |  |
| `spec.vault.storageSize` | `string` |  |  |  |
| `spec.vault.storageClassName` | `string` |  |  |  |
| `spec.components` | `KubernetesPlantonPlatformComponents` |  |  |  |
| `spec.components.authorization` | `KubernetesPlantonPlatformToggle` |  |  |  |
| `spec.components.authorization.enabled` | `bool` |  |  |  |
| `spec.components.search` | `KubernetesPlantonPlatformSearch` |  |  |  |
| `spec.components.search.enabled` | `bool` |  |  |  |
| `spec.components.search.mode` | `string` |  | `standalone` |  |
| `spec.components.search.storageSize` | `string` |  |  |  |
| `spec.components.search.storageClassName` | `string` |  |  |  |
| `spec.components.search.zookeeper` | `KubernetesPlantonPlatformZookeeper` |  |  |  |
| `spec.components.search.zookeeper.replicas` | `int32` |  | `1` |  |
| `spec.components.search.zookeeper.storageSize` | `string` |  |  |  |
| `spec.components.search.zookeeper.storageClassName` | `string` |  |  |  |
| `spec.components.graph` | `KubernetesPlantonPlatformGraph` |  |  |  |
| `spec.components.graph.enabled` | `bool` |  |  |  |
| `spec.components.graph.storageSize` | `string` |  |  |  |
| `spec.components.graph.storageClassName` | `string` |  |  |  |
| `spec.prerequisites` | `KubernetesPlantonPlatformPrerequisites` |  |  |  |
| `spec.prerequisites.postgresOperator` | `string` |  | `auto` |  |
| `spec.prerequisites.solrOperator` | `string` |  | `auto` |  |
| `spec.prerequisites.tektonPipelines` | `string` |  | `auto` |  |
| `spec.controlPlane` | `KubernetesPlantonPlatformControlPlane` |  |  |  |
| `spec.controlPlane.image` | `KubernetesPlantonPlatformImage` |  |  |  |
| `spec.controlPlane.image.repository` | `string` |  |  |  |
| `spec.controlPlane.image.tag` | `string` |  |  |  |
| `spec.controlPlane.replicas` | `int32` |  | `1` |  |
| `spec.controlPlane.externalConfigSecretName` | `string` |  |  |  |
| `spec.controlPlane.serviceAccountAnnotations` | `map<string, string>` |  |  |  |
| `spec.console` | `KubernetesPlantonPlatformConsole` |  |  |  |
| `spec.console.image` | `KubernetesPlantonPlatformImage` |  |  |  |
| `spec.console.image.repository` | `string` |  |  |  |
| `spec.console.image.tag` | `string` |  |  |  |
| `spec.console.replicas` | `int32` |  | `1` |  |
| `spec.console.externalConfigSecretName` | `string` |  |  |  |
| `spec.remoteRunners` | `KubernetesPlantonPlatformRemoteRunners` |  |  |  |
| `spec.remoteRunners.enabled` | `bool` |  | `false` |  |
| `spec.email` | `KubernetesPlantonPlatformEmail` |  |  |  |
| `spec.email.from` | `KubernetesPlantonPlatformEmailFrom` | yes |  |  |
| `spec.email.from.address` | `string` | yes |  |  |
| `spec.email.from.name` | `string` |  | `Planton` |  |
| `spec.email.replyTo` | `string` |  |  |  |
| `spec.email.smtp` | `KubernetesPlantonPlatformEmailSmtp` |  |  |  |
| `spec.email.smtp.host` | `string` | yes |  |  |
| `spec.email.smtp.port` | `int32` |  | `587` |  |
| `spec.email.smtp.security` | `string` |  | `starttls` |  |
| `spec.email.smtp.credentialsSecretName` | `string` |  |  |  |
| `spec.email.smtp.oauth2` | `KubernetesPlantonPlatformEmailSmtpOauth2` |  |  |  |
| `spec.email.smtp.oauth2.user` | `string` | yes |  |  |
| `spec.email.smtp.oauth2.tokenUrl` | `string` | yes |  |  |
| `spec.email.smtp.oauth2.scope` | `string` | yes |  |  |
| `spec.email.smtp.oauth2.clientId` | `string` | yes |  |  |
| `spec.email.smtp.oauth2.clientSecretRef` | `KubernetesPlantonPlatformSecretKeyRef` | yes |  |  |
| `spec.email.smtp.oauth2.clientSecretRef.name` | `string` | yes |  |  |
| `spec.email.smtp.oauth2.clientSecretRef.key` | `string` | yes |  |  |
| `spec.email.smtp.caBundleSecretRef` | `KubernetesPlantonPlatformSecretKeyRef` |  |  |  |
| `spec.email.smtp.caBundleSecretRef.name` | `string` | yes |  |  |
| `spec.email.smtp.caBundleSecretRef.key` | `string` | yes |  |  |
| `spec.email.resend` | `KubernetesPlantonPlatformEmailResend` |  |  |  |
| `spec.email.resend.apiKeySecretRef` | `KubernetesPlantonPlatformSecretKeyRef` | yes |  |  |
| `spec.email.resend.apiKeySecretRef.name` | `string` | yes |  |  |
| `spec.email.resend.apiKeySecretRef.key` | `string` | yes |  |  |

## Field Details

### spec.namespace

`string | valueFrom` · required

The namespace the platform lives in. Accepts a literal namespace name
or a reference to a KubernetesNamespace resource. Every platform
workload, Service, Secret, and volume is created here, named from
this resource's metadata.name.

- references: KubernetesNamespace (`spec.name`)
- rule: {"required":true}
- rule: write as {value: <literal>} or {valueFrom: {kind: KubernetesNamespace, name: <that resource's name>, fieldPath: spec.name}} -- a bare string does not parse

### spec.createNamespace

`bool`

When true, the namespace is created (with the standard Planton
governance labels) before the platform is declared, and deleted with
the resource. When false, the namespace must already exist.

### spec.version

`string` · required

The Planton platform version to deploy (e.g.
"v0.0.45") — pins the control plane, console, and
runner images as one coherent line. REQUIRED, never defaulted:
changing it is how a platform upgrades, and upgrades of a system
holding your data must always be a deliberate act.

- rule: {"string":{"minLen":"1"}}

### spec.license

`KubernetesPlantonPlatformLicense`

License activation. Without one the platform runs the Community
feature set. Set AT MOST one of key or secret_key_ref.

- rule: set at most one of key or secret_key_ref — one license, one delivery form

### spec.license.key

`string` · sensitive

The license key, inline. This is a secret: supply it as a
managed-secret reference, never inline plaintext.

### spec.license.secretKeyRef

`KubernetesPlantonPlatformSecretKeyRef`

Read the license key from an existing Kubernetes Secret in the
platform's namespace instead.

### spec.license.secretKeyRef.name

`string` · required

Secret name (in the platform's namespace).

- rule: {"string":{"minLen":"1"}}

### spec.license.secretKeyRef.key

`string` · required

Key within the Secret holding the value.

- rule: {"string":{"minLen":"1"}}

### spec.storage

`KubernetesPlantonPlatformStorage`

Platform-wide storage defaults: every persistent volume the platform
creates (databases, cache, workflow engine, search, secrets manager,
runner state) uses these unless its component overrides them.
Unset means the cluster's default StorageClass and each component's
built-in size.

### spec.storage.storageClassName

`string`

StorageClass for every platform volume unless a component overrides
it. Unset = the cluster's default class (the operator verifies the
default class can actually provision before deploying).

### spec.storage.size

`string`

Size for every platform volume unless a component overrides it
(e.g. "10Gi"). Useful when the storage backend enforces a minimum
volume size — one value lifts every volume above the floor.

- rule: size must be a Kubernetes quantity like "10Gi" or "800Gi"
- rule: {"ignore":"IGNORE_IF_ZERO_VALUE"}

### spec.database

`KubernetesPlantonPlatformDatabase`

The platform's own data stores (PostgreSQL and the Redis-protocol
cache). Sizing and placement only — the stores themselves are always
deployed; the platform cannot run without them.

### spec.database.postgresql

`KubernetesPlantonPlatformPostgresql`

PostgreSQL (CloudNativePG-managed).

### spec.database.postgresql.replicas

`int32` · optional (explicit presence)

PostgreSQL instances: 1 is a single primary; 2+ grows the SAME
database into a streaming-replication pair with automatic failover —
a LIVE edit, no reinstall.

- default: `1`
- rule: {"int32":{"gte":1}}

### spec.database.postgresql.storageSize

`string`

Volume size per instance (e.g. "10Gi"). Falls back to
spec.storage.size, then the platform default.

- rule: storage_size must be a Kubernetes quantity like "10Gi"
- rule: {"ignore":"IGNORE_IF_ZERO_VALUE"}

### spec.database.postgresql.storageClassName

`string`

StorageClass override for the database volumes.

### spec.database.redis

`KubernetesPlantonPlatformRedis`

The Redis-protocol cache (Valkey).

### spec.database.redis.storageSize

`string`

Volume size (e.g. "1Gi"). Falls back to spec.storage.size, then the
platform default.

- rule: storage_size must be a Kubernetes quantity like "1Gi"
- rule: {"ignore":"IGNORE_IF_ZERO_VALUE"}

### spec.database.redis.storageClassName

`string`

StorageClass override for the cache volume.

### spec.ingress

`KubernetesPlantonPlatformIngress`

Expose the platform at a real URL through the cluster's existing
front door — an Ingress controller, or a Gateway API Gateway
(`gateway_ref`). Off by default — the built-in gateway plus
port-forward is the zero-config door. `hostname` serves your own
domain; `tls` adds HTTPS (bring a certificate Secret or name a
cert-manager issuer).

- rule: tls requires hostname: a certificate cannot be brought or issued for an auto-derived hostname
- rule: gateway_ref and ingress_class_name name two different front doors; set one — gateway_ref attaches to a Gateway API Gateway, ingress_class_name renders an Ingress
- rule: with gateway_ref the Gateway's HTTPS listener owns the certificate: attach to a listener that already serves the hostname, or set tls.issuer to have a certificate issued for the listener to reference
- rule: reachability: public declares an address the internet reaches, but with enabled: false the platform is reached only through kubectl port-forward from the machine running it; set enabled: true, or leave reachability at auto

### spec.ingress.enabled

`bool`

Expose the platform through the cluster's front door. With no
hostname, the operator derives a working URL from the front door's
published address (magic DNS) — the Ingress controller's, or the
Gateway's.

### spec.ingress.hostname

`string`

The platform's hostname (e.g. "planton.example.com"). One hostname
serves console AND API. The identity server bakes this URL into its
realm at first boot — set it before the first sign-in.

### spec.ingress.ingressClassName

`string`

IngressClass to use. Unset = the cluster's default. Names one front
door; never combined with gateway_ref.

### spec.ingress.annotations

`map<string, string>`

Extra annotations on the Ingress (controller-specific tuning —
ALB schemes, proxy budgets). Ignored with gateway_ref: the Gateway
API expresses behavior in typed fields, not annotations.

### spec.ingress.tls

`KubernetesPlantonPlatformIngressTls`

HTTPS. Requires hostname. Exactly one of secret_name (bring your own
certificate) or issuer (cert-manager issues one) — except with
gateway_ref, where the route attaches to whichever listener matches
the hostname and HTTPS is inferred from that listener; only issuer
applies there.

- rule: exactly one of secret_name or issuer must be set — bring a certificate or have cert-manager issue one, never both

### spec.ingress.tls.secretName

`string`

An existing kubernetes.io/tls Secret in the platform's namespace.

### spec.ingress.tls.issuer

`KubernetesPlantonPlatformCertManagerIssuer`

A cert-manager issuer to obtain the certificate from. Requires
cert-manager on the cluster.

### spec.ingress.tls.issuer.name

`string` · required

Issuer name.

- rule: {"string":{"minLen":"1"}}

### spec.ingress.tls.issuer.kind

`string` · optional (explicit presence)

Issuer kind.

- default: `Issuer`
- rule: {"string":{"in":["","Issuer","ClusterIssuer"]}}

### spec.ingress.gatewayRef

`KubernetesPlantonPlatformGatewayRef`

Serve Planton through a Gateway API Gateway the cluster already runs
(Istio, Envoy Gateway, Cilium, a cloud Gateway) instead of an Ingress
controller: the operator attaches an HTTPRoute for the hostname to
that Gateway. The Gateway is the cluster team's object and is never
modified — its listeners decide which hostnames are admitted, which
namespaces may attach routes, and how HTTPS is terminated; the
operator reads those facts and explains any mismatch in the
platform's status. Requires a planton-operator chart that knows this
field (0.9.0 or newer); an older definition refuses the declaration.

### spec.ingress.gatewayRef.name

`string | valueFrom` · required

Name of the Gateway. Defaults to a KubernetesGateway foreign key
(`status.outputs.gateway_name`): `valueFrom` for a Planton-managed
Gateway, `value:` for one created outside Planton.

- references: KubernetesGateway (`status.outputs.gateway_name`)
- rule: {"required":true}
- rule: write as {value: <literal>} or {valueFrom: {kind: KubernetesGateway, name: <that resource's name>, fieldPath: status.outputs.gateway_name}} -- a bare string does not parse

### spec.ingress.gatewayRef.namespace

`string | valueFrom`

Namespace of the Gateway. Defaults to the platform's own namespace when
omitted. A Gateway in another namespace must allow routes from this one
(spec.listeners[].allowedRoutes.namespaces). Defaults to the same
KubernetesGateway foreign key as `name` (`status.outputs.namespace`), so
one resource wires both; a Gateway created outside Planton takes the
literal namespace with `value:`.

- references: KubernetesGateway (`status.outputs.namespace`)
- rule: write as {value: <literal>} or {valueFrom: {kind: KubernetesGateway, name: <that resource's name>, fieldPath: status.outputs.namespace}} -- a bare string does not parse

### spec.ingress.gatewayRef.sectionName

`string`

Pins the route to one named listener of the Gateway. When empty, the
route attaches to every listener whose hostname admits the
platform's hostname.

### spec.ingress.reachability

`string` · optional (explicit presence)

Whether the public internet can reach this front door — the one fact
about the door the operator cannot observe from inside the cluster.
The capabilities that need an inbound path from the internet (keyless
cloud connections, where the cloud fetches the issuer's discovery
document; GitHub webhook delivery) are offered only where the door is
public. `auto` (default) resolves from the door's shape: a hostname
served over HTTPS is public, anything else private. Declare `private`
for an HTTPS door only your network reaches (split DNS, a corporate
CA, an internal load balancer); declare `public` to affirm it. Only
`public` is refused when enabled is false — a port-forward door is
never reached from the internet — while `private` there is simply
true. Requires a planton-operator chart that knows this field (0.11.0
or newer); an older definition refuses the declaration.

- default: `auto`
- rule: {"string":{"in":["","auto","public","private"]}}

### spec.gateway

`KubernetesPlantonPlatformGateway`

The built-in front-door gateway: console, API, and sign-in on one
origin over a single `kubectl port-forward`. Always deployed; the
local_port here is baked into the identity server's issuer and the
console's callbacks at first boot, so pick it before the first visit.

### spec.gateway.localPort

`int32` · optional (explicit presence)

The local port the port-forward door advertises
(`kubectl port-forward ... {local_port}:80`). Baked into the identity
server's issuer and the console's callbacks at first boot — pick it
before the first visit; two port-forward platforms on one machine
need distinct ports.

- default: `8080`
- rule: {"int32":{"lte":65535,"gte":1}}

### spec.identity

`KubernetesPlantonPlatformIdentity`

The platform's identity server (Keycloak, operator-deployed).
Everything defaults: the realm is "planton" and the first console
visitor becomes the admin through the setup-code page. Set
admin_email only to pre-seed a known admin instead.

### spec.identity.realm

`string` · optional (explicit presence)

The Keycloak realm name.

- default: `planton`

### spec.identity.adminEmail

`string`

Pre-seed a known admin account instead of the first-visitor setup
page: the operator creates this user and writes a one-time password
into the platform's admin-user Secret. Unset (the default) leaves
the setup-code flow — the first console visitor becomes the admin.

- rule: admin_email must be an email address like "admin@example.com"
- rule: {"ignore":"IGNORE_IF_ZERO_VALUE"}

### spec.bootstrap

`KubernetesPlantonPlatformBootstrap`

First-boot seeding: the initial organization and environment, extra
admins, the IaC provisioner the in-cluster runner uses, and where the
platform's managed secrets live.

### spec.bootstrap.organization

`KubernetesPlantonPlatformBootstrapOrg`

The initial organization.

### spec.bootstrap.organization.slug

`string` · optional (explicit presence)

Organization slug.

- default: `default`

### spec.bootstrap.organization.name

`string`

Display name. Defaults to the slug.

### spec.bootstrap.environment

`KubernetesPlantonPlatformBootstrapEnv`

The initial environment.

### spec.bootstrap.environment.slug

`string` · optional (explicit presence)

Environment slug.

- default: `default`

### spec.bootstrap.environment.name

`string`

Display name. Defaults to the slug.

### spec.bootstrap.admins

`[]string`

Additional admin emails granted org ownership and platform-operator
rights at boot (the identity.admin_email, when set, is always
included).

### spec.bootstrap.iacProvisioner

`string` · optional (explicit presence)

The IaC provisioner the in-cluster runner deploys with.

- default: `tofu`
- rule: {"string":{"in":["","tofu","terraform"]}}

### spec.bootstrap.secretBackend

`KubernetesPlantonPlatformSecretBackend`

Where the platform's managed secrets live: the bundled secrets
manager ("platform", the default) or a cloud backend.

- rule: awsSecretsManager needs its configuration block: aws_secrets_manager.region and aws_secrets_manager.kms_key_arn are required

### spec.bootstrap.secretBackend.type

`string` · required

Backend type: "platform" (the bundled OpenBAO) or
"awsSecretsManager".

- rule: {"required":true,"string":{"in":["platform","awsSecretsManager"]}}

### spec.bootstrap.secretBackend.awsSecretsManager

`KubernetesPlantonPlatformAwsSecretsManager`

AWS Secrets Manager configuration (required when type is
awsSecretsManager). The control plane reaches AWS through its own
workload identity — see control_plane.service_account_annotations.

### spec.bootstrap.secretBackend.awsSecretsManager.region

`string` · required

AWS region (e.g. "us-east-1").

- rule: {"string":{"minLen":"1"}}

### spec.bootstrap.secretBackend.awsSecretsManager.kmsKeyArn

`string` · required

KMS key ARN encrypting the secrets.

- rule: {"string":{"minLen":"1"}}

### spec.runner

`KubernetesPlantonPlatformRunner`

The in-cluster deployment runner — how this platform deploys real
infrastructure. ON by default; disabling it leaves a platform that
can model but not deploy. Cloud identity comes from workload-identity
annotations OR a customer-owned credentials Secret — the platform
stores no cloud credentials itself.

### spec.runner.enabled

`bool` · optional (explicit presence)

Deploy the runner. Platform default: true. Explicit false leaves a
platform that can model infrastructure but not deploy it.

- default: `true`

### spec.runner.storageSize

`string`

Runner state volume size (e.g. "2Gi"). Falls back to
spec.storage.size, then the platform default.

- rule: storage_size must be a Kubernetes quantity like "2Gi"
- rule: {"ignore":"IGNORE_IF_ZERO_VALUE"}

### spec.runner.storageClassName

`string`

StorageClass override for the runner state volume.

### spec.runner.serviceAccountAnnotations

`map<string, string>`

Workload-identity annotations on the runner's ServiceAccount — the
keyless way the runner reaches your cloud (EKS:
"eks.amazonaws.com/role-arn"; GKE: "iam.gke.io/gcp-service-account";
AKS: "azure.workload.identity/client-id").

### spec.runner.cloudCredentialsSecretName

`string`

Name of a customer-owned Secret (in the platform's namespace) whose
keys are injected into the runner as environment variables — the
static-credentials way the runner reaches your cloud. The platform
stores nothing: rotate by updating YOUR Secret.

### spec.build

`KubernetesPlantonPlatformBuild`

Container-image build pipelines (Tekton). ON by default; follows the
runner off when the runner is disabled. NOTE: Tekton allows exactly
one cluster-wide build-events sink, so builds can feed only ONE
build-enabled platform per cluster — disable this on all but one when
several platforms share a cluster.

### spec.build.enabled

`bool` · optional (explicit presence)

Enable Tekton-backed build pipelines. Platform default: true
(follows the runner off when the runner is disabled). One
build-enabled platform per cluster — Tekton's build-events sink is
cluster-wide.

- default: `true`

### spec.vault

`KubernetesPlantonPlatformVault`

The bundled secrets manager (OpenBAO). ON by default — a
version-only platform stores connection secrets with zero
configuration. Explicit `enabled: false` is the deliberate opt-out
(bring a cloud secret backend through bootstrap.secret_backend
instead).

### spec.vault.enabled

`bool` · optional (explicit presence)

Deploy the bundled secrets manager (OpenBAO). Platform default:
true. Explicit false is the deliberate opt-out — pair it with a
cloud backend in bootstrap.secret_backend or connection secrets have
nowhere to live.

- default: `true`

### spec.vault.initMode

`string` · optional (explicit presence)

Initialization: "auto" (the operator initializes and unseals,
storing the unseal keys in an annotated platform Secret) or "manual"
(you run the init ceremony).

- default: `auto`
- rule: {"string":{"in":["","auto","manual"]}}

### spec.vault.storageSize

`string`

Volume size (e.g. "2Gi"). Falls back to spec.storage.size, then the
platform default.

- rule: storage_size must be a Kubernetes quantity like "2Gi"
- rule: {"ignore":"IGNORE_IF_ZERO_VALUE"}

### spec.vault.storageClassName

`string`

StorageClass override for the secrets-manager volume.

### spec.components

`KubernetesPlantonPlatformComponents`

Opt-in platform components, all off by default: fine-grained
authorization (OpenFGA), search (Solr), and the graph explorer
(Neo4j).

### spec.components.authorization

`KubernetesPlantonPlatformToggle`

Fine-grained authorization (OpenFGA). Off = the platform's
allow-authenticated authorization arm.

### spec.components.authorization.enabled

`bool`

Enable the component.

### spec.components.search

`KubernetesPlantonPlatformSearch`

Search (Solr).

### spec.components.search.enabled

`bool`

Enable search.

### spec.components.search.mode

`string` · optional (explicit presence)

Deployment mode: "standalone" (a single Solr with its ZooKeeper) or
"operator" (SolrCloud via the Solr operator — see
prerequisites.solr_operator).

- default: `standalone`
- rule: {"string":{"in":["","standalone","operator"]}}

### spec.components.search.storageSize

`string`

Volume size (e.g. "10Gi"). Falls back to spec.storage.size, then the
platform default.

- rule: storage_size must be a Kubernetes quantity like "10Gi"
- rule: {"ignore":"IGNORE_IF_ZERO_VALUE"}

### spec.components.search.storageClassName

`string`

StorageClass override for the search volumes.

### spec.components.search.zookeeper

`KubernetesPlantonPlatformZookeeper`

ZooKeeper for standalone-mode Solr.

### spec.components.search.zookeeper.replicas

`int32` · optional (explicit presence)

ZooKeeper replicas.

- default: `1`
- rule: {"int32":{"gte":1}}

### spec.components.search.zookeeper.storageSize

`string`

Volume size (e.g. "5Gi"). Falls back to spec.storage.size, then the
platform default.

- rule: storage_size must be a Kubernetes quantity like "5Gi"
- rule: {"ignore":"IGNORE_IF_ZERO_VALUE"}

### spec.components.search.zookeeper.storageClassName

`string`

StorageClass override for the ZooKeeper volumes.

### spec.components.graph

`KubernetesPlantonPlatformGraph`

The graph explorer (Neo4j).

### spec.components.graph.enabled

`bool`

Enable the graph explorer.

### spec.components.graph.storageSize

`string`

Volume size (e.g. "10Gi"). Falls back to spec.storage.size, then the
platform default.

- rule: storage_size must be a Kubernetes quantity like "10Gi"
- rule: {"ignore":"IGNORE_IF_ZERO_VALUE"}

### spec.components.graph.storageClassName

`string`

StorageClass override for the graph volume.

### spec.prerequisites

`KubernetesPlantonPlatformPrerequisites`

Cluster-shared sub-operators the platform rides (CloudNativePG,
Tekton Pipelines, the Solr operator). "auto" (the default) installs
each one only when no installation exists on the cluster; "skip"
declares that something else manages it.

### spec.prerequisites.postgresOperator

`string` · optional (explicit presence)

CloudNativePG: "auto" installs it only when absent; "skip" declares
it externally managed (a helm/GitOps CloudNativePG is respected
automatically either way).

- default: `auto`
- rule: {"string":{"in":["","auto","skip"]}}

### spec.prerequisites.solrOperator

`string` · optional (explicit presence)

The Solr operator (only relevant when components.search.mode is
"operator").

- default: `auto`
- rule: {"string":{"in":["","auto","skip"]}}

### spec.prerequisites.tektonPipelines

`string` · optional (explicit presence)

Tekton Pipelines (build pipelines). The build-events sink is wired
even on "skip".

- default: `auto`
- rule: {"string":{"in":["","auto","skip"]}}

### spec.controlPlane

`KubernetesPlantonPlatformControlPlane`

The platform's control-plane deployment (the API monolith). Sizing,
image mirrors, extra environment through a Secret, and the
platform's OWN cloud identity (for cloud secret backends and KMS) —
distinct from the runner's deploy-time identity.

### spec.controlPlane.image

`KubernetesPlantonPlatformImage`

Image override (air-gapped mirrors of the SAME version).

### spec.controlPlane.image.repository

`string`

Full image repository (e.g.
"my-mirror.example.com/planton/control-plane").

### spec.controlPlane.image.tag

`string`

Image tag. Empty = spec.version.

### spec.controlPlane.replicas

`int32` · optional (explicit presence)

Control-plane replicas.

- default: `1`
- rule: {"int32":{"gte":1}}

### spec.controlPlane.externalConfigSecretName

`string`

Name of a Secret (in the platform's namespace) whose keys are all
injected into the control plane as environment variables — the
escape hatch for configuration the spec does not model.

### spec.controlPlane.serviceAccountAnnotations

`map<string, string>`

Workload-identity annotations on the control plane's ServiceAccount
— the platform's OWN cloud identity (cloud secret backends, KMS).
Distinct from runner.service_account_annotations, which is the
DEPLOY-TIME identity.

### spec.console

`KubernetesPlantonPlatformConsole`

The web console deployment. Sizing, image mirrors, and extra
environment through a Secret.

### spec.console.image

`KubernetesPlantonPlatformImage`

Image override (air-gapped mirrors of the SAME version).

### spec.console.image.repository

`string`

Full image repository (e.g.
"my-mirror.example.com/planton/control-plane").

### spec.console.image.tag

`string`

Image tag. Empty = spec.version.

### spec.console.replicas

`int32` · optional (explicit presence)

Console replicas.

- default: `1`
- rule: {"int32":{"gte":1}}

### spec.console.externalConfigSecretName

`string`

Name of a Secret (in the platform's namespace) whose keys are all
injected into the console as environment variables.

### spec.remoteRunners

`KubernetesPlantonPlatformRemoteRunners`

Runners outside this cluster — a developer's laptop deploying with the
cloud sign-in already on it, an appliance in another network — pulling
this platform's deploy work. OFF by default. Rides the front door:
the deploy queue is routed through the platform hostname beside the
native gRPC API, so it needs a Gateway API front door (ingress with a
gateway_ref); on any other door the capability stays closed and the
platform's status says why. The in-cluster runner is unaffected.

### spec.remoteRunners.enabled

`bool` · optional (explicit presence)

Open the deploy queue to runners outside the cluster and advertise the
front door's address to them. Platform default: false — an install that
has not chosen this keeps its queue in-cluster, and a runner asking to
enroll from outside is refused with the reason, never handed an address
it cannot reach. What opens: the queue's workflow service, over TLS,
without authentication of its own (the posture the hosted platform
carries for its remote runners); the queue's administrative service
never leaves the cluster.

- default: `false`

### spec.email

`KubernetesPlantonPlatformEmail`

The one mail provider every sender on the install uses: the control
plane (invitations, alerts, license mail) and the identity server
(password resets) both send through it, so one declaration is the
whole configuration. Absent, the install sends no email: invitations
are shared as links and the sign-in page offers no "Forgot password?".
Exactly one provider arm is set — an SMTP relay or a Resend account.
Credentials are never inline: they are Secrets in the platform's
namespace, named here, and reach the control plane as mounted files so
a rotated password is live on the next send. The operator delivers the
declaration and preflights every Secret it names; the control plane
checks the relay on demand from the console's Email settings and
reports each failure in the relay's own words. Requires a
planton-operator chart that knows this field (0.14.1 or newer); an
older definition refuses the declaration.

- rule: email declares exactly one provider: set spec.email.smtp for a relay or spec.email.resend for a Resend account, never both, never neither

### spec.email.from

`KubernetesPlantonPlatformEmailFrom` · required

The identity every email carries: the address the install sends as
and the display name beside it. The relay must permit sending as this
address (a mailbox's own address, or one it has Send As rights to);
SPF and DKIM for the domain are the domain owner's job.

- rule: {"required":true}

### spec.email.from.address

`string` · required

The address the install sends as, e.g. no-reply@planton.acme.com.
Required whenever email is declared: there is no default address,
because a default would name somebody else's domain.

- rule: {"string":{"minLen":"1"}}

### spec.email.from.name

`string` · optional (explicit presence)

The name shown beside the address in mail clients. Platform default:
Planton.

- default: `Planton`

### spec.email.replyTo

`string`

Where a person's reply lands — a help desk or a shared mailbox — when
the sending address is a no-reply one. Empty, replies go to
from.address.

### spec.email.smtp

`KubernetesPlantonPlatformEmailSmtp`

Send through any SMTP relay: a workplace mail system (Exchange Online,
Google Workspace, an internal smart host) or a transactional vendor's
SMTP endpoint (SES, SendGrid, Postmark, Mailgun, Resend).

- rule: smtp authenticates one way: set credentials_secret_name for a username and password, or oauth2 for a token, not both
- rule: security: none would send credentials in the clear; keep security at starttls or tls, or drop credentials_secret_name and oauth2 for a relay that admits this cluster's address without them

### spec.email.smtp.host

`string` · required

Host of the relay, e.g. smtp.office365.com or
smtp-relay.corp.acme.com.

- rule: {"string":{"minLen":"1"}}

### spec.email.smtp.port

`int32` · optional (explicit presence)

Port the relay listens on. 587 is the submission port most relays use
with STARTTLS; implicit-TLS relays (security: tls) usually listen on
465; an internal plaintext relay on 25. Platform default: 587.

- default: `587`
- rule: {"int32":{"lte":65535,"gte":1}}

### spec.email.smtp.security

`string` · optional (explicit presence)

How the connection to the relay is protected: starttls (connect in the
clear and REQUIRE the upgrade before anything is sent — a relay that
does not offer it is a failed connection, never a silent fallback; the
default), tls (implicit TLS from the first byte), or none (plaintext
end to end, for credential-free internal relays only; credentials are
refused on it). Platform default: starttls.

- default: `starttls`
- rule: {"string":{"in":["","starttls","tls","none"]}}

### spec.email.smtp.credentialsSecretName

`string`

Name of a kubernetes.io/basic-auth Secret in the platform's namespace
whose username and password keys sign in to the relay:

  kubectl -n <namespace> create secret generic planton-email \
    --type=kubernetes.io/basic-auth \
    --from-literal=username=... --from-literal=password=...

Omit it for a relay that admits this cluster by network address. The
values reach the control plane as mounted files, so a rotated password
is live on the next send with no restart.

### spec.email.smtp.oauth2

`KubernetesPlantonPlatformEmailSmtpOauth2`

Sign in with a token from an OAuth2 client-credentials grant (SASL
XOAUTH2) instead of a password. Exchange Online: token_url
https://login.microsoftonline.com/<tenant>/oauth2/v2.0/token, scope
https://outlook.office365.com/.default, the app registration's client
id and secret, and user = the mailbox the app may send as.

### spec.email.smtp.oauth2.user

`string` · required

The mailbox the token sends as — the account the app registration has
been permitted to use.

- rule: {"string":{"minLen":"1"}}

### spec.email.smtp.oauth2.tokenUrl

`string` · required

The provider's OAuth2 token endpoint. Must be an https:// URL.

- rule: token_url must be an https:// URL
- rule: {"string":{"minLen":"1"}}

### spec.email.smtp.oauth2.scope

`string` · required

The scope requested for the token.

- rule: {"string":{"minLen":"1"}}

### spec.email.smtp.oauth2.clientId

`string` · required

The app registration's client id.

- rule: {"string":{"minLen":"1"}}

### spec.email.smtp.oauth2.clientSecretRef

`KubernetesPlantonPlatformSecretKeyRef` · required

The app registration's client secret, by reference: the secret is
never inline.

- rule: {"required":true}

### spec.email.smtp.oauth2.clientSecretRef.name

`string` · required

Secret name (in the platform's namespace).

- rule: {"string":{"minLen":"1"}}

### spec.email.smtp.oauth2.clientSecretRef.key

`string` · required

Key within the Secret holding the value.

- rule: {"string":{"minLen":"1"}}

### spec.email.smtp.caBundleSecretRef

`KubernetesPlantonPlatformSecretKeyRef`

A PEM CA bundle for verifying the relay's TLS certificate — the
private-CA case, the classic enterprise blocker. Omit it when the
relay's certificate chains to a public root.

### spec.email.smtp.caBundleSecretRef.name

`string` · required

Secret name (in the platform's namespace).

- rule: {"string":{"minLen":"1"}}

### spec.email.smtp.caBundleSecretRef.key

`string` · required

Key within the Secret holding the value.

- rule: {"string":{"minLen":"1"}}

### spec.email.resend

`KubernetesPlantonPlatformEmailResend`

Send through Resend's API with an API key.

### spec.email.resend.apiKeySecretRef

`KubernetesPlantonPlatformSecretKeyRef` · required

The Resend API key, by reference: the key is never inline. Reaches the
control plane as a mounted file, so a rotated key is live on the next
send with no restart.

- rule: {"required":true}

### spec.email.resend.apiKeySecretRef.name

`string` · required

Secret name (in the platform's namespace).

- rule: {"string":{"minLen":"1"}}

### spec.email.resend.apiKeySecretRef.key

`string` · required

Key within the Secret holding the value.

- rule: {"string":{"minLen":"1"}}

## Outputs

Reference an output from another manifest as `valueFrom: {kind: KubernetesPlantonPlatform, name: <resource-name>, fieldPath: status.outputs.<output>}`.

| Output | Type | Description |
|---|---|---|
| `status.outputs.namespace` | `string` | Namespace the platform lives in. |
| `status.outputs.platform_name` | `string` | The platform name (the PlantonPlatform CR name — the prefix of every object the operator creates for this platform). |
| `status.outputs.gateway_service` | `string` | The built-in front-door gateway Service ("{platform_name}-gateway") — console, API, and sign-in on one origin. |
| `status.outputs.setup_code_secret` | `string` | The Secret holding the first-run setup code ("{platform_name}-identity-setup-code") — the console's setup page asks for this code when the first visitor becomes the admin. |
| `status.outputs.port_forward_command` | `string` | The exact command that opens the platform's door on this machine. |
| `status.outputs.setup_code_command` | `string` | The exact command that reads the first-run setup code. |

## References

Fields that can point at another resource's outputs:

| Field | Kind | Output |
|---|---|---|
| `spec.namespace` | KubernetesNamespace | `spec.name` |
| `spec.ingress.gatewayRef.name` | KubernetesGateway | `status.outputs.gateway_name` |
| `spec.ingress.gatewayRef.namespace` | KubernetesGateway | `status.outputs.namespace` |

## See Also

- [Overview](../README.md)
