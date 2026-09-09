package component

import (
	"fmt"
	"strings"

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

// FrontDoorPosture is everything the control plane's internet-facing posture
// derives from, computed ONCE from the front door and read by every consumer
// (the keyless issuer and its availability, GitHub webhook reachability, the
// resolved reachability in status). One derivation, many readers: a posture
// that read the door on its own would be a second opinion about the same
// door, and two opinions is how a card advertises what the API refuses.
type FrontDoorPosture struct {
	// URL is the door's advertised origin -- the OIDC issuer a token names and
	// the address discovery is fetched from.
	URL string
	// HTTPS is whether the door is served over TLS. The clouds accept only an
	// HTTPS issuer, so a plain-HTTP door closes keyless even when public.
	HTTPS bool
	// Reachability is the resolved word (never "auto"): what the operator
	// concluded about whether the public internet reaches the door.
	Reachability v1.IngressReachability
	// DeclaredPrivate is whether the person wrote "private" on the ingress
	// block, as opposed to the door resolving private by its shape. Kept so
	// the closed-door sentence names the fact the person can act on: a word
	// they typed, or a certificate they can add.
	DeclaredPrivate bool
	// VaultEnabled is whether the platform vault runs. Keyless tokens are
	// signed by its Transit engine; without it there is no key to publish.
	VaultEnabled bool
}

// Public reports whether the public internet reaches the door.
func (p FrontDoorPosture) Public() bool { return p.Reachability == v1.IngressReachabilityPublic }

// KeylessOffered reports whether this install can be a keyless identity
// issuer the clouds will trust: the door is reachable from the internet, over
// HTTPS, and the vault that signs the tokens is running. Each false arm has
// its own sentence in KeylessClosedReason.
func (p FrontDoorPosture) KeylessOffered() bool {
	return p.Public() && p.HTTPS && p.VaultEnabled
}

// KeylessClosedReason is the sentence the connection wizard shows on the
// keyless card when KeylessOffered is false: the ONE fact that closed the
// door, in plain words, and the way out. One root cause per sentence, chosen
// in the order a person meets them: no front door at all (a port-forward),
// the word they wrote (private), the door's scheme (a certificate), the vault
// (a component). Empty when offered. The declared-private sentence is also
// declared, byte for byte, by the control plane's methods-by-deployment
// fixture, so a rewording here is a fixture diff there.
func (p FrontDoorPosture) KeylessClosedReason() string {
	const wayOut = " Use the runner or access-key method instead."
	const need = "Keyless connections need cloud providers to fetch this install's signing keys from its front door over the public internet, "
	switch {
	case p.KeylessOffered():
		return ""
	case !isPublicURL(p.URL):
		return need + "and this install's front door is a port-forward on your own machine." + wayOut
	case p.DeclaredPrivate:
		return need + "and the front door is declared private." + wayOut
	case !p.HTTPS:
		return need + "and the front door serves plain HTTP; cloud providers accept only an HTTPS issuer. Add a certificate to the front door, or use the runner or access-key method instead."
	default:
		return "Keyless connections are signed by the platform vault, which this install runs without. Enable the vault, or use the runner or access-key method instead."
	}
}

// resolveReachability turns the declaration on the ingress block into the
// resolved word for a door at the given URL. A declared word stands; "auto"
// (or no declaration) is public exactly when the door has a hostname served
// over HTTPS -- the shape of every real install -- and private otherwise: a
// plain-HTTP door, a port-forward. A pure function of (spec, url) so the
// component that publishes the URL and the one that derives posture from it
// cannot disagree.
func resolveReachability(ingress *v1.IngressSpec, url string) v1.IngressReachability {
	if ingress != nil && ingress.Enabled {
		switch ingress.Reachability {
		case v1.IngressReachabilityPublic, v1.IngressReachabilityPrivate:
			return ingress.Reachability
		}
	}
	if isPublicURL(url) && strings.HasPrefix(url, "https://") {
		return v1.IngressReachabilityPublic
	}
	return v1.IngressReachabilityPrivate
}

// isPublicURL reports whether the URL names a host other than the
// port-forward door's localhost -- the "has a hostname" half of the auto rule.
func isPublicURL(url string) bool {
	return url != "" && !strings.HasPrefix(url, "http://localhost:") && url != "http://localhost"
}

// frontDoorPosture derives the posture from the platform's front door.
// resolved is false in the same window frontDoorURL's is.
func frontDoorPosture(planton *v1.PlantonPlatform) (posture FrontDoorPosture, resolved bool) {
	url, ok := frontDoorURL(planton)
	if !ok || url == "" {
		return FrontDoorPosture{}, false
	}
	ingress := planton.Spec.Ingress
	return FrontDoorPosture{
		URL:             url,
		HTTPS:           strings.HasPrefix(url, "https://"),
		Reachability:    resolveReachability(ingress, url),
		DeclaredPrivate: ingress != nil && ingress.Enabled && ingress.Reachability == v1.IngressReachabilityPrivate,
		VaultEnabled:    isVaultEnabled(planton),
	}, true
}

// publishFrontDoor records the door's advertised URL and the reachability the
// operator concluded for it, together, in status. The three front-door owners
// (the Ingress edge, the Gateway API edge, the port-forward gateway) call this
// instead of writing status.consoleUrl themselves so the two facts can never
// be published apart: a person who left reachability at "auto" reads the
// answer beside the address it is an answer about.
func publishFrontDoor(planton *v1.PlantonPlatform, url string) {
	planton.Status.ConsoleURL = url
	if url == "" {
		planton.Status.Reachability = ""
		return
	}
	planton.Status.Reachability = resolveReachability(planton.Spec.Ingress, url)
}

// frontDoorRoutesNativeGRPC reports whether the platform's front door renders
// the route table's header-matched native-gRPC row. Only the Gateway API edge
// can (an HTTPRoute header match is core and portable); the Ingress edge and
// the port-forward gateway skip that row, so nothing may advertise a native
// gRPC address for them.
func frontDoorRoutesNativeGRPC(planton *v1.PlantonPlatform) bool {
	return isIngressEnabled(planton) && planton.Spec.Ingress.GatewayRef != nil
}

// remoteRunnersEnabled reports whether the install asked for runners outside
// the cluster to pull its deploy work (spec.remoteRunners.enabled). Off by
// default: opening the deploy queue to other networks is the operator's
// deliberate act.
func remoteRunnersEnabled(planton *v1.PlantonPlatform) bool {
	return planton.Spec.RemoteRunners != nil &&
		planton.Spec.RemoteRunners.Enabled != nil && *planton.Spec.RemoteRunners.Enabled
}

// remoteRunnersCarried reports whether the remote-runners capability is
// actually served: asked for AND on a front door that carries native gRPC
// (the deploy queue speaks nothing else). The two callers that act on it --
// the queue route on the door, the addresses advertised by the control plane
// -- read this one fact so they can never disagree; an install that asked on
// a door that cannot carry it is told so in the ingress component's status
// (remoteRunnersClosedReason).
func remoteRunnersCarried(planton *v1.PlantonPlatform) bool {
	return remoteRunnersEnabled(planton) && frontDoorRoutesNativeGRPC(planton)
}

// remoteRunnersClosedReason is the sentence the ingress status carries when
// remote runners were asked for but this door cannot carry them; empty when
// they are carried or were not asked for.
func remoteRunnersClosedReason(planton *v1.PlantonPlatform) string {
	if !remoteRunnersEnabled(planton) || remoteRunnersCarried(planton) {
		return ""
	}
	return "Remote runners are not served through this front door: the deploy queue speaks native gRPC, which only a Gateway API front door carries. Attach the platform to a Gateway (ingress.gatewayRef) to open it, or leave remoteRunners off."
}

// gatewayLocalPort returns the workstation port sign-in URLs are pinned to in
// gateway mode (spec.gateway.localPort, or the default).
func gatewayLocalPort(planton *v1.PlantonPlatform) int32 {
	if planton.Spec.Gateway != nil && planton.Spec.Gateway.LocalPort != nil {
		return *planton.Spec.Gateway.LocalPort
	}
	return resources.GatewayDefaultLocalPort
}
