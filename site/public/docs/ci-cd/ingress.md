---
title: "Ingress"
description: "Control whether a service is publicly accessible and configure DNS domain routing."
icon: settings
order: 60
tags:
  - Ingress
  - DNS
  - Networking
  - Service Hub
---

# Ingress

Ingress configuration determines whether a deployed service is reachable from the internet. In Service Hub, ingress is a two-field toggle on the Service spec that connects a deployment to a configured DNS domain.

## Ingress Settings

A Service's ingress configuration has two settings:

| Setting | Purpose |
|---------|---------|
| Ingress enabled | Whether the service is publicly accessible. Disabled by default. |
| DNS domain | The DNS domain to use for ingress routing. Required when ingress is enabled. |

When ingress is enabled, Planton creates the appropriate ingress resources for the deployment target — Kubernetes Ingress objects, load balancer listener rules, or platform-specific routing — and configures the service to be accessible at a subdomain under the selected DNS domain.

When ingress is disabled (the default), the service is deployed but not publicly accessible. This is appropriate for:

- Internal services that communicate only with other services in the cluster
- Background workers and cron jobs
- Services in early development where public access is not yet needed

## DNS Domains

DNS domains are managed as a separate resource in Service Hub. Each domain has:

| Property | Purpose |
|----------|---------|
| Domain name | The DNS domain (e.g., `example.com`, `*.staging.example.com`) |
| Description | Human-readable description of the domain's purpose |

Domains are configured at the organization level and can be referenced by any service in the organization. A single domain can serve multiple services — each service receives its own subdomain.

<!-- SCREENSHOT: Domains management page
  Page: /orgs/{org}/domains
  Action: Show the Domains tab with at least one configured domain
  Focus: The domain list showing domain names and associated services
  Alt: Domains management page in Service Hub showing configured DNS domains
-->

### Managing DNS Domains

#### Web Console

The **Domains** tab on the Service Hub landing page provides:

- List of all configured domains in the organization
- Search and filter
- Add new domains
- Delete existing domains

<!-- SCREENSHOT: DNS domains management
  Page: /orgs/{org}/services (Domains tab)
  Action: Show the domains list with at least 2 configured domains
  Focus: The table showing domain name, description, created by, and actions
  Alt: DNS domains management table showing configured domains with name, description, and delete action
-->

#### Configuring Ingress on a Service

In the web console, ingress is configured in the service details page under the **Overview** or **Settings** tab. The configuration provides:

- An enable/disable toggle
- A dropdown to select from configured DNS domains

<!-- SCREENSHOT: Service ingress configuration
  Page: /orgs/{org}/service/{serviceId} (Overview tab, ingress section)
  Action: Show the ingress configuration with the toggle enabled and a domain selected
  Focus: The ingress toggle and DNS domain dropdown
  Alt: Service ingress configuration showing enabled toggle and DNS domain selection dropdown
-->

## How Ingress Relates to Deployment Targets

The behavior of ingress depends on the deployment target. When ingress is enabled:

- **Kubernetes deployments**: An Ingress resource or ingress controller configuration is created, routing traffic from the domain to the service's pods.
- **AWS ECS**: Load balancer listener rules and target groups are configured.
- **GCP Cloud Run**: The service is mapped to the domain through Cloud Run's domain mapping.
- **Cloudflare Workers**: Workers are bound to routes under the configured domain.

The specific infrastructure provisioned is handled by the deployment component for each target kind. The Service's ingress configuration is the same regardless of the deployment target — the abstraction is intentional.

Two things to know when the hostname is yours rather than composed. A route or ingress you author with its own hostname is an address the platform discovers after it applies (see [How the URL Is Found](/docs/ci-cd/deployment-stage#how-the-url-is-found)) — so it appears on the service page and in `planton service urls` and is probed like any other. And `deploy.hostname` is one label for every environment on purpose: environments differ by the serving DOMAIN they declare, so `api` under `dev.example.com` and `api` under `example.com` are the dev and production addresses of one service, while two environments sharing a domain would collide and are refused at save.

## Related Documentation

- [What is a Service?](/docs/ci-cd/what-is-a-service) — Service configuration overview
- [Deployment Targets](/docs/ci-cd/deployment-targets) — Where services run
- [Pipelines](/docs/ci-cd/pipelines) — The pipeline execution model
