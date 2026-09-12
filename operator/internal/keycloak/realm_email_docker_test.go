//go:build requires_docker

package keycloak

import (
	"context"
	"testing"
)

// The identity server's email settings against a real Keycloak: the owned
// block lands and reads back with the secret masked; a converged realm writes
// nothing; a rotation is exactly one write; a moved destination carries the
// secret with it (the server would otherwise drop a masked secret arriving
// with a new host -- proven here by the key surviving); the off-state clears
// a hand-typed relay; hands-off leaves one alone.
func TestConvergence_EmailRelay(t *testing.T) {
	admin := authedAdmin(t)
	createRealm(t, admin, "mailer", nil)
	in := convergeInput("mailer")
	in.Email = &OwnedRealmEmail{Relay: passwordRelay(), RotateCredential: true}

	first := mustConverge(t, in)
	if first.Clean() {
		t.Fatal("a fresh realm must gain the relay")
	}
	realm := mustGetRealm(t, admin, "mailer")
	if realm[realmResetPasswordAllowedKey] != true {
		t.Errorf("resetPasswordAllowed = %v, want true with a relay declared", realm[realmResetPasswordAllowedKey])
	}
	live, _ := realm[realmSMTPServerKey].(map[string]any)
	for key, want := range in.Email.smtpServer() {
		if isSMTPServerSecretKey(key) {
			if got, _ := live[key].(string); got == "" || got == want {
				t.Errorf("the server must hold %s and mask it on read, got %q", key, got)
			}
			continue
		}
		if got, _ := live[key].(string); got != want {
			t.Errorf("smtpServer.%s = %q, want %q", key, got, want)
		}
	}

	// Idempotency: the masked secret is never diffed, so a converged relay
	// with no rotation signal is zero writes.
	in.Email.RotateCredential = false
	if second := mustConverge(t, in); !second.Clean() {
		t.Fatalf("second pass must write nothing, wrote %d: %v", second.Writes, second.Repairs)
	}

	// Rotation: exactly one write, named by key.
	in.Email.RotateCredential = true
	rotation := mustConverge(t, in)
	if rotation.Writes != 1 {
		t.Fatalf("rotation must be exactly one write, got %d: %v", rotation.Writes, rotation.Repairs)
	}
	in.Email.RotateCredential = false

	// A moved destination: Keycloak drops a MASKED secret that arrives with a
	// new host; the reconciler carries the real one, so the key survives.
	in.Email.Relay.Host = "smtp.moved.example.com"
	moved := mustConverge(t, in)
	if moved.Writes != 1 {
		t.Fatalf("a moved host must be exactly one write, got %d: %v", moved.Writes, moved.Repairs)
	}
	live, _ = mustGetRealm(t, admin, "mailer")[realmSMTPServerKey].(map[string]any)
	if got, _ := live["host"].(string); got != "smtp.moved.example.com" {
		t.Errorf("host = %q after the move", got)
	}
	if got, _ := live["password"].(string); got == "" {
		t.Error("the password must survive a destination move (the reconciler wrote the real value with it)")
	}
	if third := mustConverge(t, in); !third.Clean() {
		t.Fatalf("after the move the realm must be converged, wrote %d: %v", third.Writes, third.Repairs)
	}

	// The off-state: the relay is removed and the switch turned off.
	in.Email = &OwnedRealmEmail{}
	off := mustConverge(t, in)
	if off.Writes != 1 {
		t.Fatalf("the off-state must be exactly one write, got %d: %v", off.Writes, off.Repairs)
	}
	realm = mustGetRealm(t, admin, "mailer")
	if realm[realmResetPasswordAllowedKey] != false {
		t.Error("resetPasswordAllowed must be off with no relay declared")
	}
	if live, _ := realm[realmSMTPServerKey].(map[string]any); len(live) != 0 {
		t.Errorf("smtpServer must be cleared, got %v", live)
	}
	if again := mustConverge(t, in); !again.Clean() {
		t.Fatalf("the off-state must be steady, wrote %d: %v", again.Writes, again.Repairs)
	}

	// Hands off: a relay an admin typed in stays exactly as typed while the
	// desired state cannot be built.
	realm[realmSMTPServerKey] = map[string]string{"host": "typed.example.com", "from": "someone@example.com"}
	realm[realmResetPasswordAllowedKey] = true
	if err := admin.UpdateRealm(context.Background(), "mailer", realm); err != nil {
		t.Fatal(err)
	}
	in.Email = nil
	if handsOff := mustConverge(t, in); !handsOff.Clean() {
		t.Fatalf("hands off must write nothing, wrote %d: %v", handsOff.Writes, handsOff.Repairs)
	}
	realm = mustGetRealm(t, admin, "mailer")
	if live, _ := realm[realmSMTPServerKey].(map[string]any); live["host"] != "typed.example.com" {
		t.Errorf("hands off must leave the typed relay alone, got %v", live)
	}
}

func mustGetRealm(t *testing.T, admin *AdminClient, realm string) Representation {
	t.Helper()
	rep, err := admin.GetRealm(context.Background(), realm)
	if err != nil {
		t.Fatal(err)
	}
	return rep
}
