# KubernetesCnpgBarmanCloudPlugin Pulumi Module

Installs the Barman Cloud plugin for CloudNativePG from the official
Helm chart (`plugin-barman-cloud` at
`https://cloudnative-pg.github.io/charts`) as ONE Helm release in the
operator's namespace. The typed spec renders into chart values in
`module/values.go`; the `helm_values` escape hatch merges over them with
Helm `-f` semantics (maps deep-merge, later document wins, lists
replace); the chart's fixed identities are re-pinned LAST -- the exact
semantic twin of the Terraform module's `helm_release` with
`values = [typed, helm_values, pins]`.

## What the Module Creates

1. **Namespace** (optional) -- created with the standard governance
   labels when `create_namespace` is true; normally false, because the
   namespace is the OPERATOR's and already exists
2. **Helm Release `plugin-barman-cloud`** -- the plugin chart (pinned
   default 0.7.0 = plugin v0.13.0): the plugin Deployment, its
   ServiceAccount and RBAC, the fixed-name gRPC Service `barman-cloud`
   labeled `cnpg.io/pluginName` (how the operator discovers it), the
   config ConfigMap carrying the sidecar image, the cert-manager
   self-signed Issuer plus server and client Certificates for the
   operator-to-plugin TLS, and the ObjectStore CRD (kept on uninstall).
   The release name is FIXED: the chart bakes the Service, Secret, and
   ConfigMap names, and the kept CRD is adopted only by a release of the
   same name and namespace -- one plugin per operator namespace is a
   chart constraint and the name never derives from `metadata.name`

## Rendering Notes

- **The namespace must be the operator's** -- CloudNativePG discovers
  plugin Services only in its own namespace. The spec's `namespace`
  reference resolves to the operator resource's namespace output before
  the module runs; the module renders whatever literal arrives.
- **Split image shape** -- the chart takes both images as `registry` +
  `repository`; `imageMap` splits a combined reference on the
  container-runtime rule (first segment is a registry only when it
  carries a dot or a colon or is `localhost`), so a mirror never renders
  as `ghcr.io/<mirror>/...`. An unset registry keeps the chart default.
- **Chart-default-matching values render only on divergence** --
  `crds.create` only on explicit opt-out; the rendered values stay
  minimal on both engines.
- **Fixed identities re-pinned after the escape hatch** --
  `service.name` (baked into the TLS certificate) and the name overrides
  are merged as the final document, so no `helm_values` content can
  rename what the operator handshake, the E2E verifier, and the import
  recipes key on.
- **CRD keep is upstream policy** -- the chart stamps
  `helm.sh/resource-policy: keep` on the ObjectStore CRD, so
  uninstalling never deletes the ObjectStore resources databases point
  at; no keep knob is needed or modeled.

## Wait / Atomic Posture

The release installs with `Atomic` + `CleanupOnFail` and a 600s timeout,
waiting for readiness. This is where the cert-manager dependency
surfaces: the chart renders cert-manager Issuer/Certificate resources
unconditionally, and without cert-manager on the cluster the
Certificates never become ready -- the release rolls back with a clear
timeout instead of a plugin that silently never registers.

## Usage

```shell
planton pulumi up --manifest e2e/manifest.yaml --module-dir <path-to-this-module>
```

## Outputs

| Output | Description |
|---|---|
| `namespace` | Namespace the plugin runs in -- the operator's namespace |
| `release_name` | Helm release name of the plugin (fixed `plugin-barman-cloud`) |
| `plugin_name` | The CNPG-I identifier a Cluster's `plugins` list names (fixed `barman-cloud.cloudnative-pg.io`) |

## Module Structure

- `main.go`: entrypoint that calls the module
- `module/main.go`: namespace → plugin release → output exports
- `module/values.go`: typed-spec → chart values rendering (CRD
  lifecycle, sizing, the two images, pull secrets, scheduling), the
  escape-hatch merge, and the fixed-identity pins
- `module/locals.go`: resolved namespace and chart version -- kept in
  lockstep with the Terraform module's `locals.tf`
- `module/vars.go`: chart identity, pinned default version (0.7.0 =
  plugin v0.13.0), the fixed release, plugin, and Service names, the
  600s timeout
- `module/helpers.go`: shared shape renderers (resources, tolerations,
  the image split, the Helm `-f` merge)
