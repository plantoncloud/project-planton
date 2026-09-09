package component

import (
	"context"
	"fmt"
	"slices"
	"sort"
	"strings"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	v1 "github.com/plantonhq/planton/operator/api/v1"
	"github.com/plantonhq/planton/operator/internal/resources"
)

// The Gateway API edge of the front door. The Gateway is the cluster team's
// object: its listeners decide which hostnames it serves, which namespaces
// may attach routes, and how HTTPS terminates -- the operator never modifies
// it. It reads those facts, attaches one HTTPRoute for the platform's
// hostname, and turns every mismatch into a sentence that names the fix.
// Readiness is the Gateway controller's own verdict on the route (Accepted,
// ResolvedRefs), relayed verbatim: the controller knows WHY a route is not
// served far better than any wording invented here.

var (
	gatewayGVK   = schema.GroupVersionKind{Group: resources.GatewayAPIGroup, Version: resources.GatewayAPIVersion, Kind: "Gateway"}
	httpRouteGVK = schema.GroupVersionKind{Group: resources.GatewayAPIGroup, Version: resources.GatewayAPIVersion, Kind: "HTTPRoute"}
)

// gatewayEdgeFacts is what one reconcile pass learned about the Gateway the
// platform attaches to, resolved once and threaded through the pass.
type gatewayEdgeFacts struct {
	gateway   *unstructured.Unstructured
	namespace string // the Gateway's namespace (defaults to the platform's)
	// admitting are the listeners the route may attach to for the hostname
	// (protocol HTTP/HTTPS, hostname admitted, namespace allowed, section
	// name honored).
	admitting []resources.GatewayListener
}

// https reports whether any admitting listener terminates TLS: the URL the
// platform advertises follows the listener, not the tls block.
func (f gatewayEdgeFacts) https() bool {
	for _, l := range f.admitting {
		if l.Protocol == "HTTPS" {
			return true
		}
	}
	return false
}

func (i *Ingress) reconcileGatewayEdge(ctx context.Context, c client.Client, planton *v1.PlantonPlatform) (Result, error) {
	log := logf.FromContext(ctx).WithValues("component", i.Name(), "edge", "gateway-api")
	spec := planton.Spec.Ingress
	ref := spec.GatewayRef

	installed, err := i.IsCRDInstalled(ctx, c, resources.GatewayCRDName)
	if err != nil {
		return Result{}, fmt.Errorf("checking for the Gateway API: %w", err)
	}
	if !installed {
		return Result{Ready: false, Message: "the Gateway API is not installed in this cluster (Gateway CRD missing); " +
			"install the Gateway API CRDs and a Gateway controller (Istio, Envoy Gateway, Cilium, ...), " +
			"or expose the platform through an Ingress controller with spec.ingress.ingressClassName"}, nil
	}

	if msg, err := i.preflightTLS(ctx, c, planton); err != nil {
		return Result{}, err
	} else if msg != "" {
		return Result{Ready: false, Message: msg}, nil
	}

	gatewayNamespace := ref.Namespace
	if gatewayNamespace == "" {
		gatewayNamespace = planton.Namespace
	}
	gateway := &unstructured.Unstructured{}
	gateway.SetGroupVersionKind(gatewayGVK)
	if err := c.Get(ctx, types.NamespacedName{Name: ref.Name, Namespace: gatewayNamespace}, gateway); err != nil {
		if apierrors.IsNotFound(err) {
			available, listErr := i.availableGateways(ctx, c)
			if listErr != nil {
				return Result{}, listErr
			}
			return Result{Ready: false, Message: fmt.Sprintf(
				"Gateway %s/%s not found; available Gateways: %s. Set spec.ingress.gatewayRef to one of them (name and namespace)",
				gatewayNamespace, ref.Name, available)}, nil
		}
		return Result{}, fmt.Errorf("reading Gateway %s/%s: %w", gatewayNamespace, ref.Name, err)
	}

	// Hostname first: the listener match depends on it. Derivation reads
	// the Gateway's own published address -- unlike the Ingress edge there
	// is no host-less first apply, because the address belongs to the
	// Gateway, not to the route.
	hostname := spec.Hostname
	derived := false
	if hostname == "" {
		sticky, err := i.stickyDerivedHostname(ctx, c, planton)
		if err != nil {
			return Result{}, err
		}
		if sticky != "" {
			hostname = sticky
		} else {
			hostname = deriveHostnameFromGateway(planton.Name, planton.Namespace, gateway)
			if hostname == "" {
				return Result{Ready: false, Message: fmt.Sprintf(
					"waiting for Gateway %s/%s to publish an address to derive a hostname from; if it never does, set spec.ingress.hostname",
					gatewayNamespace, ref.Name)}, nil
			}
		}
		derived = true
	}

	facts, msg, err := i.matchListeners(ctx, c, planton, gateway, gatewayNamespace, hostname)
	if err != nil {
		return Result{}, err
	}
	if msg != "" {
		return Result{Ready: false, Message: msg}, nil
	}

	// The certificate arm: ask cert-manager for the hostname's certificate
	// in the platform's namespace and, when the Gateway lives elsewhere,
	// grant its namespace the right to reference the Secret. The listener
	// itself must name the Secret -- that edit is the cluster team's, and
	// the readiness message below spells it out.
	if spec.TLS != nil && spec.TLS.Issuer != nil {
		objs := []*unstructured.Unstructured{
			resources.Certificate(planton.Name, planton.Namespace, hostname,
				spec.TLS.Issuer.Name, spec.TLS.Issuer.Kind, i.OwnerReferenceFor(planton)),
		}
		if gatewayNamespace != planton.Namespace {
			objs = append(objs, resources.TLSReferenceGrant(planton.Name, planton.Namespace, gatewayNamespace, i.OwnerReferenceFor(planton)))
		}
		if err := i.ApplyManifests(ctx, c, planton, objs); err != nil {
			return Result{}, err
		}
	}

	route := resources.HTTPRoute(resources.HTTPRouteConfig{
		CRName:           planton.Name,
		Namespace:        planton.Namespace,
		OwnerRef:         i.OwnerReferenceFor(planton),
		Hostname:         hostname,
		HostnameDerived:  derived,
		GatewayName:      ref.Name,
		GatewayNamespace: gatewayNamespace,
		SectionName:      ref.SectionName,
	})
	if err := i.ApplyManifests(ctx, c, planton, []*unstructured.Unstructured{route}); err != nil {
		return Result{}, err
	}

	// The URL is published as soon as the listener facts are known -- the
	// rest of the platform converges while the route is being accepted.
	url := resources.PublicURL(hostname, facts.https())
	publishFrontDoor(planton, url)

	if spec.TLS != nil && spec.TLS.Issuer != nil {
		if msg, err := i.certificateServedByListener(ctx, c, planton, hostname, facts); err != nil {
			return Result{}, err
		} else if msg != "" {
			return Result{Ready: false, Message: msg}, nil
		}
	}

	accepted, waitMsg, err := i.routeAccepted(ctx, c, planton, gatewayNamespace, ref.Name)
	if err != nil {
		return Result{}, err
	}
	if !accepted {
		return Result{Ready: false, Message: waitMsg}, nil
	}

	msg = fmt.Sprintf("Console at %s via Gateway %s/%s", url, gatewayNamespace, ref.Name)
	if !facts.https() {
		msg += " (unencrypted HTTP; attach to an HTTPS listener for HTTPS)"
	}
	log.Info("Gateway route ready", "url", url)
	return Result{Ready: true, Message: msg}, nil
}

// matchListeners resolves which of the Gateway's listeners admit the
// platform's route for the hostname. A non-empty message is a preflight
// failure that names what to change and where.
func (i *Ingress) matchListeners(ctx context.Context, c client.Client, planton *v1.PlantonPlatform,
	gateway *unstructured.Unstructured, gatewayNamespace, hostname string) (gatewayEdgeFacts, string, error) {
	facts := gatewayEdgeFacts{gateway: gateway, namespace: gatewayNamespace}
	sectionName := planton.Spec.Ingress.GatewayRef.SectionName
	listeners := resources.ParseGatewayListeners(gateway)
	ref := fmt.Sprintf("%s/%s", gatewayNamespace, gateway.GetName())

	if len(listeners) == 0 {
		return facts, fmt.Sprintf("Gateway %s has no listeners; add an HTTP or HTTPS listener to it", ref), nil
	}

	var namespaceLabels map[string]string
	var (
		sectionMissing  = sectionName != ""
		hostnameRefused []string
		namespaceRefuse []string
	)
	for _, l := range listeners {
		if sectionName != "" {
			if l.Name != sectionName {
				continue
			}
			sectionMissing = false
		}
		if l.Protocol != "HTTP" && l.Protocol != "HTTPS" {
			continue
		}
		if !resources.ListenerAdmitsHostname(l.Hostname, hostname) {
			hostnameRefused = append(hostnameRefused, describeListener(l))
			continue
		}
		allowed, err := i.listenerAllowsNamespace(ctx, c, l, gatewayNamespace, planton.Namespace, &namespaceLabels)
		if err != nil {
			return facts, "", err
		}
		if !allowed {
			namespaceRefuse = append(namespaceRefuse, describeListener(l))
			continue
		}
		facts.admitting = append(facts.admitting, l)
	}
	if len(facts.admitting) > 0 {
		return facts, "", nil
	}

	switch {
	case sectionMissing:
		return facts, fmt.Sprintf("Gateway %s has no listener named %q (spec.ingress.gatewayRef.sectionName); its listeners are: %s",
			ref, sectionName, listenerNames(listeners)), nil
	case len(namespaceRefuse) > 0:
		return facts, fmt.Sprintf("Gateway %s admits the hostname %s but its listener(s) %s do not allow routes from namespace %q; "+
			"set the listener's allowedRoutes.namespaces to allow this namespace (from: All, or a Selector that matches it), "+
			"or run the platform in the Gateway's namespace",
			ref, hostname, strings.Join(namespaceRefuse, ", "), planton.Namespace), nil
	case len(hostnameRefused) > 0:
		return facts, fmt.Sprintf("no HTTP or HTTPS listener on Gateway %s admits the hostname %s (listeners: %s); "+
			"add a listener whose hostname covers it (an exact name, a wildcard, or no hostname), or change spec.ingress.hostname",
			ref, hostname, strings.Join(hostnameRefused, ", ")), nil
	default:
		return facts, fmt.Sprintf("Gateway %s has no HTTP or HTTPS listener (listeners: %s); add one for the platform to attach to",
			ref, listenerNames(listeners)), nil
	}
}

// listenerAllowsNamespace evaluates the listener's allowedRoutes.namespaces
// against the platform's namespace. The platform namespace's labels are read
// at most once per pass (lazily, only for Selector listeners).
func (i *Ingress) listenerAllowsNamespace(ctx context.Context, c client.Client, l resources.GatewayListener,
	gatewayNamespace, platformNamespace string, namespaceLabels *map[string]string) (bool, error) {
	switch l.AllowedNamespaces {
	case "All":
		return true, nil
	case "Selector":
		if l.NamespaceSelector == nil {
			return false, nil
		}
		if *namespaceLabels == nil {
			var ns corev1.Namespace
			if err := c.Get(ctx, types.NamespacedName{Name: platformNamespace}, &ns); err != nil {
				return false, fmt.Errorf("reading namespace %s to evaluate the listener's namespace selector: %w", platformNamespace, err)
			}
			*namespaceLabels = ns.Labels
			if *namespaceLabels == nil {
				*namespaceLabels = map[string]string{}
			}
		}
		selector, err := metav1.LabelSelectorAsSelector(l.NamespaceSelector)
		if err != nil {
			return false, nil
		}
		return selector.Matches(labels.Set(*namespaceLabels)), nil
	default: // "Same" and anything unrecognized
		return gatewayNamespace == platformNamespace, nil
	}
}

// certificateServedByListener gates readiness, on the issuer arm, on two
// facts: cert-manager has issued the certificate (its own reason relayed
// while it has not), and an HTTPS listener that admits the hostname
// references the issued Secret (the one edit only the Gateway's owner can
// make, spelled out).
func (i *Ingress) certificateServedByListener(ctx context.Context, c client.Client, planton *v1.PlantonPlatform,
	hostname string, facts gatewayEdgeFacts) (string, error) {
	cert := &unstructured.Unstructured{}
	cert.SetGroupVersionKind(schema.GroupVersionKind{Group: "cert-manager.io", Version: "v1", Kind: "Certificate"})
	if err := c.Get(ctx, types.NamespacedName{Name: resources.CertificateName(planton.Name), Namespace: planton.Namespace}, cert); err != nil {
		if apierrors.IsNotFound(err) {
			return fmt.Sprintf("waiting for cert-manager to pick up the certificate request for %s; if this persists, check that cert-manager is running and the issuer in spec.ingress.tls.issuer exists", hostname), nil
		}
		return "", fmt.Errorf("reading Certificate: %w", err)
	}
	if ready, reason := certificateReadyCondition(cert); !ready {
		return certificateWaitingMessage(hostname, reason, gatewayPublishedAddress(facts.gateway)), nil
	}

	want := fmt.Sprintf("%s/%s", planton.Namespace, resources.IngressTLSSecretName(planton.Name))
	for _, l := range facts.admitting {
		if l.Protocol != "HTTPS" {
			continue
		}
		if slices.Contains(l.CertificateRefs, want) {
			return "", nil
		}
	}
	grant := ""
	if facts.namespace != planton.Namespace {
		grant = fmt.Sprintf(" (a ReferenceGrant permitting the cross-namespace reference is in place in namespace %s)", planton.Namespace)
	}
	return fmt.Sprintf("the certificate for %s is issued as Secret %s; an HTTPS listener on Gateway %s/%s that admits the hostname must reference it in tls.certificateRefs%s -- until then the Gateway serves its own certificate for this hostname",
		hostname, want, facts.namespace, facts.gateway.GetName(), grant), nil
}

// routeAccepted lifts the Gateway controller's verdict on the platform's
// route for the named parent: Accepted and ResolvedRefs both True. Anything
// else is relayed as the controller wrote it.
func (i *Ingress) routeAccepted(ctx context.Context, c client.Client, planton *v1.PlantonPlatform, gatewayNamespace, gatewayName string) (bool, string, error) {
	route := &unstructured.Unstructured{}
	route.SetGroupVersionKind(httpRouteGVK)
	if err := c.Get(ctx, types.NamespacedName{Name: resources.HTTPRouteName(planton.Name), Namespace: planton.Namespace}, route); err != nil {
		return false, "", fmt.Errorf("reading HTTPRoute: %w", err)
	}
	parents, _, _ := unstructured.NestedSlice(route.Object, "status", "parents")
	for _, raw := range parents {
		parent, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		name, _, _ := unstructured.NestedString(parent, "parentRef", "name")
		ns, _, _ := unstructured.NestedString(parent, "parentRef", "namespace")
		if name != gatewayName || (ns != "" && ns != gatewayNamespace) {
			continue
		}
		conditions, _, _ := unstructured.NestedSlice(parent, "conditions")
		accepted, resolved := false, false
		for _, rc := range conditions {
			cond, ok := rc.(map[string]any)
			if !ok {
				continue
			}
			ctype, _ := cond["type"].(string)
			status, _ := cond["status"].(string)
			reason, _ := cond["reason"].(string)
			message, _ := cond["message"].(string)
			switch ctype {
			case "Accepted":
				if status != "True" {
					return false, fmt.Sprintf("Gateway %s/%s has not accepted the platform's route: %s", gatewayNamespace, gatewayName, relayCondition(reason, message)), nil
				}
				accepted = true
			case "ResolvedRefs":
				if status != "True" {
					return false, fmt.Sprintf("Gateway %s/%s could not resolve the platform's route: %s", gatewayNamespace, gatewayName, relayCondition(reason, message)), nil
				}
				resolved = true
			}
		}
		if accepted && resolved {
			return true, "", nil
		}
	}
	return false, fmt.Sprintf("waiting for the controller of Gateway %s/%s to accept the platform's route; if this persists, check that a Gateway controller is running and owns the Gateway's class", gatewayNamespace, gatewayName), nil
}

func relayCondition(reason, message string) string {
	switch {
	case reason != "" && message != "":
		return reason + ": " + message
	case message != "":
		return message
	case reason != "":
		return reason
	default:
		return "no reason given"
	}
}

// stickyDerivedHostname returns the hostname a PRIOR pass derived and
// recorded on the HTTPRoute, or "" -- derivation is once-only (see
// resources.DerivedHostnameAnnotation).
func (i *Ingress) stickyDerivedHostname(ctx context.Context, c client.Client, planton *v1.PlantonPlatform) (string, error) {
	route := &unstructured.Unstructured{}
	route.SetGroupVersionKind(httpRouteGVK)
	err := c.Get(ctx, types.NamespacedName{Name: resources.HTTPRouteName(planton.Name), Namespace: planton.Namespace}, route)
	if apierrors.IsNotFound(err) {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("reading HTTPRoute: %w", err)
	}
	return route.GetAnnotations()[resources.DerivedHostnameAnnotation], nil
}

// deriveHostnameFromGateway derives the magic-DNS hostname from the
// Gateway's published addresses (an IP becomes a sslip.io name; a DNS name
// is used as-is).
func deriveHostnameFromGateway(crName, namespace string, gateway *unstructured.Unstructured) string {
	addresses, _, _ := unstructured.NestedSlice(gateway.Object, "status", "addresses")
	for _, raw := range addresses {
		addr, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		value, _, _ := unstructured.NestedString(addr, "value")
		atype, _, _ := unstructured.NestedString(addr, "type")
		if value == "" {
			continue
		}
		if atype == "Hostname" {
			return resources.AutoHostname(crName, namespace, "", value)
		}
		return resources.AutoHostname(crName, namespace, value, "")
	}
	return ""
}

// gatewayPublishedAddress renders the Gateway's published addresses for the
// certificate-waiting message (best-effort, "" when none).
func gatewayPublishedAddress(gateway *unstructured.Unstructured) string {
	addresses, _, _ := unstructured.NestedSlice(gateway.Object, "status", "addresses")
	values := make([]string, 0, len(addresses))
	for _, raw := range addresses {
		addr, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		if value, _, _ := unstructured.NestedString(addr, "value"); value != "" {
			values = append(values, value)
		}
	}
	return strings.Join(values, ", ")
}

// availableGateways lists every Gateway in the cluster as "namespace/name"
// for the not-found message.
func (i *Ingress) availableGateways(ctx context.Context, c client.Client) (string, error) {
	list := &unstructured.UnstructuredList{}
	list.SetGroupVersionKind(schema.GroupVersionKind{Group: resources.GatewayAPIGroup, Version: resources.GatewayAPIVersion, Kind: "GatewayList"})
	if err := c.List(ctx, list); err != nil {
		return "", fmt.Errorf("listing Gateways: %w", err)
	}
	if len(list.Items) == 0 {
		return "none -- no Gateway exists in this cluster yet", nil
	}
	names := make([]string, 0, len(list.Items))
	for idx := range list.Items {
		names = append(names, list.Items[idx].GetNamespace()+"/"+list.Items[idx].GetName())
	}
	sort.Strings(names)
	return strings.Join(names, ", "), nil
}

func describeListener(l resources.GatewayListener) string {
	host := l.Hostname
	if host == "" {
		host = "any hostname"
	}
	return fmt.Sprintf("%s (%s %s)", l.Name, l.Protocol, host)
}

func listenerNames(listeners []resources.GatewayListener) string {
	names := make([]string, 0, len(listeners))
	for _, l := range listeners {
		names = append(names, describeListener(l))
	}
	return strings.Join(names, ", ")
}

// RBAC for the Gateway API edge: the platform's HTTPRoute and the certificate
// grant are owned; Gateways are read (the cluster team's objects, never
// written); cert-manager Certificates are written on this edge because no
// ingress-shim creates them here; the platform's Namespace is read to
// evaluate listener namespace selectors.
// +kubebuilder:rbac:groups=gateway.networking.k8s.io,resources=gateways,verbs=get;list;watch
// +kubebuilder:rbac:groups=gateway.networking.k8s.io,resources=httproutes,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=gateway.networking.k8s.io,resources=referencegrants,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=cert-manager.io,resources=certificates,verbs=get;list;watch;create;update;patch;delete
