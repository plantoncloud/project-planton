package resources

import (
	"fmt"
	"strings"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

const (
	// GatewayDefaultImageRepo is the official nginx image the front-door
	// gateway runs. Overridable via spec.gateway.image for mirrored/air-gapped
	// registries, like every other image the operator pulls.
	GatewayDefaultImageRepo = "nginx"
	// GatewayDefaultImageTag pins a specific stable line rather than
	// "latest", so installs are reproducible.
	GatewayDefaultImageTag = "1.27-alpine"

	// GatewayDefaultLocalPort is the workstation port the port-forward is
	// expected to serve the platform at when spec.gateway.localPort is unset.
	// Sign-in bakes this port into the advertised issuer and auth callback
	// URLs, which is why it is spec-configurable rather than free-form.
	GatewayDefaultLocalPort = int32(8080)

	gatewayContainerPort = 8080
	gatewayServicePort   = 80

	// GatewayConfigKey is the data key of the nginx config TEMPLATE in the
	// ConfigMap. It is a template (mounted under /etc/nginx/templates) rather
	// than a literal conf.d file because the image's entrypoint substitutes
	// ${NGINX_LOCAL_RESOLVERS} -- the cluster DNS server from the pod's own
	// resolv.conf -- which the config needs for request-time upstream
	// resolution (see GatewayNginxConfig).
	GatewayConfigKey = "default.conf.template"

	gatewayTemplatesMountPath = "/etc/nginx/templates"
)

// GatewayConfig bundles all inputs needed to build the front-door gateway.
type GatewayConfig struct {
	CRName    string
	Namespace string
	OwnerRef  *metav1.OwnerReference

	ImageRepository string
	ImageTag        string

	// ConfigHash rolls the pod when the rendered nginx config changes
	// (nginx only reads its config at startup).
	ConfigHash string
}

// GatewayDeploymentName returns the Deployment name: "{crName}-gateway".
func GatewayDeploymentName(crName string) string {
	return fmt.Sprintf("%s-gateway", crName)
}

// GatewayServiceName returns the Service name: "{crName}-gateway".
func GatewayServiceName(crName string) string {
	return fmt.Sprintf("%s-gateway", crName)
}

// GatewayConfigMapName returns the nginx config ConfigMap name:
// "{crName}-gateway-config".
func GatewayConfigMapName(crName string) string {
	return fmt.Sprintf("%s-gateway-config", crName)
}

// GatewayPortForwardCommand renders the exact command that opens the platform
// on the workstation. Printed in the gateway component's status so nobody
// composes it by hand -- and the local port matters: sign-in URLs are pinned
// to it.
func GatewayPortForwardCommand(crName, namespace string, localPort int32) string {
	return fmt.Sprintf("kubectl port-forward -n %s svc/%s %d:%d",
		namespace, GatewayServiceName(crName), localPort, gatewayServicePort)
}

// GatewayNginxConfig renders the nginx server block from the front-door
// route table (front_door_routes.go): one location per route, in table order.
// Rendering from the same table the Ingress and HTTPRoute builders use is what
// makes the port-forward door and the public doors the same architecture at
// different addresses -- URLs, sign-in callbacks, and docs carry over
// unchanged when an install graduates from port-forward to a real hostname.
//
// Notes on the directives:
//   - Upstreams are variables + a resolver, NOT literal proxy_pass hosts:
//     nginx resolves literal upstreams once at startup and refuses to
//     boot if any is absent -- but the gateway deliberately starts before
//     the platform Services exist (it is the front door, like an ingress
//     controller). Variables defer resolution to request time, so missing
//     backends are an honest 502, never a crash loop. The resolver is the
//     cluster DNS server, injected by the nginx image's own entrypoint
//     (${NGINX_LOCAL_RESOLVERS} from the pod's resolv.conf) -- which is
//     also why this renders as an entrypoint TEMPLATE, not a literal
//     conf.d file. Upstream names are FQDNs because request-time
//     resolution bypasses resolv.conf search domains.
//   - A plain prefix location per route (nginx picks the longest match),
//     the same segment-prefix meaning the Ingress and HTTPRoute rules carry.
//   - Control-plane routes: proxy_buffering off + long read timeout, because
//     gRPC-Web server-streaming (deploy progress, log tails) is a long-lived
//     chunked response; client_max_body_size raised to the server's own
//     100m limit because nginx's 1m default would 413 large manifest applies
//     and state-file uploads (the relay additionally enforces its 50MB
//     transfer cap itself).
//   - Identity and console routes: proxy_set_header Host preserves the
//     browser's localhost:port origin, from which the identity server's
//     sign-in pages and OIDC responses derive URLs (KC_PROXY_HEADERS=
//     xforwarded) and the console its callbacks.
func GatewayNginxConfig(crName, namespace string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "resolver ${NGINX_LOCAL_RESOLVERS} valid=10s;\n\nserver {\n    listen %d;\n\n", gatewayContainerPort)

	routes := FrontDoorRoutes()
	upstreamVar := func(r FrontDoorRoute) string {
		switch r.Backend {
		case BackendIdentity:
			return "$identity_upstream"
		case BackendConsole:
			return "$console_upstream"
		default:
			return "$controlplane_upstream"
		}
	}
	for _, r := range []FrontDoorRoute{
		{Backend: BackendControlPlane}, {Backend: BackendIdentity}, {Backend: BackendConsole},
	} {
		fmt.Fprintf(&b, "    set %s http://%s.%s.svc.cluster.local:%d;\n",
			upstreamVar(r), r.ServiceName(crName), namespace, r.ServicePort())
	}

	for _, r := range routes {
		if r.HeaderMatched() {
			// The port-forward door serves a workstation; a native gRPC client
			// on that workstation port-forwards the control plane's raw gRPC
			// port directly, so the header-matched row has no job here (and
			// nginx would need a map on $http_content_type to express it).
			continue
		}
		fmt.Fprintf(&b, "\n    location %s {\n        proxy_pass %s;\n        proxy_http_version 1.1;\n",
			r.PathPrefix, upstreamVar(r))
		switch r.Backend {
		case BackendControlPlane:
			b.WriteString("        proxy_buffering off;\n        proxy_request_buffering off;\n" +
				"        proxy_read_timeout 3600s;\n        proxy_send_timeout 3600s;\n" +
				"        client_max_body_size 100m;\n")
		default:
			b.WriteString("        proxy_set_header Host $http_host;\n" +
				"        proxy_set_header X-Forwarded-For $remote_addr;\n" +
				"        proxy_set_header X-Forwarded-Proto $scheme;\n" +
				"        proxy_set_header X-Forwarded-Host $http_host;\n" +
				"        proxy_buffering off;\n")
		}
		b.WriteString("    }\n")
	}
	b.WriteString("}\n")
	return b.String()
}

// GatewayConfigMap builds the ConfigMap carrying the rendered nginx config
// template.
func GatewayConfigMap(crName, namespace string, ownerRef *metav1.OwnerReference) *corev1.ConfigMap {
	cm := &corev1.ConfigMap{
		TypeMeta: metav1.TypeMeta{APIVersion: "v1", Kind: "ConfigMap"},
		ObjectMeta: metav1.ObjectMeta{
			Name:      GatewayConfigMapName(crName),
			Namespace: namespace,
			Labels:    gatewayLabels(crName),
		},
		Data: map[string]string{
			GatewayConfigKey: GatewayNginxConfig(crName, namespace),
		},
	}
	if ownerRef != nil {
		cm.OwnerReferences = []metav1.OwnerReference{*ownerRef}
	}
	return cm
}

// GatewayDeployment builds the front-door gateway Deployment: a single nginx
// replica proxying the three platform surfaces. Deliberately no readiness
// dependency on its upstreams -- like an ingress controller, the gateway comes
// up immediately and serves 502s for backends that are still booting, so the
// port-forward command works (and shows honest progress) from the first
// minute of an install.
func GatewayDeployment(cfg GatewayConfig) *appsv1.Deployment {
	imageRepo := cfg.ImageRepository
	if imageRepo == "" {
		imageRepo = GatewayDefaultImageRepo
	}
	imageTag := cfg.ImageTag
	if imageTag == "" {
		imageTag = GatewayDefaultImageTag
	}

	labels := gatewayLabels(cfg.CRName)
	replicas := int32(1)

	deploy := &appsv1.Deployment{
		TypeMeta: metav1.TypeMeta{APIVersion: "apps/v1", Kind: "Deployment"},
		ObjectMeta: metav1.ObjectMeta{
			Name:      GatewayDeploymentName(cfg.CRName),
			Namespace: cfg.Namespace,
			Labels:    labels,
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: &replicas,
			Selector: &metav1.LabelSelector{MatchLabels: labels},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: labels,
					Annotations: map[string]string{
						"planton.ai/gateway-config-hash": cfg.ConfigHash,
					},
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{{
						Name:  "gateway",
						Image: fmt.Sprintf("%s:%s", imageRepo, imageTag),
						Env: []corev1.EnvVar{{
							// Opt-in switch for the image's 15-local-resolvers
							// entrypoint script: without it NGINX_LOCAL_RESOLVERS
							// is never exported and the template's resolver
							// directive survives unsubstituted (observed live as
							// a crash loop).
							Name:  "NGINX_ENTRYPOINT_LOCAL_RESOLVERS",
							Value: "true",
						}},
						Ports: []corev1.ContainerPort{{
							Name:          "http",
							ContainerPort: gatewayContainerPort,
							Protocol:      corev1.ProtocolTCP,
						}},
						VolumeMounts: []corev1.VolumeMount{{
							Name:      "nginx-config",
							MountPath: gatewayTemplatesMountPath,
							ReadOnly:  true,
						}},
						ReadinessProbe: &corev1.Probe{
							ProbeHandler: corev1.ProbeHandler{
								TCPSocket: &corev1.TCPSocketAction{
									Port: intstr.FromInt32(gatewayContainerPort),
								},
							},
							InitialDelaySeconds: 2,
							PeriodSeconds:       5,
							TimeoutSeconds:      3,
							FailureThreshold:    3,
						},
						LivenessProbe: &corev1.Probe{
							ProbeHandler: corev1.ProbeHandler{
								TCPSocket: &corev1.TCPSocketAction{
									Port: intstr.FromInt32(gatewayContainerPort),
								},
							},
							InitialDelaySeconds: 5,
							PeriodSeconds:       30,
							TimeoutSeconds:      3,
							FailureThreshold:    3,
						},
					}},
					Volumes: []corev1.Volume{{
						Name: "nginx-config",
						VolumeSource: corev1.VolumeSource{
							ConfigMap: &corev1.ConfigMapVolumeSource{
								LocalObjectReference: corev1.LocalObjectReference{
									Name: GatewayConfigMapName(cfg.CRName),
								},
							},
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

// GatewayService builds the ClusterIP Service the port-forward targets,
// exposing the gateway on port 80 (mapped to the nginx container port).
func GatewayService(crName, namespace string, ownerRef *metav1.OwnerReference) *corev1.Service {
	labels := gatewayLabels(crName)

	svc := &corev1.Service{
		TypeMeta: metav1.TypeMeta{APIVersion: "v1", Kind: "Service"},
		ObjectMeta: metav1.ObjectMeta{
			Name:      GatewayServiceName(crName),
			Namespace: namespace,
			Labels:    labels,
		},
		Spec: corev1.ServiceSpec{
			Type:     corev1.ServiceTypeClusterIP,
			Selector: labels,
			Ports: []corev1.ServicePort{{
				Name:       "http",
				Port:       gatewayServicePort,
				TargetPort: intstr.FromInt32(gatewayContainerPort),
				Protocol:   corev1.ProtocolTCP,
			}},
		},
	}

	if ownerRef != nil {
		svc.OwnerReferences = []metav1.OwnerReference{*ownerRef}
	}

	return svc
}

func gatewayLabels(crName string) map[string]string {
	return map[string]string{
		"app.kubernetes.io/name":       "gateway",
		"app.kubernetes.io/instance":   crName,
		"app.kubernetes.io/managed-by": ManagedByLabel,
		"app.kubernetes.io/component":  "networking",
	}
}
