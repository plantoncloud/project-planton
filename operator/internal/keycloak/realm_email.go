package keycloak

import (
	"slices"
	"strconv"
)

// The identity server's share of the platform's one email declaration. The
// platform resource declares a mail provider once; the control plane sends
// invitations and alerts through it, and the identity server sends password
// resets through the same relay. This file translates that declaration into
// the realm's smtpServer map and the resetPasswordAllowed switch, so the
// sign-in page offers "Forgot password?" exactly when an email can actually
// be sent -- never a link that fails after the click.
//
// Keycloak stores the SMTP configuration as a flat map of strings on the
// realm (RealmRepresentation.smtpServer), including the two secret-bearing
// keys (password, authTokenClientSecret), which it masks on read. The
// convergence rules those two keys demand live in converge.go beside the
// other masked-secret handling; this file only knows the vocabulary.

// OwnedRealmEmail is what the platform's email declaration means for the
// realm. On the reconciler's nil-vs-empty contract (the federation state
// speaks the same way):
//
//   - a nil *OwnedRealmEmail on OwnedRealmInput means HANDS OFF this pass --
//     desired state could not be built (a referenced Secret is unreadable),
//     and a transient gap must never flip the sign-in page;
//   - a non-nil value with a nil Relay means the platform declares no email:
//     the realm carries no relay and password reset is off. The operator
//     owns the off-state too, so a relay typed into the identity server's
//     admin console cannot make "Forgot password?" work while the product
//     honestly says email is not configured -- what is advertised and what
//     is enforced never disagree;
//   - a Relay set means the realm carries exactly the declaration.
type OwnedRealmEmail struct {
	// Relay is the SMTP relay the identity server sends through; nil when
	// the platform declares no email.
	Relay *OwnedSMTPRelay

	// RotateCredential is set when the referenced Secret's content moved
	// since the last recorded write: the masked secret keys are written this
	// pass even when nothing diffable changed. Keycloak masks them on read,
	// so the caller's fingerprint record is the only rotation signal.
	RotateCredential bool
}

// OwnedSMTPAuth is how the identity server signs in to the relay.
type OwnedSMTPAuth string

const (
	// OwnedSMTPAuthNone: an internal smart host that admits the cluster by
	// network address; no credential is sent.
	OwnedSMTPAuthNone OwnedSMTPAuth = "none"
	// OwnedSMTPAuthPassword: a username and password (Keycloak's "basic").
	OwnedSMTPAuthPassword OwnedSMTPAuth = "password"
	// OwnedSMTPAuthToken: SASL XOAUTH2 with a token from an OAuth2
	// client-credentials grant (Keycloak's "token").
	OwnedSMTPAuthToken OwnedSMTPAuth = "token"
)

// OwnedSMTPRelay is the relay as the platform declares it, in the platform's
// vocabulary; smtpServer() translates it into Keycloak's.
type OwnedSMTPRelay struct {
	Host string
	Port int32

	// Security is the platform's word for how the connection is protected:
	// starttls, tls, or none. Keycloak has two booleans for the same idea
	// (starttls, ssl); the translation is literal and exclusive.
	Security string

	// From / FromDisplayName / ReplyTo are the sender identity every email
	// from this install carries -- the same three facts the control plane
	// sends with.
	From            string
	FromDisplayName string
	ReplyTo         string

	Auth OwnedSMTPAuth
	// User is the mailbox that signs in: the basic-auth username, or the
	// mailbox the OAuth2 token sends as.
	User string
	// Password is the basic-auth password (Auth = password).
	Password string
	// The client-credentials grant behind a token sign-in (Auth = token).
	TokenURL          string
	TokenScope        string
	TokenClientID     string
	TokenClientSecret string
}

// The realm-level keys this file owns, by their Admin API JSON names.
const (
	realmResetPasswordAllowedKey = "resetPasswordAllowed"
	realmSMTPServerKey           = "smtpServer"
)

// smtpServerOwnedKeys is the operator's vocabulary inside the smtpServer map:
// every key smtpServer() can render, in a fixed order so drift reports read
// the same way every time. A key in this list that the declaration does not
// render is REMOVED from the live map (a credential must not linger after
// the sign-in mode changes); a key outside it is admin territory.
var smtpServerOwnedKeys = []string{
	"host", "port", "from", "fromDisplayName", "replyTo", "ssl", "starttls",
	"auth", "authType", "user", "password",
	"authTokenUrl", "authTokenScope", "authTokenClientId", "authTokenClientSecret",
}

// The smtpServer map's secret-bearing keys. Keycloak masks both on read
// (**********), so they are never compared -- see convergeSMTPServer.
var smtpServerSecretKeys = []string{"password", "authTokenClientSecret"}

// smtpServer renders the realm's smtpServer map for this declaration: every
// value a string, as Keycloak stores them. The keys rendered here are the
// keys the operator owns inside the map; presentation extras an admin may set
// (envelopeFrom, replyToDisplayName, the timeouts) are unowned and ride
// through a write untouched. Empty when no relay is declared -- the owned
// off-state.
func (e *OwnedRealmEmail) smtpServer() map[string]string {
	if e.Relay == nil {
		return map[string]string{}
	}
	r := e.Relay
	m := map[string]string{
		"host":            r.Host,
		"port":            strconv.Itoa(int(r.Port)),
		"from":            r.From,
		"fromDisplayName": r.FromDisplayName,
		"replyTo":         r.ReplyTo,
		// Keycloak's two booleans for the platform's one security word.
		// Note for the reader weighing relays: Keycloak's starttls is an
		// opportunistic upgrade (it does not refuse a relay that stops
		// offering STARTTLS), where the control plane's arm requires it;
		// a relay that offers implicit TLS is the stricter declaration.
		"ssl":      strconv.FormatBool(r.Security == "tls"),
		"starttls": strconv.FormatBool(r.Security == "starttls"),
		"auth":     strconv.FormatBool(r.Auth != OwnedSMTPAuthNone),
	}
	switch r.Auth {
	case OwnedSMTPAuthPassword:
		m["authType"] = "basic"
		m["user"] = r.User
		m["password"] = r.Password
	case OwnedSMTPAuthToken:
		m["authType"] = "token"
		m["user"] = r.User
		m["authTokenUrl"] = r.TokenURL
		m["authTokenScope"] = r.TokenScope
		m["authTokenClientId"] = r.TokenClientID
		m["authTokenClientSecret"] = r.TokenClientSecret
	}
	return m
}

// isSMTPServerSecretKey reports whether key is one of the masked keys.
func isSMTPServerSecretKey(key string) bool {
	return slices.Contains(smtpServerSecretKeys, key)
}
