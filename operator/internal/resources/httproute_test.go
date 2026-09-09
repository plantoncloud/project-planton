package resources

import (
	"testing"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

// Listener hostname admission follows the Gateway API's rule: empty admits
// all, a wildcard admits exactly one more label, otherwise exact.
func TestListenerAdmitsHostname(t *testing.T) {
	cases := []struct {
		listener, hostname string
		want               bool
	}{
		{"", "planton.example.com", true},
		{"planton.example.com", "planton.example.com", true},
		{"other.example.com", "planton.example.com", false},
		{"*.example.com", "planton.example.com", true},
		{"*.example.com", "a.b.example.com", false}, // one label only
		{"*.example.com", "example.com", false},     // the suffix alone is not a match
		{"*.example.com", "planton.example.org", false},
		{"*.example.com", ".example.com", false},
	}
	for _, c := range cases {
		if got := ListenerAdmitsHostname(c.listener, c.hostname); got != c.want {
			t.Errorf("ListenerAdmitsHostname(%q, %q) = %v, want %v", c.listener, c.hostname, got, c.want)
		}
	}
}

// The HTTPRoute is the route table, rule for rule, with numeric backend
// ports and a disabled request timeout only where server streams flow.
func TestHTTPRouteRendersTheRouteTable(t *testing.T) {
	route := HTTPRoute(HTTPRouteConfig{
		CRName: "planton", Namespace: "planton", Hostname: "planton.example.com",
		HostnameDerived: true, GatewayName: "main", GatewayNamespace: "gw", SectionName: "https",
	})

	if route.GetAnnotations()[DerivedHostnameAnnotation] != "planton.example.com" {
		t.Error("a derived hostname must be recorded on the route")
	}
	parents, _, _ := unstructured.NestedSlice(route.Object, "spec", "parentRefs")
	parent := parents[0].(map[string]any)
	if parent["name"] != "main" || parent["namespace"] != "gw" || parent["sectionName"] != "https" {
		t.Errorf("parentRef = %v", parent)
	}
	rules, _, _ := unstructured.NestedSlice(route.Object, "spec", "rules")
	table := FrontDoorRoutes()
	if len(rules) != len(table) {
		t.Fatalf("%d rules for %d routes", len(rules), len(table))
	}
	for idx, raw := range rules {
		rule := raw.(map[string]any)
		matches, _, _ := unstructured.NestedSlice(rule, "matches")
		wantMatches := 1
		if table[idx].HeaderMatched() {
			wantMatches = len(table[idx].Header.Values)
		}
		if len(matches) != wantMatches {
			t.Errorf("rule %d has %d matches, want %d", idx, len(matches), wantMatches)
		}
		for _, m := range matches {
			match := m.(map[string]any)
			path, _, _ := unstructured.NestedMap(match, "path")
			if path["type"] != "PathPrefix" || path["value"] != table[idx].PathPrefix {
				t.Errorf("rule %d path = %v, want PathPrefix %s", idx, path, table[idx].PathPrefix)
			}
			headers, hasHeaders, _ := unstructured.NestedSlice(match, "headers")
			if hasHeaders != table[idx].HeaderMatched() {
				t.Errorf("rule %d header match present = %v, want %v", idx, hasHeaders, table[idx].HeaderMatched())
			}
			if hasHeaders {
				header := headers[0].(map[string]any)
				if header["type"] != "Exact" || header["name"] != table[idx].Header.Name {
					t.Errorf("rule %d header = %v, want an Exact match on %s", idx, header, table[idx].Header.Name)
				}
			}
		}
		backends, _, _ := unstructured.NestedSlice(rule, "backendRefs")
		backend := backends[0].(map[string]any)
		if backend["name"] != table[idx].ServiceName("planton") || backend["port"] != int64(table[idx].ServicePort()) {
			t.Errorf("rule %d backend = %v", idx, backend)
		}
		_, hasTimeout, _ := unstructured.NestedMap(rule, "timeouts")
		if hasTimeout != table[idx].Backend.ServesStreams() {
			t.Errorf("rule %d timeouts present = %v; only control-plane doors carry the streaming timeout", idx, hasTimeout)
		}
	}
}

// The native-gRPC row: one match per gRPC content type, each an Exact header
// match paired with the root prefix, delivered to the raw gRPC Service port.
// Precedence is the API's: the row sits after the longer prefixes (browser
// API, storage, identity) and, having a header match, ahead of the console's
// bare catch-all at the same prefix.
func TestHTTPRouteRoutesNativeGRPCByContentType(t *testing.T) {
	table := FrontDoorRoutes()
	grpcIdx := -1
	for idx, route := range table {
		if route.Backend == BackendControlPlaneGRPC {
			grpcIdx = idx
		}
	}
	if grpcIdx < 0 {
		t.Fatal("the route table has no native-gRPC row")
	}
	grpcRow := table[grpcIdx]
	if grpcRow.PathPrefix != ConsolePathPrefix || !grpcRow.HeaderMatched() || grpcRow.Header.Name != GRPCContentTypeHeader {
		t.Errorf("native-gRPC row = %+v; want the root prefix narrowed by %s", grpcRow, GRPCContentTypeHeader)
	}
	if grpcRow.ServicePortName() != controlPlaneGrpcPortName || grpcRow.ServicePort() != controlPlaneServicePort {
		t.Errorf("native-gRPC row targets %s:%d, want %s:%d", grpcRow.ServicePortName(), grpcRow.ServicePort(), controlPlaneGrpcPortName, controlPlaneServicePort)
	}
	console := table[len(table)-1]
	if console.Backend != BackendConsole || console.HeaderMatched() {
		t.Errorf("the table must end with the console's bare catch-all, got %+v", console)
	}
	if grpcIdx != len(table)-2 {
		t.Errorf("the native-gRPC row is at %d; it must sit just before the console catch-all so the table reads in precedence order", grpcIdx)
	}

	route := HTTPRoute(HTTPRouteConfig{CRName: "planton", Namespace: "planton", Hostname: "planton.example.com", GatewayName: "main"})
	rules, _, _ := unstructured.NestedSlice(route.Object, "spec", "rules")
	matches, _, _ := unstructured.NestedSlice(rules[grpcIdx].(map[string]any), "matches")
	seen := map[string]bool{}
	for _, m := range matches {
		headers, _, _ := unstructured.NestedSlice(m.(map[string]any), "headers")
		seen[headers[0].(map[string]any)["value"].(string)] = true
	}
	for _, ct := range GRPCContentTypes {
		if !seen[ct] {
			t.Errorf("content type %q is not matched by the native-gRPC rule", ct)
		}
	}
}

// The address device clients are told to dial follows the front door's URL:
// same host, the URL's port or the scheme's default.
func TestGRPCEndpoint(t *testing.T) {
	cases := map[string]string{
		"https://planton.example.com":      "planton.example.com:443",
		"http://planton.example.com":       "planton.example.com:80",
		"https://planton.example.com:8443": "planton.example.com:8443",
		"http://localhost:8080":            "localhost:8080",
		"":                                 "",
		"not a url":                        "",
	}
	for in, want := range cases {
		if got := GRPCEndpoint(in); got != want {
			t.Errorf("GRPCEndpoint(%q) = %q, want %q", in, got, want)
		}
	}
}

// The grant lives in the SECRET's namespace and names the Gateway's.
func TestTLSReferenceGrantPermitsTheGatewayNamespace(t *testing.T) {
	grant := TLSReferenceGrant("planton", "planton", "gw", nil)
	if grant.GetNamespace() != "planton" {
		t.Errorf("grant namespace = %s, want the Secret's namespace", grant.GetNamespace())
	}
	from, _, _ := unstructured.NestedSlice(grant.Object, "spec", "from")
	if from[0].(map[string]any)["namespace"] != "gw" || from[0].(map[string]any)["kind"] != "Gateway" {
		t.Errorf("from = %v", from[0])
	}
	to, _, _ := unstructured.NestedSlice(grant.Object, "spec", "to")
	if to[0].(map[string]any)["name"] != IngressTLSSecretName("planton") {
		t.Errorf("to = %v", to[0])
	}
}

// Listener parsing lifts the facts the edge reasons about, defaulting the
// certificate reference namespace to the Gateway's own.
func TestParseGatewayListeners(t *testing.T) {
	gw := &unstructured.Unstructured{Object: map[string]any{
		"metadata": map[string]any{"name": "main", "namespace": "gw"},
		"spec": map[string]any{"listeners": []any{
			map[string]any{"name": "http", "protocol": "HTTP", "port": int64(80)},
			map[string]any{
				"name": "https", "protocol": "HTTPS", "port": int64(443), "hostname": "*.example.com",
				"allowedRoutes": map[string]any{"namespaces": map[string]any{
					"from": "Selector", "selector": map[string]any{"matchLabels": map[string]any{"planton": "yes"}},
				}},
				"tls": map[string]any{"certificateRefs": []any{
					map[string]any{"name": "wild"},
					map[string]any{"name": "planton-ingress-tls", "namespace": "planton"},
				}},
			},
		}},
	}}
	listeners := ParseGatewayListeners(gw)
	if len(listeners) != 2 {
		t.Fatalf("%d listeners", len(listeners))
	}
	if listeners[0].AllowedNamespaces != "Same" {
		t.Errorf("allowedRoutes default = %s, want Same", listeners[0].AllowedNamespaces)
	}
	https := listeners[1]
	if https.AllowedNamespaces != "Selector" || https.NamespaceSelector == nil || https.NamespaceSelector.MatchLabels["planton"] != "yes" {
		t.Errorf("selector not lifted: %+v", https)
	}
	if len(https.CertificateRefs) != 2 || https.CertificateRefs[0] != "gw/wild" || https.CertificateRefs[1] != "planton/planton-ingress-tls" {
		t.Errorf("certificateRefs = %v", https.CertificateRefs)
	}
}
