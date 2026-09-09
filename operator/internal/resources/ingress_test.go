package resources

import (
	"strings"
	"testing"

	networkingv1 "k8s.io/api/networking/v1"
)

const (
	testIngressHostname = "planton.example.com"
	// Mirror the builder outputs under test.
	controlPlaneSvcName = "planton-control-plane"
	nginxClassName      = "nginx"
)

func baseIngressConfig() IngressConfig {
	return IngressConfig{CRName: "planton", Namespace: "default"}
}

func TestIngress_RoutesAPIStorageIdentityAndConsole(t *testing.T) {
	ing := Ingress(baseIngressConfig())

	paths := ing.Spec.Rules[0].HTTP.Paths
	// Four of the table's five rows: the header-matched native-gRPC row has
	// no portable Ingress form and is skipped, never flattened to a path.
	if len(paths) != 4 {
		t.Fatalf("expected 4 paths, got %d", len(paths))
	}
	for _, p := range paths {
		if p.Backend.Service.Port.Name == controlPlaneGrpcPortName {
			t.Errorf("path %s routes to the raw gRPC port; the Ingress door cannot express the content-type rule", p.Path)
		}
	}

	api := paths[0]
	if api.Path != APIPathPrefix {
		t.Errorf("API path = %s, want %s", api.Path, APIPathPrefix)
	}
	// Every rule is a portable segment-prefix rule: the API has its own path
	// namespace precisely so no controller-specific matching is needed.
	for _, p := range paths {
		if *p.PathType != networkingv1.PathTypePrefix {
			t.Errorf("path %s pathType = %s, want Prefix", p.Path, *p.PathType)
		}
	}
	if api.Backend.Service.Name != controlPlaneSvcName {
		t.Errorf("API backend = %s, want %s", api.Backend.Service.Name, controlPlaneSvcName)
	}
	// The browser speaks gRPC-Web; the raw gRPC port would refuse it.
	if api.Backend.Service.Port.Name != "grpc-web" {
		t.Errorf("API backend port = %s, want grpc-web", api.Backend.Service.Port.Name)
	}

	// The storage relay is served by the control plane on the same port as
	// the API: state-file transfer URLs stay same-origin for browsers.
	storage := paths[1]
	if storage.Path != "/storage" || *storage.PathType != networkingv1.PathTypePrefix {
		t.Errorf("storage path = %s (%s), want /storage (Prefix)", storage.Path, *storage.PathType)
	}
	if storage.Backend.Service.Name != controlPlaneSvcName || storage.Backend.Service.Port.Name != "grpc-web" {
		t.Errorf("storage backend = %s:%s, want %s:grpc-web",
			storage.Backend.Service.Name, storage.Backend.Service.Port.Name, controlPlaneSvcName)
	}

	// Identity on the same hostname: same-origin auth, one certificate. NOT
	// "/auth" -- the console owns /auth/* routes.
	identity := paths[2]
	if identity.Path != "/idp" || *identity.PathType != networkingv1.PathTypePrefix {
		t.Errorf("identity path = %s (%s), want /idp (Prefix)", identity.Path, *identity.PathType)
	}
	if identity.Backend.Service.Name != "planton-identity" || identity.Backend.Service.Port.Name != portNameHTTP {
		t.Errorf("identity backend = %s:%s, want planton-identity:http",
			identity.Backend.Service.Name, identity.Backend.Service.Port.Name)
	}

	console := paths[3]
	if console.Path != "/" || *console.PathType != networkingv1.PathTypePrefix {
		t.Errorf("console path = %s (%s), want / (Prefix)", console.Path, *console.PathType)
	}
	if console.Backend.Service.Name != "planton-console" || console.Backend.Service.Port.Name != portNameHTTP {
		t.Errorf("console backend = %s:%s, want planton-console:http",
			console.Backend.Service.Name, console.Backend.Service.Port.Name)
	}
}

func TestIngress_HostScoping(t *testing.T) {
	tests := []struct {
		name     string
		hostname string
	}{
		// Host-less matches any host: the transient state while an
		// auto-derived hostname is resolved from the controller's address.
		{"host-less while auto-resolving", ""},
		{"pinned to hostname", testIngressHostname},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := baseIngressConfig()
			cfg.Hostname = tt.hostname
			ing := Ingress(cfg)
			if got := ing.Spec.Rules[0].Host; got != tt.hostname {
				t.Errorf("host = %q, want %q", got, tt.hostname)
			}
		})
	}
}

func TestIngress_ClassName(t *testing.T) {
	cfg := baseIngressConfig()
	ing := Ingress(cfg)
	if ing.Spec.IngressClassName != nil {
		t.Errorf("expected nil ingressClassName (cluster default), got %s", *ing.Spec.IngressClassName)
	}

	cfg.IngressClassName = nginxClassName
	ing = Ingress(cfg)
	if ing.Spec.IngressClassName == nil || *ing.Spec.IngressClassName != nginxClassName {
		t.Error("expected ingressClassName nginx")
	}
}

func TestIngress_TLSVariants(t *testing.T) {
	tests := []struct {
		name           string
		mutate         func(*IngressConfig)
		wantSecret     string
		wantAnnotation string
		wantAnnValue   string
	}{
		{
			name:       "no TLS -> plain HTTP",
			mutate:     func(_ *IngressConfig) {},
			wantSecret: "",
		},
		{
			name: "BYO secret",
			mutate: func(c *IngressConfig) {
				c.Hostname = testIngressHostname
				c.TLSSecretName = "corp-cert"
			},
			wantSecret: "corp-cert",
		},
		{
			name: "cert-manager Issuer",
			mutate: func(c *IngressConfig) {
				c.Hostname = testIngressHostname
				c.CertManagerIssuerName = "corp-issuer"
				c.CertManagerIssuerKind = "Issuer"
			},
			wantSecret:     "planton-ingress-tls",
			wantAnnotation: "cert-manager.io/issuer",
			wantAnnValue:   "corp-issuer",
		},
		{
			name: "cert-manager ClusterIssuer",
			mutate: func(c *IngressConfig) {
				c.Hostname = testIngressHostname
				c.CertManagerIssuerName = "lets-encrypt"
				c.CertManagerIssuerKind = "ClusterIssuer"
			},
			wantSecret:     "planton-ingress-tls",
			wantAnnotation: "cert-manager.io/cluster-issuer",
			wantAnnValue:   "lets-encrypt",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := baseIngressConfig()
			tt.mutate(&cfg)
			ing := Ingress(cfg)

			if tt.wantSecret == "" {
				if len(ing.Spec.TLS) != 0 {
					t.Fatalf("expected no TLS block, got %v", ing.Spec.TLS)
				}
				return
			}
			if len(ing.Spec.TLS) != 1 || ing.Spec.TLS[0].SecretName != tt.wantSecret {
				t.Fatalf("TLS = %v, want secret %s", ing.Spec.TLS, tt.wantSecret)
			}
			if ing.Spec.TLS[0].Hosts[0] != cfg.Hostname {
				t.Errorf("TLS host = %s, want %s", ing.Spec.TLS[0].Hosts[0], cfg.Hostname)
			}
			if tt.wantAnnotation != "" {
				if got := ing.Annotations[tt.wantAnnotation]; got != tt.wantAnnValue {
					t.Errorf("annotation %s = %q, want %q", tt.wantAnnotation, got, tt.wantAnnValue)
				}
			}
		})
	}
}

func TestIngress_NginxAnnotationsOnlyWhenDetected(t *testing.T) {
	cfg := baseIngressConfig()
	ing := Ingress(cfg)
	if _, ok := ing.Annotations["nginx.ingress.kubernetes.io/proxy-buffering"]; ok {
		t.Error("nginx annotations must not be applied for non-nginx controllers")
	}

	cfg.NginxController = true
	ing = Ingress(cfg)
	if ing.Annotations["nginx.ingress.kubernetes.io/proxy-buffering"] != "off" {
		t.Error("expected nginx proxy-buffering off for gRPC-Web streaming")
	}
	if ing.Annotations["nginx.ingress.kubernetes.io/proxy-read-timeout"] != "3600" {
		t.Error("expected long nginx read timeout for gRPC-Web streaming")
	}
	// Routing never depends on nginx: the annotations above are tuning only.
	if _, ok := ing.Annotations["nginx.ingress.kubernetes.io/use-regex"]; ok {
		t.Error("use-regex must not be set; every rule is a plain Prefix rule")
	}
	// nginx's default 1m body cap would 413 large manifest applies and
	// state-file uploads while both backend servers accept 100m.
	if ing.Annotations["nginx.ingress.kubernetes.io/proxy-body-size"] != "100m" {
		t.Error("expected proxy-body-size raised to the servers' own limit")
	}
}

func TestIngress_UserAnnotationsWin(t *testing.T) {
	cfg := baseIngressConfig()
	cfg.NginxController = true
	cfg.Annotations = map[string]string{
		"nginx.ingress.kubernetes.io/proxy-read-timeout": "60",
		"example.com/custom":                             "value",
	}
	ing := Ingress(cfg)

	if got := ing.Annotations["nginx.ingress.kubernetes.io/proxy-read-timeout"]; got != "60" {
		t.Errorf("user annotation overridden: got %s, want 60", got)
	}
	if ing.Annotations["example.com/custom"] != "value" {
		t.Error("expected user passthrough annotation")
	}
}

func TestAutoHostname(t *testing.T) {
	tests := []struct {
		name              string
		crName, namespace string
		ip, publishedHost string
		want              string
	}{
		{"published DNS name used directly", "planton", "planton", "", "lb-1234.elb.example.com", "lb-1234.elb.example.com"},
		{"DNS name wins over IP", "planton", "planton", "203.0.113.7", "lb-1234.elb.example.com", "lb-1234.elb.example.com"},
		{"IP becomes magic DNS", "planton", "planton", "203.0.113.7", "", "planton-planton.203-0-113-7.sslip.io"},
		{"nothing published yet", "planton", "planton", "", "", ""},
		// Platforms share one ingress controller (one published address), so
		// the label must differ per platform or both derive the same URL and
		// the controller routes the loser's traffic to the winner.
		{"second platform derives a distinct hostname", "planton", "team-b", "203.0.113.7", "",
			"planton-team-b.203-0-113-7.sslip.io"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := AutoHostname(tt.crName, tt.namespace, tt.ip, tt.publishedHost); got != tt.want {
				t.Errorf("AutoHostname(%q, %q, %q, %q) = %q, want %q",
					tt.crName, tt.namespace, tt.ip, tt.publishedHost, got, tt.want)
			}
		})
	}
}

// The leading label must stay a valid DNS label (<= 63 chars) even for long
// platform/namespace names -- truncated, never rejected.
func TestAutoHostnameLabelLength(t *testing.T) {
	long := strings.Repeat("a", 50)
	got := AutoHostname(long, long, "203.0.113.7", "")
	label := strings.SplitN(got, ".", 2)[0]
	if len(label) > 63 {
		t.Errorf("leading label is %d chars, must be <= 63: %q", len(label), label)
	}
	if strings.HasSuffix(label, "-") {
		t.Errorf("truncation must not leave a trailing hyphen: %q", label)
	}
}

// Derivation is once-only (the identity realm bakes the URL in at first
// import), so a derived hostname must be recorded on the Ingress for later
// reconciles to reuse -- and a user passthrough annotation must never be
// able to overwrite that record.
func TestIngressRecordsDerivedHostname(t *testing.T) {
	cfg := IngressConfig{
		CRName:          "planton",
		Namespace:       "planton",
		Hostname:        "planton-planton.203-0-113-7.sslip.io",
		HostnameDerived: true,
		Annotations:     map[string]string{DerivedHostnameAnnotation: "attacker.example.com"},
	}
	ing := Ingress(cfg)
	if got := ing.Annotations[DerivedHostnameAnnotation]; got != cfg.Hostname {
		t.Errorf("derived-hostname annotation = %q, want %q", got, cfg.Hostname)
	}

	declared := IngressConfig{CRName: "planton", Namespace: "planton", Hostname: "planton.example.com"}
	if _, present := Ingress(declared).Annotations[DerivedHostnameAnnotation]; present {
		t.Error("a spec-declared hostname must not be recorded as derived")
	}
}

func TestPublicURL(t *testing.T) {
	if got := PublicURL(testIngressHostname, false); got != "http://"+testIngressHostname {
		t.Errorf("http URL = %s", got)
	}
	if got := PublicURL(testIngressHostname, true); got != "https://"+testIngressHostname {
		t.Errorf("https URL = %s", got)
	}
	if got := PublicURL("", true); got != "" {
		t.Errorf("empty hostname must yield empty URL, got %s", got)
	}
}
