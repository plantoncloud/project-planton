# The platform catalog kind offers no Search component

## What changed

- **`KubernetesPlantonPlatform.spec.components.search` is gone**, and the `KubernetesPlantonPlatformSearch` and `KubernetesPlantonPlatformZookeeper` messages with it (search was their only user). **`spec.prerequisites.solr_operator` is gone** beside it. The `components` block offers the one component that is genuinely optional on a platform, the graph explorer (Neo4j); `prerequisites` offers the two sub-operators a platform rides, CloudNativePG and Tekton Pipelines.
- **Both engines stop rendering them.** Terraform's `variables.tf` no longer accepts `components.search` or `prerequisites.solr_operator`, `locals.components_body` and `locals.prerequisites_body` no longer emit them, and the Pulumi module's `platform_cr.go` no longer copies them into the platform resource. The kind's `README.md` opt-in row, `catalog.md`'s sub-operator sentence, the `cost.yaml` opt-in drawer sentence, the e2e manifest, and the operator API README's default-footprint list follow.
- `reference.md`, `spec.pb.go`, and the proto-docs index regenerated.

## Why

The `PlantonPlatform` definition has never carried a search component or a Solr sub-operator: search on a platform is the control plane's built-in PostgreSQL projection, with no engine to select. A catalog kind that kept offering them let a Terraform module, a Pulumi program, or the console's wizard declare a platform the API server refuses under strict field validation -- and no gate could see it, because the gates compare the console against the kind's own schema, not against the operator's definition. The kind now describes exactly what the operator accepts.

## How to check

```bash
go test -count=1 ./catalog/kubernetes/kubernetesplantonplatform/v1alpha1
go test -count=1 -run TestReferenceDrift ./pkg/explain/refgen              # the reference pages are byte-identical to their schema
cd catalog/kubernetes/kubernetesplantonplatform/iac/tf && tofu init -backend=false && tofu validate
rg -n -i "solr|zookeeper|search" catalog/kubernetes/kubernetesplantonplatform  # nothing
```
