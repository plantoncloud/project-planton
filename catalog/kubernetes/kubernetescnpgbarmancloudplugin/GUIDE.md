# KubernetesCnpgBarmanCloudPlugin Guide

The judgment this guide carries: install the Barman Cloud plugin ONCE,
in the CloudNativePG operator's namespace, before the first database
declares a backup — and never install it twice, from two owners.

## Where it goes and why

CloudNativePG discovers plugins through Services labeled
`cnpg.io/pluginName` in its OWN namespace only. The plugin's `namespace`
is therefore annotated onto the operator resource's namespace output: a
bare `valueFrom` naming the operator resource puts the plugin where it
must be and orders it after the operator. A literal namespace is for a
resident operator someone else installed (`kubectl get deploy -A -l
app.kubernetes.io/name=cloudnative-pg` names it). The "cnpg-system"
namespace stays the sole-tenant case of the
[namespace-ownership pattern](../../_patterns/namespace-ownership.md):
the operator owns it, the plugin joins it (`create_namespace` false).

## The coupling that bites: a backup block with no plugin

A KubernetesPostgres that declares `spec.backup` or an object-store
`bootstrap.recovery` renders the plugin into its Cluster. With no plugin
on the cluster the operator refuses to reconcile it: the Cluster sits in
the phase "Cluster cannot proceed to reconciliation due to an unknown
plugin being required", re-checked every ten seconds, with no instances
created. Loud in `kubectl get cluster`; invisible to a Planton apply that
is simply waiting on Ready. So the plugin is a prerequisite of the BACKUP
BLOCK, not of every database — a plain Postgres needs only the operator
— and it belongs in the shared-cluster chart beside the operator, before
any environment declares a backup. In a chart, give backup-declaring
databases a `depends_on` edge to the plugin so their first reconcile
finds it.

## One owner for the singleton

The chart fixes the plugin's Service name (baked into its TLS
certificate), its two TLS Secrets, its Certificates, and its config
ConfigMap; the release name is fixed to match. Two installers in one
namespace collide on all of them. Whoever installed CloudNativePG should
install its plugin: a self-hosted platform operator that installs
CloudNativePG for its own database and offers a plugin toggle owns the
plugin too — enable it there and do not declare this kind on that
cluster. This kind is for clusters whose operator came without a plugin
toggle: the catalog's operator kind, a bare Helm or GitOps install.

## Uninstall and re-install

Uninstalling the plugin removes its Deployment, Service, RBAC, and
certificates; the ObjectStore CRD stays (the chart's keep policy), so the
ObjectStore resources databases point at survive and their backup
configuration is not lost. Re-installing later adopts the kept CRD
because the release name and namespace match — the adoption path the
fixed release name exists for. Databases whose backups were running keep
their WAL locally in the meantime and resume archiving when the plugin
returns; the operator surfaces the gap as a failing continuous-archiving
condition, not silence.

## Version pairing

Plugin v0.13.0 (chart 0.7.0) requires CloudNativePG 1.26 or later, and
upstream strongly recommends 1.27 or later, which reports plugin errors
on the Cluster's status instead of only in the operator's logs. The
catalog operator's default chart ships 1.30.0. Move the two pins
together and read the plugin's release notes for the floor when moving
against an older resident operator.

## Air-gapped clusters

Two images, two pull paths. The plugin pod pulls `image` with this
kind's `image_pull_secrets`. The SIDECAR image is what every PostgreSQL
instance pod pulls — the plugin publishes it through its config
ConfigMap and injects the container — so mirror `sidecar_image` here and
give the KubernetesPostgres resources their own pull secrets for it.

## On the diagram

The plugin renders in the shared-cluster layer beside the operator, with
its reference edge onto the operator; databases render in their
environments with no edge to it. Reviewers verify the plugin node exists
whenever any database declares backups, and that exactly one exists per
operator namespace.

## Pairs well with

- KubernetesCloudNativePgOperator — the engine it registers with (see
  its [guide](../kubernetescloudnativepgoperator/GUIDE.md)).
- KubernetesCertManager — the chart's TLS issuer; a registry
  prerequisite.
- KubernetesPostgres — whose backup blocks and object-store recoveries
  run through this plugin (see its
  [guide](../kubernetespostgres/GUIDE.md)).
