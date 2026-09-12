# api/v1 -- PlantonPlatform CRD Types

This package defines the `PlantonPlatform` custom resource API at `planton.ai/v1`.

## Why This Package Exists

The CRD types are the user contract. Everything a user can configure and everything the operator reports back is defined here. Changes to this package change the external API surface.

## API Group

`apiVersion: planton.ai/v1` -- the domain `planton.ai` serves as the API group. This is a deliberate choice for a single-CRD operator: clean, no group prefix clutter. If the operator grows to manage multiple CRDs, we would introduce a group prefix (e.g., `install.planton.ai/v1`).

## Design Decisions

### Opinionated Defaults

A user should be able to apply a CR with only `spec.version` set and get a working deployment. All storage sizes, component toggles, and ingress settings have sensible defaults:

- PostgreSQL: 10Gi per instance
- Cache (Valkey, redis-protocol): 1Gi
- Ingress: disabled (use port-forward)
- Runner and builds (Tekton): enabled -- deploying and building are the product; opting out is the explicit act
- Optional components (Graph): disabled

### Status Structure

Status uses two complementary mechanisms:

1. **ComponentStatuses**: A structured object with one field per component. This provides clear `kubectl get -o yaml` output and type-safe access from the reconciler. We chose named fields over a map to avoid stringly-typed component names.

2. **Conditions**: Standard `metav1.Condition` slice following Kubernetes API conventions. `Ready` aggregates every enabled component and its message is the `MESSAGE` column of `kubectl get plantonplatform`; `VersionSupported` says whether `spec.version` names a platform release this operator runs, and when it does not, its message names the oldest release the operator supports and the two ways forward.

### Phase Enums

`PlantonPhase` and `ComponentPhase` are string-typed enums validated by kubebuilder markers. They are intentionally separate types because the overall deployment has lifecycle states (Upgrading) that individual components do not.

## Code Generation

After modifying types in this package, run:

```bash
make generate   # Regenerate DeepCopy methods
make manifests  # Regenerate CRD YAML and RBAC
```
