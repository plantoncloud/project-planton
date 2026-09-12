# KubernetesCnpgBarmanCloudPlugin Terraform Module

Installs the Barman Cloud plugin for CloudNativePG from the official
Helm chart (`plugin-barman-cloud` at
`https://cloudnative-pg.github.io/charts`) as ONE Helm release in the
operator's namespace. The typed spec renders into chart values in
`locals.tf` (`local.typed_values`); the `helm_values` escape hatch is
passed as a SECOND values document, and the chart's fixed identities as
a THIRD, merged in order by the provider with Helm `-f` semantics -- the
exact semantic twin of the Pulumi module's `buildHelmValues` (typed →
escape hatch → pins).

## Module Behavior

- **The release name is FIXED to `plugin-barman-cloud`** -- the chart
  bakes the plugin's gRPC Service name (`barman-cloud`, embedded in its
  TLS certificate), its TLS Secrets, its Certificates, and its config
  ConfigMap into fixed names, and the ObjectStore CRD it keeps on
  uninstall is adopted only by a release of the same name and namespace.
  One plugin per operator namespace is a chart constraint; the name
  never derives from `metadata.name`.
- **The namespace must be the operator's** -- CloudNativePG discovers
  plugin Services only in its own namespace. The spec's `namespace`
  reference (onto the operator resource's namespace output) is resolved
  to a literal before Terraform runs.
- **cert-manager is a hard dependency** -- the chart renders a
  self-signed Issuer and two Certificates unconditionally; without
  cert-manager the release never becomes ready and rolls back (atomic).
- **Readiness is verified at install time** -- `wait` + `atomic` +
  `cleanup_on_fail` with a 600s timeout, so a plugin that cannot come up
  fails THIS apply instead of surfacing later as databases that never
  reconcile.
- **The module (not Helm) owns namespace creation** -- `create_namespace`
  drives a `kubernetes_namespace_v1` resource carrying the standard
  governance labels (rare here: the namespace is normally the
  operator's); `helm_release.create_namespace` is always false.

## Rendering Quirks

- **Split image shape** -- the chart takes both images as `registry` +
  `repository`; the locals split a combined reference on the
  container-runtime rule (first segment is a registry only when it
  carries a dot or a colon or is `localhost`), so a mirror never renders
  as `ghcr.io/<mirror>/...`. An unset registry keeps the chart default.
- **Fixed identities re-pinned after the escape hatch** --
  `local.fixed_identity_pins` (`service.name`, the name overrides) is
  the last values document, so no `helm_values` content can rename what
  the operator handshake, the E2E verifier, and the import recipes key
  on.
- **Null-prune idiom throughout** -- conditional entries are written as
  `key = cond ? value : null` inside one object literal and pruned, so
  numbers and booleans keep their types in the rendered YAML.
- **`crds.create` renders only on explicit opt-out** -- the CRD carries
  the keep policy either way.

## Resources

| Resource | Condition |
|---|---|
| `kubernetes_namespace_v1.barman_cloud_plugin` | `spec.create_namespace` |
| `helm_release.barman_cloud_plugin` | always |

## Usage

```bash
planton tofu apply --manifest cnpg-barman-plugin.yaml
```

## Local Development

```bash
terraform init
terraform validate
terraform plan -var-file=terraform.tfvars.json
terraform apply -var-file=terraform.tfvars.json
```

## Inputs

See `variables.tf` for the full variable specification (generated from
the spec proto). The spec arrives from the proto→tfvars converter in
snake_case with the `namespace` foreign key
(KubernetesCloudNativePgOperator's namespace output) resolved to a
literal string before Terraform runs.

## Outputs

| Output | Description |
|--------|-------------|
| `namespace` | Namespace the plugin runs in -- the operator's namespace |
| `release_name` | Helm release name of the plugin (fixed `plugin-barman-cloud`) |
| `plugin_name` | The CNPG-I identifier a Cluster's `plugins` list names (fixed `barman-cloud.cloudnative-pg.io`) |

## Parity

Kept in lockstep with the Pulumi module (`../pulumi/module/`): same
chart identity and pinned default version (0.7.0 = plugin v0.13.0), same
fixed release, plugin, and Service names, same values rendering (the
image split, the divergence-only rendering of chart defaults, the
fixed-identity pins merged last), same atomic/wait posture, same
outputs.
