# The platform catalog kind has no authorization toggle

## What changed

- **`KubernetesPlantonPlatform.spec.components.authorization` is gone**, and the `KubernetesPlantonPlatformToggle` message with it (it had no other user). The `components` block offers what is genuinely optional on a platform: search (Solr) and the graph explorer (Neo4j).
- **Both engines stop rendering it.** Terraform's `variables.tf` no longer accepts `components.authorization`, `locals.components_body` no longer emits it, and the Pulumi module's `platform_cr.go` no longer copies it into the platform resource. The kind's `README.md` opt-in row, the `cost.yaml` opt-in drawer sentence, and the e2e manifest follow.
- `reference.md` regenerated; `spec.pb.go` regenerated.

## Why

The operator installs the OpenFGA policy engine on every platform it reconciles -- the `PlantonPlatform` definition no longer has `spec.components.authorization`, and a resource that still declares it is refused by the API server's strict field validation. A catalog kind that kept offering the toggle would let a Terraform module or a Pulumi program declare a platform the operator's definition rejects, so the kind and the definition move together, in one release.

## How to check

```bash
go test -count=1 ./catalog/kubernetes/kubernetesplantonplatform/...
go test -count=1 ./pkg/explain/refgen/...                                   # the reference pages are byte-identical to their schema
cd catalog/kubernetes/kubernetesplantonplatform/iac/tf && tofu init -backend=false && tofu validate
rg -n "authorization" catalog/kubernetes/kubernetesplantonplatform/v1alpha1/spec.proto   # nothing under components
```
