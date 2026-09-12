package resources

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	appsv1 "k8s.io/api/apps/v1"

	"github.com/plantonhq/planton/operator/internal/platformversion"
)

// The boot contract is the set of environment variables the operator renders
// for the platform's own components -- the control plane, the runner, and the
// console. The platform images read them at boot, and an image released
// before a variable existed cannot read it. This test pins the variable NAMES
// of every component, rendered with every optional binding present, together
// with the platform version floor, so a change to either without the other is
// a question the commit must answer rather than a surprise an adopter meets.
//
// Scope, stated plainly: names, not meanings. A changed value for a variable
// that already existed does not fail here; whether it moves the floor is the
// engineer's judgment, and the floor is the place to record it.
var updateBootContract = flag.Bool("update", false, "rewrite testdata/boot-contract.txt from the current rendering")

const bootContractFixture = "testdata/boot-contract.txt"

func TestBootContract(t *testing.T) {
	got := renderBootContract()

	if *updateBootContract {
		if err := os.MkdirAll(filepath.Dir(bootContractFixture), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(bootContractFixture, []byte(got), 0o644); err != nil {
			t.Fatal(err)
		}
		t.Logf("wrote %s", bootContractFixture)
		return
	}

	wantBytes, err := os.ReadFile(bootContractFixture)
	if err != nil {
		t.Fatalf("reading %s: %v (run: go test ./internal/resources -run TestBootContract -update)", bootContractFixture, err)
	}
	want := string(wantBytes)
	if got == want {
		return
	}

	t.Fatalf("the environment the operator renders for the platform changed:\n\n%s\n"+
		"If a platform older than %s cannot boot without this change (a variable it now requires, "+
		"or one it must no longer see), raise platformversion.MinimumSupported to the first "+
		"platform release that reads the new shape, then refresh the fixture. If older platforms "+
		"simply ignore the change, refresh the fixture alone:\n\n"+
		"  go test ./internal/resources -run TestBootContract -update\n",
		describeContractDiff(want, got), platformversion.MinimumSupported)
}

// renderBootContract renders each platform component with every optional
// binding populated -- the widest environment the operator can produce -- and
// lists the variable names per container, sorted, under the floor they were
// last judged against.
func renderBootContract() string {
	var b strings.Builder
	fmt.Fprintf(&b, "minimumSupported: %s\n", platformversion.MinimumSupported)
	writeContainerEnv(&b, "control-plane", ControlPlaneDeployment(fullControlPlaneConfig()))
	writeContainerEnv(&b, "runner", RunnerDeployment(fullRunnerConfig()))
	writeContainerEnv(&b, "console", ConsoleDeployment(fullConsoleConfig()))
	return b.String()
}

func writeContainerEnv(b *strings.Builder, component string, deploy *appsv1.Deployment) {
	for _, c := range deploy.Spec.Template.Spec.Containers {
		names := make([]string, 0, len(c.Env))
		for _, e := range c.Env {
			names = append(names, e.Name)
		}
		sort.Strings(names)
		fmt.Fprintf(b, "\n[%s/%s]\n", component, c.Name)
		for _, n := range names {
			fmt.Fprintf(b, "%s\n", n)
		}
	}
}

// describeContractDiff names what was added and removed per section, so the
// failure reads as a change to the contract rather than as two blobs.
func describeContractDiff(want, got string) string {
	wantSet, gotSet := contractLines(want), contractLines(got)
	var out []string
	for line := range gotSet {
		if !wantSet[line] {
			out = append(out, "  added:   "+line)
		}
	}
	for line := range wantSet {
		if !gotSet[line] {
			out = append(out, "  removed: "+line)
		}
	}
	sort.Strings(out)
	return strings.Join(out, "\n")
}

// contractLines keys every variable by its section so the same name in two
// components is two facts.
func contractLines(s string) map[string]bool {
	set := map[string]bool{}
	section := ""
	for line := range strings.SplitSeq(s, "\n") {
		switch {
		case line == "":
		case strings.HasPrefix(line, "["):
			section = line
		case strings.HasPrefix(line, "minimumSupported:"):
			set[line] = true
		default:
			set[section+" "+line] = true
		}
	}
	return set
}

func fullControlPlaneConfig() ControlPlaneConfig {
	cfg := testControlPlaneConfig()
	cfg.ExternalConfigSecretName = "planton-extra-config"
	cfg.IacModulesVersion = "v0.0.0-fixture"
	cfg.OpenFGA = OpenFGAConnection("planton", "default")
	neo4j := Neo4jConnection("planton", "default")
	cfg.Neo4j = &neo4j
	cfg.Runner = &RunnerBinding{
		CloudOpsSecretName: "planton-runner-cloudops",
		Provisioner:        "tofu",
		DirectDialHost:     "planton-runner.default.svc.cluster.local",
	}
	cfg.RemoteRunners = &RemoteRunnersBinding{
		PlantonAPIEndpoint: "planton.example.com:443",
		TemporalEndpoint:   "planton.example.com:443",
	}
	vault := OpenBAOConnection("planton", "default")
	cfg.Vault = &VaultBinding{
		APIAddr:        vault.APIAddr,
		InitSecretName: vault.InitSecretName,
		RootTokenKey:   vault.RootTokenKey,
	}
	cfg.SecretBackend = &SecretBackendBinding{Type: "aws-secrets-manager", AwsRegion: "ap-south-1"}
	cfg.License = &LicenseBinding{SecretName: "acme-license", SecretKey: "license-key"}
	// Every email arm and every way in at once: not a shape the component can
	// produce (the CRD keeps them exclusive), but the renderer emits by
	// presence, and the contract lists every NAME the operator can render.
	cfg.Email = &EmailBinding{
		Provider:    EmailProviderSMTP,
		FromAddress: "no-reply@planton.example.com",
		FromName:    "Planton",
		ReplyTo:     "help@planton.example.com",
		SMTP: &EmailSMTPBinding{
			Host: "smtp.example.com", Port: 587, Security: "starttls",
			CredentialsSecretName: "planton-email",
			OAuth2: &EmailSMTPOAuth2Binding{
				User: "planton@example.com", TokenURL: "https://login.example.com/token", Scope: "smtp",
				ClientID: "fixture", ClientSecretName: "planton-email-oauth", ClientSecretKey: "client-secret",
			},
			CABundleSecretName: "corp-ca", CABundleSecretKey: "ca.crt",
		},
		Resend: &EmailResendBinding{APIKeySecretName: "planton-email", APIKeySecretKey: "api-key"},
	}
	// The closed arm: it renders one more name (the reason) than the offered
	// arm, and the contract lists the widest environment.
	cfg.WebIdentity = &WebIdentityBinding{IssuerURL: "https://planton.example.com", Offered: false, ClosedReason: "fixture reason"}
	cfg.GithubWebhooks = &GithubWebhooksBinding{Reachable: false, ReceiverURL: "https://planton.example.com/webhooks/github"}
	cfg.Console = &ConsoleBinding{URL: "https://planton.example.com"}
	return cfg
}

func fullRunnerConfig() RunnerConfig {
	cfg := testRunnerConfig()
	cfg.CloudCredentialsSecretName = "planton-runner-cloud-credentials"
	return cfg
}

func fullConsoleConfig() ConsoleConfig {
	return ConsoleConfig{
		CRName: "planton", Namespace: "default", Version: "v1.0.0",
		Replicas:                 1,
		ExternalConfigSecretName: "planton-console-extra-config",
		PublicURL:                "http://planton.example.com",
		GRPCEndpoint:             "planton.example.com:80",
		Identity: &ConsoleIdentityConfig{
			IssuerURL:         "http://planton.example.com/idp/realms/planton",
			InternalIssuerURL: "http://planton-identity.default.svc.cluster.local/idp/realms/planton",
			Realm:             "planton",
		},
	}
}
