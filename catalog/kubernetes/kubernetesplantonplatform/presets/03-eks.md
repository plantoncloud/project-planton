# EKS

The platform shaped for Amazon EKS: gp3 storage, the AWS Load Balancer
Controller serving the hostname with an ACM certificate, and IRSA giving
the runner keyless AWS identity.

## When to Use

- Production or team platforms on EKS

## Prerequisites

- **EBS CSI driver** — an EKS managed addon with its own IAM identity
  (Pod Identity or IRSA). NOT installed by the platform; without it every
  volume stays Pending and the operator's status explains exactly that
- **AWS Load Balancer Controller** for the `alb` ingress class
- **An ACM certificate** for the hostname (TLS terminates at the load
  balancer — hence no in-cluster `tls` block)
- **An IAM role for the runner** trusted by the cluster's OIDC provider
  (IRSA), carrying the policies your deploys need

## Key Configuration Choices

- **`storage.storage_class_name: gp3`** — one setting lifts every
  platform volume off the legacy gp2 class
- **ALB annotations instead of `tls`** — the certificate lives in ACM at
  the edge; `tls` is for in-cluster termination (bring-your-own Secret or
  cert-manager)
- **`ingress.reachability: public`** — with no in-cluster `tls` block the
  platform's default (`auto`) would read this door as private and keep the
  doors that need an inbound path from the internet (keyless cloud
  connections, GitHub webhook delivery) honestly closed. The
  `internet-facing` ALB scheme is the same fact in the controller's
  words, so the preset declares it. An `internal` ALB drops the line.
  Requires operator chart 0.11.0 or newer
- **IRSA over stored keys** — `runner.service_account_annotations` is the
  keyless path; `runner.cloud_credentials_secret_name` exists when static
  keys are the only option (the platform stores nothing either way)

## Placeholders to Replace

- `planton.example.com` — your platform's hostname (point DNS at the ALB)
- The ACM certificate ARN and the runner role ARN

## Related Presets

- **01-zero-config** — start here when the cluster addons are not ready
- **02-ingress-tls** — in-cluster TLS via cert-manager (nginx-style
  ingress controllers)
- **05-email-smtp** — outbound email through your own relay (SES's SMTP
  endpoint included)
