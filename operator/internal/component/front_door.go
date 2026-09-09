package component

import (
	"fmt"

	v1 "github.com/plantonhq/planton/operator/api/v1"
	"github.com/plantonhq/planton/operator/internal/resources"
)

// frontDoorURL resolves the browser-facing URL of the platform's ONE front
// door. Every URL-bearing surface derives from it -- Keycloak's advertised
// issuer and redirect URIs, the console's API endpoint and auth callbacks,
// status.consoleUrl -- so the two access modes stay the same architecture at
// different addresses:
//
//   - ingress enabled:  the public URL (explicit hostname, or the
//     auto-derived hostname once the ingress component resolves it)
//   - ingress disabled: the deterministic localhost URL the gateway
//     component serves over kubectl port-forward
//
// resolved is false only in the auto-hostname window where the ingress URL is
// not yet known; the gateway URL is always known.
func frontDoorURL(planton *v1.PlantonPlatform) (url string, resolved bool) {
	if !isIngressEnabled(planton) {
		return fmt.Sprintf("http://localhost:%d", gatewayLocalPort(planton)), true
	}
	spec := planton.Spec.Ingress
	if spec.Hostname != "" && spec.GatewayRef == nil {
		// Explicit hostname on the Ingress edge: the URL is known statically
		// (the tls block decides the scheme) -- render the public endpoints
		// immediately, independent of ingress health.
		return resources.PublicURL(spec.Hostname, spec.TLS != nil), true
	}
	// Otherwise the URL is resolved by the Ingress component, which
	// reconciles earlier in the same pass and publishes it in status: an
	// auto-derived hostname on either edge, or -- on the Gateway API edge --
	// the scheme, which follows the listener the route attaches to rather
	// than the tls block.
	if planton.Status.ConsoleURL != "" {
		return planton.Status.ConsoleURL, true
	}
	return "", false
}

// frontDoorRoutesNativeGRPC reports whether the platform's front door renders
// the route table's header-matched native-gRPC row. Only the Gateway API edge
// can (an HTTPRoute header match is core and portable); the Ingress edge and
// the port-forward gateway skip that row, so nothing may advertise a native
// gRPC address for them.
func frontDoorRoutesNativeGRPC(planton *v1.PlantonPlatform) bool {
	return isIngressEnabled(planton) && planton.Spec.Ingress.GatewayRef != nil
}

// gatewayLocalPort returns the workstation port sign-in URLs are pinned to in
// gateway mode (spec.gateway.localPort, or the default).
func gatewayLocalPort(planton *v1.PlantonPlatform) int32 {
	if planton.Spec.Gateway != nil && planton.Spec.Gateway.LocalPort != nil {
		return *planton.Spec.Gateway.LocalPort
	}
	return resources.GatewayDefaultLocalPort
}
