# Standard

This preset installs the Barman Cloud plugin beside the catalog's
CloudNativePG operator: the plugin release alone, in the operator's
namespace by reference, pinned, sized, and prioritized with the operator.
From the moment it registers, every KubernetesPostgres backup block and
object-store recovery on the cluster works. One plugin per operator: the
chart fixes the plugin's Service, Secret, and ConfigMap names, and the
release name is fixed to `plugin-barman-cloud`.

## When to Use

- Any cluster running the catalog's KubernetesCloudNativePgOperator whose
  databases will declare backup blocks — WAL archiving and scheduled base
  backups to S3, GCS, Azure Blob, or an S3-compatible store
- The production default: install it in the shared-cluster chart right
  after the operator, before the first database declares a backup — a
  backup block on a cluster without the plugin parks the database in the
  operator's unknown-plugin phase

## Key Configuration Choices

- **`namespace` by reference to the operator resource** — a bare
  `valueFrom` with the operator's name resolves to its namespace output;
  the plugin lands where the operator can discover it and deploys after
  the operator
- **`create_namespace: false`** — the namespace is the operator's; the
  plugin joins it
- **`chart_version: "0.7.0"`** (spec default, plugin v0.13.0) — requires
  CloudNativePG 1.26 or later (1.27 or later recommended); the catalog
  operator's default chart ships 1.30.0
- **`replicas: 1`** (chart default) — extra replicas are leader-elected
  warm standbys that shorten failover of the plugin itself
- **Explicit resources** — the chart ships no requests/limits by default;
  the plugin is light (the backup work runs in a sidecar inside each
  database pod)
- **`priority_class_name: system-cluster-critical`** — archiving falls
  behind while the plugin is evicted; it should outlive stateless
  workloads under node pressure
- **cert-manager is a hard prerequisite** — the chart renders
  cert-manager Certificates unconditionally; without cert-manager the
  release fails to install and rolls back cleanly (atomic)

## Placeholders to Replace

- **`cnpg`** in `namespace.valueFrom.name` — the name of your
  KubernetesCloudNativePgOperator resource

## Related Presets

- **02-beside-resident-operator** — the same plugin beside a
  CloudNativePG someone else installed, with the operator's namespace as
  a literal
