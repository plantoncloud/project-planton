# Email Through Your Own SMTP Relay

The zero-config platform with outbound email declared once: the control
plane and the identity server both send through your workplace relay,
from your address, so invitations reach inboxes, alerts reach people, and
"Forgot password?" works on the sign-in page.

## When to Use

- A platform a team runs day to day, where invitations should arrive by
  email instead of being shared as links
- Any relay that speaks SMTP: Exchange Online, Google Workspace, an
  internal smart host, or a transactional vendor's SMTP endpoint (SES,
  SendGrid, Postmark, Mailgun, Resend)

## Prerequisites

- A mailbox or relay account allowed to send as `from.address` (SPF and
  DKIM for the domain are the domain owner's job)
- The credentials Secret in the platform's namespace, created BEFORE the
  platform is applied — the operator preflights it and reports a missing
  Secret in words while the platform runs as if no email were declared
- Platform release `v0.0.60` or newer (the first whose control plane
  reads the `smtp` arm) and operator chart 0.14.1 or newer (the first
  whose definition knows `email`)

## Key Configuration Choices

- **Exactly one provider arm** — `smtp` for a relay, or `resend` with
  `api_key_secret_ref` for a Resend account; never both
- **`security` is a promise, not a hint** — `starttls` REQUIRES the
  upgrade and fails a relay that does not offer it; `tls` opens TLS from
  the first byte (port 465 on most relays); `none` is plaintext for
  credential-free internal relays only, and credentials are refused on it
- **One way in** — a username and password (`credentials_secret_name`,
  a `kubernetes.io/basic-auth` Secret), an OAuth2 app registration
  (`oauth2`, SASL XOAUTH2 — Exchange Online after Microsoft's password
  retirement), or no credential at all; not two
- **Credentials are never inline** — Secret names and Secret key
  references only; they reach the control plane as mounted files, so a
  rotated password is live on the next send with no restart
- **Prove it from the console** — Organization Settings, Email runs the
  relay checks on demand (resolve, connect, secure, authenticate) and
  sends a test email to your own address, reporting the relay's answer
  in its own words

## Placeholders to Replace

- `no-reply@planton.example.com` — the address the install sends as
- `it-help@example.com` — where replies land (optional)
- `smtp.office365.com` — your relay's host
- `planton-email` — the name of your credentials Secret

## Related Presets

- **01-zero-config** — the same platform with no email declared;
  invitations are shared as links
- **02-ingress-tls** — expose the platform at a real hostname; combine
  with this preset for a team-ready install
- **03-eks** — the EKS-shaped variant
- **04-gateway-api** — the same URL through a Gateway API Gateway
