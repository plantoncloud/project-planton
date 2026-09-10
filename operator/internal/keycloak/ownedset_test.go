package keycloak

import (
	"encoding/json"
	"testing"

	"github.com/plantonhq/planton/operator/internal/resources"
)

// testInput is the owned set's input for an install that declares no email --
// the shape the realm import renders for (the import bakes the email
// off-state and never a declaration, see IdentityRealmImport), so the
// agreement pin below checks both new owned keys against it.
func testInput() OwnedRealmInput {
	return OwnedRealmInput{
		Realm:               "planton",
		PublicURL:           "https://planton.example.com",
		ConsoleClientSecret: "console-secret",
		UsersClientSecret:   "users-secret",
		Email:               &OwnedRealmEmail{},
	}
}

// TestIdentityRealmImportAgreesWithOwnedSet mechanically pins DD'd law: the
// first-boot realm import and the reconciler's owned set derive from the same
// constants and cannot drift apart. It renders the real import JSON and
// asserts every owned client and every owned realm setting appears there with
// exactly the owned values -- a divergence in either direction fails here
// before it can ship.
func TestIdentityRealmImportAgreesWithOwnedSet(t *testing.T) {
	in := testInput()

	data, err := resources.IdentityRealmImport(resources.IdentityRealmImportConfig{
		Realm:        in.Realm,
		PublicURL:    in.PublicURL,
		ClientSecret: in.ConsoleClientSecret,
		UsersSecret:  in.UsersClientSecret,
	})
	if err != nil {
		t.Fatal(err)
	}
	var imported map[string]any
	if err := json.Unmarshal(data, &imported); err != nil {
		t.Fatal(err)
	}

	// Owned realm settings: the import must bake exactly the values the
	// reconciler converges -- including the email off-state (resetPasswordAllowed
	// false, an empty smtpServer), which is the only email posture the import
	// ever carries: a fresh realm's import is create-only and rides a Secret
	// whose content hash rolls the pod, so a relay password there would roll
	// the identity server on every rotation for nothing. The reconciler owns
	// the declaration itself, seconds after first boot.
	for key, want := range OwnedRealmSettings(in) {
		if !jsonEqual(want, imported[key]) {
			t.Errorf("realm setting %s: import bakes %v, owned set converges %v", key, imported[key], want)
		}
	}
	for _, key := range []string{realmResetPasswordAllowedKey, realmSMTPServerKey} {
		if _, ok := imported[key]; !ok {
			t.Errorf("the import must bake the email off-state key %s", key)
		}
	}

	importedClients := map[string]map[string]any{}
	for _, c := range imported["clients"].([]any) {
		client := c.(map[string]any)
		importedClients[client["clientId"].(string)] = client
	}

	owned := OwnedClients(in)
	if len(owned) != len(importedClients) {
		t.Fatalf("import carries %d clients, owned set enumerates %d -- the two derivations diverged",
			len(importedClients), len(owned))
	}

	for _, oc := range owned {
		client, ok := importedClients[oc.ClientID]
		if !ok {
			t.Errorf("owned client %s missing from the realm import", oc.ClientID)
			continue
		}
		for key, want := range oc.Fields {
			if !jsonEqual(want, client[key]) {
				t.Errorf("client %s field %s: import bakes %v, owned set converges %v",
					oc.ClientID, key, client[key], want)
			}
		}
		if !jsonEqual(oc.RedirectURIs, client["redirectUris"]) {
			t.Errorf("client %s redirectUris: import bakes %v, owned set converges %v",
				oc.ClientID, client["redirectUris"], oc.RedirectURIs)
		}
		attrs, _ := client["attributes"].(map[string]any)
		for key, want := range oc.Attributes {
			if got, _ := attrs[key].(string); got != want {
				t.Errorf("client %s attribute %s: import bakes %q, owned set converges %q",
					oc.ClientID, key, got, want)
			}
		}

		importedMappers := map[string]map[string]any{}
		if raw, ok := client["protocolMappers"].([]any); ok {
			for _, m := range raw {
				mapper := m.(map[string]any)
				importedMappers[mapper["name"].(string)] = mapper
			}
		}
		if len(importedMappers) != len(oc.Mappers) {
			t.Errorf("client %s: import bakes %d mappers, owned set converges %d",
				oc.ClientID, len(importedMappers), len(oc.Mappers))
		}
		for _, om := range oc.Mappers {
			mapper, ok := importedMappers[om.Name]
			if !ok {
				t.Errorf("client %s: owned mapper %s missing from the import", oc.ClientID, om.Name)
				continue
			}
			if !jsonEqual(om.ProtocolMapper, mapper["protocolMapper"]) {
				t.Errorf("client %s mapper %s type: import %v, owned %v",
					oc.ClientID, om.Name, mapper["protocolMapper"], om.ProtocolMapper)
			}
			if !jsonEqual(om.Config, mapper["config"]) {
				t.Errorf("client %s mapper %s config: import %v, owned %v",
					oc.ClientID, om.Name, mapper["config"], om.Config)
			}
		}
	}

	// The users client's service-account roles must agree with the import's
	// clientRoles entry.
	var importedRoles []any
	for _, u := range imported["users"].([]any) {
		user := u.(map[string]any)
		if user["serviceAccountClientId"] == resources.IdentityUsersClientID {
			importedRoles = user["clientRoles"].(map[string]any)[resources.IdentityRealmManagementClientID].([]any)
		}
	}
	for _, oc := range owned {
		if oc.ClientID != resources.IdentityUsersClientID {
			continue
		}
		if !jsonEqual(oc.ServiceAccountRoles, importedRoles) {
			t.Errorf("users service-account roles: import bakes %v, owned set converges %v",
				importedRoles, oc.ServiceAccountRoles)
		}
	}
}

// The whitelist property in miniature: the owned enumeration names exactly
// the three Planton clients, and the redirect URIs derive from the CURRENT
// front door (the property that heals a deliberate hostname change).
func TestOwnedClients(t *testing.T) {
	in := testInput()
	owned := OwnedClients(in)

	ids := make([]string, 0, len(owned))
	for _, oc := range owned {
		ids = append(ids, oc.ClientID)
	}
	want := []string{
		resources.IdentityConsoleClientID,
		resources.IdentityUsersClientID,
		resources.IdentityCLIClientID,
	}
	if !jsonEqual(want, ids) {
		t.Fatalf("owned clients = %v, want %v", ids, want)
	}

	for _, oc := range owned {
		if oc.Attributes[resources.IdentityManagedAttribute] != "true" {
			t.Errorf("client %s must carry the managed mark", oc.ClientID)
		}
	}

	// B-90's property: a different front door means different desired
	// redirect URIs on the console client -- the reconciler re-points them.
	moved := in
	moved.PublicURL = "https://moved.example.com"
	for i, oc := range OwnedClients(moved) {
		if oc.ClientID != resources.IdentityConsoleClientID {
			continue
		}
		if jsonEqual(owned[i].RedirectURIs, oc.RedirectURIs) {
			t.Error("console redirect URIs must derive from the CURRENT front door")
		}
		if oc.RedirectURIs[0] != "https://moved.example.com"+resources.IdentityConsoleCallbackPath {
			t.Errorf("console redirect URI = %s, want the moved front door's callback", oc.RedirectURIs[0])
		}
	}
}

// jsonEqual is the diff primitive the whole convergence stands on; its
// number/slice/map normalization is what keeps a converged realm at zero
// writes (a false diff would write every pass -- the anti-idempotency bug).
func TestJSONEqual(t *testing.T) {
	cases := []struct {
		name string
		want any
		got  any
		eq   bool
	}{
		{"int vs decoded float64", 900, float64(900), true},
		{"int vs different float64", 900, float64(901), false},
		{"[]string vs decoded []any", []string{"a", "b"}, []any{"a", "b"}, true},
		{"slice order matters", []string{"a", "b"}, []any{"b", "a"}, false},
		{"map[string]string vs decoded map", map[string]string{"k": "v"}, map[string]any{"k": "v"}, true},
		{"bool", false, false, true},
		{"desired false vs live missing", false, nil, false},
	}
	for _, tc := range cases {
		if got := jsonEqual(tc.want, tc.got); got != tc.eq {
			t.Errorf("%s: jsonEqual = %v, want %v", tc.name, got, tc.eq)
		}
	}
}

// stringSetEqual guards the idempotency of set-stored fields: Keycloak
// returns redirect URIs in ITS order, and an order-sensitive diff would
// write every pass (the bug the convergence suite caught live).
func TestStringSetEqual(t *testing.T) {
	cases := []struct {
		name string
		want []string
		got  any
		eq   bool
	}{
		{"same order", []string{"a", "b"}, []any{"a", "b"}, true},
		{"reordered", []string{"a", "b"}, []any{"b", "a"}, true},
		{"different member", []string{"a", "b"}, []any{"a", "c"}, false},
		{"missing member", []string{"a", "b"}, []any{"a"}, false},
		{"duplicate respected", []string{"a", "a"}, []any{"a", "b"}, false},
		{"empty vs nil", []string{}, nil, true},
		{"non-empty vs nil", []string{"a"}, nil, false},
	}
	for _, tc := range cases {
		if got := stringSetEqual(tc.want, tc.got); got != tc.eq {
			t.Errorf("%s: stringSetEqual = %v, want %v", tc.name, got, tc.eq)
		}
	}
}

// createRepresentation must render a complete client (create is the one
// place scopes and mappers go inline) and must omit the secret key entirely
// for public clients.
func TestCreateRepresentation(t *testing.T) {
	for _, oc := range OwnedClients(testInput()) {
		rep := oc.createRepresentation()
		if rep["clientId"] != oc.ClientID {
			t.Errorf("clientId = %v, want %s", rep["clientId"], oc.ClientID)
		}
		if _, ok := rep["protocolMappers"]; !ok {
			t.Errorf("client %s: create representation must carry mappers inline", oc.ClientID)
		}
		if _, ok := rep["defaultClientScopes"]; !ok {
			t.Errorf("client %s: create representation must carry scopes inline", oc.ClientID)
		}
		_, hasSecret := rep["secret"]
		if oc.Secret == "" && hasSecret {
			t.Errorf("client %s: public client must not carry a secret key", oc.ClientID)
		}
		if oc.Secret != "" && !hasSecret {
			t.Errorf("client %s: confidential client must carry its secret", oc.ClientID)
		}
	}
}
