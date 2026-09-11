# Ingress + TLS

The zero-config platform exposed at a real hostname over HTTPS: the
cluster's ingress controller serves one origin for console, API, and
sign-in, and cert-manager issues (and renews) the certificate.

## When to Use

- A platform your team reaches by URL, not port-forward
- Clusters that already run an ingress controller and cert-manager

## Prerequisites

- An ingress controller (the cluster default class, or name one with
  `ingress_class_name`)
- cert-manager with the issuer named below (or replace `issuer` with
  `secret_name` and bring your own `kubernetes.io/tls` Secret)
- DNS: point the hostname at the ingress controller's address — the
  operator's status names the exact record while it waits

## Key Configuration Choices

- **Set the hostname BEFORE the first sign-in** — the identity server
  bakes the platform URL into its realm at first boot
- **`tls` requires `hostname`** (a certificate cannot be issued for an
  auto-derived address), and takes EXACTLY one of `issuer` or
  `secret_name`
- **Magic DNS alternative** — `enabled: true` alone (no hostname) derives
  a working URL from the controller's published address, for clusters
  without a domain

## Placeholders to Replace

- `planton.example.com` — your platform's hostname
- `letsencrypt` — your cert-manager issuer

## Related Presets

- **01-zero-config** — no ingress; the port-forward door
- **03-eks** — the EKS-shaped variant (ALB annotations, gp3, IRSA)
- **04-gateway-api** — the same URL through a Gateway API Gateway
  instead of an Ingress controller
- **05-email-smtp** — add outbound email through your own relay for a
  team-ready install
