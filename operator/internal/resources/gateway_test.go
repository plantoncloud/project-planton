package resources

import (
	"strings"
	"testing"
)

// The gateway's nginx config must mirror the ingress path layout exactly --
// the two front-door modes are the same architecture at different addresses.
func TestGatewayNginxConfig_MirrorsIngressLayout(t *testing.T) {
	config := GatewayNginxConfig("planton", "planton-ns")

	// The API routes by the plain path namespace the control plane serves
	// its gRPC-Web door under -- the same prefix rule every front door
	// carries (front_door_routes.go); no regex, no controller-specific syntax.
	if !strings.Contains(config, "location "+APIPathPrefix+" {") {
		t.Errorf("the API location must be the plain %s prefix:\n%s", APIPathPrefix, config)
	}
	if strings.Contains(config, "~*") || strings.Contains(config, "ai.planton") {
		t.Error("no regex location and no gRPC package name may remain in the gateway config")
	}
	if !strings.Contains(config, "http://planton-control-plane.planton-ns.svc.cluster.local:8081") {
		t.Errorf("the API path must route to the control plane's gRPC-Web port:\n%s", config)
	}
	if !strings.Contains(config, "location /idp") {
		t.Error("the identity path must be routed")
	}
	if !strings.Contains(config, "http://planton-identity.planton-ns.svc.cluster.local:80") {
		t.Error("the identity path must route to the identity Service")
	}
	if !strings.Contains(config, "http://planton-console.planton-ns.svc.cluster.local:80") {
		t.Error("the catch-all must route to the console Service")
	}
	// The port-forward door is an honest issuer too: the discovery document a
	// developer fetches at localhost names the door it is served at, and the
	// webhook namespace reaches the same port.
	for _, loc := range []string{OIDCDiscoveryPath, OIDCJWKSPath} {
		if !strings.Contains(config, "location = "+loc+" {") {
			t.Errorf("the %s document must be an exact location (one document, not a namespace):\n%s", loc, config)
		}
	}
	if !strings.Contains(config, "location "+WebhooksPathPrefix+" {") {
		t.Errorf("the %s namespace must be a prefix location:\n%s", WebhooksPathPrefix, config)
	}
	if !strings.Contains(config, "set $controlplane_webhook_upstream http://planton-control-plane.planton-ns.svc.cluster.local:8086;") ||
		!strings.Contains(config, "proxy_pass $controlplane_webhook_upstream") {
		t.Errorf("the webhook port needs its own request-time upstream:\n%s", config)
	}
	// The header-matched native-gRPC row is skipped on this door: exactly one
	// root location (the console), and no upstream on the raw gRPC port.
	if n := strings.Count(config, "location / {"); n != 1 {
		t.Errorf("root locations = %d, want exactly one (the console catch-all)", n)
	}
	if strings.Contains(config, "planton-control-plane.planton-ns.svc.cluster.local:80;") {
		t.Error("the port-forward door must not proxy to the raw gRPC port; native clients port-forward it directly")
	}

	// Request-time upstream resolution (variables + resolver): the gateway
	// deploys BEFORE its backends exist, so literal proxy_pass hosts would
	// crash-loop nginx at startup ("host not found in upstream" -- observed
	// live). Variables defer DNS to request time; missing backends become an
	// honest 502 instead.
	if !strings.Contains(config, "resolver ${NGINX_LOCAL_RESOLVERS}") {
		t.Error("the config must use the entrypoint-injected cluster resolver")
	}
	if !strings.Contains(config, "proxy_pass $controlplane_upstream") ||
		!strings.Contains(config, "proxy_pass $identity_upstream") ||
		!strings.Contains(config, "proxy_pass $console_upstream") {
		t.Error("proxy_pass must use variables so upstream DNS resolves at request time, not startup")
	}

	// A browser session is bigger than nginx's defaults: the console's
	// sign-in callback answers with a session cookie larger than the 8k
	// response-header buffer (a 502 "upstream sent too big header" and the
	// CLI's browser sign-in never completes), and the browser sends it back
	// on every request.
	if !strings.Contains(config, "proxy_buffer_size 32k;") {
		t.Error("the identity and console routes must raise proxy_buffer_size so the sign-in callback's session cookie fits")
	}
	// nginx checks the three buffer directives against each other at parse
	// time; a raised proxy_buffer_size alone refuses to start (observed live).
	if !strings.Contains(config, "proxy_buffers 4 32k;") || !strings.Contains(config, "proxy_busy_buffers_size 64k;") {
		t.Error("proxy_buffers and proxy_busy_buffers_size must be set consistently with proxy_buffer_size or nginx refuses to start")
	}
	if !strings.Contains(config, "large_client_header_buffers 4 32k;") {
		t.Error("the server must accept the session cookie the browser sends back (large_client_header_buffers)")
	}

	// Streaming: gRPC-Web server-streaming is a long-lived chunked response;
	// buffering or a short read timeout would stall and sever it.
	if !strings.Contains(config, "proxy_buffering off") {
		t.Error("proxy buffering must be off for gRPC-Web streaming")
	}
	if !strings.Contains(config, "proxy_read_timeout 3600s") {
		t.Error("the read timeout must accommodate long-lived streams")
	}

	// The identity server derives URLs from forwarded headers
	// (KC_PROXY_HEADERS=xforwarded): the browser's localhost origin must
	// survive the hop.
	if !strings.Contains(config, "proxy_set_header X-Forwarded-Host $http_host") {
		t.Error("the identity path must forward the original host")
	}

	// The storage relay (expiring state-file transfer URLs) is served by the
	// control plane on the API port and must be routed like the Ingress does.
	if !strings.Contains(config, "location "+StoragePathPrefix+" {") {
		t.Error("the storage relay path must be routed to the control plane")
	}
}

// nginx defaults client_max_body_size to 1m, which would 413 any browser
// payload over a megabyte while both backend servers accept 100m -- large
// manifest applies on the API path, state-file uploads on the storage path.
// The cap must be raised wherever bodies flow to the control plane.
func TestGatewayNginxConfig_RaisesBodySizeCapOnUploadPaths(t *testing.T) {
	config := GatewayNginxConfig("planton", "planton-ns")

	if got := strings.Count(config, "client_max_body_size 100m;"); got != 2 {
		t.Errorf("client_max_body_size 100m must be set on the API and storage locations, found %d", got)
	}
}

// The config ships as an nginx-image entrypoint TEMPLATE (that is what makes
// ${NGINX_LOCAL_RESOLVERS} substitution happen), so the mount point and key
// must match the entrypoint's contract: /etc/nginx/templates/*.template.
func TestGatewayDeployment_MountsEntrypointTemplate(t *testing.T) {
	deploy := GatewayDeployment(GatewayConfig{CRName: "planton", Namespace: "default"})

	mount := deploy.Spec.Template.Spec.Containers[0].VolumeMounts[0]
	if mount.MountPath != "/etc/nginx/templates" {
		t.Errorf("mount path = %q, want the nginx entrypoint's templates dir", mount.MountPath)
	}
	if !strings.HasSuffix(GatewayConfigKey, ".template") {
		t.Errorf("config key = %q, want a .template suffix (the entrypoint only substitutes templates)", GatewayConfigKey)
	}

	// The resolver env is exported only behind this opt-in; without it the
	// resolver directive survives unsubstituted and nginx crash-loops.
	env := deploy.Spec.Template.Spec.Containers[0].Env
	found := false
	for _, e := range env {
		if e.Name == "NGINX_ENTRYPOINT_LOCAL_RESOLVERS" && e.Value == "true" {
			found = true
		}
	}
	if !found {
		t.Error("NGINX_ENTRYPOINT_LOCAL_RESOLVERS=true must be set for resolver substitution")
	}
}

func TestGatewayDeployment_ConfigHashRollsThePod(t *testing.T) {
	cfg := GatewayConfig{CRName: "planton", Namespace: "default", ConfigHash: "abc123"}
	deploy := GatewayDeployment(cfg)

	if got := deploy.Spec.Template.Annotations["planton.ai/gateway-config-hash"]; got != "abc123" {
		t.Errorf("config hash annotation = %q, want abc123 (nginx only reads config at startup)", got)
	}
}

func TestGatewayDeployment_ImageDefaultsAndOverride(t *testing.T) {
	deploy := GatewayDeployment(GatewayConfig{CRName: "planton", Namespace: "default"})
	if got := deploy.Spec.Template.Spec.Containers[0].Image; got != "nginx:1.27-alpine" {
		t.Errorf("image = %q, want the pinned nginx default", got)
	}

	deploy = GatewayDeployment(GatewayConfig{
		CRName: "planton", Namespace: "default",
		ImageRepository: "mirror.example.com/nginx", ImageTag: "1.27",
	})
	if got := deploy.Spec.Template.Spec.Containers[0].Image; got != "mirror.example.com/nginx:1.27" {
		t.Errorf("image = %q, want the override (air-gapped registries)", got)
	}
}

// The port-forward command is user-facing UX: it must target the gateway
// Service and map the exact local port sign-in URLs are pinned to.
func TestGatewayPortForwardCommand(t *testing.T) {
	got := GatewayPortForwardCommand("planton", "planton-ns", 9000)
	want := "kubectl port-forward -n planton-ns svc/planton-gateway 9000:80"
	if got != want {
		t.Errorf("command = %q, want %q", got, want)
	}
}

func TestGatewayService_TargetsNginx(t *testing.T) {
	svc := GatewayService("planton", "default", nil)
	if svc.Spec.Ports[0].Port != 80 {
		t.Errorf("service port = %d, want 80", svc.Spec.Ports[0].Port)
	}
	if svc.Spec.Ports[0].TargetPort.IntValue() != 8080 {
		t.Errorf("target port = %d, want the nginx container port", svc.Spec.Ports[0].TargetPort.IntValue())
	}
}
