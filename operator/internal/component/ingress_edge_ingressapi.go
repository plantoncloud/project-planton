package component

import (
	"context"
	"fmt"
	"sort"
	"strings"

	networkingv1 "k8s.io/api/networking/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	v1 "github.com/plantonhq/planton/operator/api/v1"
	"github.com/plantonhq/planton/operator/internal/resources"
)

// defaultIngressClassAnnotation marks the cluster's default IngressClass,
// used when spec.ingress.ingressClassName is not set.
const defaultIngressClassAnnotation = "ingressclass.kubernetes.io/is-default-class"

// reconcileIngressEdge is the Ingress-controller edge: an Ingress object with
// one Prefix rule per route-table entry, the class resolved (or the cluster
// default), TLS by a brought Secret or cert-manager's ingress-shim, and the
// auto-derived hostname read from the controller's published address.
func (i *Ingress) reconcileIngressEdge(ctx context.Context, c client.Client, planton *v1.PlantonPlatform) (Result, error) {
	log := logf.FromContext(ctx).WithValues("component", i.Name())
	spec := planton.Spec.Ingress

	nginx, preflightMsg, err := i.resolveIngressClass(ctx, c, spec.IngressClassName)
	if err != nil {
		return Result{}, err
	}
	if preflightMsg != "" {
		return Refused(preflightMsg), nil
	}

	if msg, err := i.preflightTLS(ctx, c, planton); err != nil {
		return Result{}, err
	} else if msg != "" {
		return Refused(msg), nil
	}

	cfg := resources.IngressConfig{
		CRName:           planton.Name,
		Namespace:        planton.Namespace,
		OwnerRef:         i.OwnerReferenceFor(planton),
		Hostname:         spec.Hostname,
		IngressClassName: spec.IngressClassName,
		Annotations:      spec.Annotations,
		NginxController:  nginx,
	}
	if spec.TLS != nil {
		cfg.TLSSecretName = spec.TLS.SecretName
		if spec.TLS.Issuer != nil {
			cfg.CertManagerIssuerName = spec.TLS.Issuer.Name
			cfg.CertManagerIssuerKind = spec.TLS.Issuer.Kind
		}
	}

	autoMode := spec.Hostname == ""
	if autoMode {
		// Auto-hostname: admit the Ingress host-less first so the controller
		// publishes its address, derive the hostname from it, then pin the
		// rules to that hostname -- a catch-all left behind on a shared
		// cluster would swallow other tenants' unmatched traffic. Derivation
		// is ONCE per install: the first derived hostname is recorded on the
		// Ingress and reused ever after (the identity realm bakes the URL in
		// at first import, so a hostname that drifted with a re-published
		// controller address would break sign-in silently).
		hostname, waitMsg, err := i.resolveAutoHostname(ctx, c, cfg)
		if err != nil {
			return Result{}, err
		}
		if waitMsg != "" {
			return Result{Ready: false, Message: waitMsg}, nil
		}
		cfg.Hostname = hostname
		cfg.HostnameDerived = true
	}

	if err := i.ApplyTypedObject(ctx, c, resources.Ingress(cfg)); err != nil {
		return Result{}, fmt.Errorf("applying Ingress: %w", err)
	}

	// The URL the platform advertises; the console component renders its
	// public endpoints from it in the same reconcile pass. Published even
	// while a certificate is still being issued (below): no component
	// depends on the ingress being Ready, only on the URL existing, so the
	// rest of the platform converges while the person points their DNS.
	url := resources.PublicURL(cfg.Hostname, spec.TLS != nil)
	publishFrontDoor(planton, url)

	// A cert-manager-issued certificate gates readiness: until it is
	// actually issued, the advertised HTTPS door answers with the ingress
	// controller's placeholder certificate and every browser refuses it --
	// "Ready" would be a lie. Issuance usually waits on the adopter's DNS
	// record (internet validators like Let's Encrypt must reach the
	// hostname), so the waiting message names that exact task.
	if spec.TLS != nil && spec.TLS.Issuer != nil {
		issued, waitMsg, err := i.certificateIssued(ctx, c, planton, cfg.Hostname)
		if err != nil {
			return Result{}, err
		}
		if !issued {
			return Result{Ready: false, Message: waitMsg}, nil
		}
	}

	msg := fmt.Sprintf("Console at %s", url)
	if spec.TLS == nil {
		msg += " (unencrypted HTTP; set spec.ingress.tls for HTTPS)"
	}
	if reason := remoteRunnersClosedReason(planton); reason != "" {
		msg += ". " + reason
	}
	log.Info("Ingress ready", "url", url)
	return Result{Ready: true, Message: msg}, nil
}

// certificateIssued reports whether cert-manager has issued the platform's
// certificate. The Certificate object is located by its spec.secretName --
// ingress-shim creates it targeting exactly the Secret the Ingress
// references (resources.IngressTLSSecretName) -- so the match is robust to
// whatever name ingress-shim gives the object itself.
func (i *Ingress) certificateIssued(ctx context.Context, c client.Client, planton *v1.PlantonPlatform, hostname string) (bool, string, error) {
	certs := &unstructured.UnstructuredList{}
	certs.SetGroupVersionKind(schema.GroupVersionKind{
		Group: "cert-manager.io", Version: "v1", Kind: "CertificateList",
	})
	if err := c.List(ctx, certs, client.InNamespace(planton.Namespace)); err != nil {
		return false, "", fmt.Errorf("listing cert-manager Certificates: %w", err)
	}

	wantSecret := resources.IngressTLSSecretName(planton.Name)
	for idx := range certs.Items {
		secretName, _, _ := unstructured.NestedString(certs.Items[idx].Object, "spec", "secretName")
		if secretName != wantSecret {
			continue
		}
		if ready, reason := certificateReadyCondition(&certs.Items[idx]); !ready {
			return false, certificateWaitingMessage(hostname, reason, i.publishedAddress(ctx, c, planton)), nil
		}
		return true, "", nil
	}
	// ingress-shim has not created the Certificate yet -- normal for a few
	// seconds after the Ingress first applies; persisting means cert-manager
	// is not acting on the annotation.
	return false, fmt.Sprintf(
		"waiting for cert-manager to pick up the certificate request for %s; if this persists, check that cert-manager is running and the issuer in spec.ingress.tls.issuer exists",
		hostname), nil
}

// certificateReadyCondition lifts the Certificate's Ready condition and, when
// not ready, cert-manager's own message -- the issuer knows WHY (a pending
// DNS challenge, a missing CA secret, a rejected order) far better than any
// wording we could invent, so it is relayed verbatim.
func certificateReadyCondition(cert *unstructured.Unstructured) (ready bool, message string) {
	conditions, _, _ := unstructured.NestedSlice(cert.Object, "status", "conditions")
	for _, raw := range conditions {
		condition, ok := raw.(map[string]any)
		if !ok || condition["type"] != "Ready" {
			continue
		}
		msg, _ := condition["message"].(string)
		return condition["status"] == "True", msg
	}
	return false, ""
}

// certificateWaitingMessage names the wait AND the way out. The published
// address is used opportunistically, never required: datacenter ingress
// controllers (RKE2's node-served nginx) legitimately publish nothing, and
// the person's hostname may point at a front-end load balancer the cluster
// knows nothing about.
func certificateWaitingMessage(hostname, certManagerReason, publishedAddress string) string {
	msg := fmt.Sprintf("waiting for the certificate for %s", hostname)
	if certManagerReason != "" {
		msg += ": " + certManagerReason
	}
	if publishedAddress != "" {
		msg += fmt.Sprintf(". If your DNS record is not created yet, point %s at %s", hostname, publishedAddress)
	} else {
		msg += fmt.Sprintf(". If your DNS record is not created yet, point %s at the address your ingress controller serves on", hostname)
	}
	msg += " -- issuers that validate over the internet can only succeed once the hostname reaches this cluster"
	return msg
}

// publishedAddress reads the ingress controller's published address(es) for
// the waiting message. Best-effort: an unreadable or address-less Ingress
// yields "", and the message degrades to generic guidance.
func (i *Ingress) publishedAddress(ctx context.Context, c client.Client, planton *v1.PlantonPlatform) string {
	var applied networkingv1.Ingress
	if err := c.Get(ctx, types.NamespacedName{
		Name: resources.IngressName(planton.Name), Namespace: planton.Namespace,
	}, &applied); err != nil {
		return ""
	}
	var addresses []string
	for _, lb := range applied.Status.LoadBalancer.Ingress {
		if lb.Hostname != "" {
			addresses = append(addresses, lb.Hostname)
		}
		if lb.IP != "" {
			addresses = append(addresses, lb.IP)
		}
	}
	return strings.Join(addresses, ", ")
}

// resolveIngressClass validates the requested (or default) IngressClass and
// reports whether the serving controller is ingress-nginx. A non-empty
// message means a plain-language preflight failure the user can act on.
func (i *Ingress) resolveIngressClass(ctx context.Context, c client.Client, className string) (nginx bool, message string, err error) {
	var classes networkingv1.IngressClassList
	if err := c.List(ctx, &classes); err != nil {
		return false, "", fmt.Errorf("listing IngressClasses: %w", err)
	}

	if className != "" {
		for idx := range classes.Items {
			if classes.Items[idx].Name == className {
				return classes.Items[idx].Spec.Controller == resources.NginxIngressController, "", nil
			}
		}
		return false, fmt.Sprintf(
			"IngressClass %q not found; available: %s",
			className, availableClassNames(classes.Items)), nil
	}

	for idx := range classes.Items {
		if classes.Items[idx].Annotations[defaultIngressClassAnnotation] == "true" {
			return classes.Items[idx].Spec.Controller == resources.NginxIngressController, "", nil
		}
	}
	return false, fmt.Sprintf(
		"no default IngressClass in this cluster; set spec.ingress.ingressClassName (available: %s)",
		availableClassNames(classes.Items)), nil
}

// resolveAutoHostname derives the public hostname from the ingress
// controller's published address. A hostname derived by a PRIOR reconcile
// (recorded on the Ingress under DerivedHostnameAnnotation) is reused as-is
// -- derivation is once-only, because the identity realm bakes the
// advertised URL in at first import. Otherwise it applies the Ingress
// host-less so the controller admits it and publishes an address, then
// reads it back. A non-empty message means the address is not (yet)
// available.
func (i *Ingress) resolveAutoHostname(ctx context.Context, c client.Client, cfg resources.IngressConfig) (hostname, message string, err error) {
	var existing networkingv1.Ingress
	getErr := c.Get(ctx, types.NamespacedName{
		Name: resources.IngressName(cfg.CRName), Namespace: cfg.Namespace,
	}, &existing)
	if getErr == nil {
		if sticky := existing.Annotations[resources.DerivedHostnameAnnotation]; sticky != "" {
			return sticky, "", nil
		}
	} else if !apierrors.IsNotFound(getErr) {
		return "", "", fmt.Errorf("reading Ingress: %w", getErr)
	}

	hostless := cfg
	hostless.Hostname = ""
	hostless.HostnameDerived = false
	if err := i.ApplyTypedObject(ctx, c, resources.Ingress(hostless)); err != nil {
		return "", "", fmt.Errorf("applying Ingress: %w", err)
	}

	var applied networkingv1.Ingress
	if err := c.Get(ctx, types.NamespacedName{
		Name: resources.IngressName(cfg.CRName), Namespace: cfg.Namespace,
	}, &applied); err != nil {
		return "", "", fmt.Errorf("reading Ingress status: %w", err)
	}

	for _, lb := range applied.Status.LoadBalancer.Ingress {
		if derived := resources.AutoHostname(cfg.CRName, cfg.Namespace, lb.IP, lb.Hostname); derived != "" {
			return derived, "", nil
		}
	}
	return "", "waiting for the ingress controller to publish an address to derive a hostname from; if it never does, set spec.ingress.hostname", nil
}

func availableClassNames(classes []networkingv1.IngressClass) string {
	if len(classes) == 0 {
		return "none -- no ingress controller appears to be installed"
	}
	names := make([]string, 0, len(classes))
	for idx := range classes {
		names = append(names, classes[idx].Name)
	}
	sort.Strings(names)
	return strings.Join(names, ", ")
}
