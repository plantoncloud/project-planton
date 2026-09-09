# KubernetesCloudNativePgOperator Guide

The judgment this guide carries: one CloudNativePG operator serves every
PostgreSQL cluster on the cluster — and the coupling that actually bites
is not installation but CONFIGURATION: a database's backup declaration is
inert unless THIS operator was installed with the matching plugin.

## Once per cluster, in the shared-cluster chart

Install it once; application environments declare KubernetesPostgres
clusters, never their own operator (the
[Postgres guide](../kubernetespostgres/GUIDE.md) approaches the
same coupling from the database side). The dependency draws no diagram
edge — the
[operator-prerequisite pattern](../../_patterns/operator-prerequisite.md)
is the mechanism. Its "cnpg-system" namespace is the sole-tenant case of
the [namespace-ownership pattern](../../_patterns/namespace-ownership.md).

## The backup-plugin coupling

A KubernetesPostgres declaring `spec.backup` requires this operator
installed with `barmanCloudPlugin.enabled` — the backup objects the
database renders are reconciled by that plugin, and with a plugin-less
operator they are silently inert infrastructure (the Postgres guide
names this from its side; the plugin toggle lives HERE). When any
database in the architecture declares backups, verify this operator's
plugin arm in the same review.

## When CloudNativePG is already on the cluster

Some clusters arrive with CloudNativePG installed by someone else — a
self-hosted Planton installs one for the platform's own database, and a
GitOps or Helm install by the cluster team counts the same. The CRDs and
webhooks are cluster singletons, so a second full install fights the
resident one. The architecture then declares this kind with
`installOperator: false` and `barmanCloudPlugin.enabled: true`: only the
plugin is installed, beside the resident operator, and every
KubernetesPostgres backup block on the cluster starts working. Destroying
that declaration removes the plugin and leaves the operator running
(proven live on GKE). Decide the posture by looking, not guessing:
`kubectl get deploy -A -l app.kubernetes.io/name=cloudnative-pg` — a hit
means plugin-only.

One trap: a CloudNativePG that was uninstalled by a non-Helm owner can
leave its cluster-scoped CRDs, webhook configurations, and RBAC behind
carrying that owner's labels. A full install here then fails Helm's
ownership check ("managed-by must equal Helm") — the leftovers must be
deleted first; nothing here adopts them.

## On the diagram

The operator renders in the shared-cluster layer; databases render in
their environments with no edge to it. Reviewers verify the operator
node exists — and, when backups are declared anywhere, that its plugin
configuration matches.

## Pairs well with

- KubernetesPostgres — the clusters this operator reconciles (see its
  [guide](../kubernetespostgres/GUIDE.md)).
