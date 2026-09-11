# A Data Factory is the room its pipelines and runtimes live in

## What changed

- **`AzureDataFactory` is a container kind.** Its metadata already said so in words -- "the workspace every other Data Factory resource lives inside: pipelines, data flows, linked services, datasets, triggers, and integration runtimes are all created against a factory's ARM ID" -- and `kind_meta.container_kind: true` now says it to the platform. The six children reference the factory through `data_factory_id`; each of those references is placement, so on a diagram they stand inside the factory.
- **The Azure-SSIS integration runtime's network injection is containment-exempt.** `AzureDataFactoryIntegrationRuntimeSsisExpressVnetIntegration.subnet_id`, `AzureDataFactoryIntegrationRuntimeSsisVnetIntegration.vnet_id`, and `.subnet_id` are the subnet and network the runtime's nodes attach to. The runtime is an ARM child of its factory and lives there; the network is access, not placement -- the AKS node pool's rule. Without the exemption a runtime naming its factory and a subnet would have sat between two rooms with no way to choose. Each field's comment now says so.
- The containment-decision registry (`shared/cloudresourcekind/testdata/containment_decisions.txt`) gains six `contained` lines (every child's `data_factory_id`) and moves exactly three lines from `contained` to `exempt`. Nothing else moved.

## Why

A customer's ETL estate is a factory with its pipelines, datasets, connections, triggers, and compute inside it -- the shape Azure's own portal draws. Without the mark, every one of those six kinds named only a parent the platform did not treat as a room, so they had no room at all: a factory with thirty pipelines drew as a card and thirty leaves scattered outside every wall with lines crossing to it. The Traffic Manager profile, the Front Door origin group, the Virtual WAN hub, and the two backup vaults were marked containers for the same words in their metadata.

The only references INTO a factory across the whole catalog are its six children's, so the mark makes nothing else nest wrongly. A linked service that also references a storage account gets its room only once the storage-account sweep is released alongside this (the linked service's blob and Data Lake endpoints are access there); a reach exempted there stays correct here.

## How to check

```bash
go test ./shared/cloudresourcekind/... -run TestContainmentDecisions   # green; six placements into the factory, three SSIS network lines exempt
grep -n -A6 'AzureDataFactory = 2198' shared/cloudresourcekind/cloud_resource_kind.proto | grep container_kind
grep -rn containment_exempt catalog/azure/azuredatafactoryintegrationruntime --include=spec.proto
```
