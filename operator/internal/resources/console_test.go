package resources

import (
	"strings"
	"testing"

	appsv1 "k8s.io/api/apps/v1"
)

func TestConsoleDeployment_Image(t *testing.T) {
	cfg := ConsoleConfig{CRName: "planton", Namespace: "default", Version: "v1.0.0", Replicas: 1}
	deploy := ConsoleDeployment(cfg)

	expected := ConsoleDefaultImageRepo + ":v1.0.0"
	actual := deploy.Spec.Template.Spec.Containers[0].Image
	if actual != expected {
		t.Errorf("image = %s, want %s", actual, expected)
	}
}

func TestConsoleDeployment_ImageOverride(t *testing.T) {
	cfg := ConsoleConfig{
		CRName: "planton", Namespace: "default", Version: "v1.0.0",
		Replicas: 1, ImageRepository: "nginx", ImageTag: "1.27-alpine",
	}
	deploy := ConsoleDeployment(cfg)

	expected := "nginx:1.27-alpine"
	actual := deploy.Spec.Template.Spec.Containers[0].Image
	if actual != expected {
		t.Errorf("image = %s, want %s", actual, expected)
	}
}

func TestConsoleDeployment_APIEndpoint(t *testing.T) {
	cfg := ConsoleConfig{CRName: "planton", Namespace: "default", Version: "v1.0.0", Replicas: 1}
	deploy := ConsoleDeployment(cfg)
	envs := deploy.Spec.Template.Spec.Containers[0].Env

	var found bool
	for _, e := range envs {
		if e.Name == "API_ENDPOINT" {
			found = true
			if !strings.Contains(e.Value, "planton-control-plane") {
				t.Errorf("API_ENDPOINT = %s, expected to contain planton-control-plane", e.Value)
			}
			// The console speaks gRPC-Web from the browser; it must target the
			// control plane's gRPC-Web port, never the raw gRPC port, under the
			// API path namespace the door is served at.
			if !strings.HasSuffix(e.Value, ":8081"+APIPathPrefix) {
				t.Errorf("API_ENDPOINT = %s, expected the gRPC-Web port :8081 under %s", e.Value, APIPathPrefix)
			}
		}
	}
	if !found {
		t.Error("missing API_ENDPOINT env var")
	}
}

// The console must bind all interfaces (HOSTNAME=0.0.0.0). Next.js standalone binds
// process.env.HOSTNAME, which Kubernetes sets to the pod name -> pod IP only, refusing
// loopback and breaking kubectl port-forward. Regression guard.
func TestConsoleDeployment_BindsAllInterfaces(t *testing.T) {
	cfg := ConsoleConfig{CRName: "planton", Namespace: "default", Version: "v1.0.0", Replicas: 1}
	deploy := ConsoleDeployment(cfg)
	envs := deploy.Spec.Template.Spec.Containers[0].Env

	var found bool
	for _, e := range envs {
		if e.Name == "HOSTNAME" {
			found = true
			if e.Value != "0.0.0.0" {
				t.Errorf("HOSTNAME = %s, want 0.0.0.0", e.Value)
			}
		}
	}
	if !found {
		t.Error("missing HOSTNAME env var (needed so port-forward reaches the console)")
	}
}

func TestConsoleDeployment_Ports(t *testing.T) {
	cfg := ConsoleConfig{CRName: "planton", Namespace: "default", Version: "v1.0.0", Replicas: 1}
	deploy := ConsoleDeployment(cfg)
	ports := deploy.Spec.Template.Spec.Containers[0].Ports
	if len(ports) != 1 {
		t.Fatalf("expected 1 port, got %d", len(ports))
	}
	if ports[0].ContainerPort != 3000 {
		t.Errorf("container port = %d, want 3000", ports[0].ContainerPort)
	}
}

// The probe contract is a regression guard for a live kill-loop: the old
// render-priced ("/") readiness/liveness with a 3s timeout got the console
// killed by the kubelet under node load, mid-sign-in. Startup keeps the full
// render (proves the app boots); steady-state probes ride the cheap health
// endpoint with patient thresholds.
func TestConsoleDeployment_HTTPProbes(t *testing.T) {
	cfg := ConsoleConfig{CRName: "planton", Namespace: "default", Version: "v1.0.0", Replicas: 1}
	deploy := ConsoleDeployment(cfg)
	container := deploy.Spec.Template.Spec.Containers[0]

	startup := container.StartupProbe
	if startup == nil || startup.HTTPGet == nil {
		t.Fatal("expected HTTP startup probe")
	}
	if startup.HTTPGet.Path != "/" {
		t.Errorf("startup path = %q, want / (the one render-priced check, at boot)", startup.HTTPGet.Path)
	}
	if startup.FailureThreshold < 30 {
		t.Errorf("startup failureThreshold = %d, want a generous boot window (>= 30)", startup.FailureThreshold)
	}

	readiness := container.ReadinessProbe
	if readiness == nil || readiness.HTTPGet == nil {
		t.Fatal("expected HTTP readiness probe")
	}
	if readiness.HTTPGet.Path != consoleHealthzPath {
		t.Errorf("readiness path = %q, want %s (never render-priced)", readiness.HTTPGet.Path, consoleHealthzPath)
	}

	liveness := container.LivenessProbe
	if liveness == nil || liveness.HTTPGet == nil {
		t.Fatal("expected HTTP liveness probe")
	}
	if liveness.HTTPGet.Path != consoleHealthzPath {
		t.Errorf("liveness path = %q, want %s (never render-priced)", liveness.HTTPGet.Path, consoleHealthzPath)
	}
	// The restart-shy floor: >= 3 minutes of sustained unresponsiveness
	// before a kill (the thresholds that held under the load that
	// kill-looped the old probe).
	grace := liveness.PeriodSeconds * liveness.FailureThreshold
	if grace < 180 {
		t.Errorf("liveness grace = %ds (period x failures), want >= 180s", grace)
	}
	if liveness.TimeoutSeconds < 10 {
		t.Errorf("liveness timeout = %ds, want >= 10s (3s died under load)", liveness.TimeoutSeconds)
	}
}

// Without a resource request the console can be starved on a busy node into
// failing its own probes; without a memory limit floor it can be OOM-killed
// confusingly (the Postgres/identity lesson). No CPU limit: renders are
// bursty and throttling recreates the slowness probes then punish.
func TestConsoleDeployment_ResourceFloor(t *testing.T) {
	cfg := ConsoleConfig{CRName: "planton", Namespace: "default", Version: "v1.0.0", Replicas: 1}
	res := ConsoleDeployment(cfg).Spec.Template.Spec.Containers[0].Resources

	if res.Requests.Cpu().IsZero() || res.Requests.Memory().IsZero() {
		t.Error("expected CPU + memory requests (starvation guard)")
	}
	if res.Limits.Memory().IsZero() {
		t.Error("expected a memory limit floor")
	}
	if !res.Limits.Cpu().IsZero() {
		t.Error("CPU must not be limited (bursty renders must never be throttled)")
	}
}

func TestConsoleDeployment_ExternalConfig(t *testing.T) {
	cfg := ConsoleConfig{
		CRName: "planton", Namespace: "default", Version: "v1.0.0",
		Replicas: 1, ExternalConfigSecretName: "console-config",
	}
	deploy := ConsoleDeployment(cfg)
	envFrom := deploy.Spec.Template.Spec.Containers[0].EnvFrom
	if len(envFrom) != 1 {
		t.Fatalf("expected 1 envFrom, got %d", len(envFrom))
	}
	if envFrom[0].SecretRef == nil || envFrom[0].SecretRef.Name != "console-config" {
		t.Error("expected external config secretRef")
	}
}

// With ingress, the browser reaches the API through the public URL and auth
// callbacks derive from the same hostname -- nothing hand-configured.
func TestConsoleDeployment_PublicURL(t *testing.T) {
	const publicURL = "https://planton.example.com"
	cfg := ConsoleConfig{
		CRName: "planton", Namespace: "default", Version: "v1.0.0",
		Replicas: 1, PublicURL: publicURL,
	}
	deploy := ConsoleDeployment(cfg)

	env := map[string]string{}
	for _, e := range deploy.Spec.Template.Spec.Containers[0].Env {
		env[e.Name] = e.Value
	}
	if env["API_ENDPOINT"] != publicURL+APIPathPrefix {
		t.Errorf("API_ENDPOINT = %s, want the public URL under %s", env["API_ENDPOINT"], APIPathPrefix)
	}
	if env["NEXTAUTH_URL"] != publicURL {
		t.Errorf("NEXTAUTH_URL = %s, want the public URL", env["NEXTAUTH_URL"])
	}
}

// The console publishes what the deployment declares about itself: the
// deployment shape always (the same fact the control plane boots with), the
// native gRPC address only when the component says the front door routes it.
func TestConsoleDeployment_DeviceDiscoveryFacts(t *testing.T) {
	base := ConsoleConfig{CRName: "planton", Namespace: "default", Version: "v1.0.0", Replicas: 1,
		PublicURL: "https://planton.example.com"}

	env := consoleEnv(ConsoleDeployment(base))
	if env["PLANTON_DEPLOYMENT_KIND"] != DeploymentKindSelfHosted {
		t.Errorf("PLANTON_DEPLOYMENT_KIND = %q, want %s on every install", env["PLANTON_DEPLOYMENT_KIND"], DeploymentKindSelfHosted)
	}
	if _, present := env["GRPC_ENDPOINT"]; present {
		t.Error("GRPC_ENDPOINT must not be published when the front door does not route native gRPC")
	}

	withGRPC := base
	withGRPC.GRPCEndpoint = GRPCEndpoint(base.PublicURL)
	env = consoleEnv(ConsoleDeployment(withGRPC))
	if env["GRPC_ENDPOINT"] != "planton.example.com:443" {
		t.Errorf("GRPC_ENDPOINT = %q, want planton.example.com:443", env["GRPC_ENDPOINT"])
	}
}

func consoleEnv(deploy *appsv1.Deployment) map[string]string {
	env := map[string]string{}
	for _, e := range deploy.Spec.Template.Spec.Containers[0].Env {
		env[e.Name] = e.Value
	}
	return env
}

func TestConsoleDeployment_NoNextAuthURLWithoutIngress(t *testing.T) {
	cfg := ConsoleConfig{CRName: "planton", Namespace: "default", Version: "v1.0.0", Replicas: 1}
	deploy := ConsoleDeployment(cfg)
	for _, e := range deploy.Spec.Template.Spec.Containers[0].Env {
		if e.Name == "NEXTAUTH_URL" {
			t.Error("NEXTAUTH_URL must not be set without a public URL")
		}
	}
}

// A published console signs in through the bundled identity server: the full
// issuer URL (path-mounted, scheme included -- the plain-HTTP evaluation step
// must work), the client credentials by Secret reference, and in-pod
// self-calls for the auth routes.
func TestConsoleDeployment_IdentityWiring(t *testing.T) {
	const issuer = "http://planton.example.com/idp/realms/planton"
	const realm = "planton"
	const internalIssuer = "http://planton-identity.default.svc.cluster.local/idp/realms/planton"
	cfg := ConsoleConfig{
		CRName: "planton", Namespace: "default", Version: "v1.0.0",
		Replicas:  1,
		PublicURL: "http://planton.example.com",
		Identity: &ConsoleIdentityConfig{
			IssuerURL:         issuer,
			InternalIssuerURL: internalIssuer,
			Realm:             realm,
		},
	}
	deploy := ConsoleDeployment(cfg)
	envMap := envVarMap(deploy.Spec.Template.Spec.Containers[0].Env)

	if envMap["IDP_PROVIDER"] != IdentityProviderKeycloak {
		t.Errorf("IDP_PROVIDER = %q, want keycloak", envMap["IDP_PROVIDER"])
	}
	if envMap["IAM_ISSUER_URL"] != issuer {
		t.Errorf("IAM_ISSUER_URL = %q, want the full path-mounted issuer", envMap["IAM_ISSUER_URL"])
	}
	// Split horizon: the pod's server-side token/userinfo fetches must never
	// depend on the advertised issuer being reachable from inside the cluster.
	if envMap["IAM_ISSUER_INTERNAL_URL"] != internalIssuer {
		t.Errorf("IAM_ISSUER_INTERNAL_URL = %q, want the in-cluster issuer address", envMap["IAM_ISSUER_INTERNAL_URL"])
	}
	if envMap["IAM_REALM"] != realm {
		t.Errorf("IAM_REALM = %q, want planton", envMap["IAM_REALM"])
	}
	if envMap["IAM_CLIENT_ID"] != IdentityConsoleClientID {
		t.Errorf("IAM_CLIENT_ID = %q, want %s", envMap["IAM_CLIENT_ID"], IdentityConsoleClientID)
	}
	// The device-auth discovery route publishes this so a CLI pointed at the
	// instance URL learns the public PKCE client id with nothing hand-typed.
	if envMap["IAM_CLI_CLIENT_ID"] != IdentityCLIClientID {
		t.Errorf("IAM_CLI_CLIENT_ID = %q, want %s", envMap["IAM_CLI_CLIENT_ID"], IdentityCLIClientID)
	}
	// Credentials by reference, never literals in the pod spec.
	if envMap["IAM_CLIENT_SECRET"] != fromSecretRef {
		t.Error("IAM_CLIENT_SECRET must come from the OIDC client Secret")
	}
	if envMap["NEXTAUTH_SECRET"] != fromSecretRef {
		t.Error("NEXTAUTH_SECRET must come from the NextAuth Secret")
	}
	if envMap["NEXTAUTH_URL_INTERNAL"] != "http://localhost:3000" {
		t.Errorf("NEXTAUTH_URL_INTERNAL = %q, want the in-pod address", envMap["NEXTAUTH_URL_INTERNAL"])
	}
}

func TestConsoleDeployment_NoIdentityEnvWithoutIdentity(t *testing.T) {
	cfg := ConsoleConfig{CRName: "planton", Namespace: "default", Version: "v1.0.0", Replicas: 1}
	deploy := ConsoleDeployment(cfg)
	envMap := envVarMap(deploy.Spec.Template.Spec.Containers[0].Env)
	for _, name := range []string{"IDP_PROVIDER", "IAM_ISSUER_URL", "IAM_CLIENT_ID", "NEXTAUTH_SECRET"} {
		if _, ok := envMap[name]; ok {
			t.Errorf("%s must not be set without identity (no-ingress installs stay unchanged)", name)
		}
	}
}

// The console decides its billing visibility ITSELF by asking the control
// plane what shape it is (the instance-entitlements advertisement) -- the
// operator must never inject billing kill switches. The old env-var pair
// had no reader in the console; injecting it again would be dead config
// masquerading as a working off switch.
func TestConsoleDeployment_NoBillingKillSwitches(t *testing.T) {
	cfg := ConsoleConfig{CRName: "planton", Namespace: "default", Version: "v1.0.0", Replicas: 1}
	envMap := envVarMap(ConsoleDeployment(cfg).Spec.Template.Spec.Containers[0].Env)

	for _, name := range []string{"BILLING_ALERT_ENABLED", "BILLING_ENFORCEMENT_ENABLED"} {
		if _, ok := envMap[name]; ok {
			t.Errorf("%s must not be injected (the console derives billing visibility from the advertisement)", name)
		}
	}
}

func TestConsoleService(t *testing.T) {
	svc := ConsoleService("planton", "default", nil)
	if svc.Name != "planton-console" {
		t.Errorf("name = %s, want planton-console", svc.Name)
	}
	if svc.Spec.Type != "ClusterIP" {
		t.Errorf("type = %s, want ClusterIP", svc.Spec.Type)
	}
	if svc.Spec.Ports[0].Port != 80 {
		t.Errorf("port = %d, want 80", svc.Spec.Ports[0].Port)
	}
}
