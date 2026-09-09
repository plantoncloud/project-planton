# A Virtual WAN hub is the room its gateways and spoke connections stand in on a diagram

## What changed

- **`AzureVirtualHub` and `AzureVirtualWan` are container kinds.** The kind metadata already said it in words -- ARM deploys a Virtual WAN VPN gateway, ExpressRoute gateway, and point-to-site gateway INTO a virtual hub, a hub connection is an ARM child of the hub, and hubs and branch sites are created into the WAN -- and now says it with `container_kind: true`, so the containment resolvers nest those kinds where Azure puts them: the gateways and the spoke connections inside the hub, the hubs and the VPN sites inside the WAN, the WAN inside its resource group.
- **The hub connection's spoke network is containment-exempt.** `AzureVirtualHubConnectionSpec.remote_virtual_network_id` names the spoke network the connection attaches; the connection lives in the hub and never in that network. Until now the reference was placement by omission, so a hub connection would have been drawn inside the spoke it merely attaches -- and had the exemption landed alone, with the hub not yet a container, the connection would have had no room at all.
- The containment-decision registry (`shared/cloudresourcekind/testdata/containment_decisions.txt`) moves that one line from `contained` to `exempt` and gains twenty-two `contained` lines -- every typed reference INTO the hub or the WAN: the three gateways' and the hub connection's `virtual_hub_id`, the hub's and the VPN site's `virtual_wan_id`, and the hub route-table and route-map references carried by the hub connection, the VPN gateway connection, the point-to-site gateway, and the ExpressRoute gateway's connections (each of which already lives in the hub). Nothing else in the registry moved.

## Why

`container_kind` says a kind is a box other resources nest inside; `containment_exempt` says a reference into such a box is access, not placement. A Virtual WAN hub is Azure's regional router: its gateways are deployed into it (one VPN gateway per hub), and every spoke joins it through a connection that is the hub's own child resource. Drawing the hub as a card with lines to seven gateways and connections hides the picture every Azure architect expects and Azure's own portal draws -- a hub holding its gateways, with its connections at its wall pointing at the spokes. The exemption and the container marks travel together because each is only truthful beside the other.

## How to check

```bash
go test ./shared/cloudresourcekind/... -run TestContainmentDecisions   # green; the golden carries the twenty-two placements and the one exemption
grep -n -A7 'AzureVirtualHub = 2149' shared/cloudresourcekind/cloud_resource_kind.proto | grep container_kind
grep -n containment_exempt catalog/azure/azurevirtualhubconnection/v1alpha1/spec.proto
```
