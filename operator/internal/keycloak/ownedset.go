package keycloak

import (
	"maps"

	"github.com/plantonhq/planton/operator/internal/resources"
)

// This file is the ONE enumeration of realm state the operator owns -- the
// whitelist the never-clobber contract stands on. The reconciler converges
// exactly what is listed here and nothing else; everything unlisted is admin
// territory by default: users, groups, role assignments, any client or
// mapper the operator did not create, themes, and every realm setting outside
// OwnedRealmSettings. The realm import (internal/resources) derives from the
// same shared constants, so the import and this owned set cannot drift apart
// (pinned by TestIdentityRealmImportAgreesWithOwnedSet).

// OwnedRealmInput carries the live inputs the owned set is derived from --
// the same values the realm import renders from.
type OwnedRealmInput struct {
	Realm string

	// PublicURL is the platform's CURRENT front door. Redirect URIs and the
	// post-logout URI derive from it every pass, which is what keeps a
	// deliberate hostname change from stranding sign-in.
	PublicURL string

	// ConsoleClientSecret / UsersClientSecret are the operator-managed
	// credentials (from the Kubernetes Secrets) the confidential clients
	// must carry.
	ConsoleClientSecret string
	UsersClientSecret   string

	// Email is the identity server's share of the platform's email
	// declaration (see realm_email.go): nil hands off the realm's email
	// settings this pass, a value with no relay owns them off, a relay owns
	// them as declared.
	Email *OwnedRealmEmail
}

// OwnedRealmSettings returns the realm-level fields the operator owns, keyed
// by their Admin API JSON names, derived from the live input the way the
// owned clients are. Deliberately NOT here: loginTheme (the server-wide
// --spi-theme--default flag covers it, and realm theming beyond that is an
// admin customization), displayName/displayNameHtml (branding an admin may
// legitimately adjust).
//
// Flat keys are compared whole. The one nested key, smtpServer, is a map
// Keycloak masks two entries of on read, so the reconciler converges it by
// key with its own rule (convergeSMTPServer); it is listed here so the
// import-agreement pin sees every owned key in one place.
func OwnedRealmSettings(in OwnedRealmInput) map[string]any {
	settings := map[string]any{
		// The platform's security posture (DD'd product numbers, not tuning
		// knobs -- see the constants' comments in internal/resources).
		"accessTokenLifespan":   resources.IdentityAccessTokenLifespanSeconds,
		"ssoSessionIdleTimeout": resources.IdentitySSOSessionIdleSeconds,
		// The realm is the access boundary: self-registration stays off.
		"registrationAllowed": false,
		// The friction ladder's first step serves plain HTTP; the ingress
		// owns transport security (see the realm import's comment).
		"sslRequired": "none",
	}
	if in.Email != nil {
		// "Forgot password?" is offered exactly when an email can be sent:
		// the switch and the relay are one declaration, owned together.
		settings[realmResetPasswordAllowedKey] = in.Email.Relay != nil
		settings[realmSMTPServerKey] = in.Email.smtpServer()
	}
	return settings
}

// OwnedMapper is a protocol mapper the operator owns on one of its clients,
// matched by name.
type OwnedMapper struct {
	Name string
	// ProtocolMapper is the Keycloak mapper type (e.g. oidc-audience-mapper).
	ProtocolMapper string
	// Config keys are converged individually (merged), so a config key the
	// operator has no opinion on survives.
	Config map[string]string
}

// OwnedClient is one of the operator's realm clients: what must exist, and
// which of its fields the reconciler corrects when drifted.
type OwnedClient struct {
	ClientID string
	Name     string

	// Fields are the owned top-level representation fields, by JSON name.
	// The flags that decide what kind of client this IS (public vs
	// confidential, which flows) are owned; a hand-flip of any of them
	// breaks sign-in and is a repair case, not an admin choice.
	Fields map[string]any

	// RedirectURIs is owned whole (exact URIs are the security property).
	RedirectURIs []string

	// Attributes are owned BY KEY and merged -- an attribute key the
	// operator does not name survives untouched on the live client.
	Attributes map[string]string

	// Secret is the desired client secret ("" for public clients). Owned:
	// the operator-managed Kubernetes Secret is the source of truth, and a
	// realm-side regeneration would break the consumers reading that Secret.
	Secret string

	// Mappers converge through the dedicated mapper endpoints (Keycloak
	// ignores protocolMappers on client update).
	Mappers []OwnedMapper

	// ServiceAccountRoles are realm-management roles the client's service
	// account must hold EXACTLY: missing roles are granted, extra
	// realm-management roles are revoked (least-privilege is the posture on
	// an owned service account). Roles on OTHER clients are not touched.
	ServiceAccountRoles []string

	// DefaultClientScopes / OptionalClientScopes are set at CREATE only.
	// Keycloak ignores these lists on client update (scope membership has
	// its own endpoints), and DD'd ownership does not extend to them: a
	// scope drift on an existing client falls to the recreate path.
	DefaultClientScopes  []string
	OptionalClientScopes []string
}

// OwnedClients returns the three Planton clients the realm must carry. These
// are owned by clientId (the enumeration IS the whitelist -- realms imported
// before the managed-mark existed still converge), and every representation
// the reconciler creates also carries the managed mark for visibility.
func OwnedClients(in OwnedRealmInput) []OwnedClient {
	confidential := map[string]any{
		"enabled":                   true,
		"protocol":                  "openid-connect",
		"publicClient":              false,
		"clientAuthenticatorType":   "client-secret",
		"implicitFlowEnabled":       false,
		"directAccessGrantsEnabled": false,
	}

	consoleFields := map[string]any{}
	maps.Copy(consoleFields, confidential)
	consoleFields["standardFlowEnabled"] = true
	consoleFields["serviceAccountsEnabled"] = false

	usersFields := map[string]any{}
	maps.Copy(usersFields, confidential)
	usersFields["standardFlowEnabled"] = false
	usersFields["serviceAccountsEnabled"] = true

	cliFields := map[string]any{
		"enabled":                   true,
		"protocol":                  "openid-connect",
		"publicClient":              true,
		"standardFlowEnabled":       true,
		"implicitFlowEnabled":       false,
		"directAccessGrantsEnabled": false,
		"serviceAccountsEnabled":    false,
	}

	audienceMapper := OwnedMapper{
		Name:           resources.IdentityAudienceMapperName,
		ProtocolMapper: "oidc-audience-mapper",
		Config:         resources.IdentityAudienceProtocolMapperConfig(),
	}

	return []OwnedClient{{
		ClientID:     resources.IdentityConsoleClientID,
		Name:         "Planton Console",
		Fields:       consoleFields,
		RedirectURIs: resources.IdentityConsoleRedirectURIs(in.PublicURL),
		Attributes: map[string]string{
			resources.IdentityPostLogoutAttribute: in.PublicURL + resources.IdentityConsolePostLogoutPath,
			resources.IdentityManagedAttribute:    "true",
		},
		Secret:               in.ConsoleClientSecret,
		Mappers:              []OwnedMapper{audienceMapper},
		DefaultClientScopes:  []string{"acr", "basic", "email", "profile", "roles", "web-origins"},
		OptionalClientScopes: []string{"offline_access"},
	}, {
		ClientID:     resources.IdentityUsersClientID,
		Name:         "Planton User Directory",
		Fields:       usersFields,
		RedirectURIs: []string{},
		Attributes: map[string]string{
			resources.IdentityManagedAttribute: "true",
		},
		Secret:               in.UsersClientSecret,
		Mappers:              []OwnedMapper{},
		ServiceAccountRoles:  resources.IdentityUsersServiceAccountRoles(),
		DefaultClientScopes:  []string{"basic", "roles"},
		OptionalClientScopes: []string{},
	}, {
		ClientID:     resources.IdentityCLIClientID,
		Name:         "Planton CLI",
		Fields:       cliFields,
		RedirectURIs: resources.IdentityCLIRedirectURIs(),
		Attributes: map[string]string{
			resources.IdentityPKCEMethodAttribute: resources.IdentityPKCEMethodS256,
			resources.IdentityManagedAttribute:    "true",
		},
		Mappers:              []OwnedMapper{audienceMapper},
		DefaultClientScopes:  []string{"acr", "basic", "email", "profile", "roles", "web-origins", "offline_access"},
		OptionalClientScopes: []string{},
	}}
}

// createRepresentation renders the full Admin API representation for creating
// this client from scratch (the missing-client repair and the fresh-realm
// case). Only creation sends scopes and mappers inline -- Keycloak honors
// them on POST.
func (oc OwnedClient) createRepresentation() Representation {
	rep := Representation{
		"clientId":             oc.ClientID,
		"name":                 oc.Name,
		"redirectUris":         oc.RedirectURIs,
		"attributes":           oc.Attributes,
		"defaultClientScopes":  oc.DefaultClientScopes,
		"optionalClientScopes": oc.OptionalClientScopes,
	}
	maps.Copy(rep, oc.Fields)
	if oc.Secret != "" {
		rep["secret"] = oc.Secret
	}
	mappers := make([]Representation, 0, len(oc.Mappers))
	for _, m := range oc.Mappers {
		mappers = append(mappers, Representation{
			"name":            m.Name,
			"protocol":        "openid-connect",
			"protocolMapper":  m.ProtocolMapper,
			"consentRequired": false,
			"config":          m.Config,
		})
	}
	rep["protocolMappers"] = mappers
	return rep
}
