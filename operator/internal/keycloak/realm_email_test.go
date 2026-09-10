package keycloak

import (
	"maps"
	"slices"
	"strings"
	"testing"
)

func passwordRelay() *OwnedSMTPRelay {
	return &OwnedSMTPRelay{
		Host:            "smtp.office365.com",
		Port:            587,
		Security:        "starttls",
		From:            "no-reply@planton.acme.com",
		FromDisplayName: "Planton",
		ReplyTo:         "it-help@acme.com",
		Auth:            OwnedSMTPAuthPassword,
		User:            "planton@acme.com",
		Password:        "relay-secret",
	}
}

func tokenRelay() *OwnedSMTPRelay {
	r := passwordRelay()
	r.Auth = OwnedSMTPAuthToken
	r.Password = ""
	r.TokenURL = "https://login.microsoftonline.com/tenant/oauth2/v2.0/token"
	r.TokenScope = "https://outlook.office365.com/.default"
	r.TokenClientID = "client-id"
	r.TokenClientSecret = "client-secret"
	return r
}

// The translation from the platform's vocabulary to Keycloak's: one security
// word becomes two exclusive booleans, one sign-in mode becomes auth +
// authType + exactly the credential keys that mode uses, every value a
// string as Keycloak stores them.
func TestSMTPServerRendering(t *testing.T) {
	cases := []struct {
		name  string
		relay *OwnedSMTPRelay
		want  map[string]string
	}{
		{
			name:  "password over a required STARTTLS upgrade",
			relay: passwordRelay(),
			want: map[string]string{
				"host": "smtp.office365.com", "port": "587",
				"from": "no-reply@planton.acme.com", "fromDisplayName": "Planton", "replyTo": "it-help@acme.com",
				"ssl": "false", "starttls": "true",
				"auth": "true", "authType": "basic", "user": "planton@acme.com", "password": "relay-secret",
			},
		},
		{
			name:  "token over implicit TLS",
			relay: func() *OwnedSMTPRelay { r := tokenRelay(); r.Security = "tls"; r.Port = 465; return r }(),
			want: map[string]string{
				"host": "smtp.office365.com", "port": "465",
				"from": "no-reply@planton.acme.com", "fromDisplayName": "Planton", "replyTo": "it-help@acme.com",
				"ssl": "true", "starttls": "false",
				"auth": "true", "authType": "token", "user": "planton@acme.com",
				"authTokenUrl":      "https://login.microsoftonline.com/tenant/oauth2/v2.0/token",
				"authTokenScope":    "https://outlook.office365.com/.default",
				"authTokenClientId": "client-id", "authTokenClientSecret": "client-secret",
			},
		},
		{
			name: "no credential, plaintext internal relay",
			relay: &OwnedSMTPRelay{Host: "smtp-relay.corp.acme.com", Port: 25, Security: "none",
				From: "planton@acme.com", FromDisplayName: "Planton", Auth: OwnedSMTPAuthNone},
			want: map[string]string{
				"host": "smtp-relay.corp.acme.com", "port": "25",
				"from": "planton@acme.com", "fromDisplayName": "Planton", "replyTo": "",
				"ssl": "false", "starttls": "false", "auth": "false",
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := (&OwnedRealmEmail{Relay: tc.relay}).smtpServer()
			if !jsonEqual(tc.want, got) {
				t.Errorf("smtpServer = %v, want %v", got, tc.want)
			}
			for key := range got {
				if !slices.Contains(smtpServerOwnedKeys, key) {
					t.Errorf("rendered key %s is not in the owned vocabulary", key)
				}
			}
		})
	}

	if got := (&OwnedRealmEmail{}).smtpServer(); len(got) != 0 {
		t.Errorf("no relay must render the empty off-state, got %v", got)
	}
}

// The owned realm settings follow the input: hands off carries neither email
// key, the off-state carries both with their off values, a relay carries the
// switch on and the rendered map.
func TestOwnedRealmSettings_Email(t *testing.T) {
	handsOff := testInput()
	handsOff.Email = nil
	for _, key := range []string{realmResetPasswordAllowedKey, realmSMTPServerKey} {
		if _, ok := OwnedRealmSettings(handsOff)[key]; ok {
			t.Errorf("hands off must not own %s", key)
		}
	}

	off := OwnedRealmSettings(testInput())
	if off[realmResetPasswordAllowedKey] != false {
		t.Errorf("no declaration must own resetPasswordAllowed=false, got %v", off[realmResetPasswordAllowedKey])
	}
	if !jsonEqual(map[string]string{}, off[realmSMTPServerKey]) {
		t.Errorf("no declaration must own an empty smtpServer, got %v", off[realmSMTPServerKey])
	}

	declared := testInput()
	declared.Email = &OwnedRealmEmail{Relay: passwordRelay()}
	on := OwnedRealmSettings(declared)
	if on[realmResetPasswordAllowedKey] != true {
		t.Error("a declared relay must own resetPasswordAllowed=true")
	}
	if !jsonEqual(declared.Email.smtpServer(), on[realmSMTPServerKey]) {
		t.Error("a declared relay must own the rendered smtpServer map")
	}
}

// liveSMTP is what GetRealm returns for a converged password relay: the same
// keys, the secret masked, plus an admin's own extra.
func liveSMTP(extra map[string]any) map[string]any {
	live := map[string]any{}
	for key, value := range (&OwnedRealmEmail{Relay: passwordRelay()}).smtpServer() {
		live[key] = value
	}
	live["password"] = "**********"
	maps.Copy(live, extra)
	return live
}

// The by-key rule for the one nested owned setting, in the shapes the
// convergence cadence meets: a converged relay writes nothing; a rotated
// Secret writes the masked keys; a moved destination writes the secret with
// it (Keycloak drops a masked secret arriving with a moved destination); a
// changed sign-in mode removes the credential keys the new mode does not
// render; an admin's extra key rides through every write; the off-state
// clears a relay someone typed into the admin console.
func TestConvergeSMTPServer(t *testing.T) {
	t.Run("converged relay is zero drift", func(t *testing.T) {
		live := Representation{realmSMTPServerKey: liveSMTP(nil)}
		if drifted := convergeSMTPServer(live, &OwnedRealmEmail{Relay: passwordRelay()}); drifted != nil {
			t.Errorf("a converged relay must not drift, got %v", drifted)
		}
	})

	t.Run("rotation writes the secret and only names its key", func(t *testing.T) {
		live := Representation{realmSMTPServerKey: liveSMTP(nil)}
		drifted := convergeSMTPServer(live, &OwnedRealmEmail{Relay: passwordRelay(), RotateCredential: true})
		if !jsonEqual([]string{"smtpServer.password (rotated)"}, drifted) {
			t.Errorf("drift = %v", drifted)
		}
		written := live[realmSMTPServerKey].(map[string]any)
		if written["password"] != "relay-secret" {
			t.Error("the rotation write must carry the real password, never the mask")
		}
		for _, path := range drifted {
			if strings.Contains(path, "relay-secret") {
				t.Fatal("a secret value entered a drift report")
			}
		}
	})

	t.Run("a moved destination carries the secret with it", func(t *testing.T) {
		live := Representation{realmSMTPServerKey: liveSMTP(map[string]any{"envelopeFrom": "bounces@acme.com"})}
		relay := passwordRelay()
		relay.Host = "smtp.acme.com"
		drifted := convergeSMTPServer(live, &OwnedRealmEmail{Relay: relay})
		if !jsonEqual([]string{"smtpServer.host"}, drifted) {
			t.Errorf("drift = %v", drifted)
		}
		written := live[realmSMTPServerKey].(map[string]any)
		if written["password"] != "relay-secret" {
			t.Error("any smtpServer write must carry the real secret: Keycloak drops a masked one on a moved destination")
		}
		if written["envelopeFrom"] != "bounces@acme.com" {
			t.Error("an admin's unowned key must ride through the write")
		}
	})

	t.Run("switching to token sign-in removes the password", func(t *testing.T) {
		live := Representation{realmSMTPServerKey: liveSMTP(nil)}
		drifted := convergeSMTPServer(live, &OwnedRealmEmail{Relay: tokenRelay()})
		written := live[realmSMTPServerKey].(map[string]any)
		if _, lingering := written["password"]; lingering {
			t.Error("a password must not linger after the sign-in mode changed")
		}
		if written["authType"] != "token" || written["authTokenClientSecret"] != "client-secret" {
			t.Errorf("token keys not written: %v", written)
		}
		if !slices.Contains(drifted, "smtpServer.password (removed)") {
			t.Errorf("the removal must be reported by key: %v", drifted)
		}
	})

	t.Run("a secret key missing live is written without a rotation signal", func(t *testing.T) {
		live := Representation{realmSMTPServerKey: func() map[string]any { m := liveSMTP(nil); delete(m, "password"); return m }()}
		drifted := convergeSMTPServer(live, &OwnedRealmEmail{Relay: passwordRelay()})
		if !jsonEqual([]string{"smtpServer.password"}, drifted) {
			t.Errorf("drift = %v", drifted)
		}
	})

	t.Run("the off-state clears a hand-typed relay", func(t *testing.T) {
		live := Representation{realmSMTPServerKey: liveSMTP(nil)}
		drifted := convergeSMTPServer(live, &OwnedRealmEmail{})
		if len(drifted) != 1 {
			t.Fatalf("drift = %v", drifted)
		}
		if !jsonEqual(map[string]string{}, live[realmSMTPServerKey]) {
			t.Errorf("smtpServer must be cleared, got %v", live[realmSMTPServerKey])
		}
	})

	t.Run("the off-state on an empty realm is zero drift", func(t *testing.T) {
		live := Representation{realmSMTPServerKey: map[string]any{}}
		if drifted := convergeSMTPServer(live, &OwnedRealmEmail{}); drifted != nil {
			t.Errorf("an empty map already IS the off-state, got %v", drifted)
		}
		if drifted := convergeSMTPServer(Representation{}, &OwnedRealmEmail{}); drifted != nil {
			t.Errorf("an absent map already IS the off-state, got %v", drifted)
		}
	})
}
