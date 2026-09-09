package resources

import (
	"strings"
	"testing"

	corev1 "k8s.io/api/core/v1"
)

func testControlPlaneConfig() ControlPlaneConfig {
	// OpenFGA is deliberately absent: a populated connection MEANS the
	// authorization component is enabled and the real engine gets wired.
	// The minimal footprint runs none.
	//
	// The identity binding is always present -- every install signs in
	// (there is no unauthenticated arm), so a config without one is not a
	// state the component can produce.
	return ControlPlaneConfig{
		CRName:     "planton",
		Namespace:  "default",
		Version:    "v1.0.0",
		Replicas:   1,
		PostgreSQL: PostgreSQLConnection("planton", "default"),
		Redis:      RedisConnection("planton", "default"),
		Temporal:   TemporalConnection("planton", "default"),
		Identity:   testIdentityBinding(),
		Storage:    testStorageBinding(),
	}
}

func testStorageBinding() *StorageBinding {
	return &StorageBinding{
		RelayPublicBaseURL:   "http://planton.example.com",
		RelayInternalBaseURL: "http://planton-control-plane.default.svc.cluster.local:8081",
	}
}

func testIdentityBinding() *IdentityBinding {
	return &IdentityBinding{
		IssuerURL:             "http://planton.example.com/idp/realms/planton",
		InternalIssuerURL:     "http://planton-identity.default.svc.cluster.local/idp/realms/planton",
		Hostname:              "planton.example.com",
		UsersClientSecretName: "planton-identity-users-client",
		AuthorizationProvider: "allow-authenticated",
		Bootstrap:             BootstrapBinding{OrgSlug: "default", OrgName: "default", EnvSlug: "default", EnvName: "default"},
	}
}

// The object-storage capability rides the platform's own Postgres: the
// provider selector plus the two split-horizon relay bases, and NO Cloudflare
// R2 variables anywhere -- the dead-placeholder pattern (localhost:9000
// credentials that fail at first use) must never come back.
func TestControlPlaneDeployment_StorageProviderEnvVars(t *testing.T) {
	deploy := ControlPlaneDeployment(testControlPlaneConfig())
	envMap := envVarMap(deploy.Spec.Template.Spec.Containers[0].Env)

	if v := envMap["PLANTON_STORAGE_PROVIDER"]; v != "postgres" {
		t.Errorf("PLANTON_STORAGE_PROVIDER = %q, want postgres", v)
	}
	if v := envMap["PLANTON_STORAGE_RELAY_PUBLIC_BASE_URL"]; v != "http://planton.example.com" {
		t.Errorf("relay public base = %q, want the advertised front-door URL", v)
	}
	if v := envMap["PLANTON_STORAGE_RELAY_INTERNAL_BASE_URL"]; v != "http://planton-control-plane.default.svc.cluster.local:8081" {
		t.Errorf("relay internal base = %q, want the control plane's grpc-web Service address", v)
	}
	for name := range envMap {
		if strings.HasPrefix(name, "CLOUDFLARE_R2_") {
			t.Errorf("%s must not be set: the postgres storage arm needs no R2 config", name)
		}
	}
}

// The AWS browser-setup flow (CloudFormation quick-create) is declared OFF and
// its integration env is omitted entirely -- the dead-placeholder pattern (a
// 000000000000 account id and http://localhost template URLs that mint broken
// AWS console links at first use) must never come back. Keyless is NOT
// declared here -- it is the front door's verdict, rendered from the
// WebIdentity binding (see the posture tests below).
func TestControlPlaneDeployment_AwsConnectionMethodEnvVars(t *testing.T) {
	deploy := ControlPlaneDeployment(testControlPlaneConfig())
	envMap := envVarMap(deploy.Spec.Template.Spec.Containers[0].Env)

	if v := envMap["AWS_CLOUDFORMATION_ENABLED"]; v != "false" {
		t.Errorf("AWS_CLOUDFORMATION_ENABLED = %q, want false", v)
	}
	for _, placeholder := range []string{
		"AWS_CLOUDFORMATION_CALLBACK_URL",
		"AWS_CLOUDFORMATION_SETUP_TTL_MINUTES",
		"AWS_CLOUDFORMATION_STACK_NAME_PREFIX",
		"AWS_CLOUDFORMATION_TEMPLATE_URL",
	} {
		if v, ok := envMap[placeholder]; ok {
			t.Errorf("%s must not be set (placeholder rot); got %q", placeholder, v)
		}
	}
}

func TestControlPlaneRelayInternalBaseURL(t *testing.T) {
	got := ControlPlaneRelayInternalBaseURL("planton", "planton-ns")
	want := "http://planton-control-plane.planton-ns.svc.cluster.local:8081"
	if got != want {
		t.Errorf("relay internal base URL = %q, want %q", got, want)
	}
}

func TestControlPlaneDeployment_Image(t *testing.T) {
	cfg := testControlPlaneConfig()
	deploy := ControlPlaneDeployment(cfg)

	expected := ControlPlaneDefaultImageRepo + ":v1.0.0"
	actual := deploy.Spec.Template.Spec.Containers[0].Image
	if actual != expected {
		t.Errorf("image = %s, want %s", actual, expected)
	}
}

func TestControlPlaneDeployment_ImageOverride(t *testing.T) {
	cfg := testControlPlaneConfig()
	cfg.ImageRepository = "nginx"
	cfg.ImageTag = "1.27-alpine"
	deploy := ControlPlaneDeployment(cfg)

	expected := "nginx:1.27-alpine"
	actual := deploy.Spec.Template.Spec.Containers[0].Image
	if actual != expected {
		t.Errorf("image = %s, want %s", actual, expected)
	}
}

func TestControlPlaneDeployment_Ports(t *testing.T) {
	deploy := ControlPlaneDeployment(testControlPlaneConfig())
	ports := deploy.Spec.Template.Spec.Containers[0].Ports
	if len(ports) != 4 {
		t.Fatalf("expected 4 ports (grpc, grpc-web, webhook, debug), got %d", len(ports))
	}
	if ports[0].ContainerPort != 8080 {
		t.Errorf("grpc port = %d, want 8080", ports[0].ContainerPort)
	}
	if ports[1].ContainerPort != 8081 {
		t.Errorf("grpc-web port = %d, want 8081", ports[1].ContainerPort)
	}
	if ports[2].Name != "webhook" || ports[2].ContainerPort != 8086 {
		t.Errorf("webhook port = %s/%d, want webhook/8086", ports[2].Name, ports[2].ContainerPort)
	}
	if ports[3].ContainerPort != 5005 {
		t.Errorf("debug port = %d, want 5005", ports[3].ContainerPort)
	}
}

func TestControlPlaneDeployment_GRPCProbes(t *testing.T) {
	deploy := ControlPlaneDeployment(testControlPlaneConfig())
	container := deploy.Spec.Template.Spec.Containers[0]

	if container.StartupProbe == nil || container.StartupProbe.GRPC == nil {
		t.Fatal("expected gRPC startup probe")
	}
	if container.ReadinessProbe == nil || container.ReadinessProbe.GRPC == nil {
		t.Fatal("expected gRPC readiness probe")
	}
	if container.LivenessProbe == nil || container.LivenessProbe.GRPC == nil {
		t.Fatal("expected gRPC liveness probe")
	}
	if container.StartupProbe.FailureThreshold != 90 {
		t.Errorf("startup failureThreshold = %d, want 90", container.StartupProbe.FailureThreshold)
	}
}

func TestControlPlaneDeployment_PostgreSQLEnvVars(t *testing.T) {
	deploy := ControlPlaneDeployment(testControlPlaneConfig())
	envMap := envVarMap(deploy.Spec.Template.Spec.Containers[0].Env)

	if _, ok := envMap["DB_HOST"]; !ok {
		t.Error("missing DB_HOST env var")
	}
	if _, ok := envMap["DB_USERNAME"]; !ok {
		t.Error("missing DB_USERNAME env var")
	}
	// The one-database contract: the operator sets exactly DB_NAME=planton, and
	// no per-domain *_DB_NAME env var exists anymore.
	if envMap["DB_NAME"] != DBBase {
		t.Errorf("DB_NAME = %q, want %q", envMap["DB_NAME"], DBBase)
	}
	for name := range envMap {
		if name != "DB_NAME" && strings.HasSuffix(name, "_DB_NAME") {
			t.Errorf("unexpected per-domain database env var %s (one-database contract)", name)
		}
	}
}

// Retired plumbing must never reappear: domain events ride the transactional
// event log inside the platform database, so no broker connection or stream
// wiring may ever be injected into the control plane. Reappearance of any of
// these is rot.
func TestControlPlaneDeployment_NoMessageBrokerEnvVars(t *testing.T) {
	deploy := ControlPlaneDeployment(testControlPlaneConfig())
	envMap := envVarMap(deploy.Spec.Template.Spec.Containers[0].Env)

	for _, name := range []string{
		"NATS_HOST",
		"NATS_PORT",
		"NATS_USERNAME",
		"NATS_PASSWORD",
		"NATS_API_RESOURCES_STREAM",
		"NATS_INFRA_PIPELINE_STATUS_STREAM",
		"NATS_PIPELINE_STATUS_STREAM",
		"NATS_STACK_JOB_STATUS_STREAM",
		"NATS_WEBHOOKS_STREAM",
	} {
		if _, present := envMap[name]; present {
			t.Errorf("%s must not be injected into the control plane", name)
		}
	}
}

// The minimal footprint still runs no policy engine: the authorization
// posture is governed by the provider mode (allow-authenticated below), and
// the FGA_* settings are present only as inert placeholders so the Spring
// context binds (mirroring the desktop contract). Sign-in itself is
// unconditional -- there is no local IDP arm to fall back to.
func TestControlPlaneDeployment_MinimalFootprintAuthorization(t *testing.T) {
	deploy := ControlPlaneDeployment(testControlPlaneConfig())
	envMap := envVarMap(deploy.Spec.Template.Spec.Containers[0].Env)

	if envMap["IDP_PROVIDER"] != IdentityProviderKeycloak {
		t.Errorf("IDP_PROVIDER = %q, want keycloak (sign-in is unconditional)", envMap["IDP_PROVIDER"])
	}
	// FGA settings are literal placeholders, not wired from an OpenFGA bootstrap.
	if envMap["FGA_STORE_ID"] != "local" {
		t.Errorf("FGA_STORE_ID = %q, want local (inert placeholder)", envMap["FGA_STORE_ID"])
	}
}

// With the authorization component enabled the FGA connection is REAL: the
// endpoint points at the deployed engine and the store/model ids come from the
// component's bootstrap ConfigMap -- never placeholders, never hand-wired.
func TestControlPlaneDeployment_OpenFGAOptIn(t *testing.T) {
	cfg := testControlPlaneConfig()
	cfg.OpenFGA = OpenFGAConnection("planton", "default")
	deploy := ControlPlaneDeployment(cfg)

	envMap := envVarMap(deploy.Spec.Template.Spec.Containers[0].Env)
	if envMap["FGA_API_ENDPOINT"] != cfg.OpenFGA.HTTPURL {
		t.Errorf("FGA_API_ENDPOINT = %q, want the deployed engine %q", envMap["FGA_API_ENDPOINT"], cfg.OpenFGA.HTTPURL)
	}
	// The store id exists only after the component bootstraps it; the
	// ConfigMap reference is what makes the pod wait for that instead of
	// booting against an id that does not exist.
	for _, envVar := range deploy.Spec.Template.Spec.Containers[0].Env {
		if envVar.Name == "FGA_STORE_ID" {
			if envVar.ValueFrom == nil || envVar.ValueFrom.ConfigMapKeyRef == nil ||
				envVar.ValueFrom.ConfigMapKeyRef.Name != cfg.OpenFGA.BootstrapConfigMapName ||
				envVar.ValueFrom.ConfigMapKeyRef.Key != "store_id" {
				t.Error("FGA_STORE_ID must come from the bootstrap ConfigMap's store_id key")
			}
		}
	}
	// The model is the control plane's, established at its own boot from the
	// rulebook of its own version; the operator hands over no model id, only
	// the instruction to manage it. The hyphen-stripped name is load-bearing
	// (relaxed binding of planton.bootstrap.authorization-model.manage).
	if _, set := envMap["FGA_MODEL_ID"]; set {
		t.Error("FGA_MODEL_ID must not be set: the control plane establishes its own model")
	}
	if envMap["PLANTON_BOOTSTRAP_AUTHORIZATIONMODEL_MANAGE"] != "true" {
		t.Errorf("PLANTON_BOOTSTRAP_AUTHORIZATIONMODEL_MANAGE = %q, want true", envMap["PLANTON_BOOTSTRAP_AUTHORIZATIONMODEL_MANAGE"])
	}
	if _, set := envMap["PLANTON_BOOTSTRAP_AUTHORIZATION_MODEL_MANAGE"]; set {
		t.Error("PLANTON_BOOTSTRAP_AUTHORIZATION_MODEL_MANAGE must not be set (does not bind to planton.bootstrap.authorization-model.manage)")
	}
}

// The control plane runs as a real OIDC relying party: issuer-backed JWT
// validation, audience checks, JIT provisioning against the bundled identity
// server -- with split-horizon fetches (IDP_INTERNAL_URL). There is no
// process-level machine identity: cross-domain calls are in-process
// service-layer calls, and background flows act as the platform system
// caller.
func TestControlPlaneDeployment_IdentityBinding(t *testing.T) {
	cfg := testControlPlaneConfig()
	cfg.Identity.Bootstrap.Admins = []string{"admin@example.com", "second@example.com"}
	deploy := ControlPlaneDeployment(cfg)
	envMap := envVarMap(deploy.Spec.Template.Spec.Containers[0].Env)

	if envMap["IDP_PROVIDER"] != IdentityProviderKeycloak {
		t.Errorf("IDP_PROVIDER = %q, want keycloak", envMap["IDP_PROVIDER"])
	}
	if envMap["IDP_URL"] != "http://planton.example.com/idp/realms/planton" {
		t.Errorf("IDP_URL = %q, want the exact advertised issuer", envMap["IDP_URL"])
	}
	// Split horizon: every issuer FETCH goes to the in-cluster address; only
	// validation uses the advertised issuer above.
	if envMap["IDP_INTERNAL_URL"] != "http://planton-identity.default.svc.cluster.local/idp/realms/planton" {
		t.Errorf("IDP_INTERNAL_URL = %q, want the in-cluster issuer address", envMap["IDP_INTERNAL_URL"])
	}
	// The audience is a fixed logical name so an auto-derived hostname never
	// churns token validation config; the realm's mapper stamps the same value.
	if envMap["IDP_API_AUDIENCE"] != IDPAPIAudience {
		t.Errorf("IDP_API_AUDIENCE = %q, want %s", envMap["IDP_API_AUDIENCE"], IDPAPIAudience)
	}
	if envMap["IDP_DOMAIN"] != "planton.example.com" {
		t.Errorf("IDP_DOMAIN = %q, want the bare hostname", envMap["IDP_DOMAIN"])
	}
	// Retired plumbing must never reappear: no machine-identity env (internal
	// calls need no process identity), no RPC_AUTHORIZATION_ENABLED (the
	// key it fed has zero readers -- enforcement is governed by the
	// authorization provider mode), and none of the pre-pluggable-IDP Auth0
	// bindings (their owning config block was removed with zero Java
	// consumers). Reappearance of any of these is rot.
	for _, name := range []string{
		"MICROSERVICE_IDENTITY_ENABLED",
		"MICROSERVICE_IDENTITY_IDP_CLIENT_ID",
		"MICROSERVICE_IDENTITY_IDP_CLIENT_SECRET",
		"RPC_AUTHORIZATION_ENABLED",
		"AUTH0_MANAGEMENT_API_URL",
		"IDP_CLIENT_ID_CONSOLE",
		"IDP_CLIENT_ID_CLI",
		"IDP_DATABASE_CONNECTION_CONSOLE_DATABASE",
		"IDP_DATABASE_CONNECTION_CONSOLE_GOOGLE",
	} {
		if _, present := envMap[name]; present {
			t.Errorf("%s must not be injected into the control plane", name)
		}
	}
	// The trusting-team authorization arm plus the first-boot seeds ride the
	// identity arm: sign-in without them is the silent-failure state this
	// wiring exists to prevent.
	if envMap["PLANTON_AUTHORIZATION_PROVIDER"] != "allow-authenticated" {
		t.Errorf("PLANTON_AUTHORIZATION_PROVIDER = %q, want allow-authenticated", envMap["PLANTON_AUTHORIZATION_PROVIDER"])
	}
	if envMap["PLANTON_BOOTSTRAP_ORGANIZATION_SLUG"] != "default" {
		t.Errorf("PLANTON_BOOTSTRAP_ORGANIZATION_SLUG = %q, want default", envMap["PLANTON_BOOTSTRAP_ORGANIZATION_SLUG"])
	}
	if envMap["PLANTON_BOOTSTRAP_ENVIRONMENT_SLUG"] != "default" {
		t.Errorf("PLANTON_BOOTSTRAP_ENVIRONMENT_SLUG = %q, want default", envMap["PLANTON_BOOTSTRAP_ENVIRONMENT_SLUG"])
	}
	if envMap["PLANTON_BOOTSTRAP_ADMINS"] != "admin@example.com,second@example.com" {
		t.Errorf("PLANTON_BOOTSTRAP_ADMINS = %q, want the comma-joined admins", envMap["PLANTON_BOOTSTRAP_ADMINS"])
	}
}

// Without declared admins the admins env must be ABSENT (not empty): the
// control plane's grant reconciler activates on the property's presence.
func TestControlPlaneDeployment_IdentityBindingWithoutAdmins(t *testing.T) {
	cfg := testControlPlaneConfig()
	deploy := ControlPlaneDeployment(cfg)
	envMap := envVarMap(deploy.Spec.Template.Spec.Containers[0].Env)

	if _, ok := envMap["PLANTON_BOOTSTRAP_ADMINS"]; ok {
		t.Error("PLANTON_BOOTSTRAP_ADMINS must be absent when no admins are declared")
	}
	if envMap["PLANTON_BOOTSTRAP_ORGANIZATION_SLUG"] != "default" {
		t.Error("org/env seeds are still set: the workspace exists even before its admin does")
	}
}

// The user-directory credential is wired unconditionally (invitations need it
// beyond first-run setup), always from its Secret.
func TestControlPlaneDeployment_UserDirectoryClient(t *testing.T) {
	deploy := ControlPlaneDeployment(testControlPlaneConfig())
	envMap := envVarMap(deploy.Spec.Template.Spec.Containers[0].Env)

	if envMap["IDP_REALM_USERS_CLIENT_ID"] != IdentityUsersClientID {
		t.Errorf("IDP_REALM_USERS_CLIENT_ID = %q, want %s",
			envMap["IDP_REALM_USERS_CLIENT_ID"], IdentityUsersClientID)
	}
	if envMap["IDP_REALM_USERS_CLIENT_SECRET"] != fromSecretRef {
		t.Error("the user-directory client secret must come from its Secret, never a literal")
	}
}

// The federation facts reach the control plane as a MOUNTED ConfigMap plus a
// static path env var -- never as env values. Env changes rewrite the
// Deployment and roll the pod (at the worst moment: right after an admin
// applies their identity manifest), while mounted ConfigMap content updates
// in place. The mount is the whole directory (a subPath mount would freeze
// updates) and read-only.
func TestControlPlaneDeployment_FederationFactsMount(t *testing.T) {
	deploy := ControlPlaneDeployment(testControlPlaneConfig())
	podSpec := deploy.Spec.Template.Spec

	if len(podSpec.Volumes) != 1 || podSpec.Volumes[0].ConfigMap == nil ||
		podSpec.Volumes[0].ConfigMap.Name != IdentityFederationFactsConfigMapName("planton") {
		t.Fatalf("expected the federation facts ConfigMap volume, got %+v", podSpec.Volumes)
	}
	if podSpec.Volumes[0].ConfigMap.Optional == nil || !*podSpec.Volumes[0].ConfigMap.Optional {
		t.Error("the facts volume must be optional (belt-and-braces; the identity component ensures it exists)")
	}

	mounts := podSpec.Containers[0].VolumeMounts
	if len(mounts) != 1 || mounts[0].MountPath != IdentityFederationFactsMountPath || !mounts[0].ReadOnly {
		t.Fatalf("expected one read-only mount at %s, got %+v", IdentityFederationFactsMountPath, mounts)
	}
	if mounts[0].SubPath != "" {
		t.Error("the facts mount must never use subPath: subPath mounts freeze kubelet's in-place updates")
	}

	envMap := envVarMap(podSpec.Containers[0].Env)
	if envMap["IDP_FEDERATION_FACTS_FILE"] != IdentityFederationFactsFilePath() {
		t.Errorf("IDP_FEDERATION_FACTS_FILE = %q, want the static mounted path %q",
			envMap["IDP_FEDERATION_FACTS_FILE"], IdentityFederationFactsFilePath())
	}
}

// First-run setup mode (no admin declared): the setup code rides its Secret
// and the property's PRESENCE opens the control plane's public setup RPCs --
// so with a declared admin both envs must be ABSENT, keeping the whole setup
// surface structurally closed rather than merely gated.
func TestControlPlaneDeployment_FirstRunSetupMode(t *testing.T) {
	cfg := testControlPlaneConfig()
	cfg.Identity.SetupCodeSecretName = "planton-identity-setup-code"
	cfg.Identity.SetupCodeHint = "kubectl -n default get secret planton-identity-setup-code ..."
	deploy := ControlPlaneDeployment(cfg)
	envMap := envVarMap(deploy.Spec.Template.Spec.Containers[0].Env)

	if envMap["PLANTON_BOOTSTRAP_SETUP_CODE"] != fromSecretRef {
		t.Error("the setup code must come from its Secret, never a literal")
	}
	if envMap["PLANTON_BOOTSTRAP_SETUP_CODE_HINT"] == "" {
		t.Error("the setup-code hint must travel with the code (the setup page prints it)")
	}

	// Declared-admin shape: the setup surface does not exist.
	deploy = ControlPlaneDeployment(testControlPlaneConfig())
	envMap = envVarMap(deploy.Spec.Template.Spec.Containers[0].Env)
	if _, ok := envMap["PLANTON_BOOTSTRAP_SETUP_CODE"]; ok {
		t.Error("PLANTON_BOOTSTRAP_SETUP_CODE must be absent when an admin is declared")
	}
	if _, ok := envMap["PLANTON_BOOTSTRAP_SETUP_CODE_HINT"]; ok {
		t.Error("PLANTON_BOOTSTRAP_SETUP_CODE_HINT must be absent when an admin is declared")
	}
}

// The minimal footprint uses the built-in Postgres graph provider (search is
// always the built-in Postgres projection -- there is no search provider to
// select) and injects no downstream dial-out endpoint (cross-domain calls are
// in-process).
func TestControlPlaneDeployment_MinimalFootprintContract(t *testing.T) {
	deploy := ControlPlaneDeployment(testControlPlaneConfig())
	envMap := envVarMap(deploy.Spec.Template.Spec.Containers[0].Env)

	if _, present := envMap["PLANTON_SEARCH_PROVIDER"]; present {
		t.Error("PLANTON_SEARCH_PROVIDER must not be injected; search has exactly one engine and no provider property")
	}
	if envMap["PLANTON_ESTATE_PROVIDER"] != "postgres" {
		t.Errorf("PLANTON_ESTATE_PROVIDER = %q, want postgres", envMap["PLANTON_ESTATE_PROVIDER"])
	}
	// Cross-domain calls are in-process service-layer calls; the monolith has
	// no dial-out endpoint, so the operator must not inject one.
	if _, present := envMap["PLANTON_APIS_GRPC_ENDPOINT"]; present {
		t.Errorf("PLANTON_APIS_GRPC_ENDPOINT must not be injected; the monolith has no downstream dial-out")
	}
	// The gRPC-Web opt-in: presence of this env is what makes the monolith
	// serve browsers; the operator always sets it for the console's sake.
	if envMap["GRPC_WEB_PORT"] != "8081" {
		t.Errorf("GRPC_WEB_PORT = %q, want 8081", envMap["GRPC_WEB_PORT"])
	}
	// The one database the app self-provisions at boot.
	if envMap["DB_NAME"] != "planton" {
		t.Errorf("DB_NAME = %q, want planton", envMap["DB_NAME"])
	}
}

func TestControlPlaneDeployment_ExternalConfig(t *testing.T) {
	cfg := testControlPlaneConfig()
	cfg.ExternalConfigSecretName = "my-external-config"
	deploy := ControlPlaneDeployment(cfg)

	envFrom := deploy.Spec.Template.Spec.Containers[0].EnvFrom
	if len(envFrom) != 1 {
		t.Fatalf("expected 1 envFrom source, got %d", len(envFrom))
	}
	if envFrom[0].SecretRef == nil || envFrom[0].SecretRef.Name != "my-external-config" {
		t.Error("expected external config secretRef")
	}
}

func TestControlPlaneDeployment_NoExternalConfig(t *testing.T) {
	deploy := ControlPlaneDeployment(testControlPlaneConfig())
	envFrom := deploy.Spec.Template.Spec.Containers[0].EnvFrom
	if len(envFrom) != 0 {
		t.Errorf("expected 0 envFrom sources, got %d", len(envFrom))
	}
}

func TestControlPlaneDeployment_OptionalNeo4j(t *testing.T) {
	cfg := testControlPlaneConfig()
	neo4j := Neo4jConnection("planton", "default")
	cfg.Neo4j = &neo4j
	deploy := ControlPlaneDeployment(cfg)
	envMap := envVarMap(deploy.Spec.Template.Spec.Containers[0].Env)

	if _, ok := envMap["NEO4J_URL"]; !ok {
		t.Error("missing NEO4J_URL when Neo4j is enabled")
	}
}

func TestControlPlaneDeployment_NoNeo4jWhenNil(t *testing.T) {
	deploy := ControlPlaneDeployment(testControlPlaneConfig())
	envMap := envVarMap(deploy.Spec.Template.Spec.Containers[0].Env)

	if _, ok := envMap["NEO4J_URL"]; ok {
		t.Error("NEO4J_URL should not be set when Neo4j is nil")
	}
}

func TestControlPlaneService(t *testing.T) {
	svc := ControlPlaneService("planton", "default", nil)
	if svc.Name != "planton-control-plane" {
		t.Errorf("name = %s, want planton-control-plane", svc.Name)
	}
	if svc.Spec.Type != "ClusterIP" {
		t.Errorf("type = %s, want ClusterIP", svc.Spec.Type)
	}
	if len(svc.Spec.Ports) != 3 {
		t.Fatalf("expected 3 ports (grpc, grpc-web, webhook), got %d", len(svc.Spec.Ports))
	}
	if svc.Spec.Ports[0].Port != 80 {
		t.Errorf("grpc service port = %d, want 80", svc.Spec.Ports[0].Port)
	}
	if svc.Spec.Ports[1].Name != "grpc-web" || svc.Spec.Ports[1].Port != 8081 {
		t.Errorf("grpc-web service port = %s/%d, want grpc-web/8081",
			svc.Spec.Ports[1].Name, svc.Spec.Ports[1].Port)
	}
	if svc.Spec.Ports[1].AppProtocol == nil || *svc.Spec.Ports[1].AppProtocol != "http" {
		t.Error("grpc-web service port should declare appProtocol http for ingress routing")
	}
}

func TestControlPlaneDeploymentName(t *testing.T) {
	if got := ControlPlaneDeploymentName("planton"); got != "planton-control-plane" {
		t.Errorf("got %s, want planton-control-plane", got)
	}
}

// The runner binding activates the boot seeds (slug presence is the Java
// side's activation gate), enables badge verification for exactly this
// install's namespace, and advertises the runner-connectivity capability.
// NO credential env exists: the runner's proof is its projected badge.
func TestControlPlaneDeployment_RunnerBinding(t *testing.T) {
	cfg := testControlPlaneConfig()
	cfg.Runner = &RunnerBinding{
		CloudOpsSecretName: "planton-runner-cloudops",
		Provisioner:        "tofu",
		DirectDialHost:     "planton-runner.planton.svc.cluster.local",
	}
	deploy := ControlPlaneDeployment(cfg)
	envMap := envVarMap(deploy.Spec.Template.Spec.Containers[0].Env)

	if envMap["PLANTON_BOOTSTRAP_RUNNER_SLUG"] != RunnerSlug(cfg.CRName) {
		t.Errorf("PLANTON_BOOTSTRAP_RUNNER_SLUG = %q, want %s (the ServiceAccount name IS the slug)",
			envMap["PLANTON_BOOTSTRAP_RUNNER_SLUG"], RunnerSlug(cfg.CRName))
	}
	if envMap["PLANTON_BOOTSTRAP_RUNNER_NAMESPACE"] != cfg.Namespace {
		t.Errorf("PLANTON_BOOTSTRAP_RUNNER_NAMESPACE = %q, want the CR namespace (the declared "+
			"workload identity's namespace half)", envMap["PLANTON_BOOTSTRAP_RUNNER_NAMESPACE"])
	}
	if envMap["PLANTON_BOOTSTRAP_RUNNER_PROVISIONER"] != "tofu" {
		t.Errorf("PLANTON_BOOTSTRAP_RUNNER_PROVISIONER = %q, want tofu", envMap["PLANTON_BOOTSTRAP_RUNNER_PROVISIONER"])
	}
	// The credential seed is GONE: no API key exists on this install in any
	// form -- the badge is the identity, per call, cluster-verified.
	if _, ok := envMap["PLANTON_BOOTSTRAP_RUNNER_API_KEY"]; ok {
		t.Error("PLANTON_BOOTSTRAP_RUNNER_API_KEY must not exist: the runner's proof is its badge, never a key")
	}
	// Badge verification: enabled, audience-pinned, trusting exactly the CR
	// namespace. The boot probe kills the pod if the TokenReview grant is
	// missing, so these three only ever ship together with the ClusterRole.
	if envMap["KUBERNETES_WORKLOAD_AUTH_ENABLED"] != "true" {
		t.Errorf("KUBERNETES_WORKLOAD_AUTH_ENABLED = %q, want true with a runner binding",
			envMap["KUBERNETES_WORKLOAD_AUTH_ENABLED"])
	}
	if envMap["KUBERNETES_WORKLOAD_AUTH_AUDIENCE"] != RunnerBadgeAudience {
		t.Errorf("KUBERNETES_WORKLOAD_AUTH_AUDIENCE = %q, want %s",
			envMap["KUBERNETES_WORKLOAD_AUTH_AUDIENCE"], RunnerBadgeAudience)
	}
	if envMap["KUBERNETES_WORKLOAD_AUTH_TRUSTED_NAMESPACES"] != cfg.Namespace {
		t.Errorf("KUBERNETES_WORKLOAD_AUTH_TRUSTED_NAMESPACES = %q, want exactly the CR namespace",
			envMap["KUBERNETES_WORKLOAD_AUTH_TRUSTED_NAMESPACES"])
	}
	// Runner-connectivity advertisement: minted identity documents and the
	// materializer's capability gate both read these.
	if envMap["CONNECT_RUNNER_TEMPORAL_ENDPOINT"] != cfg.Temporal.FrontendEndpoint {
		t.Errorf("CONNECT_RUNNER_TEMPORAL_ENDPOINT = %q, want the in-cluster frontend",
			envMap["CONNECT_RUNNER_TEMPORAL_ENDPOINT"])
	}
	if envMap["CONNECT_RUNNER_TEMPORAL_NAMESPACE"] != "platform.pipelines" {
		t.Errorf("CONNECT_RUNNER_TEMPORAL_NAMESPACE = %q, want platform.pipelines",
			envMap["CONNECT_RUNNER_TEMPORAL_NAMESPACE"])
	}
	// Single-runner direct dial: CloudOps reaches the runner at its Service
	// with the shared bearer -- sourced from the SAME Secret key the runner
	// consumes, so the two sides cannot disagree.
	if envMap["RUNNER_DIRECT_ENABLED"] != "true" {
		t.Errorf("RUNNER_DIRECT_ENABLED = %q, want true with a runner binding", envMap["RUNNER_DIRECT_ENABLED"])
	}
	if envMap["RUNNER_DIRECT_HOST"] != "planton-runner.planton.svc.cluster.local" {
		t.Errorf("RUNNER_DIRECT_HOST = %q, want the runner Service FQDN", envMap["RUNNER_DIRECT_HOST"])
	}
	if envMap["RUNNER_DIRECT_AUTH_TOKEN"] != fromSecretRef {
		t.Error("the direct-dial token must come from its Secret, never a literal in the pod spec")
	}
	assertSecretEnv(t, deploy.Spec.Template.Spec.Containers[0].Env, "RUNNER_DIRECT_AUTH_TOKEN",
		"planton-runner-cloudops", RunnerCloudOpsSecretKeyToken)
	if envMap["TUNNEL_ENABLED"] != "false" {
		t.Error("the tunnel stays off: it exists to cross networks, and this runner shares the control plane's")
	}
	// The tunnel-less posture: no tunnel endpoint declared, no CA material
	// anywhere -- identity documents mint without tunnel material, and no
	// placeholder bytes exist to pass a validation this posture retired.
	for _, absent := range []string{"CONNECT_RUNNER_TUNNEL_ENDPOINT",
		"CONNECT_RUNNER_CA_CERT_BASE64", "CONNECT_RUNNER_CA_KEY_BASE64",
		"CONNECT_RUNNER_CERTIFICATE_VALIDITY_DAYS"} {
		if _, ok := envMap[absent]; ok {
			t.Errorf("%s must be absent: this install operates no runner tunnel and ships no CA", absent)
		}
	}
	// The document endpoint is the in-cluster Service -- the same
	// reachability horizon as the advertised Temporal endpoint.
	wantEndpoint := ControlPlaneServiceFQDN(cfg.CRName, cfg.Namespace) + ":80"
	if envMap["CONNECT_RUNNER_PLANTON_API_ENDPOINT"] != wantEndpoint {
		t.Errorf("CONNECT_RUNNER_PLANTON_API_ENDPOINT = %q, want %s",
			envMap["CONNECT_RUNNER_PLANTON_API_ENDPOINT"], wantEndpoint)
	}
}

// The badge-verification grant: TokenReview + SelfSubjectAccessReview create
// (the arm's boot probe needs the latter), bound to the control plane's
// DEDICATED ServiceAccount -- never a namespace default, so no co-located
// workload inherits verification power.
func TestControlPlaneTokenReviewerGrant(t *testing.T) {
	cfg := testControlPlaneConfig()

	// Cluster-scoped, so the name carries the NAMESPACE: platforms are
	// namespaced, and two same-named platforms sharing this object would
	// force-apply each other's binding subject away -- the losing control
	// plane's boot probe would crash-loop it.
	role := ControlPlaneTokenReviewerClusterRole(cfg)
	if role.Name != "default-planton-control-plane-token-reviewer" {
		t.Errorf("name = %s, want default-planton-control-plane-token-reviewer", role.Name)
	}
	if len(role.Rules) != 2 {
		t.Fatalf("rules = %+v, want exactly tokenreviews + selfsubjectaccessreviews create", role.Rules)
	}
	if role.Rules[0].APIGroups[0] != "authentication.k8s.io" ||
		role.Rules[0].Resources[0] != "tokenreviews" ||
		len(role.Rules[0].Verbs) != 1 || role.Rules[0].Verbs[0] != "create" {
		t.Errorf("rules[0] = %+v, want authentication.k8s.io/tokenreviews create only", role.Rules[0])
	}
	if role.Rules[1].APIGroups[0] != "authorization.k8s.io" ||
		role.Rules[1].Resources[0] != "selfsubjectaccessreviews" ||
		len(role.Rules[1].Verbs) != 1 || role.Rules[1].Verbs[0] != "create" {
		t.Errorf("rules[1] = %+v, want authorization.k8s.io/selfsubjectaccessreviews create only", role.Rules[1])
	}

	binding := ControlPlaneTokenReviewerClusterRoleBinding(cfg)
	if binding.RoleRef.Kind != "ClusterRole" || binding.RoleRef.Name != role.Name {
		t.Errorf("roleRef = %+v, want the token-reviewer ClusterRole", binding.RoleRef)
	}
	if len(binding.Subjects) != 1 || binding.Subjects[0].Kind != "ServiceAccount" ||
		binding.Subjects[0].Name != ControlPlaneServiceAccountName(cfg.CRName) ||
		binding.Subjects[0].Namespace != cfg.Namespace {
		t.Errorf("subjects = %+v, want exactly the control plane's dedicated ServiceAccount", binding.Subjects)
	}
}

// Without a runner binding the seed envs must be ABSENT (not empty): the Java
// seeders activate on the slug property's presence.
func TestControlPlaneDeployment_NoRunnerBinding(t *testing.T) {
	deploy := ControlPlaneDeployment(testControlPlaneConfig())
	envMap := envVarMap(deploy.Spec.Template.Spec.Containers[0].Env)

	for _, name := range []string{
		"PLANTON_BOOTSTRAP_RUNNER_SLUG",
		"PLANTON_BOOTSTRAP_RUNNER_NAMESPACE",
		"PLANTON_BOOTSTRAP_RUNNER_PROVISIONER",
		"KUBERNETES_WORKLOAD_AUTH_ENABLED",
		"KUBERNETES_WORKLOAD_AUTH_AUDIENCE",
		"KUBERNETES_WORKLOAD_AUTH_TRUSTED_NAMESPACES",
		"CONNECT_RUNNER_TEMPORAL_ENDPOINT",
		"CONNECT_RUNNER_TEMPORAL_NAMESPACE",
		"RUNNER_DIRECT_HOST",
		"RUNNER_DIRECT_AUTH_TOKEN",
	} {
		if _, ok := envMap[name]; ok {
			t.Errorf("%s must be absent when the runner is disabled", name)
		}
	}
	// The direct dial is declared off, not merely left to defaults: the env
	// contract mirrors plantond's local.env, where the switch is explicit.
	if envMap["RUNNER_DIRECT_ENABLED"] != "false" {
		t.Errorf("RUNNER_DIRECT_ENABLED = %q, want the explicit false without a runner", envMap["RUNNER_DIRECT_ENABLED"])
	}
}

// The build-routing boot seed rides the runner binding: with builds enabled
// (the shipped default) the control plane seeds this cluster's build-cluster
// connection and the platform default pointing at it. RUNNER presence is the
// Java seeders' activation gate; the env names are the relaxed-binding forms
// with hyphens STRIPPED (an underscored TEKTON_CONNECTION would satisfy the
// gate but bind nothing).
func TestControlPlaneDeployment_BuildRoutingSeed(t *testing.T) {
	cfg := testControlPlaneConfig()
	cfg.Runner = &RunnerBinding{
		CloudOpsSecretName: "planton-runner-cloudops",
		Provisioner:        "tofu",
		DirectDialHost:     "planton-runner.planton.svc.cluster.local",
		BuildEnabled:       true,
	}
	deploy := ControlPlaneDeployment(cfg)
	envMap := envVarMap(deploy.Spec.Template.Spec.Containers[0].Env)

	if envMap["PLANTON_BOOTSTRAP_TEKTONCONNECTION_RUNNER"] != RunnerSlug(cfg.CRName) {
		t.Errorf("PLANTON_BOOTSTRAP_TEKTONCONNECTION_RUNNER = %q, want %s",
			envMap["PLANTON_BOOTSTRAP_TEKTONCONNECTION_RUNNER"], RunnerSlug(cfg.CRName))
	}
	if envMap["PLANTON_BOOTSTRAP_TEKTONCONNECTION_ORG"] != "default" {
		t.Errorf("PLANTON_BOOTSTRAP_TEKTONCONNECTION_ORG = %q, want the bootstrap org",
			envMap["PLANTON_BOOTSTRAP_TEKTONCONNECTION_ORG"])
	}
	// The namespace variable stays unset by design: empty means "the
	// runner's own placement", which keeps the seeded connection inside the
	// log streamer's watch by construction.
	if _, ok := envMap["PLANTON_BOOTSTRAP_TEKTONCONNECTION_NAMESPACE"]; ok {
		t.Error("PLANTON_BOOTSTRAP_TEKTONCONNECTION_NAMESPACE must not be set -- empty selects the runner's placement")
	}
}

// Builds off (or no runner) means NO build-routing seed variables, not empty
// ones: presence is the activation gate.
func TestControlPlaneDeployment_BuildRoutingSeedAbsentWhenBuildsOff(t *testing.T) {
	withRunnerBuildsOff := testControlPlaneConfig()
	withRunnerBuildsOff.Runner = &RunnerBinding{
		CloudOpsSecretName: "planton-runner-cloudops",
		Provisioner:        "tofu",
		DirectDialHost:     "planton-runner.planton.svc.cluster.local",
		BuildEnabled:       false,
	}
	noRunner := testControlPlaneConfig()

	for name, cfg := range map[string]ControlPlaneConfig{
		"runner with builds off": withRunnerBuildsOff,
		"no runner":              noRunner,
	} {
		envMap := envVarMap(ControlPlaneDeployment(cfg).Spec.Template.Spec.Containers[0].Env)
		for _, envName := range []string{
			"PLANTON_BOOTSTRAP_TEKTONCONNECTION_RUNNER",
			"PLANTON_BOOTSTRAP_TEKTONCONNECTION_ORG",
			"PLANTON_BOOTSTRAP_TEKTONCONNECTION_NAMESPACE",
		} {
			if _, ok := envMap[envName]; ok {
				t.Errorf("%s: %s must be absent", name, envName)
			}
		}
	}
}

// Neither pipeline family receives catalog coordinates: service builds
// compile at dispatch from release-pinned content, and the infra family's
// git-repository lane is deliberately inert (unset catalog, creation-time
// refusal). The build knobs are the workspace sizes and the task queues --
// and the retired coordinates must never reappear, or a deployment would
// silently re-arm cluster-side git resolution.
func TestControlPlaneDeployment_TektonBuildEnv(t *testing.T) {
	deploy := ControlPlaneDeployment(testControlPlaneConfig())
	envMap := envVarMap(deploy.Spec.Template.Spec.Containers[0].Env)

	want := map[string]string{
		"TEKTON_INFRA_PIPELINE_DISK_SIZE":                   "1Gi",
		"TEKTON_SERVICE_PIPELINE_DISK_SIZE":                 "5Gi",
		"TEMPORAL_TASK_QUEUE_SERVICE_PIPELINE_BUILD_STAGE":  "service-pipeline-build-stage",
		"TEMPORAL_TASK_QUEUE_SERVICE_PIPELINE_DEPLOY_STAGE": "service-pipeline-deploy-stage",
		"TEMPORAL_TASK_QUEUE_SERVICE_CLEANUP":               "service-cleanup",
	}
	for name, value := range want {
		if envMap[name] != value {
			t.Errorf("%s = %q, want %q", name, envMap[name], value)
		}
	}
	for _, retired := range []string{
		"TEKTON_PIPELINE_GIT_REPO_URL",
		"TEKTON_PIPELINE_GIT_REVISION",
		"TEKTON_PIPELINE_FILE_PATH_IN_REPO_KUSTOMIZE",
	} {
		if _, present := envMap[retired]; present {
			t.Errorf("%s must not be set: no pipeline definition is resolved from git", retired)
		}
	}
}

// The dispatcher's queue and the worker's queue are one derivation: renaming
// the bootstrap org moves both or neither.
func TestControlPlaneDeployment_RunnerTaskQueueFollowsOrg(t *testing.T) {
	cfg := testControlPlaneConfig()
	cfg.Identity.Bootstrap.OrgSlug = "acme"
	deploy := ControlPlaneDeployment(cfg)
	envMap := envVarMap(deploy.Spec.Template.Spec.Containers[0].Env)

	want := "iac-operation.org.acme.runner." + RunnerSlug(cfg.CRName)
	if envMap["TEMPORAL_PLATFORM_RUNNER_TASK_QUEUE_DEFAULT"] != want {
		t.Errorf("TEMPORAL_PLATFORM_RUNNER_TASK_QUEUE_DEFAULT = %q, want %q",
			envMap["TEMPORAL_PLATFORM_RUNNER_TASK_QUEUE_DEFAULT"], want)
	}
	if envMap["TEMPORAL_PLATFORM_RUNNER_TASK_QUEUE_AWS"] != want {
		t.Errorf("TEMPORAL_PLATFORM_RUNNER_TASK_QUEUE_AWS = %q, want %q",
			envMap["TEMPORAL_PLATFORM_RUNNER_TASK_QUEUE_AWS"], want)
	}
}

// PLANTON_VERSION resolves IaC module artifacts from the public CDN and is
// pinned independently of the platform image version -- a platform tag with
// no published artifacts would make every deploy 404 at download. The CR's
// spec.controlPlane.iacModulesVersion is the per-install override; empty
// means the compiled pin.
func TestControlPlaneDeployment_ModuleArtifactsVersionPinned(t *testing.T) {
	cfg := testControlPlaneConfig()
	cfg.Version = "v99.0.0"
	deploy := ControlPlaneDeployment(cfg)
	envMap := envVarMap(deploy.Spec.Template.Spec.Containers[0].Env)

	if envMap["PLANTON_VERSION"] != controlPlaneModuleArtifactsVersion {
		t.Errorf("PLANTON_VERSION = %q, want the pinned %s (not the platform version)",
			envMap["PLANTON_VERSION"], controlPlaneModuleArtifactsVersion)
	}

	cfg.IacModulesVersion = "v0.6.1"
	overridden := envVarMap(ControlPlaneDeployment(cfg).Spec.Template.Spec.Containers[0].Env)
	if overridden["PLANTON_VERSION"] != "v0.6.1" {
		t.Errorf("PLANTON_VERSION = %q, want the CR override v0.6.1 to beat the pin",
			overridden["PLANTON_VERSION"])
	}
	// The chart bundle's location is the control plane's to derive from its own
	// catalog pin; the operator only switches the seed on. The hyphen-stripped
	// name is load-bearing: the underscored variant does not bind.
	if envMap["PLANTON_BOOTSTRAP_INFRACHARTS_ENABLED"] != "true" {
		t.Errorf("PLANTON_BOOTSTRAP_INFRACHARTS_ENABLED = %q, want true", envMap["PLANTON_BOOTSTRAP_INFRACHARTS_ENABLED"])
	}
	for _, stale := range []string{"PLANTON_BOOTSTRAP_INFRACHARTS_SOURCEURL", "PLANTON_BOOTSTRAP_INFRA_CHARTS_ENABLED"} {
		if _, ok := envMap[stale]; ok {
			t.Errorf("%s must not be set", stale)
		}
	}
}

// The platform vault is present or absent -- never a placeholder. Without the
// vault component the pod must carry the explicit opt-out and NO vault address
// at all: the dead localhost:8200 pair (boots fine, fails confusingly at first
// use, log-screams from the signing-key bootstrap) must never come back.
func TestControlPlaneDeployment_NoVaultMeansExplicitOptOut(t *testing.T) {
	deploy := ControlPlaneDeployment(testControlPlaneConfig())
	envMap := envVarMap(deploy.Spec.Template.Spec.Containers[0].Env)

	if envMap["PLANTON_VAULT_ENABLED"] != "false" {
		t.Errorf("PLANTON_VAULT_ENABLED = %q, want the explicit false opt-out", envMap["PLANTON_VAULT_ENABLED"])
	}
	for _, name := range []string{"VAULT_ADDR", "VAULT_TOKEN"} {
		if _, ok := envMap[name]; ok {
			t.Errorf("%s must be absent when the vault component is disabled (placeholders are dead)", name)
		}
	}
}

// With the vault component enabled, the control plane gets the real OpenBAO
// Service address and the root token by Secret reference (this vault is
// single-tenant and exists solely for this control plane), and no opt-out.
func TestControlPlaneDeployment_VaultBinding(t *testing.T) {
	cfg := testControlPlaneConfig()
	conn := OpenBAOConnection("planton", "default")
	cfg.Vault = &VaultBinding{
		APIAddr:        conn.APIAddr,
		InitSecretName: conn.InitSecretName,
		RootTokenKey:   conn.RootTokenKey,
	}
	deploy := ControlPlaneDeployment(cfg)
	envMap := envVarMap(deploy.Spec.Template.Spec.Containers[0].Env)

	if envMap["VAULT_ADDR"] != conn.APIAddr {
		t.Errorf("VAULT_ADDR = %q, want the deployed OpenBAO address %q", envMap["VAULT_ADDR"], conn.APIAddr)
	}
	if envMap["VAULT_TOKEN"] != fromSecretRef {
		t.Error("VAULT_TOKEN must come from the init Secret, never a literal")
	}
	if _, ok := envMap["PLANTON_VAULT_ENABLED"]; ok {
		t.Error("PLANTON_VAULT_ENABLED must be absent when the vault is wired (enabled is the Java default)")
	}
}

// The secret-backend seed envs activate the Java-side boot seed; addressing
// only, never credentials -- absent entirely when nothing is seeded.
func TestControlPlaneDeployment_SecretBackendBinding(t *testing.T) {
	cfg := testControlPlaneConfig()
	cfg.SecretBackend = &SecretBackendBinding{
		Type:      "aws-secrets-manager",
		AwsRegion: "ap-south-1",
	}
	deploy := ControlPlaneDeployment(cfg)
	envMap := envVarMap(deploy.Spec.Template.Spec.Containers[0].Env)

	if envMap["PLANTON_BOOTSTRAP_SECRETBACKEND_TYPE"] != "aws-secrets-manager" {
		t.Errorf("PLANTON_BOOTSTRAP_SECRETBACKEND_TYPE = %q, want aws-secrets-manager",
			envMap["PLANTON_BOOTSTRAP_SECRETBACKEND_TYPE"])
	}
	if envMap["PLANTON_BOOTSTRAP_SECRETBACKEND_AWSSECRETSMANAGER_REGION"] != "ap-south-1" {
		t.Error("aws region must ride the seed env")
	}
	if envMap["PLANTON_BOOTSTRAP_SECRETBACKEND_AWSSECRETSMANAGER_REGION"] == "" {
		t.Error("kms key arn must ride the seed env")
	}
}

func TestControlPlaneDeployment_NoSecretBackendBinding(t *testing.T) {
	deploy := ControlPlaneDeployment(testControlPlaneConfig())
	envMap := envVarMap(deploy.Spec.Template.Spec.Containers[0].Env)

	if _, ok := envMap["PLANTON_BOOTSTRAP_SECRETBACKEND_TYPE"]; ok {
		t.Error("secret-backend seed envs must be absent when nothing is seeded (presence is the activation gate)")
	}
}

// An inline license key rides PLANTON_LICENSING_KEY as a literal -- the
// CANONICAL relaxed-binding form of planton.licensing.key.
func TestControlPlaneDeployment_LicenseInlineKey(t *testing.T) {
	cfg := testControlPlaneConfig()
	cfg.License = &LicenseBinding{Key: "plk1.1.claims.signature"}
	deploy := ControlPlaneDeployment(cfg)
	envMap := envVarMap(deploy.Spec.Template.Spec.Containers[0].Env)

	if envMap["PLANTON_LICENSING_KEY"] != "plk1.1.claims.signature" {
		t.Errorf("PLANTON_LICENSING_KEY = %q, want the inline key verbatim", envMap["PLANTON_LICENSING_KEY"])
	}
	// The near-namesake binds a DIFFERENT property (planton.license.key) and
	// leaves the deployment silently unlicensed -- caught live by the kind
	// license drill. Guard against its return explicitly.
	if _, ok := envMap["PLANTON_LICENSE_KEY"]; ok {
		t.Error("PLANTON_LICENSE_KEY must not be set (does not bind to planton.licensing.key)")
	}
}

// A Secret-backed license references the entry, never inlines its value --
// and a rotated Secret re-delivers on the next pod start.
func TestControlPlaneDeployment_LicenseSecretRef(t *testing.T) {
	cfg := testControlPlaneConfig()
	cfg.License = &LicenseBinding{SecretName: "acme-license", SecretKey: "license-key"}
	deploy := ControlPlaneDeployment(cfg)

	assertSecretEnv(t, deploy.Spec.Template.Spec.Containers[0].Env,
		"PLANTON_LICENSING_KEY", "acme-license", "license-key")
}

// Community means NO license env at all -- absence, never an empty literal
// (the same present-or-absent honesty as the vault binding).
func TestControlPlaneDeployment_NoLicenseBinding(t *testing.T) {
	deploy := ControlPlaneDeployment(testControlPlaneConfig())
	envMap := envVarMap(deploy.Spec.Template.Spec.Containers[0].Env)

	if _, ok := envMap["PLANTON_LICENSING_KEY"]; ok {
		t.Error("PLANTON_LICENSING_KEY must be absent on a Community install")
	}
}

// The dedicated ServiceAccount is the workload-identity seam for the
// platform's OWN cloud calls (ambient secret backends + KMS KEKs). It always
// exists -- annotation-free by default -- so granting an identity later is a
// pure spec edit, and the pod always runs as it.
func TestControlPlaneServiceAccount(t *testing.T) {
	cfg := testControlPlaneConfig()
	cfg.ServiceAccountAnnotations = map[string]string{
		"eks.amazonaws.com/role-arn": "arn:aws:iam::111122223333:role/planton-control-plane",
	}

	sa := ControlPlaneServiceAccount(cfg)
	if sa.Name != "planton-control-plane" {
		t.Errorf("ServiceAccount name = %q, want planton-control-plane", sa.Name)
	}
	if sa.Annotations["eks.amazonaws.com/role-arn"] == "" {
		t.Error("declared annotations must land on the ServiceAccount")
	}

	deploy := ControlPlaneDeployment(cfg)
	if got := deploy.Spec.Template.Spec.ServiceAccountName; got != sa.Name {
		t.Errorf("pod ServiceAccountName = %q, want %q", got, sa.Name)
	}
}

// fromSecretRef is envVarMap's marker for env vars sourced from a Secret
// reference rather than a literal value.
const fromSecretRef = "(from-ref)"

func envVarMap(envs []corev1.EnvVar) map[string]string {
	m := make(map[string]string, len(envs))
	for _, e := range envs {
		if e.Value != "" {
			m[e.Name] = e.Value
		} else {
			m[e.Name] = fromSecretRef
		}
	}
	return m
}

// assertSecretEnv asserts that the named env var is sourced from exactly the
// given Secret name + data key -- the shape that keeps credential plaintext
// out of pod specs.
func assertSecretEnv(t *testing.T, envs []corev1.EnvVar, envName, secretName, key string) {
	t.Helper()
	for _, e := range envs {
		if e.Name != envName {
			continue
		}
		if e.ValueFrom == nil || e.ValueFrom.SecretKeyRef == nil ||
			e.ValueFrom.SecretKeyRef.Name != secretName ||
			e.ValueFrom.SecretKeyRef.Key != key {
			t.Errorf("%s must reference Secret %s key %s, got %+v", envName, secretName, key, e)
		}
		return
	}
	t.Errorf("env %s not found", envName)
}

func publicDoorWebIdentity() *WebIdentityBinding {
	return &WebIdentityBinding{IssuerURL: "https://planton.example.com", Offered: true}
}

func publicDoorGithubWebhooks() *GithubWebhooksBinding {
	return &GithubWebhooksBinding{Reachable: true, ReceiverURL: "https://planton.example.com/webhooks/github"}
}

// The connection-method posture the control plane boots with, pinned per
// front-door arm exactly as the control plane's methods-by-deployment fixture
// declares it (its self-hosted public-door and private-door sections): a value
// changed here without that fixture, or vice versa, is an install that
// advertises one thing and enforces another. Three arms: a public https door,
// a door declared private, the port-forward door.
func TestControlPlaneDeployment_PostureFollowsTheFrontDoor(t *testing.T) {
	const privateReason = "Keyless connections need cloud providers to fetch this install's signing keys from its " +
		"front door over the public internet, and the front door is declared private. Use the runner or access-key method instead."

	arms := []struct {
		name   string
		webID  *WebIdentityBinding
		hooks  *GithubWebhooksBinding
		want   map[string]string
		absent []string
	}{
		{
			name:  "public https door",
			webID: publicDoorWebIdentity(),
			hooks: publicDoorGithubWebhooks(),
			want: map[string]string{
				"OIDC_ISSUER_URL": "https://planton.example.com",
				"PLANTON_CONNECT_METHODAVAILABILITY_OIDC_AVAILABILITY": "available",
				"GITHUB_WEBHOOKS_REACHABLE":                            "true",
				"GITHUB_WEBHOOKS_RECEIVER_URL":                         "https://planton.example.com/webhooks/github",
			},
			absent: []string{"PLANTON_CONNECT_METHODAVAILABILITY_OIDC_REASON"},
		},
		{
			name:  "door declared private",
			webID: &WebIdentityBinding{IssuerURL: "https://planton.example.com", Offered: false, ClosedReason: privateReason},
			hooks: &GithubWebhooksBinding{Reachable: false, ReceiverURL: "https://planton.example.com/webhooks/github"},
			want: map[string]string{
				"OIDC_ISSUER_URL": "https://planton.example.com",
				"PLANTON_CONNECT_METHODAVAILABILITY_OIDC_AVAILABILITY": "unavailable",
				"PLANTON_CONNECT_METHODAVAILABILITY_OIDC_REASON":       privateReason,
				"GITHUB_WEBHOOKS_REACHABLE":                            "false",
				"GITHUB_WEBHOOKS_RECEIVER_URL":                         "https://planton.example.com/webhooks/github",
			},
		},
		{
			name:  "port-forward door",
			webID: &WebIdentityBinding{IssuerURL: "http://localhost:8080", Offered: false, ClosedReason: "port-forward sentence"},
			hooks: &GithubWebhooksBinding{Reachable: false, ReceiverURL: "http://localhost:8080/webhooks/github"},
			want: map[string]string{
				"OIDC_ISSUER_URL": "http://localhost:8080",
				"PLANTON_CONNECT_METHODAVAILABILITY_OIDC_AVAILABILITY": "unavailable",
				"PLANTON_CONNECT_METHODAVAILABILITY_OIDC_REASON":       "port-forward sentence",
				"GITHUB_WEBHOOKS_REACHABLE":                            "false",
				"GITHUB_WEBHOOKS_RECEIVER_URL":                         "http://localhost:8080/webhooks/github",
			},
		},
	}
	// Every arm declares the app-less doors closed the same way.
	everyArm := map[string]string{
		"PLANTON_CONNECT_METHODAVAILABILITY_PLATFORMAPP_AVAILABILITY": "unavailable",
		"PLANTON_CONNECT_METHODAVAILABILITY_PLATFORMAPP_REASON":       PlatformAppUnavailableReason,
		"GCP_OAUTH_ENABLED":          "false",
		"AZURE_OAUTH_ENABLED":        "false",
		"AWS_CLOUDFORMATION_ENABLED": "false",
		"WEBHOOK_PORT":               "8086",
	}
	for _, arm := range arms {
		t.Run(arm.name, func(t *testing.T) {
			cfg := testControlPlaneConfig()
			cfg.WebIdentity = arm.webID
			cfg.GithubWebhooks = arm.hooks
			envMap := envVarMap(ControlPlaneDeployment(cfg).Spec.Template.Spec.Containers[0].Env)
			for k, want := range arm.want {
				if got := envMap[k]; got != want {
					t.Errorf("%s = %q, want %q", k, got, want)
				}
			}
			for k, want := range everyArm {
				if got := envMap[k]; got != want {
					t.Errorf("%s = %q, want %q on every arm", k, got, want)
				}
			}
			for _, k := range arm.absent {
				if got, ok := envMap[k]; ok {
					t.Errorf("%s must be absent on this arm (a blank reason would be a lie about an offered mode); got %q", k, got)
				}
			}
			if v, ok := envMap["GITHUB_WEBHOOKS_RECEIVER_URL"]; ok && v == "http://localhost" {
				t.Error("the receiver URL is the door plus the webhook namespace, never the placeholder")
			}
		})
	}
}

// The webhook servlet is exposed on the container and the Service under one
// name, plain HTTP, so the front doors can route the issuer's discovery paths
// and the webhook namespace to it by name or by number.
func TestControlPlane_ExposesTheWebhookPort(t *testing.T) {
	deploy := ControlPlaneDeployment(testControlPlaneConfig())
	var found bool
	for _, port := range deploy.Spec.Template.Spec.Containers[0].Ports {
		if port.Name == "webhook" && port.ContainerPort == 8086 {
			found = true
		}
	}
	if !found {
		t.Errorf("container ports = %+v, want a webhook port 8086", deploy.Spec.Template.Spec.Containers[0].Ports)
	}
	svc := ControlPlaneService("planton", "default", nil)
	found = false
	for _, port := range svc.Spec.Ports {
		if port.Name == "webhook" && port.Port == 8086 && port.AppProtocol != nil && *port.AppProtocol == "http" {
			found = true
		}
	}
	if !found {
		t.Errorf("service ports = %+v, want a webhook port 8086 with appProtocol http", svc.Spec.Ports)
	}
}
