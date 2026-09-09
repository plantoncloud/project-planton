package resources

import (
	"net"
	"net/url"
)

// The front-door route table: the ONE statement of how a request that reaches
// a Planton platform at its public origin is routed to a component. Every
// front door renders from it -- the Ingress object, the Gateway API HTTPRoute,
// and the built-in nginx gateway that serves over kubectl port-forward -- so
// the three doors are the same architecture at different addresses, and a
// route added here appears on all of them in one change.
//
// Every rule is a plain path prefix with the segment-wise meaning Kubernetes
// defines as portable (Ingress pathType Prefix, Gateway API PathPrefix). That
// is why the browser API has a path namespace of its own (APIPathPrefix): a
// raw gRPC request path is a single segment, /ai.planton.<Service>/<Method>,
// which no portable prefix rule can tell from a console page -- it took a
// controller-specific regex to route, and a controller that does not know the
// regex sends every API call to the console. The control plane serves its
// gRPC-Web door under the same prefix, and the console is handed a base URL
// that ends with it, so no client composes the shape.
//
// Native gRPC clients (the CLI, the runner, any grpc-go or grpc-java program)
// cannot use a path namespace: their request path IS the single segment, fixed
// by the protocol. What they do carry, always, is the header the protocol
// requires -- "content-type: application/grpc" -- and an exact header match is
// a core, portable Gateway API rule. So one rule matches the root prefix
// together with that header and delivers to the control plane's raw gRPC port.
// The Gateway API's precedence order (longer path prefix first, then the rule
// with more header matches) keeps the browser API's and identity's prefixes
// ahead of it and puts it ahead of the console's catch-all. The framed-gRPC
// path through the gRPC-Web door is deliberately not offered: that door exists
// to translate for browsers, and a native client behind it loses its trailers
// on the way back.
//
// The Ingress object and the nginx gateway have no portable header match, so
// they render the path-only rules and skip this one; on those doors a native
// client reaches the control plane through a port-forward to its raw gRPC
// port, and the README says so.
//
// The control plane's webhook port is the fourth backend: the keyless issuer's
// two discovery documents at the root the specification pins them to, and the
// inbound webhooks under a namespace of their own (see WebhooksPathPrefix). It
// is routed on every door, the port-forward one included, so the discovery
// document a developer fetches at http://localhost:8080 is honest about the
// door it is served at.
//
// Order is most-specific first. Renderers that match by longest prefix (the
// Ingress, the Gateway API) do not depend on it; nginx does not either
// (longest-prefix location wins), but the rendered config reads in the same
// order as this table.

const (
	// APIPathPrefix is the path namespace of the browser-facing gRPC-Web API.
	// Mirrors the control plane's GrpcWebServerConfig.API_PATH_PREFIX -- the
	// two cannot import each other, and the boot-contract floor is where a
	// change to one is reconciled with the other.
	APIPathPrefix = "/rpc"

	// StoragePathPrefix routes the storage relay: expiring transfer URLs
	// (state-file download/upload) served by the control plane on its
	// browser-API port. The path shape is the control plane's
	// BlobRelayService.RELAY_PATH_PREFIX contract ("/storage/v1/relay/...");
	// routing the "/storage" root keeps room for future storage surfaces
	// without another edge change.
	StoragePathPrefix = "/storage"

	// OIDCDiscoveryPath and OIDCJWKSPath are the keyless identity issuer's two
	// documents, pinned to the issuer's root by the OpenID Connect
	// specification: the front door IS the issuer, so they cannot live under
	// a namespace. Routed as the two exact paths (each a segment-wise prefix
	// of exactly itself), never the whole "/.well-known": that namespace is
	// shared with cert-manager's ACME HTTP-01 challenges and with any
	// well-known file the console may one day serve, and a rule that claimed
	// it whole would send those to a port that answers only these two.
	OIDCDiscoveryPath = "/.well-known/openid-configuration"
	OIDCJWKSPath      = "/.well-known/jwks.json"

	// WebhooksPathPrefix is the path namespace of the control plane's inbound
	// webhooks. Mirrors the control plane's WebhookPathNamespaceFilter.NAMESPACE
	// -- the two cannot import each other, and the boot-contract floor is
	// where a change to one is reconciled with the other. A namespace for the
	// same reason the browser API has one: at the root, the GitHub receiver
	// ("/github") and the console's GitHub App setup page ("/github/app/setup")
	// are one prefix, and a portable rule cannot tell them apart. GitHub is
	// handed the full receiver URL, so the namespace costs the provider
	// nothing.
	WebhooksPathPrefix = "/webhooks"

	// ConsolePathPrefix is the catch-all: everything no other rule claims is
	// a web console page.
	ConsolePathPrefix = "/"

	// GRPCContentTypeHeader is the request header every gRPC client sets; its
	// value is what tells a native gRPC call apart from a console page at the
	// same path depth.
	GRPCContentTypeHeader = "content-type"

	// DeploymentKindSelfHosted is the deployment shape every install of this
	// operator declares to the platform's components (the control plane's
	// entitlement semantics, the console's device discovery document): the
	// platform's own vocabulary, "a fact, never inferred".
	DeploymentKindSelfHosted = "self_hosted"

	// The URL schemes a front door serves; the default ports GRPCEndpoint
	// falls back to follow them.
	schemeHTTP  = "http"
	schemeHTTPS = "https"
)

// GRPCContentTypes are the exact content-type values native gRPC clients send
// for protobuf messages. grpc-go and grpc-java send the bare form; the
// protocol also permits the explicit subtype. gRPC-Web forms
// ("application/grpc-web+proto") are absent on purpose: they belong to the
// browser API's own path namespace.
var GRPCContentTypes = []string{"application/grpc", "application/grpc+proto"}

// FrontDoorBackend names the component a route delivers to.
type FrontDoorBackend int

const (
	// BackendControlPlane is the control plane's browser-API port (gRPC-Web
	// plus the storage relay).
	BackendControlPlane FrontDoorBackend = iota
	// BackendControlPlaneGRPC is the control plane's raw gRPC port, the door
	// for native gRPC clients (CLI, runner).
	BackendControlPlaneGRPC
	// BackendControlPlaneWebhook is the control plane's public unauthenticated
	// HTTP surface: the keyless issuer's discovery documents and the
	// signature-verified webhook receivers. Every request here proves itself
	// (a signature, or nothing to protect); no session, no bearer.
	BackendControlPlaneWebhook
	// BackendIdentity is the identity server (sign-in pages, OIDC endpoints).
	BackendIdentity
	// BackendConsole is the web console.
	BackendConsole
	// BackendTemporal is the deploy queue's frontend (native gRPC), routed
	// through the front door ONLY when the install opens remote runners: a
	// laptop or an appliance in another network polls its owner's deploy
	// work here. Exactly one service is carried -- the workflow service
	// runners use -- so the queue's administrative service never leaves the
	// cluster (see RemoteRunnerRoutes).
	BackendTemporal
)

// TemporalWorkflowServicePath is the request-path prefix of the deploy
// queue's workflow service: a gRPC path is "/<package>.<Service>/<Method>",
// and a segment-wise PathPrefix on the service segment matches every method
// of that one service and nothing else. It is the single Temporal surface a
// runner needs (task polling, completions, heartbeats, the SDK's
// connect-time GetSystemInfo). The operator service -- namespace
// registration, search attributes, cluster administration -- is deliberately
// absent: opening the queue to runners never means opening its controls.
const TemporalWorkflowServicePath = "/temporal.api.workflowservice.v1.WorkflowService"

// ServesStreams reports whether responses through this backend may be
// long-lived (server streams such as deploy progress and log tails, or the
// deploy queue's long polls, which hold a request open for about a minute
// waiting for work), which a front door's default request timeout would
// sever. Both control-plane doors and the queue do.
func (b FrontDoorBackend) ServesStreams() bool {
	return b == BackendControlPlane || b == BackendControlPlaneGRPC || b == BackendTemporal
}

// FrontDoorHeaderMatch narrows a rule to requests whose named header carries
// one of the given values exactly (the rule is the OR of the values).
type FrontDoorHeaderMatch struct {
	Name   string
	Values []string
}

// FrontDoorRoute is one rule of the table.
type FrontDoorRoute struct {
	// PathPrefix is matched segment-wise against the request path.
	PathPrefix string
	// Header, when set, additionally requires an exact header value. Only
	// the Gateway API door renders header-matched rules; the Ingress and
	// nginx doors skip them (see the package comment).
	Header  *FrontDoorHeaderMatch
	Backend FrontDoorBackend
}

// HeaderMatched reports whether the rule needs a header match to be
// expressed, which only some front doors can do.
func (r FrontDoorRoute) HeaderMatched() bool { return r.Header != nil }

// FrontDoorRoutes returns the table, most-specific rule first.
func FrontDoorRoutes() []FrontDoorRoute {
	return []FrontDoorRoute{
		{PathPrefix: APIPathPrefix, Backend: BackendControlPlane},
		{PathPrefix: StoragePathPrefix, Backend: BackendControlPlane},
		{PathPrefix: IdentityPathPrefix, Backend: BackendIdentity},
		{PathPrefix: OIDCDiscoveryPath, Backend: BackendControlPlaneWebhook},
		{PathPrefix: OIDCJWKSPath, Backend: BackendControlPlaneWebhook},
		{PathPrefix: WebhooksPathPrefix, Backend: BackendControlPlaneWebhook},
		{
			PathPrefix: ConsolePathPrefix,
			Header:     &FrontDoorHeaderMatch{Name: GRPCContentTypeHeader, Values: GRPCContentTypes},
			Backend:    BackendControlPlaneGRPC,
		},
		{PathPrefix: ConsolePathPrefix, Backend: BackendConsole},
	}
}

// RemoteRunnerRoutes returns the rules the remote-runners capability adds to
// the front door, most-specific first: the deploy queue's workflow service,
// delivered to the queue frontend. A service-segment prefix outranks the
// content-type-matched gRPC root rule by path length (the Gateway API's
// precedence order), so native gRPC for the queue lands on the queue and
// every other native gRPC call still lands on the control plane. Rendered by
// the Gateway API door only: it is the one door that carries native gRPC at
// all, which is why the capability requires it.
func RemoteRunnerRoutes() []FrontDoorRoute {
	return []FrontDoorRoute{
		{PathPrefix: TemporalWorkflowServicePath, Backend: BackendTemporal},
	}
}

// ServiceName is the Kubernetes Service the route's backend is reached at.
func (r FrontDoorRoute) ServiceName(crName string) string {
	switch r.Backend {
	case BackendIdentity:
		return IdentityServiceName(crName)
	case BackendConsole:
		return ConsoleServiceName(crName)
	case BackendTemporal:
		return TemporalFrontendServiceName(crName)
	default:
		return ControlPlaneServiceName(crName)
	}
}

// ServicePortName is the named Service port the route targets; Ingress and
// HTTPRoute backends reference ports by name so a port number change never
// touches the doors.
func (r FrontDoorRoute) ServicePortName() string {
	switch r.Backend {
	case BackendControlPlane:
		return controlPlaneGrpcWebPortName
	case BackendControlPlaneGRPC:
		return controlPlaneGrpcPortName
	case BackendControlPlaneWebhook:
		return controlPlaneWebhookPortName
	case BackendTemporal:
		return temporalFrontendGRPCPortName
	default:
		return "http"
	}
}

// ServicePort is the numeric Service port, for the doors that reference
// ports by number (the Gateway API) or dial upstreams by address (nginx).
func (r FrontDoorRoute) ServicePort() int {
	switch r.Backend {
	case BackendIdentity:
		return identityServicePort
	case BackendConsole:
		return consoleServicePort
	case BackendControlPlaneGRPC:
		return controlPlaneServicePort
	case BackendControlPlaneWebhook:
		return controlPlaneWebhookPort
	case BackendTemporal:
		return TemporalFrontendGRPCPort
	default:
		return controlPlaneGrpcWebPort
	}
}

// GithubWebhookReceiverURL renders where GitHub delivers to a platform whose
// front door is the given URL: the webhook namespace plus the GitHub receiver's
// own path. The path after the namespace is the control plane's controller
// mapping (POST /github), the same one hosted's dedicated webhooks hostname
// routes at the root.
func GithubWebhookReceiverURL(frontDoorURL string) string {
	return frontDoorURL + WebhooksPathPrefix + "/github"
}

// APIURL renders the base URL a browser client is given for the API: the
// front-door origin plus the API path namespace. Clients append
// "/<service>/<method>" to it.
func APIURL(frontDoorURL string) string {
	return frontDoorURL + APIPathPrefix
}

// GRPCEndpoint renders the host:port a native gRPC client dials for a platform
// whose front door is the given public URL: the same host, on the URL's port
// or the scheme's default, because the door routes gRPC by content type on the
// same listener that serves the console. It is what the console publishes to
// device clients in its discovery document. Empty when the URL does not parse
// -- a client is better off deriving nothing than dialing a wrong address.
func GRPCEndpoint(publicURL string) string {
	u, err := url.Parse(publicURL)
	if err != nil || u.Hostname() == "" {
		return ""
	}
	port := u.Port()
	if port == "" {
		port = "80"
		if u.Scheme == schemeHTTPS {
			port = "443"
		}
	}
	return net.JoinHostPort(u.Hostname(), port)
}
