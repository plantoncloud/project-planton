# Zero Config

This preset declares a complete self-hosted Planton platform with exactly
one decision made: the version. Everything else rides the operator's
defaults — databases, cache, workflow engine, identity server, secrets
manager, and the in-cluster runner all deploy; console, API, and sign-in
share one origin behind the built-in gateway; and the first person to
open the console becomes the admin using a setup code read from a Secret.

## When to Use

- The first platform on any cluster — evaluation, a team install, a lab
- Any time you want a working platform BEFORE deciding about hostnames,
  TLS, or storage classes: every refinement is a later spec edit

## After Deploy

The two commands you need are this resource's outputs:

- `port_forward_command` — opens the platform's door on your machine
  (`http://localhost:8080`)
- `setup_code_command` — reads the setup code the console's first-visit
  page asks for

## Key Configuration Choices

- **`version` is the only choice** — required and never defaulted, so a
  platform upgrade is always a deliberate one-line edit
- **No ingress** — the built-in gateway plus port-forward is the
  zero-config door; add `ingress` later for a real URL (the identity
  server bakes the URL at first boot, so set a hostname BEFORE the first
  sign-in if you know you want one)
- **Secrets manager on** (the default) — connection secrets work with
  zero configuration
- **Runner on** (the default) — the platform can deploy real
  infrastructure out of the box; give it cloud identity when you connect
  a cloud account

## Placeholders to Replace

None — this preset deploys as-is.

## Related Presets

- **02-ingress-tls** — a real hostname over HTTPS via cert-manager
- **03-eks** — EKS posture: gp3 storage, ALB ingress, IRSA runner
  identity
- **04-gateway-api** — a real hostname through a Gateway API Gateway
- **05-email-smtp** — the same platform sending email through your own
  relay, so invitations arrive instead of being shared as links
