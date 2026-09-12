# Standard

This preset installs the CloudNativePG operator in its standard posture:
the operator release alone, cluster-wide watch scope, pinned chart
version, sized and prioritized for a production control plane. Databases
declared with KubernetesPostgres run, replicate, and fail over; their
backup blocks need the Barman Cloud plugin beside this operator
(KubernetesCnpgBarmanCloudPlugin, its own resource). One installation
per cluster: the CRDs and webhooks are cluster-scoped singletons, and the
release name is fixed to `cnpg`.

## When to Use

- Any cluster that will run KubernetesPostgres databases
- The 30-second choice: this is the standard first CloudNativePG
  installation; add the plugin resource beside it when a database needs
  object-store backups

## Key Configuration Choices

- **Operator only** — the Barman Cloud plugin (and its cert-manager
  dependency) is a separate resource, KubernetesCnpgBarmanCloudPlugin,
  declared into this operator's namespace when a database needs backups;
  adding it later touches nothing here
- **`replicas: 1`** (chart default) — extra replicas are leader-elected
  warm standbys that shorten failover of the operator itself; they add
  no reconciliation throughput (`max_concurrent_reconciles` is that
  knob)
- **Explicit resources** — the chart ships no requests/limits by
  default; a control-plane component should have both
- **`priority_class_name: system-cluster-critical`** — databases stop
  failing over without their operator; it should outlive stateless
  workloads under node pressure
- **`namespace: cnpg-system` + `create_namespace: true`** — the upstream
  convention, in a namespace this resource creates and owns
- **CRDs keep-on-uninstall** — the chart stamps
  `helm.sh/resource-policy: keep` on every CRD unconditionally, so
  removing the component never cascade-deletes the databases

## Placeholders to Replace

None — this preset deploys as-is.

## Related Presets

- **KubernetesCnpgBarmanCloudPlugin / 01-standard** — the backup plugin
  beside this operator, for clusters whose databases declare backup
  blocks
