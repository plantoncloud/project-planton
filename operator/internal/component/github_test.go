package component

import (
	"context"
	"strings"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	v1 "github.com/plantonhq/planton/operator/api/v1"
)

func githubPlatform(hosts ...v1.GithubHostSpec) *v1.PlantonPlatform {
	p := &v1.PlantonPlatform{ObjectMeta: metav1.ObjectMeta{Name: "planton", Namespace: bindingTestNamespace}}
	if hosts != nil {
		p.Spec.Github = &v1.GithubSpec{Hosts: hosts}
	}
	return p
}

func enterpriseHost(webhooks v1.GithubWebhooksPosture) v1.GithubHostSpec {
	return v1.GithubHostSpec{
		Host: "github.example.com",
		App: &v1.GithubAppSpec{
			ClientID:            "Iv1.8a61f9b3a7aba766",
			PrivateKeySecretRef: v1.SecretKeyRef{Name: "planton-github-example", Key: "private-key.pem"},
			WebhookSecretRef:    &v1.SecretKeyRef{Name: "planton-github-example", Key: "webhook-secret"},
		},
		Webhooks: webhooks,
	}
}

// Nothing declared resolves to nil: the renderer states the github.com
// default itself.
func TestResolveGithub_Undeclared(t *testing.T) {
	p := githubPlatform()
	c := fake.NewClientBuilder().WithScheme(bindingScheme(t)).WithObjects(p).Build()
	binding, refusal, err := resolveGithub(context.Background(), c, p, true)
	if err != nil || refusal != "" || binding != nil {
		t.Fatalf("binding=%+v refusal=%q err=%v, want nil/empty", binding, refusal, err)
	}
}

// A declared App whose Secret holds both keys resolves whole; webhook verdicts
// follow the declaration when it states one and the door when it says auto.
func TestResolveGithub_DeclaredAndPreflighted(t *testing.T) {
	p := githubPlatform(enterpriseHost(v1.GithubWebhooksReachable), v1.GithubHostSpec{Host: "github.com"})
	p.Spec.Github.HostLogin = true
	c := fake.NewClientBuilder().WithScheme(bindingScheme(t)).
		WithObjects(p, federationTestSecret("planton-github-example", map[string]string{
			"private-key.pem": "-----BEGIN RSA PRIVATE KEY-----", "webhook-secret": "s3cret"})).Build()

	binding, refusal, err := resolveGithub(context.Background(), c, p, false)
	if err != nil || refusal != "" {
		t.Fatalf("refusal=%q err=%v, want none", refusal, err)
	}
	if !binding.HostLogin || len(binding.Hosts) != 2 {
		t.Fatalf("expected host login and two hosts, got %+v", binding)
	}
	ent := binding.Hosts[0]
	if ent.App == nil || ent.App.ClientID != "Iv1.8a61f9b3a7aba766" || ent.App.PrivateKeySecretKey != "private-key.pem" || ent.App.WebhookSecretKey != "webhook-secret" {
		t.Errorf("the App resolves whole, got %+v", ent.App)
	}
	if !ent.WebhooksReachable || ent.WebhooksPosture != "reachable" || !strings.Contains(ent.WebhooksReason, "share a network") {
		t.Errorf("a declared reachable posture wins over a private door, got %+v", ent)
	}
	com := binding.Hosts[1]
	if com.App != nil || com.WebhooksReachable || com.WebhooksPosture != "auto" || !strings.Contains(com.WebhooksReason, "front door") {
		t.Errorf("github.com on auto follows the private door, got %+v", com)
	}
}

// A missing Secret or key is refused in words naming the host, the field,
// what the Secret must hold, and what teams do meanwhile -- and the host keeps
// its place with the App absent, so every other host still works.
func TestResolveGithub_MissingSecretIsRefusedInWords(t *testing.T) {
	ctx := context.Background()

	t.Run("secret absent", func(t *testing.T) {
		p := githubPlatform(enterpriseHost(v1.GithubWebhooksAuto), v1.GithubHostSpec{Host: "github.com"})
		c := fake.NewClientBuilder().WithScheme(bindingScheme(t)).WithObjects(p).Build()
		binding, refusal, err := resolveGithub(ctx, c, p, true)
		if err != nil {
			t.Fatal(err)
		}
		for _, want := range []string{`GitHub App Secret "planton-github-example" not found`, "spec.github.hosts[0].app.privateKeySecretRef",
			"PEM text", "teams connect to github.example.com with their own GitHub App"} {
			if !strings.Contains(refusal, want) {
				t.Errorf("refusal must contain %q, got: %s", want, refusal)
			}
		}
		if len(binding.Hosts) != 2 || binding.Hosts[0].App != nil || binding.Hosts[0].AppUnavailableReason != refusal {
			t.Errorf("the host keeps its place with the App absent and the reason beside it, got %+v", binding.Hosts)
		}
	})

	t.Run("webhook key missing", func(t *testing.T) {
		p := githubPlatform(enterpriseHost(v1.GithubWebhooksAuto))
		c := fake.NewClientBuilder().WithScheme(bindingScheme(t)).
			WithObjects(p, federationTestSecret("planton-github-example", map[string]string{"private-key.pem": "pem"})).Build()
		_, refusal, err := resolveGithub(ctx, c, p, true)
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(refusal, `has no "webhook-secret" key`) || !strings.Contains(refusal, "spec.github.hosts[0].app.webhookSecretRef") {
			t.Errorf("the missing key and its field are named, got: %s", refusal)
		}
	})
}

// The verdict table, pinned.
func TestWebhooksVerdict(t *testing.T) {
	cases := []struct {
		posture    v1.GithubWebhooksPosture
		doorPublic bool
		reachable  bool
		want       string
	}{
		{v1.GithubWebhooksAuto, true, true, "auto"},
		{v1.GithubWebhooksAuto, false, false, "auto"},
		{"", false, false, "auto"},
		{v1.GithubWebhooksReachable, false, true, "reachable"},
		{v1.GithubWebhooksUnreachable, true, false, "unreachable"},
	}
	for _, tc := range cases {
		posture, reachable, reason := webhooksVerdict(&v1.GithubHostSpec{Host: "h", Webhooks: tc.posture}, tc.doorPublic)
		if posture != tc.want || reachable != tc.reachable || reason == "" {
			t.Errorf("posture %q door %v: got %q/%v/%q", tc.posture, tc.doorPublic, posture, reachable, reason)
		}
	}
}
