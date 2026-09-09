package component

import (
	"strings"
	"testing"

	v1 "github.com/plantonhq/planton/operator/api/v1"
)

// The front door is the keyless identity issuer, and every internet-facing
// posture derives from ONE resolution of its reachability. This table is that
// resolution, arm by arm: what a person declared, what the door's shape says,
// and the single sentence the keyless card shows when the door is closed.
func TestFrontDoorPosture_DerivesEveryArmFromTheDoor(t *testing.T) {
	falseV := false
	cases := []struct {
		name         string
		platform     func() *v1.PlantonPlatform
		wantURL      string
		wantReach    v1.IngressReachability
		wantKeyless  bool
		wantReasonIn string // a fragment that names the ONE cause; empty when offered
	}{
		{
			name: "auto on an https hostname resolves public and offers keyless",
			platform: func() *v1.PlantonPlatform {
				p := ingressPlatform(true)
				p.Spec.Ingress.TLS = &v1.IngressTLSSpec{SecretName: "planton-tls"}
				return p
			},
			wantURL: "https://planton.example.com", wantReach: v1.IngressReachabilityPublic, wantKeyless: true,
		},
		{
			name: "auto on a plain-http hostname resolves private and names the certificate as the way out",
			platform: func() *v1.PlantonPlatform {
				return ingressPlatform(true)
			},
			wantURL: "http://planton.example.com", wantReach: v1.IngressReachabilityPrivate, wantKeyless: false,
			wantReasonIn: "serves plain HTTP",
		},
		{
			name: "private declared on an https door closes keyless with the declaration as the cause",
			platform: func() *v1.PlantonPlatform {
				p := ingressPlatform(true)
				p.Spec.Ingress.TLS = &v1.IngressTLSSpec{SecretName: "planton-tls"}
				p.Spec.Ingress.Reachability = v1.IngressReachabilityPrivate
				return p
			},
			wantURL: "https://planton.example.com", wantReach: v1.IngressReachabilityPrivate, wantKeyless: false,
			wantReasonIn: "declared private",
		},
		{
			name: "public declared on a plain-http door is public but still not an issuer the clouds accept",
			platform: func() *v1.PlantonPlatform {
				p := ingressPlatform(true)
				p.Spec.Ingress.Reachability = v1.IngressReachabilityPublic
				return p
			},
			wantURL: "http://planton.example.com", wantReach: v1.IngressReachabilityPublic, wantKeyless: false,
			wantReasonIn: "serves plain HTTP",
		},
		{
			name: "the port-forward door is private and says so in the port-forward's words",
			platform: func() *v1.PlantonPlatform {
				return ingressPlatform(false)
			},
			wantURL: "http://localhost:8080", wantReach: v1.IngressReachabilityPrivate, wantKeyless: false,
			wantReasonIn: "port-forward on your own machine",
		},
		{
			name: "a private declaration on a disabled ingress changes nothing about the port-forward door",
			platform: func() *v1.PlantonPlatform {
				p := ingressPlatform(false)
				p.Spec.Ingress = &v1.IngressSpec{Enabled: false, Reachability: v1.IngressReachabilityPrivate}
				return p
			},
			wantURL: "http://localhost:8080", wantReach: v1.IngressReachabilityPrivate, wantKeyless: false,
			wantReasonIn: "port-forward on your own machine",
		},
		{
			name: "a public https door without the vault has no key to publish",
			platform: func() *v1.PlantonPlatform {
				p := ingressPlatform(true)
				p.Spec.Ingress.TLS = &v1.IngressTLSSpec{SecretName: "planton-tls"}
				p.Spec.Vault = &v1.OpenBAOSpec{Enabled: &falseV}
				return p
			},
			wantURL: "https://planton.example.com", wantReach: v1.IngressReachabilityPublic, wantKeyless: false,
			wantReasonIn: "platform vault",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			posture, resolved := frontDoorPosture(tc.platform())
			if !resolved {
				t.Fatal("posture must resolve whenever the URL does")
			}
			if posture.URL != tc.wantURL {
				t.Errorf("URL = %q, want %q", posture.URL, tc.wantURL)
			}
			if posture.Reachability != tc.wantReach {
				t.Errorf("reachability = %q, want %q", posture.Reachability, tc.wantReach)
			}
			if posture.KeylessOffered() != tc.wantKeyless {
				t.Errorf("keyless offered = %v, want %v", posture.KeylessOffered(), tc.wantKeyless)
			}
			reason := posture.KeylessClosedReason()
			if tc.wantKeyless && reason != "" {
				t.Errorf("an offered door carries no reason; got %q", reason)
			}
			if !tc.wantKeyless {
				if !strings.Contains(reason, tc.wantReasonIn) {
					t.Errorf("reason = %q, want it to name %q", reason, tc.wantReasonIn)
				}
				if !strings.Contains(reason, "runner or access-key method") {
					t.Errorf("reason = %q, want the way out named", reason)
				}
			}
		})
	}
}

// The declared-private sentence is declared byte for byte by the control
// plane's methods-by-deployment fixture (its self-hosted private-door arm), so
// the wizard on a real install and the reviewed fixture read the same words.
func TestFrontDoorPosture_DeclaredPrivateSentenceIsTheFixturesSentence(t *testing.T) {
	p := ingressPlatform(true)
	p.Spec.Ingress.TLS = &v1.IngressTLSSpec{SecretName: "planton-tls"}
	p.Spec.Ingress.Reachability = v1.IngressReachabilityPrivate
	posture, _ := frontDoorPosture(p)
	want := "Keyless connections need cloud providers to fetch this install's signing keys from its front door " +
		"over the public internet, and the front door is declared private. Use the runner or access-key method instead."
	if got := posture.KeylessClosedReason(); got != want {
		t.Errorf("declared-private reason drifted from the fixture's sentence:\n got %q\nwant %q", got, want)
	}
}

// The Gateway API edge learns the scheme from the listener, not from the tls
// block; the posture reads the resolved URL, so a Gateway that terminates TLS
// itself yields a public door with no tls block at all.
func TestFrontDoorPosture_GatewayEdgeReadsTheSchemeFromThePublishedURL(t *testing.T) {
	p := ingressPlatform(true)
	p.Spec.Ingress.GatewayRef = &v1.GatewayParentRef{Name: "main"}
	if _, resolved := frontDoorPosture(p); resolved {
		t.Fatal("before the Gateway edge publishes the URL the posture is unresolved")
	}
	publishFrontDoor(p, "https://planton.example.com")
	posture, resolved := frontDoorPosture(p)
	if !resolved || !posture.HTTPS || !posture.KeylessOffered() {
		t.Errorf("posture = %+v, want a resolved public https door offering keyless", posture)
	}
	if p.Status.Reachability != v1.IngressReachabilityPublic {
		t.Errorf("status.reachability = %q, want public published beside the URL", p.Status.Reachability)
	}
}

// publishFrontDoor writes the URL and its reachability together and clears
// them together, so status never shows one without the other.
func TestPublishFrontDoor_PublishesAndClearsBothFacts(t *testing.T) {
	p := ingressPlatform(true)
	p.Spec.Ingress.Reachability = v1.IngressReachabilityPrivate
	publishFrontDoor(p, "https://planton.example.com")
	if p.Status.ConsoleURL != "https://planton.example.com" || p.Status.Reachability != v1.IngressReachabilityPrivate {
		t.Errorf("status = %q / %q, want the URL with the declared word", p.Status.ConsoleURL, p.Status.Reachability)
	}
	publishFrontDoor(p, "")
	if p.Status.ConsoleURL != "" || p.Status.Reachability != "" {
		t.Errorf("status = %q / %q, want both cleared", p.Status.ConsoleURL, p.Status.Reachability)
	}
}

// buildConfig hands the renderer the derived bindings, never literals: the
// issuer is the door, the verdict follows the posture, the receiver is the door
// under the webhook namespace, and the browser console is the door itself.
func TestBuildConfig_WebIdentityAndWebhookBindingsFollowTheDoor(t *testing.T) {
	cp := &ControlPlane{}

	public := ingressPlatform(true)
	public.Spec.Ingress.TLS = &v1.IngressTLSSpec{SecretName: "planton-tls"}
	cfg := cp.buildConfig(public, nil)
	if cfg.WebIdentity == nil || cfg.WebIdentity.IssuerURL != "https://planton.example.com" || !cfg.WebIdentity.Offered || cfg.WebIdentity.ClosedReason != "" {
		t.Errorf("public door WebIdentity = %+v, want the door as issuer, offered, no reason", cfg.WebIdentity)
	}
	if cfg.GithubWebhooks == nil || !cfg.GithubWebhooks.Reachable || cfg.GithubWebhooks.ReceiverURL != "https://planton.example.com/webhooks/github" {
		t.Errorf("public door GithubWebhooks = %+v, want reachable at the door's webhook namespace", cfg.GithubWebhooks)
	}
	if cfg.Console == nil || cfg.Console.URL != "https://planton.example.com" {
		t.Errorf("public door Console = %+v, want the door as the browser console's origin", cfg.Console)
	}

	portForward := ingressPlatform(false)
	cfg = cp.buildConfig(portForward, nil)
	if cfg.WebIdentity == nil || cfg.WebIdentity.IssuerURL != "http://localhost:8080" || cfg.WebIdentity.Offered || cfg.WebIdentity.ClosedReason == "" {
		t.Errorf("port-forward WebIdentity = %+v, want the localhost issuer, closed, with a reason", cfg.WebIdentity)
	}
	if cfg.GithubWebhooks == nil || cfg.GithubWebhooks.Reachable || cfg.GithubWebhooks.ReceiverURL != "http://localhost:8080/webhooks/github" {
		t.Errorf("port-forward GithubWebhooks = %+v, want unreachable, the true localhost receiver, never a placeholder", cfg.GithubWebhooks)
	}
	if cfg.Console == nil || cfg.Console.URL != "http://localhost:8080" {
		t.Errorf("port-forward Console = %+v, want the loopback console the browser reaches, never a placeholder", cfg.Console)
	}
}
