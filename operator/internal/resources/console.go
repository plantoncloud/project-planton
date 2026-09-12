package resources

import (
	"fmt"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

const (
	ConsoleDefaultImageRepo = "ghcr.io/plantonhq/planton/client-apps/web"
	consoleContainerPort    = 3000
	consoleServicePort      = 80

	// consoleHealthzPath is the console's purpose-built health endpoint: no
	// auth, no data fetch, no React render -- it answers as long as the
	// server's event loop is alive. Steady-state probes MUST use it: probing
	// a full page render ("/") priced every check at seconds of CPU, and on
	// a loaded node the render blew the probe timeout and the kubelet killed
	// the console mid-sign-in. NOTE: the path assumes a console image that
	// ships the endpoint -- the same image-contemporaneity contract as the
	// control plane's gRPC-Web port and the split-horizon issuer env; image
	// lines publish coherently with the operator.
	consoleHealthzPath = "/api/healthz"
)

// ConsoleConfig bundles all inputs needed to build the Console Deployment.
type ConsoleConfig struct {
	CRName    string
	Namespace string
	Version   string
	OwnerRef  *metav1.OwnerReference

	Replicas                 int32
	ImageRepository          string
	ImageTag                 string
	ExternalConfigSecretName string

	// PublicURL is the browser-facing front-door URL: the ingress hostname
	// URL, or the gateway's localhost port-forward URL. The browser calls
	// the API through it, and auth callback URLs derive from it -- one
	// origin, nothing hand-configured.
	PublicURL string

	// GRPCEndpoint is the host:port a native gRPC client (the CLI, the
	// runner) dials, published by the console's device discovery document.
	// Set by the component only when the front door actually routes native
	// gRPC (the Gateway API edge renders the content-type rule; the Ingress
	// and port-forward doors do not), so the document never advertises an
	// address that would answer with a console page.
	GRPCEndpoint string

	// Identity wires the console's sign-in stack to the bundled identity
	// server. Always set by the component once the front-door URL is known
	// -- every install signs in.
	Identity *ConsoleIdentityConfig
}

// ConsoleIdentityConfig carries the sign-in wiring for the console.
type ConsoleIdentityConfig struct {
	// IssuerURL is the OIDC issuer (front-door URL + identity path + realm)
	// -- the ADVERTISED horizon: it drives the browser-facing authorize
	// redirect and the iss validation on tokens.
	IssuerURL string

	// InternalIssuerURL is the same issuer at its in-cluster Service address
	// -- the FETCH horizon for the console pod's server-side calls (token
	// exchange, refresh, userinfo). In gateway mode the advertised localhost
	// URL is not reachable from the pod at all; with ingress it removes the
	// hairpin requirement.
	InternalIssuerURL string

	// Realm is the identity server realm name.
	Realm string
}

// ConsoleNextAuthSecretName holds the console's session-cookie encryption
// secret: "{crName}-console-nextauth".
func ConsoleNextAuthSecretName(crName string) string {
	return fmt.Sprintf("%s-console-nextauth", crName)
}

// ConsoleNextAuthSecretKey is the data key in the NextAuth Secret.
const ConsoleNextAuthSecretKey = "secret"

// ConsoleDeploymentName returns the Deployment name: "{crName}-console".
func ConsoleDeploymentName(crName string) string {
	return fmt.Sprintf("%s-console", crName)
}

// ConsoleServiceName returns the Service name: "{crName}-console".
func ConsoleServiceName(crName string) string {
	return fmt.Sprintf("%s-console", crName)
}

// ConsoleDeployment builds the Kubernetes Deployment for the web console.
// The console receives the API_ENDPOINT env var pointing to the control
// plane's ClusterIP Service. All other configuration (Auth0, NextAuth, etc.)
// comes from the optional external config Secret.
func ConsoleDeployment(cfg ConsoleConfig) *appsv1.Deployment {
	imageRepo := cfg.ImageRepository
	if imageRepo == "" {
		imageRepo = ConsoleDefaultImageRepo
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
		"app.kubernetes.io/name":       "console",
		"app.kubernetes.io/instance":   cfg.CRName,
		"app.kubernetes.io/managed-by": ManagedByLabel,
		"app.kubernetes.io/component":  "application",
	}

	// The console's connect client speaks gRPC-Web from the browser, so it
	// must point at the control plane's gRPC-Web door -- the raw gRPC port
	// would refuse the browser dialect. The browser reaches the API through
	// the front-door URL, under the API path namespace every front door
	// routes (APIPathPrefix); the client appends "/<service>/<method>". The
	// in-cluster fallback only covers the transient window before the
	// front-door URL resolves, and carries the same prefix because the
	// control plane serves its door under it everywhere.
	apiOrigin := fmt.Sprintf("http://%s:%d",
		ControlPlaneServiceFQDN(cfg.CRName, cfg.Namespace), controlPlaneGrpcWebPort)
	if cfg.PublicURL != "" {
		apiOrigin = cfg.PublicURL
	}
	apiEndpoint := APIURL(apiOrigin)

	envVars := []corev1.EnvVar{
		{Name: "API_ENDPOINT", Value: apiEndpoint},
		// The deployment shape, the same declared fact the control plane
		// boots with. The console publishes it in its device discovery
		// document so a CLI or desktop pointed at this instance learns what
		// it is from the instance itself, never from its hostname.
		{Name: "PLANTON_DEPLOYMENT_KIND", Value: DeploymentKindSelfHosted},
		// Bind the Next.js standalone server on all interfaces. It listens on
		// process.env.HOSTNAME, which Kubernetes sets to the pod name (resolving to
		// the pod IP), so it would otherwise bind that single interface and refuse
		// loopback -- breaking `kubectl port-forward`. 0.0.0.0 keeps Service/ingress
		// access working and makes port-forward reachable.
		// Billing surfaces need no kill switch here: the console asks the
		// control plane what shape it is (the instance-entitlements
		// advertisement) and hides the billing rail on self-hosted installs
		// by itself.
		{Name: "HOSTNAME", Value: "0.0.0.0"},
	}
	if cfg.PublicURL != "" {
		// Auth callbacks derive from the front-door URL -- the sign-in stack
		// (NextAuth) builds its redirect/callback endpoints from NEXTAUTH_URL,
		// so giving the platform its address is the ONLY step; nobody
		// hand-configures a callback.
		envVars = append(envVars, corev1.EnvVar{Name: "NEXTAUTH_URL", Value: cfg.PublicURL})
	}
	if cfg.GRPCEndpoint != "" {
		// Published by the discovery document beside the sign-in facts: the
		// one address a native gRPC client needs, declared by the deployment
		// that serves it.
		envVars = append(envVars, corev1.EnvVar{Name: "GRPC_ENDPOINT", Value: cfg.GRPCEndpoint})
	}
	if cfg.Identity != nil {
		envVars = append(envVars,
			corev1.EnvVar{Name: "IDP_PROVIDER", Value: IdentityProviderKeycloak},
			// The full issuer URL rather than a composed domain: the identity
			// server lives under a path on the shared hostname, and the
			// plain-HTTP steps must work too -- neither survives the sign-in
			// stack's default https://{domain}/realms/{realm} composition.
			corev1.EnvVar{Name: "IAM_ISSUER_URL", Value: cfg.Identity.IssuerURL},
			// Split horizon: the pod's server-side token/userinfo fetches go
			// to the issuer's in-cluster address; the browser-facing
			// authorize redirect and iss validation stay on the advertised
			// issuer above.
			corev1.EnvVar{Name: "IAM_ISSUER_INTERNAL_URL", Value: cfg.Identity.InternalIssuerURL},
			corev1.EnvVar{Name: "IAM_REALM", Value: cfg.Identity.Realm},
			corev1.EnvVar{Name: "IAM_CLIENT_ID", Value: IdentityConsoleClientID},
			// Published by the console's device-auth discovery route so a
			// CLI pointed at the instance URL learns the public PKCE client
			// to sign in with -- the constant would otherwise be known only
			// to this operator, an invisible drift hazard.
			corev1.EnvVar{Name: "IAM_CLI_CLIENT_ID", Value: IdentityCLIClientID},
			secretEnv("IAM_CLIENT_SECRET", IdentityOIDCClientSecretName(cfg.CRName), IdentityOIDCClientSecretKey),
			secretEnv("NEXTAUTH_SECRET", ConsoleNextAuthSecretName(cfg.CRName), ConsoleNextAuthSecretKey),
			// The console's server-side calls to its own auth routes stay
			// in-pod instead of round-tripping through the front door.
			corev1.EnvVar{Name: "NEXTAUTH_URL_INTERNAL", Value: fmt.Sprintf("http://localhost:%d", consoleContainerPort)},
		)
	}

	var envFrom []corev1.EnvFromSource
	if cfg.ExternalConfigSecretName != "" {
		envFrom = append(envFrom, corev1.EnvFromSource{
			SecretRef: &corev1.SecretEnvSource{
				LocalObjectReference: corev1.LocalObjectReference{
					Name: cfg.ExternalConfigSecretName,
				},
			},
		})
	}

	deploy := &appsv1.Deployment{
		TypeMeta: metav1.TypeMeta{APIVersion: "apps/v1", Kind: "Deployment"},
		ObjectMeta: metav1.ObjectMeta{
			Name:      ConsoleDeploymentName(cfg.CRName),
			Namespace: cfg.Namespace,
			Labels:    labels,
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: &replicas,
			Selector: &metav1.LabelSelector{MatchLabels: labels},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{Labels: labels},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{{
						Name:    "console",
						Image:   fmt.Sprintf("%s:%s", imageRepo, imageTag),
						Ports:   []corev1.ContainerPort{{Name: "http", ContainerPort: consoleContainerPort, Protocol: corev1.ProtocolTCP}},
						Env:     envVars,
						EnvFrom: envFrom,
						// Explicit floor (the same lesson as Postgres and the
						// identity server): without a request the console can
						// be CPU/memory-starved on a busy node into failing
						// its own probes. No CPU limit -- page renders are
						// bursty and throttling them recreates the slowness
						// the probes then punish.
						Resources: corev1.ResourceRequirements{
							Requests: corev1.ResourceList{
								corev1.ResourceCPU:    resource.MustParse("250m"),
								corev1.ResourceMemory: resource.MustParse("512Mi"),
							},
							Limits: corev1.ResourceList{
								corev1.ResourceMemory: resource.MustParse("2Gi"),
							},
						},
						// Startup is the ONE moment a full page render is the
						// right check -- it proves the app genuinely boots
						// (build intact, env sane), and a kill on persistent
						// failure is correct there. Generous window for slow
						// or emulated nodes; readiness/liveness take over on
						// the cheap endpoint once startup succeeds.
						StartupProbe: &corev1.Probe{
							ProbeHandler: corev1.ProbeHandler{
								HTTPGet: &corev1.HTTPGetAction{
									Path: "/",
									Port: intstr.FromInt32(consoleContainerPort),
								},
							},
							InitialDelaySeconds: 5,
							PeriodSeconds:       5,
							TimeoutSeconds:      10,
							FailureThreshold:    60,
						},
						// Cheap-endpoint-priced: a render-priced readiness
						// would drop the single replica out of its Service
						// under load -- the gateway then 502s mid-sign-in,
						// making load failure worse instead of safer.
						ReadinessProbe: &corev1.Probe{
							ProbeHandler: corev1.ProbeHandler{
								HTTPGet: &corev1.HTTPGetAction{
									Path: consoleHealthzPath,
									Port: intstr.FromInt32(consoleContainerPort),
								},
							},
							PeriodSeconds:    10,
							TimeoutSeconds:   5,
							FailureThreshold: 3,
						},
						// Restart-shy on purpose: a kill is only correct for
						// genuine event-loop death, so the console gets 3+
						// minutes of sustained unresponsiveness (30s x 6,
						// 10s timeout) before the kubelet intervenes. These
						// are the thresholds that held up under the load that
						// kill-looped the old render-priced 3s probe.
						LivenessProbe: &corev1.Probe{
							ProbeHandler: corev1.ProbeHandler{
								HTTPGet: &corev1.HTTPGetAction{
									Path: consoleHealthzPath,
									Port: intstr.FromInt32(consoleContainerPort),
								},
							},
							PeriodSeconds:    30,
							TimeoutSeconds:   10,
							FailureThreshold: 6,
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

// ConsoleService builds the ClusterIP Service exposing the web console
// on port 80 (mapped to container port 3000).
func ConsoleService(crName, namespace string, ownerRef *metav1.OwnerReference) *corev1.Service {
	labels := map[string]string{
		"app.kubernetes.io/name":       "console",
		"app.kubernetes.io/instance":   crName,
		"app.kubernetes.io/managed-by": ManagedByLabel,
		"app.kubernetes.io/component":  "application",
	}

	svc := &corev1.Service{
		TypeMeta: metav1.TypeMeta{APIVersion: "v1", Kind: "Service"},
		ObjectMeta: metav1.ObjectMeta{
			Name:      ConsoleServiceName(crName),
			Namespace: namespace,
			Labels:    labels,
		},
		Spec: corev1.ServiceSpec{
			Type:     corev1.ServiceTypeClusterIP,
			Selector: labels,
			Ports: []corev1.ServicePort{{
				Name:       "http",
				Port:       consoleServicePort,
				TargetPort: intstr.FromInt32(consoleContainerPort),
				Protocol:   corev1.ProtocolTCP,
			}},
		},
	}

	if ownerRef != nil {
		svc.OwnerReferences = []metav1.OwnerReference{*ownerRef}
	}

	return svc
}
