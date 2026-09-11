//go:build requires_docker

package keycloak

// The realm-convergence suite: every never-clobber and repair property of the
// reconciler proven against a REAL Keycloak (the production-pinned image, the
// production path prefix), not a mock of one -- and the federation half
// proven against a REAL Samba Active Directory (the lab directory,
// hack/lab-directory). Docker-gated by build tag -- exactly like the
// operator's e2e tag and the Java requires-docker suites -- so the fast unit
// gates stay Docker-free by construction; gazelle skips tagged files, so
// Bazel never sees this. Run it deliberately:
//
//	make test-realm-convergence
//
// One Keycloak and one lab directory serve the whole suite (container boots
// dominate the runtime); each test creates its own realm, so tests stay
// isolated and order-free. The two containers share a Docker network:
// KEYCLOAK is the LDAP client (federation runs server-side), so the lab
// directory must be reachable from inside Keycloak's container, not just
// from the test process. The lab's private CA is mounted into Keycloak's
// truststore exactly the way the operator mounts a manifest's
// caBundleSecretRef in production -- every federation test therefore
// rehearses private-CA LDAPS, the classic enterprise blocker.
//
// Container management is hand-rolled docker CLI in TestMain rather than a
// testcontainers-go dependency: the suite is excluded from Bazel, so the
// dependency would swell MODULE.bazel for targets that never compile it, and
// the whole need is a few `docker run`s plus readiness polls.

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/plantonhq/planton/operator/internal/keycloaklogintheme"
	"github.com/plantonhq/planton/operator/internal/resources"
)

const (
	testBootstrapAdminUser = "admin"
	testBootstrapAdminPass = "convergence-suite-password"
	testPublicURL          = "https://planton.example.com"

	// testNetworkAlias is Keycloak's name on the suite's Docker network --
	// the entra-sim issuer host the broker tests dial from BOTH sides (the
	// test process through a dialer override, Keycloak through container
	// DNS), so the issuer string stays one value everywhere.
	testNetworkAlias = "idp"
)

var (
	testServerRoot string
	testLab        *labDirectory
	// testMailpit is the suite's mail server: SMTP at mailpit:1025 from
	// Keycloak's side of the network, the inbox API at this host address.
	testMailpit *mailpit
)

// testThemeFacts is the install the suite's Keycloak believes it serves; the
// email-theme test asserts these words in the messages it renders.
var testThemeFacts = keycloaklogintheme.EmailFacts{
	BrandName:  "Acme Platform",
	ConsoleURL: testPublicURL,
	ReplyTo:    "it-help@acme.example.com",
}

func TestMain(m *testing.M) {
	suffix := fmt.Sprintf("%d", os.Getpid())
	network := "planton-identity-suite-" + suffix

	if out, err := exec.Command("docker", "network", "create", network).CombinedOutput(); err != nil {
		fmt.Fprintf(os.Stderr, "creating suite network: %v\n%s\n", err, out)
		os.Exit(1)
	}
	cleanupNetwork := func() { _ = exec.Command("docker", "network", "rm", network).Run() }

	// The lab directory boots FIRST: Keycloak's run needs the lab CA on
	// disk to mount into its truststore.
	lab, err := startLabDirectory(network, "lab-dc-"+suffix)
	if err != nil {
		fmt.Fprintf(os.Stderr, "starting the lab directory: %v\n", err)
		cleanupNetwork()
		os.Exit(1)
	}
	testLab = lab
	cleanupLab := func() { lab.stop(); cleanupNetwork() }

	// The mail server Keycloak sends through: the email-theme test reads
	// what it rendered from Mailpit's inbox API.
	mail, err := startMailpit(network, "mailpit-"+suffix)
	if err != nil {
		fmt.Fprintf(os.Stderr, "starting mailpit: %v\n", err)
		cleanupLab()
		os.Exit(1)
	}
	testMailpit = mail
	cleanupMail := func() { mail.stop(); cleanupLab() }

	image := resources.IdentityDefaultImageRepo + ":" + resources.IdentityDefaultImageTag

	// The production serving shape: bootstrap admin from env, the /idp
	// relative path, the CA bundle at the production truststore path with
	// KC_TRUSTSTORE_PATHS -- so the suite exercises the same URLs and the
	// same trust wiring the operator composes in-cluster. The CA travels by
	// create + docker cp + start, NEVER a bind mount: Docker Desktop's VM
	// file sharing silently materializes an unshared host path as an EMPTY
	// directory, and Keycloak then boots happily trusting nothing (caught
	// live by this suite's first LDAPS run).
	// The Planton theme rides in the same way the operator mounts it (the
	// theme directory under /opt/keycloak/themes, selected as the server's
	// default for every type), so the suite renders the sign-in pages and
	// the emails the way an install does. No --rm: a boot failure's logs
	// must survive for the diagnostics below (cleanup is the explicit rm -f
	// either way).
	out, err := exec.Command("docker", "create",
		"--network", network, "--network-alias", testNetworkAlias,
		"-e", "KC_BOOTSTRAP_ADMIN_USERNAME="+testBootstrapAdminUser,
		"-e", "KC_BOOTSTRAP_ADMIN_PASSWORD="+testBootstrapAdminPass,
		"-e", "KC_HTTP_RELATIVE_PATH="+resources.IdentityPathPrefix,
		"-e", "KC_TRUSTSTORE_PATHS="+resources.IdentityCATruststorePath+"/"+resources.IdentityCABundleFileName,
		"-p", "127.0.0.1:0:8080",
		image, "start-dev", "--spi-theme--default="+keycloaklogintheme.ThemeName).Output()
	if err != nil {
		fmt.Fprintf(os.Stderr, "creating keycloak container: %v\n", err)
		cleanupMail()
		os.Exit(1)
	}
	containerID := strings.TrimSpace(string(out))
	cleanupAll := func() {
		_ = exec.Command("docker", "rm", "-f", containerID).Run()
		cleanupMail()
	}

	if cpOut, err := exec.Command("docker", "cp", lab.caDir, containerID+":"+resources.IdentityCATruststorePath).CombinedOutput(); err != nil {
		fmt.Fprintf(os.Stderr, "copying the lab CA into keycloak: %v\n%s\n", err, cpOut)
		cleanupAll()
		os.Exit(1)
	}
	themeDir, err := writeThemeDir(testThemeFacts)
	if err != nil {
		fmt.Fprintf(os.Stderr, "writing the theme to disk: %v\n", err)
		cleanupAll()
		os.Exit(1)
	}
	if cpOut, err := exec.Command("docker", "cp", themeDir, containerID+":/opt/keycloak/themes/").CombinedOutput(); err != nil {
		fmt.Fprintf(os.Stderr, "copying the theme into keycloak: %v\n%s\n", err, cpOut)
		cleanupAll()
		os.Exit(1)
	}
	if startOut, err := exec.Command("docker", "start", containerID).CombinedOutput(); err != nil {
		fmt.Fprintf(os.Stderr, "starting keycloak container: %v\n%s\n", err, startOut)
		cleanupAll()
		os.Exit(1)
	}

	portOut, err := exec.Command("docker", "port", containerID, "8080/tcp").Output()
	if err != nil {
		fmt.Fprintf(os.Stderr, "reading container port: %v\n", err)
		cleanupAll()
		os.Exit(1)
	}
	hostPort := strings.TrimSpace(strings.Split(string(portOut), "\n")[0])
	testServerRoot = "http://" + hostPort + resources.IdentityPathPrefix

	if err := waitForKeycloak(testServerRoot, 3*time.Minute); err != nil {
		logs, _ := exec.Command("docker", "logs", "--tail", "50", containerID).CombinedOutput()
		fmt.Fprintf(os.Stderr, "keycloak never became ready: %v\ncontainer logs:\n%s\n", err, logs)
		cleanupAll()
		os.Exit(1)
	}

	code := m.Run()
	cleanupAll()
	os.Exit(code)
}

func waitForKeycloak(serverRoot string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	client := &http.Client{Timeout: 3 * time.Second}
	for time.Now().Before(deadline) {
		resp, err := client.Get(serverRoot + "/realms/master")
		if err == nil {
			resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				return nil
			}
		}
		time.Sleep(2 * time.Second)
	}
	return fmt.Errorf("no 200 from %s/realms/master within %s", serverRoot, timeout)
}

// authedAdmin returns an authenticated raw admin client for test fixtures --
// creating realms, seeding admin-owned state, breaking owned clients.
func authedAdmin(t *testing.T) *AdminClient {
	t.Helper()
	admin := NewAdminClient(&http.Client{Timeout: 10 * time.Second}, testServerRoot)
	if err := admin.Authenticate(context.Background(), testBootstrapAdminUser, testBootstrapAdminPass); err != nil {
		t.Fatalf("authenticating fixture admin: %v", err)
	}
	return admin
}

// createRealm creates a test realm from the given representation (nil means
// an empty enabled realm).
func createRealm(t *testing.T, admin *AdminClient, name string, rep Representation) {
	t.Helper()
	if rep == nil {
		rep = Representation{}
	}
	rep["realm"] = name
	rep["enabled"] = true
	if err := admin.do(context.Background(), http.MethodPost,
		admin.serverRoot+"/admin/realms", rep, http.StatusCreated, nil); err != nil {
		t.Fatalf("creating realm %s: %v", name, err)
	}
}

func convergeInput(realm string) ConvergeInput {
	return ConvergeInput{
		OwnedRealmInput: OwnedRealmInput{
			Realm:               realm,
			PublicURL:           testPublicURL,
			ConsoleClientSecret: "console-secret-value",
			UsersClientSecret:   "users-secret-value",
		},
		ServerRoot:    testServerRoot,
		AdminUsername: testBootstrapAdminUser,
		AdminPassword: testBootstrapAdminPass,
		HTTPClient:    &http.Client{Timeout: 10 * time.Second},
	}
}

func mustConverge(t *testing.T, in ConvergeInput) *Report {
	t.Helper()
	report, err := Converge(context.Background(), in)
	if err != nil {
		t.Fatalf("converge failed: %v (repairs so far: %v)", err, report.Repairs)
	}
	return report
}

// A fresh (empty) realm converges to the full owned set: the security
// settings, all three clients with their mappers, the users client's
// service-account roles -- and the SECOND pass writes nothing, which is the
// idempotency property the whole reconcile cadence stands on.
func TestConvergence_FreshRealmAndIdempotency(t *testing.T) {
	admin := authedAdmin(t)
	createRealm(t, admin, "fresh", nil)
	in := convergeInput("fresh")

	first := mustConverge(t, in)
	if first.Clean() {
		t.Fatal("first pass on a fresh realm must repair (nothing was there)")
	}

	// Realm settings converged to the owned values.
	realm, err := admin.GetRealm(context.Background(), "fresh")
	if err != nil {
		t.Fatal(err)
	}
	for key, want := range OwnedRealmSettings(in.OwnedRealmInput) {
		if !jsonEqual(want, realm[key]) {
			t.Errorf("realm setting %s = %v, want %v", key, realm[key], want)
		}
	}

	// Every owned client exists with its owned shape.
	for _, owned := range OwnedClients(in.OwnedRealmInput) {
		live, found, err := admin.FindClientByClientID(context.Background(), "fresh", owned.ClientID)
		if err != nil {
			t.Fatal(err)
		}
		if !found {
			t.Fatalf("client %s missing after convergence", owned.ClientID)
		}
		for key, want := range owned.Fields {
			if !jsonEqual(want, live[key]) {
				t.Errorf("client %s field %s = %v, want %v", owned.ClientID, key, live[key], want)
			}
		}
		// Set comparison: Keycloak stores redirect URIs unordered.
		if !stringSetEqual(owned.RedirectURIs, live["redirectUris"]) {
			t.Errorf("client %s redirectUris = %v, want %v", owned.ClientID, live["redirectUris"], owned.RedirectURIs)
		}
		attrs, _ := live["attributes"].(map[string]any)
		for key, want := range owned.Attributes {
			if got, _ := attrs[key].(string); got != want {
				t.Errorf("client %s attribute %s = %q, want %q", owned.ClientID, key, got, want)
			}
		}
		clientUUID, _ := live["id"].(string)
		mappers, err := admin.ListProtocolMappers(context.Background(), "fresh", clientUUID)
		if err != nil {
			t.Fatal(err)
		}
		for _, wantMapper := range owned.Mappers {
			found := false
			for _, m := range mappers {
				if m["name"] == wantMapper.Name {
					found = true
				}
			}
			if !found {
				t.Errorf("client %s missing mapper %s", owned.ClientID, wantMapper.Name)
			}
		}
		if owned.Secret != "" {
			secret, err := admin.GetClientSecret(context.Background(), "fresh", clientUUID)
			if err != nil {
				t.Fatal(err)
			}
			if secret != owned.Secret {
				t.Errorf("client %s secret not converged to the operator-managed value", owned.ClientID)
			}
		}
	}

	// The users client's service account holds exactly the desired roles.
	assertServiceAccountRoles(t, admin, "fresh",
		resources.IdentityUsersClientID, resources.IdentityUsersServiceAccountRoles())

	// Idempotency: a converged realm produces ZERO writes.
	second := mustConverge(t, in)
	if !second.Clean() {
		t.Fatalf("second pass must write nothing, wrote %d: %v", second.Writes, second.Repairs)
	}
}

// The never-clobber proof: admin-created state -- a user and a client the
// operator does not own -- survives convergence byte-identical.
func TestConvergence_NeverClobbersAdminCreatedState(t *testing.T) {
	admin := authedAdmin(t)
	createRealm(t, admin, "adminland", nil)
	in := convergeInput("adminland")
	mustConverge(t, in)

	ctx := context.Background()

	// An admin-created user with state a full-object write would mangle.
	if err := admin.do(ctx, http.MethodPost,
		admin.serverRoot+"/admin/realms/adminland/users",
		Representation{
			"username": "hand-made-admin", "enabled": true,
			"email": "hand@example.com", "firstName": "Hand", "lastName": "Made",
			"attributes": map[string]any{"custom-note": []string{"do-not-touch"}},
		}, http.StatusCreated, nil); err != nil {
		t.Fatal(err)
	}
	// An admin-created client with a clientId the operator does not own.
	if err := admin.CreateClient(ctx, "adminland", Representation{
		"clientId": "corp-custom-tool", "enabled": true, "protocol": "openid-connect",
		"publicClient": false, "secret": "corp-secret", "standardFlowEnabled": true,
		"redirectUris": []string{"https://corp-tool.example.com/callback"},
		"attributes":   map[string]any{"corp-attr": "corp-value"},
	}); err != nil {
		t.Fatal(err)
	}

	snapshotUser := findUserByUsername(t, admin, "adminland", "hand-made-admin")
	snapshotClient, _, err := admin.FindClientByClientID(ctx, "adminland", "corp-custom-tool")
	if err != nil {
		t.Fatal(err)
	}

	report := mustConverge(t, in)
	if !report.Clean() {
		t.Fatalf("admin-created state must not register as drift; wrote %d: %v", report.Writes, report.Repairs)
	}

	afterUser := findUserByUsername(t, admin, "adminland", "hand-made-admin")
	if !jsonEqual(snapshotUser, afterUser) {
		t.Errorf("admin-created user changed across convergence:\nbefore %v\nafter  %v", snapshotUser, afterUser)
	}
	afterClient, _, err := admin.FindClientByClientID(ctx, "adminland", "corp-custom-tool")
	if err != nil {
		t.Fatal(err)
	}
	if !jsonEqual(snapshotClient, afterClient) {
		t.Errorf("admin-created client changed across convergence:\nbefore %v\nafter  %v", snapshotClient, afterClient)
	}
}

// A hand-broken owned client is a repair case, not an admin choice: flipped
// flags, a rogue redirect URI, and a DELETED audience mapper all converge
// back -- while an admin-added attribute on the same client survives the
// repair (owned KEYS, not the owned object wholesale).
func TestConvergence_RepairsHandBrokenOwnedClient(t *testing.T) {
	admin := authedAdmin(t)
	createRealm(t, admin, "broken", nil)
	in := convergeInput("broken")
	mustConverge(t, in)

	ctx := context.Background()
	live, _, err := admin.FindClientByClientID(ctx, "broken", resources.IdentityConsoleClientID)
	if err != nil {
		t.Fatal(err)
	}
	clientUUID, _ := live["id"].(string)

	// Break it: wrong flow flags, wrong redirect, an admin attribute added.
	live["standardFlowEnabled"] = false
	live["directAccessGrantsEnabled"] = true
	live["redirectUris"] = []string{"https://evil.example.com/steal"}
	attrs, _ := live["attributes"].(map[string]any)
	attrs["admin-added-note"] = "an admin put this here"
	live["attributes"] = attrs
	if err := admin.UpdateClient(ctx, "broken", clientUUID, live); err != nil {
		t.Fatal(err)
	}
	// Delete the audience mapper outright (the drift that rejects every
	// browser token).
	mappers, err := admin.ListProtocolMappers(ctx, "broken", clientUUID)
	if err != nil {
		t.Fatal(err)
	}
	for _, m := range mappers {
		if m["name"] == resources.IdentityAudienceMapperName {
			mapperID, _ := m["id"].(string)
			if err := admin.do(ctx, http.MethodDelete,
				admin.serverRoot+"/admin/realms/broken/clients/"+clientUUID+"/protocol-mappers/models/"+mapperID,
				nil, http.StatusNoContent, nil); err != nil {
				t.Fatal(err)
			}
		}
	}

	report := mustConverge(t, in)
	if report.Clean() {
		t.Fatal("a hand-broken owned client must register as drift")
	}

	repaired, _, err := admin.FindClientByClientID(ctx, "broken", resources.IdentityConsoleClientID)
	if err != nil {
		t.Fatal(err)
	}
	if repaired["standardFlowEnabled"] != true || repaired["directAccessGrantsEnabled"] != false {
		t.Error("flow flags not repaired")
	}
	if !stringSetEqual(resources.IdentityConsoleRedirectURIs(testPublicURL), repaired["redirectUris"]) {
		t.Errorf("redirect URIs not repaired: %v", repaired["redirectUris"])
	}
	repairedAttrs, _ := repaired["attributes"].(map[string]any)
	if repairedAttrs["admin-added-note"] != "an admin put this here" {
		t.Error("the repair clobbered an admin-added attribute -- owned KEYS only, never the whole map")
	}
	mappers, err = admin.ListProtocolMappers(ctx, "broken", clientUUID)
	if err != nil {
		t.Fatal(err)
	}
	foundMapper := false
	for _, m := range mappers {
		if m["name"] == resources.IdentityAudienceMapperName {
			foundMapper = true
		}
	}
	if !foundMapper {
		t.Error("deleted audience mapper not recreated")
	}
}

// The B-21 case: a realm imported before planton-users (and planton-cli)
// existed gains the missing clients -- with the users client's service
// account holding exactly its roles, which client creation alone does not
// grant.
func TestConvergence_PreUsersClientRealmGainsMissingClients(t *testing.T) {
	admin := authedAdmin(t)
	// The old import shape: console client only, old settings.
	createRealm(t, admin, "oldrealm", Representation{
		"sslRequired": "none", "registrationAllowed": false,
		"clients": []Representation{{
			"clientId": resources.IdentityConsoleClientID, "name": "Planton Console",
			"enabled": true, "protocol": "openid-connect", "publicClient": false,
			"secret": "console-secret-value", "standardFlowEnabled": true,
			"redirectUris": []string{testPublicURL + resources.IdentityConsoleCallbackPath},
		}},
	})

	in := convergeInput("oldrealm")
	mustConverge(t, in)

	for _, clientID := range []string{resources.IdentityUsersClientID, resources.IdentityCLIClientID} {
		_, found, err := admin.FindClientByClientID(context.Background(), "oldrealm", clientID)
		if err != nil {
			t.Fatal(err)
		}
		if !found {
			t.Errorf("pre-existing realm did not gain client %s", clientID)
		}
	}
	assertServiceAccountRoles(t, admin, "oldrealm",
		resources.IdentityUsersClientID, resources.IdentityUsersServiceAccountRoles())

	// The CLI client's PKCE posture on the repaired realm.
	cli, _, err := admin.FindClientByClientID(context.Background(), "oldrealm", resources.IdentityCLIClientID)
	if err != nil {
		t.Fatal(err)
	}
	if cli["publicClient"] != true {
		t.Error("CLI client must be public")
	}
	attrs, _ := cli["attributes"].(map[string]any)
	if attrs[resources.IdentityPKCEMethodAttribute] != resources.IdentityPKCEMethodS256 {
		t.Error("CLI client must enforce PKCE S256")
	}
}

// The B-90 case: a deliberate front-door change re-points the console
// client's redirect URIs and post-logout URI on the next pass -- and the
// realm is idempotent again at the NEW address.
func TestConvergence_FrontDoorMoveRepointsRedirects(t *testing.T) {
	admin := authedAdmin(t)
	createRealm(t, admin, "moved", nil)
	in := convergeInput("moved")
	mustConverge(t, in)

	movedURL := "https://planton-moved.example.com"
	in.PublicURL = movedURL
	report := mustConverge(t, in)
	if report.Clean() {
		t.Fatal("a front-door move must register as drift")
	}

	console, _, err := admin.FindClientByClientID(context.Background(), "moved", resources.IdentityConsoleClientID)
	if err != nil {
		t.Fatal(err)
	}
	if !stringSetEqual(resources.IdentityConsoleRedirectURIs(movedURL), console["redirectUris"]) {
		t.Errorf("redirect URIs = %v, want the moved front door's callbacks", console["redirectUris"])
	}
	attrs, _ := console["attributes"].(map[string]any)
	if attrs[resources.IdentityPostLogoutAttribute] != movedURL+resources.IdentityConsolePostLogoutPath {
		t.Errorf("post-logout URI = %v, want the moved front door's login page", attrs[resources.IdentityPostLogoutAttribute])
	}

	if !mustConverge(t, in).Clean() {
		t.Error("realm must be idempotent again at the new address")
	}
}

// An excess admin role on an OWNED service account is a least-privilege
// drift and is revoked; the desired roles survive.
func TestConvergence_ServiceAccountExcessRoleRevoked(t *testing.T) {
	admin := authedAdmin(t)
	createRealm(t, admin, "excess", nil)
	in := convergeInput("excess")
	mustConverge(t, in)

	ctx := context.Background()
	users, _, err := admin.FindClientByClientID(ctx, "excess", resources.IdentityUsersClientID)
	if err != nil {
		t.Fatal(err)
	}
	usersUUID, _ := users["id"].(string)
	saUser, err := admin.GetServiceAccountUser(ctx, "excess", usersUUID)
	if err != nil {
		t.Fatal(err)
	}
	userID, _ := saUser["id"].(string)

	rm, _, err := admin.FindClientByClientID(ctx, "excess", resources.IdentityRealmManagementClientID)
	if err != nil {
		t.Fatal(err)
	}
	rmUUID, _ := rm["id"].(string)
	roles, err := admin.ListClientRoles(ctx, "excess", rmUUID)
	if err != nil {
		t.Fatal(err)
	}
	for _, role := range roles {
		if role["name"] == "realm-admin" {
			if err := admin.AddUserClientRoleMappings(ctx, "excess", userID, rmUUID, []Representation{role}); err != nil {
				t.Fatal(err)
			}
		}
	}

	report := mustConverge(t, in)
	if report.Clean() {
		t.Fatal("an excess realm-admin role on the owned service account must register as drift")
	}
	assertServiceAccountRoles(t, admin, "excess",
		resources.IdentityUsersClientID, resources.IdentityUsersServiceAccountRoles())
}

// assertServiceAccountRoles asserts the client's service account holds
// EXACTLY the wanted realm-management roles.
func assertServiceAccountRoles(t *testing.T, admin *AdminClient, realm, clientID string, want []string) {
	t.Helper()
	ctx := context.Background()

	client, _, err := admin.FindClientByClientID(ctx, realm, clientID)
	if err != nil {
		t.Fatal(err)
	}
	clientUUID, _ := client["id"].(string)
	saUser, err := admin.GetServiceAccountUser(ctx, realm, clientUUID)
	if err != nil {
		t.Fatal(err)
	}
	userID, _ := saUser["id"].(string)

	rm, _, err := admin.FindClientByClientID(ctx, realm, resources.IdentityRealmManagementClientID)
	if err != nil {
		t.Fatal(err)
	}
	rmUUID, _ := rm["id"].(string)

	held, err := admin.ListUserClientRoleMappings(ctx, realm, userID, rmUUID)
	if err != nil {
		t.Fatal(err)
	}
	heldNames := make(map[string]bool, len(held))
	for _, role := range held {
		if name, _ := role["name"].(string); name != "" {
			heldNames[name] = true
		}
	}
	if len(heldNames) != len(want) {
		t.Errorf("service account of %s holds %v, want exactly %v", clientID, heldNames, want)
	}
	for _, name := range want {
		if !heldNames[name] {
			t.Errorf("service account of %s missing role %s", clientID, name)
		}
	}
}

func findUserByUsername(t *testing.T, admin *AdminClient, realm, username string) Representation {
	t.Helper()
	var users []Representation
	if err := admin.do(context.Background(), http.MethodGet,
		admin.serverRoot+"/admin/realms/"+realm+"/users?exact=true&username="+username,
		nil, http.StatusOK, &users); err != nil {
		t.Fatal(err)
	}
	if len(users) != 1 {
		t.Fatalf("expected exactly one user %s, got %d", username, len(users))
	}
	return users[0]
}
