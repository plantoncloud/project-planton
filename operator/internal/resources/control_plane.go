package resources

import (
	"fmt"
	"strings"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

const (
	ControlPlaneDefaultImageRepo = "ghcr.io/plantonhq/planton/control-plane"
	controlPlaneContainerPort    = 8080
	controlPlaneServicePort      = 80
	// gRPC-Web listener for browser clients (the console). Serving it is opt-in
	// in the control plane -- setting GRPC_WEB_PORT is what turns it on -- and
	// this operator always opts in, because the console it deploys can only
	// speak gRPC-Web. The raw gRPC port stays reserved for CLI/runner/loopback.
	controlPlaneGrpcWebPort = 8081
	controlPlaneDebugPort   = 5005
	controlPlaneAppProtocol = "grpc"
	// The named ports the front doors reference: Ingress backends and the route
	// table point at ports by name so a number change never touches a door.
	controlPlaneGrpcPortName    = "grpc"
	controlPlaneGrpcWebPortName = "grpc-web"
	// The webhook servlet port: OIDC discovery + JWKS and every inbound
	// webhook. Matches the control plane's own default (WEBHOOK_PORT); set
	// explicitly in the env so the contract states it rather than relying on
	// the image's default, exactly as the runner's is.
	controlPlaneWebhookPort     = 8086
	controlPlaneWebhookPortName = "webhook"

	controlPlaneDefaultLogLevel          = "info"
	controlPlaneDefaultTemporalNamespace = "default"

	// controlPlaneModuleArtifactsVersion pins PLANTON_VERSION: the version at
	// which the control plane resolves IaC module artifacts from the public
	// CDN (downloads URL construction), NOT the platform image version. The
	// two release trains are independent -- a platform tag with no module
	// artifacts published under it would make every deploy 404 at download --
	// so this advances deliberately, when a verified artifact set exists.
	// The CR's spec.controlPlane.iacModulesVersion overrides this default
	// per install; the pin is the value every plain install must be able to
	// trust, so it only ever names a tag whose artifact set was verified
	// live against the CDN (HEAD on the module zips, not inferred from the
	// release existing).
	controlPlaneModuleArtifactsVersion = "v0.5.33"
)

// ControlPlaneConfig bundles all inputs needed to build the ControlPlane
// Deployment. Using a config struct avoids a massive function signature and
// keeps the builder testable with partial configurations.
type ControlPlaneConfig struct {
	CRName    string
	Namespace string
	Version   string
	OwnerRef  *metav1.OwnerReference

	Replicas                 int32
	ImageRepository          string
	ImageTag                 string
	ExternalConfigSecretName string

	// IacModulesVersion overrides controlPlaneModuleArtifactsVersion
	// (PLANTON_VERSION) when the CR sets spec.controlPlane.iacModulesVersion.
	// Empty means the compiled pin.
	IacModulesVersion string

	PostgreSQL PostgreSQLConnectionInfo
	Redis      RedisConnectionInfo
	OpenFGA    OpenFGAConnectionInfo
	Temporal   TemporalConnectionInfo

	Neo4j *Neo4jConnectionInfo

	// Identity wires the control plane as an OIDC relying party against the
	// bundled identity server. Always set by the component once the
	// front-door URL is known -- every install signs in.
	Identity *IdentityBinding

	// Runner, when set, activates the control plane's in-cluster runner boot
	// seeds (registration + credential hash + deploy defaults) and the badge
	// verification the in-cluster runner proves itself with. Nil when the
	// runner is disabled -- the seed properties stay unset and the arm stays
	// inert.
	Runner *RunnerBinding

	// RemoteRunners, when set, advertises this install's deploy queue and API
	// to runners enrolling from outside the cluster at addresses they can
	// reach (see RemoteRunnersBinding). Nil leaves the queue unadvertised, so
	// remote enrollment is refused honestly.
	RemoteRunners *RemoteRunnersBinding

	// Storage wires the object-storage capability onto the platform's own
	// Postgres (the planton.storage.provider seam's postgres arm): state-file
	// relay and log archives live in the database, and transfer URLs are
	// served by the control plane's own relay endpoint on the browser-API
	// port. Always set by the component once the front-door URL is known --
	// like Identity, it is never nil on a rendered Deployment, and no R2
	// placeholders exist anywhere in this install.
	Storage *StorageBinding

	// WebIdentity is the keyless identity issuer this install is: the front
	// door's URL and whether the clouds can trust it. Always set by the
	// component once the front-door URL is known (the issuer exists even when
	// keyless is closed -- the discovery document must still be honest about
	// the address it is served at).
	WebIdentity *WebIdentityBinding

	// GithubWebhooks is where GitHub delivers to this install and whether it
	// can. Always set alongside WebIdentity: the receiver is the door plus the
	// webhook namespace on every install; reachability is the door's.
	GithubWebhooks *GithubWebhooksBinding

	// Console is where this install's browser console is served -- the front
	// door -- for the control plane to hand out as the address a third party
	// sends a person's browser back to (a customer's own GitHub App points its
	// Setup URL and Callback URL at console pages here). Always set alongside
	// WebIdentity: the console and the issuer are the same door on a
	// self-hosted install.
	Console *ConsoleBinding

	// Vault wires the control plane to the deployed OpenBAO component. Nil
	// when the vault component is disabled -- then the pod carries
	// PLANTON_VAULT_ENABLED=false and NO vault address at all (present or
	// absent, never a placeholder: the control plane's vault consumers
	// degrade with plain language instead of dialing a dead address).
	Vault *VaultBinding

	// SecretBackend, when set, activates the control plane's default
	// secret-backend boot seed (planton.bootstrap.secret-backend.*). Nil
	// means no default backend is seeded and secret-dependent features
	// funnel in the console until one is created.
	SecretBackend *SecretBackendBinding

	// License, when set, delivers the license key (inline or by Secret
	// reference) as PLANTON_LICENSING_KEY. Nil means Community: the env var
	// is entirely absent, never empty.
	License *LicenseBinding

	// Email is the resolved spec.email (control_plane_email.go). Nil renders
	// PLANTON_EMAIL_PROVIDER=none -- said out loud, because the control
	// plane's seam reads an unset provider as the hosted arm. The component
	// resolves it purely and preflights every Secret it names first.
	Email *EmailBinding

	// Github is the resolved spec.github (control_plane_github.go): the
	// hosts, their install Apps, their webhook verdicts. Nil is the undeclared
	// install -- github.com, no install App, webhooks judged by the door --
	// which the facts file states out loud. The component resolves it,
	// preflights every App Secret, and writes the facts ConfigMap itself; the
	// Deployment only mounts.
	Github *GithubBinding

	// ServiceAccountAnnotations land on the control plane's dedicated
	// ServiceAccount -- the workload-identity seam for the platform's own
	// cloud calls (ambient secret backends + KMS KEKs).
	ServiceAccountAnnotations map[string]string
}

// VaultBinding carries what the control plane needs to reach the deployed
// OpenBAO. The root token rides a Secret reference (the operator-owned init
// Secret), never a literal: this vault is single-tenant and exists solely for
// this control plane, so the root token to its sole consumer is a deliberate
// trust call (a scoped periodic token would add a renewal lifecycle -- a new
// failure mode -- for no boundary gain; the unseal keys stay separate).
type VaultBinding struct {
	// APIAddr is the OpenBAO Service's in-cluster HTTP address.
	APIAddr string

	// InitSecretName is the operator's init Secret holding the root token.
	InitSecretName string

	// RootTokenKey is the token's key within the init Secret.
	RootTokenKey string
}

// LicenseBinding carries the resolved license-key delivery: Key for an
// inline spec value, SecretName+SecretKey for a Secret-backed one (exactly
// one group is populated -- the component resolver enforces it). The
// operator is deliberately dumb about the key itself: no verification, no
// shape check -- the control plane owns verification and answers with typed
// outcomes, so a bad key fails with a precise message there instead of a
// vague one here.
type LicenseBinding struct {
	// Key is the inline license key from the CR spec.
	Key string

	// SecretName/SecretKey reference the Secret entry holding the key.
	SecretName string
	SecretKey  string
}

// SecretBackendBinding is the resolved default-secret-backend seed
// configuration (addressing only -- never credentials; the ambient arm is
// what makes this honest to seed from configuration).
type SecretBackendBinding struct {
	// Type is the seed's declared kind on the Spring property surface:
	// "platform" or "aws-secrets-manager".
	Type string

	// AwsRegion configures the aws-secrets-manager kind:
	// the region secrets live in and the KMS key (ARN/id/alias) that
	// envelope-encrypts them, both reached with the pod's own identity.
	AwsRegion string
}

// StorageBinding carries the split-horizon base URLs relay transfer URLs are
// minted against -- the same two horizons the OIDC issuer uses: browsers and
// CLIs dereference at the advertised front-door address, in-cluster workloads
// (the IaC runner) at the control plane's own Service address.
type StorageBinding struct {
	// RelayPublicBaseURL is the advertised front-door base (ingress URL or
	// the gateway's localhost URL).
	RelayPublicBaseURL string

	// RelayInternalBaseURL is the control plane's in-cluster address on the
	// browser-API port, where the relay endpoint is mounted.
	RelayInternalBaseURL string
}

// WebIdentityBinding carries the keyless identity issuer's posture, derived
// ONCE by the component from the front door (its URL, its scheme, its
// declared reachability, the vault). The control plane mints tokens naming
// IssuerURL and serves discovery there; the connection-method catalog reads
// Offered and, when closed, ClosedReason -- the sentence the wizard shows on
// the keyless card, naming the one fact that closed the door.
type WebIdentityBinding struct {
	// IssuerURL is the front door's origin: the iss claim, the discovery
	// document's issuer, and the address the clouds fetch keys from.
	IssuerURL string

	// Offered is whether keyless connections are advertised and accepted on
	// this install: the door is public, HTTPS, and the vault runs.
	Offered bool

	// ClosedReason is the plain-language reason when Offered is false; empty
	// when offered. Rendered as the mode's reason so the catalog shows this
	// install's sentence instead of its generic fallback.
	ClosedReason string
}

// GithubWebhooksBinding carries GitHub webhook delivery posture: whether GitHub
// can reach this install and the URL it would deliver to. Both derive from the
// front door -- the receiver is the door plus the webhook namespace, and it is
// reachable exactly when the door is.
type GithubWebhooksBinding struct {
	// Reachable is whether GitHub can deliver to the receiver: the door is
	// reachable from the public internet. False turns every GitHub method's
	// card to "Planton checks GitHub for pushes instead".
	Reachable bool

	// ReceiverURL is the door plus the webhook namespace plus the GitHub
	// path -- true on every install, reachable only on a public one; never a
	// placeholder.
	ReceiverURL string
}

// ConsoleBinding carries the origin the install's browser console is served
// at: the front door, whatever its shape. On a port-forward door this is the
// loopback address the person's own browser reaches -- still true, because a
// third party only ever redirects the BROWSER there, never calls it. Hosted
// Planton pins its console; a desktop's local instance declares none (the
// desktop application is its console), so the variable is never a placeholder.
type ConsoleBinding struct {
	// URL is the console's origin, no path.
	URL string
}

// RunnerBinding carries what the control plane needs to seed the in-cluster
// runner at boot. No enrollment credential rides it: the runner's proof is
// its projected ServiceAccount badge, and the seeded registration's declared
// workload identity is what provisions the identity the badge resolves to.
type RunnerBinding struct {
	// CloudOpsSecretName is the Secret whose cloudops-auth-token entry is
	// the direct-dial bearer (RUNNER_DIRECT_AUTH_TOKEN) -- referenced by
	// name so its plaintext never lands in the pod spec, and read from the
	// SAME key the runner consumes so the two sides cannot disagree.
	CloudOpsSecretName string

	// Provisioner is the org's default IaC provisioner ("tofu"/"terraform").
	Provisioner string

	// DirectDialHost is the runner Service DNS name CloudOps dials directly
	// for live cloud operations (RUNNER_DIRECT_HOST).
	DirectDialHost string

	// BuildEnabled activates the build-routing boot seed: the control plane
	// creates this install's build-cluster connection (create-once, pointing
	// at the in-cluster runner) and the platform-scoped default referencing
	// it, so the first service pipeline resolves a build destination with
	// zero registration ceremony. Follows the effective build toggle
	// (spec.build AND spec.runner).
	BuildEnabled bool
}

// RemoteRunnersBinding is what the install advertises to runners that enroll
// from OUTSIDE the cluster (developer laptops, appliances in other networks):
// the two addresses stamped into their identity documents. Present exactly
// when the remote-runners capability is on AND the front door carries it;
// nil otherwise, which leaves the deploy-queue advertisement UNSET so the
// control plane refuses remote enrollment with the reason instead of minting
// an address only this cluster's pods resolve. The in-cluster runner never
// reads these: the operator renders its identity document itself, with the
// in-cluster addresses.
type RemoteRunnersBinding struct {
	// PlantonAPIEndpoint is the control plane's native gRPC address as a
	// runner outside the cluster dials it (host:port; :443 means TLS) -- the
	// front door's gRPC endpoint.
	PlantonAPIEndpoint string
	// TemporalEndpoint is the deploy queue's address as a runner outside the
	// cluster dials it -- the same front door, which routes the queue's
	// workflow service beside the API.
	TemporalEndpoint string
}

// IdentityBinding carries what the control plane needs to validate browser
// tokens from the bundled identity server.
type IdentityBinding struct {
	// IssuerURL is the exact OIDC issuer (front-door URL + identity path +
	// realm) -- the ADVERTISED horizon that tokens carry in their iss claim.
	IssuerURL string

	// InternalIssuerURL is the same issuer at its in-cluster Service address
	// -- the FETCH horizon. Discovery, JWKS, token, and userinfo requests go
	// here (the identity server's dynamic backchannel returns in-cluster
	// endpoint URLs), so the control plane never dials the advertised URL.
	InternalIssuerURL string

	// Hostname is the platform's public hostname (no scheme).
	Hostname string

	// UsersClientSecretName is the Secret holding the user-directory client's
	// secret (key IdentityOIDCClientSecretKey) -- the least-privilege
	// credential the control plane uses to drive user lifecycle in the
	// bundled identity server (first-run admin creation, later invitations).
	UsersClientSecretName string

	// SetupCodeSecretName, when non-empty, activates first-run setup mode:
	// no admin was declared, so the console's setup page (backed by the
	// control plane's public setup RPC) creates the first admin. The Secret's
	// setup-code entry is the cluster-access proof the page demands.
	SetupCodeSecretName string

	// SetupCodeHint is the human-readable command for reading the setup code,
	// passed through the control plane to the setup page so no UI hardcodes
	// deployment names. Set exactly when SetupCodeSecretName is.
	SetupCodeHint string

	// AuthorizationProvider selects the control plane's granular-authorization
	// arm (the PLANTON_AUTHORIZATION_PROVIDER seam): "allow-authenticated" for
	// the trusting-team default that runs no policy engine, "openfga" when the
	// authorization component is enabled and wired. Only meaningful with a
	// real issuer -- the local arm implies allow-owner and never sets it.
	AuthorizationProvider string

	// Bootstrap carries the config-driven first-boot seeds the control plane
	// consumes as planton.bootstrap.* properties: the default org + starter
	// environment, and the declared admins.
	Bootstrap BootstrapBinding
}

// BootstrapBinding is the resolved (defaulted) first-boot seed configuration.
type BootstrapBinding struct {
	OrgSlug string
	OrgName string
	EnvSlug string
	EnvName string
	// Admins are the declared admin emails; joined comma-separated into
	// PLANTON_BOOTSTRAP_ADMINS (Spring's relaxed binding splits it back).
	Admins []string
}

// ControlPlaneDeploymentName returns the Deployment name: "{crName}-control-plane".
func ControlPlaneDeploymentName(crName string) string {
	return fmt.Sprintf("%s-control-plane", crName)
}

// ControlPlaneServiceAccountName returns the dedicated ServiceAccount name:
// "{crName}-control-plane".
func ControlPlaneServiceAccountName(crName string) string {
	return fmt.Sprintf("%s-control-plane", crName)
}

// ControlPlaneServiceAccount builds the control plane's dedicated
// ServiceAccount. Created even when no annotations are declared, so granting
// the platform a cloud identity later (workload identity for ambient secret
// backends and their KMS keys) is a pure annotation edit -- the same seam the
// runner established for the deploy worker's identity.
func ControlPlaneServiceAccount(cfg ControlPlaneConfig) *corev1.ServiceAccount {
	sa := &corev1.ServiceAccount{
		TypeMeta: metav1.TypeMeta{APIVersion: "v1", Kind: "ServiceAccount"},
		ObjectMeta: metav1.ObjectMeta{
			Name:        ControlPlaneServiceAccountName(cfg.CRName),
			Namespace:   cfg.Namespace,
			Labels:      controlPlaneComponentLabels(cfg.CRName),
			Annotations: cfg.ServiceAccountAnnotations,
		},
	}
	if cfg.OwnerRef != nil {
		sa.OwnerReferences = []metav1.OwnerReference{*cfg.OwnerRef}
	}
	return sa
}

// ControlPlaneTokenReviewerClusterRoleName returns the cluster-scoped name of
// the control plane's badge-verification grant:
// "{namespace}-{crName}-control-plane-token-reviewer". The namespace is part
// of the name because the object is cluster-scoped while platforms are
// namespaced: same-named platforms in different namespaces must never share
// it -- a shared binding force-applied by both reconciles would flap its
// subject between the two control planes, crash-looping whichever one lost
// the last write.
func ControlPlaneTokenReviewerClusterRoleName(namespace, crName string) string {
	return fmt.Sprintf("%s-%s-control-plane-token-reviewer", namespace, crName)
}

// ControlPlaneTokenReviewerClusterRole grants the control plane the cluster's
// badge-verification power: TokenReview create (verifying runners' projected
// ServiceAccount tokens with the cluster itself) plus SelfSubjectAccessReview
// create (the kubernetes-auth arm's boot probe verifies it holds the grant
// and fails startup loudly when it does not). This mirrors the hosted
// deployment's authored token-reviewer manifest -- one grant shape for every
// badge-verifying control plane.
//
// Cluster-scoped objects cannot carry a namespaced owner reference, so this
// pair is not garbage-collected with the CR. Instead it carries the owning
// platform's UID as a label (PlatformUIDLabel), and the operator's janitor
// deletes any pair whose UID names no platform still on the cluster. The UID,
// not the name: a platform deleted and recreated under the same name a moment
// later must never lose the grant its new reconcile just applied. Until the
// janitor runs, the orphan is INERT by construction: the binding's only
// subject is the control plane's namespaced ServiceAccount, which dies with
// the namespace -- a leftover grant grants nothing to nobody.
func ControlPlaneTokenReviewerClusterRole(cfg ControlPlaneConfig) *rbacv1.ClusterRole {
	return &rbacv1.ClusterRole{
		TypeMeta: metav1.TypeMeta{APIVersion: "rbac.authorization.k8s.io/v1", Kind: "ClusterRole"},
		ObjectMeta: metav1.ObjectMeta{
			Name:   ControlPlaneTokenReviewerClusterRoleName(cfg.Namespace, cfg.CRName),
			Labels: platformSatelliteLabels(cfg),
		},
		Rules: []rbacv1.PolicyRule{
			{
				APIGroups: []string{"authentication.k8s.io"},
				Resources: []string{"tokenreviews"},
				Verbs:     []string{"create"},
			},
			{
				APIGroups: []string{"authorization.k8s.io"},
				Resources: []string{"selfsubjectaccessreviews"},
				Verbs:     []string{"create"},
			},
		},
	}
}

// ControlPlaneTokenReviewerClusterRoleBinding binds the badge-verification
// grant to the control plane's dedicated ServiceAccount -- deliberately never
// a namespace default, so no co-located workload inherits verification power.
func ControlPlaneTokenReviewerClusterRoleBinding(cfg ControlPlaneConfig) *rbacv1.ClusterRoleBinding {
	return &rbacv1.ClusterRoleBinding{
		TypeMeta: metav1.TypeMeta{APIVersion: "rbac.authorization.k8s.io/v1", Kind: "ClusterRoleBinding"},
		ObjectMeta: metav1.ObjectMeta{
			Name:   ControlPlaneTokenReviewerClusterRoleName(cfg.Namespace, cfg.CRName),
			Labels: platformSatelliteLabels(cfg),
		},
		RoleRef: rbacv1.RoleRef{
			APIGroup: "rbac.authorization.k8s.io",
			Kind:     "ClusterRole",
			Name:     ControlPlaneTokenReviewerClusterRoleName(cfg.Namespace, cfg.CRName),
		},
		Subjects: []rbacv1.Subject{{
			Kind:      "ServiceAccount",
			Name:      ControlPlaneServiceAccountName(cfg.CRName),
			Namespace: cfg.Namespace,
		}},
	}
}

// PlatformUIDLabel names the PlantonPlatform a cluster-scoped satellite
// belongs to. A namespaced owner cannot garbage-collect a cluster-scoped
// dependent, so the platform's UID rides as a label instead and the janitor
// reads it: a satellite whose UID names no live platform is removed.
const PlatformUIDLabel = "planton.ai/platform-uid"

// platformSatelliteLabels are the control plane's component labels plus the
// owning platform's UID -- the label set every cluster-scoped object the
// platform needs must carry.
func platformSatelliteLabels(cfg ControlPlaneConfig) map[string]string {
	labels := controlPlaneComponentLabels(cfg.CRName)
	if cfg.OwnerRef != nil {
		labels[PlatformUIDLabel] = string(cfg.OwnerRef.UID)
	}
	return labels
}

func controlPlaneComponentLabels(crName string) map[string]string {
	return map[string]string{
		"app.kubernetes.io/name":       "control-plane",
		"app.kubernetes.io/instance":   crName,
		"app.kubernetes.io/managed-by": ManagedByLabel,
		"app.kubernetes.io/component":  "application",
	}
}

// ControlPlaneServiceName returns the Service name: "{crName}-control-plane".
func ControlPlaneServiceName(crName string) string {
	return fmt.Sprintf("%s-control-plane", crName)
}

// ControlPlaneServiceFQDN returns the in-cluster FQDN for the control plane.
func ControlPlaneServiceFQDN(crName, namespace string) string {
	return fmt.Sprintf("%s.%s.svc.cluster.local", ControlPlaneServiceName(crName), namespace)
}

// ControlPlaneRelayInternalBaseURL returns the in-cluster base URL for the
// storage relay: the control plane's Service on the browser-API port, where
// the relay endpoint is mounted alongside gRPC-Web.
func ControlPlaneRelayInternalBaseURL(crName, namespace string) string {
	return fmt.Sprintf("http://%s:%d", ControlPlaneServiceFQDN(crName, namespace), controlPlaneGrpcWebPort)
}

// ControlPlaneDeployment builds the Kubernetes Deployment for the control plane
// monolith with all internal environment variables wired to the operator-managed
// data layer and supporting services.
func ControlPlaneDeployment(cfg ControlPlaneConfig) *appsv1.Deployment {
	imageRepo := cfg.ImageRepository
	if imageRepo == "" {
		imageRepo = ControlPlaneDefaultImageRepo
	}
	imageTag := cfg.ImageTag
	if imageTag == "" {
		imageTag = cfg.Version
	}
	replicas := cfg.Replicas
	if replicas <= 0 {
		replicas = 1
	}

	labels := map[string]string{
		"app.kubernetes.io/name":       "control-plane",
		"app.kubernetes.io/instance":   cfg.CRName,
		"app.kubernetes.io/managed-by": ManagedByLabel,
		"app.kubernetes.io/component":  "application",
	}

	envVars := controlPlaneEnvVars(cfg)

	envFrom := controlPlaneEnvFrom(cfg)

	// The identity component publishes federation facts (arm + verification
	// verdicts from the bound identity manifest) as a ConfigMap the identity
	// component ensures exists on every install BEFORE this Deployment
	// renders (controlplane depends on identity). Mounted as a volume --
	// never env -- so kubelet updates the content in place and a facts
	// change NEVER rolls this pod; whole-directory mount, never subPath
	// (subPath mounts freeze at pod start). Optional is belt-and-braces only.
	volumes := []corev1.Volume{{
		Name: "identity-federation-facts",
		VolumeSource: corev1.VolumeSource{
			ConfigMap: &corev1.ConfigMapVolumeSource{
				LocalObjectReference: corev1.LocalObjectReference{
					Name: IdentityFederationFactsConfigMapName(cfg.CRName),
				},
				Optional: new(true),
			},
		},
	}}
	volumeMounts := []corev1.VolumeMount{{
		Name:      "identity-federation-facts",
		MountPath: IdentityFederationFactsMountPath,
		ReadOnly:  true,
	}}

	// The email credentials volume follows the same live-update shape: every
	// secret value spec.email references is a projected file, present only
	// when a Secret is referenced, so a rotated relay password is live on the
	// next send without a pod roll.
	if volume, mount := emailCredentialsVolume(cfg.Email); volume != nil {
		volumes = append(volumes, *volume)
		volumeMounts = append(volumeMounts, *mount)
	}

	// The GitHub facts file rides the identity-federation shape (mounted on
	// every install, updated in place); the App credentials ride the email
	// shape (projected files, present only when an App is declared).
	factsVolume, factsMount := githubFactsVolume(cfg.CRName)
	volumes = append(volumes, factsVolume)
	volumeMounts = append(volumeMounts, factsMount)
	if volume, mount := githubCredentialsVolume(cfg.Github); volume != nil {
		volumes = append(volumes, *volume)
		volumeMounts = append(volumeMounts, *mount)
	}

	deploy := &appsv1.Deployment{
		TypeMeta: metav1.TypeMeta{APIVersion: "apps/v1", Kind: "Deployment"},
		ObjectMeta: metav1.ObjectMeta{
			Name:      ControlPlaneDeploymentName(cfg.CRName),
			Namespace: cfg.Namespace,
			Labels:    labels,
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: &replicas,
			Selector: &metav1.LabelSelector{MatchLabels: labels},
			Strategy: appsv1.DeploymentStrategy{
				Type: appsv1.RollingUpdateDeploymentStrategyType,
				RollingUpdate: &appsv1.RollingUpdateDeployment{
					MaxSurge:       intOrStr(1),
					MaxUnavailable: intOrStr(0),
				},
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{Labels: labels},
				Spec: corev1.PodSpec{
					ServiceAccountName: ControlPlaneServiceAccountName(cfg.CRName),
					Volumes:            volumes,
					Containers: []corev1.Container{{
						Name:  "control-plane",
						Image: fmt.Sprintf("%s:%s", imageRepo, imageTag),
						Ports: []corev1.ContainerPort{
							{Name: controlPlaneGrpcPortName, ContainerPort: controlPlaneContainerPort, Protocol: corev1.ProtocolTCP},
							{Name: controlPlaneGrpcWebPortName, ContainerPort: controlPlaneGrpcWebPort, Protocol: corev1.ProtocolTCP},
							{Name: controlPlaneWebhookPortName, ContainerPort: controlPlaneWebhookPort, Protocol: corev1.ProtocolTCP},
							{Name: "debug", ContainerPort: controlPlaneDebugPort, Protocol: corev1.ProtocolTCP},
						},
						VolumeMounts: volumeMounts,
						Env:          envVars,
						EnvFrom:      envFrom,
						// First boot self-provisions and migrates every database, which
						// on a cold cluster takes several minutes; allow a generous
						// window (10s x 90 = 15m) before the kubelet gives up, so the
						// JVM is never killed mid-migration. Readiness/liveness take
						// over with tight thresholds once startup succeeds.
						StartupProbe: &corev1.Probe{
							ProbeHandler: corev1.ProbeHandler{
								GRPC: &corev1.GRPCAction{Port: controlPlaneContainerPort},
							},
							InitialDelaySeconds: 15,
							PeriodSeconds:       10,
							TimeoutSeconds:      5,
							FailureThreshold:    90,
						},
						ReadinessProbe: &corev1.Probe{
							ProbeHandler: corev1.ProbeHandler{
								GRPC: &corev1.GRPCAction{Port: controlPlaneContainerPort},
							},
							InitialDelaySeconds: 10,
							PeriodSeconds:       10,
							TimeoutSeconds:      5,
							FailureThreshold:    3,
						},
						LivenessProbe: &corev1.Probe{
							ProbeHandler: corev1.ProbeHandler{
								GRPC: &corev1.GRPCAction{Port: controlPlaneContainerPort},
							},
							InitialDelaySeconds: 30,
							PeriodSeconds:       30,
							TimeoutSeconds:      5,
							FailureThreshold:    3,
						},
					}},
				},
			},
		},
	}

	if cfg.OwnerRef != nil {
		deploy.OwnerReferences = []metav1.OwnerReference{*cfg.OwnerRef}
	}

	return deploy
}

// ControlPlaneService builds the ClusterIP Service exposing the control plane
// gRPC API on port 80 (mapped to container port 8080) and the browser-facing
// gRPC-Web endpoint on its own port.
func ControlPlaneService(crName, namespace string, ownerRef *metav1.OwnerReference) *corev1.Service {
	labels := map[string]string{
		"app.kubernetes.io/name":       "control-plane",
		"app.kubernetes.io/instance":   crName,
		"app.kubernetes.io/managed-by": ManagedByLabel,
		"app.kubernetes.io/component":  "application",
	}

	svc := &corev1.Service{
		TypeMeta: metav1.TypeMeta{APIVersion: "v1", Kind: "Service"},
		ObjectMeta: metav1.ObjectMeta{
			Name:      ControlPlaneServiceName(crName),
			Namespace: namespace,
			Labels:    labels,
		},
		Spec: corev1.ServiceSpec{
			Type:     corev1.ServiceTypeClusterIP,
			Selector: labels,
			Ports: []corev1.ServicePort{
				{
					Name:        controlPlaneGrpcPortName,
					Port:        controlPlaneServicePort,
					TargetPort:  intstr.FromInt32(controlPlaneContainerPort),
					Protocol:    corev1.ProtocolTCP,
					AppProtocol: new(controlPlaneAppProtocol),
				},
				// gRPC-Web rides plain HTTP/1.1 (or h2) -- appProtocol http, so
				// ingress controllers route it like ordinary web traffic.
				{
					Name:        controlPlaneGrpcWebPortName,
					Port:        controlPlaneGrpcWebPort,
					TargetPort:  intstr.FromInt32(controlPlaneGrpcWebPort),
					Protocol:    corev1.ProtocolTCP,
					AppProtocol: new("http"),
				},
				// The webhook servlet: the control plane's public unauthenticated
				// HTTP surface (OIDC discovery + JWKS, signature-verified
				// webhooks). Plain HTTP/1.1; the front door routes the issuer's
				// two discovery paths and the webhook namespace here.
				{
					Name:        controlPlaneWebhookPortName,
					Port:        controlPlaneWebhookPort,
					TargetPort:  intstr.FromInt32(controlPlaneWebhookPort),
					Protocol:    corev1.ProtocolTCP,
					AppProtocol: new("http"),
				},
			},
		},
	}

	if ownerRef != nil {
		svc.OwnerReferences = []metav1.OwnerReference{*ownerRef}
	}

	return svc
}

// controlPlaneEnvVars builds the control-plane boot contract.
//
// This is the Kubernetes provider of the SAME contract the desktop daemon boots
// against (its local boot environment file). The operator is
// the only consumer of the control-plane image, so it owns the full contract here
// -- legible in the Deployment env (`kubectl describe`), with no opaque external
// config Secret. Every value mirrors the daemon's proven-to-boot contract, except:
//   - datastore hosts/ports/credentials point at the operator-managed services
//     (service DNS + generated Secrets) instead of localhost + trust;
//   - the runner endpoints are the in-pod loopback on the container port;
//   - local-only single-runner wiring is off (the runner is a separate component).
//
// The minimal footprint runs the lightweight built-in capabilities (search on the
// Postgres projection; estate indexing without Neo4j) and the local allow-owner
// authorization arm (no OpenFGA). Not-yet-graduated integrations carry the same honest, marked
// placeholders the daemon uses; the corresponding clients are lazy or gated, so
// the context binds without a real credential. Stigmer and object storage are the
// two that validate eagerly -- they are made genuinely optional in the control
// plane so even their placeholders fall away.
//
// IMPORTANT: this and local.env are two providers of one contract. Until they are
// unified behind a shared definition, a change to the control-plane's required
// config (a new stream, queue, database, or property) must be reflected in BOTH.
func controlPlaneEnvVars(cfg ControlPlaneConfig) []corev1.EnvVar {
	// Estate indexing defaults to the lightweight built-in Postgres provider.
	// Enabling the Neo4j component switches the provider and wires the
	// component's connection env below -- the capability, chosen once per install.
	estateProvider := "postgres"
	if cfg.Neo4j != nil {
		estateProvider = "neo4j"
	}

	envs := identityEnvVars(cfg.Identity)

	envs = append(envs, []corev1.EnvVar{
		// ── deployment shape ──
		// A declared fact, never inferred: the control plane refuses to boot
		// without it, and the operator's installs are customer clusters by
		// definition. Selects the self-hosted entitlement semantics (license
		// enforcement) under every write.
		{Name: "PLANTON_DEPLOYMENT_KIND", Value: DeploymentKindSelfHosted},

		// ── gRPC server ──
		{Name: "PORT", Value: fmt.Sprintf("%d", controlPlaneContainerPort)},
		// Setting this is the opt-in that makes the monolith serve gRPC-Web for
		// the browser console; unset, the app runs no gRPC-Web listener at all.
		{Name: "GRPC_WEB_PORT", Value: fmt.Sprintf("%d", controlPlaneGrpcWebPort)},

		// ── application / observability (off) ──
		{Name: "ENV", Value: cfg.CRName},
		{Name: "SERVICE_NAME", Value: "control-plane"},
		{Name: "DEPLOYMENT_VERSION", Value: cfg.Version},
		{Name: "LOG_LEVEL", Value: controlPlaneDefaultLogLevel},
		{Name: "OBSERVABILITY_ENABLED", Value: "false"},
		{Name: "OTEL_EXPORTER_OTLP_ENDPOINT", Value: "http://localhost:4317"},
		{Name: "OTEL_EXPORTER_OTLP_TRANSPORT", Value: "grpc"},

		// ── Postgres (operator-managed; the fat-jar self-provisions databases) ──
		{Name: "DB_HOST", Value: cfg.PostgreSQL.Host},
		{Name: "DB_NAME", Value: DBBase},
		{Name: "DB_USERNAME", Value: cfg.PostgreSQL.User},
		secretEnv("DB_PASSWORD", cfg.PostgreSQL.SecretName, cfg.PostgreSQL.PassKey),

		// ── Redis (one instance serves both cache groups) ──
		{Name: "REDIS_CLUSTER_HOSTNAME", Value: cfg.Redis.Host},
		{Name: "REDIS_CLUSTER_PORT", Value: fmt.Sprintf("%d", cfg.Redis.Port)},
		secretEnv("REDIS_CLUSTER_PASSWORD", cfg.Redis.SecretName, cfg.Redis.PassKey),
		{Name: "TEKTON_LOGS_REDIS_HOSTNAME", Value: cfg.Redis.Host},
		{Name: "TEKTON_LOGS_REDIS_PORT", Value: fmt.Sprintf("%d", cfg.Redis.Port)},
		secretEnv("TEKTON_LOGS_REDIS_PASSWORD", cfg.Redis.SecretName, cfg.Redis.PassKey),

		// ── Temporal ──
		{Name: "TEMPORAL_SERVICE_ADDRESS", Value: cfg.Temporal.FrontendEndpoint},
		{Name: "TEMPORAL_NAMESPACE", Value: controlPlaneDefaultTemporalNamespace},
		{Name: "TEMPORAL_TASK_QUEUE_AWS_CLOUDFORMATION_SETUP", Value: "aws-cloudformation-setup"},
		{Name: "TEMPORAL_TASK_QUEUE_BILLING_CLEANUP", Value: "billing-cleanup"},
		{Name: "TEMPORAL_TASK_QUEUE_CLOUD_RESOURCE_PURGE", Value: "cloud-resource-purge"},
		{Name: "TEMPORAL_TASK_QUEUE_GIT_WEBHOOKS", Value: "git-webhooks"},
		{Name: "TEMPORAL_TASK_QUEUE_INFRA_HUB_CLEANUP", Value: "infra-hub-cleanup"},
		{Name: "TEMPORAL_TASK_QUEUE_INFRA_PIPELINE_BUILD_STAGE", Value: "infra-pipeline-build-stage"},
		{Name: "TEMPORAL_TASK_QUEUE_INFRA_PIPELINE_DEPLOY_STAGE", Value: "infra-pipeline-deploy-stage"},
		{Name: "TEMPORAL_TASK_QUEUE_INFRA_PROJECT_GIT_COMMIT", Value: "infra-project-git-commit"},
		{Name: "TEMPORAL_TASK_QUEUE_INFRA_PROJECT_PURGE", Value: "infra-project-purge"},
		{Name: "TEMPORAL_TASK_QUEUE_ORGANIZATION_ESTATE_REINDEX", Value: "estate-organization-reindex"},
		{Name: "TEMPORAL_TASK_QUEUE_PROVIDER_CONNECTION_AUTHORIZATION", Value: "provider_connection_authorization"},
		{Name: "TEMPORAL_TASK_QUEUE_RESOURCE_MANAGER_CLEANUP", Value: "resource-manager-cleanup"},
		{Name: "TEMPORAL_TASK_QUEUE_SERVICE_PIPELINE_BUILD_STAGE", Value: "service-pipeline-build-stage"},
		{Name: "TEMPORAL_TASK_QUEUE_SERVICE_PIPELINE_DEPLOY_STAGE", Value: "service-pipeline-deploy-stage"},
		{Name: "TEMPORAL_TASK_QUEUE_SERVICE_CLEANUP", Value: "service-cleanup"},
		{Name: "TEMPORAL_TASK_QUEUE_STACK_JOB", Value: "stack-job"},
		{Name: "TEMPORAL_TASK_QUEUE_TEKTON_CONNECTION_VERIFY", Value: "tekton-connection-verify"},
		{Name: "TEMPORAL_TASK_QUEUE_STACK_JOB_IAC_OPERATION", Value: "stack-job-iac-operation"},
		{Name: "TEMPORAL_TASK_QUEUE_STATE_BACKEND_MIGRATION", Value: "state-backend-migration"},
		{Name: "TEMPORAL_TASK_QUEUE_STORED_DOCUMENT_MIGRATION", Value: "stored-document-migration"},
		// Self-hosted installs upgrade without a platform operator watching:
		// stored-document migrations start automatically at boot when a release
		// changes storage versions.
		{Name: "PLANTON_INFRA_HUB_STORED_DOCUMENT_MIGRATION_AUTO_RUN", Value: "true"},
		// Derived from the bootstrap org -- the SAME derivation the runner
		// resources use for the worker's queue, so dispatcher and poller
		// cannot drift apart on a renamed org.
		{Name: "TEMPORAL_PLATFORM_RUNNER_TASK_QUEUE_AWS", Value: RunnerTaskQueue(cfg.CRName, cfg.Identity.Bootstrap.OrgSlug)},
		{Name: "TEMPORAL_PLATFORM_RUNNER_TASK_QUEUE_DEFAULT", Value: RunnerTaskQueue(cfg.CRName, cfg.Identity.Bootstrap.OrgSlug)},

		// Auth0-path FGA bindings: never used with the bundled identity
		// server (they serve the auth0 provider only) but part of the
		// fail-fast boot contract, so they bind with inert placeholders.
		{Name: "AUTH0_FGA_API_ENDPOINT", Value: "http://localhost:8088"},
		{Name: "AUTH0_FGA_STORE_ID", Value: "local"},
		{Name: "AUTH0_FGA_MODEL_ID", Value: "local"},

		// ── estate: built-in Postgres by default, or the opt-in Neo4j component ──
		{Name: "PLANTON_ESTATE_PROVIDER", Value: estateProvider},

		// ── oidc issuer: the token lifetime; the issuer itself is a binding ──
		{Name: "OIDC_TOKEN_TTL_SECONDS", Value: "900"},
		{Name: "WEBHOOK_PORT", Value: fmt.Sprintf("%d", controlPlaneWebhookPort)},

		// ── GitHub app (connect): no self-hosted install carries Planton's App ──
		// The credentials are placeholders that keep the beans booting and are
		// never read for meaning; the posture is declared beside them, with the
		// sentence a server's person can act on (the catalog's generic copy
		// offers "the sign-in on this machine", which a server does not have).
		// The platform release the floor admits still reads its GitHub
		// posture through these one-host variables. They are fed from the
		// facts' github.com entry where the declaration can be honored
		// through env (the webhook verdict, host login) and stay at their
		// no-App values where it cannot (an App key is a mounted PEM file,
		// which this env contract has no shape for). When the platform reads
		// the facts file, these variables and githubLegacyEnvVars leave
		// together with the floor.
		{Name: "GITHUB_APP_CLIENT_ID", Value: "local"},
		{Name: "GITHUB_APP_PRIVATE_KEY_BASE64", Value: "ZHVtbXk="},
		{Name: "PLANTON_CONNECT_METHODAVAILABILITY_PLATFORMAPP_AVAILABILITY", Value: "unavailable"},
		{Name: "PLANTON_CONNECT_METHODAVAILABILITY_PLATFORMAPP_REASON", Value: PlatformAppUnavailableReason},
		{Name: "GITHUB_BUILD_STAGE_CHECK_NAME", Value: "build"},
		{Name: "GITHUB_WEBHOOKS_SECRET_TOKEN", Value: "local"},

		// ── cloud oauth (connect): no self-hosted install carries Planton's apps ──
		// Each cloud's sign-in and one-click keyless setup follow that cloud's
		// OAuth app's own enabled flag; the credentials below are placeholders
		// that keep the beans booting and are never read for meaning.
		{Name: "GCP_OAUTH_ENABLED", Value: "false"},
		{Name: "AZURE_OAUTH_ENABLED", Value: "false"},
		{Name: "AZURE_OAUTH_CLIENT_ID", Value: "local"},
		{Name: "AZURE_OAUTH_CLIENT_SECRET", Value: "local"},
		{Name: "AZURE_OAUTH_HMAC_SECRET_KEY", Value: "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"},
		{Name: "AZURE_OAUTH_SCOPES", Value: "https://management.azure.com/user_impersonation offline_access openid profile"},
		{Name: "AZURE_OAUTH_SESSION_TTL_MINUTES", Value: "10"},
		{Name: "GCP_OAUTH_CLIENT_ID", Value: "local"},
		{Name: "GCP_OAUTH_CLIENT_SECRET", Value: "local"},
		{Name: "GCP_OAUTH_HMAC_SECRET_KEY", Value: "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"},
		{Name: "GCP_OAUTH_SCOPES", Value: "openid email profile https://www.googleapis.com/auth/cloud-platform"},
		{Name: "GCP_OAUTH_SESSION_TTL_MINUTES", Value: "10"},

		// ── aws browser setup (connect) ──
		// The browser-based CloudFormation quick-create flow depends on platform-side
		// integrations (a hosted callback webhook, hosted templates) this deployment
		// does not run: declared off, and the integration env is omitted entirely
		// (the config's own defaults absorb the absent bindings). Keyless itself is
		// NOT declared here: it is a fact about the front door, rendered from the
		// WebIdentity binding below.
		{Name: "AWS_CLOUDFORMATION_ENABLED", Value: "false"},
		{Name: "CLOUD_ACCOUNT_GCP_CUSTOMER_SERVICE_ACCOUNTS_PROJECT_ID", Value: "local"},
		{Name: "CLOUD_ACCOUNT_GCP_CUSTOMER_SERVICE_ACCOUNTS_PROJECT_NUMBER", Value: "0"},

		// ── connect runner enrollment + tunnel posture ──
		// This install operates NO runner tunnel (the tunnel exists for live
		// cloud operations across networks; the one runner this install
		// ships shares the control plane's, so CloudOps reaches it by DIRECT
		// dial -- the RUNNER_DIRECT_* arm below). CONNECT_RUNNER_TUNNEL_ENDPOINT
		// is therefore deliberately absent, which is the control plane's
		// declared tunnel-less posture: identity documents mint WITHOUT
		// tunnel material, and none of the CA issuance configuration exists
		// here. The API address stamped into enrolling runners' documents
		// (the connect domain's two-address contract, remote beside
		// platform) is the front door's gRPC endpoint when remote runners are
		// open -- the address a laptop dials -- and the in-cluster Service
		// otherwise (the variable is boot-required, and no remote runner is
		// admitted without the queue advertisement below anyway). The
		// platform-scoped address is always the in-cluster Service.
		{Name: "CONNECT_RUNNER_PLANTON_API_ENDPOINT", Value: remoteRunnerAPIEndpoint(cfg)},
		{Name: "CONNECT_RUNNER_PLATFORM_PLANTON_API_ENDPOINT", Value: fmt.Sprintf("%s:%d",
			ControlPlaneServiceFQDN(cfg.CRName, cfg.Namespace), controlPlaneServicePort)},
		{Name: "RUNNER_HOSTNAME_SUFFIX", Value: "local"},
		{Name: "RUNNER_TARGET_PORT", Value: "50051"},
		{Name: "TUNNEL_ENABLED", Value: "false"},
		{Name: "TUNNEL_TLS_ENABLED", Value: "false"},
		{Name: "TUNNEL_CHANNEL_CACHE_TTL", Value: "600"},
		{Name: "TUNNEL_CHANNEL_IDLE_TIMEOUT", Value: "300"},
		{Name: "TUNNEL_SERVER_HOST", Value: "localhost"},
		{Name: "TUNNEL_SERVER_PORT", Value: "7080"},
		{Name: "TUNNEL_TLS_CA_CERT_PATH", Value: ""},
		{Name: "TUNNEL_TLS_CLIENT_CERT_PATH", Value: ""},
		{Name: "TUNNEL_TLS_CLIENT_KEY_PATH", Value: ""},

		// ── tekton build workspaces ──
		// No cluster credential here: the control plane never talks to a
		// Tekton cluster -- builds execute on the runner named by the
		// pipeline's resolved TektonConnection. Pipeline definitions need no
		// coordinates at all: service builds compile at dispatch from
		// release-pinned content the platform carries, and the infra
		// family's git-repository lane is deliberately inert (its catalog
		// is unset everywhere and creation refuses honestly). The only build
		// knobs are the source workspace sizes.
		{Name: "TEKTON_INFRA_PIPELINE_DISK_SIZE", Value: "1Gi"},
		{Name: "TEKTON_SERVICE_PIPELINE_DISK_SIZE", Value: "5Gi"},

		// ── misc ──
		{Name: "PLANTON_VERSION", Value: effectiveIacModulesVersion(cfg)},
		// The control plane seeds the InfraChart catalog from the bundle of its
		// OWN catalog release: the charts are validated against its protos at
		// apply, so only the release those protos came from can ever be right,
		// and the control plane carries that pin itself. The operator only
		// switches the seed on. Canonical Spring relaxed-binding form of
		// planton.bootstrap.infra-charts.enabled: hyphens are STRIPPED, not
		// underscored (same as PLANTON_BOOTSTRAP_SECRETBACKEND_TYPE); an
		// underscored INFRA_CHARTS_ENABLED would not bind.
		{Name: "PLANTON_BOOTSTRAP_INFRACHARTS_ENABLED", Value: "true"},
		{Name: "PULUMI_ORG", Value: "local"},
		{Name: "STACK_EXECUTION_LOGS_GCS_BUCKET", Value: "local"},
		{Name: "STIGMER_API_KEY", Value: "local"},
		{Name: "STIGMER_ORG_ID", Value: "local"},
	}...)

	envs = append(envs, fgaEnvVars(cfg.OpenFGA)...)
	envs = append(envs, storageEnvVars(cfg.Storage)...)
	envs = append(envs, webIdentityEnvVars(cfg.WebIdentity)...)
	envs = append(envs, githubWebhooksEnvVars(cfg.GithubWebhooks, cfg.Github)...)
	envs = append(envs, githubFactsEnvVars()...)
	envs = append(envs, githubLegacyEnvVars(cfg.Github)...)
	envs = append(envs, consoleEnvVars(cfg.Console)...)
	envs = append(envs, vaultEnvVars(cfg.Vault)...)
	envs = append(envs, secretBackendEnvVars(cfg.SecretBackend)...)
	envs = append(envs, licenseEnvVars(cfg.License)...)
	envs = append(envs, emailEnvVars(cfg.Email)...)
	envs = append(envs, emailSetupHintEnvVars(cfg.CRName, cfg.Namespace)...)

	// Remote-runners capability: the deploy-queue advertisement
	// (CONNECT_RUNNER_TEMPORAL_*) that minted identity documents and the
	// materializer's capability gate both read. Set ONLY when the install
	// opened remote runners and the front door carries the queue -- every
	// reader of this variable on the platform is a remote-runner gate or
	// minter (the in-cluster runner gets its queue address from its own
	// Deployment, never from here), so leaving it unset is what makes the
	// control plane refuse a laptop honestly ("this instance doesn't support
	// deploying from your own machine yet") instead of handing it an address
	// only this cluster's pods resolve.
	if cfg.RemoteRunners != nil {
		envs = append(envs,
			corev1.EnvVar{Name: "CONNECT_RUNNER_TEMPORAL_ENDPOINT", Value: cfg.RemoteRunners.TemporalEndpoint},
			corev1.EnvVar{Name: "CONNECT_RUNNER_TEMPORAL_NAMESPACE", Value: runnerTemporalNamespace},
		)
	}

	// In-cluster runner arm: the boot seeds (slug presence is the activation
	// gate) plus the badge-verification enablement. No credential rides this
	// block: the runner's registration declares its Kubernetes workload
	// identity (namespace + the slug-named ServiceAccount), the seeded
	// declaration provisions its identity account, and the runner proves
	// itself per call with a projected badge the control plane verifies with
	// the cluster itself.
	if cfg.Runner != nil {
		envs = append(envs,
			corev1.EnvVar{Name: "PLANTON_BOOTSTRAP_RUNNER_SLUG", Value: RunnerSlug(cfg.CRName)},
			corev1.EnvVar{Name: "PLANTON_BOOTSTRAP_RUNNER_NAMESPACE", Value: cfg.Namespace},
			corev1.EnvVar{Name: "PLANTON_BOOTSTRAP_RUNNER_PROVISIONER", Value: cfg.Runner.Provisioner},
			// The badge-verification arm: enabled with the CR namespace as
			// the one trusted namespace and the shared audience convention.
			// The kubernetes-auth boot probe verifies the TokenReview grant
			// (the operator-rendered ClusterRole/Binding on the control
			// plane's ServiceAccount) and fails startup loudly without it.
			corev1.EnvVar{Name: "KUBERNETES_WORKLOAD_AUTH_ENABLED", Value: "true"},
			corev1.EnvVar{Name: "KUBERNETES_WORKLOAD_AUTH_AUDIENCE", Value: RunnerBadgeAudience},
			corev1.EnvVar{Name: "KUBERNETES_WORKLOAD_AUTH_TRUSTED_NAMESPACES", Value: cfg.Namespace},
			// Live cloud operations (CloudOps) reach the one in-cluster
			// runner by single-runner direct dial: the runner Service plus
			// the shared bearer token -- read from the SAME Secret key the
			// runner consumes, so the two sides cannot disagree. The runner's
			// gRPC surface rejects tokenless callers, which is what makes the
			// cross-pod dial safe without the (cross-network) mTLS tunnel.
			corev1.EnvVar{Name: "RUNNER_DIRECT_ENABLED", Value: "true"},
			corev1.EnvVar{Name: "RUNNER_DIRECT_HOST", Value: cfg.Runner.DirectDialHost},
			secretEnv("RUNNER_DIRECT_AUTH_TOKEN", cfg.Runner.CloudOpsSecretName, RunnerCloudOpsSecretKeyToken),
		)
		// Build-routing boot seed: create-once records making this cluster
		// the platform's build destination (the build-cluster connection
		// under its well-known slug + the platform-scoped default referencing
		// it). Presence of the RUNNER value is the seeders' activation gate;
		// builds off means NO variables, not empty ones. The env names are
		// the canonical relaxed-binding forms of
		// planton.bootstrap.tekton-connection.* -- hyphens STRIPPED, not
		// underscored (see PLANTON_BOOTSTRAP_INFRACHARTS_ENABLED below).
		// The connection's namespace variable is deliberately not set: empty
		// means "the runner's own placement" (TEKTON_NAMESPACE on the runner
		// Deployment), which keeps the seeded connection inside the log
		// streamer's watch by construction. Deliberately NOT the
		// PLANTON_BOOTSTRAP_RUNNER_SLUG / _ORGANIZATION_SLUG properties --
		// each gates a DIFFERENT seeder.
		if cfg.Runner.BuildEnabled {
			envs = append(envs,
				corev1.EnvVar{Name: "PLANTON_BOOTSTRAP_TEKTONCONNECTION_RUNNER", Value: RunnerSlug(cfg.CRName)},
				corev1.EnvVar{Name: "PLANTON_BOOTSTRAP_TEKTONCONNECTION_ORG", Value: cfg.Identity.Bootstrap.OrgSlug},
			)
		}
	} else {
		// No runner, no dial target: the direct arm stays off (its yaml
		// defaults are already off/loopback; declared here for the mirrored
		// local.env contract's readability).
		envs = append(envs, corev1.EnvVar{Name: "RUNNER_DIRECT_ENABLED", Value: "false"})
	}

	// Opt-in estate backend: wire the component's connection when enabled.
	if cfg.Neo4j != nil {
		envs = append(envs,
			corev1.EnvVar{Name: "NEO4J_URL", Value: cfg.Neo4j.BoltURI},
			corev1.EnvVar{Name: "NEO4J_USERNAME", Value: cfg.Neo4j.Username},
			secretEnv("NEO4J_PASSWORD", cfg.Neo4j.AuthSecretName, cfg.Neo4j.PasswordKey),
		)
	}

	return envs
}

// fgaEnvVars wires the policy-engine connection. With the authorization
// component enabled (a populated connection) the real endpoint is set, the
// store id comes from the component's bootstrap ConfigMap -- the pod
// deliberately cannot start before that ConfigMap exists, which is why the
// controlplane component depends on openfga when the component is enabled --
// and the control plane is told to manage the authorization MODEL itself:
// the model belongs to the control plane's version, so at boot it compares the
// store's latest with its own and writes its own when they differ. No model id
// is ever passed. Otherwise the FGA settings are inert placeholders that exist
// only because their yaml bindings are part of the fail-fast boot contract; no
// arm dials them (allow-owner and allow-authenticated run no policy engine).
// effectiveIacModulesVersion resolves PLANTON_VERSION: the CR's explicit
// spec.controlPlane.iacModulesVersion when set, otherwise the operator's
// verified default pin. The override exists because the module-artifact train
// advances independently of operator releases -- an install must be able to
// adopt a newer verified artifact set (or route around a retracted one)
// without waiting for a new operator image.
func effectiveIacModulesVersion(cfg ControlPlaneConfig) string {
	if cfg.IacModulesVersion != "" {
		return cfg.IacModulesVersion
	}
	return controlPlaneModuleArtifactsVersion
}

// remoteRunnerAPIEndpoint resolves the control-plane address stamped into the
// identity documents of runners that ENROLL (the remote side of the connect
// domain's two-address contract): the front door's gRPC endpoint when the
// install has opened remote runners -- what a laptop actually dials -- and
// the in-cluster Service otherwise. The variable is boot-required, so the
// closed posture still needs a value; it is truthful for the only runners that
// can enroll then (this cluster's), and no runner from outside is admitted
// without the deploy-queue advertisement that the capability alone sets.
func remoteRunnerAPIEndpoint(cfg ControlPlaneConfig) string {
	if cfg.RemoteRunners != nil && cfg.RemoteRunners.PlantonAPIEndpoint != "" {
		return cfg.RemoteRunners.PlantonAPIEndpoint
	}
	return fmt.Sprintf("%s:%d", ControlPlaneServiceFQDN(cfg.CRName, cfg.Namespace), controlPlaneServicePort)
}

func fgaEnvVars(fga OpenFGAConnectionInfo) []corev1.EnvVar {
	if fga.HTTPURL == "" {
		return []corev1.EnvVar{
			{Name: "FGA_API_ENDPOINT", Value: "http://localhost:8088"},
			{Name: "FGA_STORE_ID", Value: "local"},
			{Name: "FGA_READ_TIMEOUT_SECONDS", Value: "30"},
			{Name: "FGA_CONNECT_TIMEOUT_SECONDS", Value: "10"},
			{Name: "FGA_WRITE_TIMEOUT_SECONDS", Value: "30"},
		}
	}
	return []corev1.EnvVar{
		{Name: "FGA_API_ENDPOINT", Value: fga.HTTPURL},
		configMapEnv("FGA_STORE_ID", fga.BootstrapConfigMapName, "store_id"),
		// Relaxed-binding form of planton.bootstrap.authorization-model.manage
		// (hyphens stripped).
		{Name: "PLANTON_BOOTSTRAP_AUTHORIZATIONMODEL_MANAGE", Value: "true"},
		{Name: "FGA_READ_TIMEOUT_SECONDS", Value: "30"},
		{Name: "FGA_CONNECT_TIMEOUT_SECONDS", Value: "10"},
		{Name: "FGA_WRITE_TIMEOUT_SECONDS", Value: "30"},
	}
}

// storageEnvVars selects the object-storage capability's postgres arm: the
// platform's own database doubles as the object store (no external storage
// service, no credentials), and the control plane serves expiring relay
// transfer URLs on its browser-API port -- routed by the same front door that
// serves the console, so browsers see same-origin URLs and the in-cluster
// runner dials the Service directly. The storage database's name and host
// ride the DB_* defaults already in the contract.
func storageEnvVars(binding *StorageBinding) []corev1.EnvVar {
	if binding == nil {
		// Unreachable on a rendered Deployment (the component always sets the
		// binding alongside Identity); returning nothing here would select
		// the r2 arm, whose fail-fast validation names the missing env.
		return nil
	}
	return []corev1.EnvVar{
		// ── object storage: the platform's Postgres doubles as the store ──
		{Name: "PLANTON_STORAGE_PROVIDER", Value: "postgres"},
		{Name: "PLANTON_STORAGE_RELAY_PUBLIC_BASE_URL", Value: binding.RelayPublicBaseURL},
		{Name: "PLANTON_STORAGE_RELAY_INTERNAL_BASE_URL", Value: binding.RelayInternalBaseURL},
	}
}

// PlatformAppUnavailableReason is the sentence on the platform GitHub App's
// card on every self-hosted install. The catalog's canonical copy names the
// desktop's way out (the sign-in on this machine); a server's one door is the
// customer's own App, so the operator says that instead.
const PlatformAppUnavailableReason = "This install has no GitHub App of its own. Connect your own GitHub App instead."

// webIdentityEnvVars renders the keyless identity issuer: the issuer URL every
// install has (discovery must be honest about its own address even when
// keyless is closed) and the connection-method verdict for the oidc mode.
// The reason is rendered only when closed -- the control plane treats a blank
// reason as "use the canonical copy", and an offered mode has no reason.
func webIdentityEnvVars(binding *WebIdentityBinding) []corev1.EnvVar {
	if binding == nil {
		// Unreachable on a rendered Deployment (set alongside Identity); the
		// control plane's default issuer is the hosted one, which a self-hosted
		// install must never mint.
		return nil
	}
	envs := []corev1.EnvVar{
		// ── keyless identity issuer: the front door ──
		{Name: "OIDC_ISSUER_URL", Value: binding.IssuerURL},
	}
	if binding.Offered {
		return append(envs, corev1.EnvVar{Name: "PLANTON_CONNECT_METHODAVAILABILITY_OIDC_AVAILABILITY", Value: "available"})
	}
	return append(envs,
		corev1.EnvVar{Name: "PLANTON_CONNECT_METHODAVAILABILITY_OIDC_AVAILABILITY", Value: "unavailable"},
		corev1.EnvVar{Name: "PLANTON_CONNECT_METHODAVAILABILITY_OIDC_REASON", Value: binding.ClosedReason},
	)
}

// githubWebhooksEnvVars renders GitHub webhook delivery posture: the receiver
// URL (true on every install) and whether GitHub can reach it.
func githubWebhooksEnvVars(binding *GithubWebhooksBinding, github *GithubBinding) []corev1.EnvVar {
	if binding == nil {
		return nil
	}
	// The one-host variable follows github.com's declared verdict when the
	// install declared one (a private door whose github.com posture is
	// declared reachable through a perimeter, or the reverse); the door's
	// own reachability otherwise.
	reachable := binding.Reachable
	if h := githubDefaultHostBinding(github); h != nil {
		reachable = h.WebhooksReachable
	}
	return []corev1.EnvVar{
		// ── GitHub webhook delivery: the front door's webhook namespace ──
		{Name: "GITHUB_WEBHOOKS_RECEIVER_URL", Value: binding.ReceiverURL},
		{Name: "GITHUB_WEBHOOKS_REACHABLE", Value: fmt.Sprintf("%t", reachable)},
	}
}

// githubLegacyEnvVars renders the parts of the declaration the one-host env
// contract can carry: host login. Absent (never "unavailable") when the
// declaration does not turn it on -- the platform's own default is off.
func githubLegacyEnvVars(github *GithubBinding) []corev1.EnvVar {
	if github == nil || !github.HostLogin {
		return nil
	}
	return []corev1.EnvVar{
		{Name: "PLANTON_CONNECT_METHODAVAILABILITY_HOSTLOGIN_AVAILABILITY", Value: "available"},
	}
}

// consoleEnvVars names where the browser console lives, for the control plane
// to compose the console pages a customer's GitHub App is pointed at. One
// variable, deployment-wide: the same name hosted pins and a local instance
// leaves empty, so the control plane has one spelling of "where is my console"
// to converge every per-domain return address onto.
func consoleEnvVars(binding *ConsoleBinding) []corev1.EnvVar {
	if binding == nil {
		return nil
	}
	return []corev1.EnvVar{
		// ── browser console: the front door ──
		{Name: "PLANTON_CONSOLE_URL", Value: binding.URL},
	}
}

// vaultEnvVars wires the platform vault: the deployed OpenBAO's real address
// and root token when the component is enabled, or an explicit opt-out when it
// is not. There is deliberately NO placeholder arm -- a dead vault address
// boots fine and then fails confusingly at first use (and log-screams from the
// OIDC signing-key bootstrap on every boot), which is the exact rot class the
// storage seam eliminated for the R2 variables.
func vaultEnvVars(binding *VaultBinding) []corev1.EnvVar {
	if binding == nil {
		// The control plane's vault default is enabled+required (so a hosted
		// deploy that loses VAULT_ADDR still fails loudly at rollout); running
		// without one is therefore an explicit, legible choice in the pod spec.
		return []corev1.EnvVar{
			{Name: "PLANTON_VAULT_ENABLED", Value: "false"},
		}
	}
	return []corev1.EnvVar{
		// ── platform vault (OpenBAO component): secrets + Transit signing ──
		{Name: "VAULT_ADDR", Value: binding.APIAddr},
		secretEnv("VAULT_TOKEN", binding.InitSecretName, binding.RootTokenKey),
	}
}

// secretBackendEnvVars activates the control plane's default-secret-backend
// boot seed (planton.bootstrap.secret-backend.* via Spring relaxed binding).
// Addressing only -- the seeded kinds are credential-free by construction.
func secretBackendEnvVars(binding *SecretBackendBinding) []corev1.EnvVar {
	if binding == nil {
		return nil
	}
	envs := []corev1.EnvVar{
		// ── default secret backend seed ──
		{Name: "PLANTON_BOOTSTRAP_SECRETBACKEND_TYPE", Value: binding.Type},
	}
	if binding.AwsRegion != "" {
		envs = append(envs, corev1.EnvVar{
			Name: "PLANTON_BOOTSTRAP_SECRETBACKEND_AWSSECRETSMANAGER_REGION", Value: binding.AwsRegion,
		})
	}
	return envs
}

// licenseEnvVars delivers the license key onto the resolver's entry point
// (planton.licensing.key). The env name is the property's CANONICAL relaxed-
// binding form -- PLANTON_LICENSING_KEY, never PLANTON_LICENSE_KEY, which
// binds a different property (planton.license.key) and leaves the deployment
// silently unlicensed; caught live by the kind license drill. Nil binding =
// Community = no variable at all. Two delivery notes that shape behavior: a
// runtime key entered through the platform's own API wins over this value
// inside the control plane, and a rotated Secret takes effect on the next
// pod restart (env is read at container start -- a renewal is Secret edit +
// rollout).
func licenseEnvVars(binding *LicenseBinding) []corev1.EnvVar {
	if binding == nil {
		return nil
	}
	if binding.SecretName != "" {
		return []corev1.EnvVar{secretEnv("PLANTON_LICENSING_KEY", binding.SecretName, binding.SecretKey)}
	}
	return []corev1.EnvVar{{Name: "PLANTON_LICENSING_KEY", Value: binding.Key}}
}

// identityEnvVars builds the control plane's identity arm. There is exactly
// one: every install runs as a standard OIDC relying party against the
// bundled identity server (sign-in is unconditional -- through the gateway's
// port-forward front door or the ingress hostname).
//
// The control plane discovers the issuer EAGERLY at boot (which is why the
// controlplane component depends on identity) and validates browser tokens by
// audience. All issuer FETCHES (discovery, JWKS, token, userinfo) go to the
// in-cluster internal URL -- split horizon -- while validation stays pinned
// to the advertised issuer the tokens carry. Cross-domain calls inside the
// control plane are in-process service-layer calls, so no machine identity
// is provisioned or injected here.
func identityEnvVars(binding *IdentityBinding) []corev1.EnvVar {
	envs := []corev1.EnvVar{
		// ── identity: OIDC relying party against the bundled identity server ──
		{Name: "IDP_PROVIDER", Value: IdentityProviderKeycloak},
		{Name: "IDP_API_AUDIENCE", Value: IDPAPIAudience},
		{Name: "IDP_DOMAIN", Value: binding.Hostname},
		{Name: "IDP_URL", Value: binding.IssuerURL},
		{Name: "IDP_INTERNAL_URL", Value: binding.InternalIssuerURL},
		{Name: "IDP_TOKEN_CUSTOM_CLAIMS_KEY", Value: "https://planton.ai"},
		{Name: "PLANTON_CLOUD_API_AUDIENCE", Value: IDPAPIAudience},

		// ── user-directory capability (least-privilege realm-users client) ──
		// Drives user lifecycle in the bundled identity server: first-run
		// admin creation now, invitation-created teammates later.
		{Name: "IDP_REALM_USERS_CLIENT_ID", Value: IdentityUsersClientID},
		secretEnv("IDP_REALM_USERS_CLIENT_SECRET", binding.UsersClientSecretName, IdentityOIDCClientSecretKey),

		// ── federation facts (published by the identity component) ──
		// A STATIC path to the mounted facts file, deliberately not the
		// facts themselves: env changes roll the pod, mounted content
		// updates in place. Unset (the yaml default) on non-operator
		// installs, which keeps the reader inert there.
		{Name: "IDP_FEDERATION_FACTS_FILE", Value: IdentityFederationFactsFilePath()},

		// ── granular authorization arm (planton.authorization.provider seam) ──
		{Name: "PLANTON_AUTHORIZATION_PROVIDER", Value: binding.AuthorizationProvider},

		// ── first-boot seeds (planton.bootstrap.* via Spring relaxed binding) ──
		// Presence of the org slug is what activates the control plane's
		// seeder; a hosted deployment never sets these.
		{Name: "PLANTON_BOOTSTRAP_ORGANIZATION_SLUG", Value: binding.Bootstrap.OrgSlug},
		{Name: "PLANTON_BOOTSTRAP_ORGANIZATION_NAME", Value: binding.Bootstrap.OrgName},
		{Name: "PLANTON_BOOTSTRAP_ENVIRONMENT_SLUG", Value: binding.Bootstrap.EnvSlug},
		{Name: "PLANTON_BOOTSTRAP_ENVIRONMENT_NAME", Value: binding.Bootstrap.EnvName},
	}

	// The admins env is only set when admins are declared: the control plane's
	// grant reconciler activates on the property's PRESENCE, and an empty
	// string would activate it with nothing to do.
	if len(binding.Bootstrap.Admins) > 0 {
		envs = append(envs, corev1.EnvVar{
			Name:  "PLANTON_BOOTSTRAP_ADMINS",
			Value: strings.Join(binding.Bootstrap.Admins, ","),
		})
	}

	// First-run setup mode (no admin declared): the setup-code property's
	// PRESENCE is what opens the control plane's public setup RPCs -- same
	// activation grain as the seeds above. The hint travels alongside so the
	// setup page can print the exact read-the-code command.
	if binding.SetupCodeSecretName != "" {
		envs = append(envs,
			secretEnv("PLANTON_BOOTSTRAP_SETUP_CODE", binding.SetupCodeSecretName, IdentitySetupCodeSecretKey),
			corev1.EnvVar{Name: "PLANTON_BOOTSTRAP_SETUP_CODE_HINT", Value: binding.SetupCodeHint},
		)
	}

	return envs
}

func controlPlaneEnvFrom(cfg ControlPlaneConfig) []corev1.EnvFromSource {
	var sources []corev1.EnvFromSource

	if cfg.ExternalConfigSecretName != "" {
		sources = append(sources, corev1.EnvFromSource{
			SecretRef: &corev1.SecretEnvSource{
				LocalObjectReference: corev1.LocalObjectReference{
					Name: cfg.ExternalConfigSecretName,
				},
			},
		})
	}

	return sources
}

func secretEnv(envName, secretName, key string) corev1.EnvVar {
	return corev1.EnvVar{
		Name: envName,
		ValueFrom: &corev1.EnvVarSource{
			SecretKeyRef: &corev1.SecretKeySelector{
				LocalObjectReference: corev1.LocalObjectReference{Name: secretName},
				Key:                  key,
			},
		},
	}
}

func configMapEnv(envName, configMapName, key string) corev1.EnvVar {
	return corev1.EnvVar{
		Name: envName,
		ValueFrom: &corev1.EnvVarSource{
			ConfigMapKeyRef: &corev1.ConfigMapKeySelector{
				LocalObjectReference: corev1.LocalObjectReference{Name: configMapName},
				Key:                  key,
			},
		},
	}
}

func intOrStr(val int) *intstr.IntOrString {
	v := intstr.FromInt32(int32(val))
	return &v
}

//go:fix inline
func strPtr(s string) *string {
	return new(s)
}

//go:fix inline
func ptrBool(b bool) *bool {
	return new(b)
}

//go:fix inline
func int64Ptr(i int64) *int64 {
	return new(i)
}
