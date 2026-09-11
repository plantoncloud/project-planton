package component

import (
	"context"
	"fmt"
	"strings"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	v1 "github.com/plantonhq/planton/operator/api/v1"
	"github.com/plantonhq/planton/operator/internal/resources"
)

// ControlPlane deploys the Planton control plane monolith as a Kubernetes
// Deployment with all internal connections wired to the operator-managed data
// layer and supporting services.
type ControlPlane struct{ Base }

func (cp *ControlPlane) Name() string { return "controlplane" }

func (cp *ControlPlane) Dependencies(planton *v1.PlantonPlatform) []string {
	// The identity dependency is unconditional because sign-in is: the
	// control plane boots as an OIDC relying party and performs EAGER issuer
	// discovery at startup -- the identity server must be serving before the
	// JVM comes up, or the Spring context fails and the pod crash-loops
	// through no fault of its own.
	deps := []string{"postgresql", "redis", "temporal", "identity"}
	// With policy-engine authorization enabled, the control plane's FGA
	// store/model env comes from the openfga component's bootstrap ConfigMap
	// (ConfigMapKeyRef) -- the pod literally cannot start before that exists,
	// so the dependency makes the wait an explained status instead of a
	// CreateContainerConfigError. The minimal footprint keeps no dependency:
	// its arms run no policy engine.
	if isAuthorizationEnabled(planton) {
		deps = append(deps, "openfga")
	}
	// With the vault component enabled, the control plane's VAULT_TOKEN is a
	// SecretKeyRef into the openbao init Secret -- same tie as the FGA
	// ConfigMap above: depend on it so the wait is an explained status, and
	// so the vault is initialized, unsealed and engine-mounted before the
	// first Java consumer dials it.
	if isVaultEnabled(planton) {
		deps = append(deps, "openbao")
	}
	return deps
}

func (cp *ControlPlane) IsEnabled(_ *v1.PlantonPlatform) bool { return true }

func (cp *ControlPlane) Reconcile(ctx context.Context, c client.Client, _ *runtime.Scheme, planton *v1.PlantonPlatform) (Result, error) {
	log := logf.FromContext(ctx).WithValues("component", cp.Name())
	ownerRef := cp.OwnerReferenceFor(planton)

	cfg := cp.buildConfig(planton, ownerRef)

	// Every Secret spec.email names must exist with its keys BEFORE the
	// Deployment projects them, or the pod sits in FailedMount with no reason
	// anyone can read. A finding is reported as this component's message and
	// the pod is rendered as if no email were declared: the platform keeps
	// running, invitations stay links, and the person reads exactly which
	// Secret to create. The 30-second requeue picks it up when it appears.
	emailPreflight, err := preflightEmailSecrets(ctx, c, planton)
	if err != nil {
		return Result{}, fmt.Errorf("preflighting email Secrets: %w", err)
	}
	if emailPreflight != "" {
		cfg.Email = nil
	}

	if cfg.Identity == nil {
		// Unreachable in a healthy pass (the identity dependency implies a
		// resolved front-door URL), but a Deployment without the identity arm
		// must never be rendered -- there is no unauthenticated arm to fall
		// back to.
		return Result{Ready: false, Message: "Waiting for the front-door URL before wiring sign-in"}, nil
	}

	// The Deployment about to be rendered references the CloudOps auth token
	// by Secret name (the direct-dial bearer both sides hold) -- mint it
	// first, or the pod sits in CreateContainerConfigError. The runner
	// component reconciles AFTER this one (it depends on controlplane), so
	// it cannot be the one to break the tie.
	if cfg.Runner != nil {
		if _, err := EnsureCloudOpsToken(ctx, c, planton.Name, planton.Namespace, ownerRef); err != nil {
			return Result{}, fmt.Errorf("ensuring runner CloudOps token: %w", err)
		}
	}

	// The dedicated ServiceAccount exists on every install (annotation-free by
	// default) so granting the platform a cloud identity later is a pure spec
	// edit -- the pod already runs as it.
	if err := cp.ApplyTypedObject(ctx, c, resources.ControlPlaneServiceAccount(cfg)); err != nil {
		return Result{}, fmt.Errorf("applying ControlPlane ServiceAccount: %w", err)
	}

	// The badge-verification grant precedes the Deployment: the control
	// plane's kubernetes-auth arm probes its own TokenReview permission at
	// boot and fails startup without it, so applying the grant after the pod
	// starts would be a crash-loop by construction. Applied whenever the
	// runner is enabled (the badge arm's activation follows the runner env).
	if cfg.Runner != nil {
		if err := cp.ApplyTypedObject(ctx, c, resources.ControlPlaneTokenReviewerClusterRole(cfg)); err != nil {
			return Result{}, fmt.Errorf("applying ControlPlane token-reviewer ClusterRole: %w", err)
		}
		if err := cp.ApplyTypedObject(ctx, c, resources.ControlPlaneTokenReviewerClusterRoleBinding(cfg)); err != nil {
			return Result{}, fmt.Errorf("applying ControlPlane token-reviewer ClusterRoleBinding: %w", err)
		}
	}

	deploy := resources.ControlPlaneDeployment(cfg)
	if err := cp.ApplyTypedObject(ctx, c, deploy); err != nil {
		return Result{}, fmt.Errorf("applying ControlPlane Deployment: %w", err)
	}

	svc := resources.ControlPlaneService(planton.Name, planton.Namespace, ownerRef)
	if err := cp.ApplyTypedObject(ctx, c, svc); err != nil {
		return Result{}, fmt.Errorf("applying ControlPlane Service: %w", err)
	}

	deployName := resources.ControlPlaneDeploymentName(planton.Name)
	ready, err := cp.IsDeploymentReady(ctx, c, deployName, planton.Namespace)
	if err != nil {
		return Result{}, fmt.Errorf("checking ControlPlane readiness: %w", err)
	}
	if !ready {
		log.Info("ControlPlane not ready")
		return cp.NotReady(ctx, c, planton.Namespace, DeploymentRef(deployName), "Waiting for ControlPlane Deployment"), nil
	}

	// A healthy pod under a declaration that could not be honored is not
	// Ready: "Ready" means what the manifest declared is what runs.
	if emailPreflight != "" {
		log.Info("ControlPlane running without the declared email", "reason", emailPreflight)
		return Refused(emailPreflight), nil
	}

	log.Info("ControlPlane ready")
	return Result{Ready: true, Message: "ControlPlane healthy"}, nil
}

func (cp *ControlPlane) buildConfig(planton *v1.PlantonPlatform, ownerRef *metav1.OwnerReference) resources.ControlPlaneConfig {
	cfg := resources.ControlPlaneConfig{
		CRName:     planton.Name,
		Namespace:  planton.Namespace,
		Version:    planton.Spec.Version,
		OwnerRef:   ownerRef,
		Replicas:   1,
		PostgreSQL: resources.PostgreSQLConnection(planton.Name, planton.Namespace),
		Redis:      resources.RedisConnection(planton.Name, planton.Namespace),
		Temporal:   resources.TemporalConnection(planton.Name, planton.Namespace),
	}

	if planton.Spec.ControlPlane != nil {
		if planton.Spec.ControlPlane.Replicas != nil {
			cfg.Replicas = *planton.Spec.ControlPlane.Replicas
		}
		if planton.Spec.ControlPlane.Image != nil {
			cfg.ImageRepository = planton.Spec.ControlPlane.Image.Repository
			cfg.ImageTag = planton.Spec.ControlPlane.Image.Tag
		}
		cfg.ExternalConfigSecretName = planton.Spec.ControlPlane.ExternalConfigSecretName
		cfg.IacModulesVersion = planton.Spec.ControlPlane.IacModulesVersion
		cfg.ServiceAccountAnnotations = planton.Spec.ControlPlane.ServiceAccountAnnotations
	}

	// The platform vault is present or absent -- never a placeholder. Enabled:
	// the real OpenBAO address + the root token by Secret reference. Disabled:
	// an explicit PLANTON_VAULT_ENABLED=false (the Java side treats missing
	// VAULT_ADDR withOUT that opt-out as a loud boot failure, protecting the
	// hosted deployment shape from silently losing its vault).
	if isVaultEnabled(planton) {
		conn := resources.OpenBAOConnection(planton.Name, planton.Namespace)
		cfg.Vault = &resources.VaultBinding{
			APIAddr:        conn.APIAddr,
			InitSecretName: conn.InitSecretName,
			RootTokenKey:   conn.RootTokenKey,
		}
	}

	cfg.SecretBackend = effectiveSecretBackend(planton)
	cfg.License = effectiveLicense(planton)
	cfg.Email = effectiveEmail(planton)

	if isAuthorizationEnabled(planton) {
		cfg.OpenFGA = resources.OpenFGAConnection(planton.Name, planton.Namespace)
	}

	if isNeo4jEnabled(planton) {
		conn := resources.Neo4jConnection(planton.Name, planton.Namespace)
		cfg.Neo4j = &conn
	}

	if isRunnerEnabled(planton) {
		cfg.Runner = &resources.RunnerBinding{
			CloudOpsSecretName: resources.RunnerCloudOpsSecretName(planton.Name),
			Provisioner:        effectiveIacProvisioner(planton),
			DirectDialHost:     resources.RunnerServiceFQDN(planton.Name, planton.Namespace),
			BuildEnabled:       isBuildEffective(planton),
		}
	}

	// The control plane always runs as an OIDC relying party against the
	// bundled identity server -- there is no unauthenticated arm. The
	// front-door URL is always known by the time this executes: the
	// controlplane component depends on identity, which does not report Ready
	// until the URL has resolved (and the gateway URL is deterministic).
	if publicURL, resolved := frontDoorURL(planton); resolved && publicURL != "" {
		// The trusting-team arm is the default -- no policy engine runs, so
		// every signed-in teammate may act while ownership/operator records
		// still land in Postgres. Enabling the authorization component
		// upgrades to the real engine (its connection is wired above and the
		// boot backfill mirrors the Postgres records into it).
		authorizationProvider := "allow-authenticated"
		if isAuthorizationEnabled(planton) {
			authorizationProvider = "openfga"
		}
		realm := identityRealm(planton)
		cfg.Identity = &resources.IdentityBinding{
			IssuerURL: resources.IdentityIssuerURL(publicURL, realm),
			// Split horizon: everything the control plane FETCHES from the
			// issuer (discovery, JWKS, token, userinfo) goes to the identity
			// Service in-cluster, while token validation stays pinned to the
			// advertised issuer above. Removes the "cluster must hairpin its
			// own public hostname" requirement -- and in gateway mode the
			// advertised localhost URL is not in-cluster-reachable at all.
			InternalIssuerURL:     resources.IdentityInternalIssuerURL(planton.Name, planton.Namespace, realm),
			Hostname:              publicHostname(publicURL),
			UsersClientSecretName: resources.IdentityUsersClientSecretName(planton.Name),
			AuthorizationProvider: authorizationProvider,
			Bootstrap:             effectiveBootstrap(planton),
		}
		// First-run setup mode keys on exactly spec.identity.adminEmail being
		// unset -- NOT on "no admins declared": spec.bootstrap.admins may name
		// people without seeding anyone a sign-in account, and those declared
		// admins coexist with setup (the first visitor creates the first
		// sign-in-able account; declared admins get their grants whenever
		// they later sign in). The identity component generates the code
		// Secret before this Deployment renders (controlplane depends on
		// identity), so the reference never dangles.
		if identityAdminEmail(planton) == "" {
			cfg.Identity.SetupCodeSecretName = resources.IdentitySetupCodeSecretName(planton.Name)
			cfg.Identity.SetupCodeHint = resources.IdentitySetupCodeHint(planton.Name, planton.Namespace)
		}

		// Object storage rides the same front door: the platform's Postgres
		// is the store, and the control plane's relay endpoint serves the
		// transfer URLs -- public ones on the advertised URL (same origin as
		// the console), runner-facing ones on the in-cluster Service.
		cfg.Storage = &resources.StorageBinding{
			RelayPublicBaseURL:   publicURL,
			RelayInternalBaseURL: resources.ControlPlaneRelayInternalBaseURL(planton.Name, planton.Namespace),
		}

		// The front door is the keyless identity issuer, and every posture
		// that needs an inbound path from the internet derives from the ONE
		// posture computed for it -- never from a hand-set literal. The
		// posture resolves exactly when the URL does (same inputs), so it is
		// always known here.
		posture, _ := frontDoorPosture(planton)
		cfg.WebIdentity = &resources.WebIdentityBinding{
			IssuerURL:    posture.URL,
			Offered:      posture.KeylessOffered(),
			ClosedReason: posture.KeylessClosedReason(),
		}
		cfg.GithubWebhooks = &resources.GithubWebhooksBinding{
			Reachable:   posture.Public(),
			ReceiverURL: resources.GithubWebhookReceiverURL(publicURL),
		}
		cfg.Console = &resources.ConsoleBinding{URL: posture.URL}

		// Remote runners ride the same door: when the install opened them and
		// this door carries native gRPC, the queue and the API are advertised
		// at the door's gRPC endpoint (the ingress component routes the
		// queue's workflow service beside the API). A door that cannot carry
		// it leaves the binding nil -- the control plane then refuses remote
		// enrollment with the reason, and the ingress status names the door.
		if remoteRunnersCarried(planton) {
			endpoint := resources.GRPCEndpoint(publicURL)
			cfg.RemoteRunners = &resources.RemoteRunnersBinding{
				PlantonAPIEndpoint: endpoint,
				TemporalEndpoint:   endpoint,
			}
		}
	}

	return cfg
}

// effectiveBootstrap resolves spec.bootstrap into concrete seed values.
// Defaulting lives HERE, not only in CRD markers: kubebuilder defaults apply
// solely when the parent struct is present (a nil spec.bootstrap gets
// nothing), and the admins default is cross-field ([spec.identity.adminEmail])
// which CRD markers cannot express at all.
func effectiveBootstrap(planton *v1.PlantonPlatform) resources.BootstrapBinding {
	binding := resources.BootstrapBinding{
		OrgSlug: "default",
		EnvSlug: "default",
	}

	bootstrap := planton.Spec.Bootstrap
	if bootstrap != nil {
		if bootstrap.Organization != nil && bootstrap.Organization.Slug != "" {
			binding.OrgSlug = bootstrap.Organization.Slug
		}
		if bootstrap.Organization != nil {
			binding.OrgName = bootstrap.Organization.Name
		}
		if bootstrap.Environment != nil && bootstrap.Environment.Slug != "" {
			binding.EnvSlug = bootstrap.Environment.Slug
		}
		if bootstrap.Environment != nil {
			binding.EnvName = bootstrap.Environment.Name
		}
		binding.Admins = bootstrap.Admins
	}

	// Display names default to the slug -- one identifier, no ceremony.
	if binding.OrgName == "" {
		binding.OrgName = binding.OrgSlug
	}
	if binding.EnvName == "" {
		binding.EnvName = binding.EnvSlug
	}

	// The declared identity admin IS the default bootstrap admin: naming one
	// person twice to get a working install would be pure friction.
	if len(binding.Admins) == 0 {
		if adminEmail := identityAdminEmail(planton); adminEmail != "" {
			binding.Admins = []string{adminEmail}
		}
	}

	return binding
}

// effectiveIacProvisioner resolves spec.bootstrap.iacProvisioner. Defaulted
// here as well as in the CRD marker for the same reason as effectiveBootstrap:
// a nil spec.bootstrap gets no marker defaults.
func effectiveIacProvisioner(planton *v1.PlantonPlatform) string {
	if planton.Spec.Bootstrap != nil && planton.Spec.Bootstrap.IacProvisioner != "" {
		return planton.Spec.Bootstrap.IacProvisioner
	}
	return "tofu"
}

// effectiveSecretBackend resolves the default-secret-backend seed: a declared
// spec.bootstrap.secretBackend always wins; with nothing declared and the
// vault running (the default), the platform kind is seeded automatically (the
// vault exists to be the secret store -- running it and still having "create
// a secret" fail would be a bewildering install). Vault explicitly opted out
// and nothing declared ⇒ nil: no default backend, console funnels until one
// is created.
func effectiveSecretBackend(planton *v1.PlantonPlatform) *resources.SecretBackendBinding {
	declared := (*v1.BootstrapSecretBackendSpec)(nil)
	if planton.Spec.Bootstrap != nil {
		declared = planton.Spec.Bootstrap.SecretBackend
	}

	if declared == nil {
		if isVaultEnabled(planton) {
			return &resources.SecretBackendBinding{Type: "platform"}
		}
		return nil
	}

	switch declared.Type {
	case "awsSecretsManager":
		// CRD CEL guarantees the block + both fields when the type is declared.
		return &resources.SecretBackendBinding{
			Type:      "aws-secrets-manager",
			AwsRegion: declared.AwsSecretsManager.Region,
		}
	default: // "platform" (the CRD enum admits nothing else)
		return &resources.SecretBackendBinding{Type: "platform"}
	}
}

// effectiveLicense resolves spec.license into the delivery binding. The CRD's
// CEL rule guarantees at most one of key/secretKeyRef; a declared-but-empty
// block (or a blank key) renders nothing, exactly like an absent one --
// Community is the honest default, never an empty env var. Verification is
// deliberately NOT here: the control plane owns it and answers with typed
// outcomes, so a bad key fails with a precise message there.
func effectiveLicense(planton *v1.PlantonPlatform) *resources.LicenseBinding {
	l := planton.Spec.License
	if l == nil {
		return nil
	}
	if l.SecretKeyRef != nil {
		return &resources.LicenseBinding{
			SecretName: l.SecretKeyRef.Name,
			SecretKey:  l.SecretKeyRef.Key,
		}
	}
	if key := strings.TrimSpace(l.Key); key != "" {
		return &resources.LicenseBinding{Key: key}
	}
	return nil
}

// isVaultEnabled defaults to true: the bundled secrets manager is integral
// (credential store, envelope-encryption KEK, OIDC signing key), so absence of
// spec.vault means it runs. Must agree with OpenBAO.IsEnabled (openbao.go) and
// isOpenBAOEnabled (status package), or the control-plane wiring, the
// reconciler, and the status slot disagree about the component's existence.
func isVaultEnabled(p *v1.PlantonPlatform) bool {
	return p.Spec.Vault == nil || p.Spec.Vault.Enabled == nil || *p.Spec.Vault.Enabled
}

// publicHostname strips the scheme from a public URL for config fields that
// want a bare hostname.
func publicHostname(publicURL string) string {
	host := strings.TrimPrefix(publicURL, "https://")
	return strings.TrimPrefix(host, "http://")
}

func isAuthorizationEnabled(p *v1.PlantonPlatform) bool {
	return p.Spec.Components != nil && p.Spec.Components.Authorization != nil && p.Spec.Components.Authorization.Enabled
}

func isNeo4jEnabled(p *v1.PlantonPlatform) bool {
	return p.Spec.Components != nil && p.Spec.Components.Graph != nil && p.Spec.Components.Graph.Enabled
}
