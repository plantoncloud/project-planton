# Gateway API Front Door

The zero-config platform published at a real hostname through a Gateway
API Gateway the cluster already runs — Istio, Envoy Gateway, Cilium, a
cloud Gateway — instead of an Ingress controller. Planton attaches one
route for the hostname to your Gateway and reads its listeners; the
Gateway stays yours and is never modified.

## When to Use

- Clusters whose north-south front door is a Gateway API Gateway (no
  IngressClass at all, or one you would rather not use)
- A Gateway whose HTTPS listener already serves the hostname — then no
  `tls` block is needed; the platform advertises `https://` because the
  listener does

## Prerequisites

- The Gateway API CRDs and a Gateway controller on the cluster
- A Gateway with a listener that admits the hostname (a wildcard counts)
  and allows routes from the platform's namespace
  (`spec.listeners[].allowedRoutes.namespaces`) — the platform's status
  names the exact mismatch when either is missing
- A planton-operator chart that knows `gateway_ref` (0.9.0 or newer). An
  older definition refuses the declaration; the refusal names the resource
  to upgrade (`KubernetesPlantonOperator`, `spec.chart_version`). That
  operator's platform floor is `v0.0.50`, so `version` is at least that
- DNS: point the hostname at the Gateway's address — the operator's status
  names the exact record while it waits

## Key Configuration Choices

- **`gateway_ref` is the fork** — set it INSTEAD of `ingress_class_name`
  (never both); `annotations` are ignored on this door because the Gateway
  API expresses behavior in typed fields
- **`gateway_ref.name` and `namespace` are foreign keys to
  KubernetesGateway** — `value:` for a Gateway created outside Planton (as
  in this preset); `valueFrom:` naming a KubernetesGateway resource when the
  Gateway is Planton's own, so an infra chart declares the Gateway and the
  platform together, the platform deploys after its Gateway, and a renamed
  Gateway carries the route with it. One `valueFrom` pair wires both fields
  from the same resource (its `gateway_name` and `namespace` outputs)
- **`section_name` pins one listener** — omit it to attach to every
  listener whose hostname admits the platform's
- **HTTPS is the listener's** — when the listener already serves the
  hostname, omit `tls`. To have Planton obtain the certificate instead,
  set `tls.issuer`: cert-manager issues it into the platform's namespace,
  a ReferenceGrant lets the Gateway's namespace use it, and the status
  tells you the one `certificateRefs` line to add to your listener.
  `tls.secret_name` does not apply here (the listener owns certificates)
- **Set the hostname BEFORE the first sign-in** — the identity server
  bakes the platform URL into its realm at first boot
- **Never route your own Gateway to the platform's port-forward Service**
  — pages load, but the platform still believes it lives at
  `http://localhost:8080` and sign-in sends the browser there. Declare
  the Gateway on the platform instead

## Placeholders to Replace

- `planton.example.com` — your platform's hostname
- `main` / `istio-ingress` — your Gateway's name and namespace (or replace
  both `value:` blocks with `valueFrom:` against your KubernetesGateway
  resource)
- `https` — the name of the listener serving the hostname (or omit)

## Related Presets

- **02-ingress-tls** — the same URL through an Ingress controller
- **01-zero-config** — no front door; the port-forward door
- **05-email-smtp** — outbound email through your own relay
