package resources

import (
	"fmt"
	"maps"
	"strings"

	networkingv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	// NginxIngressController is the controller value ingress-nginx registers
	// on its IngressClass. Used to apply nginx-specific TUNING only where it
	// means something; nothing about routing depends on the controller (see
	// front_door_routes.go).
	NginxIngressController = "k8s.io/ingress-nginx"

	// certManagerIssuerAnnotation / certManagerClusterIssuerAnnotation ask
	// cert-manager's ingress-shim to obtain and renew the TLS certificate.
	certManagerIssuerAnnotation        = "cert-manager.io/issuer"
	certManagerClusterIssuerAnnotation = "cert-manager.io/cluster-issuer"
)

// IngressConfig bundles all inputs needed to build the Ingress resource. The
// builder is pure: class resolution, controller detection, and auto-hostname
// derivation happen in the component; this only shapes the object.
type IngressConfig struct {
	CRName    string
	Namespace string
	OwnerRef  *metav1.OwnerReference

	// Hostname scopes the rules to one host. Empty means host-less (match any
	// host) -- the transient state while an auto-derived hostname is being
	// resolved from the controller's published address.
	Hostname string

	// HostnameDerived marks Hostname as operator-derived rather than
	// spec-declared; the builder then records it under
	// DerivedHostnameAnnotation so later reconciles reuse it instead of
	// re-deriving (see the annotation's doc for why derivation is
	// once-only).
	HostnameDerived bool

	// IngressClassName is the class from the spec. Empty relies on the
	// cluster's default IngressClass.
	IngressClassName string

	// Annotations are the user's passthrough annotations. They win over the
	// operator's controller-specific defaults.
	Annotations map[string]string

	// NginxController marks that the effective IngressClass is served by
	// ingress-nginx, enabling its streaming-friendly defaults.
	NginxController bool

	// TLSSecretName enables HTTPS with an existing certificate Secret.
	TLSSecretName string

	// CertManagerIssuerName/Kind enable HTTPS via cert-manager; the
	// certificate Secret name is derived (IngressTLSSecretName).
	CertManagerIssuerName string
	CertManagerIssuerKind string
}

// IngressName returns the Ingress name: "{crName}-ingress".
func IngressName(crName string) string {
	return fmt.Sprintf("%s-ingress", crName)
}

// IngressTLSSecretName returns the derived certificate Secret name used when
// cert-manager issues the certificate: "{crName}-ingress-tls".
func IngressTLSSecretName(crName string) string {
	return fmt.Sprintf("%s-ingress-tls", crName)
}

// AutoHostname derives a hostname from an ingress controller's published
// address when the user configured none. A published DNS name (cloud load
// balancers) is used directly; a bare IP becomes a sslip.io magic-DNS name
// that resolves to itself -- a working URL with zero DNS setup, for
// evaluation rather than production.
//
// The magic-DNS label carries the platform's name AND namespace: platforms
// share the cluster's ingress controller (one published address), so a
// fixed label would derive the identical hostname for every platform behind
// it and the controller would route the loser's traffic to the winner. A
// published DNS name cannot be qualified the same way (it is a real record,
// not a wildcard) -- a second platform behind such a controller must set
// spec.ingress.hostname.
func AutoHostname(crName, namespace, publishedIP, publishedHostname string) string {
	if publishedHostname != "" {
		return publishedHostname
	}
	if publishedIP != "" {
		return fmt.Sprintf("%s.%s.sslip.io",
			autoHostnameLabel(crName, namespace), strings.ReplaceAll(publishedIP, ".", "-"))
	}
	return ""
}

// autoHostnameLabel composes the platform-unique leading DNS label
// "{crName}-{namespace}", truncated to the 63-character DNS label limit
// (both parts are valid labels already, so the join only ever violates
// length, never charset).
func autoHostnameLabel(crName, namespace string) string {
	label := fmt.Sprintf("%s-%s", crName, namespace)
	if len(label) > 63 {
		label = strings.Trim(label[:63], "-")
	}
	return label
}

// DerivedHostnameAnnotation records, on the platform's own Ingress, the
// hostname the operator auto-derived from the controller's published
// address. Derivation happens ONCE: the identity server's realm bakes the
// advertised URL into its OIDC clients at first import, so a hostname that
// silently changed on a later reconcile (a re-published load-balancer
// address, a changed derivation) would break every sign-in while the status
// still looked healthy. The annotation makes the first derivation sticky;
// it lives on the Ingress (owner-referenced) so an uninstall clears it and
// a fresh install derives fresh. Setting spec.ingress.hostname always wins
// over it.
const DerivedHostnameAnnotation = "planton.ai/derived-hostname"

// PublicURL renders the browser-facing URL for a hostname.
func PublicURL(hostname string, tls bool) string {
	if hostname == "" {
		return ""
	}
	scheme := schemeHTTP
	if tls {
		scheme = schemeHTTPS
	}
	return fmt.Sprintf("%s://%s", scheme, hostname)
}

// Ingress builds the single Ingress serving Planton on one hostname, one
// rule per entry of the front-door route table (front_door_routes.go): API,
// storage relay, identity server, console. Same-origin by construction, so
// no CORS surface exists; every rule is pathType Prefix, so the object means
// the same thing on every Ingress controller.
func Ingress(cfg IngressConfig) *networkingv1.Ingress {
	labels := map[string]string{
		"app.kubernetes.io/name":       "ingress",
		"app.kubernetes.io/instance":   cfg.CRName,
		"app.kubernetes.io/managed-by": ManagedByLabel,
		"app.kubernetes.io/component":  "networking",
	}

	pathTypePrefix := networkingv1.PathTypePrefix
	routes := FrontDoorRoutes()
	paths := make([]networkingv1.HTTPIngressPath, 0, len(routes))
	for _, route := range routes {
		if route.HeaderMatched() {
			// Ingress has no portable header match (only controller-specific
			// annotations), so the native-gRPC row is not offered on this
			// door; native clients port-forward to the control plane's raw
			// gRPC port instead. Rendering the row path-only would be wrong,
			// not merely incomplete: it would send every console page to the
			// gRPC port.
			continue
		}
		paths = append(paths, networkingv1.HTTPIngressPath{
			Path:     route.PathPrefix,
			PathType: &pathTypePrefix,
			Backend: networkingv1.IngressBackend{
				Service: &networkingv1.IngressServiceBackend{
					Name: route.ServiceName(cfg.CRName),
					Port: networkingv1.ServiceBackendPort{Name: route.ServicePortName()},
				},
			},
		})
	}
	rule := networkingv1.IngressRuleValue{
		HTTP: &networkingv1.HTTPIngressRuleValue{Paths: paths},
	}

	ing := &networkingv1.Ingress{
		TypeMeta: metav1.TypeMeta{APIVersion: "networking.k8s.io/v1", Kind: "Ingress"},
		ObjectMeta: metav1.ObjectMeta{
			Name:        IngressName(cfg.CRName),
			Namespace:   cfg.Namespace,
			Labels:      labels,
			Annotations: ingressAnnotations(cfg),
		},
		Spec: networkingv1.IngressSpec{
			Rules: []networkingv1.IngressRule{{
				Host:             cfg.Hostname,
				IngressRuleValue: rule,
			}},
		},
	}

	if cfg.IngressClassName != "" {
		className := cfg.IngressClassName
		ing.Spec.IngressClassName = &className
	}

	if secretName := effectiveTLSSecretName(cfg); secretName != "" {
		ing.Spec.TLS = []networkingv1.IngressTLS{{
			Hosts:      []string{cfg.Hostname},
			SecretName: secretName,
		}}
	}

	if cfg.OwnerRef != nil {
		ing.OwnerReferences = []metav1.OwnerReference{*cfg.OwnerRef}
	}

	return ing
}

// effectiveTLSSecretName resolves which certificate Secret the Ingress
// references: the user's own Secret, or the derived name cert-manager fills.
func effectiveTLSSecretName(cfg IngressConfig) string {
	if cfg.TLSSecretName != "" {
		return cfg.TLSSecretName
	}
	if cfg.CertManagerIssuerName != "" {
		return IngressTLSSecretName(cfg.CRName)
	}
	return ""
}

// ingressAnnotations merges the operator's controller-specific defaults with
// the user's passthrough annotations; the user always wins on conflicts.
func ingressAnnotations(cfg IngressConfig) map[string]string {
	annotations := map[string]string{}

	if cfg.NginxController {
		// gRPC-Web server-streaming (deploy progress, log tails) is a
		// long-lived chunked response; nginx's default response buffering
		// and 60s read timeout would stall and sever it.
		annotations["nginx.ingress.kubernetes.io/proxy-buffering"] = "off"
		annotations["nginx.ingress.kubernetes.io/proxy-read-timeout"] = "3600"
		annotations["nginx.ingress.kubernetes.io/proxy-send-timeout"] = "3600"
		// nginx defaults the request-body cap to 1m, which would 413 any
		// browser payload over a megabyte -- large manifest applies on the
		// API path, state-file uploads on the storage path -- while both
		// backend servers accept 100m. Raise the edge to the servers' own
		// limit; the servers stay the authority (the storage relay
		// additionally enforces its 50MB transfer cap itself).
		annotations["nginx.ingress.kubernetes.io/proxy-body-size"] = "100m"
	}

	if cfg.CertManagerIssuerName != "" {
		annotation := certManagerIssuerAnnotation
		if cfg.CertManagerIssuerKind == "ClusterIssuer" {
			annotation = certManagerClusterIssuerAnnotation
		}
		annotations[annotation] = cfg.CertManagerIssuerName
	}

	maps.Copy(annotations, cfg.Annotations)

	// Operator STATE, not a default: recorded after the user-annotation copy
	// so a passthrough annotation can never overwrite the sticky derivation.
	if cfg.HostnameDerived && cfg.Hostname != "" {
		annotations[DerivedHostnameAnnotation] = cfg.Hostname
	}

	if len(annotations) == 0 {
		return nil
	}
	return annotations
}
