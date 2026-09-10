package keycloak

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"maps"
	"net/http"

	"github.com/plantonhq/planton/operator/internal/resources"
)

// ConvergeInput carries everything one convergence pass needs.
type ConvergeInput struct {
	OwnedRealmInput

	// Federation is what the bound PlantonIdentityProvider declares; nil
	// means none is bound and the sweep removes every operator-named
	// federation object (the manifest-deletion path).
	Federation *OwnedFederation

	// ServerRoot is the identity server's in-cluster base URL including the
	// serving path prefix (resources.IdentityInternalServerRootURL).
	ServerRoot string

	// AdminUsername/AdminPassword are the master-realm bootstrap admin
	// credentials the operator provisions at first boot.
	AdminUsername string
	AdminPassword string

	// HTTPClient is injected by the caller (the operator's thin-client
	// idiom).
	HTTPClient *http.Client
}

// Report is what one convergence pass did, in the grant-reconciler idiom:
// nothing to say when clean, one plain sentence per repair when it repaired.
// Writes counts every mutating call -- a converged realm reports zero, which
// is the idempotency property the convergence suite asserts.
type Report struct {
	Repairs []string
	Writes  int
}

func (r *Report) repaired(format string, args ...any) {
	r.Repairs = append(r.Repairs, fmt.Sprintf(format, args...))
	r.Writes++
}

// Clean reports whether the pass found the realm already converged.
func (r *Report) Clean() bool { return r.Writes == 0 }

// Converge drives the live realm to the owned set: realm settings, each
// owned client with its mappers and service-account roles, then the
// federation state a bound identity provider declares (or its removal).
// Reads everything, writes only what disagrees. Admin-created state is
// structurally out of reach -- every loop iterates the owned enumeration,
// never the realm's inventory, so an object the operator does not own is
// never even diffed.
func Converge(ctx context.Context, in ConvergeInput) (*Report, error) {
	admin := NewAdminClient(in.HTTPClient, in.ServerRoot)
	if err := admin.Authenticate(ctx, in.AdminUsername, in.AdminPassword); err != nil {
		return nil, fmt.Errorf("authenticating to the admin API: %w", err)
	}

	report := &Report{}

	// The realm is fetched ONCE: the settings phase mutates owned keys on
	// this representation, and the federation phase needs its internal id
	// (the components API's parent).
	liveRealm, err := admin.GetRealm(ctx, in.Realm)
	if err != nil {
		return report, err
	}

	if err := convergeRealmSettings(ctx, admin, in.Realm, liveRealm, in.OwnedRealmInput, report); err != nil {
		return report, err
	}

	for _, owned := range OwnedClients(in.OwnedRealmInput) {
		if err := convergeClient(ctx, admin, in.Realm, owned, report); err != nil {
			return report, err
		}
	}

	realmID, _ := liveRealm["id"].(string)
	if realmID == "" {
		return report, fmt.Errorf("realm %s representation carries no id", in.Realm)
	}
	if err := convergeFederation(ctx, admin, in.Realm, realmID, in.Federation, report); err != nil {
		return report, err
	}

	return report, nil
}

// convergeRealmSettings read-modify-writes the owned realm-level fields on
// the already-fetched live representation: owned keys are corrected in
// place, and everything else rides back untouched. Zero writes when nothing
// disagrees. The flat keys compare whole; smtpServer converges by key under
// its own masked-secret rule.
func convergeRealmSettings(ctx context.Context, admin *AdminClient, realm string, live Representation, in OwnedRealmInput, report *Report) error {
	var drifted []string
	for key, want := range OwnedRealmSettings(in) {
		if key == realmSMTPServerKey {
			continue
		}
		if !jsonEqual(want, live[key]) {
			live[key] = want
			drifted = append(drifted, key)
		}
	}
	if in.Email != nil {
		drifted = append(drifted, convergeSMTPServer(live, in.Email)...)
	}
	if len(drifted) == 0 {
		return nil
	}

	if err := admin.UpdateRealm(ctx, realm, live); err != nil {
		return err
	}
	report.repaired("realm settings corrected: %v", drifted)
	return nil
}

// convergeSMTPServer corrects the realm's smtpServer map in place and returns
// the drift paths (keys only -- a value never enters a report). Two facts
// about Keycloak shape the rule:
//
//   - the password and the OAuth2 client secret are masked on read, so they
//     are never compared; they are written when the caller says the
//     referenced Secret rotated (the fingerprint record is the only signal);
//   - on a realm update, Keycloak keeps a masked secret ONLY when the relay's
//     destination (host, port, ssl, starttls, from, user, the token facts) is
//     unchanged; a masked secret arriving with a moved destination is
//     dropped. So every write that touches this map carries the real secret
//     values, never the mask -- "any drift writes the secret with it", not
//     "rotation only".
//
// Declared: every key in the owned vocabulary (smtpServerOwnedKeys) is
// corrected by key -- set when the declaration renders it, removed when it
// does not (a password left behind after a switch to token sign-in is a
// lingering credential, not admin territory) -- and unowned extras ride
// through. Not declared: the map is cleared whole, the owned off-state.
func convergeSMTPServer(live Representation, email *OwnedRealmEmail) []string {
	liveSMTP, _ := live[realmSMTPServerKey].(map[string]any)
	if liveSMTP == nil {
		liveSMTP = map[string]any{}
	}

	if email.Relay == nil {
		if len(liveSMTP) == 0 {
			return nil
		}
		live[realmSMTPServerKey] = map[string]string{}
		return []string{realmSMTPServerKey + " (no email is declared; the relay is removed)"}
	}

	want := email.smtpServer()
	var drifted []string
	for _, key := range smtpServerOwnedKeys {
		value, wanted := want[key]
		got, present := liveSMTP[key]
		path := realmSMTPServerKey + "." + key
		switch {
		case wanted && !present:
			drifted = append(drifted, path)
		case wanted && !isSMTPServerSecretKey(key):
			if gotString, _ := got.(string); gotString != value {
				drifted = append(drifted, path)
			}
		case wanted && email.RotateCredential:
			// A secret key: masked on read, written on rotation.
			drifted = append(drifted, path+" (rotated)")
		case !wanted && present:
			drifted = append(drifted, path+" (removed)")
		}
	}
	if len(drifted) == 0 {
		return nil
	}

	merged := make(map[string]any, len(liveSMTP)+len(want))
	maps.Copy(merged, liveSMTP)
	for _, key := range smtpServerOwnedKeys {
		delete(merged, key)
	}
	for key, value := range want {
		merged[key] = value
	}
	live[realmSMTPServerKey] = merged
	return drifted
}

// convergeClient ensures one owned client exists and its owned fields match:
// creates it whole when missing (the B-21 repair), otherwise corrects
// top-level fields, redirect URIs, owned attribute keys, and the secret in a
// single write, then converges mappers and service-account roles through
// their own endpoints.
func convergeClient(ctx context.Context, admin *AdminClient, realm string, owned OwnedClient, report *Report) error {
	live, found, err := admin.FindClientByClientID(ctx, realm, owned.ClientID)
	if err != nil {
		return err
	}

	if !found {
		if err := admin.CreateClient(ctx, realm, owned.createRepresentation()); err != nil {
			return err
		}
		report.repaired("client %s was missing and has been created", owned.ClientID)
		// Re-fetch for the server UUID the role-mapping step needs.
		live, found, err = admin.FindClientByClientID(ctx, realm, owned.ClientID)
		if err != nil {
			return err
		}
		if !found {
			return fmt.Errorf("client %s not found immediately after creation", owned.ClientID)
		}
	}

	clientUUID, _ := live["id"].(string)
	if clientUUID == "" {
		return fmt.Errorf("client %s representation carries no id", owned.ClientID)
	}

	if err := convergeClientFields(ctx, admin, realm, owned, live, clientUUID, report); err != nil {
		return err
	}
	if err := convergeMappers(ctx, admin, realm, owned, clientUUID, report); err != nil {
		return err
	}
	if len(owned.ServiceAccountRoles) > 0 {
		if err := convergeServiceAccountRoles(ctx, admin, realm, owned, clientUUID, report); err != nil {
			return err
		}
	}
	return nil
}

func convergeClientFields(ctx context.Context, admin *AdminClient, realm string, owned OwnedClient, live Representation, clientUUID string, report *Report) error {
	var drifted []string

	for key, want := range owned.Fields {
		if !jsonEqual(want, live[key]) {
			live[key] = want
			drifted = append(drifted, key)
		}
	}

	// Keycloak stores redirect URIs as an unordered SET and returns them in
	// its own order -- an order-sensitive diff here writes every pass,
	// forever (caught live by the convergence suite's zero-writes
	// assertion). Compare as sets; write the desired list only on real
	// membership drift.
	if !stringSetEqual(owned.RedirectURIs, live["redirectUris"]) {
		live["redirectUris"] = owned.RedirectURIs
		drifted = append(drifted, "redirectUris")
	}

	// Attributes merge BY KEY: only keys the operator names are corrected;
	// an admin-added attribute on this client survives untouched.
	liveAttrs, _ := live["attributes"].(map[string]any)
	if liveAttrs == nil {
		liveAttrs = map[string]any{}
	}
	for key, want := range owned.Attributes {
		if got, _ := liveAttrs[key].(string); got != want {
			liveAttrs[key] = want
			drifted = append(drifted, "attributes."+key)
		}
	}
	live["attributes"] = liveAttrs

	// The secret is compared through its dedicated read endpoint (client
	// representations mask it) and set through the representation write.
	if owned.Secret != "" {
		liveSecret, err := admin.GetClientSecret(ctx, realm, clientUUID)
		if err != nil {
			return err
		}
		if liveSecret != owned.Secret {
			live["secret"] = owned.Secret
			drifted = append(drifted, "secret")
		}
	}

	if len(drifted) == 0 {
		return nil
	}
	if err := admin.UpdateClient(ctx, realm, clientUUID, live); err != nil {
		return err
	}
	report.repaired("client %s corrected: %v", owned.ClientID, drifted)
	return nil
}

func convergeMappers(ctx context.Context, admin *AdminClient, realm string, owned OwnedClient, clientUUID string, report *Report) error {
	if len(owned.Mappers) == 0 {
		return nil
	}

	liveMappers, err := admin.ListProtocolMappers(ctx, realm, clientUUID)
	if err != nil {
		return err
	}
	byName := make(map[string]Representation, len(liveMappers))
	for _, m := range liveMappers {
		if name, _ := m["name"].(string); name != "" {
			byName[name] = m
		}
	}

	for _, want := range owned.Mappers {
		live, ok := byName[want.Name]
		if !ok {
			rep := Representation{
				"name":            want.Name,
				"protocol":        "openid-connect",
				"protocolMapper":  want.ProtocolMapper,
				"consentRequired": false,
				"config":          want.Config,
			}
			if err := admin.CreateProtocolMapper(ctx, realm, clientUUID, rep); err != nil {
				return err
			}
			report.repaired("client %s gained missing protocol mapper %s", owned.ClientID, want.Name)
			continue
		}

		var drifted []string
		if !jsonEqual(want.ProtocolMapper, live["protocolMapper"]) {
			live["protocolMapper"] = want.ProtocolMapper
			drifted = append(drifted, "protocolMapper")
		}
		liveConfig, _ := live["config"].(map[string]any)
		if liveConfig == nil {
			liveConfig = map[string]any{}
		}
		for key, cfgWant := range want.Config {
			if got, _ := liveConfig[key].(string); got != cfgWant {
				liveConfig[key] = cfgWant
				drifted = append(drifted, "config."+key)
			}
		}
		live["config"] = liveConfig

		if len(drifted) == 0 {
			continue
		}
		mapperID, _ := live["id"].(string)
		if err := admin.UpdateProtocolMapper(ctx, realm, clientUUID, mapperID, live); err != nil {
			return err
		}
		report.repaired("client %s protocol mapper %s corrected: %v", owned.ClientID, want.Name, drifted)
	}
	return nil
}

// convergeServiceAccountRoles makes the owned client's service account hold
// EXACTLY the desired realm-management roles. Extra realm-management roles
// are revoked -- on an OWNED service account a widened admin role is a
// least-privilege drift, not an admin choice. Role mappings on other clients
// are never read or touched.
func convergeServiceAccountRoles(ctx context.Context, admin *AdminClient, realm string, owned OwnedClient, clientUUID string, report *Report) error {
	saUser, err := admin.GetServiceAccountUser(ctx, realm, clientUUID)
	if err != nil {
		return err
	}
	userID, _ := saUser["id"].(string)
	if userID == "" {
		return fmt.Errorf("service account of client %s carries no id", owned.ClientID)
	}

	rmClient, found, err := admin.FindClientByClientID(ctx, realm, resources.IdentityRealmManagementClientID)
	if err != nil {
		return err
	}
	if !found {
		return fmt.Errorf("built-in client %s not found in realm %s", resources.IdentityRealmManagementClientID, realm)
	}
	rmUUID, _ := rmClient["id"].(string)

	held, err := admin.ListUserClientRoleMappings(ctx, realm, userID, rmUUID)
	if err != nil {
		return err
	}
	heldByName := make(map[string]Representation, len(held))
	for _, role := range held {
		if name, _ := role["name"].(string); name != "" {
			heldByName[name] = role
		}
	}

	desired := make(map[string]bool, len(owned.ServiceAccountRoles))
	for _, name := range owned.ServiceAccountRoles {
		desired[name] = true
	}

	var missing []string
	for name := range desired {
		if _, ok := heldByName[name]; !ok {
			missing = append(missing, name)
		}
	}
	var extra []Representation
	var extraNames []string
	for name, role := range heldByName {
		if !desired[name] {
			extra = append(extra, role)
			extraNames = append(extraNames, name)
		}
	}

	if len(missing) > 0 {
		available, err := admin.ListClientRoles(ctx, realm, rmUUID)
		if err != nil {
			return err
		}
		availableByName := make(map[string]Representation, len(available))
		for _, role := range available {
			if name, _ := role["name"].(string); name != "" {
				availableByName[name] = role
			}
		}
		grant := make([]Representation, 0, len(missing))
		for _, name := range missing {
			role, ok := availableByName[name]
			if !ok {
				return fmt.Errorf("role %s not defined on %s", name, resources.IdentityRealmManagementClientID)
			}
			grant = append(grant, role)
		}
		if err := admin.AddUserClientRoleMappings(ctx, realm, userID, rmUUID, grant); err != nil {
			return err
		}
		report.repaired("client %s service account granted missing roles: %v", owned.ClientID, missing)
	}

	if len(extra) > 0 {
		if err := admin.RemoveUserClientRoleMappings(ctx, realm, userID, rmUUID, extra); err != nil {
			return err
		}
		report.repaired("client %s service account revoked excess roles: %v", owned.ClientID, extraNames)
	}
	return nil
}

// stringSetEqual compares a desired string list against a live JSON-decoded
// value as SETS -- for fields Keycloak stores unordered (redirect URIs).
func stringSetEqual(want []string, got any) bool {
	gotList, ok := got.([]any)
	if !ok {
		return got == nil && len(want) == 0
	}
	if len(gotList) != len(want) {
		return false
	}
	wantSet := make(map[string]int, len(want))
	for _, w := range want {
		wantSet[w]++
	}
	for _, g := range gotList {
		s, ok := g.(string)
		if !ok || wantSet[s] == 0 {
			return false
		}
		wantSet[s]--
	}
	return true
}

// jsonEqual compares a desired value against a live JSON-decoded value by
// normalizing both through JSON encoding: int vs float64, []string vs []any,
// and map[string]string vs map[string]any all compare by VALUE, which is the
// only comparison that means anything for a wire representation.
func jsonEqual(want, got any) bool {
	wantJSON, err := json.Marshal(want)
	if err != nil {
		return false
	}
	gotJSON, err := json.Marshal(got)
	if err != nil {
		return false
	}
	return bytes.Equal(wantJSON, gotJSON)
}
