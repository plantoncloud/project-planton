package resources

import (
	"encoding/json"
	"fmt"
	"strings"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"

	keycloaklogintheme "github.com/plantonhq/planton/operator/internal/keycloaklogintheme"
)

// The identity component deploys Keycloak as the platform's bundled OIDC
// issuer. Planton itself never handles passwords -- the backend and console
// stay standard OIDC relying parties -- so the credential-handling surface
// (password hashing, brute-force lockout, reset flows, 2FA, and later LDAP/AD
// federation) lives entirely in the battle-tested identity server. The
// operator provisions everything declaratively: realm, OIDC client, and the
// first admin user are generated here, with credentials in Kubernetes
// Secrets, so sign-in works with zero external identity setup.
const (
	IdentityDefaultImageRepo = "quay.io/keycloak/keycloak"

	// IdentityDefaultImageTag pins the bundled Keycloak version. Identity is a
	// security surface: upgrades are deliberate operator releases, never a
	// floating tag.
	IdentityDefaultImageTag = "26.3"

	// IdentityPathPrefix is where the identity server is served on the ONE
	// public hostname (console on "/", API on the gRPC-Web prefix). Everything
	// Keycloak serves -- login pages, OIDC endpoints, admin console -- lives
	// under this prefix. NOTE: "/auth" (Keycloak's historical default) is NOT
	// usable here -- the console owns /auth/* routes (its own account
	// provisioning interstitial among them).
	IdentityPathPrefix = "/idp"

	// IDPAPIAudience is the audience value stamped into access tokens by the
	// realm's audience mapper and validated by the control plane. It is a
	// fixed logical name, deliberately hostname-independent, so an
	// auto-derived hostname never churns token validation config.
	IDPAPIAudience = "planton-api"

	// IdentityConsoleClientID is the OIDC client the console signs in with.
	IdentityConsoleClientID = "planton-console"

	// IdentityUsersClientID is the control plane's least-privilege
	// user-directory client: its service account holds exactly the realm's
	// manage-users/view-users roles, so the platform can drive user LIFECYCLE
	// (the first-run admin, later invitation-created teammates) without any
	// broader identity-server administration surface. Realm configuration,
	// clients, and mappers stay operator-provisioned; this credential cannot
	// touch them.
	IdentityUsersClientID = "planton-users"

	// IdentityCLIClientID is the public PKCE client `planton login` signs in
	// through. Public because a CLI binary cannot hold a secret; PKCE (S256,
	// enforced via IdentityPKCEMethodAttribute) is what makes the public
	// client safe. Its redirect URIs are the CLI's fixed loopback callback
	// (IdentityCLIRedirectURIs below).
	IdentityCLIClientID = "planton-cli"

	// IdentityAudienceMapperName names the protocol mapper that stamps
	// IDPAPIAudience into access tokens. Shared by the realm import and the
	// realm reconciler so the two derivations can never disagree on which
	// mapper is theirs.
	IdentityAudienceMapperName = "planton-api-audience"

	// IdentityAccessTokenLifespanSeconds is the realm's access-token lifespan:
	// 15 minutes. The console refreshes transparently; longer-lived access
	// tokens only widen the replay window. This exact number is also the
	// bound the platform's offboarding guarantee quotes ("no session outlives
	// its 15-minute token"), so changing it is a product decision, not a
	// tuning knob.
	IdentityAccessTokenLifespanSeconds = 900

	// IdentitySSOSessionIdleSeconds is the realm's idle SSO session timeout:
	// 8h -- the workday, not Keycloak's 30m default, which would bounce users
	// back to the login form after every coffee break.
	IdentitySSOSessionIdleSeconds = 28800

	// IdentityConsoleCallbackPath is the console sign-in stack's OAuth
	// callback, appended to the front-door URL to form the console client's
	// exact redirect URI.
	IdentityConsoleCallbackPath = "/api/auth/callback/iam"

	// IdentityDeviceCallbackPath is the console's device-auth broker
	// callback: desktop and CLI sign-ins ride the console's confidential
	// client through this second redirect URI (the broker exchanges the
	// code server-side and hands the device a time-bound ticket). It moves
	// with the console web app's /api/device/auth/callback route.
	IdentityDeviceCallbackPath = "/api/device/auth/callback"

	// IdentityConsolePostLogoutPath is where the signout flow returns,
	// appended to the front-door URL.
	IdentityConsolePostLogoutPath = "/login"

	// IdentityPostLogoutAttribute is Keycloak's client attribute carrying the
	// allowed post-logout redirect URIs.
	IdentityPostLogoutAttribute = "post.logout.redirect.uris"

	// IdentityPKCEMethodAttribute + IdentityPKCEMethodS256 pin the CLI
	// client's PKCE code-challenge method. Without the attribute Keycloak
	// ACCEPTS plain non-PKCE authorization requests on a public client --
	// enforcing S256 is what closes the code-interception class.
	IdentityPKCEMethodAttribute = "pkce.code.challenge.method"
	IdentityPKCEMethodS256      = "S256"

	// IdentityManagedAttribute marks realm objects the operator manages.
	// The realm reconciler converges ONLY objects it created or that carry
	// this mark (plus the three enumerated Planton clients, which are owned
	// by clientId) -- an admin-created object is invisible to it by
	// construction rather than by carefulness.
	IdentityManagedAttribute = "planton.ai/managed"

	// IdentityRealmManagementClientID is Keycloak's built-in realm-management
	// client, the source of the admin roles granted to the users client's
	// service account.
	IdentityRealmManagementClientID = "realm-management"

	// IdentityBootstrapAdminUsername is the Keycloak master-realm bootstrap
	// admin login (KC_BOOTSTRAP_ADMIN_USERNAME). Distinct from
	// IdentityAdminUsername, the seeded Planton admin USER in the platform
	// realm.
	IdentityBootstrapAdminUsername = "admin"

	// IdentityBootstrapAdminPasswordKey is the data key in the bootstrap
	// admin Secret (IdentityBootstrapAdminSecretName).
	IdentityBootstrapAdminPasswordKey = "password"

	// IdentityDBName is Keycloak's database in the platform PostgreSQL. The
	// control plane self-provisions only its own databases, so the identity
	// Deployment carries an init step that creates this one.
	IdentityDBName = "keycloak"

	identityHTTPPort       = 8080
	identityServicePort    = 80
	identityManagementPort = 9000

	// identityDBInitImage provides psql for the ensure-database init step.
	// Official image, pinned major: the client only needs to speak the wire
	// protocol, which is stable across server versions.
	identityDBInitImage = "postgres:17-alpine"

	// IdentityProviderKeycloak is the provider selector value the control
	// plane's auth seam and the console's sign-in stack both key on.
	IdentityProviderKeycloak = "keycloak"

	// IdentityAdminUsername is the seeded first admin's LOGIN at the sign-in
	// form. Fixed and short on purpose (the familiar "admin"), while the
	// account's EMAIL stays the real person declared in the manifest --
	// email is the key the platform's grants match on; the username is only
	// what gets typed at the door.
	IdentityAdminUsername = "admin"

	// IdentityRealmImportKey is the file name the realm JSON is mounted as;
	// Keycloak imports every *.json in the import directory at startup.
	IdentityRealmImportKey = "planton-realm.json"

	identityRealmImportMountPath = "/opt/keycloak/data/import"

	// IdentityRealmImportHashAnnotation rolls the Keycloak pod when the
	// generated realm import changes (e.g. the public URL moved), since a
	// Secret content change alone does not restart consumers.
	IdentityRealmImportHashAnnotation = "planton.ai/realm-import-hash"

	// IdentityGroupsClaim is the ONE canonical token claim carrying a user's
	// directory group memberships, regardless of federation arm. The control
	// plane's sign-in provisioning reads exactly this claim -- the arm
	// (LDAP-federated realm groups vs a broker-imported attribute) decides
	// which protocol mapper fills it, never which claim it lands in.
	IdentityGroupsClaim = "groups"

	// IdentityGroupsMapperName names the protocol mapper emitting
	// IdentityGroupsClaim on the sign-in clients. The mapper's TYPE is
	// arm-specific (group-membership for LDAP, user-attribute for brokering),
	// so it is federation state -- provisioned with the federation and swept
	// with it, owned by this constant name (protocol mappers carry no
	// attributes; the name is the mark).
	IdentityGroupsMapperName = "planton-directory-groups"

	// IdentityDirectoryGroupsAttribute is the user attribute the broker arm's
	// claim importer fills from the upstream groups claim; the groups
	// protocol mapper reads it back out into IdentityGroupsClaim. LDAP-arm
	// users never carry it (their groups live as realm groups).
	IdentityDirectoryGroupsAttribute = "directory-groups"

	// IdentityDirectorySubjectClaim is the ONE canonical token claim carrying
	// the directory's OWN stable id for a federated user (AD objectGUID /
	// Entra object id), regardless of arm. Its presence is what makes a user
	// directory-born BY CONSTRUCTION -- every federated user carries it,
	// local realm users never do -- and the control plane's sign-in
	// provisioning stamps it as the account's origin_ref: the correlation
	// key directory sync and the offboarding guarantee run on (the realm's
	// sub is Keycloak's id, not the directory's).
	IdentityDirectorySubjectClaim = "directory_subject"

	// IdentityDirectorySubjectMapperName names the protocol mapper emitting
	// IdentityDirectorySubjectClaim on the sign-in clients. Federation state
	// like the groups mapper: provisioned with the federation, swept with it,
	// owned by this constant name.
	IdentityDirectorySubjectMapperName = "planton-directory-subject"

	// IdentityDirectorySubjectAttribute is the user attribute the broker
	// arm's subject importer fills from the upstream's stable-subject claim;
	// the subject protocol mapper reads it back out. LDAP-arm users never
	// carry it -- their directory id lives in Keycloak's own LDAP_ID
	// attribute, which the LDAP arm's mapper reads directly.
	IdentityDirectorySubjectAttribute = "directory-subject"

	// IdentityLDAPComponentName names the operator-provisioned LDAP
	// user-federation component on the realm.
	IdentityLDAPComponentName = "planton-directory"

	// IdentityBrokerAlias is the identity-broker instance's alias. It is a
	// CONSTANT, never derived from the manifest's name: the alias appears in
	// the broker redirect URI the company registers on its upstream identity
	// provider ({front-door}/idp/realms/{realm}/broker/{alias}/endpoint), so
	// a rename would silently break the upstream registration.
	IdentityBrokerAlias = "corp-directory"

	// IdentityDirectoryGroupsPath is the realm-group subtree the LDAP group
	// mapper syncs directory groups into. Namespacing mirrored groups under
	// one parent keeps drop-non-existing sync semantics scoped to directory
	// state and structurally away from admin-created realm groups.
	IdentityDirectoryGroupsPath = "/directory"

	// IdentityCredentialHashConfigKey carries a non-secret fingerprint of the
	// last-written credential on federation objects. Keycloak masks secret
	// config values on read (**********), so the credential itself cannot be
	// diffed -- the hash is what makes credential rotation converge
	// deterministically instead of either writing every pass or never.
	IdentityCredentialHashConfigKey = "planton.ai/credential-hash"

	// IdentityCATruststorePath is where the LDAP CA bundle (when a manifest
	// declares one) is mounted in the Keycloak pod; KC_TRUSTSTORE_PATHS
	// points here so the server trusts the directory's private CA.
	IdentityCATruststorePath = "/opt/keycloak/conf/ldap-ca"

	// IdentityCABundleFileName is the CA bundle's file name under the mount.
	IdentityCABundleFileName = "ldap-ca.crt"

	// IdentityCABundleHashAnnotation rolls the Keycloak pod when the mounted
	// LDAP CA bundle changes -- truststores are read at server start.
	IdentityCABundleHashAnnotation = "planton.ai/ldap-ca-hash"

	// IdentityEmailCATruststorePath is where the mail relay's CA bundle (when
	// spec.email.smtp.caBundleSecretRef declares one) is mounted; it joins
	// KC_TRUSTSTORE_PATHS beside the directory CA, because Keycloak's SMTP
	// sender verifies the relay's certificate against the server truststore.
	IdentityEmailCATruststorePath = "/opt/keycloak/conf/email-ca"

	// IdentityEmailCABundleFileName is the relay CA bundle's file name under
	// its mount.
	IdentityEmailCABundleFileName = "email-ca.crt"

	// IdentityEmailCAHashAnnotation rolls the Keycloak pod when the mounted
	// relay CA bundle changes -- the one restart email delivery ever costs
	// the identity server; a rotated relay password never rolls it.
	IdentityEmailCAHashAnnotation = "planton.ai/email-ca-hash"
)

// IdentityUsersServiceAccountRoles are the realm-management roles the users
// client's service account holds -- exactly user lifecycle, nothing broader.
// One derivation shared by the realm import and the realm reconciler.
func IdentityUsersServiceAccountRoles() []string {
	return []string{"manage-users", "view-users"}
}

// IdentityConsoleRedirectURIs are the console client's exact redirect URIs,
// derived from the CURRENT front door each reconcile pass: the sign-in
// stack's own OAuth callback, and the device-auth broker's callback --
// desktop and CLI sign-ins ride the same confidential client through the
// broker, so its callback must be registered or Keycloak refuses the
// device flow with invalid_redirect_uri before the login form ever renders.
// Exact URIs, no wildcards: the paths are fixed by the console web app.
func IdentityConsoleRedirectURIs(publicURL string) []string {
	return []string{
		publicURL + IdentityConsoleCallbackPath,
		publicURL + IdentityDeviceCallbackPath,
	}
}

// IdentityCLIRedirectURIs are the CLI client's exact loopback redirect URIs.
// The CLI's direct-PKCE flow binds a FIXED port (its default callback URL is
// localhost:8088/auth/callback);
// Keycloak has no port wildcards, so exact URIs are both the only option and
// the tightest one. Both host spellings are registered because loopback
// resolvers differ. If the CLI's callback contract ever moves, this list and
// that file move together.
func IdentityCLIRedirectURIs() []string {
	return []string{
		"http://localhost:8088/auth/callback",
		"http://127.0.0.1:8088/auth/callback",
	}
}

// IdentityConfig bundles all inputs needed to build the identity server
// Deployment.
type IdentityConfig struct {
	CRName    string
	Namespace string
	OwnerRef  *metav1.OwnerReference

	ImageRepository string
	ImageTag        string

	// Realm is the Keycloak realm holding Planton's users and client.
	Realm string

	// PublicURL is the platform's browser-facing URL; the identity server is
	// served under IdentityPathPrefix on the same hostname.
	PublicURL string

	// RealmImportHash fingerprints the generated realm import content so the
	// pod restarts (and re-runs the import) when it changes.
	RealmImportHash string

	// ThemeHash fingerprints the login-theme content (see identity_theme.go)
	// so a theme change rolls the pod past Keycloak's theme cache.
	ThemeHash string

	// CABundleSecretName/CABundleSecretKey mount a private CA bundle (from a
	// bound identity-provider manifest's caBundleSecretRef) into Keycloak's
	// truststore -- LDAPS against a corporate CA is unverifiable without it.
	// Empty means no bundle: KC_TRUSTSTORE_PATHS is not set and the JVM's
	// default CAs cover public-CA directories.
	CABundleSecretName string
	CABundleSecretKey  string

	// CABundleHash fingerprints the CA content so a rotated CA rolls the pod
	// (truststores are read at server start, never re-read live).
	CABundleHash string

	// EmailCABundleSecretName/EmailCABundleSecretKey mount the mail relay's
	// private CA bundle (spec.email.smtp.caBundleSecretRef) into Keycloak's
	// truststore so password-reset emails can be sent through a relay behind
	// a corporate CA. Empty means the relay's certificate chains to a public
	// root (or no email is declared). EmailCABundleHash rolls the pod when
	// the bundle changes, the same way CABundleHash does.
	EmailCABundleSecretName string
	EmailCABundleSecretKey  string
	EmailCABundleHash       string

	PostgreSQL PostgreSQLConnectionInfo
}

// identityCAMount is one private CA the identity server must trust: which
// Secret key holds the PEM bundle, where it is mounted and under what file
// name, and the pod annotation that rolls the server when the bundle
// changes. Both CA arms (directory, mail relay) render through this one
// shape so a third would be a fourth line, not a fourth block.
type identityCAMount struct {
	secretName string
	secretKey  string
	hash       string

	volumeName     string
	mountPath      string
	fileName       string
	hashAnnotation string
}

// IdentityDeploymentName returns the Deployment name: "{crName}-identity".
func IdentityDeploymentName(crName string) string {
	return fmt.Sprintf("%s-identity", crName)
}

// IdentityServiceName returns the Service name: "{crName}-identity".
func IdentityServiceName(crName string) string {
	return fmt.Sprintf("%s-identity", crName)
}

// IdentityBootstrapAdminSecretName holds the Keycloak bootstrap (master
// realm) admin password: "{crName}-identity-bootstrap-admin".
func IdentityBootstrapAdminSecretName(crName string) string {
	return fmt.Sprintf("%s-identity-bootstrap-admin", crName)
}

// IdentityAdminUserSecretName holds the seeded Planton admin user's login and
// one-time password: "{crName}-identity-admin-user".
func IdentityAdminUserSecretName(crName string) string {
	return fmt.Sprintf("%s-identity-admin-user", crName)
}

// IdentityOIDCClientSecretName holds the console OIDC client secret:
// "{crName}-identity-oidc-client".
func IdentityOIDCClientSecretName(crName string) string {
	return fmt.Sprintf("%s-identity-oidc-client", crName)
}

// IdentityUsersClientSecretName holds the user-directory client's secret:
// "{crName}-identity-users-client".
func IdentityUsersClientSecretName(crName string) string {
	return fmt.Sprintf("%s-identity-users-client", crName)
}

// IdentityRealmStateSecretName holds the operator's own bookkeeping about
// what it last wrote to the identity server's realm: the fingerprint of each
// credential it handed over (Keycloak masks secret config on read, so a
// rotation is detectable only against a record) and the last-discovered OIDC
// endpoints (so a hand-deleted broker can be recreated without re-fetching
// discovery on the reconcile cadence). One record for the whole realm, read
// once and written once per reconcile pass -- two writers of one Secret under
// server-side apply would each drop the other's keys. A Secret rather than a
// ConfigMap because a fingerprint, while not a credential, is derived from
// one: "{crName}-identity-realm-state".
func IdentityRealmStateSecretName(crName string) string {
	return fmt.Sprintf("%s-identity-realm-state", crName)
}

// The realm-state Secret's data keys: the directory federation credential's
// fingerprint, the broker's discovered endpoints, and the mail relay
// credential's fingerprint.
const (
	IdentityRealmStateFederationCredentialKey = "federation-credential-sha256"
	IdentityRealmStateOIDCEndpointsKey        = "oidc-endpoints"
	IdentityRealmStateEmailCredentialKey      = "email-credential-sha256"
)

// IdentityFederationFactsConfigMapName holds the federation facts the
// operator ADVERTISES to the product: which federation arm the bound
// identity manifest declares and the verification verdicts the federation
// reconcile recorded. Deliberately a ConfigMap beside the state Secret --
// the state Secret is the operator's PRIVATE bookkeeping (credential
// fingerprints), while this is a published projection the control plane
// mounts and serves to admins. It exists on EVERY install (content says
// configured=false when no manifest binds), so the control plane's mount
// never depends on late-created-volume semantics:
// "{crName}-identity-federation-facts".
func IdentityFederationFactsConfigMapName(crName string) string {
	return fmt.Sprintf("%s-identity-federation-facts", crName)
}

// IdentityFederationFactsKey is the single data key inside the facts
// ConfigMap; IdentityFederationFactsMountPath is where the control plane
// mounts the ConfigMap (the WHOLE directory -- a subPath mount would freeze
// kubelet's in-place content updates, and live updates are the point).
const (
	IdentityFederationFactsKey       = "facts.json"
	IdentityFederationFactsMountPath = "/etc/planton/identity-federation"
)

// IdentityFederationFactsFilePath is the in-container path of the facts
// file, injected into the control plane as a STATIC env var -- the value
// never changes, so facts updates never roll the pod.
func IdentityFederationFactsFilePath() string {
	return IdentityFederationFactsMountPath + "/" + IdentityFederationFactsKey
}

// IdentityFederationFacts is the operator→product projection contract. The
// control plane parses exactly this shape (its reader pins the same fixture
// its own tests use), so a field change here is a contract change: update
// both sides in the same change and keep every field adopter-readable --
// verdict messages land verbatim on admin screens.
type IdentityFederationFacts struct {
	// Configured is false when no identity manifest is bound on this
	// install -- the product renders its connect journey, not health.
	Configured bool `json:"configured"`
	// Arm is "ldap" (Active Directory user federation) or "oidc"
	// (identity brokering); empty when not configured.
	Arm string `json:"arm,omitempty"`
	// ProviderLabel is the name admins know the directory by (the
	// sign-in button label, defaulted arm-appropriately).
	ProviderLabel string `json:"providerLabel,omitempty"`
	// Provisioned mirrors the manifest's Provisioned condition: federation
	// exists on the identity server exactly as declared.
	Provisioned bool `json:"provisioned,omitempty"`
	// ObservedAt is when the verification verdicts below were recorded
	// (RFC 3339). Preserved across projections; re-stamped only when a
	// verification pass actually ran.
	ObservedAt string `json:"observedAt,omitempty"`
	// Checks are the verification verdicts, verbatim from the manifest's
	// status: Verdict is "Passed" | "Failed" | "Unknown".
	Checks []IdentityFederationFactsCheck `json:"checks,omitempty"`
}

// IdentityFederationFactsCheck is one projected verification verdict.
type IdentityFederationFactsCheck struct {
	Name    string `json:"name"`
	Verdict string `json:"verdict"`
	Message string `json:"message,omitempty"`
}

// IdentityFederationFactsConfigMap renders the facts ConfigMap.
func IdentityFederationFactsConfigMap(crName, namespace string, facts IdentityFederationFacts, ownerRef *metav1.OwnerReference) (*corev1.ConfigMap, error) {
	payload, err := json.Marshal(facts)
	if err != nil {
		return nil, fmt.Errorf("marshaling federation facts: %w", err)
	}
	configMap := &corev1.ConfigMap{
		TypeMeta: metav1.TypeMeta{APIVersion: "v1", Kind: "ConfigMap"},
		ObjectMeta: metav1.ObjectMeta{
			Name:      IdentityFederationFactsConfigMapName(crName),
			Namespace: namespace,
		},
		Data: map[string]string{IdentityFederationFactsKey: string(payload)},
	}
	if ownerRef != nil {
		configMap.OwnerReferences = []metav1.OwnerReference{*ownerRef}
	}
	return configMap, nil
}

// IdentitySetupCodeSecretName holds the first-run setup code -- the
// cluster-access proof the console's setup page asks for before creating the
// first admin: "{crName}-identity-setup-code".
func IdentitySetupCodeSecretName(crName string) string {
	return fmt.Sprintf("%s-identity-setup-code", crName)
}

// IdentitySetupCodeSecretKey is the data key inside the setup-code Secret.
const IdentitySetupCodeSecretKey = "setup-code"

// IdentitySetupCodeHint renders the exact command a person copies to read the
// setup code -- surfaced through the control plane to the console's setup
// page, so no UI ever hardcodes deployment names.
func IdentitySetupCodeHint(crName, namespace string) string {
	return fmt.Sprintf(
		"kubectl -n %s get secret %s -o jsonpath='{.data.%s}' | base64 -d",
		namespace, IdentitySetupCodeSecretName(crName), IdentitySetupCodeSecretKey)
}

// IdentityRealmImportSecretName holds the generated realm import JSON:
// "{crName}-identity-realm-import". A Secret (not a ConfigMap) because the
// import carries the OIDC client secret and the admin user's initial password.
func IdentityRealmImportSecretName(crName string) string {
	return fmt.Sprintf("%s-identity-realm-import", crName)
}

// IdentityOneTimePasswordAnnotation marks the admin-user credentials Secret
// as self-describing: the password inside is one-time, and nothing else at
// the point of reading says so. Without this note, a person reading the
// Secret after the admin's first sign-in gets a stale password and a bare
// "invalid username or password" at the sign-in form.
const IdentityOneTimePasswordAnnotation = "planton.ai/one-time-password"

// IdentityOneTimePasswordNote renders the annotation's plain-language
// explanation: the one-time semantics and the exact recovery path.
func IdentityOneTimePasswordNote(crName, publicURL string) string {
	return fmt.Sprintf(
		"The password in this Secret is one-time: the admin sets their own at first sign-in, "+
			"after which this value stops working. If sign-in fails with 'invalid username or password', "+
			"the password was already consumed -- manage users in the identity server's admin console at %s%s "+
			"(credentials in Secret %s).",
		publicURL, IdentityPathPrefix, IdentityBootstrapAdminSecretName(crName))
}

// IdentityOIDCClientSecretKey is the data key in the OIDC client Secret.
const IdentityOIDCClientSecretKey = "client-secret"

// IdentityIssuerURL returns the OIDC issuer for the platform realm. This
// exact string is what Keycloak stamps into the `iss` claim and what the
// console's sign-in stack redirects the browser through -- the ADVERTISED
// horizon. In-cluster fetches (JWKS, token, userinfo) use the internal URL
// below instead; both must name the same realm.
func IdentityIssuerURL(publicURL, realm string) string {
	return fmt.Sprintf("%s%s/realms/%s", publicURL, IdentityPathPrefix, realm)
}

// IdentityInternalIssuerURL returns the in-cluster address of the same issuer,
// served by the identity Service -- the INTERNAL horizon. Keycloak's dynamic
// backchannel (KC_HOSTNAME_BACKCHANNEL_DYNAMIC) makes discovery fetched here
// return in-cluster endpoint URLs while tokens keep the advertised issuer, so
// nothing inside the cluster ever needs to reach the public hostname (no
// hairpin requirement) -- and in gateway mode the advertised localhost URL is
// not in-cluster-reachable at all.
func IdentityInternalIssuerURL(crName, namespace, realm string) string {
	return fmt.Sprintf("http://%s.%s.svc.cluster.local%s/realms/%s",
		IdentityServiceName(crName), namespace, IdentityPathPrefix, realm)
}

// IdentityInternalServerRootURL is the in-cluster address of the identity
// SERVER itself (not a realm): the base the admin API and the master-realm
// token endpoint hang off. The realm reconciler talks here -- the same
// split-horizon backchannel as the issuer above -- so reconciliation works
// before DNS exists, in port-forward mode, and on egress-restricted clusters.
func IdentityInternalServerRootURL(crName, namespace string) string {
	return fmt.Sprintf("http://%s.%s.svc.cluster.local%s",
		IdentityServiceName(crName), namespace, IdentityPathPrefix)
}

// identityRealmImport models the subset of Keycloak's realm import format the
// operator provisions. Only fields the platform depends on are set; realm
// defaults cover the rest (default client scopes, auth flows, and so on).
type identityRealmImport struct {
	Realm                string `json:"realm"`
	Enabled              bool   `json:"enabled"`
	DisplayName          string `json:"displayName"`
	DisplayNameHTML      string `json:"displayNameHtml"`
	LoginTheme           string `json:"loginTheme"`
	SSLRequired          string `json:"sslRequired"`
	RegistrationAllowed  bool   `json:"registrationAllowed"`
	SSOSessionIdleTimout int    `json:"ssoSessionIdleTimeout"`
	AccessTokenLifespan  int    `json:"accessTokenLifespan"`
	// The email off-state, the only email posture the import ever bakes:
	// "Forgot password?" off and no relay. The platform's email declaration
	// itself is converged onto the live realm by the reconciler seconds after
	// first boot, never imported -- the import is create-only and rides a
	// Secret whose content hash rolls the pod, so a relay password in it
	// would roll the identity server on every rotation for nothing.
	ResetPasswordAllowed bool                  `json:"resetPasswordAllowed"`
	SMTPServer           map[string]string     `json:"smtpServer"`
	Clients              []identityRealmClient `json:"clients"`
	Users                []identityRealmUser   `json:"users"`
}

type identityRealmClient struct {
	ClientID                  string                        `json:"clientId"`
	Name                      string                        `json:"name"`
	Enabled                   bool                          `json:"enabled"`
	Protocol                  string                        `json:"protocol"`
	PublicClient              bool                          `json:"publicClient"`
	ClientAuthenticatorType   string                        `json:"clientAuthenticatorType"`
	Secret                    string                        `json:"secret"`
	StandardFlowEnabled       bool                          `json:"standardFlowEnabled"`
	ImplicitFlowEnabled       bool                          `json:"implicitFlowEnabled"`
	DirectAccessGrantsEnabled bool                          `json:"directAccessGrantsEnabled"`
	ServiceAccountsEnabled    bool                          `json:"serviceAccountsEnabled"`
	RedirectURIs              []string                      `json:"redirectUris"`
	Attributes                map[string]string             `json:"attributes"`
	DefaultClientScopes       []string                      `json:"defaultClientScopes"`
	OptionalClientScopes      []string                      `json:"optionalClientScopes"`
	ProtocolMappers           []identityRealmProtocolMapper `json:"protocolMappers"`
}

type identityRealmProtocolMapper struct {
	Name            string            `json:"name"`
	Protocol        string            `json:"protocol"`
	ProtocolMapper  string            `json:"protocolMapper"`
	ConsentRequired bool              `json:"consentRequired"`
	Config          map[string]string `json:"config"`
}

type identityRealmUser struct {
	Username        string                    `json:"username"`
	Email           string                    `json:"email,omitempty"`
	Enabled         bool                      `json:"enabled"`
	EmailVerified   bool                      `json:"emailVerified,omitempty"`
	RealmRoles      []string                  `json:"realmRoles,omitempty"`
	RequiredActions []string                  `json:"requiredActions,omitempty"`
	Credentials     []identityRealmCredential `json:"credentials,omitempty"`
	// ServiceAccountClientId marks this entry as an OAuth client's service
	// account instead of a person; clientRoles is how the import grants it
	// realm-management roles (admin-UI role assignment would violate the
	// everything-declarative contract).
	ServiceAccountClientID string              `json:"serviceAccountClientId,omitempty"`
	ClientRoles            map[string][]string `json:"clientRoles,omitempty"`
}

type identityRealmCredential struct {
	Type string `json:"type"`
	// Temporary forces a password change at first sign-in, so the generated
	// secret never remains a long-lived credential.
	Temporary bool   `json:"temporary"`
	Value     string `json:"value"`
}

// IdentityRealmImportConfig carries the inputs for the generated realm.
type IdentityRealmImportConfig struct {
	Realm         string
	PublicURL     string
	ClientSecret  string
	UsersSecret   string
	AdminEmail    string
	AdminPassword string
}

// IdentityAudienceProtocolMapperConfig is the oidc-audience-mapper config
// stamping IDPAPIAudience into access tokens. Keycloak does NOT put a custom
// API audience in access tokens by default -- without this mapper the control
// plane's audience validator rejects every token. One derivation shared by
// the realm import and the realm reconciler's owned set.
func IdentityAudienceProtocolMapperConfig() map[string]string {
	return map[string]string{
		"included.custom.audience": IDPAPIAudience,
		"access.token.claim":       "true",
		"id.token.claim":           "false",
	}
}

// identityAudienceMapper renders the shared audience mapper in the realm
// import's shape.
func identityAudienceMapper() identityRealmProtocolMapper {
	return identityRealmProtocolMapper{
		Name:            IdentityAudienceMapperName,
		Protocol:        "openid-connect",
		ProtocolMapper:  "oidc-audience-mapper",
		ConsentRequired: false,
		Config:          IdentityAudienceProtocolMapperConfig(),
	}
}

// IdentityRealmImport renders the realm import JSON Keycloak consumes at
// FIRST boot (--import-realm; Keycloak skips realms that already exist). The
// import is the fast, offline, race-free bootstrap -- the CONTRACT for realm
// state is the realm reconciler (internal/keycloak), which converges the
// operator-owned set on the live realm every reconcile and never touches
// admin-created state. Both derive from the same shared constants above, so
// what the import bakes and what the reconciler converges cannot disagree
// (pinned by TestIdentityRealmImportAgreesWithOwnedSet in internal/keycloak).
func IdentityRealmImport(cfg IdentityRealmImportConfig) ([]byte, error) {
	realm := identityRealmImport{
		Realm:   cfg.Realm,
		Enabled: true,
		// The product name, not the realm slug: displayName is the page
		// <title> at the sign-in form; displayNameHtml is the header text,
		// which the Planton login theme replaces with the logo mark via CSS.
		DisplayName:     "Planton",
		DisplayNameHTML: "Planton",
		// The Planton login theme (operator-provisioned ConfigMap; see
		// identity_theme.go). Set per-realm here for fresh installs; the
		// server-wide default-theme flag on the Deployment covers realms
		// that predate the theme in this import.
		LoginTheme: keycloaklogintheme.ThemeName,
		// The friction ladder's first step serves plain HTTP on a derived
		// hostname; Keycloak's default sslRequired=external would refuse those
		// logins outright. The ingress owns transport security -- when TLS is
		// configured, every login already rides HTTPS end to end.
		SSLRequired: "none",
		// No self-registration: the realm is the access boundary. Admins
		// create users (Keycloak admin console under the same hostname).
		RegistrationAllowed:  false,
		SSOSessionIdleTimout: IdentitySSOSessionIdleSeconds,
		AccessTokenLifespan:  IdentityAccessTokenLifespanSeconds,
		// Email off until the reconciler converges the platform's declaration
		// (see the struct's comment); an empty map, never null, so the import
		// and the owned set compare equal.
		ResetPasswordAllowed: false,
		SMTPServer:           map[string]string{},
		Clients: []identityRealmClient{{
			ClientID:                  IdentityConsoleClientID,
			Name:                      "Planton Console",
			Enabled:                   true,
			Protocol:                  "openid-connect",
			PublicClient:              false,
			ClientAuthenticatorType:   "client-secret",
			Secret:                    cfg.ClientSecret,
			StandardFlowEnabled:       true,
			ImplicitFlowEnabled:       false,
			DirectAccessGrantsEnabled: false,
			ServiceAccountsEnabled:    false,
			// Exact URIs, no wildcards (IdentityConsoleRedirectURIs); the
			// signout flow returns to the login page.
			RedirectURIs: IdentityConsoleRedirectURIs(cfg.PublicURL),
			Attributes: map[string]string{
				IdentityPostLogoutAttribute: cfg.PublicURL + IdentityConsolePostLogoutPath,
				IdentityManagedAttribute:    "true",
			},
			// Scopes are explicit because realm import does NOT reliably give
			// an imported client the realm's usual defaults. offline_access is
			// load-bearing: the console requests it so sessions survive access
			// token expiry (proven live -- without it the token exchange fails
			// with "Offline tokens not allowed for the user or client").
			DefaultClientScopes:  []string{"acr", "basic", "email", "profile", "roles", "web-origins"},
			OptionalClientScopes: []string{"offline_access"},
			ProtocolMappers:      []identityRealmProtocolMapper{identityAudienceMapper()},
		}, {
			// The user-directory client: client-credentials only, and its
			// token is only good for user lifecycle -- the service-account
			// role mapping below is the entire privilege. No audience mapper:
			// this token talks to the identity server's own admin API, never
			// to Planton.
			ClientID:                  IdentityUsersClientID,
			Name:                      "Planton User Directory",
			Enabled:                   true,
			Protocol:                  "openid-connect",
			PublicClient:              false,
			ClientAuthenticatorType:   "client-secret",
			Secret:                    cfg.UsersSecret,
			StandardFlowEnabled:       false,
			ImplicitFlowEnabled:       false,
			DirectAccessGrantsEnabled: false,
			ServiceAccountsEnabled:    true,
			RedirectURIs:              []string{},
			Attributes: map[string]string{
				IdentityManagedAttribute: "true",
			},
			DefaultClientScopes:  []string{"basic", "roles"},
			OptionalClientScopes: []string{},
			ProtocolMappers:      []identityRealmProtocolMapper{},
		}, {
			// The CLI's sign-in client: public (a CLI binary cannot hold a
			// secret) with PKCE S256 enforced, which is what makes a public
			// client safe. Standard flow only; the token carries the same
			// API audience the console's does, because both talk to the same
			// control plane.
			ClientID:                  IdentityCLIClientID,
			Name:                      "Planton CLI",
			Enabled:                   true,
			Protocol:                  "openid-connect",
			PublicClient:              true,
			ClientAuthenticatorType:   "client-secret",
			StandardFlowEnabled:       true,
			ImplicitFlowEnabled:       false,
			DirectAccessGrantsEnabled: false,
			ServiceAccountsEnabled:    false,
			RedirectURIs:              IdentityCLIRedirectURIs(),
			Attributes: map[string]string{
				IdentityPKCEMethodAttribute: IdentityPKCEMethodS256,
				IdentityManagedAttribute:    "true",
			},
			// offline_access is default (not optional) here: the CLI always
			// requests it so `planton login` survives access-token expiry
			// without re-prompting -- the scope list mirrors the one the
			// CLI's Keycloak provider requests.
			DefaultClientScopes:  []string{"acr", "basic", "email", "profile", "roles", "web-origins", "offline_access"},
			OptionalClientScopes: []string{},
			ProtocolMappers:      []identityRealmProtocolMapper{identityAudienceMapper()},
		}},
		// The user-directory client's service account, granted exactly the
		// realm's user-management roles (proven live: an import-declared
		// service-account user with these clientRoles can create users
		// through the admin API; without the entry the service account
		// exists but holds no roles). No PEOPLE are seeded here unless an
		// admin email is declared below -- there is deliberately no generic
		// pre-baked identity; the first-run setup flow (or the declared
		// email) is how the realm gets its first person.
		Users: []identityRealmUser{{
			Username:               "service-account-" + IdentityUsersClientID,
			Enabled:                true,
			ServiceAccountClientID: IdentityUsersClientID,
			ClientRoles: map[string][]string{
				IdentityRealmManagementClientID: IdentityUsersServiceAccountRoles(),
			},
		}},
	}

	if cfg.AdminEmail != "" {
		realm.Users = append(realm.Users, identityRealmUser{
			// The LOGIN is the short, obvious "admin" (the sign-in form
			// accepts username or email); the EMAIL stays the declared real
			// person's -- it is the key every downstream grant matches on
			// (account creation, org ownership, the platform-operator role).
			// This is a login convenience, not a generic identity: the
			// account is still the manifest-named person.
			Username:      IdentityAdminUsername,
			Email:         cfg.AdminEmail,
			Enabled:       true,
			EmailVerified: true,
			// No name is baked in -- the admin is a real person, and
			// UPDATE_PROFILE (below) asks THEM for their name at first
			// sign-in, on the same screen as the forced password change.
			//
			// IMPORTED users get exactly the roles listed here -- unlike
			// admin-created users, the realm's default-role composite is not
			// applied. offline_access is load-bearing: without the role the
			// token exchange fails (proven live), because the console always
			// requests an offline session for silent refresh.
			RealmRoles: []string{"default-roles-" + cfg.Realm, "offline_access"},
			// The credential's temporary flag alone does not survive realm
			// import (proven live: no password prompt appeared); the required
			// actions are what actually run at first sign-in.
			RequiredActions: []string{"UPDATE_PASSWORD", "UPDATE_PROFILE"},
			Credentials: []identityRealmCredential{{
				Type:      "password",
				Temporary: true,
				Value:     cfg.AdminPassword,
			}},
		})
	}

	data, err := json.MarshalIndent(realm, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("marshaling realm import: %w", err)
	}
	return data, nil
}

// IdentityDeployment builds the Keycloak Deployment: an init step that
// ensures Keycloak's database exists, then the server itself configured
// entirely by environment (no admin-UI setup), importing the generated realm
// at startup.
func IdentityDeployment(cfg IdentityConfig) *appsv1.Deployment {
	imageRepo := cfg.ImageRepository
	if imageRepo == "" {
		imageRepo = IdentityDefaultImageRepo
	}
	imageTag := cfg.ImageTag
	if imageTag == "" {
		imageTag = IdentityDefaultImageTag
	}

	labels := map[string]string{
		"app.kubernetes.io/name":       "identity",
		"app.kubernetes.io/instance":   cfg.CRName,
		"app.kubernetes.io/managed-by": ManagedByLabel,
		"app.kubernetes.io/component":  "application",
	}

	replicas := int32(1)

	// Idempotent CREATE DATABASE: the connection user is the PostgreSQL
	// superuser (the platform-wide single-user contract), so creating the
	// identity database is always within its rights.
	ensureDBScript := fmt.Sprintf(
		`psql -h %q -p %d -U %q -d postgres -tAc "SELECT 1 FROM pg_database WHERE datname = '%s'" | grep -q 1 || psql -h %q -p %d -U %q -d postgres -c 'CREATE DATABASE %q'`,
		cfg.PostgreSQL.Host, cfg.PostgreSQL.Port, cfg.PostgreSQL.User, IdentityDBName,
		cfg.PostgreSQL.Host, cfg.PostgreSQL.Port, cfg.PostgreSQL.User, IdentityDBName,
	)

	envVars := []corev1.EnvVar{
		// Database.
		{Name: "KC_DB", Value: "postgres"},
		{Name: "KC_DB_URL_HOST", Value: cfg.PostgreSQL.Host},
		{Name: "KC_DB_URL_PORT", Value: fmt.Sprintf("%d", cfg.PostgreSQL.Port)},
		{Name: "KC_DB_URL_DATABASE", Value: IdentityDBName},
		{Name: "KC_DB_USERNAME", Value: cfg.PostgreSQL.User},
		secretEnv("KC_DB_PASSWORD", cfg.PostgreSQL.SecretName, cfg.PostgreSQL.PassKey),

		// Master-realm bootstrap admin (operator-generated Secret). Distinct
		// from the seeded Planton admin USER in the platform realm. The same
		// credential authenticates the realm reconciler (internal/keycloak).
		{Name: "KC_BOOTSTRAP_ADMIN_USERNAME", Value: IdentityBootstrapAdminUsername},
		secretEnv("KC_BOOTSTRAP_ADMIN_PASSWORD", IdentityBootstrapAdminSecretName(cfg.CRName), IdentityBootstrapAdminPasswordKey),

		// Serving: plain HTTP in-cluster (the front door terminates TLS when
		// there is any), under the shared hostname's path prefix. KC_HOSTNAME
		// carries the full advertised URL so issuer/redirects are correct
		// behind the proxy.
		{Name: "KC_HTTP_ENABLED", Value: "true"},
		{Name: "KC_HTTP_PORT", Value: fmt.Sprintf("%d", identityHTTPPort)},
		{Name: "KC_HTTP_RELATIVE_PATH", Value: IdentityPathPrefix},
		{Name: "KC_HOSTNAME", Value: cfg.PublicURL + IdentityPathPrefix},
		// Split horizon: browser-facing (frontchannel) URLs stay pinned to
		// KC_HOSTNAME above, while backchannel requests -- discovery, JWKS,
		// token, userinfo fetched by in-cluster callers over the identity
		// Service -- get URLs derived from the request address. Tokens keep
		// the advertised issuer either way, so in-cluster callers validate
		// the advertised issuer while never dialing the public hostname.
		{Name: "KC_HOSTNAME_BACKCHANNEL_DYNAMIC", Value: "true"},
		{Name: "KC_PROXY_HEADERS", Value: "xforwarded"},

		// Health endpoints on the management port; pinned to "/" so probe
		// paths do not move with the serving path prefix above.
		{Name: "KC_HEALTH_ENABLED", Value: "true"},
		{Name: "KC_HTTP_MANAGEMENT_RELATIVE_PATH", Value: "/"},

		// Single replica: skip cluster cache discovery entirely.
		{Name: "KC_CACHE", Value: "local"},
	}

	// Private CAs the server must trust: the directory's (a bound identity
	// manifest) and the mail relay's (spec.email). Each rides its own
	// read-only Secret mount with its own restart hash; KC_TRUSTSTORE_PATHS
	// lists whichever are declared. KC_TRUSTSTORE_PATHS APPENDS to the JVM's
	// default CAs (verified Keycloak 26 semantics), so a private CA never
	// un-trusts public roots.
	caMounts := []identityCAMount{
		{cfg.CABundleSecretName, cfg.CABundleSecretKey, cfg.CABundleHash,
			"ldap-ca", IdentityCATruststorePath, IdentityCABundleFileName, IdentityCABundleHashAnnotation},
		{cfg.EmailCABundleSecretName, cfg.EmailCABundleSecretKey, cfg.EmailCABundleHash,
			"email-ca", IdentityEmailCATruststorePath, IdentityEmailCABundleFileName, IdentityEmailCAHashAnnotation},
	}
	var truststorePaths []string
	for _, ca := range caMounts {
		if ca.secretName != "" {
			truststorePaths = append(truststorePaths, ca.mountPath+"/"+ca.fileName)
		}
	}
	if len(truststorePaths) > 0 {
		envVars = append(envVars, corev1.EnvVar{
			Name:  "KC_TRUSTSTORE_PATHS",
			Value: strings.Join(truststorePaths, ","),
		})
	}

	themeVolume, themeMounts := identityThemeVolume(cfg.CRName)

	podAnnotations := map[string]string{
		IdentityRealmImportHashAnnotation: cfg.RealmImportHash,
		IdentityThemeHashAnnotation:       cfg.ThemeHash,
	}

	volumeMounts := append([]corev1.VolumeMount{{
		Name:      "realm-import",
		MountPath: identityRealmImportMountPath,
		ReadOnly:  true,
	}}, themeMounts...)
	volumes := []corev1.Volume{{
		Name: "realm-import",
		VolumeSource: corev1.VolumeSource{
			Secret: &corev1.SecretVolumeSource{
				SecretName: IdentityRealmImportSecretName(cfg.CRName),
			},
		},
	}, themeVolume}

	for _, ca := range caMounts {
		if ca.secretName == "" {
			continue
		}
		podAnnotations[ca.hashAnnotation] = ca.hash
		volumeMounts = append(volumeMounts, corev1.VolumeMount{
			Name:      ca.volumeName,
			MountPath: ca.mountPath,
			ReadOnly:  true,
		})
		volumes = append(volumes, corev1.Volume{
			Name: ca.volumeName,
			VolumeSource: corev1.VolumeSource{
				Secret: &corev1.SecretVolumeSource{
					SecretName: ca.secretName,
					Items: []corev1.KeyToPath{{
						Key:  ca.secretKey,
						Path: ca.fileName,
					}},
				},
			},
		})
	}

	deploy := &appsv1.Deployment{
		TypeMeta: metav1.TypeMeta{APIVersion: "apps/v1", Kind: "Deployment"},
		ObjectMeta: metav1.ObjectMeta{
			Name:      IdentityDeploymentName(cfg.CRName),
			Namespace: cfg.Namespace,
			Labels:    labels,
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: &replicas,
			Selector: &metav1.LabelSelector{MatchLabels: labels},
			// Recreate: two Keycloaks importing one realm into one database
			// mid-rollout is a conflict, not high availability.
			Strategy: appsv1.DeploymentStrategy{Type: appsv1.RecreateDeploymentStrategyType},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels:      labels,
					Annotations: podAnnotations,
				},
				Spec: corev1.PodSpec{
					InitContainers: []corev1.Container{{
						Name:    "ensure-database",
						Image:   identityDBInitImage,
						Command: []string{"sh", "-c", ensureDBScript},
						Env: []corev1.EnvVar{
							secretEnv("PGPASSWORD", cfg.PostgreSQL.SecretName, cfg.PostgreSQL.PassKey),
						},
					}},
					Containers: []corev1.Container{{
						Name:  "keycloak",
						Image: fmt.Sprintf("%s:%s", imageRepo, imageTag),
						// --spi-theme--default makes the Planton theme the
						// server-wide fallback, so realms that predate the
						// theme (the realm import is create-only) get it
						// without a realm mutation. Verified live on 26.3:
						// the flag is accepted and the login page serves the
						// theme's stylesheet. Freshly imported realms also
						// pin loginTheme explicitly (realm import above).
						Args: []string{"start", "--import-realm",
							"--spi-theme--default=" + keycloaklogintheme.ThemeName},
						Ports: []corev1.ContainerPort{
							{Name: "http", ContainerPort: identityHTTPPort, Protocol: corev1.ProtocolTCP},
							{Name: "management", ContainerPort: identityManagementPort, Protocol: corev1.ProtocolTCP},
						},
						Env:          envVars,
						VolumeMounts: volumeMounts,
						// Explicit floor (the data-layer OOM lesson):
						// a JVM plus first-boot realm import dies confusingly
						// under default limits. No CPU limit so the import is
						// never throttled.
						Resources: corev1.ResourceRequirements{
							Requests: corev1.ResourceList{
								corev1.ResourceCPU:    resource.MustParse("250m"),
								corev1.ResourceMemory: resource.MustParse("512Mi"),
							},
							Limits: corev1.ResourceList{
								corev1.ResourceMemory: resource.MustParse("1536Mi"),
							},
						},
						// First boot runs schema migrations + the realm import;
						// allow a generous window (10s x 60 = 10m) before the
						// kubelet gives up, mirroring the control plane.
						StartupProbe: &corev1.Probe{
							ProbeHandler: corev1.ProbeHandler{
								HTTPGet: &corev1.HTTPGetAction{
									Path: "/health/started",
									Port: intstr.FromInt32(identityManagementPort),
								},
							},
							InitialDelaySeconds: 15,
							PeriodSeconds:       10,
							TimeoutSeconds:      5,
							FailureThreshold:    60,
						},
						ReadinessProbe: &corev1.Probe{
							ProbeHandler: corev1.ProbeHandler{
								HTTPGet: &corev1.HTTPGetAction{
									Path: "/health/ready",
									Port: intstr.FromInt32(identityManagementPort),
								},
							},
							PeriodSeconds:    10,
							TimeoutSeconds:   5,
							FailureThreshold: 3,
						},
						LivenessProbe: &corev1.Probe{
							ProbeHandler: corev1.ProbeHandler{
								HTTPGet: &corev1.HTTPGetAction{
									Path: "/health/live",
									Port: intstr.FromInt32(identityManagementPort),
								},
							},
							PeriodSeconds:    30,
							TimeoutSeconds:   5,
							FailureThreshold: 3,
						},
					}},
					Volumes: volumes,
				},
			},
		},
	}

	if cfg.OwnerRef != nil {
		deploy.OwnerReferences = []metav1.OwnerReference{*cfg.OwnerRef}
	}

	return deploy
}

// IdentityService builds the ClusterIP Service exposing the identity server
// on port 80 (mapped to Keycloak's HTTP port) for the ingress path route.
func IdentityService(crName, namespace string, ownerRef *metav1.OwnerReference) *corev1.Service {
	labels := map[string]string{
		"app.kubernetes.io/name":       "identity",
		"app.kubernetes.io/instance":   crName,
		"app.kubernetes.io/managed-by": ManagedByLabel,
		"app.kubernetes.io/component":  "application",
	}

	svc := &corev1.Service{
		TypeMeta: metav1.TypeMeta{APIVersion: "v1", Kind: "Service"},
		ObjectMeta: metav1.ObjectMeta{
			Name:      IdentityServiceName(crName),
			Namespace: namespace,
			Labels:    labels,
		},
		Spec: corev1.ServiceSpec{
			Type:     corev1.ServiceTypeClusterIP,
			Selector: labels,
			Ports: []corev1.ServicePort{{
				Name:       "http",
				Port:       identityServicePort,
				TargetPort: intstr.FromInt32(identityHTTPPort),
				Protocol:   corev1.ProtocolTCP,
			}},
		},
	}

	if ownerRef != nil {
		svc.OwnerReferences = []metav1.OwnerReference{*ownerRef}
	}

	return svc
}
