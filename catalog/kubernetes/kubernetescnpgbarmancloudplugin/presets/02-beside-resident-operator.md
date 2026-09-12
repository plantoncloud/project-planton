# Beside Resident Operator

This preset installs the Barman Cloud plugin beside a CloudNativePG that
is ALREADY on the cluster — installed by another hand and without a
backup plugin. The operator's namespace is given as a literal (there is
no operator resource to reference); the plugin joins it, registers with
the resident operator over CNPG-I, and from then on every
KubernetesPostgres backup block on the cluster works. Destroying the
plugin leaves the resident operator running.

## When to Use

- A cluster whose CloudNativePG came from a Helm or GitOps install, or
  from a self-hosted platform operator that installs CloudNativePG for
  its own database but offers no plugin toggle
- The self-hosted management-cluster shape: catalog databases on the same
  cluster the platform runs on, needing backups

## When NOT to Use

- **The operator's installer offers its own plugin toggle** — enable it
  there. The plugin is a singleton per namespace (fixed Service, Secret,
  and ConfigMap names); two owners collide.
- **The catalog's operator is on the cluster** — reference it instead
  (01-standard); a literal that drifts from the operator's namespace
  installs a plugin nobody discovers.

## Key Configuration Choices

- **`namespace` as a literal** — the resident operator's namespace,
  found with `kubectl get deploy -A -l
  app.kubernetes.io/name=cloudnative-pg`; anywhere else the plugin runs
  and the operator never finds it
- **`create_namespace: false`** — the namespace is the resident
  operator's
- **`chart_version: "0.7.0"`** — plugin v0.13.0 requires the resident
  CloudNativePG to be 1.26 or later (1.27 or later recommended); check
  the resident's version before installing
- **cert-manager is a hard prerequisite** — the chart renders
  cert-manager Certificates unconditionally; a resident cert-manager
  serves, a missing one fails the install cleanly (atomic)
- **Everything else as 01-standard** — sized, one replica,
  `system-cluster-critical` priority

## Placeholders to Replace

- **`cnpg-system`** — the resident operator's namespace, if it differs
  from the upstream convention

## Related Presets

- **01-standard** — the same plugin beside the catalog's operator, with
  the namespace by reference
