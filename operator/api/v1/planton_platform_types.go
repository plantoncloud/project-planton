/*
Copyright 2026.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package v1

import (
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// PlantonPhase represents the high-level lifecycle phase of a PlantonPlatform deployment.
// +kubebuilder:validation:Enum=Pending;Deploying;Ready;Error;Upgrading
type PlantonPhase string

const (
	PhasePending   PlantonPhase = "Pending"
	PhaseDeploying PlantonPhase = "Deploying"
	PhaseReady     PlantonPhase = "Ready"
	PhaseError     PlantonPhase = "Error"
	PhaseUpgrading PlantonPhase = "Upgrading"
)

// ComponentPhase represents the lifecycle phase of an individual component.
// +kubebuilder:validation:Enum=Pending;Deploying;Ready;Error
type ComponentPhase string

const (
	ComponentPhasePending   ComponentPhase = "Pending"
	ComponentPhaseDeploying ComponentPhase = "Deploying"
	ComponentPhaseReady     ComponentPhase = "Ready"
	ComponentPhaseError     ComponentPhase = "Error"
)

// Condition types for PlantonPlatform.
const (
	// ConditionReady is True when all enabled components are in Ready phase.
	ConditionReady = "Ready"

	// ConditionVersionSupported is True when spec.version names a platform
	// release this operator runs. False stops reconciliation before any object
	// is created: the message says which release is the oldest this operator
	// supports and how to move (the version, or an operator built for it).
	ConditionVersionSupported = "VersionSupported"
)

// License delivery modes reported in status.license -- how the key reaches
// the control plane, never whether it verified (live license state is the
// control plane's own entitlements advertisement).
const (
	LicenseModeCommunity = "Community"
	LicenseModeInlineKey = "InlineKey"
	LicenseModeSecretRef = "SecretRef"
)

// Email delivery modes reported in status.email -- which provider arm
// spec.email declares, never whether the relay accepts mail (the control
// plane checks that on demand and reports each verdict in words).
const (
	EmailModeNotConfigured = "NotConfigured"
	EmailModeSMTP          = "SMTP"
	EmailModeResend        = "Resend"
)

// OpenBAOInitMode controls how OpenBAO is initialized after deployment.
// "auto" (default): the operator initializes and unseals OpenBAO automatically,
// storing unseal keys and root token in a Kubernetes Secret.
// "manual": the operator deploys the chart but leaves initialization to the user.
// +kubebuilder:validation:Enum=auto;manual
type OpenBAOInitMode string

const (
	OpenBAOInitModeAuto   OpenBAOInitMode = "auto"
	OpenBAOInitModeManual OpenBAOInitMode = "manual"
)

// IngressReachability is the one fact about the front door the operator
// cannot observe from inside the cluster: whether the public internet can
// reach it. Everything the platform offers that needs an inbound path from
// the internet -- keyless cloud connections (the cloud fetches the issuer's
// discovery document), GitHub webhook delivery -- derives from this
// declaration, so a wrong "public" fails at the cloud's first fetch and a
// wrong "private" hides doors that would have worked.
//
// "auto" (default): resolved from the door's shape -- a hostname served over
// HTTPS is treated as public, anything else (a plain-HTTP door, a
// port-forward) as private. The shape of every real install; the right
// answer for a hand-written manifest.
// "public": the explicit affirmation the install journey writes after asking
// the person, prefilled from the door's shape.
// "private": the honest word for an HTTPS door only a network can reach --
// split DNS, a corporate CA, an internal load balancer.
// +kubebuilder:validation:Enum=auto;public;private
type IngressReachability string

const (
	IngressReachabilityAuto    IngressReachability = "auto"
	IngressReachabilityPublic  IngressReachability = "public"
	IngressReachabilityPrivate IngressReachability = "private"
)

// ImageSpec allows overriding the container image for a component.
// When not specified, the operator uses its built-in default image repository
// and derives the tag from spec.version.
type ImageSpec struct {
	// repository is the full container image repository
	// (e.g., "ghcr.io/plantonhq/planton/control-plane").
	// +optional
	Repository string `json:"repository,omitempty"`

	// tag overrides the image tag. Defaults to spec.version when empty.
	// +optional
	Tag string `json:"tag,omitempty"`
}

// ControlPlaneSpec configures the Planton control plane monolith deployment.
// The control plane is a Java/Spring Boot application that consolidates all
// backend services (IAM, resource manager, billing, etc.) into a single
// Kubernetes Deployment.
type ControlPlaneSpec struct {
	// image overrides the default container image for the control plane.
	// +optional
	Image *ImageSpec `json:"image,omitempty"`

	// replicas sets the number of control plane pods.
	// +kubebuilder:default=1
	// +kubebuilder:validation:Minimum=1
	// +optional
	Replicas *int32 `json:"replicas,omitempty"`

	// externalConfigSecretName references a user-created Secret in the same
	// namespace containing external service credentials (Auth0, Stripe,
	// GitHub, etc.). All keys in the Secret are injected as environment
	// variables via envFrom. The control plane will fail to start without
	// this configuration when running with the real (non-placeholder) image.
	// +optional
	ExternalConfigSecretName string `json:"externalConfigSecretName,omitempty"`

	// iacModulesVersion overrides the release version at which the platform
	// resolves official IaC module artifacts (both engines: the OpenTofu
	// module zips and the Pulumi module binaries ride the same release tag).
	// It feeds the control plane's PLANTON_VERSION environment variable and
	// controls nothing else -- deliberately NOT the platform image version
	// (spec.version) and NOT the infra-charts pin (charts are validated by
	// the control plane's own protos, so their tag stays compile-locked to
	// the image). Unset means the operator's verified default pin; setting
	// it is a deliberate operator act, e.g. adopting a newer module release
	// ahead of an operator upgrade. Every value must have a published
	// artifact set under downloads.planton.dev/releases/<version>/ or
	// deploys fail at module download.
	// +kubebuilder:validation:Pattern=`^v\d+\.\d+\.\d+$`
	// +optional
	IacModulesVersion string `json:"iacModulesVersion,omitempty"`

	// serviceAccountAnnotations are applied to the control plane pod's
	// dedicated Kubernetes ServiceAccount. This is the workload-identity seam
	// for the control plane's OWN cloud calls -- ambient-authenticated secret
	// backends and their KMS encryption keys (e.g.
	// eks.amazonaws.com/role-arn: <role> on EKS, the GKE/AKS equivalents
	// elsewhere). The ServiceAccount always exists, so adding annotations
	// later is a pure spec edit -- no pod surgery. Distinct from
	// spec.runner.serviceAccountAnnotations, which grants the DEPLOY worker
	// its cloud identity; this grants the platform itself one.
	// +optional
	ServiceAccountAnnotations map[string]string `json:"serviceAccountAnnotations,omitempty"`
}

// ConsoleSpec configures the Planton web console deployment.
// The console is a Next.js application that provides the web UI.
type ConsoleSpec struct {
	// image overrides the default container image for the web console.
	// +optional
	Image *ImageSpec `json:"image,omitempty"`

	// replicas sets the number of web console pods.
	// +kubebuilder:default=1
	// +kubebuilder:validation:Minimum=1
	// +optional
	Replicas *int32 `json:"replicas,omitempty"`

	// externalConfigSecretName references a user-created Secret in the same
	// namespace containing external configuration (Auth0, NextAuth, etc.).
	// All keys in the Secret are injected as environment variables via envFrom.
	// +optional
	ExternalConfigSecretName string `json:"externalConfigSecretName,omitempty"`
}

// RunnerSpec configures the in-cluster Planton runner: the worker pod that
// executes IaC deployments (OpenTofu) for cloud resources created through the
// platform. Deployed by default -- without it an install can browse the
// catalog but never deploy anything real. The operator seeds everything the
// runner needs (registration, credential, deploy defaults) at control-plane
// boot; no registration ceremony.
type RunnerSpec struct {
	// enabled controls whether the in-cluster runner is deployed. Default
	// true: a Planton that cannot deploy infrastructure is a browsing UI, so
	// opting OUT is the deliberate act (e.g. an install that only uses
	// externally registered runners).
	// +kubebuilder:default=true
	// +optional
	Enabled *bool `json:"enabled,omitempty"`

	// image overrides the default runner container image.
	// +optional
	Image *ImageSpec `json:"image,omitempty"`

	// storageSize is the persistent volume holding IaC state: the default
	// state backend keeps each resource's OpenTofu state file here. Defaults
	// to spec.storage.size, then 2Gi. Honest limits: state durability equals
	// this volume's durability -- back it up like you would any state file,
	// or graduate to an S3/GCS/Azure/R2 bucket StateBackend (or a Terraform
	// Cloud remote backend) through the console for managed-grade durability.
	// +optional
	StorageSize resource.Quantity `json:"storageSize,omitempty"`

	// storageClassName pins the IaC-state volume to a StorageClass. Defaults
	// to spec.storage.storageClassName, then the cluster default. Applied
	// when the volume is first created; an existing volume keeps its class.
	// +optional
	StorageClassName string `json:"storageClassName,omitempty"`

	// serviceAccountAnnotations are applied to the runner pod's dedicated
	// Kubernetes ServiceAccount. This is the workload-identity seam: annotate
	// with your cloud's binding (EKS "eks.amazonaws.com/role-arn", GKE
	// "iam.gke.io/gcp-service-account", AKS "azure.workload.identity/client-id")
	// and the runner's deploys use that ambient identity -- no cloud keys
	// stored anywhere, in the platform or the cluster.
	// +optional
	ServiceAccountAnnotations map[string]string `json:"serviceAccountAnnotations,omitempty"`

	// cloudCredentialsSecretName references a Secret in the same namespace
	// whose keys are injected into the runner pod as environment variables
	// (e.g. AWS_ACCESS_KEY_ID/AWS_SECRET_ACCESS_KEY). The path for clusters
	// without workload identity: the Secret stays yours, in your cluster --
	// the platform never stores cloud credentials. Prefer
	// serviceAccountAnnotations where the cluster supports it.
	// +optional
	CloudCredentialsSecretName string `json:"cloudCredentialsSecretName,omitempty"`
}

// BuildSpec configures the pipeline-build capability -- the machinery that
// turns a git push into a container image on this cluster. Enabling it (the
// default) makes the operator install Tekton Pipelines (or detect an existing
// install), point Tekton's CloudEvents sink at the runner's webhook, run the
// runner's pipeline-build worker, and seed this cluster as the platform's
// build destination at control-plane boot. The field is named for the
// CAPABILITY (builds), not the engine (Tekton) -- the same split as
// components.authorization/OpenFGA.
type BuildSpec struct {
	// enabled controls whether the build capability is deployed. Default
	// true: builds power Service Hub -- an install without them can deploy
	// infrastructure but never build a service from source, which is half
	// the product. Opting OUT is the deliberate act (e.g. an install that
	// only manages infrastructure). Builds also require the runner: with
	// spec.runner disabled, builds follow it off.
	// +kubebuilder:default=true
	// +optional
	Enabled *bool `json:"enabled,omitempty"`
}

// RemoteRunnersSpec configures the remote-runners capability: runners in other
// networks (developer laptops, appliances) pulling this install's deploy work.
// The capability rides the front door: the operator routes the deploy queue's
// service through the platform hostname, beside the native gRPC API, and
// advertises that address to runners that enroll from outside. It needs a
// Gateway API front door serving the hostname (the queue speaks native gRPC,
// which only that door carries) -- on any other door the capability stays
// closed and the ingress component's status says why. Named for the
// CAPABILITY (remote runners), not the mechanism (a queue route).
//
// What is exposed: the deploy queue's WorkflowService, over TLS, without
// authentication of its own -- the same posture the hosted platform carries
// for its remote runners. Only the queue's workflow service is routed; its
// administrative service never leaves the cluster.
type RemoteRunnersSpec struct {
	// enabled opens the deploy queue to runners outside this cluster and
	// advertises the front door's address to them. Default false: an install
	// that has not chosen this keeps its queue in-cluster, and a runner that
	// asks to enroll from outside is refused with the reason -- never handed
	// an address it cannot reach.
	// +kubebuilder:default=false
	// +optional
	Enabled *bool `json:"enabled,omitempty"`
}

// PlantonPlatformSpec defines the desired state of a self-hosted Planton deployment.
// A minimal spec requires only the version field; all other fields have sensible defaults.
// +kubebuilder:validation:XValidation:rule="!has(self.bootstrap) || !has(self.bootstrap.secretBackend) || self.bootstrap.secretBackend.type != 'platform' || !has(self.vault) || !has(self.vault.enabled) || self.vault.enabled",message="bootstrap.secretBackend type 'platform' stores secrets in the bundled vault, which spec.vault.enabled: false has opted out of; re-enable the vault or use type awsSecretsManager"
// +kubebuilder:validation:XValidation:rule="!has(self.remoteRunners) || !has(self.remoteRunners.enabled) || !self.remoteRunners.enabled || (has(self.ingress) && self.ingress.enabled)",message="remoteRunners.enabled opens the deploy queue to runners outside the cluster through the front door, but with ingress disabled there is no front door a laptop could reach; set ingress.enabled: true with a gatewayRef, or leave remoteRunners off"
type PlantonPlatformSpec struct {
	// version is the Planton platform release to deploy, as vMAJOR.MINOR.PATCH
	// (a pre-release suffix is allowed). The control plane, console, and runner
	// run this version as one coherent line; changing it is how the platform
	// upgrades. The operator refuses a release older than the oldest it runs
	// and says so in status (VersionSupported). To run a custom build, keep
	// version at a release and set image.tag on the component: the version
	// names the contract, the tag names the bytes.
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:XValidation:rule="self.matches('^v[0-9]+\\\\.[0-9]+\\\\.[0-9]+(-[0-9A-Za-z.-]+)?(\\\\+[0-9A-Za-z.-]+)?$')",message="spec.version must name a Planton release as vMAJOR.MINOR.PATCH (a pre-release suffix is allowed); to run a custom build, keep version at a release and set image.tag on the component"
	Version string `json:"version"`

	// license delivers the deployment's license key -- inline or by Secret
	// reference (at most one). Without it, Planton runs in Community mode.
	// +optional
	License *LicenseSpec `json:"license,omitempty"`

	// email declares the mail provider this install sends through -- an SMTP
	// relay or a Resend account, the address it sends as, credentials by
	// Secret reference. Its presence is what turns email on: invitations are
	// emailed as well as linked, alerts reach people, and the sign-in page
	// offers "Forgot password?". Without it the install sends nothing and
	// says so wherever an email would have gone.
	// +optional
	Email *EmailSpec `json:"email,omitempty"`

	// storage sets platform-wide storage defaults for every persistent
	// volume the operator creates: the StorageClass volumes are provisioned
	// from and one size applied across all of them. Component settings
	// (e.g. spec.database.postgresql.storageSize) override these per volume.
	// +optional
	Storage *StorageSpec `json:"storage,omitempty"`

	// database configures storage for the data-layer components
	// (PostgreSQL and Redis).
	// +optional
	Database *DatabaseSpec `json:"database,omitempty"`

	// ingress configures external access to the Planton web console and API.
	// When disabled, the platform is served by the built-in front-door gateway
	// over kubectl port-forward (see spec.gateway) -- sign-in included.
	// +optional
	Ingress *IngressSpec `json:"ingress,omitempty"`

	// gateway tunes the built-in front door that serves the platform when
	// ingress is disabled: one in-cluster proxy Service presenting the console,
	// the API, and sign-in on a single origin, reached with one kubectl
	// port-forward command (printed in the gateway component's status). Every
	// field is optional; the gateway deploys automatically whenever ingress is
	// off and retires when ingress takes over as the front door.
	// +optional
	Gateway *GatewaySpec `json:"gateway,omitempty"`

	// identity tunes the bundled identity server (Keycloak) that provides
	// browser sign-in. Identity is not a toggle: every install deploys it, so
	// "deploy Planton" always yields working multi-user sign-in -- through the
	// port-forward front door without ingress, or at the public hostname with
	// it. Every field is optional: a zero-config install gets a fully
	// provisioned identity server, and the console's first-run setup page
	// creates the first admin (adminEmail remains the GitOps override that
	// seeds one instead).
	// +optional
	Identity *IdentitySpec `json:"identity,omitempty"`

	// bootstrap seeds the first-boot workspace: a default organization with a
	// starter environment, and the admins who own it. The first declared admin
	// to sign in lands inside a ready workspace as its owner and the install's
	// platform operator -- no create-org ceremony. Every field is optional;
	// defaults produce an organization and environment both named "default",
	// with spec.identity.adminEmail as the sole admin.
	// +optional
	Bootstrap *BootstrapSpec `json:"bootstrap,omitempty"`

	// runner configures the in-cluster worker that executes IaC deployments.
	// Deployed by default so a fresh install can deploy real infrastructure
	// out of the box; every field is optional.
	// +optional
	Runner *RunnerSpec `json:"runner,omitempty"`

	// build configures the pipeline-build capability: Tekton on this cluster,
	// the runner's build worker and webhook, and the seeded build routing.
	// Enabled by default -- builds power Service Hub, and an install without
	// them is half a product; every field is optional.
	// +optional
	Build *BuildSpec `json:"build,omitempty"`

	// remoteRunners lets runners OUTSIDE this cluster -- a developer's laptop
	// deploying with the cloud sign-in already on it, an appliance in another
	// network -- pull deploy work from this install. Off by default; every
	// field is optional. The in-cluster runner is unaffected either way.
	// +optional
	RemoteRunners *RemoteRunnersSpec `json:"remoteRunners,omitempty"`

	// vault configures the bundled secrets manager (OpenBAO, the open-source
	// Vault fork). Deployed by default: it is integral the way the database
	// is -- it backs the credential store for pasted connection secrets, the
	// default envelope-encryption key, and the OIDC issuer's signing key
	// (keyless connections). Every field is optional; a zero-config install
	// gets an initialized, unsealed vault with its engines mounted and the
	// platform secret backend seeded as the org default. Choosing a cloud
	// secret backend instead is a layered choice, not a reason to opt out.
	// +optional
	Vault *OpenBAOSpec `json:"vault,omitempty"`

	// components toggles optional platform capabilities that are disabled by default
	// to keep the minimal deployment footprint small.
	// +optional
	Components *ComponentsSpec `json:"components,omitempty"`

	// prerequisites controls how the operator deploys its own dependencies
	// (sub-operators like CloudNativePG). Defaults to auto-detection.
	// +optional
	Prerequisites *PrerequisitesSpec `json:"prerequisites,omitempty"`

	// controlPlane configures the Planton control plane monolith.
	// The control plane is always deployed. Image defaults to the official
	// GHCR image tagged with spec.version.
	// +optional
	ControlPlane *ControlPlaneSpec `json:"controlPlane,omitempty"`

	// console configures the Planton web console (Next.js).
	// The console is always deployed. Image defaults to the official
	// GHCR image tagged with spec.version.
	// +optional
	Console *ConsoleSpec `json:"console,omitempty"`
}

// PrerequisitesSpec controls deployment of sub-operators that the Planton
// operator depends on. The default behavior ("auto") detects whether each
// sub-operator is already installed and deploys it only if missing.
type PrerequisitesSpec struct {
	// postgresOperator controls deployment of the CloudNativePG operator
	// (the engine behind the platform's PostgreSQL cluster).
	// "auto" (default): detect if installed, deploy if missing. A
	// CloudNativePG installed by any other means (Helm, GitOps) is detected
	// and respected automatically.
	// "skip": assume already installed, do not deploy.
	// +kubebuilder:default="auto"
	// +kubebuilder:validation:Enum=auto;skip
	// +optional
	PostgresOperator string `json:"postgresOperator,omitempty"`

	// tektonPipelines controls deployment of Tekton Pipelines (the build
	// engine behind spec.build).
	// "auto" (default): detect if installed, deploy if missing.
	// "skip": assume already installed, do not deploy. The operator still
	// points Tekton's CloudEvents sink at the runner webhook -- sink wiring
	// is the build capability's job regardless of who installed Tekton.
	// +kubebuilder:default="auto"
	// +kubebuilder:validation:Enum=auto;skip
	// +optional
	TektonPipelines string `json:"tektonPipelines,omitempty"`
}

// LicenseSpec delivers the deployment's Planton license key. Without one,
// Planton runs as Community -- fully functional core, no licensed extras.
// The operator only delivers the key; the control plane verifies it and
// resolves what it grants (expiry never blocks running workloads).
// +kubebuilder:validation:XValidation:rule="!has(self.key) || !has(self.secretKeyRef)",message="set at most one of key or secretKeyRef"
type LicenseSpec struct {
	// key is the license key pasted verbatim (the compact signed token from
	// the purchase email). Convenient for a first install; prefer
	// secretKeyRef in GitOps trees so the key never lives in the CR.
	// +optional
	Key string `json:"key,omitempty"`

	// secretKeyRef points at an entry in an existing Secret (same
	// namespace) holding the license key. Rotating the Secret's value
	// re-delivers the key on the next control-plane restart -- a renewal is
	// a Secret edit, never a reinstall.
	// +optional
	SecretKeyRef *SecretKeyRef `json:"secretKeyRef,omitempty"`
}

// EmailSpec declares the one mail provider every sender on the install uses:
// the control plane (invitations, alerts, license mail) and the identity
// server (password resets) both send through it, so one declaration is the
// whole configuration. Exactly one provider arm is set. The operator delivers
// the declaration and preflights every Secret it names; it never probes the
// relay itself (a relay probed every thirty seconds is somebody's intrusion
// alert). The control plane checks the connection on demand and reports each
// relay failure in the relay's own words.
// +kubebuilder:validation:XValidation:rule="has(self.smtp) != has(self.resend)",message="email declares exactly one provider: set spec.email.smtp for a relay or spec.email.resend for a Resend account, never both, never neither"
type EmailSpec struct {
	// from is the identity every email carries: the address the install
	// sends as and the display name beside it. The relay must permit sending
	// as this address (a mailbox's own address, or one it has Send As rights
	// to); SPF and DKIM for the domain are the domain owner's job.
	From EmailFromSpec `json:"from"`

	// replyTo is where a person's reply lands -- a help desk or a shared
	// mailbox -- when the sending address is a no-reply one. Unset, replies
	// go to from.address.
	// +optional
	ReplyTo string `json:"replyTo,omitempty"`

	// smtp sends through any SMTP relay: a workplace mail system (Exchange
	// Online, Google Workspace, an internal smart host) or a transactional
	// vendor's SMTP endpoint (SES, SendGrid, Postmark, Mailgun, Resend).
	// +optional
	SMTP *EmailSMTPSpec `json:"smtp,omitempty"`

	// resend sends through Resend's API with an API key.
	// +optional
	Resend *EmailResendSpec `json:"resend,omitempty"`
}

// EmailFromSpec is the sender identity on every email the install sends.
type EmailFromSpec struct {
	// address the install sends as, e.g. no-reply@planton.acme.com. Required
	// whenever email is declared: there is no default address, because a
	// default would name somebody else's domain.
	// +kubebuilder:validation:MinLength=1
	Address string `json:"address"`

	// name shown beside the address in mail clients.
	// +kubebuilder:default="Planton"
	// +optional
	Name string `json:"name,omitempty"`
}

// EmailSMTPSecurity is how the connection to the relay is protected.
// +kubebuilder:validation:Enum=starttls;tls;none
type EmailSMTPSecurity string

const (
	// EmailSMTPSecurityStartTLS connects in the clear and REQUIRES the
	// STARTTLS upgrade before anything else is sent; a relay that does not
	// offer it is a failed connection, never a silent fallback. The
	// submission default (port 587).
	EmailSMTPSecurityStartTLS EmailSMTPSecurity = "starttls"
	// EmailSMTPSecurityTLS opens a TLS connection from the first byte
	// (implicit TLS, port 465 on most relays).
	EmailSMTPSecurityTLS EmailSMTPSecurity = "tls"
	// EmailSMTPSecurityNone is plaintext end to end and never upgrades. For
	// an internal relay that admits this cluster's address without a
	// credential; credentials are refused on it.
	EmailSMTPSecurityNone EmailSMTPSecurity = "none"
)

// EmailSMTPSpec points the install at an SMTP relay. Three ways to be let in,
// matching how mail teams actually run relays: a username and password
// (kubernetes.io/basic-auth Secret), an OAuth2 app registration (Exchange
// Online, whose password submission Microsoft is retiring), or no credential
// at all (an internal smart host that allow-lists the cluster's egress).
// +kubebuilder:validation:XValidation:rule="!(has(self.credentialsSecretName) && size(self.credentialsSecretName) > 0 && has(self.oauth2))",message="smtp authenticates one way: set credentialsSecretName for a username and password, or oauth2 for a token, not both"
// +kubebuilder:validation:XValidation:rule="!has(self.security) || self.security != 'none' || (!(has(self.credentialsSecretName) && size(self.credentialsSecretName) > 0) && !has(self.oauth2))",message="security: none would send credentials in the clear; keep security at starttls or tls, or drop credentialsSecretName and oauth2 for a relay that admits this cluster's address without them"
type EmailSMTPSpec struct {
	// host of the relay, e.g. smtp.office365.com or smtp-relay.corp.acme.com.
	// +kubebuilder:validation:MinLength=1
	Host string `json:"host"`

	// port the relay listens on. 587 is the submission port most relays use
	// with STARTTLS; implicit-TLS relays (security: tls) usually listen on
	// 465; an internal plaintext relay on 25.
	// +kubebuilder:default=587
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:validation:Maximum=65535
	// +optional
	Port int32 `json:"port,omitempty"`

	// security protects the connection: starttls (required upgrade after
	// connect, the default), tls (implicit TLS from the first byte), or none
	// (plaintext, credential-free relays only).
	// +kubebuilder:default="starttls"
	// +optional
	Security EmailSMTPSecurity `json:"security,omitempty"`

	// credentialsSecretName names a kubernetes.io/basic-auth Secret in this
	// namespace whose username and password keys sign in to the relay:
	//
	//   kubectl -n <namespace> create secret generic planton-email \
	//     --type=kubernetes.io/basic-auth \
	//     --from-literal=username=... --from-literal=password=...
	//
	// Omit it for a relay that admits this cluster by network address. The
	// values are never inline and reach the control plane as mounted files,
	// so a rotated password is live on the next send with no restart.
	// +optional
	CredentialsSecretName string `json:"credentialsSecretName,omitempty"`

	// oauth2 signs in with a token from an OAuth2 client-credentials grant
	// (SASL XOAUTH2) instead of a password. Exchange Online: tokenUrl
	// https://login.microsoftonline.com/<tenant>/oauth2/v2.0/token, scope
	// https://outlook.office365.com/.default, the app registration's client
	// id and secret, and user = the mailbox the app may send as.
	// +optional
	OAuth2 *EmailSMTPOAuth2Spec `json:"oauth2,omitempty"`

	// caBundleSecretRef points at a PEM CA bundle for verifying the relay's
	// TLS certificate -- the private-CA case, the classic enterprise blocker.
	// Omit it when the relay's certificate chains to a public root.
	// +optional
	CABundleSecretRef *SecretKeyRef `json:"caBundleSecretRef,omitempty"`
}

// EmailSMTPOAuth2Spec is the client-credentials grant that yields the SMTP
// token: the same four facts the identity server needs, so one declaration
// powers both senders.
type EmailSMTPOAuth2Spec struct {
	// user is the mailbox the token sends as -- the account the app
	// registration has been permitted to use.
	// +kubebuilder:validation:MinLength=1
	User string `json:"user"`

	// tokenUrl is the provider's OAuth2 token endpoint.
	// +kubebuilder:validation:Pattern=`^https://`
	TokenURL string `json:"tokenUrl"`

	// scope requested for the token.
	// +kubebuilder:validation:MinLength=1
	Scope string `json:"scope"`

	// clientId of the app registration.
	// +kubebuilder:validation:MinLength=1
	ClientID string `json:"clientId"`

	// clientSecretRef points at the app registration's client secret.
	// Required by reference: the secret is never inline.
	ClientSecretRef SecretKeyRef `json:"clientSecretRef"`
}

// EmailResendSpec sends through Resend's HTTP API.
type EmailResendSpec struct {
	// apiKeySecretRef points at the Resend API key. Required by reference:
	// the key is never inline. Reaches the control plane as a mounted file,
	// so a rotated key is live on the next send with no restart.
	APIKeySecretRef SecretKeyRef `json:"apiKeySecretRef"`
}

// StorageSpec sets platform-wide storage defaults. Some backends make these
// essential in combination: a cluster whose only StorageClass is not marked
// default needs storageClassName, and backends enforcing large minimum volume
// sizes (e.g. NAS systems with an 800Gi floor) need size -- one block covers
// every volume instead of a per-component hunt.
//
// Resolution order for every volume: the component's own setting wins, then
// this block, then the component's documented default (with the cluster's
// default StorageClass when no class is named anywhere). Storage class and
// size are fixed when a volume is first created -- set them before
// installing; changing them afterward requires recreating the volume.
type StorageSpec struct {
	// storageClassName is the StorageClass every persistent volume is
	// provisioned from unless a component names its own. When empty, volumes
	// use the cluster's default StorageClass.
	// +optional
	StorageClassName string `json:"storageClassName,omitempty"`

	// size is applied to every persistent volume unless a component sets
	// its own storageSize.
	// +optional
	Size resource.Quantity `json:"size,omitempty"`
}

// DatabaseSpec configures storage for the data layer components.
// All sizes use Kubernetes resource.Quantity format (e.g., "10Gi", "500Mi").
type DatabaseSpec struct {
	// postgresql configures the platform's PostgreSQL cluster.
	// +optional
	PostgreSQL *PostgreSQLSpec `json:"postgresql,omitempty"`

	// redis configures storage for the redis-protocol cache instance. The
	// field keeps the protocol-role name every consumer speaks; the engine
	// serving it is Valkey (see the valkey resources for the rationale).
	// +optional
	Redis *RedisSpec `json:"redis,omitempty"`
}

// PostgreSQLSpec configures the platform's PostgreSQL cluster, deployed and
// managed by CloudNativePG (installed automatically as a prerequisite unless
// spec.prerequisites.postgresOperator is set to "skip"). One cluster hosts
// every platform database.
type PostgreSQLSpec struct {
	// storageSize is the persistent volume size per PostgreSQL instance.
	// Defaults to spec.storage.size, then 10Gi. (Defaults resolve in the
	// operator, not in this schema, so the platform-wide size can tell
	// "unset" apart from "chosen".) Sizes can grow after install by editing
	// this field; they can never shrink.
	// +optional
	StorageSize resource.Quantity `json:"storageSize,omitempty"`

	// storageClassName pins the PostgreSQL volumes to a StorageClass.
	// Defaults to spec.storage.storageClassName, then the cluster default.
	// +optional
	StorageClassName string `json:"storageClassName,omitempty"`

	// replicas is the number of PostgreSQL instances in the cluster: a
	// primary plus streaming-replication hot standbys with automated
	// failover. 1 (the default) is a single instance; raising it after
	// install grows the topology live.
	// +kubebuilder:default=1
	// +kubebuilder:validation:Minimum=1
	// +optional
	Replicas *int32 `json:"replicas,omitempty"`
}

// RedisSpec configures storage for the redis-protocol cache (served by Valkey).
type RedisSpec struct {
	// storageSize is the persistent volume size for the cache instance.
	// Defaults to spec.storage.size, then 1Gi.
	// +optional
	StorageSize resource.Quantity `json:"storageSize,omitempty"`

	// storageClassName pins the cache volume to a StorageClass. Defaults to
	// spec.storage.storageClassName, then the cluster default.
	// +optional
	StorageClassName string `json:"storageClassName,omitempty"`
}

// IngressSpec configures external access to Planton through the cluster's
// existing front door -- an Ingress controller, or a Gateway API Gateway. One
// hostname serves the web console, the API the browser calls, and sign-in
// (same origin -- no cross-site concerns, and auth callback URLs are derived
// from it rather than configured).
//
// The fields form a friction ladder -- each is one step up from the previous,
// and every step is a working deployment:
//
//	enabled: true                     -> a URL derived from the front door's
//	                                     public address (no DNS setup needed),
//	                                     plain HTTP
//	+ hostname                        -> your own hostname, plain HTTP
//	+ tls.secretName                  -> HTTPS with a cert you bring
//	+ tls.issuer                      -> HTTPS issued/renewed by cert-manager
//
// Which front door serves the platform is the one fork: ingressClassName (or
// the cluster's default IngressClass) renders an Ingress object; gatewayRef
// renders an HTTPRoute attached to the named Gateway. Both carry the same
// route table.
//
// +kubebuilder:validation:XValidation:rule="!has(self.tls) || (has(self.hostname) && self.hostname != \"\")",message="tls requires hostname: a certificate cannot be brought or issued for an auto-derived hostname"
// +kubebuilder:validation:XValidation:rule="!has(self.gatewayRef) || !has(self.ingressClassName) || self.ingressClassName == \"\"",message="gatewayRef and ingressClassName name two different front doors; set one -- gatewayRef attaches to a Gateway API Gateway, ingressClassName renders an Ingress"
// +kubebuilder:validation:XValidation:rule="!has(self.gatewayRef) || !has(self.tls) || !has(self.tls.secretName)",message="with gatewayRef the Gateway's HTTPS listener owns the certificate: attach to a listener that already serves the hostname, or set tls.issuer to have a certificate issued for the listener to reference"
// +kubebuilder:validation:XValidation:rule="!has(self.reachability) || self.reachability != \"public\" || self.enabled",message="reachability: public declares an address the internet reaches, but with enabled: false the platform is reached only through kubectl port-forward from the machine running it; set enabled: true, or leave reachability at auto"
type IngressSpec struct {
	// enabled controls whether a front-door route is created.
	// When false, use kubectl port-forward for access.
	// +kubebuilder:default=false
	// +optional
	Enabled bool `json:"enabled,omitempty"`

	// hostname is the domain Planton is served at (e.g., "planton.example.com").
	// When empty, the operator derives a hostname from the front door's
	// published address (an IP becomes a sslip.io magic-DNS name; a load
	// balancer DNS name is used directly) and reports it in status.consoleUrl.
	// The derived hostname is a zero-setup convenience for evaluation, not a
	// production endpoint.
	// +optional
	Hostname string `json:"hostname,omitempty"`

	// ingressClassName selects which ingress controller serves Planton.
	// When empty (and gatewayRef is unset), the cluster's default
	// IngressClass is used.
	// +optional
	IngressClassName string `json:"ingressClassName,omitempty"`

	// gatewayRef serves Planton through a Gateway API Gateway the cluster
	// already runs (Istio, Envoy Gateway, Cilium, a cloud Gateway) instead
	// of an Ingress controller: the operator attaches an HTTPRoute for the
	// hostname to that Gateway. The Gateway is the cluster team's object and
	// is never modified -- its listeners decide which hostnames are admitted,
	// which namespaces may attach routes, and how HTTPS is terminated; the
	// operator reads those facts and explains any mismatch in this
	// component's status.
	// +optional
	GatewayRef *GatewayParentRef `json:"gatewayRef,omitempty"`

	// annotations are added to the Ingress resource for controller-specific
	// behavior. They take precedence over the operator's own defaults.
	// Ignored with gatewayRef (the Gateway API expresses behavior in typed
	// fields, not annotations).
	// +optional
	Annotations map[string]string `json:"annotations,omitempty"`

	// tls enables HTTPS. When absent, Planton is served over plain HTTP and
	// the ingress component's status notes the connection is unencrypted --
	// except with gatewayRef, where the route attaches to whichever listener
	// matches the hostname and HTTPS is inferred from that listener.
	// +optional
	TLS *IngressTLSSpec `json:"tls,omitempty"`

	// reachability declares whether the public internet can reach this
	// front door. The operator cannot observe that from inside the cluster,
	// and the capabilities that need an inbound path from the internet --
	// keyless cloud connections, GitHub webhook delivery -- are offered only
	// where the door is public. "auto" (default) resolves from the door's
	// shape: a hostname served over HTTPS is public, anything else private.
	// Declare "private" for an HTTPS door only your network can reach
	// (split DNS, a corporate CA); declare "public" to affirm it. Only
	// "public" is refused on a disabled ingress -- a port-forward door is
	// never reached from the internet -- while "private" there is simply
	// true. A definition older than this field refuses the declaration
	// outright (server-side apply never prunes an unknown field), and the
	// refusal names the operator to upgrade.
	// +kubebuilder:default="auto"
	// +optional
	Reachability IngressReachability `json:"reachability,omitempty"`
}

// GatewayParentRef names the Gateway API Gateway an HTTPRoute attaches to.
type GatewayParentRef struct {
	// name of the Gateway.
	// +kubebuilder:validation:MinLength=1
	Name string `json:"name"`

	// namespace of the Gateway. Defaults to the platform's own namespace.
	// A Gateway in another namespace must allow routes from this one
	// (spec.listeners[].allowedRoutes.namespaces).
	// +optional
	Namespace string `json:"namespace,omitempty"`

	// sectionName pins the route to one named listener of the Gateway. When
	// empty, the route attaches to every listener whose hostname admits the
	// platform's hostname.
	// +optional
	SectionName string `json:"sectionName,omitempty"`
}

// IngressTLSSpec configures how the TLS certificate for the hostname is
// obtained. Exactly one of secretName (bring your own) or issuer
// (cert-manager) must be set.
//
// With gatewayRef, only issuer applies: the operator asks cert-manager for the
// certificate in the platform's namespace and grants the Gateway's namespace
// permission to reference the resulting Secret (a ReferenceGrant); the
// Gateway's HTTPS listener names that Secret in its certificateRefs. A
// certificate the listener already serves needs no tls block at all.
// +kubebuilder:validation:XValidation:rule="has(self.secretName) != has(self.issuer)",message="exactly one of secretName or issuer must be set"
type IngressTLSSpec struct {
	// secretName references an existing kubernetes.io/tls Secret in the same
	// namespace holding the certificate for the hostname.
	// +optional
	SecretName string `json:"secretName,omitempty"`

	// issuer references a cert-manager Issuer or ClusterIssuer that obtains
	// and renews the certificate automatically. Requires cert-manager to be
	// installed in the cluster.
	// +optional
	Issuer *CertManagerIssuerRef `json:"issuer,omitempty"`
}

// GatewaySpec tunes the built-in front-door gateway that serves the platform
// when ingress is disabled. The gateway mirrors the ingress path layout on one
// origin -- "/" to the console, the gRPC-Web API prefix to the control plane,
// "/idp" to the identity server -- so the two access modes are the same
// architecture at different addresses.
//
// The local port matters because sign-in bakes it into URLs: the identity
// server's advertised issuer and the console's auth callbacks are pinned to
// http://localhost:{localPort}, so the port-forward MUST map that exact local
// port (the gateway component's status prints the full command).
type GatewaySpec struct {
	// localPort is the localhost port the port-forward is expected to serve
	// the platform at. Change it only when the default clashes with something
	// already listening on your workstation.
	// +kubebuilder:default=8080
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:validation:Maximum=65535
	// +optional
	LocalPort *int32 `json:"localPort,omitempty"`

	// image overrides the default nginx container image the gateway runs.
	// +optional
	Image *ImageSpec `json:"image,omitempty"`
}

// CertManagerIssuerRef identifies a cert-manager issuer.
type CertManagerIssuerRef struct {
	// name of the Issuer or ClusterIssuer.
	// +kubebuilder:validation:MinLength=1
	Name string `json:"name"`

	// kind of the issuer resource.
	// +kubebuilder:default="Issuer"
	// +kubebuilder:validation:Enum=Issuer;ClusterIssuer
	// +optional
	Kind string `json:"kind,omitempty"`
}

// IdentitySpec tunes the bundled identity server. The operator deploys and
// fully provisions Keycloak on every install -- realm, OIDC client, and the
// first admin user are generated declaratively (credentials land in
// Kubernetes Secrets) -- so sign-in works with no external identity setup.
// These fields only adjust that provisioning; none are required.
type IdentitySpec struct {
	// image overrides the default Keycloak container image.
	// +optional
	Image *ImageSpec `json:"image,omitempty"`

	// realm is the Keycloak realm name that holds Planton's users and the
	// console's OIDC client.
	// +kubebuilder:default="planton"
	// +optional
	Realm string `json:"realm,omitempty"`

	// adminEmail is the GitOps override for the first admin: when set, a user
	// with this email is seeded into the realm, their generated one-time
	// password lands in the {crName}-identity-admin-user Secret, and first
	// sign-in forces a password change and asks for their name. When unset
	// (the default), NO user is seeded (there is deliberately no generic
	// pre-baked admin identity): the first person to open the console is
	// asked for THEIR email plus a setup code from the
	// {crName}-identity-setup-code Secret -- proof of cluster access -- and
	// the admin account is created from that, with admin privileges pinned to
	// the created account rather than to an email string.
	// +optional
	AdminEmail string `json:"adminEmail,omitempty"`
}

// BootstrapSpec configures the config-driven first-boot seeds. The seeded
// records are created idempotently by the control plane on startup, keyed on
// their identity, so re-applying or editing the spec reconciles rather than
// duplicates.
type BootstrapSpec struct {
	// organization is the default organization every declared admin owns.
	// +optional
	Organization *BootstrapOrganizationSpec `json:"organization,omitempty"`

	// environment is the starter environment seeded inside the organization.
	// +optional
	Environment *BootstrapEnvironmentSpec `json:"environment,omitempty"`

	// admins are the emails granted organization ownership AND the install's
	// platform-operator role -- at boot when their account already exists, or
	// the moment they first sign in. Editing this list takes effect on the
	// next control-plane restart (declarative, never fire-once). Defaults to
	// [spec.identity.adminEmail] when that field is set.
	// +optional
	Admins []string `json:"admins,omitempty"`

	// iacProvisioner is the IaC engine seeded as the organization's deploy
	// default. "tofu" (OpenTofu, the default) and "terraform" both execute
	// with the bundled OpenTofu binary against the Terraform-compatible
	// module catalog. Seeded create-once: a different choice made later in
	// the console is never overwritten by a restart. Pulumi is deliberately
	// not accepted here -- it has no zero-config state backend (its file
	// backend requires a state-encryption passphrase), so seeding it as the
	// default would produce an install where every deploy fails.
	// +kubebuilder:default="tofu"
	// +kubebuilder:validation:Enum=tofu;terraform
	// +optional
	IacProvisioner string `json:"iacProvisioner,omitempty"`

	// secretBackend declares the organization's default secret backend,
	// seeded create-once at boot (a choice made later in the console is never
	// overwritten). Both supported kinds are credential-free by construction:
	// "platform" stores secrets in the bundled vault (deployed by default;
	// incompatible with spec.vault.enabled: false) and is seeded
	// automatically when the vault runs and nothing is declared here;
	// "awsSecretsManager" stores secrets in AWS Secrets Manager, reached with
	// the control plane pod's own cloud identity (workload identity via
	// spec.controlPlane.serviceAccountAnnotations, instance profiles, or env
	// credentials) -- no key is ever typed or stored. When neither applies,
	// no default backend exists and the console guides secret-related
	// features until one is created.
	// +optional
	SecretBackend *BootstrapSecretBackendSpec `json:"secretBackend,omitempty"`
}

// BootstrapSecretBackendSpec declares the seeded default secret backend.
//
// The rule uses size(...) > 0 rather than comparing against an empty
// single-quoted string literal: gofmt (Go 1.26+) rewrites two adjacent single
// quotes in comments into one typographic quote, which would silently corrupt
// the CEL expression.
// +kubebuilder:validation:XValidation:rule="self.type != 'awsSecretsManager' || (has(self.awsSecretsManager) && size(self.awsSecretsManager.region) > 0)",message="awsSecretsManager.region is required when type is awsSecretsManager"
type BootstrapSecretBackendSpec struct {
	// type selects the backend kind: "platform" (the bundled vault) or
	// "awsSecretsManager" (ambient-authenticated AWS Secrets Manager).
	// +kubebuilder:validation:Enum=platform;awsSecretsManager
	Type string `json:"type"`

	// awsSecretsManager configures the awsSecretsManager type.
	// +optional
	AwsSecretsManager *BootstrapAwsSecretsManagerSpec `json:"awsSecretsManager,omitempty"`
}

// BootstrapAwsSecretsManagerSpec configures the ambient AWS Secrets Manager
// default backend: where secrets live. Addressing, never credentials -- the
// control plane reaches Secrets Manager with its own runtime identity, and
// values are stored provider-native under AWS's own encryption at rest.
type BootstrapAwsSecretsManagerSpec struct {
	// region is the AWS region secrets are stored in (e.g. "ap-south-1").
	Region string `json:"region"`
}

// BootstrapOrganizationSpec names the seeded default organization.
type BootstrapOrganizationSpec struct {
	// slug is the organization's URL-safe identifier (also its id).
	// +kubebuilder:default="default"
	// +optional
	Slug string `json:"slug,omitempty"`

	// name is the organization's display name. Defaults to the slug.
	// +optional
	Name string `json:"name,omitempty"`
}

// BootstrapEnvironmentSpec names the seeded starter environment.
type BootstrapEnvironmentSpec struct {
	// slug is the environment's URL-safe identifier within the organization.
	// +kubebuilder:default="default"
	// +optional
	Slug string `json:"slug,omitempty"`

	// name is the environment's display name. Defaults to the slug.
	// +optional
	Name string `json:"name,omitempty"`
}

// ComponentsSpec toggles optional platform capabilities.
//
// The minimal footprint runs only the essential core: the control plane serves
// authorization from its built-in allow-owner arm, search from its built-in
// Postgres projection, and the resource graph from the built-in Postgres
// provider. Each entry here is an opt-in upgrade to a heavier dedicated
// backend, off by default.
type ComponentsSpec struct {
	// authorization deploys OpenFGA and switches the control plane to
	// policy-engine authorization (fine-grained RBAC). Disabled by default: the
	// control plane runs the built-in allow-owner authorization arm, which needs
	// no OpenFGA. Enable this for multi-tenant, per-resource access control.
	// +optional
	Authorization *ComponentToggle `json:"authorization,omitempty"`

	// graph configures Neo4j for relationship graph queries.
	// Disabled by default.
	// +optional
	Graph *Neo4jSpec `json:"graph,omitempty"`
}

// ComponentToggle is a simple on/off switch for optional components.
type ComponentToggle struct {
	// enabled controls whether this component is deployed.
	// +kubebuilder:default=false
	// +optional
	Enabled bool `json:"enabled,omitempty"`
}

// OpenBAOSpec configures the bundled secrets manager (OpenBAO).
type OpenBAOSpec struct {
	// enabled controls whether the bundled secrets manager is deployed.
	// Default true: a Planton without a secrets store cannot hold pasted
	// connection credentials or serve keyless connections, so opting OUT is
	// the deliberate act (e.g. an install that runs exclusively on an
	// ambient-authenticated cloud secret backend and accepts losing those
	// capabilities).
	// +kubebuilder:default=true
	// +optional
	Enabled *bool `json:"enabled,omitempty"`

	// initMode controls how OpenBAO is initialized after deployment.
	// "auto" (default): the operator calls /sys/init and /sys/unseal
	//   automatically, storing unseal keys and root token in a Kubernetes
	//   Secret ({crName}-openbao-init). Convenient for dev and Kind clusters.
	// "manual": the operator deploys the chart but leaves initialization
	//   to the user. The component reports Deploying until manually
	//   initialized and unsealed.
	// +kubebuilder:default="auto"
	// +optional
	InitMode OpenBAOInitMode `json:"initMode,omitempty"`

	// storageSize is the persistent volume size for OpenBAO data. Defaults
	// to spec.storage.size, then 2Gi (secrets and encryption keys are
	// kilobytes; the volume needs headroom, not bulk).
	// +optional
	StorageSize resource.Quantity `json:"storageSize,omitempty"`

	// storageClassName pins the OpenBAO volume to a StorageClass. Defaults
	// to spec.storage.storageClassName, then the cluster default.
	// +optional
	StorageClassName string `json:"storageClassName,omitempty"`
}

// Neo4jSpec configures the Neo4j graph database deployment.
type Neo4jSpec struct {
	// enabled controls whether Neo4j is deployed.
	// +kubebuilder:default=false
	// +optional
	Enabled bool `json:"enabled,omitempty"`

	// storageSize is the persistent volume size for Neo4j data. Defaults to
	// spec.storage.size, then 10Gi.
	// +optional
	StorageSize resource.Quantity `json:"storageSize,omitempty"`

	// storageClassName pins the Neo4j data volume to a StorageClass.
	// Defaults to spec.storage.storageClassName, then the cluster default.
	// +optional
	StorageClassName string `json:"storageClassName,omitempty"`
}

// PlantonPlatformStatus defines the observed state of a PlantonPlatform deployment.
type PlantonPlatformStatus struct {
	// phase is the high-level lifecycle phase of the entire deployment.
	// +optional
	Phase PlantonPhase `json:"phase,omitempty"`

	// version is the currently deployed Planton platform version.
	// +optional
	Version string `json:"version,omitempty"`

	// consoleUrl is the URL the web console is reachable at once ingress is
	// configured and admitted. The platform tells you your URL -- with an
	// auto-derived hostname this is where the derived address is published.
	// +optional
	ConsoleURL string `json:"consoleUrl,omitempty"`

	// reachability is what the operator concluded about whether the public
	// internet reaches the front door: "public" or "private", never "auto".
	// Published beside consoleUrl by the component that owns the door, so a
	// person who left spec.ingress.reachability at its default reads the
	// answer next to the address it is an answer about. Every internet-facing
	// posture the control plane advertises (the keyless identity issuer,
	// GitHub webhook delivery) derives from this one word.
	// +optional
	Reachability IngressReachability `json:"reachability,omitempty"`

	// license echoes how the license key is delivered to the control plane
	// (Community, InlineKey, or SecretRef) -- configuration echo like
	// version, feeding the kubectl column. Whether the key VERIFIES and
	// what it grants is the control plane's own answer, served by its
	// entitlements advertisement, never guessed here.
	// +optional
	License string `json:"license,omitempty"`

	// email echoes which provider arm spec.email declares (NotConfigured,
	// SMTP, or Resend) -- configuration echo like license, feeding the
	// kubectl column. Whether the relay ACCEPTS mail is the control plane's
	// own answer, checked on demand from its settings page, never guessed
	// here: the operator has no channel to the relay and must not probe it.
	// +optional
	Email string `json:"email,omitempty"`

	// components reports the status of each individual component.
	// +optional
	Components ComponentStatuses `json:"components,omitempty"`

	// conditions represent the latest available observations of the deployment's state.
	// Condition types: Ready (all enabled components healthy) and VersionSupported
	// (spec.version names a release this operator runs).
	// +listType=map
	// +listMapKey=type
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// ComponentStatuses holds the status of every managed component.
type ComponentStatuses struct {
	// +optional
	PostgreSQL *ComponentStatus `json:"postgresql,omitempty"`
	// +optional
	Redis *ComponentStatus `json:"redis,omitempty"`
	// +optional
	OpenFGA *ComponentStatus `json:"openFGA,omitempty"`
	// +optional
	Temporal *ComponentStatus `json:"temporal,omitempty"`
	// +optional
	OpenBAO *ComponentStatus `json:"openBao,omitempty"`
	// +optional
	Neo4j *ComponentStatus `json:"neo4j,omitempty"`
	// +optional
	ControlPlane *ComponentStatus `json:"controlPlane,omitempty"`
	// +optional
	Console *ComponentStatus `json:"console,omitempty"`
	// +optional
	Ingress *ComponentStatus `json:"ingress,omitempty"`
	// +optional
	Gateway *ComponentStatus `json:"gateway,omitempty"`
	// +optional
	Identity *ComponentStatus `json:"identity,omitempty"`
	// +optional
	Runner *ComponentStatus `json:"runner,omitempty"`
	// +optional
	Tekton *ComponentStatus `json:"tekton,omitempty"`
}

// ComponentStatus describes the observed state of an individual component.
type ComponentStatus struct {
	// phase is the lifecycle phase of this component.
	Phase ComponentPhase `json:"phase"`

	// message provides human-readable detail about the component's current state.
	// +optional
	Message string `json:"message,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Phase",type=string,JSONPath=`.status.phase`,description="Deployment lifecycle phase"
// +kubebuilder:printcolumn:name="Version",type=string,JSONPath=`.status.version`,description="Deployed platform version"
// +kubebuilder:printcolumn:name="URL",type=string,JSONPath=`.status.consoleUrl`,description="Web console URL once ingress is admitted"
// +kubebuilder:printcolumn:name="Reachability",type=string,JSONPath=`.status.reachability`,description="Whether the public internet reaches the front door, as the operator concluded"
// +kubebuilder:printcolumn:name="License",type=string,JSONPath=`.status.license`,description="License delivery mode (Community when none configured)"
// +kubebuilder:printcolumn:name="Email",type=string,JSONPath=`.status.email`,description="Email provider as declared (NotConfigured when spec.email is absent)"
// +kubebuilder:printcolumn:name="Message",type=string,JSONPath=`.status.conditions[?(@.type=="Ready")].message`,description="Why the platform is in its phase, in plain language"
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`

// PlantonPlatform is the Schema for deploying a complete Planton platform
// on any Kubernetes cluster. A single PlantonPlatform resource orchestrates
// the full stack: data layer, supporting services, and application layer.
type PlantonPlatform struct {
	metav1.TypeMeta `json:",inline"`

	// +optional
	metav1.ObjectMeta `json:"metadata,omitzero"`

	// +required
	Spec PlantonPlatformSpec `json:"spec"`

	// +optional
	Status PlantonPlatformStatus `json:"status,omitzero"`
}

// +kubebuilder:object:root=true

// PlantonPlatformList contains a list of PlantonPlatform.
type PlantonPlatformList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitzero"`
	Items           []PlantonPlatform `json:"items"`
}

func init() {
	SchemeBuilder.Register(&PlantonPlatform{}, &PlantonPlatformList{})
}
