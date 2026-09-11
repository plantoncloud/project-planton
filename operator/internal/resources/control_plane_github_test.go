package resources

import (
	"encoding/json"
	"flag"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

var updateGithubFacts = flag.Bool("update-github-facts", false, "rewrite testdata/github-facts.json from the renderer")

func exampleGithubBinding() *GithubBinding {
	return &GithubBinding{
		HostLogin: false,
		Hosts: []GithubHostBinding{
			{
				Host: "github.example.com",
				App: &GithubAppBinding{
					ClientID:             "Iv1.8a61f9b3a7aba766",
					PrivateKeySecretName: "planton-github-example",
					PrivateKeySecretKey:  "private-key.pem",
					WebhookSecretName:    "planton-github-example",
					WebhookSecretKey:     "webhook-secret",
				},
				WebhooksReachable: true,
				WebhooksPosture:   "reachable",
				WebhooksReason:    "the install declares that github.example.com can deliver webhooks to it (they share a network); pushes trigger runs",
			},
			{
				Host:              "github.com",
				WebhooksReachable: false,
				WebhooksPosture:   "auto",
				WebhooksReason:    FrontDoorWebhooksReason(false),
			},
		},
	}
}

// The facts file is the operator→product contract: the control plane's reader
// pins this same fixture, so a change here is a change on both sides.
func TestGithubFacts_Fixture(t *testing.T) {
	facts := GithubFactsFrom(exampleGithubBinding(), false)
	rendered, err := json.MarshalIndent(facts, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	rendered = append(rendered, '\n')
	fixture := filepath.Join("testdata", "github-facts.json")
	if *updateGithubFacts {
		if err := os.WriteFile(fixture, rendered, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	want, err := os.ReadFile(fixture)
	if err != nil {
		t.Fatalf("read fixture: %v (run with -update-github-facts to create it)", err)
	}
	if string(want) != string(rendered) {
		t.Errorf("the GitHub facts contract changed; if the control plane's reader moved with it, refresh with -update-github-facts.\n got: %s\nwant: %s", rendered, want)
	}
}

// An install that declares nothing still gets a true facts file: github.com,
// no install App with the reason, webhooks judged by the front door.
func TestGithubFacts_UndeclaredIsTheGithubComDefault(t *testing.T) {
	for _, doorPublic := range []bool{true, false} {
		facts := GithubFactsFrom(nil, doorPublic)
		if len(facts.Hosts) != 1 || facts.Hosts[0].Host != GithubDefaultHost {
			t.Fatalf("expected the github.com default, got %+v", facts.Hosts)
		}
		h := facts.Hosts[0]
		if h.Declared {
			t.Error("the default is not a declaration")
		}
		if h.App != nil || h.AppUnavailableReason != PlatformAppUnavailableReason {
			t.Errorf("no install App and the standing reason, got %+v / %q", h.App, h.AppUnavailableReason)
		}
		if h.Webhooks.Reachable != doorPublic || h.Webhooks.Posture != "auto" {
			t.Errorf("webhooks follow the door (%v), got %+v", doorPublic, h.Webhooks)
		}
		if !strings.Contains(h.Webhooks.Reason, "front door") {
			t.Errorf("the reason names the front door, got %q", h.Webhooks.Reason)
		}
	}
}

// Declared hosts keep their order (the wizard offers them in it), an App's
// files are named under the host's own directory, and a host without an App
// carries the reason.
func TestGithubFacts_DeclaredHosts(t *testing.T) {
	facts := GithubFactsFrom(exampleGithubBinding(), true)
	if len(facts.Hosts) != 2 || facts.Hosts[0].Host != "github.example.com" || facts.Hosts[1].Host != "github.com" {
		t.Fatalf("declaration order kept, got %+v", facts.Hosts)
	}
	app := facts.Hosts[0].App
	if app == nil || app.ClientID != "Iv1.8a61f9b3a7aba766" ||
		app.PrivateKeyFile != "/etc/planton/github-credentials/github.example.com/private-key.pem" ||
		app.WebhookSecretFile != "/etc/planton/github-credentials/github.example.com/webhook-secret" {
		t.Errorf("the App's files live under the host's directory, got %+v", app)
	}
	if !facts.Hosts[0].Declared || !facts.Hosts[1].Declared {
		t.Error("declared hosts are declared")
	}
	if facts.Hosts[1].App != nil || facts.Hosts[1].AppUnavailableReason == "" {
		t.Errorf("a host without an App carries the reason, got %+v", facts.Hosts[1])
	}
	// The door is public here, but github.com's verdict is the binding's
	// (the component decided it); the facts renderer never overrides a
	// declared host's verdict with the door's.
	if facts.Hosts[1].Webhooks.Reachable {
		t.Error("a declared host's verdict is the binding's, not the door's")
	}
}

// The credentials volume projects each App's Secret keys onto fixed file
// names under the host directory, and is absent when no App is declared.
func TestGithubCredentialsVolume(t *testing.T) {
	if volume, mount := githubCredentialsVolume(nil); volume != nil || mount != nil {
		t.Error("no declaration, no volume")
	}
	noApps := &GithubBinding{Hosts: []GithubHostBinding{{Host: "github.com"}}}
	if volume, _ := githubCredentialsVolume(noApps); volume != nil {
		t.Error("hosts without Apps mount nothing")
	}

	volume, mount := githubCredentialsVolume(exampleGithubBinding())
	if volume == nil || mount == nil || mount.MountPath != GithubCredentialsMountPath || !mount.ReadOnly || mount.SubPath != "" {
		t.Fatalf("expected a read-only whole-directory mount at %s, got %+v", GithubCredentialsMountPath, mount)
	}
	sources := volume.Projected.Sources
	if len(sources) != 2 {
		t.Fatalf("one projection per (Secret, key), got %d", len(sources))
	}
	if sources[0].Secret.Name != "planton-github-example" || sources[0].Secret.Items[0].Key != "private-key.pem" ||
		sources[0].Secret.Items[0].Path != "github.example.com/private-key.pem" {
		t.Errorf("the key lands under the host's directory, got %+v", sources[0].Secret.Items)
	}
	if sources[1].Secret.Items[0].Path != "github.example.com/webhook-secret" {
		t.Errorf("the webhook secret lands beside it, got %+v", sources[1].Secret.Items)
	}
}

// The Deployment: the facts file is mounted on every install and named by a
// static variable; the credentials volume appears with an App; the one-host
// variables the current platform reads follow github.com's declared verdict
// and the host-login toggle.
func TestControlPlaneDeployment_GithubDeclaration(t *testing.T) {
	cfg := testControlPlaneConfig()
	cfg.Github = exampleGithubBinding()
	cfg.Github.HostLogin = true
	cfg.GithubWebhooks = &GithubWebhooksBinding{Reachable: true, ReceiverURL: "https://planton.example.com/webhooks/github"}

	deploy := ControlPlaneDeployment(cfg)
	podSpec := deploy.Spec.Template.Spec
	envMap := envVarMap(podSpec.Containers[0].Env)

	if envMap["PLANTON_GITHUB_FACTS_FILE"] != GithubFactsFilePath() {
		t.Errorf("PLANTON_GITHUB_FACTS_FILE = %q, want %q", envMap["PLANTON_GITHUB_FACTS_FILE"], GithubFactsFilePath())
	}
	if envMap["GITHUB_WEBHOOKS_REACHABLE"] != strconv.FormatBool(false) {
		t.Errorf("the one-host variable follows github.com's declared verdict (unreachable here), got %q", envMap["GITHUB_WEBHOOKS_REACHABLE"])
	}
	if envMap["PLANTON_CONNECT_METHODAVAILABILITY_HOSTLOGIN_AVAILABILITY"] != "available" {
		t.Error("host login declared on renders available")
	}
	if _, present := envMap["GITHUB_CHECKS_DETAILS_URL_FORMAT"]; present {
		t.Error("the unconsumed variable is no longer rendered")
	}

	var credentials, facts bool
	for _, v := range podSpec.Volumes {
		switch v.Name {
		case githubCredentialsVolumeName:
			credentials = true
		case githubFactsVolumeName:
			facts = true
		}
	}
	if !credentials || !facts {
		t.Errorf("expected the facts and credentials volumes, got %+v", podSpec.Volumes)
	}

	plain := ControlPlaneDeployment(testControlPlaneConfig())
	plainEnv := envVarMap(plain.Spec.Template.Spec.Containers[0].Env)
	if _, present := plainEnv["PLANTON_CONNECT_METHODAVAILABILITY_HOSTLOGIN_AVAILABILITY"]; present {
		t.Error("host login undeclared renders nothing: the platform's own default is off")
	}
}
