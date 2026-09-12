# The operator's README describes the email declaration

## What changed

- **A `spec.email` paragraph under "Declaring a Platform."** The README documented the platform declaration paragraph by paragraph (`spec.version`, the front door, the GitHub declaration) and said nothing about email. It now says what the block carries (a sender identity and exactly one provider: an SMTP relay with every way in, or a Resend account), what admission refuses in words, how the operator carries the declaration to the control plane (environment plus one projected volume of credential files that rotate in place; a missing Secret preflighted and named, never a stuck pod), what `status.email` and the `EMAIL` column echo, how the same declaration owns the identity server's mail settings so "Forgot password?" appears exactly when an email can be sent, and the two setup hints the control plane shows when nothing is declared.
- **The package map names both theme types.** `internal/keycloaklogintheme/` carries the identity server's email theme as well as its sign-in theme, and the map now says so.

## Why

The README is where an adopter reads first. A capability the operator delivers to two senders, with its own admission rules and status column, belongs beside the other declaration paragraphs rather than only in the sample manifest.

## How to check

- Read `operator/README.md` under "Declaring a Platform"; every sentence in the new paragraph is a fact the operator's tests and lab lanes already assert.
