# The Azure rooms: ten kinds become the rooms their children live in

## What changed

Ten Azure kinds whose metadata already described children "created inside" or "an ARM child of" them now carry `kind_meta.container_kind: true`, so the platform draws those children inside them on a diagram, as the portal's own blades list them. Every reference INTO one of the ten across the whole catalog was read; the ones that are a reach from a kind living elsewhere are marked `containment_exempt` so the new rooms never swallow a neighbour. Each field's comment says why.

- **`AzureVirtualHub` and `AzureVirtualWan`** are containers: ARM deploys the hub's VPN, ExpressRoute, and point-to-site gateways and its spoke connections into the hub, and the hubs and branch sites into the WAN. The hub connection's `remote_virtual_network_id` (the spoke it attaches) is exempt: the connection lives in its hub and reaches the spoke.
- **`AzureTrafficManagerProfile` and `AzureFrontDoorOriginGroup`** are containers: every endpoint is an ARM child of its profile, every origin of its origin group. Exempt: a nested endpoint's `target_profile_id` (it lives in its own parent profile and points at the child), a Front Door route's `origin_group_id` and a rule set's override `origin_group_id` (a route lives in its endpoint and a rule set in its profile; both forward TO the group).
- **`AzureComputeGallery`** is a container: every image definition is an ARM child of its gallery. The image ids a machine, a scale set, or a disk boots from are plain strings and place nothing. Exempt beside it, the compute family's reads: a snapshot's `storage_account_id` and its two legacy-encryption `source_vault_id`s, an image version's `storage_account_id`.
- **`AzureRecoveryServicesVault` and `AzureDataProtectionBackupVault`** are containers: policies, protected items, the storage-account registration, and backup instances are ARM children of a vault. Exempt: a Data Protection backup instance's `storage_account_id` (blob and Data Lake arms) and `snapshot_resource_group_name` (disk and AKS arms) -- what an instance protects or where it parks snapshots is reach, never home.
- **`AzureDataFactory`** is a container: pipelines, data flows, datasets, linked services, triggers, and integration runtimes are all created against a factory's ARM ID. Exempt: the Azure-SSIS runtime's `expressVnetIntegration.subnetId`, `vnetIntegration.vnetId`, and `vnetIntegration.subnetId` -- the runtime lives in its factory and its nodes attach to a network (the AKS node pool's rule).
- **`AzureEventgridTopic` and `AzureEventgridSystemTopic`** are containers: every event subscription is an ARM child created under the topic it listens to (`{topic_id}/providers/Microsoft.EventGrid/eventSubscriptions/{name}`) and cannot exist without it. Exempt: a Data Factory trigger's `eventgrid_topic_id` (the trigger lives in its factory and subscribes to the topic) and an Event Grid namespace's MQTT `route_topic_id` (the namespace lives in its group and routes into the topic).
- **`AzureEventgridDomain` and `AzureEventgridNamespace`** are containers: every domain topic is an ARM child of its domain, every namespace topic of its namespace. Nothing else across the catalog names either kind, so nothing travels with these two marks.
- The containment-decision registry (`shared/cloudresourcekind/testdata/containment_decisions.txt`) gains forty-two `contained` lines -- every typed reference into one of the ten from a kind that lives inside it -- and seventeen `exempt` lines: twelve moved from `contained`, five new because their targets were not rooms before this change. Nothing else moved.

## Why

A diagram is truthful when what lives inside a thing is drawn inside it. Each of these ten kinds is the parent of other catalog kinds in Azure's own resource model, and the metadata said so in words; without the mark, every one of those children named only a parent the platform did not treat as a room, so it had no room at all -- a factory with thirty pipelines drew as a card with thirty leaves scattered outside every wall, a Traffic Manager profile as a card with its endpoints plated on it, an Event Grid topic as a duct with its subscriptions standing outside the resource group pointing back at it. The marks give each child its room; the exemptions keep a neighbour that merely reaches into a room from being drawn inside it.

The ten travel together because every container mark regenerates the same kind-registry descriptor, so separate pull requests conflict with each other the moment the first merges; one change, one regeneration, one merge. The Virtual WAN, Traffic Manager and origin group, Compute Gallery, backup vault, and Data Factory marks were each proposed and described on their own earlier this month; this change supersedes those proposals and carries their reasoning word for word.

## How to check

```bash
go test ./shared/cloudresourcekind/... -run TestContainmentDecisions   # green; 42 placements added, 17 reaches exempt
grep -n -B1 'container_kind: true' shared/cloudresourcekind/cloud_resource_kind.proto | grep -c Azure   # the Azure container kinds, ten more than before
grep -rln containment_exempt catalog/azure --include=spec.proto | wc -l
```
