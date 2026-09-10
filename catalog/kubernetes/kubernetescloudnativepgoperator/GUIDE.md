# KubernetesCloudNativePgOperator Guide

The judgment this guide carries: one CloudNativePG operator serves every
PostgreSQL cluster on the cluster — and backups are NOT part of it. The
operator reconciles databases; the Barman Cloud plugin, a sibling kind
installed into the same namespace, is what makes their backup blocks
work.

## Once per cluster, in the shared-cluster chart

Install it once; application environments declare KubernetesPostgres
clusters, never their own operator (the
[Postgres guide](../kubernetespostgres/GUIDE.md) approaches the
same coupling from the database side). The dependency draws no diagram
edge — the
[operator-prerequisite pattern](../../_patterns/operator-prerequisite.md)
is the mechanism. Its "cnpg-system" namespace is the sole-tenant case of
the [namespace-ownership pattern](../../_patterns/namespace-ownership.md).

## Backups are the plugin's job, in this namespace

A KubernetesPostgres declaring `spec.backup` (or an object-store
`bootstrap.recovery`) renders its Cluster against the Barman Cloud
plugin. That plugin is
[KubernetesCnpgBarmanCloudPlugin](../kubernetescnpgbarmancloudplugin/GUIDE.md),
declared with `namespace` referencing THIS resource — the operator only
discovers plugins in its own namespace. When any database in the
architecture declares backups, verify the plugin node exists beside this
operator in the same review; without it the operator parks the database
in an unknown-plugin phase and it never comes up.

## When CloudNativePG is already on the cluster

Some clusters arrive with CloudNativePG installed by someone else — a
self-hosted platform operator installs one for the platform's own
database, and a GitOps or Helm install by the cluster team counts the
same. The CRDs and webhooks are cluster singletons, so a second full
install fights the resident one: do not declare this kind there. Declare
only what the cluster is missing — usually the plugin kind, with the
resident operator's namespace as a literal. Decide by looking, not
guessing: `kubectl get deploy -A -l app.kubernetes.io/name=cloudnative-pg`
— a hit means an operator is resident.

One trap: a CloudNativePG that was uninstalled by a non-Helm owner can
leave its cluster-scoped CRDs, webhook configurations, and RBAC behind
carrying that owner's labels. An install here then fails Helm's
ownership check ("managed-by must equal Helm") — the leftovers must be
deleted first; nothing here adopts them.

## On the diagram

The operator renders in the shared-cluster layer; databases render in
their environments with no edge to it. Reviewers verify the operator
node exists — and, when backups are declared anywhere, that a plugin
node sits beside it referencing its namespace.

## Pairs well with

- KubernetesCnpgBarmanCloudPlugin — the backup engine, installed into
  this operator's namespace (see its
  [guide](../kubernetescnpgbarmancloudplugin/GUIDE.md)).
- KubernetesPostgres — the clusters this operator reconciles (see its
  [guide](../kubernetespostgres/GUIDE.md)).
