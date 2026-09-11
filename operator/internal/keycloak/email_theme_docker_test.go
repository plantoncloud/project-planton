//go:build requires_docker

package keycloak

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/plantonhq/planton/operator/internal/keycloaklogintheme"
)

// The identity server's emails, rendered by a real Keycloak through the
// Planton theme and read back from a real inbox: the layout the platform
// generates is FreeMarker the server accepts, every message body composes it,
// the install's facts (its name, its console, its reply-to) reach the words a
// person reads, and both renderings -- HTML and text -- arrive.
//
// The message under test is the one an administrator triggers
// (execute-actions-email with UPDATE_PASSWORD): it exercises the layout, the
// button, the fallback link, the expiry sentence, and the required-actions
// list, and it needs no browser session. The sign-in page's own "Forgot
// password?" rides the same layout through password-reset.ftl; the lab lane
// proves that round trip end to end.
func TestEmailTheme_RendersThroughKeycloak(t *testing.T) {
	admin := authedAdmin(t)
	createRealm(t, admin, "themed", nil)

	// The realm sends through the suite's Mailpit, plainly and without a
	// credential: the relay is inside the suite's own network.
	in := convergeInput("themed")
	in.Email = &OwnedRealmEmail{Relay: &OwnedSMTPRelay{
		Host:            testMailpit.alias,
		Port:            1025,
		Security:        "none",
		From:            "no-reply@acme.example.com",
		FromDisplayName: testThemeFacts.BrandName,
		ReplyTo:         testThemeFacts.ReplyTo,
		Auth:            OwnedSMTPAuthNone,
	}}
	mustConverge(t, in)

	// A person with an address, and an administrator asking them to set a
	// password.
	ctx := context.Background()
	if err := admin.do(ctx, http.MethodPost, admin.adminPath("themed", "/users"), Representation{
		"username": "dev", "email": "dev@acme.example.com", "firstName": "Dev", "lastName": "Patel",
		"enabled": true, "emailVerified": true,
	}, http.StatusCreated, nil); err != nil {
		t.Fatalf("creating the user: %v", err)
	}
	users, err := admin.FindUsersByEmail(ctx, "themed", "dev@acme.example.com")
	if err != nil || len(users) != 1 {
		t.Fatalf("finding the user: %v (%d found)", err, len(users))
	}
	userID, _ := users[0]["id"].(string)
	if err := admin.do(ctx, http.MethodPut,
		admin.adminPath("themed", "/users/"+userID+"/execute-actions-email?lifespan=3600"),
		[]string{"UPDATE_PASSWORD"}, http.StatusNoContent, nil); err != nil {
		t.Fatalf("asking for the account update email: %v", err)
	}

	message := testMailpit.waitForMessage(t, "dev@acme.example.com", 60*time.Second)

	if got := message.Subject; got != "Finish setting up your account" {
		t.Errorf("subject = %q, want the theme's, not Keycloak's stock line", got)
	}
	if got := message.From.Name; got != testThemeFacts.BrandName {
		t.Errorf("From display name = %q, want %q", got, testThemeFacts.BrandName)
	}
	html, text := message.HTML, message.Text
	for _, want := range []string{
		testThemeFacts.BrandName,                                        // the wordmark and the sentences name the install
		"Finish setting up your account",                                // the heading
		"Update Your Account",                                           // the button
		"Update Password",                                               // the required action, in Keycloak's own words
		"Sent by " + testThemeFacts.BrandName,                           // the footer
		testThemeFacts.ConsoleHost(),                                    // the console host in the footer
		`Questions? Write to <a href="mailto:` + testThemeFacts.ReplyTo, // the reply-to, as a link a person can act on
		"/realms/themed/login-actions/action-token",                     // the action link
		`class="pl-button"`,                                             // the generated layout's own markup
		"Or open <a",                                                    // the labelled fallback: a way in without a wall of token
		"the account update link</a>.",                                  // ... with the label, not the URL, as its text
		"This link works for",                                           // the expiry sentence
	} {
		if !strings.Contains(html, want) {
			t.Errorf("html part is missing %q:\n%s", want, html)
		}
	}
	for _, want := range []string{
		testThemeFacts.BrandName,
		"/realms/themed/login-actions/action-token",
		"Sent by " + testThemeFacts.BrandName,
		"Questions? Write to " + testThemeFacts.ReplyTo,
	} {
		if !strings.Contains(text, want) {
			t.Errorf("text part is missing %q:\n%s", want, text)
		}
	}
	for _, forbidden := range []string{"${", "<#", "__FTL_", "{{", "[KEYCLOAK]", "realmName"} {
		if strings.Contains(html, forbidden) || strings.Contains(text, forbidden) {
			t.Errorf("a rendered email carries template syntax %q", forbidden)
		}
	}
}

// writeThemeDir writes the theme as Keycloak expects it on disk, under a
// directory named for the theme, for `docker cp` into the container.
func writeThemeDir(facts keycloaklogintheme.EmailFacts) (string, error) {
	root, err := os.MkdirTemp("", "planton-theme-")
	if err != nil {
		return "", err
	}
	themeRoot := filepath.Join(root, keycloaklogintheme.ThemeName)
	for p, content := range keycloaklogintheme.Files(facts) {
		dest := filepath.Join(themeRoot, filepath.FromSlash(p))
		if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
			return "", err
		}
		if err := os.WriteFile(dest, content, 0o644); err != nil {
			return "", err
		}
	}
	return themeRoot, nil
}

// mailpit is the suite's mail server: one container on the suite network,
// SMTP on 1025 for Keycloak, the inbox API published to the host for the
// tests.
type mailpit struct {
	containerID string
	alias       string
	apiRoot     string
}

const mailpitImage = "axllent/mailpit:v1.27"

func startMailpit(network, alias string) (*mailpit, error) {
	out, err := exec.Command("docker", "run", "-d", "--rm",
		"--network", network, "--network-alias", alias,
		"-p", "127.0.0.1:0:8025",
		mailpitImage).Output()
	if err != nil {
		return nil, fmt.Errorf("starting mailpit: %w", err)
	}
	m := &mailpit{containerID: strings.TrimSpace(string(out)), alias: alias}
	portOut, err := exec.Command("docker", "port", m.containerID, "8025/tcp").Output()
	if err != nil {
		m.stop()
		return nil, fmt.Errorf("reading mailpit's port: %w", err)
	}
	m.apiRoot = "http://" + strings.TrimSpace(strings.Split(string(portOut), "\n")[0])

	deadline := time.Now().Add(time.Minute)
	client := &http.Client{Timeout: 3 * time.Second}
	for time.Now().Before(deadline) {
		resp, err := client.Get(m.apiRoot + "/api/v1/info")
		if err == nil {
			_ = resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				return m, nil
			}
		}
		time.Sleep(time.Second)
	}
	m.stop()
	return nil, fmt.Errorf("mailpit never answered at %s", m.apiRoot)
}

func (m *mailpit) stop() {
	_ = exec.Command("docker", "rm", "-f", m.containerID).Run()
}

// mailpitMessage is the slice of Mailpit's message representation the tests read.
type mailpitMessage struct {
	ID      string `json:"ID"`
	Subject string `json:"Subject"`
	From    struct {
		Name    string `json:"Name"`
		Address string `json:"Address"`
	} `json:"From"`
	To []struct {
		Address string `json:"Address"`
	} `json:"To"`
	HTML string `json:"HTML"`
	Text string `json:"Text"`
}

// waitForMessage polls the inbox for the first message addressed to `to` and
// returns it whole (Mailpit's list omits the bodies; the single-message read
// carries them).
func (m *mailpit) waitForMessage(t *testing.T, to string, timeout time.Duration) mailpitMessage {
	t.Helper()
	client := &http.Client{Timeout: 5 * time.Second}
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		var list struct {
			Messages []mailpitMessage `json:"messages"`
		}
		if err := getJSON(client, m.apiRoot+"/api/v1/messages", &list); err == nil {
			for _, candidate := range list.Messages {
				for _, recipient := range candidate.To {
					if recipient.Address == to {
						var full mailpitMessage
						if err := getJSON(client, m.apiRoot+"/api/v1/message/"+candidate.ID, &full); err != nil {
							t.Fatalf("reading message %s: %v", candidate.ID, err)
						}
						return full
					}
				}
			}
		}
		time.Sleep(time.Second)
	}
	t.Fatalf("no message to %s reached mailpit within %s", to, timeout)
	return mailpitMessage{}
}

func getJSON(client *http.Client, endpoint string, out any) error {
	resp, err := client.Get(endpoint)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("%s answered %d", endpoint, resp.StatusCode)
	}
	return json.NewDecoder(resp.Body).Decode(out)
}
