package resources

import (
	"fmt"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

// The Gateway API edge of the front door: an HTTPRoute for the platform's
// hostname attached to a Gateway the cluster already runs, rendered from the
// same route table as the Ingress and the built-in gateway. Objects are built
// unstructured on purpose -- the operator never imports the Gateway API
// module, the same reasoning as its cert-manager touchpoints (a CRD-presence
// probe plus typed-by-hand objects, no Go dependency that would have to track
// the cluster's API version).

const (
	// GatewayAPIGroup is the API group of Gateway, HTTPRoute, and
	// ReferenceGrant.
	GatewayAPIGroup = "gateway.networking.k8s.io"
	// GatewayAPIVersion is the version the operator reads and writes for
	// Gateway and HTTPRoute (GA since Gateway API v1.0).
	GatewayAPIVersion = "v1"
	// ReferenceGrantAPIVersion is the version ReferenceGrant is served at:
	// the standard channel still ships it as v1beta1.
	ReferenceGrantAPIVersion = "v1beta1"
	// GatewayCRDName is how the Gateway API's presence is detected before an
	// HTTPRoute is written that nothing would reconcile.
	GatewayCRDName = "gateways." + GatewayAPIGroup
)

// HTTPRouteConfig bundles the inputs of the HTTPRoute builder. The builder is
// pure; listener matching, hostname derivation, and readiness live in the
// component.
type HTTPRouteConfig struct {
	CRName    string
	Namespace string
	OwnerRef  *metav1.OwnerReference

	// Hostname is the one hostname the route serves.
	Hostname string
	// HostnameDerived marks Hostname as operator-derived from the Gateway's
	// published address; the builder records it under
	// DerivedHostnameAnnotation so derivation stays once-only (see the
	// annotation's doc).
	HostnameDerived bool

	// GatewayName/GatewayNamespace/SectionName identify the parent Gateway
	// (and optionally one listener) the route attaches to.
	GatewayName      string
	GatewayNamespace string
	SectionName      string

	// RemoteRunners adds the remote-runners capability's rules (the deploy
	// queue's workflow service) to the route. Only this door renders them:
	// it is the one that carries native gRPC.
	RemoteRunners bool
}

// HTTPRouteName returns the HTTPRoute name: "{crName}-ingress" -- the same
// name the Ingress edge gives its object, because it is the same front door
// rendered for a different controller.
func HTTPRouteName(crName string) string {
	return IngressName(crName)
}

// HTTPRoute builds the platform's route: one rule per entry of the front-door
// route table (plus the remote-runners rules when that capability is on),
// each a Kubernetes-core match (PathPrefix, plus an Exact header match where
// the table asks for one; no implementation-specific matching anywhere),
// attached to the named Gateway for the one hostname.
func HTTPRoute(cfg HTTPRouteConfig) *unstructured.Unstructured {
	table := FrontDoorRoutes()
	if cfg.RemoteRunners {
		table = append(RemoteRunnerRoutes(), table...)
	}
	rules := make([]any, 0, len(table))
	for _, route := range table {
		rule := map[string]any{
			"matches": httpRouteMatches(route),
			// BackendRef ports are numeric in the Gateway API (Ingress
			// references them by name); the table knows both.
			"backendRefs": []any{map[string]any{
				"name": route.ServiceName(cfg.CRName),
				"port": int64(route.ServicePort()),
			}},
		}
		if route.Backend.ServesStreams() {
			// Server streams (deploy progress, log tails) are long-lived
			// responses on both control-plane doors, and the deploy queue's
			// long polls hold a request open for about a minute; several
			// Gateway implementations default a request timeout (Envoy
			// Gateway: 15s) that would sever them. Zero disables the timeout
			// per the API's definition. Timeouts are an Extended feature: a
			// Gateway that does not implement them says so on the route's
			// Accepted condition, which the component relays.
			rule["timeouts"] = map[string]any{"request": "0s"}
		}
		rules = append(rules, rule)
	}

	parent := map[string]any{
		"group": GatewayAPIGroup,
		"kind":  "Gateway",
		"name":  cfg.GatewayName,
	}
	if cfg.GatewayNamespace != "" {
		parent["namespace"] = cfg.GatewayNamespace
	}
	if cfg.SectionName != "" {
		parent["sectionName"] = cfg.SectionName
	}

	metadata := map[string]any{
		"name":      HTTPRouteName(cfg.CRName),
		"namespace": cfg.Namespace,
		"labels": map[string]any{
			"app.kubernetes.io/name":       "ingress",
			"app.kubernetes.io/instance":   cfg.CRName,
			"app.kubernetes.io/managed-by": ManagedByLabel,
			"app.kubernetes.io/component":  "networking",
		},
	}
	if cfg.HostnameDerived && cfg.Hostname != "" {
		metadata["annotations"] = map[string]any{DerivedHostnameAnnotation: cfg.Hostname}
	}
	if cfg.OwnerRef != nil {
		metadata["ownerReferences"] = []any{ownerReferenceMap(cfg.OwnerRef)}
	}

	route := &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": GatewayAPIGroup + "/" + GatewayAPIVersion,
		"kind":       "HTTPRoute",
		"metadata":   metadata,
		"spec": map[string]any{
			"parentRefs": []any{parent},
			"hostnames":  []any{cfg.Hostname},
			"rules":      rules,
		},
	}}
	return route
}

// httpRouteMatches renders a table row's matches. A path-only row is one
// PathPrefix match. A header-matched row is one match per accepted header
// value, each pairing the path prefix with an Exact header match: matches
// within a rule are ORed by the API, and Exact is the core (always
// implemented) header match type, so the row stays portable across Gateway
// implementations.
func httpRouteMatches(route FrontDoorRoute) []any {
	path := map[string]any{"type": "PathPrefix", "value": route.PathPrefix}
	if !route.HeaderMatched() {
		return []any{map[string]any{"path": path}}
	}
	matches := make([]any, 0, len(route.Header.Values))
	for _, value := range route.Header.Values {
		matches = append(matches, map[string]any{
			"path": path,
			"headers": []any{map[string]any{
				"type":  "Exact",
				"name":  route.Header.Name,
				"value": value,
			}},
		})
	}
	return matches
}

// ReferenceGrantName returns the name of the grant that lets the Gateway's
// namespace reference the platform's certificate Secret:
// "{crName}-ingress-tls".
func ReferenceGrantName(crName string) string {
	return IngressTLSSecretName(crName)
}

// TLSReferenceGrant builds the ReferenceGrant that permits Gateways in
// gatewayNamespace to reference the platform's issued certificate Secret. It
// lives in the SECRET's namespace (the Gateway API grants from the side being
// referenced), so it is the platform's own object and dies with it. Only
// needed when the Gateway lives in another namespace; same-namespace
// references need no grant.
func TLSReferenceGrant(crName, namespace, gatewayNamespace string, ownerRef *metav1.OwnerReference) *unstructured.Unstructured {
	metadata := map[string]any{
		"name":      ReferenceGrantName(crName),
		"namespace": namespace,
		"labels": map[string]any{
			"app.kubernetes.io/name":       "ingress",
			"app.kubernetes.io/instance":   crName,
			"app.kubernetes.io/managed-by": ManagedByLabel,
			"app.kubernetes.io/component":  "networking",
		},
	}
	if ownerRef != nil {
		metadata["ownerReferences"] = []any{ownerReferenceMap(ownerRef)}
	}
	return &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": GatewayAPIGroup + "/" + ReferenceGrantAPIVersion,
		"kind":       "ReferenceGrant",
		"metadata":   metadata,
		"spec": map[string]any{
			"from": []any{map[string]any{
				"group":     GatewayAPIGroup,
				"kind":      "Gateway",
				"namespace": gatewayNamespace,
			}},
			"to": []any{map[string]any{
				"group": "",
				"kind":  "Secret",
				"name":  IngressTLSSecretName(crName),
			}},
		},
	}}
}

// CertificateName returns the cert-manager Certificate name the Gateway edge
// creates for the hostname: "{crName}-ingress-tls" (the same as its Secret,
// which is also what ingress-shim names its Certificate on the Ingress edge).
func CertificateName(crName string) string {
	return IngressTLSSecretName(crName)
}

// Certificate builds the cert-manager Certificate for the hostname on the
// Gateway edge. On the Ingress edge cert-manager's ingress-shim creates this
// object from an annotation; a Gateway is the cluster team's object and the
// operator never annotates it, so here the operator asks for the certificate
// itself and hands the Gateway's listener the resulting Secret to reference.
func Certificate(crName, namespace, hostname, issuerName, issuerKind string, ownerRef *metav1.OwnerReference) *unstructured.Unstructured {
	if issuerKind == "" {
		issuerKind = "Issuer"
	}
	metadata := map[string]any{
		"name":      CertificateName(crName),
		"namespace": namespace,
		"labels": map[string]any{
			"app.kubernetes.io/name":       "ingress",
			"app.kubernetes.io/instance":   crName,
			"app.kubernetes.io/managed-by": ManagedByLabel,
			"app.kubernetes.io/component":  "networking",
		},
	}
	if ownerRef != nil {
		metadata["ownerReferences"] = []any{ownerReferenceMap(ownerRef)}
	}
	return &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "cert-manager.io/v1",
		"kind":       "Certificate",
		"metadata":   metadata,
		"spec": map[string]any{
			"secretName": IngressTLSSecretName(crName),
			"dnsNames":   []any{hostname},
			"issuerRef": map[string]any{
				"name":  issuerName,
				"kind":  issuerKind,
				"group": "cert-manager.io",
			},
		},
	}}
}

// ownerReferenceMap renders an OwnerReference for an unstructured object.
func ownerReferenceMap(ref *metav1.OwnerReference) map[string]any {
	m := map[string]any{
		"apiVersion": ref.APIVersion,
		"kind":       ref.Kind,
		"name":       ref.Name,
		"uid":        string(ref.UID),
	}
	if ref.Controller != nil {
		m["controller"] = *ref.Controller
	}
	if ref.BlockOwnerDeletion != nil {
		m["blockOwnerDeletion"] = *ref.BlockOwnerDeletion
	}
	return m
}

// GatewayListener is the operator's read of one listener of a Gateway: the
// facts that decide whether the platform's route may attach and how the URL
// is served.
type GatewayListener struct {
	Name     string
	Protocol string
	Hostname string
	// Port the listener serves on, for the plain-language messages.
	Port int64
	// AllowedNamespaces is the listener's allowedRoutes.namespaces.from:
	// "Same" (the default), "All", or "Selector".
	AllowedNamespaces string
	// NamespaceSelector holds the label selector when AllowedNamespaces is
	// "Selector".
	NamespaceSelector *metav1.LabelSelector
	// CertificateRefs lists "namespace/name" of every Secret the listener's
	// TLS config references (namespace defaults to the Gateway's).
	CertificateRefs []string
}

// ListenerAdmitsHostname reports whether a listener's hostname constraint
// admits the platform's hostname: an empty listener hostname admits every
// hostname; a wildcard ("*.example.com") admits one more label in front of
// its suffix; otherwise the names must match exactly.
func ListenerAdmitsHostname(listenerHostname, hostname string) bool {
	switch {
	case listenerHostname == "":
		return true
	case listenerHostname == hostname:
		return true
	case len(listenerHostname) > 1 && listenerHostname[0] == '*' && listenerHostname[1] == '.':
		suffix := listenerHostname[1:] // ".example.com"
		if len(hostname) <= len(suffix) || hostname[len(hostname)-len(suffix):] != suffix {
			return false
		}
		// Exactly one label in front of the suffix.
		label := hostname[:len(hostname)-len(suffix)]
		for _, r := range label {
			if r == '.' {
				return false
			}
		}
		return label != ""
	default:
		return false
	}
}

// ParseGatewayListeners lifts the listeners the operator cares about from an
// unstructured Gateway.
func ParseGatewayListeners(gateway *unstructured.Unstructured) []GatewayListener {
	raw, _, _ := unstructured.NestedSlice(gateway.Object, "spec", "listeners")
	gatewayNamespace := gateway.GetNamespace()
	listeners := make([]GatewayListener, 0, len(raw))
	for _, item := range raw {
		l, ok := item.(map[string]any)
		if !ok {
			continue
		}
		listener := GatewayListener{AllowedNamespaces: "Same"}
		listener.Name, _, _ = unstructured.NestedString(l, "name")
		listener.Protocol, _, _ = unstructured.NestedString(l, "protocol")
		listener.Hostname, _, _ = unstructured.NestedString(l, "hostname")
		listener.Port, _, _ = unstructured.NestedInt64(l, "port")
		if from, found, _ := unstructured.NestedString(l, "allowedRoutes", "namespaces", "from"); found && from != "" {
			listener.AllowedNamespaces = from
		}
		if sel, found, _ := unstructured.NestedMap(l, "allowedRoutes", "namespaces", "selector"); found {
			listener.NamespaceSelector = labelSelectorFromMap(sel)
		}
		refs, _, _ := unstructured.NestedSlice(l, "tls", "certificateRefs")
		for _, r := range refs {
			ref, ok := r.(map[string]any)
			if !ok {
				continue
			}
			name, _, _ := unstructured.NestedString(ref, "name")
			ns, _, _ := unstructured.NestedString(ref, "namespace")
			if ns == "" {
				ns = gatewayNamespace
			}
			listener.CertificateRefs = append(listener.CertificateRefs, fmt.Sprintf("%s/%s", ns, name))
		}
		listeners = append(listeners, listener)
	}
	return listeners
}

func labelSelectorFromMap(m map[string]any) *metav1.LabelSelector {
	sel := &metav1.LabelSelector{}
	if labels, found, _ := unstructured.NestedStringMap(m, "matchLabels"); found {
		sel.MatchLabels = labels
	}
	exprs, _, _ := unstructured.NestedSlice(m, "matchExpressions")
	for _, e := range exprs {
		em, ok := e.(map[string]any)
		if !ok {
			continue
		}
		key, _, _ := unstructured.NestedString(em, "key")
		op, _, _ := unstructured.NestedString(em, "operator")
		values, _, _ := unstructured.NestedStringSlice(em, "values")
		sel.MatchExpressions = append(sel.MatchExpressions, metav1.LabelSelectorRequirement{
			Key: key, Operator: metav1.LabelSelectorOperator(op), Values: values,
		})
	}
	return sel
}
